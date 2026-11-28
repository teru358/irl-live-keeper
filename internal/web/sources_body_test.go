package web

import (
	"encoding/json"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourcesBodyAPI(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	b.snap.Cameras = []core.CameraStatus{{ID: "cam1"}}
	h := apiHandler(b, nil)
	call := func(method, path, body string, want int) string {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	quote := func(s string) string { raw, _ := json.Marshal(s); return string(raw) }
	// Round trip: what was saved comes back through the API and from the file.
	text := "<p>日本語 \"quoted\" {cam1.kbps}</p>\n<b>x</b>"
	call("POST", "/api/sources", `{"name":"demo","title":"表示","body":`+quote(text)+`}`, 201)
	var got struct {
		Name, Title, Body string
		ObsWidth          int `json:"obs_width"`
	}
	one := call("GET", "/api/sources/demo", "", 200)
	if err := json.Unmarshal([]byte(one), &got); err != nil || got.Body != text || got.Title != "表示" || got.ObsWidth != 800 {
		t.Fatal(got, err)
	}
	if strings.Contains(one, `"template"`) || strings.Contains(one, `"css"`) || strings.Contains(one, `"style"`) {
		t.Fatal(one)
	}
	raw, _ := os.ReadFile(b.store.Path)
	for _, forbidden := range []string{"template:", "css:", "style:"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal(forbidden, string(raw))
		}
	}
	// Old fields and unknown fields are rejected.
	for _, extra := range []string{`"template":"x"`, `"css":"a{}"`, `"style":{}`, `"nope":1`} {
		call("POST", "/api/sources", `{"name":"old","body":"x",`+extra+`}`, 400)
		call("PUT", "/api/sources/demo", `{"name":"demo","body":"x",`+extra+`}`, 400)
	}
	// 64 KiB: exactly 65536 bytes is accepted, 65537 is not.
	call("PUT", "/api/sources/demo", `{"name":"demo","body":"`+strings.Repeat("x", 65536)+`"}`, 200)
	big := call("PUT", "/api/sources/demo", `{"name":"demo","body":"`+strings.Repeat("x", 65537)+`"}`, 422)
	if !strings.Contains(big, "sources.body_too_large") && !strings.Contains(big, "64 KiB") {
		t.Fatal(big)
	}
	// Reserved names.
	for _, name := range []string{"files", "assets"} {
		reserved := call("POST", "/api/sources", `{"name":"`+name+`","body":"x"}`, 422)
		if !strings.Contains(reserved, name) || !strings.Contains(reserved, "\"error\"") {
			t.Fatal(reserved)
		}
	}
	// A broken body is refused on save, and a stored one is reported as needing repair.
	call("PUT", "/api/sources/demo", `{"name":"demo","body":"{unknown}"}`, 422)
	if err := os.WriteFile(b.store.Path, []byte("- name: bad\n  body: '{unknown}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if list := call("GET", "/api/sources", "", 200); !strings.Contains(list, `"broken":{"bad":`) {
		t.Fatal(list)
	}
}

func TestSourcePreviewRendersBody(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	b.snap.Cameras = []core.CameraStatus{{ID: "cam1"}}
	h := apiHandler(b, nil)
	r := httptest.NewRequest("POST", "/api/sources/preview", strings.NewReader(`{"source":{"body":"<b>{cam1.state}</b>"},"simulate":{"line":"LOW"}}`))
	r.Header.Set(headerName, "1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out struct{ Text, CSS string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Text != "<b>LOW</b>" || out.CSS != sources.BaseStylesheet {
		t.Fatal(w.Code, w.Body.String())
	}
}
