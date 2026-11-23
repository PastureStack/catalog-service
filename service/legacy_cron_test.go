//go:build cgo

package service

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type cronNaming struct{ schema.NamingStrategy }

func (n cronNaming) TableName(value string) string {
	name := strings.TrimSuffix(n.NamingStrategy.TableName(value), "_model")
	if name == "catalog" {
		return name
	}
	if name == "template_label" {
		name = "label"
	}
	return "catalog_" + name
}

func cronDB(t *testing.T, environment string) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "catalog.db")), &gorm.Config{
		NamingStrategy: cronNaming{schema.NamingStrategy{SingularTable: true}},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.CatalogModel{}, &model.TemplateModel{}, &model.CategoryModel{}, &model.TemplateCategoryModel{}, &model.TemplateLabelModel{}, &model.VersionModel{}, &model.FileModel{}, &model.VersionLabelModel{}); err != nil {
		t.Fatal(err)
	}
	cat := model.CatalogModel{Catalog: model.Catalog{Name: "pasturestack", EnvironmentId: environment, URL: "https://github.com/PastureStack/catalog-templates.git"}}
	if err := database.Create(&cat).Error; err != nil {
		t.Fatal(err)
	}
	template := model.TemplateModel{Template: model.Template{CatalogId: cat.ID, EnvironmentId: environment, Base: "infra", FolderName: "container-crontab", Name: "Legacy cron bridge", DefaultVersion: "v0.6.2"}}
	if err := database.Create(&template).Error; err != nil {
		t.Fatal(err)
	}
	label := model.TemplateLabelModel{TemplateLabel: model.TemplateLabel{TemplateId: template.ID, Key: "io.pasturestack.catalog.compatibility-only", Value: "true"}}
	if err := database.Create(&label).Error; err != nil {
		t.Fatal(err)
	}
	for revision, version := range map[int]string{3: "v0.5.0", 4: "v0.6.2"} {
		rev := revision
		v := model.VersionModel{Version: model.Version{TemplateId: template.ID, Revision: &rev, Version: version}}
		if err := database.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
		file := model.FileModel{File: model.File{VersionId: v.ID, Name: "docker-compose.yml", Contents: "container-crontab:\n  image: reviewed/cron:" + version + "\n"}}
		if err := database.Create(&file).Error; err != nil {
			t.Fatal(err)
		}
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return database
}

func TestLegacyCronExactReadsRetainIDsAndNativeUpgradeLinks(t *testing.T) {
	previousManager, previousDB := m, db
	t.Cleanup(func() { m, db = previousManager, previousDB })
	database := cronDB(t, "global")
	router := NewRouter(nil, database)
	for _, path := range []string{
		"/v1-catalog/templates/library:infra*container-crontab",
		"/v1-catalog/templates/library:infra*container-crontab:3",
		"/v1-catalog/templateversions/library:infra*container-crontab:3",
		"/v1-catalog/templateversions/library:infra*container-crontab:4",
	} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest("GET", path+"?projectId=1a5&rancherVersion=v1.6.604", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			expected := path[strings.LastIndex(path, "/")+1:]
			if body["id"] != expected {
				t.Fatalf("requested ID replaced: %v", body["id"])
			}
			if strings.HasSuffix(path, ":3") {
				links, ok := body["upgradeVersionLinks"].(map[string]any)
				if !ok || !strings.Contains(links["v0.6.2"].(string), "library:infra*container-crontab:4") {
					t.Fatalf("native upgrade target missing: %v", links)
				}
				files := body["files"].(map[string]any)
				if !strings.HasPrefix(files["docker-compose.yml"].(string), "container-crontab:") {
					t.Fatal("legacy service identity changed")
				}
			}
		})
	}
	for _, query := range []string{"projectId=1a5", "projectId=1a5&catalogId=pasturestack"} {
		request := httptest.NewRequest("GET", "/v1-catalog/templates?"+query, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 200 || strings.Contains(response.Body.String(), "container-crontab") {
			t.Fatalf("compatibility-only template was offered to new installs: %d %s", response.Code, response.Body.String())
		}
	}
	var count int64
	if err := database.Table("catalog").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("GET created a duplicate library catalog")
	}
}

func TestLegacyCronAliasDoesNotCrossProjectsInventRevisionsOrOverrideCustomSources(t *testing.T) {
	database := cronDB(t, "project-a")
	if template, _ := model.LookupTemplateForRead(database, "project-b", "library", "container-crontab", "infra"); template != nil {
		t.Fatal("another project's template was exposed")
	}
	if template, _ := model.LookupTemplateForRead(database, "project-a", "custom", "container-crontab", "infra"); template != nil {
		t.Fatal("custom namespace inherited the alias")
	}
	template, source := model.LookupTemplateForRead(database, "project-a", "library", "container-crontab", "infra")
	if template == nil || source != "pasturestack" || model.LookupVersionByRevision(database, "project-a", source, "infra", "container-crontab", 999) != nil {
		t.Fatal("alias missing or fabricated a historical revision")
	}
	custom := model.CatalogModel{Catalog: model.Catalog{Name: "library", EnvironmentId: "project-a", URL: "https://custom.example/catalog.git"}}
	if err := database.Create(&custom).Error; err != nil {
		t.Fatal(err)
	}
	if template, _ := model.LookupTemplateForRead(database, "project-a", "library", "container-crontab", "infra"); template != nil {
		t.Fatal("custom library source was silently substituted")
	}
	native := model.TemplateModel{Template: model.Template{CatalogId: custom.ID, EnvironmentId: "project-a", Base: "infra", FolderName: "container-crontab", Name: "Native custom cron"}}
	if err := database.Create(&native).Error; err != nil {
		t.Fatal(err)
	}
	if template, source := model.LookupTemplateForRead(database, "project-a", "library", "container-crontab", "infra"); template == nil || source != "library" || template.Name != native.Name {
		t.Fatal("native source lost precedence")
	}
}

func TestCompatibilityMarkerCannotHideUnrelatedTemplates(t *testing.T) {
	for _, catalog := range []string{"library", "custom", "pasturestack"} {
		template := model.Template{Catalog: catalog, Base: "infra", FolderName: "another-template", Labels: map[string]string{"io.pasturestack.catalog.compatibility-only": "true"}}
		if model.IsCompatibilityOnlyTemplate(template) {
			t.Fatal("unrelated template hidden")
		}
	}
}
