package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestContainsString(t *testing.T) {
	assert.True(t, containsString([]string{"Storage", "Networking"}, "Storage"))
	assert.False(t, containsString([]string{"Storage", "Networking"}, "storage"))
	assert.False(t, containsString(nil, "Storage"))
}

func TestLookupTemplatesExpandsGormV2SliceBindings(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE catalog (id INTEGER PRIMARY KEY, environment_id TEXT, name TEXT)`,
		`CREATE TABLE catalog_template (id INTEGER PRIMARY KEY, catalog_id INTEGER, environment_id TEXT, base TEXT, folder_name TEXT, name TEXT)`,
		`CREATE TABLE catalog_category (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE catalog_template_category (template_id INTEGER, category_id INTEGER)`,
		`CREATE TABLE catalog_label (id INTEGER PRIMARY KEY, template_id INTEGER, key TEXT, value TEXT)`,
		`CREATE TABLE catalog_version (id INTEGER PRIMARY KEY, template_id INTEGER, version TEXT, revision INTEGER)`,
		`CREATE TABLE catalog_version_label (id INTEGER PRIMARY KEY, version_id INTEGER, key TEXT, value TEXT)`,
		`CREATE TABLE catalog_file (id INTEGER PRIMARY KEY, version_id INTEGER, name TEXT, contents TEXT)`,
		`INSERT INTO catalog VALUES (1, 'global', 'orig')`,
		`INSERT INTO catalog_template VALUES (10, 1, 'global', '', 'example', 'Example')`,
		`INSERT INTO catalog_category VALUES (20, 'Storage')`,
		`INSERT INTO catalog_template_category VALUES (10, 20)`,
		`INSERT INTO catalog_label VALUES (30, 10, 'tier', 'gold')`,
		`INSERT INTO catalog_version VALUES (40, 10, '1.0.0', 0)`,
		`INSERT INTO catalog_version_label VALUES (50, 40, 'channel', 'stable')`,
		`INSERT INTO catalog_file VALUES (60, 40, 'compose.yml', 'services: {}')`,
	} {
		if err := database.Exec(statement).Error; err != nil {
			t.Fatalf("execute %q: %v", statement, err)
		}
	}

	templates := LookupTemplates(database, "e1", "", "", nil, nil)
	if len(templates) != 1 {
		t.Fatalf("expected one template, got %d", len(templates))
	}
	template := templates[0]
	assert.Equal(t, "orig", template.Catalog)
	assert.Equal(t, []string{"Storage"}, template.Categories)
	assert.Equal(t, "gold", template.Labels["tier"])
	if len(template.Versions) != 1 {
		t.Fatalf("expected one version, got %d", len(template.Versions))
	}
	assert.Equal(t, "stable", template.Versions[0].Labels["channel"])
	if len(template.Versions[0].Files) != 1 {
		t.Fatalf("expected one version file, got %d", len(template.Versions[0].Files))
	}
	assert.Equal(t, "compose.yml", template.Versions[0].Files[0].Name)
	assert.Nil(t, LookupTemplate(database, "e1", "orig", "missing", ""))
	assert.Nil(t, LookupVersionByVersion(database, "e1", "orig", "", "example", "9.9.9"))
	assert.Nil(t, LookupVersionByRevision(database, "e1", "orig", "", "example", 99))
	assert.Nil(t, GetCatalog(database, 999))
}
