package model

import (
	"strings"

	"github.com/Masterminds/semver/v3"
	client "github.com/PastureStack/catalog-service/internal/catalogclient"
	"gorm.io/gorm"
)

const (
	baseVersionQuery = `SELECT catalog_version.*
FROM catalog_version, catalog_template, catalog
WHERE (catalog.environment_id = ? OR catalog.environment_id = ?)
AND catalog_version.template_id = catalog_template.id
AND catalog_template.catalog_id = catalog.id
AND catalog.name = ?
AND catalog_template.base = ?
AND catalog_template.folder_name = ?`
)

type Version struct {
	TemplateId uint `gorm:"type:integer REFERENCES catalog_template(id) ON DELETE CASCADE"`

	Revision              *int   `json:"revision"`
	Version               string `json:"version"`
	MinimumRancherVersion string `json:"minimumRancherVersion" yaml:"minimum_rancher_version"`
	MaximumRancherVersion string `json:"maximumRancherVersion" yaml:"maximum_rancher_version"`
	UpgradeFrom           string `json:"upgradeFrom" yaml:"upgrade_from"`
	Readme                string `json:"readme"`

	Labels map[string]string `gorm:"-" json:"labels"`

	Files     []File     `gorm:"-"`
	Questions []Question `gorm:"-"`
}

type Versions []Version

type VersionModel struct {
	Base
	Version
}

type TemplateVersionResource struct {
	client.Resource
	Version

	Bindings            map[string]Bindings `json:"bindings"`
	Files               map[string]string   `json:"files"`
	Questions           []Question          `json:"questions"`
	UpgradeVersionLinks map[string]string   `json:"upgradeVersionLinks"`
	TemplateId          string              `json:"templateId"`
}

func (v Versions) Len() int {
	return len(v)
}

func (v Versions) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v Versions) Less(i, j int) bool {
	a, aErr := semver.StrictNewVersion(strings.TrimLeft(v[i].Version, "v"))
	b, bErr := semver.StrictNewVersion(strings.TrimLeft(v[j].Version, "v"))
	if aErr != nil || bErr != nil {
		if aErr != nil && bErr != nil {
			return v[i].Version < v[j].Version
		}
		return aErr != nil
	}
	return a.LessThan(b)
}

func LookupVersionByRevision(db *gorm.DB, environmentId, catalog, base, template string, revision int) *Version {
	var versionModel VersionModel
	result := db.Raw(baseVersionQuery+`
AND catalog_version.revision = ?
`, environmentId, "global", catalog, base, template, revision).Scan(&versionModel)
	if result.Error != nil || result.RowsAffected == 0 {
		return nil
	}

	versionModel.Labels = lookupVersionLabels(db, versionModel.ID)
	versionModel.Files = lookupFiles(db, versionModel.ID)

	return &versionModel.Version
}

func LookupVersionByVersion(db *gorm.DB, environmentId, catalog, base, template string, version string) *Version {
	var versionModel VersionModel
	result := db.Raw(baseVersionQuery+`
AND catalog_version.version = ?
`, environmentId, "global", catalog, base, template, version).Scan(&versionModel)
	if result.Error != nil || result.RowsAffected == 0 {
		return nil
	}

	versionModel.Labels = lookupVersionLabels(db, versionModel.ID)
	versionModel.Files = lookupFiles(db, versionModel.ID)

	return &versionModel.Version
}

func lookupVersions(db *gorm.DB, templateId uint) []Version {
	var versionModels []VersionModel
	db.Where("template_id = ?", templateId).Find(&versionModels)

	var versions []Version
	for _, versionModel := range versionModels {
		versionModel.Labels = lookupVersionLabels(db, versionModel.ID)
		versionModel.Files = lookupFiles(db, versionModel.ID)
		versions = append(versions, versionModel.Version)
	}
	return versions
}
