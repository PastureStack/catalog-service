package manager

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/model"
	"github.com/PastureStack/catalog-service/outbound"
)

func runCatalogGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeCatalogFixture(t *testing.T, repo, value string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "fixture.txt"), []byte(value+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCatalogGit(t, repo, "add", "fixture.txt")
	runCatalogGit(t, repo, "commit", "-m", value)
	return runCatalogGit(t, repo, "rev-parse", "HEAD")
}

func TestPrepareGitRepoPathKeepsPinnedCommit(t *testing.T) {
	source, err := os.MkdirTemp("", "pasturestack-catalog-source-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(source)
	cache, err := os.MkdirTemp("", "pasturestack-catalog-cache-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cache)

	runCatalogGit(t, source, "init", "-b", "main")
	runCatalogGit(t, source, "config", "user.name", "PastureStack Test")
	runCatalogGit(t, source, "config", "user.email", "test@example.invalid")
	pinned := writeCatalogFixture(t, source, "first")
	writeCatalogFixture(t, source, "second")

	policy, err := outbound.NewSourcePolicy(nil, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	m := newManagerWithPolicy(cache, "", false, nil, policy)
	catalog := model.Catalog{
		Name:          "pasturestack",
		URL:           source,
		Branch:        "main",
		PinnedCommit:  pinned,
		EnvironmentId: "global",
	}
	repoRoot, commit, _, err := m.prepareGitRepoPath(catalog, true, CatalogTypeNative)
	if err != nil {
		t.Fatal(err)
	}
	repoRoot.Close()
	if commit != pinned {
		t.Fatalf("catalog commit %s does not match pinned commit %s", commit, pinned)
	}

	writeCatalogFixture(t, source, "third")
	repoRoot, commit, _, err = m.prepareGitRepoPath(catalog, true, CatalogTypeNative)
	if err != nil {
		t.Fatal(err)
	}
	repoRoot.Close()
	if commit != pinned {
		t.Fatalf("refresh advanced pinned catalog from %s to %s", pinned, commit)
	}
}

func TestPrepareGitRepoPathRejectsShortPinnedCommit(t *testing.T) {
	policy, err := outbound.NewSourcePolicy([]string{"https://example.invalid"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := newManagerWithPolicy(t.TempDir(), "", false, nil, policy)
	_, _, _, err = m.prepareGitRepoPath(model.Catalog{
		URL:          "https://example.invalid/catalog.git",
		Branch:       "main",
		PinnedCommit: "deadbeef",
	}, false, CatalogTypeNative)
	if err == nil {
		t.Fatal("expected a short pinned commit to be rejected")
	}
}

func TestCatalogCachePathRejectsTraversal(t *testing.T) {
	policy, err := outbound.NewSourcePolicy(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := newManagerWithPolicy(t.TempDir(), "", false, nil, policy)
	for _, environmentID := range []string{"", ".", "..", "../escape", "project/../../escape"} {
		if _, err := m.catalogCacheRoot(environmentID, strings.Repeat("0", 64)); err == nil {
			t.Errorf("unsafe environment ID %q was accepted", environmentID)
		}
	}
	root, err := m.catalogCacheRoot("project-a", strings.Repeat("0", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	relative, err := filepath.Rel(m.cacheRoot, root.Name())
	if err != nil || !filepath.IsLocal(relative) {
		t.Fatalf("safe cache path escaped root: %q, %v", root.Name(), err)
	}
}

func TestCatalogCacheRootRejectsSymlinkEscape(t *testing.T) {
	cache := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(cache, "project-a")); err != nil {
		if os.IsPermission(err) {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		t.Fatal(err)
	}
	policy, err := outbound.NewSourcePolicy(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := newManagerWithPolicy(cache, "", false, nil, policy)
	if _, err := m.catalogCacheRoot("project-a", strings.Repeat("0", 64)); err == nil {
		t.Fatal("cache symlink escape was accepted")
	}
}

func TestSafeLogValueRemovesRecordSeparators(t *testing.T) {
	if got := safeLogValue("catalog\r\nforged\tentry"); got != "catalogforgedentry" {
		t.Fatalf("safe log value = %q", got)
	}
}

func TestFormatGitURLAcceptsOnlyCanonicalPublicGitHubRepository(t *testing.T) {
	if got := formatGitURL("https://github.com/PastureStack/catalog-templates.git", "main"); got != "https://api.github.com/repos/PastureStack/catalog-templates/commits/main" {
		t.Fatalf("formatted URL = %q", got)
	}
	for _, endpoint := range []string{
		"http://github.com/PastureStack/catalog-templates.git",
		"https://github.com:444/PastureStack/catalog-templates.git",
		"https://user@github.com/PastureStack/catalog-templates.git",
		"https://github.com/PastureStack/catalog-templates/extra",
		"https://github.com.evil/PastureStack/catalog-templates.git",
		"ssh://github.com/PastureStack/catalog-templates.git",
	} {
		if got := formatGitURL(endpoint, "main"); got != "" {
			t.Errorf("unsafe endpoint %q produced %q", endpoint, got)
		}
	}
}

func TestRemoteSHAChangedUsesPolicyCheckedClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()
	policy, err := outbound.NewSourcePolicy([]string{server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := newManagerWithPolicy(t.TempDir(), "", false, nil, policy)
	request, err := http.NewRequest(http.MethodGet, server.URL+"/commit", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d", response.StatusCode)
	}

	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unauthorized server must not receive a request")
	}))
	defer attacker.Close()
	request, err = http.NewRequest(http.MethodGet, attacker.URL+"/commit", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.httpClient.Do(request); err == nil {
		t.Fatal("unauthorized request was accepted")
	}
}
