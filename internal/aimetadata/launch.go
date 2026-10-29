package aimetadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/PastureStack/catalog-service/internal/launchoptions"
	"github.com/PastureStack/catalog-service/model"
	"github.com/PastureStack/catalog-service/parse"
)

const LaunchOptionsPath = "/v1-catalog/ai/launch-options"
const LaunchMaterialPath = "/v1-catalog/ai/launch-material"
const LaunchValidatePath = "/v1-catalog/ai/launch-validate"
const maxLaunchBytes = 1024 * 1024
const maxLaunchFiles = 64
const MaxLaunchResponseBytes = 4 * 1024 * 1024

// LaunchMaterial is exclusively server-to-server input. It must never be
// projected through a public resource schema, audit payload or model tool.
type LaunchMaterial struct {
	TemplateID     string                   `json:"templateId"`
	VersionID      string                   `json:"versionId"`
	Template       Template                 `json:"template"`
	Version        Version                  `json:"version"`
	ExternalID     string                   `json:"externalId"`
	Digest         string                   `json:"digest"`
	Files          map[string]string        `json:"files"`
	Questions      []model.Question         `json:"questions"`
	QuestionSchema []launchoptions.Question `json:"questionSchema"`
}

type LaunchSource interface {
	ReadLaunch(context.Context, string, string, string) (*LaunchMaterial, error)
}

type launchHandler struct {
	source   LaunchSource
	material bool
}

func NewLaunchOptionsHandler(source LaunchSource) http.Handler {
	return &launchHandler{source: source}
}

func NewLaunchMaterialHandler(source LaunchSource) http.Handler {
	return &launchHandler{source: source, material: true}
}

func (h *launchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		fail(w, 405, "CatalogLaunchMethodDenied")
		return
	}
	// Only the actual peer is trusted. Forwarded headers and supplied roles
	// cannot turn a remote caller into the Engine's same-container transport.
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
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 512 || len(values) != 2 || len(values["templateId"]) != 1 ||
		len(values["versionId"]) != 1 || !metadataID.MatchString(values.Get("templateId")) || !versionID.MatchString(values.Get("versionId")) {
		fail(w, 422, "CatalogLaunchInputInvalid")
		return
	}
	if h.source == nil {
		fail(w, 503, "CatalogLaunchUnavailable")
		return
	}
	material, err := h.source.ReadLaunch(r.Context(), project, values.Get("templateId"), values.Get("versionId"))
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "CatalogMetadataNotFound")
		return
	}
	if errors.Is(err, errCursorStale) {
		fail(w, 409, "CatalogLaunchSourceChanged")
		return
	}
	if err != nil || material == nil {
		fail(w, 503, "CatalogLaunchUnavailable")
		return
	}
	questions, err := parse.QuestionsFromFiles(material.Files)
	if err != nil {
		fail(w, 422, "CatalogLaunchUnsupported")
		return
	}
	safe, err := launchoptions.Build(questions)
	if err != nil {
		fail(w, 422, "CatalogLaunchUnsupported")
		return
	}
	if h.material {
		material.Questions = questions
		material.QuestionSchema = safe
		writeLaunchJSON(w, material)
		return
	}
	// Neither private source digest nor any default crosses this safe route.
	writeJSON(w, 200, struct {
		Questions []launchoptions.Question `json:"questions"`
	}{safe})
}

func writeLaunchJSON(w http.ResponseWriter, value interface{}) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded)+1 > MaxLaunchResponseBytes {
		fail(w, 422, "CatalogLaunchUnsupported")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write(append(encoded, '\n'))
}

func (s SQLSource) ReadLaunch(ctx context.Context, project, templateID, selectedID string) (*LaunchMaterial, error) {
	versions, err := s.ReadVersions(ctx, project, Query{ExactID: templateID, Versions: true, VersionID: selectedID}, PageSize+1)
	if err != nil {
		return nil, err
	}
	if len(versions) != 1 {
		return nil, sql.ErrNoRows
	}
	// One joined statement captures the current source/version/files. Recheck
	// logical identity after resolving private cache IDs to prevent cache races.
	rows, err := s.DB.QueryContext(ctx, "SELECT t.environment_id, c.name, c.url, c.branch, c.pinned_commit, c.`commit`,\n"+`
t.folder_name, t.base, t.name, v.version, v.revision, f.name, SUBSTR(f.contents, 1, 1048577)
FROM catalog_version v JOIN catalog_template t ON v.template_id = t.id JOIN catalog c ON c.id = t.catalog_id
LEFT JOIN catalog_file f ON f.version_id = v.id
WHERE (t.environment_id = ? OR t.environment_id = 'global') AND c.environment_id = t.environment_id AND v.id = ?
ORDER BY f.id ASC LIMIT ?`, project, versions[0].rowID, maxLaunchFiles+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	material := &LaunchMaterial{TemplateID: templateID, VersionID: selectedID, Files: map[string]string{}}
	total, count := 0, 0
	var sourceCommit string
	for rows.Next() {
		var environment, catalog, repo, branch, pin, commit, folder, base, name, version, file, contents sql.NullString
		var revision sql.NullInt64
		if err := rows.Scan(&environment, &catalog, &repo, &branch, &pin, &commit, &folder, &base, &name, &version, &revision, &file, &contents); err != nil {
			return nil, err
		}
		logical, err := logicalID(identity{Scope: environment.String, Catalog: catalog.String, Repo: repo.String,
			Branch: branch.String, Pin: pin.String, Folder: folder.String, Base: base.String})
		var rev *int64
		if revision.Valid {
			value := revision.Int64
			if value < 0 || value > 9007199254740991 {
				return nil, errUnavailable
			}
			rev = &value
		}
		if err != nil || logical != templateID || versionLogicalID(logical, version.String, rev) != selectedID || !safeTemplateKey(folder.String) {
			return nil, errCursorStale
		}
		scope := "project"
		if environment.String == "global" {
			scope = "global"
		}
		material.Template = Template{ID: logical, Name: name.String, CatalogID: catalog.String, TemplateKey: folder.String, TemplateBase: base.String, CatalogScope: scope}
		material.Version = versions[0]
		material.Version.rowID = 0
		legacyVersion := model.Version{Version: version.String}
		if rev != nil {
			numeric := int(*rev)
			legacyVersion.Revision = &numeric
		}
		material.ExternalID = "catalog://" + model.VersionAPIID(catalog.String, model.Template{FolderName: folder.String, Base: base.String}, legacyVersion)
		sourceCommit = commit.String
		if !file.Valid {
			continue
		}
		count++
		total += len(file.String) + len(contents.String)
		_, duplicate := material.Files[file.String]
		if count > maxLaunchFiles || total > maxLaunchBytes || !safeLaunchFile(file.String) || duplicate {
			return nil, errUnavailable
		}
		material.Files[file.String] = contents.String
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, sql.ErrNoRows
	}
	encoded, err := json.Marshal([]interface{}{"catalog-launch-v1", templateID, selectedID, sourceCommit, material.Files})
	if err != nil {
		return nil, errUnavailable
	}
	digest := sha256.Sum256(encoded)
	material.Digest = hex.EncodeToString(digest[:])
	return material, nil
}

func safeLaunchFile(name string) bool {
	return name != "" && len(name) <= 512 && path.Clean(name) == name && name != "." &&
		!strings.HasPrefix(name, "/") && name != ".." && !strings.HasPrefix(name, "../") &&
		!strings.ContainsAny(name, "\\:\x00\r\n")
}
