package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PastureStack/catalog-service/git"
	"github.com/PastureStack/catalog-service/helm"
	"github.com/PastureStack/catalog-service/model"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var fullCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
var cacheEnvironmentID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var cacheRepositoryHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

func dirEmpty(root *os.Root) (bool, error) {
	if root == nil {
		return false, fmt.Errorf("Catalog cache root is missing")
	}
	f, err := root.Open(".")
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func (m *Manager) prepareRepoPath(catalog model.Catalog, update bool) (*os.Root, string, CatalogType, error) {
	if catalog.Kind == "" || catalog.Kind == NativeTemplateType {
		return m.prepareGitRepoPath(catalog, update, CatalogTypeNative)
	}
	if catalog.Kind == HelmTemplateType {
		if source, err := m.sourcePolicy.AuthorizeGitSource(catalog.URL); err == nil && git.IsValid(source) {
			return m.prepareGitRepoPath(catalog, update, CatalogTypeHelmGitRepo)
		}
		return m.prepareHelmRepoPath(catalog, update)
	}
	return nil, "", CatalogTypeInvalid, fmt.Errorf("Unknown catalog kind=%s", catalog.Kind)
}

func (m *Manager) prepareHelmRepoPath(catalog model.Catalog, update bool) (*os.Root, string, CatalogType, error) {
	index, err := helm.DownloadIndex(m.httpClient, catalog.URL)
	if err != nil {
		return nil, "", CatalogTypeInvalid, err
	}

	repoRoot, err := m.catalogCacheRoot(catalog.EnvironmentId, index.Hash)
	if err != nil {
		return nil, "", CatalogTypeInvalid, err
	}

	if err := helm.SaveIndex(index, repoRoot); err != nil {
		repoRoot.Close()
		return nil, "", CatalogTypeInvalid, err
	}

	return repoRoot, index.Hash, CatalogTypeHelmObjectRepo, nil
}

func (m *Manager) prepareGitRepoPath(catalog model.Catalog, update bool, catalogType CatalogType) (*os.Root, string, CatalogType, error) {
	source, err := m.sourcePolicy.AuthorizeGitSource(catalog.URL)
	if err != nil {
		return nil, "", catalogType, err
	}
	branch := catalog.Branch
	if catalog.Branch == "" {
		branch = "master"
	}
	pinnedCommit := strings.TrimSpace(catalog.PinnedCommit)
	if pinnedCommit != "" && !fullCommitSHA.MatchString(pinnedCommit) {
		return nil, "", catalogType, fmt.Errorf("Pinned commit must be a full 40-character Git commit SHA")
	}
	pinnedCommit = strings.ToLower(pinnedCommit)

	sum := sha256.Sum256([]byte(catalog.URL + "\x00" + branch + "\x00" + pinnedCommit))
	repoBranchHash := hex.EncodeToString(sum[:])
	repoRoot, err := m.catalogCacheRoot(catalog.EnvironmentId, repoBranchHash)
	if err != nil {
		return nil, "", catalogType, err
	}
	repoPath := repoRoot.Name()

	empty, err := dirEmpty(repoRoot)
	if err != nil {
		repoRoot.Close()
		return nil, "", catalogType, errors.Wrap(err, "Empty directory check failed")
	}

	if empty {
		if err = git.Clone(repoPath, source, branch); err != nil {
			repoRoot.Close()
			return nil, "", catalogType, errors.Wrap(err, "Clone failed")
		}
		if pinnedCommit != "" {
			if err = git.CheckoutCommit(repoPath, pinnedCommit); err != nil {
				repoRoot.Close()
				return nil, "", catalogType, errors.Wrap(err, "Pinned commit checkout failed")
			}
		}
	} else {
		if pinnedCommit != "" {
			currentCommit, headErr := git.HeadCommit(repoPath)
			if headErr != nil {
				repoRoot.Close()
				return nil, "", catalogType, errors.Wrap(headErr, "Retrieving pinned catalog commit failed")
			}
			if !strings.EqualFold(currentCommit, pinnedCommit) {
				if err = git.CheckoutCommit(repoPath, pinnedCommit); err != nil {
					repoRoot.Close()
					return nil, "", catalogType, errors.Wrap(err, "Pinned commit checkout failed")
				}
			}
		} else if update {
			changed, err := m.remoteShaChanged(catalog.URL, branch, catalog.Commit)
			if err != nil {
				repoRoot.Close()
				return nil, "", catalogType, errors.Wrap(err, "Remote commit check failed")
			}
			if changed {
				if err = git.Update(repoPath, source, branch); err != nil {
					repoRoot.Close()
					return nil, "", catalogType, errors.Wrap(err, "Update failed")
				}
				log.Debug("Catalog source was updated")
			}
		}
	}

	commit, err := git.HeadCommit(repoPath)
	if err != nil {
		err = errors.Wrap(err, "Retrieving head commit failed")
	} else if pinnedCommit != "" && !strings.EqualFold(commit, pinnedCommit) {
		err = fmt.Errorf("Catalog HEAD %s does not match pinned commit %s", commit, pinnedCommit)
	}
	if err != nil {
		repoRoot.Close()
		return nil, commit, catalogType, err
	}
	return repoRoot, commit, catalogType, nil
}

func (m *Manager) catalogCacheRoot(environmentID, repositoryHash string) (*os.Root, error) {
	if m == nil || strings.TrimSpace(m.cacheRoot) == "" {
		return nil, fmt.Errorf("Catalog cache root is missing")
	}
	repositoryHash = strings.ToLower(repositoryHash)
	if !cacheRepositoryHash.MatchString(repositoryHash) {
		return nil, fmt.Errorf("Catalog cache key is invalid")
	}
	if !cacheEnvironmentID.MatchString(environmentID) || !filepath.IsLocal(environmentID) {
		return nil, fmt.Errorf("Catalog environment ID is not a safe path segment")
	}
	root, err := filepath.Abs(m.cacheRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	cacheRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer cacheRoot.Close()
	relative := filepath.Join(environmentID, repositoryHash)
	if !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("Catalog cache path escapes its root")
	}
	if err := cacheRoot.MkdirAll(relative, 0755); err != nil {
		return nil, err
	}
	return cacheRoot.OpenRoot(relative)
}

func formatGitURL(endpoint, branch string) string {
	formattedURL := ""
	if u, err := url.Parse(endpoint); err == nil {
		pathParts := strings.Split(u.Path, "/")
		switch strings.ToLower(u.Hostname()) {
		case "github.com":
			if u.Scheme == "https" && u.Port() == "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && len(pathParts) == 3 && pathParts[0] == "" && pathParts[1] != "" && pathParts[2] != "" {
				org := pathParts[1]
				repo := strings.TrimSuffix(pathParts[2], ".git")
				if org != "" && repo != "" {
					formattedURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", url.PathEscape(org), url.PathEscape(repo), url.PathEscape(branch))
				}
			}
		}
	}
	return formattedURL
}

