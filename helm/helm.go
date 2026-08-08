package helm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/PastureStack/catalog-service/model"
	"github.com/PastureStack/catalog-service/outbound"
	"gopkg.in/yaml.v2"
)

const (
	maxIndexBytes         = 16 << 20
	maxChartBytes         = 128 << 20
	maxChartExpandedBytes = 128 << 20
	maxChartFiles         = 2048
	maxFileBytes          = 16 << 20
)

func DownloadIndex(client *outbound.Client, indexURL string) (*HelmRepoIndex, error) {
	indexURL = strings.TrimSuffix(strings.TrimSpace(indexURL), "/")
	if indexURL == "" {
		return nil, fmt.Errorf("Helm repository URL is missing")
	}
	indexURL = indexURL + "/index.yaml"
	resp, err := client.Get(indexURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := requireHTTPSuccess(resp); err != nil {
		return nil, err
	}
	body, err := readLimited(resp.Body, maxIndexBytes, "Helm repository index")
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	helmRepoIndex := &HelmRepoIndex{
		URL:       indexURL,
		IndexFile: &IndexFile{},
		Hash:      hash,
	}
	return helmRepoIndex, yaml.Unmarshal(body, helmRepoIndex.IndexFile)
}

func SaveIndex(index *HelmRepoIndex, root *os.Root) error {
	fileBytes, err := yaml.Marshal(index.IndexFile)
	if err != nil {
		return err
	}

	if root == nil {
		return fmt.Errorf("Helm repository cache root is missing")
	}
	f, err := root.OpenFile("index.yaml", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(fileBytes)
	return err
}

func LoadIndex(root *os.Root) (*HelmRepoIndex, error) {
	if root == nil {
		return nil, fmt.Errorf("Helm repository cache root is missing")
	}
	f, err := root.Open("index.yaml")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	body, err := readLimited(f, maxIndexBytes, "cached Helm repository index")
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	helmRepoIndex := &HelmRepoIndex{
		IndexFile: &IndexFile{},
		Hash:      hash,
	}
	return helmRepoIndex, yaml.Unmarshal(body, helmRepoIndex.IndexFile)
}

func FetchFiles(client *outbound.Client, indexURL string, urls []string) ([]model.File, error) {
	if len(urls) == 0 {
		return nil, nil
	}

	files := []model.File{}
	for _, chartURL := range urls {
		resp, err := client.ResolveAndGet(indexURL, chartURL)
		if err != nil {
			return nil, err
		}
		if err := requireHTTPSuccess(resp); err != nil {
			resp.Body.Close()
			return nil, err
		}

		if resp.ContentLength > maxChartBytes {
			resp.Body.Close()
			return nil, fmt.Errorf("Helm chart archive exceeds %d bytes", maxChartBytes)
		}
		archive, readErr := readLimited(resp.Body, maxChartBytes, "Helm chart archive")
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		chartFiles, err := extractChartFiles(archive)
		if err != nil {
			return nil, err
		}
		files = append(files, chartFiles...)
	}
	return files, nil
}

func extractChartFiles(archive []byte) ([]model.File, error) {
	gzf, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}

	files := []model.File{}
	seen := map[string]struct{}{}
	expanded := &io.LimitedReader{R: gzf, N: maxChartExpandedBytes + 1}
	tarReader := tar.NewReader(expanded)
	var expandedBytes int64
	entryCount := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			gzf.Close()
			return nil, err
		}
		entryCount++
		if entryCount > maxChartFiles {
			gzf.Close()
			return nil, fmt.Errorf("Helm chart contains more than %d entries", maxChartFiles)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, tar.TypeRegA:
			name := strings.TrimPrefix(header.Name, "./")
			if name == "." || strings.Contains(name, "\\") || !fs.ValidPath(name) {
				gzf.Close()
				return nil, fmt.Errorf("Helm chart file path %q is not local to the archive", header.Name)
			}
			if _, exists := seen[name]; exists {
				gzf.Close()
				return nil, fmt.Errorf("Helm chart contains duplicate file %q", name)
			}
			seen[name] = struct{}{}
			if header.Size < 0 || header.Size > maxFileBytes {
				gzf.Close()
				return nil, fmt.Errorf("Helm chart file %q exceeds %d bytes", name, maxFileBytes)
			}
			if expandedBytes > maxChartExpandedBytes-header.Size {
				gzf.Close()
				return nil, fmt.Errorf("Helm chart expanded contents exceed %d bytes", maxChartExpandedBytes)
			}
			expandedBytes += header.Size
			contents, err := readLimited(tarReader, maxFileBytes, "Helm chart file")
			if err != nil {
				gzf.Close()
				return nil, err
			}
			files = append(files, filterFile(model.File{Name: name, Contents: string(contents)}))
		}
	}
	if _, err := io.Copy(io.Discard, expanded); err != nil {
		gzf.Close()
		return nil, err
	}
	if expanded.N <= 0 {
		gzf.Close()
		return nil, fmt.Errorf("Helm chart expanded contents exceed %d bytes", maxChartExpandedBytes)
	}
	if err := gzf.Close(); err != nil {
		return nil, err
	}
	return files, nil
}

func LoadMetadata(root *os.Root, relativePath string) (*ChartMetadata, error) {
	if root == nil || relativePath == "." || !fs.ValidPath(relativePath) {
		return nil, fmt.Errorf("Helm metadata path is not local to its repository root")
	}
	f, err := root.Open(relativePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := readLimited(f, maxFileBytes, "Helm metadata")
	if err != nil {
		return nil, err
	}
	metadata := &ChartMetadata{}
	return metadata, yaml.Unmarshal(data, metadata)
}

func LoadFile(root *os.Root, relativePath string) (*model.File, error) {
	if root == nil || relativePath == "." || !fs.ValidPath(relativePath) {
		return nil, fmt.Errorf("Helm file path is not local to its repository root")
	}
	f, err := root.Open(relativePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := readLimited(f, maxFileBytes, "Helm file")
	if err != nil {
		return nil, err
	}
	filteredFile := filterFile(model.File{
		Name:     relativePath,
		Contents: string(data),
	})
	return &filteredFile, nil
}

func requireHTTPSuccess(response *http.Response) error {
	if response == nil {
		return fmt.Errorf("catalog HTTP response is missing")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("catalog HTTP request returned %s", response.Status)
	}
	return nil
}

func readLimited(reader io.Reader, limit int64, description string) ([]byte, error) {
	data, err := ioutil.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", description, limit)
	}
	return data, nil
}
