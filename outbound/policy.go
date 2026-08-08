package outbound

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// AllowedOriginsEnvironment is controlled by the service operator. Catalog
	// API requests and catalog metadata cannot add outbound destinations.
	AllowedOriginsEnvironment = "PASTURESTACK_CATALOG_ALLOWED_EXTERNAL_ORIGINS"
	// AllowedLocalRootsEnvironment enables local Git repositories for isolated
	// tests. Every configured entry must resolve to the platform temporary root.
	AllowedLocalRootsEnvironment = "PASTURESTACK_CATALOG_ALLOWED_LOCAL_ROOTS"
)

var printableURL = regexp.MustCompile(`^[\x21-\x7e]+$`)

// GitHubOrigins are the exact public GitHub origins required by reviewed
// catalogs, raw icons, release assets, and GitHub commit checks.
func GitHubOrigins() []string {
	return []string{
		"https://github.com",
		"https://api.github.com",
		"https://raw.githubusercontent.com",
		"https://objects.githubusercontent.com",
		"https://release-assets.githubusercontent.com",
		"https://codeload.github.com",
	}
}

// OriginPolicy authorizes outbound HTTP requests by exact scheme, hostname,
// and effective port. Paths and catalog documents cannot expand the policy.
type OriginPolicy struct {
	origins map[string]struct{}
}

// NewOriginPolicy constructs an exact-origin policy. Entries must not contain
// credentials, non-root paths, queries, or fragments.
func NewOriginPolicy(origins ...string) (*OriginPolicy, error) {
	policy := &OriginPolicy{origins: make(map[string]struct{}, len(origins))}
	for _, rawOrigin := range origins {
		parsed, err := parseHTTPURL(rawOrigin)
		if err != nil {
			return nil, fmt.Errorf("invalid authorized catalog origin %q: %w", rawOrigin, err)
		}
		if parsed.EscapedPath() != "" && parsed.EscapedPath() != "/" {
			return nil, fmt.Errorf("authorized catalog origin %q must not contain a path", rawOrigin)
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("authorized catalog origin %q must not contain a query or fragment", rawOrigin)
		}
		origin, err := canonicalOrigin(parsed)
		if err != nil {
			return nil, fmt.Errorf("invalid authorized catalog origin %q: %w", rawOrigin, err)
		}
		policy.origins[origin] = struct{}{}
	}
	return policy, nil
}

// AuthorizeURL parses a URL and verifies its exact origin.
func (policy *OriginPolicy) AuthorizeURL(rawURL string) (*url.URL, error) {
	if policy == nil {
		return nil, fmt.Errorf("catalog outbound origin policy is not configured")
	}
	parsed, err := parseHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}
	origin, err := canonicalOrigin(parsed)
	if err != nil {
		return nil, err
	}
	if _, allowed := policy.origins[origin]; !allowed {
		return nil, fmt.Errorf("catalog origin %q is not authorized by %s", origin, AllowedOriginsEnvironment)
	}
	return parsed, nil
}

// ResolveURL resolves a catalog-provided reference against an already
// authorized base URL, then authorizes the resulting destination independently.
func (policy *OriginPolicy) ResolveURL(baseURL, reference string) (*url.URL, error) {
	base, err := policy.AuthorizeURL(baseURL)
	if err != nil {
		return nil, err
	}
	reference = strings.TrimSpace(reference)
	if reference == "" || !printableURL.MatchString(reference) {
		return nil, fmt.Errorf("catalog URL reference must contain only printable ASCII without whitespace")
	}
	ref, err := url.Parse(reference)
	if err != nil {
		return nil, fmt.Errorf("parse catalog URL reference: %w", err)
	}
	resolved := base.ResolveReference(ref)
	return policy.AuthorizeURL(resolved.String())
}

// IsValidRedirectURL is checked immediately before every request and redirect.
// The explicit name is also recognized by static security analyzers.
func (policy *OriginPolicy) IsValidRedirectURL(rawURL string) bool {
	_, err := policy.AuthorizeURL(rawURL)
	return err == nil
}

