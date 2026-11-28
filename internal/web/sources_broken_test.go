package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/sources"
)

const brokenSourcesYAML = "- name: broken\n  title: Old source\n  body: '{link.kbps}'\n- name: good\n  body: hello\n"

func TestBrokenSourcesAPI(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	if err := os.WriteFile(b.store.Path, []byte(brokenSourcesYAML), 0600); err != nil {
		t.Fatal(err)
	}
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
	check := func(wantBroken bool) {
		t.Helper()
		body := call("GET", "/api/sources?lang=ja", "", 200)
		var data struct {
			Sources []map[string]any
			Broken  map[string]struct {
				Code, Error  string
				Line, Column int
			}
			Names   []string
			Default sources.Source
		}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			t.Fatal(err)
		}
		if len(data.Sources) < 2 || len(data.Names) == 0 || data.Default.OBSWidth != 800 || (len(data.Broken) > 0) != wantBroken {
			t.Fatal(body)
		}
		for _, src := range data.Sources {
			if _, ok := src["broken"]; ok {
				t.Fatal("metadata inside source", body)
			}
		}
		if wantBroken {
			d := data.Broken["broken"]
			if d.Code != "template.unknown" || d.Error != "1 行 2 文字目: 知らない値の名前です" || d.Line != 1 || d.Column != 2 {
				t.Fatal(body)
			}
		}
	}
	check(true)
	call("POST", "/api/sources/preview", `{"names":["broken","good"]}`, 200)
	call("POST", "/api/sources", `{"name":"new","body":"new"}`, 201)
	call("PUT", "/api/sources/good", `{"name":"good","body":"updated"}`, 200)
	call("DELETE", "/api/sources/new", "", 200)
	raw, err := os.ReadFile(b.store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "{link.kbps}") || strings.Contains(string(raw), "broken:") {
		t.Fatal(string(raw))
	}
	check(true)
	before := string(raw)
	call("PUT", "/api/sources/broken", `{"name":"broken","body":"{link.kbps}"}`, 422)
	raw, _ = os.ReadFile(b.store.Path)
	if string(raw) != before {
		t.Fatal("failed PUT wrote file")
	}
	call("PUT", "/api/sources/broken", `{"name":"broken","body":"fixed"}`, 200)
	check(false)
	if err := os.WriteFile(b.store.Path, []byte(brokenSourcesYAML), 0600); err != nil {
		t.Fatal(err)
	}
	call("DELETE", "/api/sources/broken", "", 200)
	call("GET", "/api/sources/broken", "", 404)
	// Even a wholly unreadable file must keep the editor's definitions/defaults.
	if err := os.WriteFile(b.store.Path, []byte("["), 0600); err != nil {
		t.Fatal(err)
	}
	body := call("GET", "/api/sources", "", 200)
	for _, field := range []string{`"names":[`, `"default":`, `"broken":{}`, `"code":"sources.unreadable"`} {
		if !strings.Contains(body, field) {
			t.Fatal(body)
		}
	}
}

func TestBrokenSourcesPublicAndPreview(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	if err := os.WriteFile(b.store.Path, []byte(brokenSourcesYAML), 0600); err != nil {
		t.Fatal(err)
	}
	e := NewEvents(b, nil, nil)
	defer e.Close()
	h := apiHandler(b, e)
	// The page starts with an empty outer frame for both; the body arrives over SSE (S2-78).
	for _, tc := range []struct {
		path, want string
		code       int
	}{
		{"/source/broken", `<div class="irl-source irl-source-broken"></div>`, 200},
		{"/source/good", `<div class="irl-source irl-source-good"></div>`, 200},
		{"/source/broken/style.css", "", 404},
	} {
		w := httptest.NewRecorder()
		serveSource(w, httptest.NewRequest("GET", tc.path, nil), b, e, "en")
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/api/sources/preview", strings.NewReader(`{"names":["broken","good"],"simulate":{"line":"OK"}}`))
	r.Header.Set(headerName, "1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"sources":{"broken":"","good":"hello"}`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, name := range []string{"broken", "good"} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := httptest.NewRequest("GET", "/source/"+name+"/events", nil).WithContext(ctx)
		w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
		serveSource(w, r, b, e, "en")
		want := `data: ""`
		if name == "good" {
			want = `data: "hello"`
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
			t.Fatal(name, w.Code, w.Body.String())
		}
	}
	// A previously working value must be cleared, while the other source keeps running.
	e.mu.Lock()
	e.sourceTexts = map[string]string{"broken": "old", "good": "hello"}
	e.mu.Unlock()
	e.updateSources(time.Now())
	if e.texts()["broken"] != "" || e.texts()["good"] != "hello" {
		t.Fatal(e.texts())
	}
}
