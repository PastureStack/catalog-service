package aimetadata

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/PastureStack/catalog-service/internal/launchoptions"
	"github.com/PastureStack/catalog-service/parse"
)

type validateHandler struct{ source LaunchSource }

func NewLaunchValidateHandler(source LaunchSource) http.Handler {
	return &validateHandler{source: source}
}

// Private validation never creates resources or treats an ID as permission.
func (h *validateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		fail(w, 405, "CatalogLaunchMethodDenied")
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		fail(w, 403, "CatalogLaunchCallerDenied")
		return
	}
	project := r.Header.Get(ProjectHeader)
	if !projectID.MatchString(project) || len(r.Header.Values(ProjectHeader)) != 1 {
		fail(w, 403, "CatalogLaunchProjectDenied")
		return
	}
	if r.URL.RawQuery != "" {
		fail(w, 422, "CatalogLaunchInputInvalid")
		return
	}
	raw := http.MaxBytesReader(w, r.Body, 256*1024)
	decoder := json.NewDecoder(raw)
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var input struct {
		TemplateID   string                 `json:"templateId"`
		VersionID    string                 `json:"versionId"`
		Digest       string                 `json:"digest"`
		SafeAnswers  map[string]interface{} `json:"safeAnswers"`
		HumanAnswers map[string]interface{} `json:"humanAnswers"`
		References   map[string]interface{} `json:"referenceBindings"`
	}
	if decoder.Decode(&input) != nil || !metadataID.MatchString(input.TemplateID) || !versionID.MatchString(input.VersionID) || len(input.Digest) != 64 {
		fail(w, 422, "CatalogLaunchInputInvalid")
		return
	}
	var extra interface{}
	if decoder.Decode(&extra) != io.EOF {
		fail(w, 422, "CatalogLaunchInputInvalid")
		return
	}
	if h.source == nil {
		fail(w, 503, "CatalogLaunchUnavailable")
		return
	}
	references := map[string]string{}
	for name, value := range input.References {
		text, ok := value.(string)
		if !ok {
			fail(w, 422, "CatalogLaunchInputInvalid")
			return
		}
		references[name] = text
	}
	material, err := h.source.ReadLaunch(r.Context(), project, input.TemplateID, input.VersionID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errCursorStale) {
		fail(w, 409, "CatalogLaunchSourceChanged")
		return
	}
	if err != nil || material == nil {
		fail(w, 503, "CatalogLaunchUnavailable")
		return
	}
	if subtle.ConstantTimeCompare([]byte(material.Digest), []byte(input.Digest)) != 1 {
		fail(w, 409, "CatalogLaunchSourceChanged")
		return
	}
	definitions, err := parse.QuestionsFromFiles(material.Files)
	if err != nil {
		fail(w, 422, "CatalogLaunchUnsupported")
		return
	}
	variables, missing, err := launchoptions.ValidateLaunch(definitions, input.SafeAnswers, input.HumanAnswers, references)
	if err != nil {
		code := "CatalogLaunchInputInvalid"
		if errors.Is(err, launchoptions.ErrUnsupported) {
			code = "CatalogLaunchUnsupported"
		}
		if errors.Is(err, launchoptions.ErrOptionUnknown) {
			code = "CatalogLaunchOptionUnknown"
		}
		fail(w, 422, code)
		return
	}
	explicitStrings := map[string]string{}
	for name, value := range variables {
		explicitStrings[name] = fmt.Sprint(value)
	}
	writeJSON(w, 200, struct {
		Digest    string                   `json:"digest"`
		Variables map[string]string        `json:"variables"`
		Missing   []launchoptions.Question `json:"missingHumanInputs"`
	}{material.Digest, explicitStrings, missing})
}
