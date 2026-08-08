package outbound

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOriginPolicyRequiresExactOrigin(t *testing.T) {
	policy, err := NewOriginPolicy("https://catalog.example:8443")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		url     string
		allowed bool
	}{
		{"https://catalog.example:8443/index.yaml", true},
		{"https://catalog.example:8443/charts/app.tgz?download=1", true},
		{"https://catalog.example/index.yaml", false},
		{"http://catalog.example:8443/index.yaml", false},
		{"https://catalog.example.evil:8443/index.yaml", false},
		{"https://catalog.example:8443@evil.example/index.yaml", false},
		{"file:///etc/passwd", false},
		{"https://catalog.example:8443/index.yaml\nX-Test: injected", false},
	} {
		if got := policy.IsValidRedirectURL(test.url); got != test.allowed {
			t.Errorf("authorization for %q = %v, want %v", test.url, got, test.allowed)
		}
	}
}

func TestOriginPolicyRejectsNonOriginEntries(t *testing.T) {
	for _, origin := range []string{
		"https://catalog.example/path",
		"https://catalog.example?query=1",
		"https://user:pass@catalog.example",
		"http://catalog.example",
		"file:///tmp/catalog",
	} {
		if _, err := NewOriginPolicy(origin); err == nil {
			t.Errorf("expected origin %q to be rejected", origin)
		}
	}
}

func TestOriginPolicyAllowsPlainHTTPOnlyForLoopbackTests(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:8080", "http://[::1]:8080", "http://localhost:8080"} {
		if _, err := NewOriginPolicy(origin); err != nil {
			t.Errorf("loopback origin %q was rejected: %v", origin, err)
		}
	}
}

func TestClientBlocksUnauthorizedRequestAndRedirect(t *testing.T) {
	var attackerRequests atomic.Int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	trusted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, attacker.URL+"/target", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer trusted.Close()

	policy, err := NewOriginPolicy(trusted.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(policy)

	response, err := client.Get(trusted.URL + "/index.yaml")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	if _, err := client.Get(attacker.URL + "/direct"); err == nil {
		t.Fatal("unauthorized direct request was accepted")
	}
	if _, err := client.Get(trusted.URL + "/redirect"); err == nil {
		t.Fatal("redirect to an unauthorized origin was accepted")
	}
	if got := attackerRequests.Load(); got != 0 {
		t.Fatalf("unauthorized server received %d requests", got)
	}
}

func TestResolveURLAuthorizesResolvedDestination(t *testing.T) {
	policy, err := NewOriginPolicy("https://catalog.example")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := policy.ResolveURL("https://catalog.example/index.yaml", "charts/app.tgz")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.String() != "https://catalog.example/charts/app.tgz" {
		t.Fatalf("resolved URL = %q", resolved.String())
	}
	if _, err := policy.ResolveURL("https://catalog.example/index.yaml", "//attacker.example/app.tgz"); err == nil {
		t.Fatal("scheme-relative unauthorized destination was accepted")
	}
}

func TestSourcePolicyRestrictsLocalRootsAndProtocols(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	sibling := filepath.Join(root, "allowed-elsewhere")
	for _, directory := range []string{allowed, sibling} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	policy, err := NewSourcePolicy([]string{"https://github.com"}, []string{allowed})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := policy.AuthorizeGitSource(allowed)
	if err != nil || !approved.IsLocal() {
		t.Fatalf("authorized local source rejected: %#v, %v", approved, err)
	}
	if _, err := policy.AuthorizeGitSource(sibling); err == nil {
		t.Fatal("sibling path outside the authorized root was accepted")
	}
	outside := t.TempDir()
	escape := filepath.Join(allowed, "escape")
	if err := os.Symlink(outside, escape); err == nil {
		if _, err := policy.AuthorizeGitSource(escape); err == nil {
			t.Fatal("symlink escaping the authorized root was accepted")
		}
	}
	for _, source := range []string{
		"../relative",
		"file:///tmp/catalog",
		"ssh://github.com/example/catalog.git",
		"git@github.com:example/catalog.git",
		"ext::sh -c touch /tmp/pwned",
		"https://attacker.example/catalog.git",
	} {
		if _, err := policy.AuthorizeGitSource(source); err == nil {
			t.Errorf("unsafe source %q was accepted", source)
		}
	}
	remote, err := policy.AuthorizeGitSource("https://github.com/PastureStack/catalog-templates.git")
	if err != nil || remote.IsLocal() || !strings.HasPrefix(remote.String(), "https://github.com/") {
		t.Fatalf("authorized remote source rejected: %#v, %v", remote, err)
	}
}

func TestEnvironmentLocalRootIsLimitedToTemporaryDirectory(t *testing.T) {
	t.Setenv(AllowedLocalRootsEnvironment, os.TempDir())
	if _, err := FromEnvironment(); err != nil {
		t.Fatalf("platform temporary root was rejected: %v", err)
	}

	t.Setenv(AllowedLocalRootsEnvironment, filepath.Dir(os.TempDir()))
	if _, err := FromEnvironment(); err == nil {
		t.Fatal("non-temporary local root was accepted")
	}
}
