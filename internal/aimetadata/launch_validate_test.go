package aimetadata

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func validationRequest(t *testing.T, digest string) *http.Request {
	t.Helper()
	raw, _ := json.Marshal(map[string]interface{}{"templateId": "ctm-" + strings.Repeat("a", 64), "versionId": "cvm-" + strings.Repeat("b", 64), "digest": digest, "safeAnswers": map[string]interface{}{"enabled": false}})
	r := httptest.NewRequest("POST", LaunchValidatePath, strings.NewReader(string(raw)))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set(ProjectHeader, "1a5")
	return r
}

func TestPrivateLaunchValidateRechecksDigestAndNeverAutoFillsHumanDefaults(t *testing.T) {
	digest := strings.Repeat("d", 64)
	f := &launchFixture{material: &LaunchMaterial{Digest: digest, Files: map[string]string{"rancher-compose.yml": ".catalog:\n  questions:\n  - variable: enabled\n    type: boolean\n    required: true\n  - variable: password\n    type: password\n    default: private-never-default\n"}}}
	w := httptest.NewRecorder()
	NewLaunchValidateHandler(f).ServeHTTP(w, validationRequest(t, digest))
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-never-default") || !strings.Contains(w.Body.String(), `"enabled":"false"`) || !strings.Contains(w.Body.String(), `"requiresHuman":true`) {
		t.Fatal("private validation contract changed")
	}
	w = httptest.NewRecorder()
	NewLaunchValidateHandler(f).ServeHTTP(w, validationRequest(t, strings.Repeat("a", 64)))
	if w.Code != 409 || strings.Contains(w.Body.String(), digest) {
		t.Fatal("stale source admitted or private digest echoed")
	}
	f.err = errors.New("private-driver-message")
	w = httptest.NewRecorder()
	NewLaunchValidateHandler(f).ServeHTTP(w, validationRequest(t, digest))
	if w.Code != 503 || strings.Contains(w.Body.String(), "private-driver") {
		t.Fatal("source outage misclassified or private error echoed")
	}
}

func TestPrivateLaunchValidateRejectsForwardedPeerOpenBodyAndOversizedInputs(t *testing.T) {
	f := &launchFixture{}
	for _, change := range []func(*http.Request){
		func(r *http.Request) { r.RemoteAddr = "10.0.0.1:1234"; r.Header.Set("X-Forwarded-For", "127.0.0.1") },
		func(r *http.Request) { r.Header.Add(ProjectHeader, "1a6") },
		func(r *http.Request) { r.URL.RawQuery = "url=http://remote" },
		func(r *http.Request) { r.Body = http.NoBody },
		func(r *http.Request) {
			r.Body = ioBody(`{"templateId":"ctm-` + strings.Repeat("a", 64) + `","versionId":"cvm-` + strings.Repeat("b", 64) + `","digest":"` + strings.Repeat("d", 64) + `","url":"http://remote"}`)
		},
		func(r *http.Request) { r.Body = ioBody(strings.Repeat("x", 256*1024+1)) },
		func(r *http.Request) {
			r.Body = ioBody(`{"templateId":"ctm-` + strings.Repeat("a", 64) + `","versionId":"cvm-` + strings.Repeat("b", 64) + `","digest":"` + strings.Repeat("d", 64) + `","referenceBindings":{"optional":null}}`)
		},
	} {
		r := validationRequest(t, strings.Repeat("d", 64))
		change(r)
		w := httptest.NewRecorder()
		NewLaunchValidateHandler(f).ServeHTTP(w, r)
		if w.Code < 400 || f.calls != 0 {
			t.Fatal("untrusted validation input reached owner source")
		}
	}
}

type stringBody struct{ *strings.Reader }

func (stringBody) Close() error      { return nil }
func ioBody(value string) stringBody { return stringBody{strings.NewReader(value)} }
