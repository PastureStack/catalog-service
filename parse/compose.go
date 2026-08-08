package parse

import (
	"github.com/PastureStack/catalog-service/model"
	"gopkg.in/yaml.v2"
)

type composeCatalogEnvelope struct {
	Version  string                 `yaml:"version,omitempty"`
	Services map[string]interface{} `yaml:"services,omitempty"`
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
	if err := yaml.Unmarshal([]byte(contents), &data); err != nil {
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
	if err := yaml.Unmarshal(contents, &template); err != nil {
		return model.Version{}, err
	}

	return template, nil
}

func CatalogInfoFromLegacyCompose(contents []byte) (model.Version, error) {
	var compose composeCatalogEnvelope
	if err := yaml.Unmarshal(contents, &compose); err != nil {
		return model.Version{}, err
	}
	var rawCatalogConfig interface{}

	if compose.Version == "2" && compose.Services[".catalog"] != nil {
		rawCatalogConfig = compose.Services[".catalog"]
	}

	var data map[string]interface{}
	if err := yaml.Unmarshal(contents, &data); err != nil {
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
