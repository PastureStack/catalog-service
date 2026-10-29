package aimetadata

import (
	"context"
	"database/sql"
	"errors"
)

type SQLSource struct{ DB *sql.DB }

var errCursorStale = errors.New("catalog metadata cursor changed")
var errUnavailable = errors.New("catalog metadata source unavailable")

const maxDetailRows = 10000

// Source columns remain private. Exact detail recomputes logical identity from
// authorized rows; a cache row ID is never accepted as an identity fallback.
func (s SQLSource) Read(ctx context.Context, project string, query Query, limit int) ([]Template, error) {
	if s.DB == nil || limit != PageSize+1 {
		return nil, errUnavailable
	}
	if query.AfterID > 0 {
		anchors, err := s.query(ctx, project, Query{}, query.AfterID, 1)
		if err != nil {
			return nil, err
		}
		if len(anchors) != 1 || anchors[0].ID != query.AfterIdentity {
			return nil, errCursorStale
		}
	}
	if query.ExactID != "" {
		if !metadataID.MatchString(query.ExactID) {
			return nil, errUnavailable
		}
		rows, err := s.query(ctx, project, Query{}, 0, maxDetailRows+1)
		if err != nil {
			return nil, err
		}
		if len(rows) > maxDetailRows {
			return nil, errUnavailable
		}
		for _, row := range rows {
			if row.ID == query.ExactID {
				return []Template{row}, nil
			}
		}
		return nil, nil
	}
	return s.query(ctx, project, query, 0, limit)
}

func (s SQLSource) query(ctx context.Context, project string, query Query, anchor int64, limit int) ([]Template, error) {
	statement := `SELECT t.id, t.name, t.description, c.name, t.base, t.default_version, t.maintainer, t.license, t.environment_id, c.url, c.branch, c.pinned_commit, t.folder_name
FROM catalog_template t JOIN catalog c ON c.id = t.catalog_id
WHERE (t.environment_id = ? OR t.environment_id = 'global')
AND c.environment_id = t.environment_id`
	args := []interface{}{project}
	if anchor != 0 {
		statement += " AND t.id = ?"
		args = append(args, anchor)
	} else {
		statement += " AND t.id > ?"
		args = append(args, query.AfterID)
		if query.Text != "" {
			statement += " AND INSTR(LOWER(t.name), LOWER(?)) > 0"
			args = append(args, query.Text)
		}
	}
	statement += " ORDER BY t.id ASC LIMIT ?"
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Template{}
	for rows.Next() {
		var id int64
		var name, description, catalog, base, version, maintainer, license, environment, repo, branch, pin, folder sql.NullString
		if err := rows.Scan(&id, &name, &description, &catalog, &base, &version, &maintainer, &license, &environment, &repo, &branch, &pin, &folder); err != nil {
			return nil, err
		}
		if id <= 0 {
			return nil, errUnavailable
		}
		scope := "project"
		if environment.String == "global" {
			scope = "global"
		}
		logical, err := logicalID(identity{Scope: environment.String, Catalog: catalog.String, Repo: repo.String,
			Branch: branch.String, Pin: pin.String, Folder: folder.String, Base: base.String})
		if err != nil {
			return nil, errUnavailable
		}
		result = append(result, Template{ID: logical, rowID: id, Name: name.String, Description: description.String,
			CatalogID: catalog.String, TemplateBase: base.String, TemplateKey: folder.String, CatalogScope: scope, DefaultVersion: version.String, Maintainer: maintainer.String, License: license.String})
	}
	return result, rows.Err()
}
