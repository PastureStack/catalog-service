package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PastureStack/catalog-service/model"
	"github.com/PastureStack/catalog-service/outbound"
)

type bundledFixture struct {
	repo, assets, manifest, config string
	entry                          bundledSource
	catalog                        model.Catalog
	policy                         *outbound.SourcePolicy
	requests                       atomic.Int32
}

func newBundledFixture(t *testing.T) *bundledFixture {
	t.Helper()
	f := &bundledFixture{repo: t.TempDir(), assets: t.TempDir()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	runCatalogGit(t, f.repo, "init", "-b", "main")
	runCatalogGit(t, f.repo, "config", "user.name", "PastureStack Test")
	runCatalogGit(t, f.repo, "config", "user.email", "test@example.invalid")
	writeTraversalFile(t, f.repo, ".gitignore", "templates/ignored-evil/\n")
	writeTraversalFile(t, f.repo, "infra-templates/healthcheck/config.yml", "name: Healthcheck\nversion: 1.0.0\n")
	writeTraversalFile(t, f.repo, "infra-templates/healthcheck/7/docker-compose.yml", "services:\n  check:\n    image: busybox:1.36.1\n")
	writeTraversalFile(t, f.repo, "infra-templates/healthcheck/7/rancher-compose.yml", ".catalog:\n  version: 1.0.0\n")
	runCatalogGit(t, f.repo, "add", ".")
	runCatalogGit(t, f.repo, "commit", "-m", "bundled fixture")
	pin := runCatalogGit(t, f.repo, "rev-parse", "HEAD")
	f.entry = bundledSource{URL: server.URL + "/catalog.git", Branch: "main", PinnedCommit: pin, Bundle: "catalog.bundle", RequiredTemplates: []requiredTemplate{{Base: "infra", Name: "healthcheck", Revision: 7}}}
	f.catalog = model.Catalog{Name: "pasturestack", EnvironmentId: "global", URL: f.entry.URL, Branch: "main", PinnedCommit: pin}
	f.manifest = filepath.Join(f.assets, "manifest.json")
	f.config = filepath.Join(f.assets, "repo.json")
	var err error
	f.policy, err = outbound.NewSourcePolicy([]string{server.URL}, []string{f.repo})
	if err != nil {
		t.Fatal(err)
	}
	f.writeBundle(t)
	f.writeConfig(t, nil)
	return f
}

func (f *bundledFixture) writeBundle(t *testing.T) {
	t.Helper()
	runCatalogGit(t, f.repo, "update-ref", "refs/heads/pasturestack-catalog", f.entry.PinnedCommit)
	runCatalogGit(t, f.repo, "bundle", "create", filepath.Join(f.assets, f.entry.Bundle), "refs/heads/pasturestack-catalog")
	f.updateHash(t)
}

func (f *bundledFixture) updateHash(t *testing.T) {
	t.Helper()
	bytes, err := os.ReadFile(filepath.Join(f.assets, f.entry.Bundle))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bytes)
	f.entry.SHA256 = hex.EncodeToString(sum[:])
	f.writeManifest(t)
}

