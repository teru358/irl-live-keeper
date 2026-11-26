package web

import (
	"github.com/teru358/irl-live-keeper/internal/sources"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"time"
)

func (e *Events) statisticsValues(now time.Time) map[string]template.Value {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statistics.Values(now)
}
func (e *Events) needsStatistics(list []sources.Source) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range list {
		if !sources.UsesStatistics(s) {
			continue
		}
		for c, active := range e.clients {
			if active && (c.topics["sources"] || c.topics["source:"+s.Name]) {
				return true
			}
		}
	}
	return false
}
func (e *Events) pollStatistics() {
	defer close(e.statisticsDone)
	p, ok := e.backend.(interface {
		StatisticsReader() sources.StatisticsReader
	})
	if !ok || p.StatisticsReader() == nil {
		return
	}
	r := p.StatisticsReader()
	ticks := time.NewTicker(2 * time.Second)
	defer ticks.Stop()
	var stats sources.Statistics
	for {
		select {
		case <-e.done:
			return
		case <-ticks.C:
			if e.backend.Snapshot().OBS.Connected && e.needsStatistics(sourceList(e.backend)) {
				stats.Read(r, time.Now)
			} else {
				stats = sources.Statistics{}
			}
			e.mu.Lock()
			e.statistics = stats
			e.mu.Unlock()
		}
	}
}
