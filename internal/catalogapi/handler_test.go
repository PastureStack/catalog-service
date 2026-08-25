package catalogapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	client "github.com/PastureStack/catalog-service/internal/catalogclient"
)

func testSchemas() *client.Schemas {
	schemas := &client.Schemas{}
	schemas.AddType("apiVersion", client.Resource{})
	schemas.AddType("schema", client.Schema{})
	return schemas
}

func TestVersionsHandlerWritesCompatibleJSONLinks(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://catalog.example/", nil)
	response := httptest.NewRecorder()
	VersionsHandler(testSchemas(), "v1-catalog").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if actual := response.Header().Get("Content-Type"); actual != "application/json" {
		t.Fatalf("Content-Type = %q", actual)
	}
	if actual := response.Header().Get("X-API-Schemas"); actual != "https://catalog.example//schemas" {
		t.Fatalf("X-API-Schemas = %q", actual)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	links, ok := body["links"].(map[string]interface{})
	if !ok || links["latest"] != "https://catalog.example/v1-catalog" {
		t.Fatalf("latest link = %#v", body["links"])
	}
	data, ok := body["data"].([]interface{})
	if !ok || len(data) != 1 {
		t.Fatalf("data = %#v", body["data"])
	}
}

func TestAPIHandlerRejectsInvalidForwardedScheme(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://catalog.example/v1-catalog", nil)
	request.Header.Set("X-Forwarded-Proto", "javascript")
	response := httptest.NewRecorder()
	called := false
	handler := APIHandler(testSchemas(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	handler.ServeHTTP(response, request)

	if called {
		t.Fatal("wrapped handler was called with an invalid forwarded scheme")
	}
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}
