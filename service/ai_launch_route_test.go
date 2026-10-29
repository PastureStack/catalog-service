package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PastureStack/catalog-service/internal/aimetadata"
	"github.com/gorilla/mux"
)

func TestLaunchPrivateRoutesAreActuallyWiredWithoutLegacyFallback(t *testing.T) {
	previousManager, previousDB := m, db
	defer func() { m, db = previousManager, previousDB }()
	router := NewRouter(nil, nil)
	for _, route := range []struct{ method, path, name string }{
		{"GET", aimetadata.LaunchOptionsPath, "GetAILaunchOptions"}, {"GET", aimetadata.LaunchMaterialPath, "GetAILaunchMaterial"}, {"POST", aimetadata.LaunchValidatePath, "ValidateAILaunch"},
	} {
		body := ""
		target := route.path
		if route.method == "GET" {
			target += "?templateId=ctm-" + strings.Repeat("a", 64) + "&versionId=cvm-" + strings.Repeat("b", 64)
		} else {
			body = `{"templateId":"ctm-` + strings.Repeat("a", 64) + `","versionId":"cvm-` + strings.Repeat("b", 64) + `","digest":"` + strings.Repeat("d", 64) + `"}`
		}
		r := httptest.NewRequest(route.method, target, strings.NewReader(body))
		var match mux.RouteMatch
		if !router.Match(r, &match) || match.Route.GetName() != route.name {
			t.Fatal("private owner route not wired")
		}
		r.RemoteAddr = "10.0.0.1:1234"
		r.Header.Set(aimetadata.ProjectHeader, "1a5")
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("raw handler fallback or forwarded caller allowed")
		}
		r = httptest.NewRequest(route.method, target, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set(aimetadata.ProjectHeader, "1a5")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatal("missing source did not fail closed at actual owner route")
		}
	}
}
