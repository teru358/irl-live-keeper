package web

import (
	"bytes"
	"context"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourcePublic(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	s := sources.Default()
	s.Name = "demo"
	s.Body = "<b>x</b>\ntext"
	if err := b.store.Transaction(b.Snapshot(), func([]sources.Source, error) ([]sources.Source, bool) { return []sources.Source{s}, true }); err != nil {
		t.Fatal(err)
	}
	h := New(b, "secret")
	for _, path := range []string{"/source/demo", "/source/assets/source.js"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Header().Get("Content-Security-Policy") != "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self' data:; img-src 'self' data:; base-uri 'none'; form-action 'none'" {
			t.Fatal(w.Code, w.Header())
		}
		body := w.Body.String()
		if strings.Contains(body, "secret") {
			t.Fatal(body)
		}
		if path == "/source/demo" && (!strings.Contains(body, `class="irl-source irl-source-demo"`) || !strings.Contains(body, `class="irl-source irl-source-demo"></div>`) || !strings.Contains(body, "<style>html,body") || strings.Contains(body, "irl-text") || strings.Contains(body, "<script>")) {
			t.Fatal(body)
		}
		if strings.HasSuffix(path, "css") && (w.Header().Get("Content-Type") != "text/css; charset=utf-8" || body != sources.BaseStylesheet) {
			t.Fatal(body)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/source/demo/style.css", nil))
	if w.Code != 404 {
		t.Fatal("old stylesheet still served", w.Code)
	}
	for _, v := range []string{"obs.password", "stream.publish_url", "admin.password"} {
		s.Body = "{" + v + "}"
		if s.Validate(b.Snapshot()) == nil {
			t.Fatal(v)
		}
	}
}
func TestSourceTextChanges(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	s := sources.Default()
	s.Name = "demo"
	s.Body = "{now:hms}"
	_ = b.store.Transaction(b.Snapshot(), func([]sources.Source, error) ([]sources.Source, bool) { return []sources.Source{s}, true })
	c := &eventClient{topics: map[string]bool{"source:demo": true, "sources": true}, queue: make(chan eventMessage, 10)}
	e := &Events{backend: b, clients: map[*eventClient]bool{c: true}}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e.updateSources(now)
	if len(c.queue) != 2 {
		t.Fatal(len(c.queue))
	}
	<-c.queue
	<-c.queue
	b.snap = core.Snapshot{MediaMTX: "changed"}
	e.updateSources(now)
	if len(c.queue) != 0 {
		t.Fatal("unchanged text sent")
	}
	e.updateSources(now.Add(time.Second))
	if len(c.queue) != 2 {
		t.Fatal("clock not advanced")
	}
	m := <-c.queue
	if _, ok := m.value.(string); !ok {
		t.Fatal("non-text public payload", m)
	}
}

func TestSourceTopicShape(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	e := NewEvents(b, nil, nil)
	defer e.Close()
	r := httptest.NewRequest("GET", "/api/events?topics=sources", nil)
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	r = r.WithContext(ctx)
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	e.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "event: sources\ndata: {\"sources\":{},\"test_obs\":") || strings.Contains(w.Body.String(), "event: status") {
		t.Fatal(w.Body.String())
	}
}

type deadlineRecorder struct{ *httptest.ResponseRecorder }

func (*deadlineRecorder) SetWriteDeadline(time.Time) error { return nil }

func TestSourceFailureClearsAndLogs(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	if err := os.WriteFile(b.store.Path, []byte("- name: demo\n  body: '{obs.password}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, nil)))
	defer slog.SetDefault(previous)
	c := &eventClient{topics: map[string]bool{"source:demo": true}, queue: make(chan eventMessage, 2)}
	e := &Events{backend: b, sourceTexts: map[string]string{"demo": "old"}, clients: map[*eventClient]bool{c: true}}
	e.updateSources(time.Now())
	if len(c.queue) != 1 || (<-c.queue).value != "" || log.Len() == 0 {
		t.Fatal("failure did not clear and log", log.String())
	}
}

func TestSourceOldCSSRemoved(t *testing.T) {
	w := httptest.NewRecorder()
	serveSource(w, httptest.NewRequest("GET", "/source/demo/style.css", nil), &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}, nil, "en")
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestSourceFrame(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	h := New(b, "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sources/frame", nil))
	if w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	serveSources(w, httptest.NewRequest("GET", "/api/sources/frame", nil), b, "en")
	if w.Code != 200 || w.Header().Get("Content-Security-Policy") != sourceCSP || strings.Contains(w.Body.String(), "<script") || !strings.Contains(w.Body.String(), `class="irl-source irl-source-preview"></div>`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSourceSSEInitialHTML(t *testing.T) {
	e := &Events{clients: map[*eventClient]bool{}, sourceTexts: map[string]string{"demo": `<b>ready</b>`}, ticker: func(time.Duration) (<-chan time.Time, func()) { return make(chan time.Time), func() {} }}
	r := httptest.NewRequest("GET", "/source/demo/events", nil)
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	e.serveSource(w, r.WithContext(ctx), "demo")
	if got := w.Body.String(); got != "data: \"\\u003cb\\u003eready\\u003c/b\\u003e\"\n\n" {
		t.Fatal(got)
	}
}
