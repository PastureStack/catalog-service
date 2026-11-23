//go:build cgo

package manager

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PastureStack/catalog-service/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type bundledNaming struct{ schema.NamingStrategy }

func (n bundledNaming) TableName(value string) string {
	name := strings.TrimSuffix(n.NamingStrategy.TableName(value), "_model")
	if name == "catalog" {
		return name
	}
	if name == "template_label" {
		name = "label"
	}
	return "catalog_" + name
}

func bundledIndexDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "catalog.db")+"?_foreign_keys=on"), &gorm.Config{
		NamingStrategy: bundledNaming{schema.NamingStrategy{SingularTable: true}},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.CatalogModel{}, &model.TemplateModel{}, &model.CategoryModel{}, &model.TemplateCategoryModel{}, &model.TemplateLabelModel{}, &model.VersionModel{}, &model.FileModel{}, &model.VersionLabelModel{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestBundledBootstrapIgnoresUnmatchedCustomAndRepairsOldIndex(t *testing.T) {
	f := newBundledFixture(t)
	f.writeConfig(t, map[string]CatalogConfig{"custom": {URL: f.entry.URL + "-unreachable", Branch: "main"}})
	m := f.manager(t, t.TempDir())
	m.db = bundledIndexDB(t)
	old := f.catalog
	old.PinnedCommit, old.Commit = strings.Repeat("1", 40), strings.Repeat("1", 40)
	if err := m.updateDb(old, []model.Template{{FolderName: "old", Name: "Old"}}, old.Commit); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckBundledReady(); err == nil {
		t.Fatal("old index was reported ready")
	}
	if err := m.BootstrapBundled(); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckBundledReady(); err != nil {
		t.Fatal(err)
	}
	if f.requests.Load() != 0 {
		t.Fatal("bootstrap waited for the unmatched custom source")
	}
	indexed, err := m.lookupCatalog("global", f.catalog.Name)
	if err != nil || indexed.Commit != f.entry.PinnedCommit {
		t.Fatalf("old index did not advance: %#v, %v", indexed, err)
	}
	// Health must remain independent from a custom refresh holding refreshMu.
	m.refreshMu.Lock()
	done := make(chan error, 1)
	go func() { done <- m.CheckBundledReady() }()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		t.Error("healthcheck waited for the custom refresh mutex")
	}
	m.refreshMu.Unlock()
	// Same-commit, nonempty indexes must still restore an exact missing revision.
	if err := m.db.Exec("DELETE FROM catalog_version WHERE revision = ?", 7).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CheckBundledReady(); err == nil {
		t.Fatal("missing required revision was reported ready")
	}
	if err := m.BootstrapBundled(); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckBundledReady(); err != nil {
		t.Fatalf("same-commit revision repair failed: %v", err)
	}
}

func TestBundledBadRefreshPreservesReadableIndex(t *testing.T) {
	for _, failure := range []string{"hash", "parse", "required"} {
		t.Run(failure, func(t *testing.T) {
			f := newBundledFixture(t)
			m := f.manager(t, t.TempDir())
			m.db = bundledIndexDB(t)
			if err := m.BootstrapBundled(); err != nil {
				t.Fatal(err)
			}
			before, _ := m.lookupCatalog("global", f.catalog.Name)
			var beforeRow model.TemplateModel
			if err := m.db.First(&beforeRow).Error; err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "hash":
				if err := os.WriteFile(filepath.Join(f.assets, f.entry.Bundle), []byte("bad bundle"), 0644); err != nil {
					t.Fatal(err)
				}
			case "parse":
				// A good required template plus one parse error must not publish a
				// partial index when the service's ordinary strict flag is false.
				writeTraversalFile(t, f.repo, "templates/broken/config.yml", "name: [broken\n")
				runCatalogGit(t, f.repo, "add", ".")
				runCatalogGit(t, f.repo, "commit", "-m", "broken template")
				f.entry.PinnedCommit = runCatalogGit(t, f.repo, "rev-parse", "HEAD")
				f.catalog.PinnedCommit = f.entry.PinnedCommit
				f.writeBundle(t)
				f.writeConfig(t, nil)
			case "required":
				f.entry.RequiredTemplates[0].Revision = 8
				f.writeManifest(t)
			}
			next := f.manager(t, m.cacheRoot)
			next.db = m.db
			if err := next.BootstrapBundled(); err == nil {
				t.Fatal("invalid bundled refresh succeeded")
			}
			if err := next.CheckBundledReady(); err == nil {
				t.Fatal("failed bundled refresh was reported ready")
			}
			after, err := next.lookupCatalog("global", f.catalog.Name)
			if err != nil || after.Commit != before.Commit {
				t.Fatalf("good catalog projection changed: %#v, %v", after, err)
			}
			var afterRow model.TemplateModel
			if err := m.db.First(&afterRow, beforeRow.ID).Error; err != nil || afterRow.FolderName != beforeRow.FolderName {
				t.Fatalf("old template was no longer readable: %#v, %v", afterRow, err)
			}
			if f.requests.Load() != 0 {
				t.Fatal("invalid bundle attempted remote fallback")
			}
			if failure == "hash" {
				f.writeBundle(t)
				if err := next.BootstrapBundled(); err != nil {
					t.Fatalf("corrected bundle did not recover: %v", err)
				}
				if err := next.CheckBundledReady(); err != nil {
					t.Fatalf("corrected bundle did not restore readiness: %v", err)
				}
			}
		})
	}
}

