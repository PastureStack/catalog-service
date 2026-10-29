package aimetadata

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type captureDriver struct{}
type captureConnection struct{}
type captureRows struct {
	rows   [][]driver.Value
	offset int
}

var recordedQuery string
var recordedArgs []driver.NamedValue
var fixtureRows [][]driver.Value
var fixtureVersionRows [][]driver.Value
var queryCount int

func (captureDriver) Open(string) (driver.Conn, error)        { return captureConnection{}, nil }
func (captureConnection) Prepare(string) (driver.Stmt, error) { panic("not used") }
func (captureConnection) Close() error                        { return nil }
func (captureConnection) Begin() (driver.Tx, error)           { panic("not used") }
func (captureConnection) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	recordedQuery = q
	recordedArgs = append([]driver.NamedValue(nil), args...)
	queryCount++
	rows := [][]driver.Value{}
	if strings.Contains(q, "FROM catalog_version v") {
		point := args[2].Value.(int64)
		limit := args[len(args)-1].Value.(int64)
		for _, row := range fixtureVersionRows {
			id := row[0].(int64)
			if strings.Contains(q, "v.id = ?") && id != point || strings.Contains(q, "v.id > ?") && id <= point {
				continue
			}
			rows = append(rows, row)
			if int64(len(rows)) == limit {
				break
			}
		}
		return &captureRows{rows: rows}, nil
	}
	point := args[1].Value.(int64)
	limit := args[len(args)-1].Value.(int64)
	for _, row := range fixtureRows {
		id := row[0].(int64)
		if strings.Contains(q, "t.id = ?") && id != point || strings.Contains(q, "t.id > ?") && id <= point {
			continue
		}
		rows = append(rows, row)
		if int64(len(rows)) == limit {
			break
		}
	}
	return &captureRows{rows: rows}, nil
}
func (*captureRows) Columns() []string {
	return []string{"id", "name", "description", "catalog", "base", "version", "maintainer", "license", "environment", "url", "branch", "pin", "folder"}
}
func (*captureRows) Close() error { return nil }
func (r *captureRows) Next(values []driver.Value) error {
	if r.offset >= len(r.rows) {
		return io.EOF
	}
	copy(values, r.rows[r.offset])
	r.offset++
	return nil
}
func init() { sql.Register("ai-metadata-capture", captureDriver{}) }
func sourceFixture(t *testing.T) SQLSource {
	t.Helper()
	db, err := sql.Open("ai-metadata-capture", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	queryCount = 0
	fixtureVersionRows = nil
	fixtureRows = [][]driver.Value{sourceRow(51, "https://example.test/repo.git")}
	return SQLSource{DB: db}
}
func sourceRow(id int64, repo string) []driver.Value {
	return []driver.Value{id, "name", "desc", "catalog", "", "1.0", "maintainer", "license", "1a5", repo, "main", "", "folder"}
}

func TestMetadataSQLFiltersProjectParentAndMarkerBeforeBoundedProjection(t *testing.T) {
	s := sourceFixture(t)
	query := "name%' OR 1=1 --"
	rows, err := s.query(context.Background(), "1a5", Query{Text: query, AfterID: 50}, 0, 51)
	if err != nil {
		t.Fatal(err)
	}
	for _, clause := range []string{"t.environment_id = ? OR t.environment_id = 'global'", "c.environment_id = t.environment_id", "t.id > ?", "INSTR(LOWER(t.name), LOWER(?)) > 0", "ORDER BY t.id ASC LIMIT ?"} {
		if !strings.Contains(recordedQuery, clause) {
			t.Fatalf("missing %s", clause)
		}
	}
	if strings.Contains(recordedQuery, query) || strings.Contains(recordedQuery, "SELECT *") || strings.Contains(recordedQuery, "t.*") {
		t.Fatal("unbounded fields or query interpolation")
	}
	if len(recordedArgs) != 4 || recordedArgs[0].Value != "1a5" || recordedArgs[1].Value != int64(50) || recordedArgs[2].Value != query || recordedArgs[3].Value != int64(51) {
		t.Fatal("query arguments changed")
	}
	if len(rows) != 1 || !metadataID.MatchString(rows[0].ID) || rows[0].ID == "ctm-51" {
		t.Fatal("logical identity missing")
	}
	collectionSQL := recordedQuery
	collectionArgs := append([]driver.NamedValue(nil), recordedArgs...)
	_, err = s.query(context.Background(), "1a5", Query{}, 51, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(recordedQuery, "t.id = ?") {
		t.Fatal("private anchor did not use exact locator")
	}
	if file := os.Getenv("CATALOG_METADATA_SQL_PROOF_FILE"); file != "" {
		proof := map[string]interface{}{"collectionSQL": collectionSQL, "collectionArgs": collectionArgs, "anchorSQL": recordedQuery, "anchorArgs": recordedArgs}
		encoded, err := json.MarshalIndent(proof, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMetadataDetailRecomputesLogicalIdentityAcrossCacheRebuildAndSourceChange(t *testing.T) {
	s := sourceFixture(t)
	first, err := s.Read(context.Background(), "1a5", Query{}, 51)
	if err != nil {
		t.Fatal(err)
	}
	id := first[0].ID
	fixtureRows = [][]driver.Value{sourceRow(901, "https://example.test/repo.git")}
	fixtureRows[0][2] = "changed description"
	fixtureRows[0][5] = "2.0"
	rebuilt, err := s.Read(context.Background(), "1a5", Query{ExactID: id}, 51)
	if err != nil || len(rebuilt) != 1 || rebuilt[0].ID != id || rebuilt[0].rowID != 901 {
		t.Fatal("cache rebuild changed logical resource")
	}
	fixtureRows = [][]driver.Value{sourceRow(51, "https://example.test/other.git")}
	rebound, err := s.Read(context.Background(), "1a5", Query{ExactID: id}, 51)
	if err != nil || len(rebound) != 0 {
		t.Fatal("old logical identity rebound to new repository")
	}
	if _, err := s.Read(context.Background(), "1a5", Query{ExactID: "ctm-51"}, 51); err == nil {
		t.Fatal("rowid fallback accepted")
	}
	fixtureRows = [][]driver.Value{sourceRow(901, "https://PRIVATE_USER:PRIVATE_PASSWORD@example.test/repo.git?token=PRIVATE_TOKEN#PRIVATE_FRAGMENT")}
	rotated, err := s.Read(context.Background(), "1a5", Query{ExactID: id}, 51)
	encoded, _ := json.Marshal(rotated)
	if err != nil || len(rotated) != 1 || rotated[0].ID != id || strings.Contains(string(encoded), "PRIVATE_") || strings.Contains(string(encoded), "rowID") {
		t.Fatal("credential rotation changed public identity or private source/locator leaked")
	}
}

func TestMetadataCursorChecksAnchorLogicalIdentityBeforeNextPage(t *testing.T) {
	s := sourceFixture(t)
	first, _ := s.Read(context.Background(), "1a5", Query{}, 51)
	anchor := first[0].ID
	fixtureRows = [][]driver.Value{sourceRow(51, "https://example.test/replaced.git")}
	queryCount = 0
	_, err := s.Read(context.Background(), "1a5", Query{AfterID: 51, AfterIdentity: anchor}, 51)
	if !errors.Is(err, errCursorStale) || queryCount != 1 {
		t.Fatal("stale cache anchor advanced to next page")
	}
}
