package parse

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTemplateInfoCompatibilityAliases(t *testing.T) {
	template, err := TemplateInfo([]byte(`name: example
projectURL: https://example.invalid/project
version: 1.2.3
categories:
  - Storage
`))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "example", template.Name)
	assert.Equal(t, "https://example.invalid/project", template.ProjectURL)
	assert.Equal(t, "1.2.3", template.DefaultVersion)
	assert.Equal(t, []string{"Storage"}, template.Categories)
}

func TestTemplateInfoRejectsInvalidYAML(t *testing.T) {
	_, err := TemplateInfo([]byte("name: ["))
	assert.Error(t, err)
}

func TestCatalogInfoFromLegacyComposeV1(t *testing.T) {
	version, err := CatalogInfoFromLegacyCompose([]byte(`catalog:
  version: 1.0.0
  minimum_rancher_version: v1.6.0
  questions:
    - variable: storage_path
      label: Storage path
`))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "1.0.0", version.Version)
	assert.Equal(t, "v1.6.0", version.MinimumRancherVersion)
	if len(version.Questions) != 1 {
		t.Fatalf("expected one question, got %d", len(version.Questions))
	}
	assert.Equal(t, "storage_path", version.Questions[0].Variable)
}

func TestCatalogInfoFromLegacyComposeV2(t *testing.T) {
	version, err := CatalogInfoFromLegacyCompose([]byte(`version: '2'
services:
  app:
    image: example.invalid/app:1.0.0
  .catalog:
    version: 2.0.0
    upgrade_from: 1.0.0
`))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "2.0.0", version.Version)
	assert.Equal(t, "1.0.0", version.UpgradeFrom)
}

func TestTopLevelCatalogTakesPrecedence(t *testing.T) {
	version, err := CatalogInfoFromLegacyCompose([]byte(`version: '2'
services:
  .catalog:
    version: 2.0.0
catalog:
  version: 2.0.1
`))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "2.0.1", version.Version)
}

func TestCatalogInfoFromLegacyComposeWithoutCatalog(t *testing.T) {
	version, err := CatalogInfoFromLegacyCompose([]byte(`version: '2'
services:
  app:
    image: example.invalid/app:1.0.0
`))
	if err != nil {
		t.Fatal(err)
	}
	assert.Empty(t, version.Version)
}

func TestCatalogInfoFromLegacyComposeRejectsInvalidYAML(t *testing.T) {
	_, err := CatalogInfoFromLegacyCompose([]byte("version: ["))
	assert.Error(t, err)
}

func TestCatalogInfoFromComposeExtractsOnlyMetadata(t *testing.T) {
	version, err := CatalogInfoFromCompose([]byte(`service:
  image: example.invalid/app:1.0.0
.catalog:
  version: 3.0.0
  maximum_rancher_version: v1.6.99
`))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "3.0.0", version.Version)
	assert.Equal(t, "v1.6.99", version.MaximumRancherVersion)
}
