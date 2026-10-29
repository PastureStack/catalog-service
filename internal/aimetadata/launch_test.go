package aimetadata

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type launchFixture struct {
	material *LaunchMaterial
	err      error
	calls    int
}

func (f *launchFixture) ReadLaunch(_ context.Context, project, template, version string) (*LaunchMaterial, error) {
	f.calls++
	if project != "1a5" || !metadataID.MatchString(template) || !versionID.MatchString(version) {
		panic("unbound input")
	}
	return f.material, f.err
}

func launchRequest() *http.Request {
	r := httptest.NewRequest("GET", LaunchOptionsPath+"?templateId=ctm-"+strings.Repeat("a", 64)+"&versionId=cvm-"+strings.Repeat("b", 64), nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set(ProjectHeader, "1a5")
	return r
}

func TestLaunchOptionsRouteNeverProjectsPrivateDefaultsFilesOrSourceDigest(t *testing.T) {
	private := "never-output-private-default"
	f := &launchFixture{material: &LaunchMaterial{Digest: strings.Repeat("d", 64), Files: map[string]string{
		"docker-compose.yml": "services: {}", "rancher-compose.yml": ".catalog:\n  questions:\n    - variable: password\n      type: password\n      default: " + private + "\n    - variable: count\n      type: int\n      required: true\n"}}}
	w := httptest.NewRecorder()
	NewLaunchOptionsHandler(f).ServeHTTP(w, launchRequest())
	if w.Code != 200 {
		t.Fatalf("options denied: %d", w.Code)
	}
	for _, forbidden := range []string{private, "default", "digest", "docker-compose", "rancher-compose", "files"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("private field leaked: %s", forbidden)
		}
	}
	var result struct {
		Questions []struct {
			Name          string
			RequiresHuman bool
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Questions) != 2 || result.Questions[0].Name != "password" || !result.Questions[0].RequiresHuman {
		t.Fatal("safe questions missing")
	}
	w = httptest.NewRecorder()
	NewLaunchMaterialHandler(f).ServeHTTP(w, launchRequest())
	if w.Code != 200 || !strings.Contains(w.Body.String(), private) || !strings.Contains(w.Body.String(), "digest") {
		t.Fatal("private owner material missing")
	}
}

func TestLaunchRouteRejectsForwardedPeerRolesDuplicateProjectAndOpenInputs(t *testing.T) {
	f := &launchFixture{}
	for _, change := range []func(*http.Request){
		func(r *http.Request) {
			r.RemoteAddr = "10.0.0.1:1234"
			r.Header.Set("X-Forwarded-For", "127.0.0.1")
			r.Header.Set("X-API-Account-Role", "admin")
		},
		func(r *http.Request) { r.RemoteAddr = "127.0.0.1" },
		func(r *http.Request) { r.Header.Add(ProjectHeader, "1a6") },
		func(r *http.Request) { r.URL.RawQuery += "&url=http://localhost" },
		func(r *http.Request) { r.URL.RawQuery += "&templateId=ctm-" + strings.Repeat("a", 64) },
		func(r *http.Request) { r.Method = "POST" },
	} {
		r := launchRequest()
		change(r)
		w := httptest.NewRecorder()
		NewLaunchOptionsHandler(f).ServeHTTP(w, r)
		if w.Code < 400 || f.calls != 0 {
			t.Fatal("untrusted input reached source")
		}
	}
	f.err = errors.New("private-database-message-and-secret")
	w := httptest.NewRecorder()
	NewLaunchOptionsHandler(f).ServeHTTP(w, launchRequest())
	if w.Code != 503 || strings.Contains(w.Body.String(), "private-database") {
		t.Fatal("raw error exposed")
	}
}

func launchRow(file, contents, commit string) []driver.Value {
	return []driver.Value{"1a5", "catalog", "https://example.test/repo.git", "main", "", commit, "folder", "", "display", "1.0", int64(1), file, contents}
}

func TestLaunchMaterialBindsJoinedSourceAndFilesWithoutPublicCacheIDs(t *testing.T) {
	s := sourceFixture(t)
	parents, _ := s.Read(context.Background(), "1a5", Query{}, 51)
	parent := parents[0].ID
	selected := versionLogicalID(parent, "1.0", ptrRevision(1))
	fixtureVersionRows = [][]driver.Value{versionRow(10, "1.0", 1)}
	fixtureLaunchRows = [][]driver.Value{launchRow("docker-compose.yml", "version: '2'\nservices: {}", "commit-a")}
	first, err := s.ReadLaunch(context.Background(), "1a5", parent, selected)
	if err != nil || first.ExternalID != "catalog://catalog:folder:1" || len(first.Digest) != 64 {
		t.Fatalf("material unavailable: %v", err)
	}
	for _, clause := range []string{"t.environment_id = ? OR t.environment_id = 'global'", "c.environment_id = t.environment_id", "v.id = ?", "LEFT JOIN catalog_file", "SUBSTR(f.contents", "ORDER BY f.id ASC LIMIT ?"} {
		if !strings.Contains(recordedQuery, clause) {
			t.Fatalf("unbound material query: %s", clause)
		}
	}
	if recordedArgs[0].Value != "1a5" || recordedArgs[1].Value != int64(10) || recordedArgs[2].Value != int64(65) {
		t.Fatal("material locator not bound")
	}
	if file := os.Getenv("CATALOG_LAUNCH_SQL_PROOF_FILE"); file != "" {
		encoded, err := json.MarshalIndent(map[string]interface{}{"sql": recordedQuery, "args": recordedArgs}, "", "  ")
		if err != nil || os.WriteFile(file, encoded, 0600) != nil {
			t.Fatal("cannot write owned SQL proof")
		}
	}
	fixtureVersionRows[0][0] = int64(999)
	second, err := s.ReadLaunch(context.Background(), "1a5", parent, selected)
	if err != nil || second.Digest != first.Digest {
		t.Fatal("cache rebuild changed source digest")
	}
	fixtureLaunchRows[0][5] = "commit-b"
	changed, _ := s.ReadLaunch(context.Background(), "1a5", parent, selected)
	if changed.Digest == first.Digest {
		t.Fatal("source commit change not detected")
	}
	fixtureLaunchRows[0][2] = "https://example.test/replaced.git"
	if _, err := s.ReadLaunch(context.Background(), "1a5", parent, selected); !errors.Is(err, errCursorStale) {
		t.Fatal("source rebind accepted")
	}
}

func TestLaunchMaterialRejectsForeignIdentityUnsafeFilesDuplicateAndByteOverflow(t *testing.T) {
	s := sourceFixture(t)
	parents, _ := s.Read(context.Background(), "1a5", Query{}, 51)
	parent := parents[0].ID
	selected := versionLogicalID(parent, "1.0", ptrRevision(1))
	fixtureVersionRows = [][]driver.Value{versionRow(10, "1.0", 1)}
	for _, files := range [][][]driver.Value{
		{launchRow("../secret", "x", "a")}, {launchRow("/tmp/file", "x", "a")},
		{launchRow("docker-compose.yml", strings.Repeat("x", maxLaunchBytes+1), "a")},
		{launchRow("docker-compose.yml", "a", "a"), launchRow("docker-compose.yml", "b", "a")},
	} {
		fixtureLaunchRows = files
		if _, err := s.ReadLaunch(context.Background(), "1a5", parent, selected); !errors.Is(err, errUnavailable) {
			t.Fatal("unsafe material accepted")
		}
	}
	fixtureLaunchRows = [][]driver.Value{launchRow("docker-compose.yml", "a", "a")}
	fixtureLaunchRows[0][0] = "1a6"
	if _, err := s.ReadLaunch(context.Background(), "1a5", parent, selected); !errors.Is(err, errCursorStale) {
		t.Fatal("foreign project material accepted")
	}
}

func ptrRevision(value int64) *int64 { return &value }

func TestLaunchPrivateResponseByteBoundCannotEchoOversizedMaterial(t *testing.T) {
	w := httptest.NewRecorder()
	writeLaunchJSON(w, map[string]string{"private": strings.Repeat("x", MaxLaunchResponseBytes+1)})
	if w.Code != 422 || w.Body.Len() > 100 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("private material response not bounded")
	}
}
