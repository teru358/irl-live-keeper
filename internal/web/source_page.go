package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"html"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const sourceCSP = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self' data:; img-src 'self' data:"

//go:embed assets/source.js
var sourceJS string

func sourceList(b Backend) []sources.Source {
	var list []sources.Source
	if p, ok := b.(interface{ SourcesStore() *sources.Store }); ok {
		store := p.SourcesStore()
		if store == nil {
			return nil
		}
		if _, err := os.Stat(store.Path); os.IsNotExist(err) {
			return nil
		}
		_ = store.Transaction(b.Snapshot(), func(s []sources.Source, err error) ([]sources.Source, bool) {
			if err == nil {
				list = s
			} else {
				slog.Warn(i18n.Render(i18n.EN, i18n.New("sources.unreadable")), "reason", err)
			}
			return nil, false
		})
	}
	return list
}
func serveSource(w http.ResponseWriter, r *http.Request, b Backend, e *Events, lang i18n.Lang) {
	w.Header().Set("Content-Security-Policy", sourceCSP)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		writeError(w, 405, lang, i18n.New("api.method"))
		return
	}
	if r.URL.Path == "/source/assets/source.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		fmt.Fprint(w, sourceJS)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/source/"), "/")
	if len(parts) > 2 || !sources.NamePattern.MatchString(parts[0]) {
		writeError(w, 404, lang, i18n.New("api.not_found"))
		return
	}
	for _, s := range sourceList(b) {
		if s.Name != parts[0] {
			continue
		}
		if len(parts) == 2 {
			switch parts[1] {
			case "style.css":
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
				fmt.Fprint(w, sources.Stylesheet(s))
				return
			case "events":
				if e == nil {
					writeError(w, 503, lang, i18n.New("api.stopped"))
					return
				}
				e.serveSource(w, r, s.Name)
				return
			}
			break
		}
		var stats map[string]template.Value
		if e != nil {
			stats = e.statisticsValues(time.Now())
		}
		text, _ := sources.Render(s, b.Snapshot(), time.Now(), stats)
		empty := ""
		if text == "" {
			empty = " irl-empty"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/source/%s/style.css"><script defer src="/source/assets/source.js"></script></head><body><div class="irl-source irl-source-%s%s"><span class="irl-text">%s</span></div></body></html>`, s.Name, s.Name, empty, html.EscapeString(text))
		return
	}
	writeError(w, 404, lang, i18n.New("api.not_found"))
}

func (e *Events) serveSource(w http.ResponseWriter, r *http.Request, name string) {
	rc := http.NewResponseController(w)
	c := &eventClient{topics: map[string]bool{"source:" + name: true}, queue: make(chan eventMessage, 64), stop: make(chan struct{}), ended: make(chan struct{}), abort: func() { _ = rc.SetWriteDeadline(time.Now()) }}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		w.WriteHeader(503)
		return
	}
	e.clients[c] = true
	initial := e.sourceTexts[name]
	e.mu.Unlock()
	defer func() { e.mu.Lock(); e.remove(c); e.mu.Unlock(); close(c.ended) }()
	_ = rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	send := func(v any) bool {
		data, _ := json.Marshal(v)
		_, err := fmt.Fprintf(w, "data: %s\n\n", data)
		return err == nil && rc.Flush() == nil
	}
	if !send(initial) {
		return
	}
	ticks, stop := e.ticker(15 * time.Second)
	defer stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.stop:
			return
		case m := <-c.queue:
			if !send(m.value) {
				return
			}
		case <-ticks:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
