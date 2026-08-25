package catalogapi

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	client "github.com/PastureStack/catalog-service/internal/catalogclient"
)

const (
	selfLink   = "self"
	latestLink = "latest"
)

type URLBuilder interface {
	Current() string
	Collection(resourceType string) string
	ReferenceByIDLink(resourceType, id string) string
	Version(version string) string
}

type urlBuilder struct {
	schemas    *client.Schemas
	baseURL    string
	requestURL string
	apiVersion string
}

func firstHeaderValue(value string) string {
	return strings.TrimSpace(strings.Split(value, ",")[0])
}

func validHost(raw string) (string, error) {
	raw = firstHeaderValue(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\t/@?#") {
		return "", fmt.Errorf("invalid API response host")
	}
	parsed, err := url.Parse("http://" + raw)
	if err != nil || parsed.Host != raw || parsed.Hostname() == "" || parsed.User != nil {
		return "", fmt.Errorf("invalid API response host")
	}
	return raw, nil
}

func responseBase(request *http.Request) (string, error) {
	if override := firstHeaderValue(request.Header.Get("X-API-request-url")); override != "" {
		parsed, err := url.Parse(override)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return "", fmt.Errorf("invalid X-API-request-url")
		}
		return parsed.Scheme + "://" + parsed.Host, nil
	}

	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.ToLower(firstHeaderValue(request.Header.Get("X-Forwarded-Proto"))); forwarded != "" {
		if forwarded != "http" && forwarded != "https" {
			return "", fmt.Errorf("invalid X-Forwarded-Proto")
		}
		scheme = forwarded
	}
	host := request.Host
	if forwarded := firstHeaderValue(request.Header.Get("X-Forwarded-Host")); forwarded != "" {
		host = forwarded
	}
	var err error
	host, err = validHost(host)
	if err != nil {
		return "", err
	}
	if forwardedPort := firstHeaderValue(request.Header.Get("X-Forwarded-Port")); forwardedPort != "" && forwardedPort != "80" && forwardedPort != "443" {
		port, err := strconv.Atoi(forwardedPort)
		if err != nil || port < 1 || port > 65535 {
			return "", fmt.Errorf("invalid X-Forwarded-Port")
		}
		hostname := host
		if parsedHost, _, splitErr := net.SplitHostPort(host); splitErr == nil {
			hostname = parsedHost
		}
		host = net.JoinHostPort(strings.Trim(hostname, "[]"), forwardedPort)
	}
	return scheme + "://" + host, nil
}

func newURLBuilder(request *http.Request, schemas *client.Schemas) (URLBuilder, error) {
	base, err := responseBase(request)
	if err != nil {
		return nil, err
	}
	version := ""
	parts := strings.Split(strings.TrimPrefix(request.URL.Path, "/"), "/")
	if len(parts) > 0 {
		version = parts[0]
	}
	return &urlBuilder{
		schemas:    schemas,
		baseURL:    base,
		requestURL: base + request.URL.Path,
		apiVersion: version,
	}, nil
}

func (builder *urlBuilder) Current() string { return builder.requestURL }

func (builder *urlBuilder) Collection(resourceType string) string {
	return builder.construct(builder.plural(resourceType))
}

func (builder *urlBuilder) ReferenceByIDLink(resourceType, id string) string {
	return builder.construct(builder.plural(resourceType), id)
}

func (builder *urlBuilder) Version(version string) string {
	return builder.baseURL + "/" + version
}

func (builder *urlBuilder) construct(parts ...string) string {
	result := builder.baseURL + "/" + builder.apiVersion
	for _, part := range parts {
		if part == "" {
			return ""
		}
		result += "/" + part
	}
	return result
}

func (builder *urlBuilder) plural(resourceType string) string {
	schema := builder.schemas.Schema(resourceType)
	if schema.PluralName != "" {
		return strings.ToLower(schema.PluralName)
	}
	return strings.ToLower(resourceType)
}
