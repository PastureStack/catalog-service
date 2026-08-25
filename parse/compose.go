package parse

import (
	"fmt"

	"github.com/PastureStack/catalog-service/model"
	"gopkg.in/yaml.v3"
)

type composeCatalogEnvelope struct {
	Version  string                 `yaml:"version,omitempty"`
	Services map[string]interface{} `yaml:"services,omitempty"`
}

func decodeLegacyYAML(contents []byte, target interface{}) error {
	var document yaml.Node
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return err
	}
	if err := collapseLegacyEmptyDuplicateKeys(&document); err != nil {
		return err
	}
	return document.Decode(target)
}

func collapseLegacyEmptyDuplicateKeys(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		seen := map[string]int{}
		content := make([]*yaml.Node, 0, len(node.Content))
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind == yaml.ScalarNode {
				identity := key.Tag + "\x00" + key.Value
				if previous, exists := seen[identity]; exists {
					if isYAMLNull(content[previous+1]) && !isYAMLNull(value) {
						content[previous], content[previous+1] = key, value
						continue
					}
					return fmt.Errorf("duplicate YAML mapping key %q", key.Value)
				}
				seen[identity] = len(content)
			}
			content = append(content, key, value)
		}
		node.Content = content
	}

	for _, child := range node.Content {
		if err := collapseLegacyEmptyDuplicateKeys(child); err != nil {
			return err
		}
	}
	return nil
}

func isYAMLNull(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

func convertYAML(source, target interface{}) error {
	contents, err := yaml.Marshal(source)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(contents, target)
}

func TemplateInfo(contents []byte) (model.Template, error) {
	var data map[string]interface{}
	if err := decodeLegacyYAML(contents, &data); err != nil {
		return model.Template{}, err
	}

	if _, exists := data["projectURL"]; exists {
		data["project_url"] = data["projectURL"]
	}

	if _, exists := data["version"]; exists {
		data["default_version"] = data["version"]
	} else if _, exists := data["defaultVersion"]; exists {
		data["default_version"] = data["defaultVersion"]
	}

	var template model.Template
	if err := convertYAML(data, &template); err != nil {
		return model.Template{}, err
	}

	return template, nil
}

func CatalogInfoFromTemplateVersion(contents []byte) (model.Version, error) {
	var template model.Version
	if err := decodeLegacyYAML(contents, &template); err != nil {
		return model.Version{}, err
	}

	return template, nil
}

func CatalogInfoFromLegacyCompose(contents []byte) (model.Version, error) {
	var compose composeCatalogEnvelope
	if err := decodeLegacyYAML(contents, &compose); err != nil {
		return model.Version{}, err
	}
	var rawCatalogConfig interface{}

	if compose.Version == "2" && compose.Services[".catalog"] != nil {
		rawCatalogConfig = compose.Services[".catalog"]
	}

	var data map[string]interface{}
	if err := decodeLegacyYAML(contents, &data); err != nil {
		return model.Version{}, err
	}

	if data["catalog"] != nil {
		rawCatalogConfig = data["catalog"]
	} else if data[".catalog"] != nil {
		rawCatalogConfig = data[".catalog"]
	}

	if rawCatalogConfig != nil {
		var template model.Version
		if err := convertYAML(rawCatalogConfig, &template); err != nil {
			return model.Version{}, err
		}
		return template, nil
	}

	return model.Version{}, nil
}

func CatalogInfoFromCompose(contents []byte) (model.Version, error) {
	contents = []byte(extractCatalogBlock(string(contents)))
	return CatalogInfoFromLegacyCompose(contents)
}
