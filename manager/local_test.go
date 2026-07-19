package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/model"
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

	m := &Manager{cacheRoot: cache}
	catalog := model.Catalog{
		Name:          "pasturestack",
		URL:           source,
		Branch:        "main",
		PinnedCommit:  pinned,
		EnvironmentId: "global",
	}
	_, commit, _, err := m.prepareGitRepoPath(catalog, true, CatalogTypeNative)
	if err != nil {
		t.Fatal(err)
	}
	if commit != pinned {
		t.Fatalf("catalog commit %s does not match pinned commit %s", commit, pinned)
	}

	writeCatalogFixture(t, source, "third")
	_, commit, _, err = m.prepareGitRepoPath(catalog, true, CatalogTypeNative)
	if err != nil {
		t.Fatal(err)
	}
	if commit != pinned {
		t.Fatalf("refresh advanced pinned catalog from %s to %s", pinned, commit)
	}
}

func TestPrepareGitRepoPathRejectsShortPinnedCommit(t *testing.T) {
	m := &Manager{cacheRoot: t.TempDir()}
	_, _, _, err := m.prepareGitRepoPath(model.Catalog{
		URL:          "https://example.invalid/catalog.git",
		Branch:       "main",
		PinnedCommit: "deadbeef",
	}, false, CatalogTypeNative)
	if err == nil {
		t.Fatal("expected a short pinned commit to be rejected")
	}
}
