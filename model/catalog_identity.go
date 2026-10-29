package model

import "fmt"

// TemplateAPIID is the existing Catalog API identity used by ordinary launch.
// It is transport material, not the stable authorization identity of AI metadata.
func TemplateAPIID(catalog string, template Template) string {
	if template.Base == "" {
		return fmt.Sprintf("%s:%s", catalog, template.FolderName)
	}
	return fmt.Sprintf("%s:%s*%s", catalog, template.Base, template.FolderName)
}

func VersionAPIID(catalog string, template Template, version Version) string {
	if version.Revision == nil {
		return fmt.Sprintf("%s:%s", TemplateAPIID(catalog, template), version.Version)
	}
	return fmt.Sprintf("%s:%d", TemplateAPIID(catalog, template), *version.Revision)
}
