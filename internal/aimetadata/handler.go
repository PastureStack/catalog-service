// Package aimetadata owns the bounded Catalog metadata projection. The Engine
// authorizes the real caller and Key; this private same-container route accepts
// only its loopback transport and effective project, never forwarded client IPs.
package aimetadata

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"unicode/utf16"
)

const Path = "/v1-catalog/ai/templates"
const PageSize = 50
const ProjectHeader = "X-API-Project-Id"

var projectID = regexp.MustCompile(`^1a[1-9][0-9]{0,18}$`)
var metadataID = regexp.MustCompile(`^ctm-[0-9a-f]{64}$`)

// Template is the complete output allowlist. Compose files, labels, questions,
// default answers, URLs, credentials, icons and readme are never loaded/copied.
type Template struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	CatalogID       string   `json:"catalogId"`
	TemplateBase    string   `json:"templateBase"`
	CatalogScope    string   `json:"catalogScope"`
	DefaultVersion  string   `json:"defaultVersion"`
	Maintainer      string   `json:"maintainer"`
	License         string   `json:"license"`
	TruncatedFields []string `json:"truncatedFields,omitempty"`
	rowID           int64
}

type Query struct {
	Text          string
	AfterID       int64
	AfterIdentity string
	ExactID       string
	Versions      bool
	VersionID     string
}

// Source applies the effective project/global visibility and keyset condition
// before its fixed SQL limit. It must return strictly increasing persisted IDs.
type Source interface {
	Read(context.Context, string, Query, int) ([]Template, error)
}

type Handler struct{ source Source }

func NewHandler(source Source) http.Handler { return &Handler{source: source} }

type response struct {
	ResourceType string     `json:"resourceType"`
	Data         []Template `json:"data"`
	NextMarker   *string    `json:"nextMarker"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		fail(w, http.StatusMethodNotAllowed, "CatalogMetadataMethodDenied")
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		fail(w, http.StatusForbidden, "CatalogMetadataCallerDenied")
		return
	}
	project := r.Header.Get(ProjectHeader)
	if !projectID.MatchString(project) || len(r.Header.Values(ProjectHeader)) != 1 {
		fail(w, http.StatusForbidden, "CatalogMetadataProjectDenied")
		return
	}
	query, ok := parseQuery(r, project)
	if !ok {
		fail(w, http.StatusUnprocessableEntity, "CatalogMetadataInputInvalid")
		return
	}
	if h.source == nil {
		fail(w, http.StatusServiceUnavailable, "CatalogMetadataUnavailable")
		return
	}
	if query.Versions {
		h.serveVersions(w, r, project, query)
		return
	}
	rows, err := h.source.Read(r.Context(), project, query, PageSize+1)
	if errors.Is(err, errCursorStale) {
		fail(w, http.StatusConflict, "CatalogMetadataCursorStale")
		return
	}
	if err != nil || len(rows) > PageSize+1 {
		fail(w, http.StatusServiceUnavailable, "CatalogMetadataUnavailable")
		return
	}
	prior := query.AfterID
	seen := map[string]bool{}
	for i := range rows {
		if !metadataID.MatchString(rows[i].ID) || rows[i].rowID <= prior || seen[rows[i].ID] || query.ExactID != "" && rows[i].ID != query.ExactID ||
			(rows[i].CatalogScope != "global" && rows[i].CatalogScope != "project") {
			fail(w, http.StatusServiceUnavailable, "CatalogMetadataUnavailable")
			return
		}
		prior = rows[i].rowID
		seen[rows[i].ID] = true
		rows[i].Type = "catalogTemplate"
		rows[i].TruncatedFields = nil
		clipField(&rows[i].Name, "name", 512, &rows[i].TruncatedFields)
		clipField(&rows[i].Description, "description", 1024, &rows[i].TruncatedFields)
		clipField(&rows[i].CatalogID, "catalogId", 512, &rows[i].TruncatedFields)
		clipField(&rows[i].TemplateBase, "templateBase", 512, &rows[i].TruncatedFields)
		clipField(&rows[i].DefaultVersion, "defaultVersion", 512, &rows[i].TruncatedFields)
		clipField(&rows[i].Maintainer, "maintainer", 512, &rows[i].TruncatedFields)
		clipField(&rows[i].License, "license", 512, &rows[i].TruncatedFields)
	}
	if query.ExactID != "" && len(rows) != 1 {
		fail(w, http.StatusNotFound, "CatalogMetadataNotFound")
		return
	}
	result := response{ResourceType: "catalogTemplate", Data: []Template{}}
	if len(rows) > PageSize {
		rows = rows[:PageSize]
		last := rows[len(rows)-1]
		marker := cursor(project, query.Text, last.rowID, last.ID)
		result.NextMarker = &marker
	}
	result.Data = append(result.Data, rows...)
	writeJSON(w, http.StatusOK, result)
}

func parseQuery(r *http.Request, project string) (Query, bool) {
	if len(r.URL.RawQuery) > 4096 {
		return Query{}, false
	}
	// URL.Query silently drops malformed escapes; ParseQuery must fail closed.
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return Query{}, false
	}
	for key, values := range values {
		if len(values) != 1 || (key != "query" && key != "marker" && key != "templateId" && key != "versions" && key != "versionId") {
			return Query{}, false
		}
	}
	query := Query{Text: values.Get("query")}
	if len(utf16.Encode([]rune(query.Text))) > 256 {
		return Query{}, false
	}
	if values.Has("templateId") {
		if values.Has("query") || values.Has("marker") && values.Get("versions") != "true" {
			return Query{}, false
		}
		id := values.Get("templateId")
		if !metadataID.MatchString(id) {
			return Query{}, false
		}
		query.ExactID = id
	}
	if values.Has("versions") || values.Has("versionId") {
		if values.Get("versions") != "true" || query.ExactID == "" || values.Has("query") {
			return Query{}, false
		}
		query.Versions = true
		if values.Has("versionId") {
			if !versionID.MatchString(values.Get("versionId")) || values.Has("marker") {
				return Query{}, false
			}
			query.VersionID = values.Get("versionId")
		}
	}
	if values.Has("marker") {
		binding := query.Text
		if query.Versions {
			binding = query.ExactID
		}
		id, identity, valid := parseCursor(values.Get("marker"), project, binding)
		if !valid {
			return Query{}, false
		}
		query.AfterID = id
		query.AfterIdentity = identity
		if query.Versions {
			query.AfterIdentity = "cvm-" + identity[4:]
		}
	}
	return query, true
}

func clipField(value *string, field string, limit int, truncated *[]string) {
	units := 0
	for offset, char := range *value {
		size := 1
		if char > 0xffff {
			size = 2
		}
		if units+size > limit {
			*value = (*value)[:offset]
			*truncated = append(*truncated, field)
			return
		}
		units += size
	}
}

func fail(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}
func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
