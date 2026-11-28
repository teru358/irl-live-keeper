package web

import (
	"context"
	"encoding/json"
	"github.com/teru358/irl-live-keeper/internal/core"
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
	}{{"/api/sources", `{"name":"a","body":"{obs.streaming}"}`, 201, `"ok":true`}, {"/api/sources/preview", `{"names":["a"],"simulate":{"stream":"streaming"}}`, 200, `"a":"true"`}, {"/api/sources/preview", `{"source":{"body":"{obs.streaming}"},"simulate":{"stream":"offline"}}`, 200, `"text":"false"`}, {"/api/sources/preview", `{"source":{"body":"{unknown}"}}`, 200, `"template_error":`}, {"/api/sources/preview", `{"source":{"template":"x"}}`, 400, `"error":`}, {"/api/sources/preview", `{"source":{"body":"x","css":"a{}"}}`, 400, `"error":`}, {"/api/sources/preview", `{"source":{"body":"x","style":{}}}`, 400, `"error":`}, {"/api/sources/preview", `{"names":[],"source":{}}`, 400, `"error":`}} {
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
	call("PUT", "/api/sources/test-obs", `{"line":"OK"}`, false, 403)
	call("PUT", "/api/sources/test-obs", `{"line":"OK"}`, true, 200)
	e.mu.Lock()
	at := e.testOBS.Alive
	e.mu.Unlock()
	if e.simulation(at.Add(44*time.Second)).Line != "OK" {
		t.Fatal("expired early")
	}
	if !e.simulation(at.Add(45 * time.Second)).IsActual() {
		t.Fatal("not expired")
	}
	call("PUT", "/api/sources/test-obs", `{"line":"LOW"}`, true, 200)
	call("POST", "/api/sources/test-obs/alive", ``, true, 200)
	call("DELETE", "/api/sources/test-obs", ``, true, 200)
	if !e.simulation(time.Now()).IsActual() {
		t.Fatal("not disabled")
	}
	for _, kind := range []string{"stream", "obs_connected"} {
		call("PUT", "/api/sources/test-obs", `{"line":"OK","standby":"none","stream":"streaming","values":{"obs.fps":1}}`, true, 200)
		e.stopSimulationEvent(simStream{kind, true})
		if !e.simulation(time.Now()).IsActual() {
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
	r := httptest.NewRequest("PUT", "/api/sources/test-obs", strings.NewReader(`{"line":"OK"}`))
	r.Header.Set(headerName, "1")
	w := httptest.NewRecorder()
	apiHandler(b, e).ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !e.simulation(time.Now()).IsActual() {
		t.Fatal("enabled in streaming")
	}
}
func TestPublicSimulationOptIn(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	api := apiHandler(b, nil)
	r := httptest.NewRequest("POST", "/api/sources", strings.NewReader(`{"name":"a","body":"{obs.streaming}"}`))
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
	r = httptest.NewRequest("POST", "/api/sources/preview", strings.NewReader(`{"names":["a"],"simulate":{"stream":"streaming"}}`))
	r.Header.Set(headerName, "1")
	apiHandler(b, e).ServeHTTP(httptest.NewRecorder(), r)
	page("false")
	r = httptest.NewRequest("PUT", "/api/sources/test-obs", strings.NewReader(`{"stream":"streaming"}`))
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
	if !fresh.simulation(time.Now()).IsActual() {
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
		{"POST", "/api/sources/preview", `{"source":{"body":"{if "}}`, 200},
		{"POST", "/api/sources/preview", `{"source":{"body":"{unknown}"}}`, 200},
		{"POST", "/api/sources/preview", `{`, 400},
		{"POST", "/api/sources/preview", `{"source":{"name":"files"}}`, 422},
		{"POST", "/api/sources/preview", `{"source":{},"simulate":{"line":"bad"}}`, 422},
		{"POST", "/api/sources/preview", `{"source":{},"simulate":{"mode":"OK"}}`, 400},
		{"POST", "/api/sources/preview", `{"source":{},"simulate":{"standby":"standby"}}`, 422},
		{"POST", "/api/sources/preview", `{"source":{},"simulate":{"values":{"nope":1}}}`, 422},
		{"POST", "/api/sources/preview", `{"source":{},"simulate":{"values":[1]}}`, 400},
		{"POST", "/api/sources/preview", `{"names":["missing"]}`, 404},
		{"POST", "/api/sources", `{"name":"a","body":"ok"}`, 201},
		{"POST", "/api/sources", `{"name":"b","body":"{unknown}"}`, 422},
		{"PUT", "/api/sources/a", `{"name":"a","body":"{if "}`, 422},
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
func (l *simulationLog) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *simulationLog) WithGroup(string) slog.Handler      { return l }
func (l *simulationLog) Slog() *slog.Logger                 { return slog.New(l) }
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
	call("PUT", `{"line":"OK"}`)
	call("PUT", `{"line":"OK"}`)
	call("PUT", `{"line":"LOW","standby":"auto"}`)
	call("DELETE", "")
	call("DELETE", "")
	call("PUT", `{"values":{"obs.scene":"secret-value"}}`)
	call("PUT", `{"values":{"obs.scene":"another-secret"}}`)
	e.mu.Lock()
	at := e.testOBS.Alive
	e.mu.Unlock()
	e.updateSources(at.Add(45 * time.Second))
	e.updateSources(at.Add(46 * time.Second))
	for _, kind := range []string{"stream", "obs_connected"} {
		call("PUT", `{"stream":"streaming"}`)
		e.stopSimulationEvent(simStream{kind, true})
		e.stopSimulationEvent(simStream{kind, true})
	}
	call("PUT", `{"line":"DOWN"}`)
	b.snap.OBS.Streaming = true
	e.updateSources(time.Now())
	log.mu.Lock()
	defer log.mu.Unlock()
	want := []string{
		"test obs enabled line=OK standby=actual stream=actual free=false",
		"test obs enabled line=LOW standby=auto stream=actual free=false",
		"test obs disabled",
		"test obs enabled line=actual standby=actual stream=actual free=true",
		"test obs stopped reason=timeout",
		"test obs enabled line=actual standby=actual stream=streaming free=false",
		"test obs stopped reason=stream_started",
		"test obs enabled line=actual standby=actual stream=streaming free=false",
		"test obs stopped reason=stream_started",
		"test obs enabled line=DOWN standby=actual stream=actual free=false",
		"test obs stopped reason=stream_started",
	}
	if !reflect.DeepEqual(log.lines, want) {
		t.Fatalf("logs=%q want=%q", log.lines, want)
	}
}

func TestTestOBSRequestShape(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	e := NewEvents(b, nil, nil)
	defer e.Close()
	h := apiHandler(b, e)
	call := func(method, path, body string) int {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	enabled := func() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.testOBS.Enabled }
	// すべて実際で値もなければ 422（空の本文、空の値、実際の明示）。
	for _, body := range []string{`{}`, `{"values":{}}`, `{"line":"actual","standby":"actual","stream":"actual"}`, `{"line":"actual","values":{}}`} {
		if got := call("PUT", "/api/sources/test-obs", body); got != 422 || enabled() {
			t.Errorf("%s: %d enabled=%v", body, got, enabled())
		}
	}
	// 古い mode は読まない（400）。
	for _, body := range []string{`{"mode":"OK"}`, `{"mode":"actual"}`, `{"line":"OK","mode":"OK"}`, `{"line":1}`, `{`} {
		if got := call("PUT", "/api/sources/test-obs", body); got != 400 || enabled() {
			t.Errorf("%s: %d", body, got)
		}
	}
	// 軸の値の誤りと、知らない自由の値は 422。
	for _, body := range []string{`{"line":"ok"}`, `{"standby":"standby"}`, `{"stream":"offline ","values":{}}`, `{"values":{"password":"x"}}`, `{"line":"OK","values":{"obs.streaming":1}}`} {
		if got := call("PUT", "/api/sources/test-obs", body); got != 422 || enabled() {
			t.Errorf("%s: %d", body, got)
		}
	}
	// 自由だけでも、軸だけでもオンにできる。
	for _, body := range []string{`{"values":{"obs.fps":1}}`, `{"stream":"offline"}`, `{"line":"LOW","standby":"none","stream":"streaming","values":{"obs.fps":1}}`} {
		if got := call("PUT", "/api/sources/test-obs", body); got != 200 || !enabled() {
			t.Errorf("%s: %d", body, got)
		}
		call("DELETE", "/api/sources/test-obs", "")
	}
}

func TestSimulatedPagesFollowAllAxes(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	b.snap.Cameras = []core.CameraStatus{{ID: "cam1", Kbps: 777, State: "OK"}}
	api := apiHandler(b, nil)
	r := httptest.NewRequest("POST", "/api/sources", strings.NewReader(`{"name":"a","body":"{cam1.state}/{cam1.kbps}/{obs.streaming}"}`))
	r.Header.Set(headerName, "1")
	api.ServeHTTP(httptest.NewRecorder(), r)
	preview := func(path, body string) string {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(body, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	for _, tc := range []struct{ sim, want string }{
		{`{}`, `OK/777/false`},
		{`{"line":"LOW"}`, `LOW/600/false`},
		{`{"stream":"streaming"}`, `OK/777/true`},
		{`{"line":"DOWN","stream":"streaming","values":{"cam1.kbps":5}}`, `DOWN/5/true`},
		{`{"values":{"cam1.state":"LOW"}}`, `LOW/777/false`},
	} {
		if got := preview("/api/sources/preview", `{"names":["a"],"simulate":`+tc.sim+`}`); !strings.Contains(got, `"a":"`+tc.want+`"`) {
			t.Errorf("%s: %s", tc.sim, got)
		}
	}
}

func TestTestOBSStreamStartResetsWithFakeClock(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	log := &simulationLog{}
	e := NewEvents(b, log, nil)
	defer e.Close()
	h := apiHandler(b, e)
	put := func() {
		t.Helper()
		r := httptest.NewRequest("PUT", "/api/sources/test-obs", strings.NewReader(`{"line":"LOW","standby":"auto","stream":"offline","values":{"obs.fps":1}}`))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	put()
	e.mu.Lock()
	at := e.testOBS.Alive
	e.mu.Unlock()
	if sim, _ := e.simulationState(at.Add(10 * time.Second)); sim.IsActual() {
		t.Fatal("must stay on")
	}
	// 配信が始まったら、そのあとの読みでテストが止まり、理由が stream_started になる（軸も自由も全部戻る）。
	b.snap.OBS.Streaming = true
	sim, reason := e.simulationState(at.Add(11 * time.Second))
	if !sim.IsActual() || reason != "stream_started" {
		t.Fatalf("%+v %q", sim, reason)
	}
	e.mu.Lock()
	state := e.testOBS
	e.mu.Unlock()
	payload, _ := json.Marshal(e.sourcesPayload(nil))
	if state.Enabled || !state.Simulate.IsActual() || !strings.Contains(string(payload), `"reason":"stream_started"`) {
		t.Fatalf("%+v %s", state, payload)
	}
	// 配信中はオンにできない。
	r := httptest.NewRequest("PUT", "/api/sources/test-obs", strings.NewReader(`{"line":"OK"}`))
	r.Header.Set(headerName, "1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	// 配信が終われば、新しい状況でまたオンにできる。45 秒の期限は偽の時計で確かめる。
	b.snap.OBS.Streaming = false
	put()
	e.mu.Lock()
	at = e.testOBS.Alive
	e.mu.Unlock()
	if sim, _ := e.simulationState(at.Add(44 * time.Second)); sim.IsActual() {
		t.Fatal("expired early")
	}
	if sim, reason := e.simulationState(at.Add(45 * time.Second)); !sim.IsActual() || reason != "timeout" {
		t.Fatalf("%+v %q", sim, reason)
	}
}

// テストは表示だけ。status（Snapshot の中身も）は、軸と自由をすべて使っても、前後で1バイトも変わらない。
func TestSimulationDoesNotChangeStatus(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	b.snap.Cameras = []core.CameraStatus{{ID: "cam1", State: "OK", Kbps: 3000, Online: true, Protocol: "rtmp"}, {ID: "cam2", State: "DOWN"}}
	b.snap.OBS.Scene, b.snap.OBS.SceneClass = "Real", "standby"
	e := NewEvents(b, nil, nil)
	defer e.Close()
	h := apiHandler(b, e)
	status := func() string {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/status", nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	deep, _ := json.Marshal(b.snap)
	before := status()
	call := func(method, path, body string) {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(method, path, w.Code, w.Body.String())
		}
	}
	call("PUT", "/api/sources/test-obs", `{"line":"DOWN","standby":"auto","stream":"streaming","values":{"cam1.kbps":1}}`)
	call("POST", "/api/sources/preview", `{"names":[],"simulate":{"line":"LOW","standby":"none","stream":"offline"}}`)
	call("POST", "/api/source-values", `{"simulate":{"line":"OK","values":{"cam2.state":"LOW"}}}`)
	e.updateSources(time.Now())
	if got := status(); got != before {
		t.Fatalf("status changed:\n%s\n%s", before, got)
	}
	if after, _ := json.Marshal(b.snap); string(after) != string(deep) {
		t.Fatal("snapshot changed")
	}
}
