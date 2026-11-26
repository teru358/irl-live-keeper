package web

import (
	"github.com/teru358/irl-live-keeper/internal/sources"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPreviewSimulation(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	h := apiHandler(b, nil)
	for _, tc := range []struct {
		path, body string
		status     int
		want       string
	}{{"/api/sources", `{"name":"a","template":"{obs.streaming}"}`, 201, `"ok":true`}, {"/api/sources/preview", `{"names":["a"],"simulate":{"mode":"OK"}}`, 200, `"a":"true"`}, {"/api/sources/preview", `{"source":{"template":"{obs.streaming}","css":".irl-text{color:red}"},"simulate":{"mode":"offline"}}`, 200, `"text":"false"`}, {"/api/sources/preview", `{"source":{"template":"{unknown}"}}`, 422, `"column":`}, {"/api/sources/preview", `{"names":[],"source":{}}`, 400, `"error":`}} {
		r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
}
func TestTestOBSExpiry(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	e := NewEvents(b, nil, nil)
	defer e.Close()
	h := apiHandler(b, e)
	before := b.Snapshot()
	defer func() {
		if !reflect.DeepEqual(before, b.Snapshot()) {
			t.Error("snapshot changed")
		}
	}()
	call := func(method, path, body string, header bool, want int) {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if header {
			r.Header.Set(headerName, "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	call("PUT", "/api/sources/test-obs", `{"mode":"OK"}`, false, 403)
	call("PUT", "/api/sources/test-obs", `{"mode":"OK"}`, true, 200)
	e.mu.Lock()
	at := e.testOBS.Alive
	e.mu.Unlock()
	if e.simulation(at.Add(44*time.Second)).Mode != "OK" {
		t.Fatal("expired early")
	}
	if e.simulation(at.Add(45*time.Second)).Mode != "" {
		t.Fatal("not expired")
	}
	call("PUT", "/api/sources/test-obs", `{"mode":"LOW"}`, true, 200)
	call("POST", "/api/sources/test-obs/alive", ``, true, 200)
	call("DELETE", "/api/sources/test-obs", ``, true, 200)
	if e.simulation(time.Now()).Mode != "" {
		t.Fatal("not disabled")
	}
	for _, kind := range []string{"stream", "obs_connected"} {
		call("PUT", "/api/sources/test-obs", `{"mode":"OK"}`, true, 200)
		e.stopSimulationEvent(simStream{kind, true})
		if e.simulation(time.Now()).Mode != "" {
			t.Fatal(kind)
		}
	}
}

type simStream struct {
	kind      string
	Streaming bool `json:"streaming"`
}

func (s simStream) Kind() string { return s.kind }

func TestTestOBSStreamingRejected(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	b.snap.OBS.Streaming = true
	e := NewEvents(b, nil, nil)
	defer e.Close()
	r := httptest.NewRequest("PUT", "/api/sources/test-obs", strings.NewReader(`{"mode":"OK"}`))
	r.Header.Set(headerName, "1")
	w := httptest.NewRecorder()
	apiHandler(b, e).ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e.simulation(time.Now()).Mode != "" {
		t.Fatal("enabled in streaming")
	}
}
func TestPublicSimulationOptIn(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	api := apiHandler(b, nil)
	r := httptest.NewRequest("POST", "/api/sources", strings.NewReader(`{"name":"a","template":"{obs.streaming}"}`))
	r.Header.Set(headerName, "1")
	api.ServeHTTP(httptest.NewRecorder(), r)
	e := NewEvents(b, nil, nil)
	defer e.Close()
	page := func(want string) {
		t.Helper()
		w := httptest.NewRecorder()
		serveSource(w, httptest.NewRequest("GET", "/source/a", nil), b, e, "ja")
		if !strings.Contains(w.Body.String(), `<span class="irl-text">`+want+`</span>`) {
			t.Fatal(w.Body.String())
		}
	}
	page("false")
	r = httptest.NewRequest("POST", "/api/sources/preview", strings.NewReader(`{"names":["a"],"simulate":{"mode":"OK"}}`))
	r.Header.Set(headerName, "1")
	apiHandler(b, e).ServeHTTP(httptest.NewRecorder(), r)
	page("false")
	r = httptest.NewRequest("PUT", "/api/sources/test-obs", strings.NewReader(`{"mode":"OK"}`))
	r.Header.Set(headerName, "1")
	apiHandler(b, e).ServeHTTP(httptest.NewRecorder(), r)
	page("true")
	if e.texts()["a"] != "true" {
		t.Fatal(e.texts())
	}
	r = httptest.NewRequest("DELETE", "/api/sources/test-obs", nil)
	r.Header.Set(headerName, "1")
	apiHandler(b, e).ServeHTTP(httptest.NewRecorder(), r)
	page("false")
	if e.texts()["a"] != "false" {
		t.Fatal(e.texts())
	}
	fresh := NewEvents(b, nil, nil)
	defer fresh.Close()
	if fresh.simulation(time.Now()).Mode != "" {
		t.Fatal("new generation")
	}
}
