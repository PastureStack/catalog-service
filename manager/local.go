package manager

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/PastureStack/catalog-service/git"
	"github.com/PastureStack/catalog-service/helm"
	"github.com/PastureStack/catalog-service/model"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var fullCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func dirEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func (m *Manager) prepareRepoPath(catalog model.Catalog, update bool) (string, string, CatalogType, error) {
	if catalog.Kind == "" || catalog.Kind == NativeTemplateType {
		return m.prepareGitRepoPath(catalog, update, CatalogTypeNative)
	}
	if catalog.Kind == HelmTemplateType {
		if git.IsValid(catalog.URL) {
			return m.prepareGitRepoPath(catalog, update, CatalogTypeHelmGitRepo)
		}
		return m.prepareHelmRepoPath(catalog, update)
	}
	return "", "", CatalogTypeInvalid, fmt.Errorf("Unknown catalog kind=%s", catalog.Kind)
}

func (m *Manager) prepareHelmRepoPath(catalog model.Catalog, update bool) (string, string, CatalogType, error) {
	index, err := helm.DownloadIndex(catalog.URL)
	if err != nil {
		return "", "", CatalogTypeInvalid, err
	}

	repoPath := path.Join(m.cacheRoot, catalog.EnvironmentId, index.Hash)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", "", CatalogTypeInvalid, err
	}

	if err := helm.SaveIndex(index, repoPath); err != nil {
		return "", "", CatalogTypeInvalid, err
	}

	return repoPath, index.Hash, CatalogTypeHelmObjectRepo, nil
}

func (m *Manager) prepareGitRepoPath(catalog model.Catalog, update bool, catalogType CatalogType) (string, string, CatalogType, error) {
	branch := catalog.Branch
	if catalog.Branch == "" {
		branch = "master"
	}
	pinnedCommit := strings.TrimSpace(catalog.PinnedCommit)
	if pinnedCommit != "" && !fullCommitSHA.MatchString(pinnedCommit) {
		return "", "", catalogType, fmt.Errorf("Pinned commit must be a full 40-character Git commit SHA")
	}
	pinnedCommit = strings.ToLower(pinnedCommit)

	sum := md5.Sum([]byte(catalog.URL + branch + pinnedCommit))
	repoBranchHash := hex.EncodeToString(sum[:])
	repoPath := path.Join(m.cacheRoot, catalog.EnvironmentId, repoBranchHash)

	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", "", catalogType, errors.Wrap(err, "mkdir failed")
	}

	empty, err := dirEmpty(repoPath)
	if err != nil {
		return "", "", catalogType, errors.Wrap(err, "Empty directory check failed")
	}

	if empty {
		if err = git.Clone(repoPath, catalog.URL, branch); err != nil {
			return "", "", catalogType, errors.Wrap(err, "Clone failed")
		}
		if pinnedCommit != "" {
			if err = git.CheckoutCommit(repoPath, pinnedCommit); err != nil {
				return "", "", catalogType, errors.Wrap(err, "Pinned commit checkout failed")
			}
		}
	} else {
		if pinnedCommit != "" {
			currentCommit, headErr := git.HeadCommit(repoPath)
			if headErr != nil {
				return "", "", catalogType, errors.Wrap(headErr, "Retrieving pinned catalog commit failed")
			}
			if !strings.EqualFold(currentCommit, pinnedCommit) {
				if err = git.CheckoutCommit(repoPath, pinnedCommit); err != nil {
					return "", "", catalogType, errors.Wrap(err, "Pinned commit checkout failed")
				}
			}
		} else if update {
			changed, err := m.remoteShaChanged(catalog.URL, catalog.Branch, catalog.Commit, m.uuid)
			if err != nil {
				return "", "", catalogType, errors.Wrap(err, "Remote commit check failed")
			}
			if changed {
				if err = git.Update(repoPath, branch); err != nil {
					return "", "", catalogType, errors.Wrap(err, "Update failed")
				}
				log.Debugf("catalog-service: updated catalog '%v'", catalog.Name)
			}
		}
	}

	commit, err := git.HeadCommit(repoPath)
	if err != nil {
		err = errors.Wrap(err, "Retrieving head commit failed")
	} else if pinnedCommit != "" && !strings.EqualFold(commit, pinnedCommit) {
		err = fmt.Errorf("Catalog HEAD %s does not match pinned commit %s", commit, pinnedCommit)
	}
	return repoPath, commit, catalogType, err
}

func formatGitURL(endpoint, branch string) string {
	formattedURL := ""
	if u, err := url.Parse(endpoint); err == nil {
		pathParts := strings.Split(u.Path, "/")
		switch strings.Split(u.Host, ":")[0] {
		case "github.com":
			if len(pathParts) >= 3 {
				org := pathParts[1]
				repo := strings.TrimSuffix(pathParts[2], ".git")
				formattedURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", org, repo, branch)
			}
		case "git.rancher.io":
			repo := strings.TrimSuffix(pathParts[1], ".git")
			u.Path = fmt.Sprintf("/repos/%s/commits/%s", repo, branch)
			formattedURL = u.String()
		}
	}
	return formattedURL
}

func (m *Manager) remoteShaChanged(repoURL, branch, sha, uuid string) (bool, error) {
	formattedURL := formatGitURL(repoURL, branch)

	if formattedURL == "" {
		return true, nil
	}

	req, err := http.NewRequest("GET", formattedURL, nil)
	if err != nil {
		log.Warnf("Problem creating request to check git remote sha of repo [%v]: %v", repoURL, err)
		return true, nil
	}
	req.Header.Set("Accept", "application/vnd.github.chitauri-preview+sha")
	req.Header.Set("If-None-Match", fmt.Sprintf("\"%s\"", sha))
	if uuid != "" {
		req.Header.Set("X-Install-Uuid", uuid)
	}
	res, err := m.httpClient.Do(req)
	if err != nil {
		// Return timeout errors so caller can decide whether or not to proceed with updating the repo
		if uErr, ok := err.(*url.Error); ok && uErr.Timeout() {
			return false, errors.Wrapf(uErr, "Repo [%v] is not accessible", repoURL)
		}
		return true, nil
	}
	defer res.Body.Close()

	if res.StatusCode == 304 {
		return false, nil
	}

	return true, nil
}
