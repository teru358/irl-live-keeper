package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const sourceCSP = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self' data:; img-src 'self' data:; base-uri 'none'; form-action 'none'"

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
	if r.URL.Path == "/source/assets/html_rules.json" {
		w.Header().Set("Content-Type", "application/json")
		w.Write(sources.HTMLRules)
		return
	}
	if r.URL.Path == "/source/assets/html.js" || r.URL.Path == "/source/assets/reconcile.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		data, _ := assets.ReadFile("assets/app/" + strings.TrimPrefix(r.URL.Path, "/source/assets/"))
		w.Write(data)
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
		writeSourceFrame(w, s.Name, true)
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

func writeSourceFrame(w http.ResponseWriter, name string, script bool) {
	w.Header().Set("Content-Security-Policy", sourceCSP)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	styles := sources.BaseStylesheet
	if !script {
		styles += "html,body{overflow:hidden;}\n"
	}
	js := ""
	if script {
		js = `<script type="module" src="/source/assets/source.js"></script>`
	}
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="%s"><style>%s</style>%s</head><body><div class="irl-source irl-source-%s"></div></body></html>`, sourceCSP, styles, js, name)
}
