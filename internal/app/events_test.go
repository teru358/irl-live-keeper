package app

import (
	"context"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"github.com/teru358/irl-live-keeper/internal/web"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type sseBackend struct{}

func (sseBackend) Snapshot() core.Snapshot                             { return core.Snapshot{} }
func (sseBackend) Command(context.Context, switcher.CommandKind) error { return nil }

type sseWriter struct {
	h     http.Header
	ready chan struct{}
}

func (w *sseWriter) Header() http.Header         { return w.h }
func (w *sseWriter) WriteHeader(int)             {}
func (w *sseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *sseWriter) Flush() {
	select {
	case w.ready <- struct{}{}:
	default:
	}
}
func (w *sseWriter) SetWriteDeadline(time.Time) error { return nil }

type sseShutdown struct {
	t    *testing.T
	done chan struct{}
}

func (s *sseShutdown) Serve(net.Listener) error { return nil }
func (s *sseShutdown) Close() error             { return nil }
func (s *sseShutdown) Shutdown(context.Context) error {
	select {
	case <-s.done:
	case <-time.After(time.Second):
		s.t.Error("http.shutdown より前に SSE が終了していない")
	}
	return nil
}
func TestEventsGenerationShutdown(t *testing.T) {
	e := web.NewEvents(sseBackend{}, nil, nil)
	w := &sseWriter{h: make(http.Header), ready: make(chan struct{}, 1)}
	done := make(chan struct{})
	// ハンドラーの終了通知も Close が待つことを、外側の通知との順序によらず確認する。
	go func() { e.ServeHTTP(w, httptest.NewRequest("GET", "/api/events?topics=status", nil)); close(done) }()
	<-w.ready
	_, cancel := context.WithCancel(context.Background())
	r := &generationRecord{}
	a := &App{deps: appDeps{stage: func(s string) { r.add(s) }}}
	g := &generation{app: a, events: e, cancel: cancel, logger: &generationLog{log: slog.New(slog.NewTextHandler(io.Discard, nil)), r: r}}
	// Shutdown の時点で新規の接続も拒否する。
	checked := make(chan struct{})
	g.server = &sseShutdown{t: t, done: checked}
	go func() { <-done; close(checked) }()
	g.stopGeneration()
	resp := httptest.NewRecorder()
	e.ServeHTTP(resp, httptest.NewRequest("GET", "/api/events", nil))
	if resp.Code != 503 {
		t.Fatal(resp.Code)
	}
}
