package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/PastureStack/catalog-service/git"
	"github.com/PastureStack/catalog-service/model"
)

type requiredTemplate struct {
	Base     string `json:"base"`
	Name     string `json:"name"`
	Revision int    `json:"revision"`
}

type bundledSource struct {
	URL               string             `json:"url"`
	Branch            string             `json:"branch"`
	PinnedCommit      string             `json:"pinnedCommit"`
	Bundle            string             `json:"bundle"`
	SHA256            string             `json:"sha256"`
	RequiredTemplates []requiredTemplate `json:"requiredTemplates,omitempty"`
}

type bundledSources struct {
	root     string
	sources  []bundledSource
	statusMu sync.RWMutex
	states   map[bundledCatalogKey]bundledCatalogState
}

type bundledCatalogKey struct {
	Environment, Name, URL, Branch, Pin, Kind string
}

type bundledCatalogState struct {
	verified, needsReindex bool
}

func bundledKey(catalog model.Catalog) bundledCatalogKey {
	branch := catalog.Branch
	if branch == "" {
		branch = "master"
	}
	return bundledCatalogKey{catalog.EnvironmentId, catalog.Name, catalog.URL, branch, strings.ToLower(strings.TrimSpace(catalog.PinnedCommit)), effectiveCatalogKind(catalog.Kind)}
}

func effectiveCatalogKind(kind string) string {
	if kind == "" {
		return NativeTemplateType
	}
	return kind
}

