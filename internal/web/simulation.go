package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"io"
	"net/http"
	"time"
)

type testOBS struct {
	Enabled  bool               `json:"enabled"`
	Simulate sources.Simulation `json:"simulate"`
	Reason   string             `json:"reason"`
	Alive    time.Time          `json:"-"`
}

// simulationState returns the log reason to its caller, which must release all
// locks before writing it to the generation logger.
func (e *Events) simulationState(now time.Time) (sources.Simulation, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	reason := ""
	if e.testOBS.Enabled {
		if e.backend.Snapshot().OBS.Streaming {
			reason = "stream_started"
		} else if now.Sub(e.testOBS.Alive) >= 45*time.Second {
			reason = "timeout"
		}
		if reason != "" {
			e.testOBS = testOBS{Reason: reason}
		}
	}
	if e.testOBS.Enabled {
		return e.testOBS.Simulate, ""
	}
	return sources.Simulation{}, reason
}
func (e *Events) simulation(now time.Time) sources.Simulation {
	sim, reason := e.simulationState(now)
	e.logSimulationStop(reason)
	return sim
}
func (e *Events) logSimulationStop(reason string) {
	if reason != "" {
		e.logger.Info("test obs stopped", "reason", reason)
	}
}
func (e *Events) sourcesPayload(texts map[string]string) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	return map[string]any{"sources": texts, "test_obs": e.testOBS}
}

// decodeSourcesRequest shares preview's strict shape, single JSON value and size limit.
func decodeSourcesRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	d.DisallowUnknownFields()
	return d.Decode(v) == nil && d.Decode(new(any)) == io.EOF
}

func serveSimulation(w http.ResponseWriter, r *http.Request, b Backend, e *Events, lang i18n.Lang) bool {
	path := r.URL.Path
	if path != "/api/sources/preview" && path != "/api/sources/test-obs" && path != "/api/sources/test-obs/alive" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	fail := func(code int) { writeError(w, code, lang, i18n.New("sources.invalid")) }

	now := time.Now()
	if path != "/api/sources/preview" {
		if e == nil {
			fail(503)
			return true
		}
		var sim sources.Simulation
		if r.Method == "PUT" {
			if !decodeSourcesRequest(w, r, &sim) {
				fail(400)
				return true
			}
			// すべての軸が実際で値もなければ、反映するものがない。
			if sim.IsActual() {
				fail(422)
				return true
			}
			if _, err := sim.Apply(b.Snapshot(), now, nil); err != nil {
				fail(422)
				return true
			}
		}
		e.updateMu.Lock()
		e.mu.Lock()
		if r.Method == "PUT" && b.Snapshot().OBS.Streaming {
			e.mu.Unlock()
			e.updateMu.Unlock()
			writeJSON(w, 409, map[string]string{"code": "sources.test_streaming", "error": i18n.Render(lang, i18n.New("sources.test_streaming"))})
			return true
		}
		previous := e.testOBS
		switch {
		case r.Method == "DELETE":
			e.testOBS = testOBS{Reason: "disabled"}
		case r.Method == "PUT":
			e.testOBS = testOBS{Enabled: true, Simulate: sim, Alive: now}
		case e.testOBS.Enabled:
			e.testOBS.Alive = now
		}
		e.mu.Unlock()
		e.updateMu.Unlock()
		if r.Method == "PUT" && (!previous.Enabled || !previous.Simulate.SameChoices(sim)) {
			// 状況の名前だけを書く。自由の値は書かない。
			e.logger.Info("test obs enabled", sim.LogFields()...)
		} else if r.Method == "DELETE" && previous.Enabled {
			e.logger.Info("test obs disabled")
		}
		e.updateSources(now)
		writeJSON(w, 200, e.sourcesPayload(e.texts()))
		return true
	}
	var req struct {
		Names    *[]string          `json:"names"`
		Source   *json.RawMessage   `json:"source"`
		Simulate sources.Simulation `json:"simulate"`
	}
	if !decodeSourcesRequest(w, r, &req) || (req.Names == nil) == (req.Source == nil) {
		fail(400)
		return true
	}
	snapshot := b.Snapshot()
	var stats map[string]template.Value
	if e != nil {
		stats = e.statisticsValues(now)
	}
	if _, err := req.Simulate.Apply(snapshot, now, stats); err != nil {
		fail(422)
		return true
	}
	render := func(src sources.Source) (string, bool) {
		text, err := sources.RenderSimulation(src, snapshot, now, stats, req.Simulate)
		if err != nil {
			var te *template.Error
			if errors.As(err, &te) {
				detail := map[string]any{"error": i18n.Render(lang, te.Message), "code": te.Message.Code, "line": te.Line, "column": te.Column}
				if req.Source != nil {
					writeJSON(w, 200, map[string]any{"template_error": detail})
				} else {
					writeJSON(w, 422, detail)
				}
			} else {
				fail(422)
			}
			return "", false
		}
		return text, true
	}
	if req.Source != nil {
		src := sources.Default()
		d := json.NewDecoder(bytes.NewReader(*req.Source))
		d.DisallowUnknownFields()
		if d.Decode(&src) != nil {
			fail(400)
			return true
		}
		text, ok := render(src)
		if !ok {
			return true
		}
		if src.Name == "" {
			src.Name = "preview"
		}
		if src.Validate(snapshot) != nil {
			fail(422)
			return true
		}
		writeJSON(w, 200, map[string]string{"text": text, "css": sources.Stylesheet(src)})
		return true
	}
	list := sourceList(b)
	out := map[string]string{}
	for _, name := range *req.Names {
		found := false
		for _, src := range list {
			if src.Name == name {
				text, ok := render(src)
				if !ok {
					return true
				}
				out[name] = text
				found = true
				break
			}
		}
		if !found {
			fail(404)
			return true
		}
	}
	writeJSON(w, 200, map[string]any{"sources": out})
	return true
}
func (e *Events) texts() map[string]string { e.mu.Lock(); defer e.mu.Unlock(); return e.sourceTexts }

// 配信開始の通知は Snapshot の次の更新を待たずに解除する。
func (e *Events) stopSimulationEvent(v interface{ Kind() string }) {
	if v.Kind() != "stream" && v.Kind() != "obs_connected" {
		return
	}
	raw, _ := json.Marshal(v)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	streaming, _ := fields["streaming"].(bool)
	if !streaming {
		streaming, _ = fields["Streaming"].(bool)
	}
	if streaming {
		e.updateMu.Lock()
		e.mu.Lock()
		stopped := e.testOBS.Enabled
		if stopped {
			e.testOBS = testOBS{Reason: "stream_started"}
		}
		e.mu.Unlock()
		e.updateMu.Unlock()
		if stopped {
			e.logSimulationStop("stream_started")
		}
		e.updateSources(time.Now())
	}
}
