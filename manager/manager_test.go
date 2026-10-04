//go:build cgo
// +build cgo

package manager

import (
	"testing"

	"github.com/PastureStack/catalog-service/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openCatalogIndexTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	for _, statement := range []string{
		`CREATE TABLE catalog (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			environment_id TEXT NOT NULL
		)`,
		`CREATE TABLE catalog_template (
			id INTEGER PRIMARY KEY,
			catalog_id INTEGER NOT NULL,
			environment_id TEXT NOT NULL,
			folder_name TEXT
		)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	return db
}

func TestCatalogHasTemplates(t *testing.T) {
	db := openCatalogIndexTestDB(t)
	manager := &Manager{db: db}
	catalog := model.Catalog{Name: "pasturestack", EnvironmentId: "global"}

	if err := db.Exec(
		`INSERT INTO catalog (id, name, environment_id) VALUES (?, ?, ?)`,
		1, catalog.Name, catalog.EnvironmentId,
	).Error; err != nil {
		t.Fatal(err)
	}

	hasTemplates, err := manager.catalogHasTemplates(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if hasTemplates {
		t.Fatal("empty catalog index was reported as populated")
	}

	if err := db.Exec(
		`INSERT INTO catalog_template (id, catalog_id, environment_id, folder_name) VALUES (?, ?, ?, ?)`,
		1, 1, catalog.EnvironmentId, "fixture",
	).Error; err != nil {
		t.Fatal(err)
	}

	hasTemplates, err = manager.catalogHasTemplates(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTemplates {
		t.Fatal("populated catalog index was reported as empty")
	}
}

func TestCatalogHasTemplatesDoesNotCrossEnvironments(t *testing.T) {
	db := openCatalogIndexTestDB(t)
	manager := &Manager{db: db}

	if err := db.Exec(
		`INSERT INTO catalog (id, name, environment_id) VALUES (?, ?, ?)`,
		1, "pasturestack", "project-a",
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(
		`INSERT INTO catalog_template (id, catalog_id, environment_id, folder_name) VALUES (?, ?, ?, ?)`,
		1, 1, "project-a", "fixture",
	).Error; err != nil {
		t.Fatal(err)
	}

	hasTemplates, err := manager.catalogHasTemplates(model.Catalog{
		Name:          "pasturestack",
		EnvironmentId: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasTemplates {
		t.Fatal("catalog index check matched a different environment")
	}
}

func TestCatalogHasTemplatesRejectsBlankCachedRows(t *testing.T) {
	for _, blank := range []interface{}{"", nil} {
		db := openCatalogIndexTestDB(t)
		manager := &Manager{db: db}
		catalog := model.Catalog{Name: "qa-catalog", EnvironmentId: "project-a"}
		if err := db.Exec(`INSERT INTO catalog (id, name, environment_id) VALUES (?, ?, ?)`, 1, catalog.Name, catalog.EnvironmentId).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO catalog_template (id, catalog_id, environment_id, folder_name) VALUES (?, ?, ?, ?), (?, ?, ?, ?)`, 1, 1, catalog.EnvironmentId, "fixture", 2, 1, catalog.EnvironmentId, blank).Error; err != nil {
			t.Fatal(err)
		}
		populated, err := manager.catalogHasTemplates(catalog)
		if err != nil || populated {
			t.Fatalf("index containing blank template %v bypassed repair: %v", blank, err)
		}
		if err := db.Exec(`DELETE FROM catalog_template WHERE id = ?`, 2).Error; err != nil {
			t.Fatal(err)
		}
		populated, err = manager.catalogHasTemplates(catalog)
		if err != nil || !populated {
			t.Fatalf("clean same-commit index was not reusable: %v", err)
		}
	}
}

func TestCatalogHasTemplatesIgnoresBlankRowsFromOtherCatalogs(t *testing.T) {
	db := openCatalogIndexTestDB(t)
	manager := &Manager{db: db}
	catalog := model.Catalog{Name: "qa-catalog", EnvironmentId: "project-a"}
	if err := db.Exec(`INSERT INTO catalog (id, name, environment_id) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)`, 1, catalog.Name, catalog.EnvironmentId, 2, "other-catalog", catalog.EnvironmentId, 3, catalog.Name, "project-b").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO catalog_template (id, catalog_id, environment_id, folder_name) VALUES (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?)`, 1, 1, catalog.EnvironmentId, "fixture", 2, 2, catalog.EnvironmentId, "", 3, 3, "project-b", "").Error; err != nil {
		t.Fatal(err)
	}
	populated, err := manager.catalogHasTemplates(catalog)
	if err != nil || !populated {
		t.Fatalf("foreign blank index affected the clean selected Catalog: %v", err)
	}
}
