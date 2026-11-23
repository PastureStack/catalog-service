package model

import "gorm.io/gorm"

const compatibilityOnlyLabel = "io.pasturestack.catalog.compatibility-only"

// IsCompatibilityOnlyTemplate keeps a historical upgrade bridge addressable
// without offering a second scheduler to new installations.
func IsCompatibilityOnlyTemplate(template Template) bool {
	return template.Catalog == "pasturestack" && template.Base == "infra" &&
		template.FolderName == "container-crontab" && template.Labels[compatibilityOnlyLabel] == "true"
}

func reviewedCronSource(raw string) bool {
	return raw == "https://github.com/PastureStack/catalog-templates" ||
		raw == "https://github.com/PastureStack/catalog-templates.git"
}

// LookupTemplateForRead preserves native catalog precedence. Only the known
// library cron identity may fall back to the reviewed first-party bridge. The
// second return value is the lookup namespace; public response IDs retain the
// requested namespace so ordinary Stack upgrade owns all persisted changes.
func LookupTemplateForRead(database *gorm.DB, environmentID, catalog, folder, base string) (*Template, string) {
	if template := LookupTemplate(database, environmentID, catalog, folder, base); template != nil {
		return template, catalog
	}
	if catalog != "library" || folder != "container-crontab" || base != "infra" {
		return nil, catalog
	}
	// A configured custom library must never inherit an unrelated first-party
	// template. Missing historical public libraries may use the bridge.
	if original := LookupCatalog(database, environmentID, catalog); original != nil {
		switch original.URL {
		case "https://github.com/rancher/rancher-catalog", "https://github.com/rancher/rancher-catalog.git",
			"https://git.rancher.io/rancher-catalog.git", "https://git.rancher.io/rancher-catalog":
		default:
			return nil, catalog
		}
	}
	template := LookupTemplate(database, environmentID, "pasturestack", folder, base)
	if template == nil || !IsCompatibilityOnlyTemplate(*template) {
		return nil, catalog
	}
	source := GetCatalog(database, template.CatalogId)
	if source == nil || !reviewedCronSource(source.URL) {
		return nil, catalog
	}
	return template, "pasturestack"
}
