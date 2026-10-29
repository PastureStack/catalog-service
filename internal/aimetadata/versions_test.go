package aimetadata

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func versionRow(id int64, value string, revision int64) []driver.Value {
	return []driver.Value{id, value, revision, "1.6.0", "1.9.0", "1.x", "1a5", "catalog", "https://example.test/repo.git", "main", "", "folder", ""}
}

func TestVersionMetadataRevalidatesParentAndUsesOnlyBoundedSafeSQL(t *testing.T) {
	s := sourceFixture(t)
	parents, _ := s.Read(context.Background(), "1a5", Query{}, 51)
	parent := parents[0].ID
	fixtureVersionRows = [][]driver.Value{versionRow(10, "1.0", 1)}
	query := Query{ExactID: parent, Versions: true}
	rows, err := s.ReadVersions(context.Background(), "1a5", query, 51)
	if err != nil || len(rows) != 1 || !versionID.MatchString(rows[0].ID) || rows[0].TemplateID != parent {
		t.Fatalf("version metadata unavailable: %v", err)
	}
	for _, clause := range []string{"v.template_id = t.id", "c.id = t.catalog_id", "t.environment_id = ? OR t.environment_id = 'global'", "c.environment_id = t.environment_id", "t.id = ?", "v.id > ?", "ORDER BY v.id ASC LIMIT ?"} {
		if !strings.Contains(recordedQuery, clause) {
			t.Fatalf("missing %s", clause)
		}
	}
	for _, field := range []string{"v.*", "files", "questions", "readme", "default_answers"} {
		if strings.Contains(recordedQuery, field) {
			t.Fatalf("unsafe query: %s", field)
		}
	}
	if recordedArgs[0].Value != "1a5" || recordedArgs[1].Value != int64(51) || recordedArgs[2].Value != int64(0) || recordedArgs[3].Value != int64(51) {
		t.Fatal("unbound selector")
	}
	if file := os.Getenv("CATALOG_VERSION_SQL_PROOF_FILE"); file != "" {
		encoded, err := json.MarshalIndent(map[string]interface{}{"sql": recordedQuery, "args": recordedArgs}, "", "  ")
		if err != nil || os.WriteFile(file, encoded, 0600) != nil {
			t.Fatal("cannot save SQL proof")
		}
	}
	id := rows[0].ID
	fixtureRows = [][]driver.Value{sourceRow(901, "https://example.test/repo.git")}
	fixtureVersionRows[0][0] = int64(999)
	query.VersionID = id
	rebuilt, err := s.ReadVersions(context.Background(), "1a5", query, 51)
	if err != nil || len(rebuilt) != 1 || rebuilt[0].ID != id {
		t.Fatal("cache rebuild changed version identity")
	}
	fixtureVersionRows[0][8] = "https://example.test/replaced.git"
	if _, err = s.ReadVersions(context.Background(), "1a5", query, 51); !errors.Is(err, errCursorStale) {
		t.Fatal("source changed between parent and version query")
	}
	fixtureVersionRows[0] = versionRow(999, "1.0", 9007199254740992)
	if _, err = s.ReadVersions(context.Background(), "1a5", query, 51); !errors.Is(err, errUnavailable) {
		t.Fatal("unsafe numeric revision projected")
	}
	fixtureRows = nil
	if _, err = s.ReadVersions(context.Background(), "1a5", query, 51); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("foreign or removed parent accepted")
	}
}

type versionsFixture struct {
	rows    []Version
	queries []Query
}

func (*versionsFixture) Read(context.Context, string, Query, int) ([]Template, error) {
	return nil, nil
}
func (s *versionsFixture) ReadVersions(_ context.Context, _ string, q Query, _ int) ([]Version, error) {
	s.queries = append(s.queries, q)
	return s.rows, nil
}

func TestVersionsRouteClosedInputsProjectionPagingAndTemplateBinding(t *testing.T) {
	parent := template(1).ID
	source := &versionsFixture{}
	for i := 1; i <= 51; i++ {
		revision := int64(i)
		source.rows = append(source.rows, Version{ID: versionLogicalID(parent, fmt.Sprint(i), &revision), TemplateID: parent, Version: fmt.Sprint(i), Revision: &revision, rowID: int64(i)})
	}
	h := NewHandler(source)
	for _, suffix := range []string{"?versions=true", "?templateId=" + parent + "&versions=false", "?templateId=" + parent + "&versionId=cvm-1", "?templateId=" + parent + "&versions=true&query=x", "?templateId=" + parent + "&versions=true&versionId=cvm-" + strings.Repeat("a", 64) + "&marker=x"} {
		if w := serve(h, request(suffix)); w.Code != 422 {
			t.Fatalf("input accepted %s", suffix)
		}
	}
	w := serve(h, request("?templateId="+parent+"&versions=true"))
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	var body struct {
		ResourceType string    `json:"resourceType"`
		Data         []Version `json:"data"`
		NextMarker   string    `json:"nextMarker"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.ResourceType != "catalogTemplateVersion" || len(body.Data) != 50 || body.NextMarker == "" {
		t.Fatal("version pagination invalid")
	}
	if strings.Contains(w.Body.String(), "rowID") || strings.Contains(w.Body.String(), "files") || strings.Contains(w.Body.String(), "questions") {
		t.Fatal("private projection leaked")
	}
	other := template(2).ID
	if serve(h, request("?templateId="+other+"&versions=true&marker="+body.NextMarker)).Code != 422 {
		t.Fatal("cross-template cursor accepted")
	}
	source.rows = []Version{{ID: versionLogicalID(other, "1", nil), TemplateID: other, Version: "1", rowID: 1}}
	if serve(h, request("?templateId="+parent+"&versions=true")).Code != 503 {
		t.Fatal("foreign version projection accepted")
	}
}

func TestVersionCursorRechecksExactVersionAndNeverRebindsReusedCacheRow(t *testing.T) {
	s := sourceFixture(t)
	parents, _ := s.Read(context.Background(), "1a5", Query{}, 51)
	parent := parents[0].ID
	fixtureVersionRows = [][]driver.Value{versionRow(10, "1.0", 1)}
	rows, _ := s.ReadVersions(context.Background(), "1a5", Query{ExactID: parent, Versions: true}, 51)
	query := Query{ExactID: parent, Versions: true, AfterID: 10, AfterIdentity: rows[0].ID}
	fixtureVersionRows[0][1] = "2.0"
	if _, err := s.ReadVersions(context.Background(), "1a5", query, 51); !errors.Is(err, errCursorStale) {
		t.Fatal("reused version row advanced")
	}
	if versionLogicalID(parent, "1.0", nil) == versionLogicalID(parent, "1.0", new(int64)) {
		t.Fatal("nil and zero revision conflated")
	}
}