// PolicyTransport rechecks the destination at the final HTTP transport boundary.
type PolicyTransport struct {
	Base   http.RoundTripper
	Policy *OriginPolicy
}

func (transport *PolicyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("catalog outbound HTTP request URL is missing")
	}
	if transport == nil || transport.Policy == nil || !transport.Policy.IsValidRedirectURL(request.URL.String()) {
		return nil, fmt.Errorf("catalog outbound HTTP request origin is not authorized")
	}
	base := transport.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(request)
}

// Client exposes only policy-checked outbound requests.
type Client struct {
	policy *OriginPolicy
	client *http.Client
}

func NewClient(policy *OriginPolicy) *Client {
	base := http.DefaultTransport
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		base = transport.Clone()
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &PolicyTransport{Base: base, Policy: policy},
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request == nil || request.URL == nil || policy == nil || !policy.IsValidRedirectURL(request.URL.String()) {
				return fmt.Errorf("catalog redirect origin is not authorized")
			}
			if len(via) >= 10 {
				return fmt.Errorf("catalog redirect limit exceeded")
			}
			return nil
		},
	}
	return &Client{policy: policy, client: client}
}

func (client *Client) Get(rawURL string) (*http.Response, error) {
	parsed, err := client.authorize(rawURL)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	return client.Do(request)
}

func (client *Client) ResolveAndGet(baseURL, reference string) (*http.Response, error) {
	if client == nil || client.policy == nil {
		return nil, fmt.Errorf("catalog outbound HTTP client is not configured")
	}
	parsed, err := client.policy.ResolveURL(baseURL, reference)
	if err != nil {
		return nil, err
	}
	return client.Get(parsed.String())
}

func (client *Client) Do(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("catalog outbound HTTP request URL is missing")
	}
	if _, err := client.authorize(request.URL.String()); err != nil {
		return nil, err
	}
	return client.client.Do(request)
}

func (client *Client) authorize(rawURL string) (*url.URL, error) {
	if client == nil || client.policy == nil || client.client == nil {
		return nil, fmt.Errorf("catalog outbound HTTP client is not configured")
	}
	if !client.policy.IsValidRedirectURL(rawURL) {
		return nil, fmt.Errorf("catalog outbound HTTP request origin is not authorized")
	}
	return client.policy.AuthorizeURL(rawURL)
}

// AuthorizedSource is a Git source that has passed the server-controlled
// origin or local-root policy. Its fields are intentionally private.
type AuthorizedSource struct {
	value string
	local bool
}

func (source AuthorizedSource) String() string { return source.value }
func (source AuthorizedSource) IsLocal() bool  { return source.local }

// SourcePolicy combines HTTP origin authorization with symlink-resolved local
// Git root authorization.
type SourcePolicy struct {
	Origins    *OriginPolicy
	localRoots []string
}

func FromEnvironment() (*SourcePolicy, error) {
	origins := GitHubOrigins()
	for _, value := range strings.Split(os.Getenv(AllowedOriginsEnvironment), ",") {
		if value = strings.TrimSpace(value); value != "" {
			origins = append(origins, value)
		}
	}
	localRoots, err := temporaryRootsFromEnvironment(os.Getenv(AllowedLocalRootsEnvironment))
	if err != nil {
		return nil, err
	}
	return NewSourcePolicy(origins, localRoots)
}

// temporaryRootsFromEnvironment deliberately does not perform a filesystem
// operation on an operator-provided path. Local repositories exist only for
// isolated tests, so the sole supported root is the process temporary root.
func temporaryRootsFromEnvironment(rawRoots string) ([]string, error) {
	temporaryRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return nil, fmt.Errorf("resolve platform temporary root: %w", err)
	}
	temporaryRoot = filepath.Clean(temporaryRoot)

	var roots []string
	for _, value := range filepath.SplitList(rawRoots) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return nil, fmt.Errorf("resolve configured local catalog root: %w", err)
		}
		if !samePath(filepath.Clean(absolute), temporaryRoot) {
			return nil, fmt.Errorf("%s may enable only the platform temporary root", AllowedLocalRootsEnvironment)
		}
		if len(roots) == 0 {
			roots = append(roots, temporaryRoot)
		}
	}
	return roots, nil
}

