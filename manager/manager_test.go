package manager

import (
	"testing"

	"github.com/PastureStack/catalog-service/model"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

func openCatalogIndexTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
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
			environment_id TEXT NOT NULL
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
		`INSERT INTO catalog_template (id, catalog_id, environment_id) VALUES (?, ?, ?)`,
		1, 1, catalog.EnvironmentId,
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
		`INSERT INTO catalog_template (id, catalog_id, environment_id) VALUES (?, ?, ?)`,
		1, 1, "project-a",
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
