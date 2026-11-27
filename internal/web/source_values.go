package web

import (
	"net/http"
	"time"

	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
)

func serveSourceValues(w http.ResponseWriter, r *http.Request, b Backend, recent any, lang i18n.Lang) {
	w.Header().Set("Cache-Control", "no-store")
	var req struct {
		Simulate sources.Simulation `json:"simulate"`
	}
	if r.Method == http.MethodPost && !decodeSourcesRequest(w, r, &req) {
		writeError(w, 400, lang, i18n.New("sources.invalid"))
		return
	}
	now := time.Now()
	snapshot := b.Snapshot()
	var stats map[string]template.Value
	if e, ok := recent.(*Events); ok && e != nil {
		e.wantStatistics(now)
		stats = e.statisticsValues(now)
	}
	values, err := req.Simulate.Apply(snapshot, now, stats)
	if err != nil {
		writeError(w, 422, lang, i18n.New("sources.invalid"))
		return
	}
	groups, err := sources.ValueGroups(snapshot, values)
	if err != nil {
		writeError(w, 500, lang, i18n.New("sources.invalid"))
		return
	}
	writeJSON(w, 200, map[string]any{"groups": groups})
}
