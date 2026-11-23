package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/manager"
)

func TestBundledHealthFailsWithoutHidingExistingAPIRoutes(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.json")
	config := filepath.Join(dir, "repo.json")
	if err := os.WriteFile(manifest, []byte(`{"version":1,"sources":[{"url":"https://github.com/PastureStack/catalog-templates.git","branch":"main","pinnedCommit":"`+strings.Repeat("1", 40)+`","bundle":"catalog.bundle","sha256":"`+strings.Repeat("0", 64)+`"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"catalogs":{"pasturestack":{"url":"https://github.com/PastureStack/catalog-templates.git","branch":"main","pinnedCommit":"`+strings.Repeat("1", 40)+`"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := manager.NewManagerWithBundledSources(t.TempDir(), config, false, nil, manifest)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(m, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthcheck", nil))
	if response.Code != http.StatusServiceUnavailable || strings.TrimSpace(response.Body.String()) != `{"ready":false}` {
		t.Fatalf("unverified bundle health = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("failed bundled readiness hid API versions: %d %s", response.Code, response.Body.String())
	}
}
