package web

import (
	"context"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type eventBackend struct{}

func (eventBackend) Snapshot() core.Snapshot {
	return core.Snapshot{OBS: core.OBSStatus{Warnings: []i18n.Msg{i18n.New("api.stopped")}}}
}
func (eventBackend) Command(context.Context, switcher.CommandKind) error { return nil }

type eventWriter struct {
	mu        sync.Mutex
	h         http.Header
	body      string
	deadlines []time.Time
	flushed   chan struct{}
}

func (w *eventWriter) Header() http.Header { return w.h }
func (w *eventWriter) WriteHeader(int)     {}
func (w *eventWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.body += string(b)
	return len(b), nil
}
func (w *eventWriter) Flush() {
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}
func (w *eventWriter) SetWriteDeadline(t time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deadlines = append(w.deadlines, t)
	return nil
}
func TestEventsLifecycle(t *testing.T) {
	e := NewEvents(eventBackend{}, nil, func() any { return map[string]string{"state": "idle"} })
	tick := make(chan time.Time, 1)
	hb := make(chan time.Time, 1)
	e.ticker = func(d time.Duration) (<-chan time.Time, func()) {
		if d == time.Second {
			return tick, func() {}
		}
		return hb, func() {}
	}
	w := &eventWriter{h: make(http.Header), flushed: make(chan struct{}, 8)}
	done := make(chan struct{})
	go func() { defer close(done); e.ServeHTTP(w, httptest.NewRequest("GET", "/api/events", nil)) }()
	<-w.flushed
	hb <- time.Now()
	<-w.flushed
	e.broadcast(eventMessage{"sample", sampleJSON(core.SampleEvent{Cameras: []core.CameraSample{{ID: "a", Protocol: "rtmp"}}})})
	<-w.flushed
	e.Close()
	<-done
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, s := range []string{"event: status", "\"apply\"", ": keepalive", "event: sample", "\"iv_ms\""} {
		if !strings.Contains(w.body, s) {
			t.Errorf("不足: %s: %s", s, w.body)
		}
	}
	if len(w.deadlines) < 2 || !w.deadlines[0].IsZero() {
		t.Fatal("期限の解除がない")
	}
	r := httptest.NewRecorder()
	e.ServeHTTP(r, httptest.NewRequest("GET", "/api/events", nil))
	if r.Code != 503 {
		t.Fatal(r.Code)
	}
}
func TestEventsTopicsAuth(t *testing.T) {
	e := NewEvents(eventBackend{}, nil, nil)
	defer e.Close()
	h := NewWithEvents(eventBackend{}, "pw", 8787, NewAuth(nil), e)
	for _, topic := range []string{"unknown", "", "log", "status,log", "log,sources"} {
		r := httptest.NewRequest("GET", "/api/events?topics="+topic, nil)
		r.SetBasicAuth("admin", "pw")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(topic, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/events", nil))
	if w.Code != 401 || w.Header().Get("WWW-Authenticate") != "" {
		t.Fatal(w.Code)
	}
}
func TestEventsOverflow(t *testing.T) {
	e := NewEvents(eventBackend{}, nil, nil)
	c := &eventClient{topics: map[string]bool{"sample": true}, queue: make(chan eventMessage, 64), stop: make(chan struct{}), ended: make(chan struct{}), abort: func() {}}
	e.clients[c] = true
	for i := 0; i < 65; i++ {
		e.broadcast(eventMessage{kind: "sample"})
	}
	select {
	case <-c.stop:
	default:
		t.Fatal("あふれた接続が閉じない")
	}
	close(c.ended)
	e.Close()
}

func TestEventsLanguage(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
		e := NewEvents(eventBackend{}, nil, nil)
		w := &eventWriter{h: make(http.Header), flushed: make(chan struct{}, 8)}
		done := make(chan struct{})
		go func() {
			defer close(done)
			r := httptest.NewRequest("GET", "/api/events?topics=status&lang="+string(lang), nil)
			r.Header.Set("Accept-Language", "en")
			e.ServeHTTP(w, r)
		}()
		<-w.flushed
		e.Close()
		<-done
		w.mu.Lock()
		if !strings.Contains(w.body, i18n.Render(lang, i18n.New("api.stopped"))) {
			t.Error(w.body)
		}
		w.mu.Unlock()
	}
}
