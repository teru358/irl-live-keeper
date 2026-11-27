package web

import (
	"context"
	"encoding/json"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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
	}{{"/api/sources", `{"name":"a","template":"{obs.streaming}"}`, 201, `"ok":true`}, {"/api/sources/preview", `{"names":["a"],"simulate":{"mode":"OK"}}`, 200, `"a":"true"`}, {"/api/sources/preview", `{"source":{"template":"{obs.streaming}","css":".irl-text{color:red}"},"simulate":{"mode":"offline"}}`, 200, `"text":"false"`}, {"/api/sources/preview", `{"source":{"template":"{unknown}"}}`, 200, `"template_error":`}, {"/api/sources/preview", `{"names":[],"source":{}}`, 400, `"error":`}} {
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

func TestPreviewErrorContracts(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	h := apiHandler(b, nil)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/sources/preview", `{"source":{"template":"{if "}}`, 200},
		{"POST", "/api/sources/preview", `{"source":{"template":"{unknown}"}}`, 200},
		{"POST", "/api/sources/preview", `{`, 400},
		{"POST", "/api/sources/preview", `{"source":{"style":{"color":"bad"}}}`, 422},
		{"POST", "/api/sources/preview", `{"source":{},"simulate":{"mode":"bad"}}`, 422},
		{"POST", "/api/sources/preview", `{"names":["missing"]}`, 404},
		{"POST", "/api/sources", `{"name":"a","template":"ok"}`, 201},
		{"POST", "/api/sources", `{"name":"b","template":"{unknown}"}`, 422},
		{"PUT", "/api/sources/a", `{"name":"a","template":"{if "}`, 422},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body.String())
		}
		if tc.status == 200 {
			var data map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			if len(data) != 1 || data["template_error"] == nil {
				t.Fatal("unexpected preview fields", w.Body.String())
			}
			var fields map[string]any
			_ = json.Unmarshal(data["template_error"], &fields)
			if len(fields) != 4 || fields["code"] == nil || fields["error"] == nil || fields["line"] == nil || fields["column"] == nil {
				t.Fatal(fields)
			}
		}
	}
}

type simulationLog struct {
	mu     sync.Mutex
	lines  []string
	events *Events
}

func (l *simulationLog) Enabled(context.Context, slog.Level) bool { return true }
func (l *simulationLog) Handle(_ context.Context, r slog.Record) error {
	if l.events != nil {
		if !l.events.mu.TryLock() {
			panic("logging while events lock held")
		}
		l.events.mu.Unlock()
		if !l.events.updateMu.TryLock() {
			panic("logging while update lock held")
		}
		l.events.updateMu.Unlock()
	}
	line := r.Message
	r.Attrs(func(a slog.Attr) bool { line += " " + a.Key + "=" + a.Value.String(); return true })
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, line)
	return nil
}
func (l *simulationLog) WithAttrs([]slog.Attr) slog.Handler          { return l }
func (l *simulationLog) WithGroup(string) slog.Handler               { return l }
func (l *simulationLog) Slog() *slog.Logger                          { return slog.New(l) }
func (l *simulationLog) Subscribe(int) (<-chan logging.Line, func()) { return nil, func() {} }
func TestSimulationOperationLogs(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	log := &simulationLog{}
	e := NewEvents(b, log, nil)
	defer e.Close()
	// This backend has no subscriptions; calls below are synchronous.
	log.events = e
	h := apiHandler(b, e)
	call := func(method, body string) {
		t.Helper()
		r := httptest.NewRequest(method, "/api/sources/test-obs", strings.NewReader(body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	call("PUT", `{"mode":"OK"}`)
	call("PUT", `{"mode":"OK"}`)
	call("PUT", `{"mode":"LOW"}`)
	call("DELETE", "")
	call("DELETE", "")
	call("PUT", `{"mode":"custom","values":{"obs.scene":"secret-value"}}`)
	call("PUT", `{"mode":"custom","values":{"obs.scene":"another-secret"}}`)
	e.mu.Lock()
	at := e.testOBS.Alive
	e.mu.Unlock()
	e.updateSources(at.Add(45 * time.Second))
	e.updateSources(at.Add(46 * time.Second))
	for _, kind := range []string{"stream", "obs_connected"} {
		call("PUT", `{"mode":"OK"}`)
		e.stopSimulationEvent(simStream{kind, true})
		e.stopSimulationEvent(simStream{kind, true})
	}
	call("PUT", `{"mode":"DOWN"}`)
	b.snap.OBS.Streaming = true
	e.updateSources(time.Now())
	log.mu.Lock()
	defer log.mu.Unlock()
	want := []string{"test obs enabled mode=OK", "test obs enabled mode=LOW", "test obs disabled", "test obs enabled mode=custom", "test obs stopped reason=timeout", "test obs enabled mode=OK", "test obs stopped reason=stream_started", "test obs enabled mode=OK", "test obs stopped reason=stream_started", "test obs enabled mode=DOWN", "test obs stopped reason=stream_started"}
	if !reflect.DeepEqual(log.lines, want) {
		t.Fatalf("logs=%q want=%q", log.lines, want)
	}
}
