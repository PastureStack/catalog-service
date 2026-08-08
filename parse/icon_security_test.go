package parse

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/PastureStack/catalog-service/outbound"
)

func TestParseIconUsesPolicyCheckedRelativeURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("icon"))
	}))
	defer server.Close()
	policy, err := outbound.NewOriginPolicy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, filename, err := ParseIcon(outbound.NewClient(policy), server.URL+"/index.yaml", "assets/icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "icon.svg" || data != base64.StdEncoding.EncodeToString([]byte("icon")) {
		t.Fatalf("icon result = %q, %q", filename, data)
	}
}

func TestParseIconBlocksUnauthorizedOrigin(t *testing.T) {
	trusted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer trusted.Close()
	var requests atomic.Int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer attacker.Close()
	policy, err := outbound.NewOriginPolicy(trusted.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseIcon(outbound.NewClient(policy), trusted.URL+"/index.yaml", attacker.URL+"/icon.svg"); err == nil {
		t.Fatal("unauthorized icon origin was accepted")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("unauthorized icon server received %d requests", got)
	}
}
