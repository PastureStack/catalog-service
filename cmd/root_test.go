package cmd

import (
	"os"
	"os/exec"
	"testing"

	"github.com/PastureStack/catalog-service/model"
	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestBundledSourcesEnvironmentAndFlag(t *testing.T) {
	if os.Getenv("PASTURESTACK_BUNDLED_FLAG_HELPER") == "1" {
		if bundledSources != "/operator/from-env/manifest.json" {
			t.Fatalf("CATALOG_SERVICE_BUNDLED_SOURCES = %q", bundledSources)
		}
		if err := RootCmd.ParseFlags([]string{"--bundled-sources", "/operator/from-flag/manifest.json"}); err != nil {
			t.Fatal(err)
		}
		if bundledSources != "/operator/from-flag/manifest.json" {
			t.Fatalf("explicit flag did not override env: %q", bundledSources)
		}
		return
	}
	t.Setenv("PASTURESTACK_BUNDLED_FLAG_HELPER", "1")
	t.Setenv("CATALOG_SERVICE_BUNDLED_SOURCES", "/operator/from-env/manifest.json")
	command := exec.Command(os.Args[0], "-test.run=^TestBundledSourcesEnvironmentAndFlag$")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("flag/env subprocess failed: %v\n%s", err, output)
	}
}

func TestTraditionalChineseOperatorMessage(t *testing.T) {
	if got := operatorMessage("zh-TW", "start"); got != "正在啟動 PastureStack 應用目錄服務" {
		t.Fatalf("unexpected zh-TW message: %q", got)
	}
}

func TestFormatDSNPreservesMySQLDriverCompatibilityDefaults(t *testing.T) {
	config, err := mysqldriver.ParseDSN(formatDSN(
		"catalog-user",
		"catalog-password",
		"127.0.0.1:3306",
		"catalog-db",
		"readTimeout=5s",
	))
	if err != nil {
		t.Fatal(err)
	}
	if !config.AllowNativePasswords {
		t.Fatal("mysql_native_password compatibility must remain enabled for existing installations")
	}
	if !config.ParseTime {
		t.Fatal("parseTime must remain enabled")
	}
	if config.ReadTimeout.String() != "5s" {
		t.Fatalf("ReadTimeout = %s, want 5s", config.ReadTimeout)
	}
}

func TestCatalogNamingStrategyPreservesExistingTables(t *testing.T) {
	strategy := newCatalogGormConfig().NamingStrategy
	for modelName, expected := range map[string]string{
		"CatalogModel":       "catalog",
		"TemplateModel":      "catalog_template",
		"TemplateLabelModel": "catalog_label",
		"VersionModel":       "catalog_version",
	} {
		if actual := strategy.TableName(modelName); actual != expected {
			t.Fatalf("TableName(%q) = %q, want %q", modelName, actual, expected)
		}
	}
}

func TestSQLiteGormV2Migration(t *testing.T) {
	database, err := openSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := database.AutoMigrate(
		&model.CatalogModel{},
		&model.TemplateModel{},
		&model.TemplateLabelModel{},
		&model.VersionModel{},
	); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"catalog", "catalog_template", "catalog_label", "catalog_version"} {
		if !database.Migrator().HasTable(table) {
			t.Fatalf("migrated table %q is missing", table)
		}
	}
}
