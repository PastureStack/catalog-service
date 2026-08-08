package manager

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/PastureStack/catalog-service/helm"
	"github.com/PastureStack/catalog-service/model"
	"github.com/PastureStack/catalog-service/outbound"
	"github.com/PastureStack/catalog-service/parse"
	"github.com/blang/semver"
)

func traverseFiles(repoRoot *os.Root, kind string, catalogType CatalogType, httpClient *outbound.Client, sourceURL string) ([]model.Template, []error, error) {
	if kind == "" || kind == NativeTemplateType {
		return traverseGitFiles(repoRoot)
	}
	if kind == HelmTemplateType {
		if catalogType == CatalogTypeHelmGitRepo {
			return traverseHelmGitFiles(repoRoot, httpClient)
		}
		return traverseHelmFiles(repoRoot, httpClient, sourceURL)
	}
	return nil, nil, fmt.Errorf("Unknown kind %s", kind)
}

func traverseHelmGitFiles(root *os.Root, httpClient *outbound.Client) ([]model.Template, []error, error) {
	if root == nil {
		return nil, nil, fmt.Errorf("Helm repository root is missing")
	}
	templates := []model.Template{}
	var template *model.Template
	errors := []error{}
	err := fs.WalkDir(root.FS(), "stable", func(rootRelativePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if rootRelativePath == "stable" {
			return nil
		}
		relPath := strings.TrimPrefix(rootRelativePath, "stable/")
		components := strings.Split(relPath, "/")
		if len(components) == 1 {
			if template != nil {
				templates = append(templates, *template)
			}
			template = new(model.Template)
			template.Versions = make([]model.Version, 0)
			template.Versions = append(template.Versions, model.Version{
				Files: make([]model.File, 0),
			})
			template.Base = HelmTemplateBaseType
		}
		if entry == nil || entry.IsDir() {
			return nil
		}

		if strings.HasSuffix(entry.Name(), "Chart.yaml") {
			metadata, err := helm.LoadMetadata(root, rootRelativePath)
			if err != nil {
				return err
			}
			template.Description = metadata.Description
			template.DefaultVersion = metadata.Version
			if len(metadata.Sources) > 0 {
				template.ProjectURL = metadata.Sources[0]
			}
			iconData, iconFilename, err := parse.ParseIcon(httpClient, "", metadata.Icon)
			if err != nil {
				errors = append(errors, err)
			}
			rev := 0
			template.Icon = iconData
			template.IconFilename = iconFilename
			template.FolderName = components[0]
			template.Name = components[0]
			template.Versions[0].Revision = &rev
			template.Versions[0].Version = metadata.Version
		}
		file, err := helm.LoadFile(root, rootRelativePath)
		if err != nil {
			return err
		}

		file.Name = relPath

		if strings.HasSuffix(entry.Name(), "README.md") {
			template.Versions[0].Readme = file.Contents
			return nil
		}

		template.Versions[0].Files = append(template.Versions[0].Files, *file)

		return nil
	})
	if err == nil && template != nil {
		templates = append(templates, *template)
	}
	return templates, errors, err
}

