package manager

import (
	"fmt"

	"github.com/PastureStack/catalog-service/model"
	"github.com/PastureStack/catalog-service/outbound"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	HelmTemplateType     = "helm"
	NativeTemplateType   = "native"
	HelmTemplateBaseType = "kubernetes"
)

type Manager struct {
	cacheRoot    string
	configFile   string
	config       map[string]CatalogConfig
	strict       bool
	db           *gorm.DB
	sourcePolicy *outbound.SourcePolicy
	httpClient   *outbound.Client
}

func NewManager(cacheRoot string, configFile string, strict bool, db *gorm.DB) (*Manager, error) {
	policy, err := outbound.FromEnvironment()
	if err != nil {
		return nil, err
	}
	return newManagerWithPolicy(cacheRoot, configFile, strict, db, policy), nil
}

func newManagerWithPolicy(cacheRoot string, configFile string, strict bool, db *gorm.DB, policy *outbound.SourcePolicy) *Manager {
	return &Manager{
		cacheRoot:    cacheRoot,
		configFile:   configFile,
		strict:       strict,
		db:           db,
		sourcePolicy: policy,
		httpClient:   outbound.NewClient(policy.Origins),
	}
}

func (m *Manager) RefreshAll(update bool) error {
	if err := m.refreshConfigCatalogs(update); err != nil {
		return err
	}
	return m.refreshEnvironmentCatalogs("", update)
}

func (m *Manager) Refresh(environmentId string, update bool) error {
	if environmentId == "global" {
		return m.refreshConfigCatalogs(update)
	}
	return m.refreshEnvironmentCatalogs(environmentId, update)
}

type RepoRefreshError struct {
	Errors []error
}

func (e *RepoRefreshError) Error() string {
	return fmt.Sprintf("catalog refresh failed for %d source(s)", len(e.Errors))
}

func (m *Manager) refreshConfigCatalogs(update bool) error {
	if err := m.readConfig(); err != nil {
		return err
	}
	if err := m.removeCatalogsNotInConfig(); err != nil {
		return err
	}

	var refreshErrors []error
	for name, config := range m.config {
		catalog := model.Catalog{
			Name:          name,
			URL:           config.URL,
			Branch:        config.Branch,
			PinnedCommit:  config.PinnedCommit,
			EnvironmentId: "global",
			Kind:          config.Kind,
		}
		existingCatalog, err := m.lookupCatalog("global", name)
		if err == nil && existingCatalog.URL == catalog.URL &&
			existingCatalog.Branch == catalog.Branch &&
			existingCatalog.PinnedCommit == catalog.PinnedCommit {
			catalog = existingCatalog
		}
		if err := m.refreshCatalog(catalog, update); err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("Catalog refresh failed for %s (%s): %w", safeLogValue(catalog.Name), safeLogValue(catalog.URL), err))
		}
	}
	if len(refreshErrors) > 0 {
		return &RepoRefreshError{Errors: refreshErrors}
	}
	return nil
}

func (m *Manager) refreshEnvironmentCatalogs(environmentId string, update bool) error {
	catalogs, err := m.lookupCatalogs(environmentId)
	if err != nil {
		return err
	}

	var refreshErrors []error
	for _, catalog := range catalogs {
		if err := m.refreshCatalog(catalog, update); err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("Catalog refresh failed for %s (%s): %w", safeLogValue(catalog.Name), safeLogValue(catalog.URL), err))
		}
	}
	if len(refreshErrors) > 0 {
		return &RepoRefreshError{Errors: refreshErrors}
	}
	return nil
}

func (m *Manager) refreshCatalog(catalog model.Catalog, update bool) error {
	repoRoot, commit, catalogType, err := m.prepareRepoPath(catalog, update)
	if err != nil {
		return err
	}
	defer repoRoot.Close()

	if commit == catalog.Commit {
		hasTemplates, err := m.catalogHasTemplates(catalog)
		if err != nil {
			return fmt.Errorf("Catalog index check failed: %w", err)
		}
		if hasTemplates {
			log.Debug("Catalog is already up to date")
			return nil
		}
		log.Warn("Catalog index is empty or contains unnamed templates; rebuilding")
	}

	templates, errs, err := traverseFiles(repoRoot, catalog.Kind, catalogType, m.httpClient, catalog.URL)
	if err != nil {
		return fmt.Errorf("Repo traversal failed: %w", err)
	}

	if len(errs) != 0 {
		if m.strict {
			return fmt.Errorf("%v", errs)
		}
		log.WithField("error_count", len(errs)).Warn("Catalog contains entries that could not be parsed")
	}

	log.Debug("Updating catalog")
	return m.updateDb(catalog, templates, commit)
}

func (m *Manager) catalogHasTemplates(catalog model.Catalog) (bool, error) {
	var counts struct {
		Total int64
		Blank int64
	}
	err := m.db.Table("catalog_template").
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN catalog_template.folder_name IS NULL OR catalog_template.folder_name = '' THEN 1 ELSE 0 END), 0) AS blank").
		Joins("JOIN catalog ON catalog.id = catalog_template.catalog_id").
		Where("catalog.name = ? AND catalog.environment_id = ?", catalog.Name, catalog.EnvironmentId).
		Scan(&counts).Error
	return counts.Total > 0 && counts.Blank == 0, err
}
