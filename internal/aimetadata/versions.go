package aimetadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
)

var versionID = regexp.MustCompile(`^cvm-[0-9a-f]{64}$`)

// Only version selection metadata. Files, questions, defaults and readme are
// deliberately not queried, and version bounds are not a compatibility verdict.
type Version struct {
	ID                    string   `json:"id"`
	Type                  string   `json:"type"`
	TemplateID            string   `json:"templateId"`
	Version               string   `json:"version"`
	Revision              *int64   `json:"revision"`
	MinimumRancherVersion string   `json:"minimumRancherVersion"`
	MaximumRancherVersion string   `json:"maximumRancherVersion"`
	UpgradeFrom           string   `json:"upgradeFrom"`
	TruncatedFields       []string `json:"truncatedFields,omitempty"`
	rowID                 int64
}

type versionSource interface {
	ReadVersions(context.Context, string, Query, int) ([]Version, error)
}

func versionLogicalID(template, version string, revision *int64) string {
	encoded, _ := json.Marshal([]interface{}{"catalog-version-v1", template, version, revision})
	digest := sha256.Sum256(encoded)
	return "cvm-" + hex.EncodeToString(digest[:])
}

func (h *Handler) serveVersions(w http.ResponseWriter, r *http.Request, project string, query Query) {
	source, ok := h.source.(versionSource)
	if !ok {
		fail(w, 503, "CatalogMetadataUnavailable")
		return
	}
	rows, err := source.ReadVersions(r.Context(), project, query, PageSize+1)
	if errors.Is(err, errCursorStale) {
		fail(w, 409, "CatalogMetadataCursorStale")
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "CatalogMetadataNotFound")
		return
	}
	if err != nil || len(rows) > PageSize+1 {
		fail(w, 503, "CatalogMetadataUnavailable")
		return
	}
	prior := query.AfterID
	seen := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		if !versionID.MatchString(row.ID) || row.rowID <= prior || seen[row.ID] || row.TemplateID != query.ExactID ||
			query.VersionID != "" && row.ID != query.VersionID {
			fail(w, 503, "CatalogMetadataUnavailable")
			return
		}
		prior = row.rowID
		seen[row.ID] = true
		row.Type = "catalogTemplateVersion"
		row.TruncatedFields = nil
		clipField(&row.Version, "version", 512, &row.TruncatedFields)
		clipField(&row.MinimumRancherVersion, "minimumRancherVersion", 512, &row.TruncatedFields)
		clipField(&row.MaximumRancherVersion, "maximumRancherVersion", 512, &row.TruncatedFields)
		clipField(&row.UpgradeFrom, "upgradeFrom", 512, &row.TruncatedFields)
	}
	if query.VersionID != "" && len(rows) != 1 {
		fail(w, 404, "CatalogMetadataNotFound")
		return
	}
	var marker *string
	if len(rows) > PageSize {
		rows = rows[:PageSize]
		last := rows[len(rows)-1]
		value := cursor(project, query.ExactID, last.rowID, "ctm-"+last.ID[4:])
		marker = &value
	}
	if rows == nil {
		rows = []Version{}
	}
	writeJSON(w, 200, struct {
		ResourceType string    `json:"resourceType"`
		Data         []Version `json:"data"`
		NextMarker   *string   `json:"nextMarker"`
	}{"catalogTemplateVersion", rows, marker})
}

func (s SQLSource) ReadVersions(ctx context.Context, project string, query Query, limit int) ([]Version, error) {
	if !query.Versions || !metadataID.MatchString(query.ExactID) || limit != PageSize+1 {
		return nil, errUnavailable
	}
	// Resolve the parent from authorized rows each time. Private cache IDs are
	// never caller input or a durable authorization target.
	parents, err := s.Read(ctx, project, Query{ExactID: query.ExactID}, PageSize+1)
	if err != nil {
		return nil, err
	}
	if len(parents) != 1 {
		return nil, sql.ErrNoRows
	}
	parent := parents[0]
	if query.AfterID > 0 {
		anchor, err := s.versions(ctx, project, parent, query.AfterID, 0, 1)
		if err != nil {
			return nil, err
		}
		if len(anchor) != 1 || anchor[0].ID != query.AfterIdentity {
			return nil, errCursorStale
		}
	}
	if query.VersionID != "" {
		if !versionID.MatchString(query.VersionID) {
			return nil, errUnavailable
		}
		rows, err := s.versions(ctx, project, parent, 0, 0, maxDetailRows+1)
		if err != nil {
			return nil, err
		}
		if len(rows) > maxDetailRows {
			return nil, errUnavailable
		}
		for _, row := range rows {
			if row.ID == query.VersionID {
				return []Version{row}, nil
			}
		}
		return nil, nil
	}
	return s.versions(ctx, project, parent, 0, query.AfterID, limit)
}

func (s SQLSource) versions(ctx context.Context, project string, parent Template, anchor, after int64, limit int) ([]Version, error) {
	statement := `SELECT v.id, v.version, v.revision, v.minimum_rancher_version, v.maximum_rancher_version, v.upgrade_from,
t.environment_id, c.name, c.url, c.branch, c.pinned_commit, t.folder_name, t.base
FROM catalog_version v JOIN catalog_template t ON v.template_id = t.id JOIN catalog c ON c.id = t.catalog_id
WHERE (t.environment_id = ? OR t.environment_id = 'global') AND c.environment_id = t.environment_id AND t.id = ?`
	args := []interface{}{project, parent.rowID}
	if anchor > 0 {
		statement += " AND v.id = ?"
		args = append(args, anchor)
	} else {
		statement += " AND v.id > ?"
		args = append(args, after)
	}
	statement += " ORDER BY v.id ASC LIMIT ?"
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Version{}
	for rows.Next() {
		var id int64
		var version, minimum, maximum, upgrade, environment, catalog, repo, branch, pin, folder, base sql.NullString
		var revision sql.NullInt64
		if err := rows.Scan(&id, &version, &revision, &minimum, &maximum, &upgrade, &environment, &catalog, &repo, &branch, &pin, &folder, &base); err != nil {
			return nil, err
		}
		if id <= 0 || revision.Valid && (revision.Int64 < 0 || revision.Int64 > 9007199254740991) {
			return nil, errUnavailable
		}
		current, err := logicalID(identity{Scope: environment.String, Catalog: catalog.String, Repo: repo.String,
			Branch: branch.String, Pin: pin.String, Folder: folder.String, Base: base.String})
		if err != nil || current != parent.ID {
			return nil, errCursorStale
		}
		var rev *int64
		if revision.Valid {
			value := revision.Int64
			rev = &value
		}
		result = append(result, Version{ID: versionLogicalID(parent.ID, version.String, rev), TemplateID: parent.ID, Version: version.String,
			Revision: rev, MinimumRancherVersion: minimum.String, MaximumRancherVersion: maximum.String, UpgradeFrom: upgrade.String, rowID: id})
	}
	return result, rows.Err()
}
