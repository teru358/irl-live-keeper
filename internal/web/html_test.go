package web

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/sources"
)

func TestHTMLValidationAPI(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	h := apiHandler(b, nil)
	for _, c := range []struct {
		path, body string
		status     int
	}{
		{"/api/sources", `{"name":"unsafe","body":"<script></script>"}`, 422},
		{"/api/sources/preview", `{"source":{"body":"<script></script>"}}`, 200},
	} {
		req := httptest.NewRequest("POST", c.path, strings.NewReader(c.body))
		req.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != c.status || !strings.Contains(w.Body.String(), "template.html_element") {
			t.Fatalf("%s: %d %s", c.path, w.Code, w.Body.String())
		}
		if c.status == 200 && !strings.Contains(w.Body.String(), "template_error") {
			t.Fatal(w.Body.String())
		}
	}
}
func TestHTMLPolicyAssets(t *testing.T) {
	for _, path := range []string{"/assets/app/html_rules.json", "/source/assets/html_rules.json"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		if strings.HasPrefix(path, "/assets/") {
			serveAsset(w, r)
		} else {
			serveSource(w, r, nil, nil, "ja")
		}
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), sources.HTMLRules) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
