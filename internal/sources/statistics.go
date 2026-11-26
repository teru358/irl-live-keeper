package sources

import (
	"github.com/andreykaipov/goobs/api/requests/general"
	"github.com/andreykaipov/goobs/api/requests/stream"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"time"
)

type StatisticsReader interface {
	StatisticsEpoch() any
	GetStreamStatus() (*stream.GetStreamStatusResponse, error)
	GetStats() (*general.GetStatsResponse, error)
}

// Statistics is owned by the polling goroutine; published samples are independent maps.
type Statistics struct {
	epoch    any
	bytes    float64
	at       time.Time
	sampleAt time.Time
	values   map[string]template.Value
}

func (s *Statistics) Read(r StatisticsReader, now func() time.Time) {
	epoch := r.StatisticsEpoch()
	if epoch != s.epoch {
		s.at = time.Time{}
		s.epoch = epoch
	}
	s.values = map[string]template.Value{}
	status, err := r.GetStreamStatus()
	at := now()
	if r.StatisticsEpoch() != epoch {
		s.at = time.Time{}
		return
	}
	s.sampleAt = at
	if err == nil && status != nil {
		if status.OutputActive {
			s.values["stream.elapsed"] = template.Value{Defined: true, Type: template.Seconds, Number: status.OutputDuration / 1000}
			s.number("stream.dropped", status.OutputSkippedFrames)
			s.number("stream.congestion", status.OutputCongestion)
			if !s.at.IsZero() && status.OutputBytes >= s.bytes && at.After(s.at) {
				s.number("stream.kbps", (status.OutputBytes-s.bytes)*8/1000/at.Sub(s.at).Seconds())
			}
			s.bytes, s.at = status.OutputBytes, at
		} else {
			s.at = time.Time{}
		}
	}
	stats, err := r.GetStats()
	if r.StatisticsEpoch() != epoch {
		s.values = nil
		s.at = time.Time{}
		return
	}
	if err == nil && stats != nil {
		s.number("obs.fps", stats.ActiveFps)
		s.number("obs.render_skipped", stats.RenderSkippedFrames)
	}
}
func (s *Statistics) number(name string, n float64) {
	s.values[name] = template.Value{Defined: true, Type: template.Number, Number: n}
}
func (s Statistics) Values(now time.Time) map[string]template.Value {
	v := map[string]template.Value{}
	for n, x := range s.values {
		if n == "stream.elapsed" {
			x.Number += max(0, now.Sub(s.sampleAt).Seconds())
		}
		v[n] = x
	}
	return v
}

// UsesStatistics checks parsed references, including conditional expressions.
func UsesStatistics(s Source) bool {
	for _, name := range []string{"stream.elapsed", "stream.kbps", "stream.dropped", "stream.congestion", "obs.fps", "obs.render_skipped"} {
		if template.References(s.Template, name) {
			return true
		}
	}
	return false
}
