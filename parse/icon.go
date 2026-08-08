package parse

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"path"
	"strings"

	"github.com/PastureStack/catalog-service/outbound"
)

const maxIconBytes = 5 << 20

func ParseIcon(client *outbound.Client, baseURL, iconURL string) (string, string, error) {
	iconURL = strings.TrimSpace(iconURL)
	if iconURL == "" {
		return "", "", nil
	}
	if client == nil {
		return "", "", fmt.Errorf("catalog icon HTTP client is not configured")
	}
	var (
		respURL *url.URL
		resp    io.ReadCloser
	)
	if baseURL == "" {
		response, err := client.Get(iconURL)
		if err != nil {
			return "", "", err
		}
		respURL = response.Request.URL
		resp = response.Body
		if response.StatusCode < 200 || response.StatusCode > 299 {
			response.Body.Close()
			return "", "", fmt.Errorf("catalog icon request returned %s", response.Status)
		}
	} else {
		response, err := client.ResolveAndGet(baseURL, iconURL)
		if err != nil {
			return "", "", err
		}
		respURL = response.Request.URL
		resp = response.Body
		if response.StatusCode < 200 || response.StatusCode > 299 {
			response.Body.Close()
			return "", "", fmt.Errorf("catalog icon request returned %s", response.Status)
		}
	}
	defer resp.Close()
	body, err := ioutil.ReadAll(io.LimitReader(resp, maxIconBytes+1))
	if err != nil {
		return "", "", err
	}
	if len(body) > maxIconBytes {
		return "", "", fmt.Errorf("catalog icon exceeds %d bytes", maxIconBytes)
	}
	iconFilename := path.Base(respURL.Path)
	if iconFilename == "." || iconFilename == "/" || iconFilename == "" {
		iconFilename = "catalog-icon"
	}
	iconData := base64.StdEncoding.EncodeToString(body)

	return iconData, iconFilename, nil
}