func (f *bundledFixture) writeManifest(t *testing.T) {
	t.Helper()
	bytes, err := json.Marshal(struct {
		Version int             `json:"version"`
		Sources []bundledSource `json:"sources"`
	}{1, []bundledSource{f.entry}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.manifest, bytes, 0644); err != nil {
		t.Fatal(err)
	}
}

func (f *bundledFixture) writeConfig(t *testing.T, extra map[string]CatalogConfig) {
	t.Helper()
	if extra == nil {
		extra = make(map[string]CatalogConfig)
	}
	extra[f.catalog.Name] = CatalogConfig{URL: f.catalog.URL, Branch: f.catalog.Branch, PinnedCommit: f.catalog.PinnedCommit}
	contents, _ := json.Marshal(map[string]interface{}{"catalogs": extra})
	if err := os.WriteFile(f.config, contents, 0644); err != nil {
		t.Fatal(err)
	}
}

func (f *bundledFixture) manager(t *testing.T, cache string) *Manager {
	t.Helper()
	m := newManagerWithPolicy(cache, f.config, false, nil, f.policy)
	var err error
	m.bundled, err = loadBundledSources(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func prepareBundledFixture(t *testing.T, m *Manager, catalog model.Catalog) string {
	t.Helper()
	root, commit, _, err := m.prepareGitRepoPath(catalog, true, CatalogTypeNative)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if commit != catalog.PinnedCommit {
		t.Fatalf("commit = %s, want %s", commit, catalog.PinnedCommit)
	}
	return root.Name()
}

func TestBundledColdWarmAndDirtyCacheStayOffline(t *testing.T) {
	f := newBundledFixture(t)
	m := f.manager(t, t.TempDir())
	path := prepareBundledFixture(t, m, f.catalog)
	// A fresh process must validate even a warm cache against the supplied bundle.
	m = f.manager(t, m.cacheRoot)
	if got := prepareBundledFixture(t, m, f.catalog); got != path {
		t.Fatalf("warm cache changed location: %s", got)
	}
	writeTraversalFile(t, path, "infra-templates/healthcheck/config.yml", "name: Modified\n")
	writeTraversalFile(t, path, "templates/evil/config.yml", "name: Evil\n")
	writeTraversalFile(t, path, "templates/ignored-evil/config.yml", "name: Ignored evil\n")
	prepareBundledFixture(t, m, f.catalog)
	contents, _ := os.ReadFile(filepath.Join(path, "infra-templates/healthcheck/config.yml"))
	if !strings.Contains(string(contents), "Healthcheck") {
		t.Fatalf("tracked pollution survived: %q", contents)
	}
	if _, err := os.Stat(filepath.Join(path, "templates/evil")); !os.IsNotExist(err) {
		t.Fatalf("untracked template survived repair: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "templates/ignored-evil")); !os.IsNotExist(err) {
		t.Fatalf("ignored untracked template survived repair: %v", err)
	}
	runCatalogGit(t, path, "update-index", "--assume-unchanged", "infra-templates/healthcheck/config.yml")
	writeTraversalFile(t, path, "infra-templates/healthcheck/config.yml", "name: Hidden modification\n")
	prepareBundledFixture(t, m, f.catalog)
	contents, _ = os.ReadFile(filepath.Join(path, "infra-templates/healthcheck/config.yml"))
	if !strings.Contains(string(contents), "Healthcheck") {
		t.Fatalf("index flags hid a modified template: %q", contents)
	}
	if f.requests.Load() != 0 {
		t.Fatalf("bundled source made %d remote requests", f.requests.Load())
	}
}

func TestBundledBadHashOrBundlePreservesGoodCache(t *testing.T) {
	for _, rehash := range []bool{false, true} {
		t.Run(map[bool]string{false: "hash", true: "format"}[rehash], func(t *testing.T) {
			f := newBundledFixture(t)
			m := f.manager(t, t.TempDir())
			path := prepareBundledFixture(t, m, f.catalog)
			if err := os.WriteFile(filepath.Join(f.assets, f.entry.Bundle), []byte("invalid bundle"), 0644); err != nil {
				t.Fatal(err)
			}
			if rehash {
				f.updateHash(t)
			}
			m = f.manager(t, m.cacheRoot)
			if _, _, _, err := m.prepareGitRepoPath(f.catalog, true, CatalogTypeNative); err == nil {
				t.Fatal("bad bundled source was accepted")
			}
			if got := runCatalogGit(t, path, "rev-parse", "HEAD"); got != f.entry.PinnedCommit {
				t.Fatalf("good cache changed: %s", got)
			}
			if f.requests.Load() != 0 {
				t.Fatal("bad bundle fell back to the network")
			}
		})
	}
}

func TestBundledRejectsUnadvertisedPinAndLeavesOtherCache(t *testing.T) {
	f := newBundledFixture(t)
	m := f.manager(t, t.TempDir())
	path := prepareBundledFixture(t, m, f.catalog)
	f.entry.PinnedCommit = strings.Repeat("1", 40)
	f.catalog.PinnedCommit = f.entry.PinnedCommit
	f.writeManifest(t)
	m = f.manager(t, m.cacheRoot)
	if _, _, _, err := m.prepareGitRepoPath(f.catalog, true, CatalogTypeNative); err == nil {
		t.Fatal("unadvertised pin was accepted")
	}
	if _, err := os.Stat(filepath.Join(path, "infra-templates/healthcheck/config.yml")); err != nil {
		t.Fatalf("old cache was removed: %v", err)
	}
}

func TestBundledRejectsMissingPrerequisites(t *testing.T) {
	f := newBundledFixture(t)
	m := f.manager(t, t.TempDir())
	path := prepareBundledFixture(t, m, f.catalog)
	previous := f.entry.PinnedCommit
	writeTraversalFile(t, f.repo, "README.md", "second revision\n")
	runCatalogGit(t, f.repo, "add", ".")
	runCatalogGit(t, f.repo, "commit", "-m", "incremental bundle")
	f.entry.PinnedCommit = runCatalogGit(t, f.repo, "rev-parse", "HEAD")
	f.catalog.PinnedCommit = f.entry.PinnedCommit
	runCatalogGit(t, f.repo, "update-ref", "refs/heads/pasturestack-catalog", f.entry.PinnedCommit)
	runCatalogGit(t, f.repo, "bundle", "create", filepath.Join(f.assets, f.entry.Bundle), "refs/heads/pasturestack-catalog", "^"+previous)
	f.updateHash(t)
	m = f.manager(t, m.cacheRoot)
	if _, _, _, err := m.prepareGitRepoPath(f.catalog, true, CatalogTypeNative); err == nil {
		t.Fatal("bundle with missing prerequisites was accepted")
	}
	if got := runCatalogGit(t, path, "rev-parse", "HEAD"); got != previous {
		t.Fatalf("incremental bundle changed the previous cache: %s", got)
	}
}

func TestBundledDoesNotOverrideCustomCatalogOrPolicy(t *testing.T) {
	f := newBundledFixture(t)
	m := f.manager(t, t.TempDir())
	custom := f.catalog
	custom.URL = f.repo
	prepareBundledFixture(t, m, custom)
	if m.bundled.isVerified(f.catalog) {
		t.Fatal("different custom source consumed a bundle")
	}
	policy, _ := outbound.NewSourcePolicy(nil, nil)
	m.sourcePolicy = policy
	if _, _, _, err := m.prepareGitRepoPath(f.catalog, false, CatalogTypeNative); err == nil {
		t.Fatal("bundle bypassed outbound source authorization")
	}
}

func TestBundledManifestRejectsPathEscapeAndInvalidEntries(t *testing.T) {
	f := newBundledFixture(t)
	for _, bundle := range []string{"../catalog.bundle", ".", filepath.Join(t.TempDir(), "catalog.bundle")} {
		f.entry.Bundle = bundle
		f.writeManifest(t)
		if _, err := loadBundledSources(f.manifest); err == nil {
			t.Errorf("unsafe bundle path %q was accepted", bundle)
		}
	}
	f.entry.Bundle = "linked.bundle"
	outside := filepath.Join(t.TempDir(), "outside.bundle")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.assets, f.entry.Bundle)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	f.writeManifest(t)
	m := f.manager(t, t.TempDir())
	if _, _, _, err := m.prepareGitRepoPath(f.catalog, false, CatalogTypeNative); err == nil {
		t.Fatal("bundle symlink escaped the manifest directory")
	}
}
