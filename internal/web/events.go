package web

import (
	"encoding/json"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/logging"
)

// Events は世代に一つだけ作る配信係。登録は core.Run の前に行う。
type Events struct {
	updateMu      sync.Mutex
	testOBS       testOBS
	publishedTest testOBS

	statistics     sources.Statistics
	statisticsDone chan struct{}
	sourceTexts    map[string]string
	langs          []i18n.Lang
	mu             sync.Mutex
	clients        map[*eventClient]bool
	closed         bool
	done           chan struct{}
	finished       chan struct{}
	backend        Backend
	apply          func() any
	ticker         func(time.Duration) (<-chan time.Time, func())
	recent         interface {
		Recent(int, slog.Level) []logging.Line
	}
}
type eventMessage struct {
	kind  string
	value any
}
type eventClient struct {
	topics map[string]bool
	queue  chan eventMessage
	stop   chan struct{}
	ended  chan struct{}
	abort  func()
}

// NewEvents は受け手を登録し、配信を開始する。Close で必ず終了させる。
func NewEvents(b Backend, logger interface {
	Subscribe(int) (<-chan logging.Line, func())
}, apply func() any, langs ...i18n.Lang) *Events {
	e := &Events{clients: make(map[*eventClient]bool), done: make(chan struct{}), finished: make(chan struct{}), backend: b, apply: apply}
	e.langs = langs
	if recent, ok := logger.(interface {
		Recent(int, slog.Level) []logging.Line
	}); ok {
		e.recent = recent
	}
	e.ticker = func(d time.Duration) (<-chan time.Time, func()) { t := time.NewTicker(d); return t.C, t.Stop }
	var snapshots <-chan struct{}
	var events <-chan core.Event
	if c, ok := b.(interface {
		SubscribeSnapshots() <-chan struct{}
		SubscribeEvents(int) *core.EventSub
	}); ok {
		snapshots = c.SubscribeSnapshots()
		events = c.SubscribeEvents(256).C
	}
	var lines <-chan logging.Line
	unsub := func() {}
	if logger != nil {
		lines, unsub = logger.Subscribe(256)
	}
	e.statisticsDone = make(chan struct{})
	go e.pollStatistics()
	e.updateSources(time.Now())
	ticks, stop := e.ticker(time.Second)
	go func() {
		defer stop()
		defer close(e.finished)
		defer unsub()
		for {
			select {
			case now := <-ticks:
				e.updateSources(now)
			case <-e.done:
				return
			case _, ok := <-snapshots:
				if !ok {
					snapshots = nil
					continue
				}
				e.updateSources(time.Now())
				e.broadcast(eventMessage{kind: "status"})
			case v, ok := <-events:
				if !ok {
					events = nil
					continue
				}
				e.stopSimulationEvent(v)
				switch v := v.(type) {
				case core.SampleEvent:
					e.broadcast(eventMessage{"sample", sampleJSON(v)})
				case core.DroppedEvent:
					e.broadcast(eventMessage{"dropped", map[string]int{"n": v.N}})
				}
			case v, ok := <-lines:
				if !ok {
					lines = nil
					continue
				}
				e.broadcast(eventMessage{"log", map[string]any{"t": v.At, "level": v.Level.String(), "text": v.Text}})
			}
		}
	}()
	return e
}
func sampleJSON(v core.SampleEvent) any {
	cams := make([]map[string]any, 0, len(v.Cameras))
	for _, c := range v.Cameras {
		m := map[string]any{"id": c.ID, "iv_ms": c.IVMs, "kbps": c.Kbps, "state": c.State, "online": c.Online, "protocol": c.Protocol, "err": c.Err}
		if c.Protocol == "srt" {
			if c.RTTMs != nil {
				m["rtt_ms"] = *c.RTTMs
			}
			if c.Loss != nil {
				m["loss"] = *c.Loss
			}
			if c.Mbps != nil {
				m["mbps"] = *c.Mbps
			}
		}
		cams = append(cams, m)
	}
	return map[string]any{"t": v.At, "cameras": cams}
}
func (e *Events) remove(c *eventClient) {
	if e.clients[c] {
		e.clients[c] = false
		close(c.stop)
		c.abort()
	}
}
func (e *Events) broadcast(m eventMessage) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for c, active := range e.clients {
		if !active {
			continue
		}
		topic := m.kind
		if topic == "dropped" {
			topic = "sample"
		}
		if !c.topics[topic] {
			continue
		}
		select {
		case c.queue <- m:
		default:
			e.remove(c)
		}
	}
}

func (e *Events) Recent(n int, level slog.Level) []logging.Line {
	if e == nil || e.recent == nil {
		return []logging.Line{}
	}
	return e.recent.Recent(n, level)
}

