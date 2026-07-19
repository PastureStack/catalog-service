package service

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogModelFromRequestKeepsPinnedCommit(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	request := httptest.NewRequest("POST", "/v1-catalog/catalogs", strings.NewReader(`{
		"name":"pasturestack",
		"url":"https://github.com/PastureStack/catalog-templates.git",
		"branch":"main",
		"kind":"native",
		"pinnedCommit":"`+commit+`"
	}`))

	catalog, err := catalogModelFromRequest(request, "global")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.PinnedCommit != commit {
		t.Fatalf("pinned commit %s does not match request %s", catalog.PinnedCommit, commit)
	}
}
