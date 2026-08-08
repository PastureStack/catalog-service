package service

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteCatalogReadmeUsesPlainTextAndNosniff(t *testing.T) {
	response := httptest.NewRecorder()
	readme := `<script>window.catalogCompromised = true</script>`
	writeCatalogReadme(response, readme)

	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if body := response.Body.String(); !strings.Contains(body, "<script>") {
		t.Fatalf("README content was unexpectedly rewritten: %q", body)
	}
}
