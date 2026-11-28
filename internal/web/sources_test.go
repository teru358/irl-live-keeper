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

type sourcesBackend struct {
	fakeBackend
	store sources.Store
}

func (b *sourcesBackend) SourcesStore() *sources.Store { return &b.store }
func TestSourcesMethods(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	b.snap.Cameras = []core.CameraStatus{{ID: "cam1"}}
	h := apiHandler(b, nil)
	call := func(method, path, body string, header bool, want int) string {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if header {
			r.Header.Set(headerName, "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	body := `{"name":"demo","template":"{cam1.kbps}"}`
	call("POST", "/api/sources", body, false, 403)
	call("POST", "/api/sources", body, true, 201)
	call("POST", "/api/sources", body, true, 409)
	for _, old := range []string{"camera.kbps", "camera.cam1.kbps", "link.state", "link.kbps", "link.online", "link.protocol", "link.state_for", "srt.rtt", "srt.loss", "srt.mbps"} {
		got := call("POST", "/api/sources", `{"name":"removed","template":"{`+old+`}"}`, true, 422)
		if !strings.Contains(got, `"code":"template.unknown"`) {
			t.Fatal(got)
		}
	}
	call("GET", "/api/sources", "", false, 200)
	got := call("GET", "/api/sources/demo", "", false, 200)
	if !strings.Contains(got, `"obs_width":800`) {
		t.Fatal(got)
	}
	call("PUT", "/api/sources/demo", `{"name":"demo","css":"body {}"}`, true, 200)
	got = call("PUT", "/api/sources/demo", `{"name":"demo","template":"{unknown}"}`, true, 422)
	var e struct {
		Line, Column int
		Code         string
	}
	if json.Unmarshal([]byte(got), &e) != nil || e.Line != 1 || e.Column < 1 || e.Code != "template.unknown" {
		t.Fatal(got)
	}
	if err := os.WriteFile(b.store.Path, []byte("broken: ["), 0600); err != nil {
		t.Fatal(err)
	}
	got = call("GET", "/api/sources", "", false, 200)
	if !strings.Contains(got, "sources.unreadable") {
		t.Fatal(got)
	}
	call("POST", "/api/sources", body, true, 409)
	call("DELETE", "/api/sources/demo", "", true, 409)
	if err := os.WriteFile(b.store.Path, []byte("[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/sources", body, true, 201)
	call("DELETE", "/api/sources/demo", "", true, 200)
	call("GET", "/api/sources/demo", "", false, 404)
	call("PATCH", "/api/sources", "", true, 405)
}

func TestSourcesAdminAuthentication(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	h := New(b, testPass)
	for _, remote := range []string{"127.0.0.1:1234", "192.168.1.2:1234"} {
		for _, auth := range []bool{false, true} {
			r := httptest.NewRequest("GET", "http://localhost/api/sources", nil)
			r.RemoteAddr = remote
			if auth {
				r.SetBasicAuth("admin", testPass)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			want := 401
			if auth {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("remote=%s auth=%v: %d %s", remote, auth, w.Code, w.Body.String())
			}
		}
	}
}

func TestSourceNames(t *testing.T) {
	b := &sourcesBackend{}
	names := sourceNames(b)
	if len(names) == 0 {
		t.Fatal("empty definitions")
	}
	for _, name := range names {
		if strings.Contains(name, "password") || strings.Contains(name, "secret") {
			t.Fatal(name)
		}
	}
}
