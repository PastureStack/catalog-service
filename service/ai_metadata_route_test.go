package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PastureStack/catalog-service/internal/aimetadata"
	"github.com/gorilla/mux"
)

func TestAIMetadataRouteIsPrivateAndLeavesLegacyReadRoutesIntact(t *testing.T) {
	previousManager, previousDB := m, db
	defer func() { m, db = previousManager, previousDB }()
	router := NewRouter(nil, nil)
	for path, expected := range map[string]string{
		aimetadata.Path:                        "GetAIMetadata",
		"/v1-catalog/templates":                "GetTemplates",
		"/v1-catalog/templates/catalog:folder": "GetTemplate",
		"/v1-catalog/catalogs":                 "GetCatalogs",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		var match mux.RouteMatch
		if !router.Match(request, &match) || match.Route.GetName() != expected {
			t.Fatalf("route changed: %s", path)
		}
	}
	request := httptest.NewRequest(http.MethodGet, aimetadata.Path, nil)
	request.Header.Set(aimetadata.ProjectHeader, "1a5")
	request.Header.Set("X-Forwarded-For", "127.0.0.1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("external metadata caller status=%d", response.Code)
	}
}