func loadBundledSources(manifest string) (*bundledSources, error) {
	manifest, err := filepath.Abs(manifest)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Dir(manifest))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(filepath.Base(manifest))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var document struct {
		Version int             `json:"version"`
		Sources []bundledSource `json:"sources"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("Invalid bundled catalog manifest: %w", err)
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return nil, fmt.Errorf("Bundled catalog manifest has trailing data")
	}
	if document.Version != 1 || len(document.Sources) == 0 {
		return nil, fmt.Errorf("Bundled catalog manifest must have version 1 and sources")
	}
	result := &bundledSources{root: root.Name(), sources: document.Sources, states: make(map[bundledCatalogKey]bundledCatalogState)}
	seen := make(map[string]bool)
	for i := range result.sources {
		source := &result.sources[i]
		if source.URL == "" || source.Branch == "" || strings.TrimSpace(source.Branch) != source.Branch ||
			!fullCommitSHA.MatchString(source.PinnedCommit) || !cacheRepositoryHash.MatchString(source.SHA256) ||
			!filepath.IsLocal(source.Bundle) || source.Bundle == "." {
			return nil, fmt.Errorf("Invalid bundled catalog source")
		}
		source.PinnedCommit = strings.ToLower(source.PinnedCommit)
		key := source.URL + "\x00" + source.Branch + "\x00" + source.PinnedCommit
		if seen[key] {
			return nil, fmt.Errorf("Duplicate bundled catalog source")
		}
		seen[key] = true
		for _, required := range source.RequiredTemplates {
			if required.Name == "" || !safeTemplateSegment(required.Name) ||
				(required.Base != "" && !safeTemplateSegment(required.Base)) || required.Revision < 0 {
				return nil, fmt.Errorf("Invalid required bundled template")
			}
		}
	}
	return result, nil
}

func safeTemplateSegment(value string) bool {
	return value != "." && value != ".." && !strings.ContainsAny(value, "/\\\x00")
}

func (sources *bundledSources) match(url, branch, pin string) *bundledSource {
	if sources != nil {
		for i := range sources.sources {
			source := &sources.sources[i]
			if source.URL == url && source.Branch == branch && source.PinnedCommit == pin {
				return source
			}
		}
	}
	return nil
}

func (sources *bundledSources) matchCatalog(catalog model.Catalog) *bundledSource {
	branch := catalog.Branch
	if branch == "" {
		branch = "master"
	}
	return sources.match(catalog.URL, branch, strings.ToLower(strings.TrimSpace(catalog.PinnedCommit)))
}

func (sources *bundledSources) setVerified(catalog model.Catalog, verified bool) {
	sources.statusMu.Lock()
	defer sources.statusMu.Unlock()
	key := bundledKey(catalog)
	state := sources.states[key]
	state.verified = verified
	sources.states[key] = state
}

func (sources *bundledSources) isVerified(catalog model.Catalog) bool {
	sources.statusMu.RLock()
	defer sources.statusMu.RUnlock()
	return sources.states[bundledKey(catalog)].verified
}

func (sources *bundledSources) needsReindex(catalog model.Catalog) bool {
	if sources == nil {
		return false
	}
	sources.statusMu.RLock()
	defer sources.statusMu.RUnlock()
	return sources.states[bundledKey(catalog)].needsReindex
}

func (sources *bundledSources) setNeedsReindex(catalog model.Catalog, needed bool) {
	if sources == nil {
		return
	}
	sources.statusMu.Lock()
	defer sources.statusMu.Unlock()
	key := bundledKey(catalog)
	state := sources.states[key]
	state.needsReindex = needed
	sources.states[key] = state
}

// copyBundle validates the bytes that Git will actually consume. os.Root keeps
// a bundle path (including symlinks) inside the operator's asset directory.
func (sources *bundledSources) copyBundle(source *bundledSource, destination string) error {
	root, err := os.OpenRoot(sources.root)
	if err != nil {
		return err
	}
	defer root.Close()
	input, err := root.Open(source.Bundle)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("Bundled catalog must be a regular file")
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(hash.Sum(nil)) != source.SHA256 {
		return fmt.Errorf("Bundled catalog SHA256 mismatch")
	}
	return nil
}

func (m *Manager) prepareBundledRepoPath(catalog model.Catalog, branch, pin string, catalogType CatalogType, source *bundledSource) (result *os.Root, commit string, kind CatalogType, err error) {
	defer func() { m.bundled.setVerified(catalog, err == nil) }()
	sum := sha256.Sum256([]byte(catalog.URL + "\x00" + branch + "\x00" + pin))
	repoRoot, err := m.catalogCacheRoot(catalog.EnvironmentId, hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, "", catalogType, err
	}
	defer func() {
		if repoRoot != nil {
			_ = repoRoot.Close()
		}
	}()
	parent, err := os.OpenRoot(filepath.Dir(repoRoot.Name()))
	if err != nil {
		return nil, "", catalogType, err
	}
	defer parent.Close()
	stage, err := os.MkdirTemp(parent.Name(), ".bundled-")
	if err != nil {
		return nil, "", catalogType, err
	}
	stageName := filepath.Base(stage)
	defer func() {
		if stageName != "" {
			_ = parent.RemoveAll(stageName)
		}
	}()
	verifiedBundle := filepath.Join(stage, "source.bundle")
	if err := m.bundled.copyBundle(source, verifiedBundle); err != nil {
		return nil, "", catalogType, err
	}
	current, headErr := git.HeadCommit(repoRoot.Name())
	clean, cleanErr := git.IsClean(repoRoot.Name())
	cacheUsable := headErr == nil && cleanErr == nil && current == pin && clean
	if cacheUsable && m.bundled.isVerified(catalog) {
		result = repoRoot
		repoRoot = nil
		return result, pin, catalogType, nil
	}
	if err := git.CloneBundle(filepath.Join(stage, "repo"), verifiedBundle, branch, pin); err != nil {
		return nil, "", catalogType, fmt.Errorf("Bundled catalog import failed: %w", err)
	}
	if cacheUsable {
		result = repoRoot
		repoRoot = nil
		return result, pin, catalogType, nil
	}
	// Keep the old cache until the replacement has been fully imported and pinned.
	// All refresh entry points share refreshMu, including startup reconciliation.
	repoName := filepath.Base(repoRoot.Name())
	if err := repoRoot.Close(); err != nil {
		return nil, "", catalogType, err
	}
	repoRoot = nil
	old := filepath.Join(stageName, "previous")
	if err := parent.Rename(repoName, old); err != nil {
		return nil, "", catalogType, err
	}
	if err := parent.Rename(filepath.Join(stageName, "repo"), repoName); err != nil {
		if rollbackErr := parent.Rename(old, repoName); rollbackErr != nil {
			// Preserve the previous cache for recovery if an OS-level rename fails.
			stageName = ""
			return nil, "", catalogType, fmt.Errorf("Bundled catalog install and rollback failed: %w", rollbackErr)
		}
		return nil, "", catalogType, err
	}
	result, err = parent.OpenRoot(repoName)
	if err == nil {
		m.bundled.setNeedsReindex(catalog, true)
	}
	return result, pin, catalogType, err
}

// BootstrapBundled reconciles only configured, exact-matching bundled sources.
// Unmatched custom catalogs continue through the existing background refresh.
func (m *Manager) BootstrapBundled() error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if m.bundled == nil {
		return nil
	}
	if err := m.readConfig(); err != nil {
		return err
	}
	var failures []error
	for name, config := range m.config {
		catalog := model.Catalog{Name: name, EnvironmentId: "global", URL: config.URL, Branch: config.Branch, PinnedCommit: config.PinnedCommit, Kind: config.Kind}
		if m.bundled.matchCatalog(catalog) == nil {
			continue
		}
		if existing, err := m.lookupCatalog("global", name); err == nil && existing.URL == catalog.URL && existing.Branch == catalog.Branch && existing.PinnedCommit == catalog.PinnedCommit && effectiveCatalogKind(existing.Kind) == effectiveCatalogKind(catalog.Kind) {
			catalog = existing
		}
		if err := m.refreshCatalog(catalog, false); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) != 0 {
		return &RepoRefreshError{Errors: failures}
	}
	return nil
}

func (m *Manager) bundledRequirementsPresent(catalog model.Catalog) (bool, error) {
	source := m.bundled.matchCatalog(catalog)
	if source == nil {
		return true, nil
	}
	for _, required := range source.RequiredTemplates {
		var count int64
		err := m.db.Table("catalog_version").
			Joins("JOIN catalog_template ON catalog_template.id = catalog_version.template_id").
			Joins("JOIN catalog ON catalog.id = catalog_template.catalog_id").
			Where("catalog.name = ? AND catalog.environment_id = ? AND catalog_template.base = ? AND catalog_template.folder_name = ? AND catalog_version.revision = ?",
				catalog.Name, catalog.EnvironmentId, required.Base, required.Name, required.Revision).Count(&count).Error
		if err != nil || count == 0 {
			return false, err
		}
	}
	return true, nil
}

func (m *Manager) validateBundledTemplates(catalog model.Catalog, templates []model.Template) error {
	source := m.bundled.matchCatalog(catalog)
	if source == nil {
		return nil
	}
	if len(templates) == 0 {
		return fmt.Errorf("Bundled catalog has no templates")
	}
	for _, required := range source.RequiredTemplates {
		found := false
		for _, template := range templates {
			if template.Base == required.Base && template.FolderName == required.Name {
				for _, version := range template.Versions {
					if version.Revision != nil && *version.Revision == required.Revision {
						found = true
					}
				}
			}
		}
		if !found {
			return fmt.Errorf("Bundled catalog is missing a required template revision")
		}
	}
	return nil
}

// CheckBundledReady reads the current DB projection without waiting for custom
// catalog network refreshes. It does not equate any nonempty index with readiness.
func (m *Manager) CheckBundledReady() error {
	if m.bundled == nil {
		return nil
	}
	config, err := readCatalogConfig(m.configFile)
	if err != nil {
		return err
	}
	for name, config := range config {
		expected := model.Catalog{Name: name, EnvironmentId: "global", URL: config.URL, Branch: config.Branch, PinnedCommit: config.PinnedCommit, Kind: config.Kind}
		source := m.bundled.matchCatalog(expected)
		if source == nil {
			continue
		}
		if !m.bundled.isVerified(expected) || m.bundled.needsReindex(expected) {
			return fmt.Errorf("Bundled catalog source has not been verified")
		}
		catalog, err := m.lookupCatalog("global", name)
		if err != nil || catalog.URL != expected.URL || catalog.Branch != expected.Branch || effectiveCatalogKind(catalog.Kind) != effectiveCatalogKind(expected.Kind) ||
			!strings.EqualFold(catalog.PinnedCommit, source.PinnedCommit) || catalog.Commit != source.PinnedCommit {
			return fmt.Errorf("Bundled catalog index has not reached its pinned commit")
		}
		populated, err := m.catalogHasTemplates(catalog)
		if err != nil || !populated {
			return fmt.Errorf("Bundled catalog index is incomplete")
		}
		present, err := m.bundledRequirementsPresent(catalog)
		if err != nil || !present {
			return fmt.Errorf("Bundled catalog required revisions are not indexed")
		}
	}
	return nil
}