func TestBundledConcurrentStartupAndManualRefresh(t *testing.T) {
	f := newBundledFixture(t)
	m := f.manager(t, t.TempDir())
	m.db = bundledIndexDB(t)
	var group sync.WaitGroup
	errors := make(chan error, 6)
	for i := 0; i < 6; i++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			switch index % 3 {
			case 0:
				errors <- m.BootstrapBundled()
			case 1:
				errors <- m.RefreshAll(false)
			case 2:
				errors <- m.Refresh("global", true)
			}
		}(i)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := m.CheckBundledReady(); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := m.db.Table("catalog").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("concurrent refresh produced %d catalog rows: %v", count, err)
	}
}

func TestBundledCustomFailureDoesNotInvalidateGlobalReadiness(t *testing.T) {
	for _, kind := range []string{HelmTemplateType, "invalid"} {
		t.Run(kind, func(t *testing.T) {
			f := newBundledFixture(t)
			m := f.manager(t, t.TempDir())
			m.db = bundledIndexDB(t)
			if err := m.BootstrapBundled(); err != nil {
				t.Fatal(err)
			}
			custom := f.catalog
			custom.EnvironmentId, custom.Name, custom.Kind = "project-a", "custom", kind
			if err := m.db.Create(&model.CatalogModel{Catalog: custom}).Error; err != nil {
				t.Fatal(err)
			}
			if err := m.Refresh("project-a", false); err == nil {
				t.Fatal("invalid custom catalog refresh succeeded")
			}
			if err := m.CheckBundledReady(); err != nil {
				t.Fatalf("custom catalog invalidated the global index: %v", err)
			}
		})
	}
}

func TestBundledDirtyCacheRepairsSameCommitIndex(t *testing.T) {
	f := newBundledFixture(t)
	m := f.manager(t, t.TempDir())
	m.db = bundledIndexDB(t)
	if err := m.BootstrapBundled(); err != nil {
		t.Fatal(err)
	}
	path := prepareBundledFixture(t, m, f.catalog)
	writeTraversalFile(t, path, "infra-templates/healthcheck/config.yml", "name: Polluted\nversion: 1.0.0\n")
	// Model an index previously populated from this dirty tree: its commit and
	// required revision rows are both present, but its actual metadata is wrong.
	if err := m.db.Table("catalog_template").Where("folder_name = ?", "healthcheck").Update("name", "Polluted").Error; err != nil {
		t.Fatal(err)
	}
	if err := m.BootstrapBundled(); err != nil {
		t.Fatal(err)
	}
	var template model.TemplateModel
	if err := m.db.First(&template).Error; err != nil || template.Name != "Healthcheck" {
		t.Fatalf("same-commit polluted index survived cache repair: %#v, %v", template, err)
	}
	if err := m.CheckBundledReady(); err != nil {
		t.Fatal(err)
	}
}

func TestBundledExistingKindUsesNativeDefaultSemantics(t *testing.T) {
	for _, previousKind := range []string{NativeTemplateType, HelmTemplateType} {
		t.Run(previousKind, func(t *testing.T) {
			f := newBundledFixture(t)
			m := f.manager(t, t.TempDir())
			m.db = bundledIndexDB(t)
			if err := m.BootstrapBundled(); err != nil {
				t.Fatal(err)
			}
			if err := m.db.Table("catalog").Where("name = ?", f.catalog.Name).Update("kind", previousKind).Error; err != nil {
				t.Fatal(err)
			}
			var before model.TemplateModel
			if err := m.db.First(&before).Error; err != nil {
				t.Fatal(err)
			}
			fresh := f.manager(t, m.cacheRoot)
			fresh.db = m.db
			if err := fresh.BootstrapBundled(); err != nil {
				t.Fatal(err)
			}
			if err := fresh.CheckBundledReady(); err != nil {
				t.Fatalf("existing %s kind disagreed with the native default: %v", previousKind, err)
			}
			indexed, err := fresh.lookupCatalog("global", f.catalog.Name)
			if err != nil || effectiveCatalogKind(indexed.Kind) != NativeTemplateType {
				t.Fatalf("old kind overrode configured native parsing: %#v, %v", indexed, err)
			}
			if previousKind == NativeTemplateType {
				var after model.TemplateModel
				if err := m.db.First(&after, before.ID).Error; err != nil || after.ID != before.ID {
					t.Fatalf("equivalent native kind unnecessarily rewrote the index: %v", err)
				}
			}
		})
	}
}
