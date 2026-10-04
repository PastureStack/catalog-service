package manager

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/PastureStack/catalog-service/model"
)

func writeTraversalFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	file := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func openTraversalRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestTraverseGitFilesIgnoresGitMetadata(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"templates/qa-zero-scale-ab/config.yml": `name: QA private Catalog A-B
description: Isolated native Catalog indexing regression
version: 1.0.0
labels:
  io.rancher.orchestration.supported: cattle
  io.pasturestack.catalog.name.zh-tw: 原生範本
  io.pasturestack.catalog.description.zh-tw: 零規模測試
`,
		"templates/qa-zero-scale-ab/0/docker-compose.yml":  "version: '2'\nservices:\n  qa-probe:\n    image: busybox:1.36.1\n",
		"templates/qa-zero-scale-ab/0/rancher-compose.yml": "version: '2'\n.catalog:\n  version: 1.0.0\nservices:\n  qa-probe:\n    scale: 0\n    start_on_create: false\n",
		"templates/qa-zero-scale-ab/1/docker-compose.yml":  "version: '2'\nservices:\n  qa-probe:\n    image: busybox:1.36.1\n",
		"templates/qa-zero-scale-ab/1/rancher-compose.yml": "version: '2'\n.catalog:\n  version: 1.0.1\n  upgrade_from: '=1.0.0'\nservices:\n  qa-probe:\n    scale: 0\n    start_on_create: false\n",
		".git/hooks/pre-commit.sample":                     "git metadata\n",
		".git/info/exclude":                                "git metadata\n",
		".git/logs/HEAD":                                   "git metadata\n",
		".git/objects/ab/object":                           "git metadata\n",
		".git/objects/12/object":                           "git metadata in a numeric object directory\n",
		".git/refs/heads/qa":                               "git metadata\n",
	}
	for name, contents := range files {
		writeTraversalFile(t, dir, name, contents)
	}
	templates, parseErrors, err := traverseGitFiles(openTraversalRoot(t, dir))
	if err != nil || len(parseErrors) != 0 {
		t.Fatalf("traversal failed: %v, %v", err, parseErrors)
	}
	if len(templates) != 1 {
		t.Fatalf("got %d templates, want exactly the one fixture and no Git stubs", len(templates))
	}
	template := templates[0]
	labels := map[string]string{
		"io.rancher.orchestration.supported":        "cattle",
		"io.pasturestack.catalog.name.zh-tw":        "原生範本",
		"io.pasturestack.catalog.description.zh-tw": "零規模測試",
	}
	if template.FolderName != "qa-zero-scale-ab" || template.Base != "" || template.Name != "QA private Catalog A-B" || template.DefaultVersion != "1.0.0" || !reflect.DeepEqual(template.Labels, labels) {
		t.Fatalf("fixture metadata changed: %#v", template)
	}
	if len(template.Versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(template.Versions))
	}
	seen := map[int]bool{}
	for _, version := range template.Versions {
		if version.Revision == nil || seen[*version.Revision] || len(version.Files) != 2 {
			t.Fatalf("unexpected fixture version: %#v", version)
		}
		revision := *version.Revision
		seen[revision] = true
		wantVersion, wantUpgrade := "1.0.0", ""
		if revision == 1 {
			wantVersion, wantUpgrade = "1.0.1", "=1.0.0"
		} else if revision != 0 {
			t.Fatalf("unexpected revision %d", revision)
		}
		if version.Version != wantVersion || version.UpgradeFrom != wantUpgrade {
			t.Fatalf("version metadata changed: %#v", version)
		}
		seenFiles := map[string]bool{}
		for _, file := range version.Files {
			if seenFiles[file.Name] || file.Contents != files["templates/qa-zero-scale-ab/"+strconv.Itoa(revision)+"/"+file.Name] {
				t.Fatalf("unexpected fixture file: %#v", file)
			}
			seenFiles[file.Name] = true
		}
		if !seenFiles["docker-compose.yml"] || !seenFiles["rancher-compose.yml"] {
			t.Fatalf("compose files changed: %#v", seenFiles)
		}
	}
}

func TestHandleVersionFileRejectsInvalidFolderBeforeReading(t *testing.T) {
	root := openTraversalRoot(t, t.TempDir())
	for _, folder := range []string{"not-a-version", "v", "1.2", "01.2.3"} {
		index := map[string]*model.Template{}
		err := handleVersionFile(root, index, "templates/rejected/"+folder+"/missing.yml", "missing.yml")
		if err != nil || len(index) != 0 {
			t.Fatalf("invalid folder %q caused a read or empty template: %v, %#v", folder, err, index)
		}
	}
}

func TestHandleVersionFilePreservesNumericAndSemverFolders(t *testing.T) {
	for _, folder := range []string{"0", "12", "1.2.3", "v1.2.3"} {
		t.Run(folder, func(t *testing.T) {
			dir := t.TempDir()
			name := "templates/example/" + folder + "/notes.txt"
			writeTraversalFile(t, dir, name, "kept bytes\n")
			index := map[string]*model.Template{}
			if err := handleVersionFile(openTraversalRoot(t, dir), index, name, "notes.txt"); err != nil {
				t.Fatal(err)
			}
			if len(index) != 1 || index["example"] == nil || len(index["example"].Versions) != 1 {
				t.Fatalf("valid folder did not produce exactly one version: %#v", index)
			}
			version := index["example"].Versions[0]
			if len(version.Files) != 1 || version.Files[0].Name != "notes.txt" || version.Files[0].Contents != "kept bytes\n" {
				t.Fatalf("valid file changed: %#v", version)
			}
			if folder == "0" || folder == "12" {
				wantRevision := 0
				if folder == "12" {
					wantRevision = 12
				}
				if version.Revision == nil || *version.Revision != wantRevision || version.Version != "" {
					t.Fatalf("numeric semantics changed: %#v", version)
				}
			} else if version.Revision != nil || version.Version != folder {
				t.Fatalf("semver semantics changed: %#v", version)
			}
		})
	}
}