func traverseHelmFiles(repoRoot *os.Root, httpClient *outbound.Client, sourceURL string) ([]model.Template, []error, error) {
	index, err := helm.LoadIndex(repoRoot)
	if err != nil {
		return nil, nil, err
	}
	index.SetSourceURL(strings.TrimSuffix(strings.TrimSpace(sourceURL), "/") + "/index.yaml")

	templates := []model.Template{}
	var errors []error
	for chart, metadata := range index.IndexFile.Entries {
		if len(metadata) == 0 {
			errors = append(errors, fmt.Errorf("Helm chart %q has no versions", chart))
			continue
		}
		template := model.Template{
			Name: chart,
		}
		template.Description = metadata[0].Description
		template.DefaultVersion = metadata[0].Version
		if len(metadata[0].Sources) > 0 {
			template.ProjectURL = metadata[0].Sources[0]
		}
		iconData, iconFilename, err := parse.ParseIcon(httpClient, index.SourceURL(), metadata[0].Icon)
		if err != nil {
			errors = append(errors, err)
		}
		template.Icon = iconData
		template.IconFilename = iconFilename
		template.Base = HelmTemplateBaseType
		versions := make([]model.Version, 0)
		for i, version := range metadata {
			v := model.Version{
				Revision: &i,
				Version:  version.Version,
			}
			files, err := helm.FetchFiles(httpClient, index.SourceURL(), version.URLs)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			filesToAdd := []model.File{}
			for _, file := range files {
				if strings.EqualFold(fmt.Sprintf("%s/%s", chart, "readme.md"), file.Name) {
					v.Readme = file.Contents
					continue
				}
				filesToAdd = append(filesToAdd, file)
			}
			v.Files = filesToAdd
			versions = append(versions, v)
		}
		template.FolderName = chart
		template.Versions = versions

		templates = append(templates, template)
	}
	return templates, errors, nil
}

