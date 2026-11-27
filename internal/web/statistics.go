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

// statisticsValuesHold は、最後の /api/source-values のあと統計を読み続ける時間（spec v0.26）。
// 値の一覧は開いている間1秒ごとに取るので、閉じれば数秒で読まなくなる。
const statisticsValuesHold = 5 * time.Second

// wantStatistics は、値の一覧が今の値を取った時刻を受け、統計を読み続ける期限を延ばす。
func (e *Events) wantStatistics(at time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if until := at.Add(statisticsValuesHold); until.After(e.valuesUntil) {
		e.valuesUntil = until
	}
}

func (e *Events) needsStatistics(list []sources.Source) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if time.Now().Before(e.valuesUntil) {
		return true
	}
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