// Close は新規接続を拒否し、書き込み中の接続も中断して全ハンドラーを待つ。
func (e *Events) Close() {
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		close(e.done)
	}
	var waits []chan struct{}
	for c := range e.clients {
		waits = append(waits, c.ended)
		e.remove(c)
	}
	e.mu.Unlock()
	for _, ch := range waits {
		<-ch
	}
	<-e.finished
	if e.statisticsDone != nil {
		<-e.statisticsDone
	}
}
func (e *Events) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Select(r.URL.Query().Get("lang"), "", e.langs...)
	topics := map[string]bool{}
	raw := r.URL.Query().Get("topics")
	if !r.URL.Query().Has("topics") {
		raw = "status,sample"
	}
	for _, s := range strings.Split(raw, ",") {
		if s != "status" && s != "sample" && s != "log" && s != "sources" {
			writeError(w, 400, lang, i18n.New("api.invalid", "reason", "topics"))
			return
		}
		topics[s] = true
	}
	rc := http.NewResponseController(w)
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		writeError(w, 503, lang, i18n.New("api.stopped"))
		return
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		e.mu.Unlock()
		writeError(w, 500, lang, i18n.New("api.internal"))
		return
	}
	c := &eventClient{topics: topics, queue: make(chan eventMessage, 64), stop: make(chan struct{}), ended: make(chan struct{}), abort: func() { _ = rc.SetWriteDeadline(time.Now()) }}
	e.clients[c] = true
	e.mu.Unlock()
	defer func() { e.mu.Lock(); e.remove(c); e.mu.Unlock(); close(c.ended) }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(m eventMessage) bool {
		if m.kind == "status" {
			// status API と同じ正規化・翻訳を使う。
			rw := &statusCapture{header: make(http.Header)}
			req := r.Clone(r.Context())
			u := *r.URL
			u.Path = "/api/status"
			req.URL = &u
			req.Header = r.Header.Clone()
			req.Header.Del("Accept-Language")
			apiHandler(e.backend, e, lang).ServeHTTP(rw, req)
			var value map[string]any
			if json.Unmarshal(rw.body, &value) != nil {
				return false
			}
			if e.apply != nil {
				value["apply"] = e.apply()
			}
			m.value = value
		}
		b, err := json.Marshal(m.value)
		if err != nil {
			return false
		}
		if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", m.kind, b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if topics["sources"] {
		e.mu.Lock()
		value := e.sourceTexts
		e.mu.Unlock()
		if !send(eventMessage{"sources", e.sourcesPayload(value)}) {
			return
		}
	}
	if topics["status"] {
		if !send(eventMessage{kind: "status"}) {
			return
		}
	} else if rc.Flush() != nil {
		return
	}
	ticks, stop := e.ticker(time.Second)
	defer stop()
	heartbeats, stopHB := e.ticker(15 * time.Second)
	defer stopHB()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.stop:
			return
		case m := <-c.queue:
			if !send(m) {
				return
			}
		case <-ticks:
			if topics["status"] && !send(eventMessage{kind: "status"}) {
				return
			}
		case <-heartbeats:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		}
	}
}

type statusCapture struct {
	header http.Header
	body   []byte
}

func (w *statusCapture) Header() http.Header { return w.header }
func (w *statusCapture) WriteHeader(int)     {}
func (w *statusCapture) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return len(b), nil
}

func (e *Events) updateSources(now time.Time) {
	e.updateMu.Lock()
	defer e.updateMu.Unlock()
	sim := e.simulation(now)
	next := map[string]string{}
	list := sourceList(e.backend)
	for _, s := range list {
		snapshot := e.backend.Snapshot()
		text, err := sources.RenderSimulation(s, snapshot, now, e.statisticsValues(now), sim)
		if err != nil {
			slog.Warn(i18n.Render(i18n.EN, i18n.New("sources.render_failed")), "name", s.Name, "reason", err)
		}
		next[s.Name] = text
	}
	e.mu.Lock()
	stateChanged := !reflect.DeepEqual(e.publishedTest, e.testOBS)
	e.publishedTest = e.testOBS
	old := e.sourceTexts
	e.sourceTexts = next
	e.mu.Unlock()
	if reflect.DeepEqual(old, next) && !stateChanged {
		return
	}
	for name, text := range next {
		if previous, ok := old[name]; !ok || previous != text {
			e.broadcast(eventMessage{"source:" + name, text})
		}
	}
	for name := range old {
		if _, ok := next[name]; !ok {
			e.broadcast(eventMessage{"source:" + name, ""})
		}
	}
	e.broadcast(eventMessage{"sources", e.sourcesPayload(next)})
}