func NewSourcePolicy(origins, localRoots []string) (*SourcePolicy, error) {
	originPolicy, err := NewOriginPolicy(origins...)
	if err != nil {
		return nil, err
	}
	policy := &SourcePolicy{Origins: originPolicy}
	for _, root := range localRoots {
		canonical, err := canonicalExistingPath(root)
		if err != nil {
			return nil, fmt.Errorf("invalid authorized local catalog root %q: %w", root, err)
		}
		policy.localRoots = append(policy.localRoots, canonical)
	}
	return policy, nil
}

// AuthorizeGitSource rejects Git helper, SSH, scp-like, file-URL, relative,
// and unapproved local sources before Git is executed.
func (policy *SourcePolicy) AuthorizeGitSource(rawSource string) (AuthorizedSource, error) {
	if policy == nil || policy.Origins == nil {
		return AuthorizedSource{}, fmt.Errorf("catalog source policy is not configured")
	}
	rawSource = strings.TrimSpace(rawSource)
	if rawSource == "" {
		return AuthorizedSource{}, fmt.Errorf("catalog source is missing")
	}
	if filepath.IsAbs(rawSource) {
		for _, root := range policy.localRoots {
			canonical, err := authorizedLocalDirectory(root, rawSource)
			if err == nil {
				return AuthorizedSource{value: canonical, local: true}, nil
			}
		}
		return AuthorizedSource{}, fmt.Errorf("local catalog source is not authorized by %s", AllowedLocalRootsEnvironment)
	}
	parsed, err := policy.Origins.AuthorizeURL(rawSource)
	if err != nil {
		return AuthorizedSource{}, err
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return AuthorizedSource{}, fmt.Errorf("Git catalog source must not contain a query or fragment")
	}
	return AuthorizedSource{value: parsed.String(), local: false}, nil
}

func parseHTTPURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || !printableURL.MatchString(rawURL) {
		return nil, fmt.Errorf("URL must contain only printable ASCII characters without whitespace")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("URL must use HTTP or HTTPS")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("URL must contain a hostname")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("URL must not contain credentials or a fragment")
	}
	if parsed.Scheme == "http" && !isLoopbackHostname(parsed.Hostname()) {
		return nil, fmt.Errorf("plain HTTP is allowed only for loopback test origins")
	}
	if _, err := canonicalOrigin(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func isLoopbackHostname(hostname string) bool {
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))
	if hostname == "localhost" {
		return true
	}
	address := net.ParseIP(hostname)
	return address != nil && address.IsLoopback()
}

func canonicalOrigin(parsed *url.URL) (string, error) {
	if parsed == nil {
		return "", fmt.Errorf("URL is missing")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("URL must use HTTP or HTTPS")
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "" {
		return "", fmt.Errorf("URL must contain a hostname")
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", fmt.Errorf("URL contains an invalid port")
	}
	authority := hostname
	if net.ParseIP(hostname) != nil && strings.Contains(hostname, ":") {
		authority = "[" + hostname + "]"
	}
	return scheme + "://" + authority + ":" + port, nil
}

func canonicalExistingPath(rawPath string) (string, error) {
	absolute, err := filepath.Abs(rawPath)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}
	return filepath.Clean(canonical), nil
}

// authorizedLocalDirectory checks containment before any access to the
// catalog-provided path. os.Root then keeps all filesystem resolution beneath
// the already-authorized root, including when symlinks are present.
func authorizedLocalDirectory(root, rawPath string) (string, error) {
	absolute, err := filepath.Abs(rawPath)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	relative, err := filepath.Rel(root, absolute)
	if err != nil || (relative != "." && !filepath.IsLocal(relative)) {
		return "", fmt.Errorf("path is outside the authorized local root")
	}

	rooted, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer rooted.Close()
	info, err := rooted.Stat(relative)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}
	return filepath.Join(root, relative), nil
}

func samePath(left, right string) bool {
	if filepath.Separator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}