func (m *Manager) remoteShaChanged(repoURL, branch, sha string) (bool, error) {
	formattedURL := formatGitURL(repoURL, branch)

	if formattedURL == "" {
		return true, nil
	}
	if m == nil || m.httpClient == nil {
		return false, fmt.Errorf("Catalog outbound HTTP client is not configured")
	}

	req, err := http.NewRequest("GET", formattedURL, nil)
	if err != nil {
		log.Warn("Catalog remote commit request could not be created")
		return true, nil
	}
	req.Header.Set("Accept", "application/vnd.github.chitauri-preview+sha")
	req.Header.Set("If-None-Match", fmt.Sprintf("\"%s\"", sha))
	res, err := m.httpClient.Do(req)
	if err != nil {
		// Return timeout errors so caller can decide whether or not to proceed with updating the repo
		if uErr, ok := err.(*url.Error); ok && uErr.Timeout() {
			return false, errors.Wrap(uErr, "Catalog repository is not accessible")
		}
		return true, nil
	}
	defer res.Body.Close()

	if res.StatusCode == 304 {
		return false, nil
	}

	return true, nil
}

func safeLogValue(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\r' || character == '\n' || character == '\t' || character < 0x20 || character == 0x7f {
			return -1
		}
		return character
	}, value)
}
