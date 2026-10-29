package parse

import "github.com/PastureStack/catalog-service/model"

// QuestionsFromFiles preserves the original version-resource precedence for
// both the ordinary UI and the typed launch owner. Private defaults remain in
// the returned model only; callers must use launchoptions.Build for safe DTOs.
func QuestionsFromFiles(files map[string]string) ([]model.Question, error) {
	var info model.Version
	var err error
	if contents, exists := files["template-version.yml"]; exists {
		info, err = CatalogInfoFromTemplateVersion([]byte(contents))
	} else if contents, exists := files["compose.yml"]; exists {
		info, err = CatalogInfoFromCompose([]byte(contents))
	} else if contents, exists := files["rancher-compose.yml"]; exists {
		info, err = CatalogInfoFromLegacyCompose([]byte(contents))
	}
	if err != nil {
		return nil, err
	}
	return info.Questions, nil
}