func traverseGitFiles(root *os.Root) ([]model.Template, []error, error) {
	if root == nil {
		return nil, nil, fmt.Errorf("Catalog repository root is missing")
	}
	templateIndex := map[string]*model.Template{}
	var errors []error

	if err := fs.WalkDir(root.FS(), ".", func(relativePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if relativePath == "." || entry == nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		_, _, parsedCorrectly := parse.TemplatePath(relativePath)
		if !parsedCorrectly {
			return nil
		}

		_, filename := path.Split(relativePath)

		if err = handleFile(root, templateIndex, relativePath, filename); err != nil {
			errors = append(errors, fmt.Errorf("%s: %v", relativePath, err))
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	templates := []model.Template{}
	for _, template := range templateIndex {
		for i, version := range template.Versions {
			var readme string
			for _, file := range version.Files {
				if strings.ToLower(file.Name) == "readme.md" {
					readme = file.Contents
				}
			}

			var compose string
			var legacyCompose string
			var templateVersion string
			for _, file := range version.Files {
				switch file.Name {
				case "template-version.yml":
					templateVersion = file.Contents
				case "compose.yml":
					compose = file.Contents
				case "rancher-compose.yml":
					legacyCompose = file.Contents
				}
			}
			newVersion := version
			if templateVersion != "" || compose != "" || legacyCompose != "" {
				var err error
				if templateVersion != "" {
					newVersion, err = parse.CatalogInfoFromTemplateVersion([]byte(templateVersion))
				}
				if compose != "" {
					newVersion, err = parse.CatalogInfoFromCompose([]byte(compose))
				}
				if legacyCompose != "" {
					newVersion, err = parse.CatalogInfoFromLegacyCompose([]byte(legacyCompose))
				}

				if err != nil {
					var id string
					if template.Base == "" {
						id = fmt.Sprintf("%s:%d", template.FolderName, i)
					} else {
						id = fmt.Sprintf("%s*%s:%d", template.Base, template.FolderName, i)
					}
					errors = append(errors, fmt.Errorf("Failed to parse rancher-compose.yml for %s: %v", id, err))
					continue
				}
				newVersion.Revision = version.Revision
				// If rancher-compose.yml contains version, use this instead of folder version
				if newVersion.Version == "" {
					newVersion.Version = version.Version
				}
				newVersion.Files = version.Files
			}
			newVersion.Readme = readme

			template.Versions[i] = newVersion
		}
		var filteredVersions []model.Version
		for _, version := range template.Versions {
			if version.Version != "" {
				filteredVersions = append(filteredVersions, version)
			}
		}
		template.Versions = filteredVersions
		templates = append(templates, *template)
	}

	return templates, errors, nil
}

func handleFile(root *os.Root, templateIndex map[string]*model.Template, relativePath, filename string) error {
	switch {
	case filename == "config.yml" || filename == "template.yml":
		base, templateName, parsedCorrectly := parse.TemplatePath(relativePath)
		if !parsedCorrectly {
			return nil
		}
		contents, err := readRepositoryFile(root, relativePath)
		if err != nil {
			return err
		}

		var template model.Template
		if template, err = parse.TemplateInfo(contents); err != nil {
			return err
		}

		template.Base = base
		template.FolderName = templateName

		key := base + templateName

		if existingTemplate, ok := templateIndex[key]; ok {
			template.Icon = existingTemplate.Icon
			template.IconFilename = existingTemplate.IconFilename
			template.Readme = existingTemplate.Readme
			template.Versions = existingTemplate.Versions
		}
		templateIndex[key] = &template
	case strings.HasPrefix(filename, "catalogIcon") || strings.HasPrefix(filename, "icon"):
		base, templateName, parsedCorrectly := parse.TemplatePath(relativePath)
		if !parsedCorrectly {
			return nil
		}

		contents, err := readRepositoryFile(root, relativePath)
		if err != nil {
			return err
		}

		key := base + templateName

		if _, ok := templateIndex[key]; !ok {
			templateIndex[key] = &model.Template{}
		}
		templateIndex[key].Icon = base64.StdEncoding.EncodeToString([]byte(contents))
		templateIndex[key].IconFilename = filename
	case strings.HasPrefix(strings.ToLower(filename), "readme.md"):
		base, templateName, parsedCorrectly := parse.TemplatePath(relativePath)
		if !parsedCorrectly {
			return nil
		}

		_, _, _, parsedCorrectly = parse.VersionPath(relativePath)
		if parsedCorrectly {
			return handleVersionFile(root, templateIndex, relativePath, filename)
		}

		contents, err := readRepositoryFile(root, relativePath)
		if err != nil {
			return err
		}

		key := base + templateName

		if _, ok := templateIndex[key]; !ok {
			templateIndex[key] = &model.Template{}
		}
		templateIndex[key].Readme = string(contents)
	default:
		return handleVersionFile(root, templateIndex, relativePath, filename)
	}

	return nil
}

func handleVersionFile(root *os.Root, templateIndex map[string]*model.Template, relativePath, filename string) error {
	base, templateName, folderName, parsedCorrectly := parse.VersionPath(relativePath)
	if !parsedCorrectly {
		return nil
	}

	contents, err := readRepositoryFile(root, relativePath)
	if err != nil {
		return err
	}

	key := base + templateName
	file := model.File{
		Name:     filename,
		Contents: string(contents),
	}

	if _, ok := templateIndex[key]; !ok {
		templateIndex[key] = &model.Template{}
	}

	// Handle case where folder name is a revision (just a number)
	revision, err := strconv.Atoi(folderName)
	if err == nil {
		for i, version := range templateIndex[key].Versions {
			if version.Revision != nil && *version.Revision == revision {
				templateIndex[key].Versions[i].Files = append(version.Files, file)
				return nil
			}
		}
		templateIndex[key].Versions = append(templateIndex[key].Versions, model.Version{
			Revision: &revision,
			Files:    []model.File{file},
		})
		return nil
	}

	// Handle case where folder name is version (must be in semver format)
	_, err = semver.Parse(strings.Trim(folderName, "v"))
	if err == nil {
		for i, version := range templateIndex[key].Versions {
			if version.Version == folderName {
				templateIndex[key].Versions[i].Files = append(version.Files, file)
				return nil
			}
		}
		templateIndex[key].Versions = append(templateIndex[key].Versions, model.Version{
			Version: folderName,
			Files:   []model.File{file},
		})
		return nil
	}

	return nil
}

func readRepositoryFile(root *os.Root, relativePath string) ([]byte, error) {
	if root == nil || relativePath == "." || !fs.ValidPath(relativePath) {
		return nil, fmt.Errorf("Catalog repository path is not local to its root")
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const maxRepositoryFileBytes = 16 << 20
	contents, err := ioutil.ReadAll(io.LimitReader(file, maxRepositoryFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxRepositoryFileBytes {
		return nil, fmt.Errorf("Catalog repository file %q exceeds %d bytes", relativePath, maxRepositoryFileBytes)
	}
	return contents, nil
}
