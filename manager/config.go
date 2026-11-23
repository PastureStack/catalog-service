package manager

import (
	"encoding/json"
	"io/ioutil"
)

type CatalogType int

const (
	CatalogTypeNative CatalogType = iota
	CatalogTypeHelmObjectRepo
	CatalogTypeHelmGitRepo
	CatalogTypeInvalid
)

type CatalogConfig struct {
	URL          string
	Branch       string
	Kind         string
	PinnedCommit string `json:"pinnedCommit"`
}

func (m *Manager) readConfig() error {
	config, err := readCatalogConfig(m.configFile)
	if err == nil {
		m.config = config
	}
	return err
}

func readCatalogConfig(path string) (map[string]CatalogConfig, error) {
	configContents, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config map[string]map[string]CatalogConfig
	if err = json.Unmarshal(configContents, &config); err != nil {
		return nil, err
	}

	return config["catalogs"], nil
}
