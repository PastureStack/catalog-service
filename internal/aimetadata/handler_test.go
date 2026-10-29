package aimetadata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf16"
)

type sourceFunc func(context.Context, string, Query, int) ([]Template, error)

func (f sourceFunc) Read(c context.Context, p string, q Query, n int) ([]Template, error) {
	return f(c, p, q, n)
}
func request(query string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, Path+query, nil)
	r.RemoteAddr = "127.0.0.1:12345"
	r.Header.Set(ProjectHeader, "1a5")
	return r
}
func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func template(id int) Template {
	digest := sha256.Sum256([]byte(fmt.Sprintf("fixture-%d", id)))
	return Template{ID: "ctm-" + hex.EncodeToString(digest[:]), rowID: int64(id), Name: "same-name", CatalogScope: "global"}
}

func TestMetadataCallerUsesRealRemoteAddressNotForwardedOrRoleHeaders(t *testing.T) {
	calls := 0
	h := NewHandler(sourceFunc(func(_ context.Context, _ string, _ Query, _ int) ([]Template, error) { calls++; return nil, nil }))
	for _, remote := range []string{"192.0.2.1:9000", "[2001:db8::1]:9000", "127.0.0.1", "malformed"} {
		r := request("")
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		r.Header.Set("X-Real-IP", "127.0.0.1")
		r.Header.Set("X-API-Roles", "admin")
		if w := serve(h, r); w.Code != 403 {
			t.Fatalf("remote %q status=%d", remote, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("untrusted caller reached source")
	}
	for _, remote := range []string{"127.0.0.1:1", "[::1]:1"} {
		r := request("")
		r.RemoteAddr = remote
		if w := serve(h, r); w.Code != 200 {
			t.Fatalf("loopback status=%d", w.Code)
		}
	}
	if calls != 2 {
		t.Fatal("loopback reads did not reach source")
	}
}

func TestMetadataRequiresEffectiveProjectHeaderAndRejectsClientProjectSelectors(t *testing.T) {
	h := NewHandler(sourceFunc(func(_ context.Context, p string, _ Query, _ int) ([]Template, error) {
		if p != "1a5" {
			t.Fatal("project changed")
		}
		return nil, nil
	}))
	for _, project := range []string{"", "global", "1a0", "1a5/other", "1a5,1a6"} {
		r := request("")
		r.Header.Set(ProjectHeader, project)
		if w := serve(h, r); w.Code != 403 {
			t.Fatalf("project %q status=%d", project, w.Code)
		}
	}
	r := request("")
	r.Header.Add(ProjectHeader, "1a6")
	if serve(h, r).Code != 403 {
		t.Fatal("duplicate header accepted")
	}
	if serve(h, request("?projectId=1a6")).Code != 422 {
		t.Fatal("query project accepted")
	}
}

func TestMetadataInputIsClosedAndReadOnlyBeforeAnySourceRead(t *testing.T) {
	calls := 0
	h := NewHandler(sourceFunc(func(context.Context, string, Query, int) ([]Template, error) { calls++; return nil, nil }))
	for _, query := range []string{"?url=https://other", "?action=refresh", "?query=a&query=b", "?marker=ctm-0", "?marker=../x", "?marker=ctm-9223372036854775808", "?templateId=ctm-1&query=", "?query=%ZZ", "?query=" + strings.Repeat("a", 257)} {
		if w := serve(h, request(query)); w.Code != 422 {
			t.Fatalf("query %q status=%d", query, w.Code)
		}
	}
	r := request("")
	r.Method = http.MethodPost
	if w := serve(h, r); w.Code != 405 || w.Header().Get("Allow") != "GET" {
		t.Fatal("write method accepted")
	}
	if calls != 0 {
		t.Fatal("invalid input reached source")
	}
}

func TestMetadataKeysetPagingPassesMarkerBeforeFixedLimitAndKeepsDuplicateNames(t *testing.T) {
	calls := 0
	h := NewHandler(sourceFunc(func(_ context.Context, p string, q Query, n int) ([]Template, error) {
		calls++
		if p != "1a5" || n != 51 {
			t.Fatal("unbounded or foreign read")
		}
		if calls == 2 && q.AfterID != 50 {
			t.Fatal("cursor reloaded first page")
		}
		rows := []Template{}
		for i := int(q.AfterID) + 1; i <= 70 && len(rows) < n; i++ {
			rows = append(rows, template(i))
		}
		return rows, nil
	}))
	first := serve(h, request(""))
	var page response
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if first.Code != 200 || len(page.Data) != 50 || page.NextMarker == nil {
		t.Fatal("first page not bounded")
	}
	if after, id, valid := parseCursor(*page.NextMarker, "1a5", ""); !valid || after != 50 || id != template(50).ID {
		t.Fatal("cursor did not bind private locator to logical identity")
	}
	second := serve(h, request("?marker="+*page.NextMarker))
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if second.Code != 200 || len(page.Data) != 20 || page.Data[0].ID != template(51).ID || page.NextMarker != nil {
		t.Fatal("later page skipped/reloaded")
	}
	if page.Data[0].Name != page.Data[1].Name || page.Data[0].ID == page.Data[1].ID {
		t.Fatal("same names were collapsed")
	}
}

func TestMetadataDetailUsesExactIdentityOrNotFoundWithoutLeakingSourceErrors(t *testing.T) {
	h := NewHandler(sourceFunc(func(_ context.Context, _ string, q Query, _ int) ([]Template, error) {
		if q.ExactID == template(12).ID {
			return []Template{template(12)}, nil
		}
		if q.ExactID == template(13).ID {
			return nil, nil
		}
		return nil, errors.New("PRIVATE_DATABASE_CREDENTIAL")
	}))
	if w := serve(h, request("?templateId="+template(12).ID)); w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"`+template(12).ID+`"`) {
		t.Fatal("wrong detail")
	}
	if serve(h, request("?templateId="+template(13).ID)).Code != 404 {
		t.Fatal("missing detail was not404")
	}
	w := serve(h, request("?templateId="+template(14).ID))
	if w.Code != 503 || strings.Contains(w.Body.String(), "PRIVATE_DATABASE_CREDENTIAL") {
		t.Fatal("source error leaked")
	}
}

func TestMetadataProjectionPreservesUTF16BoundaryAndMarksTruncation(t *testing.T) {
	h := NewHandler(sourceFunc(func(context.Context, string, Query, int) ([]Template, error) {
		r := template(1)
		r.Name = strings.Repeat("n", 513)
		r.Description = strings.Repeat("🌿", 513)
		return []Template{r}, nil
	}))
	w := serve(h, request(""))
	var page response
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(utf16.Encode([]rune(page.Data[0].Description))) != 1024 || len(page.Data[0].Name) != 512 {
		t.Fatal("metadata lost description or exceeded bounds")
	}
	if strings.Join(page.Data[0].TruncatedFields, ",") != "name,description" {
		t.Fatal("truncation was not explicit")
	}
	for _, forbidden := range []string{`"questions"`, `"files"`, `"readme"`, `"links"`, `"environment"`, `"path"`} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("unsafe field %s", forbidden)
		}
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private response is cacheable")
	}
}

func TestMetadataRejectsNonprogressingOrOversizedSourceBeforeResponse(t *testing.T) {
	for _, rows := range [][]Template{{template(1), template(1)}, {template(2), template(1)}, make([]Template, 52)} {
		h := NewHandler(sourceFunc(func(context.Context, string, Query, int) ([]Template, error) { return rows, nil }))
		if w := serve(h, request("")); w.Code != 503 {
			t.Fatal("invalid source page accepted")
		}
	}
	h := NewHandler(sourceFunc(func(context.Context, string, Query, int) ([]Template, error) { return []Template{template(1)}, nil }))
	if serve(h, request("?marker="+cursor("1a5", "", 1, template(1).ID))).Code != 503 {
		t.Fatal("old source row repeated")
	}
}
