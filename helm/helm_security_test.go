package helm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/outbound"
)

func TestDownloadIndexAndFetchFilesStayOnAuthorizedOrigins(t *testing.T) {
	chart := chartArchive(t, map[string]string{
		"demo/Chart.yaml": "name: demo\nversion: 1.0.0\n",
		"demo/README.md":  "safe chart\n",
	})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yaml":
			fmt.Fprintf(w, "apiVersion: v1\nentries:\n  demo:\n  - name: demo\n    version: 1.0.0\n    urls:\n    - charts/demo.tgz\n")
		case "/charts/demo.tgz":
			w.Write(chart)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	policy, err := outbound.NewOriginPolicy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := outbound.NewClient(policy)
	index, err := DownloadIndex(client, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	files, err := FetchFiles(client, index.SourceURL(), index.IndexFile.Entries["demo"][0].URLs)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("file count = %d", len(files))
	}

	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unauthorized chart server must not receive a request")
	}))
	defer attacker.Close()
	if _, err := FetchFiles(client, index.SourceURL(), []string{attacker.URL + "/chart.tgz"}); err == nil {
		t.Fatal("unauthorized chart URL was accepted")
	}
}

func TestSaveIndexTruncatesExistingFile(t *testing.T) {
	repoPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoPath, "index.yaml"), []byte(strings.Repeat("x", 4096)), 0644); err != nil {
		t.Fatal(err)
	}
	index := &HelmRepoIndex{IndexFile: &IndexFile{APIVersion: "v1"}}
	root, err := os.OpenRoot(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := SaveIndex(index, root); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(repoPath, "index.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(contents, []byte("xxxx")) {
		t.Fatal("stale index contents remained after save")
	}
}

func TestLoadFileRejectsRootEscape(t *testing.T) {
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := LoadFile(root, "../outside"); err == nil {
		t.Fatal("path escape was accepted")
	}
}

func TestLoadFileRejectsSymlinkEscape(t *testing.T) {
	rootPath := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(rootPath, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		if os.IsPermission(err) {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := LoadFile(root, "escape.txt"); err == nil {
		t.Fatal("symlink escape was accepted")
	}
}

func TestFetchFilesRejectsArchivePathEscape(t *testing.T) {
	chart := chartArchive(t, map[string]string{"../escape": "unsafe"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(chart)
	}))
	defer server.Close()
	policy, err := outbound.NewOriginPolicy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FetchFiles(outbound.NewClient(policy), server.URL+"/index.yaml", []string{"chart.tgz"}); err == nil {
		t.Fatal("archive path escape was accepted")
	}
}

func TestExtractChartFilesRejectsDuplicateAndBackslashPaths(t *testing.T) {
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range []string{"demo/file.txt", "demo/file.txt"} {
		contents := []byte("content")
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := extractChartFiles(buffer.Bytes()); err == nil {
		t.Fatal("duplicate chart path was accepted")
	}

	archive := chartArchive(t, map[string]string{"demo\\escape.txt": "unsafe"})
	if _, err := extractChartFiles(archive); err == nil {
		t.Fatal("backslash chart path was accepted")
	}
}

func TestFetchFilesRejectsOversizedContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(maxChartBytes+1))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	policy, err := outbound.NewOriginPolicy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FetchFiles(outbound.NewClient(policy), server.URL+"/index.yaml", []string{"chart.tgz"}); err == nil {
		t.Fatal("oversized chart content length was accepted")
	}
}

func TestExtractChartFilesCountsDirectoryEntries(t *testing.T) {
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for i := 0; i <= maxChartFiles; i++ {
		name := fmt.Sprintf("directory-%04d/", i)
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0755, Typeflag: tar.TypeDir}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := extractChartFiles(buffer.Bytes()); err == nil {
		t.Fatal("excessive directory entries were accepted")
	}
}

func chartArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, contents := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
