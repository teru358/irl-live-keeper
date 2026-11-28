package sources

import (
	"errors"
	"github.com/andreykaipov/goobs/api/requests/general"
	"github.com/andreykaipov/goobs/api/requests/stream"
	"testing"
	"time"
)

type fakeStatistics struct {
	epoch  int
	status stream.GetStreamStatusResponse
	fail   bool
	calls  int
}

func (f *fakeStatistics) StatisticsEpoch() any { return f.epoch }
func (f *fakeStatistics) GetStreamStatus() (*stream.GetStreamStatusResponse, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("failed")
	}
	return &f.status, nil
}
func (f *fakeStatistics) GetStats() (*general.GetStatsResponse, error) {
	if f.fail {
		return nil, errors.New("failed")
	}
	return &general.GetStatsResponse{ActiveFps: 60, RenderSkippedFrames: 3}, nil
}
func TestStatistics(t *testing.T) {
	now := time.Unix(100, 0)
	f := &fakeStatistics{status: stream.GetStreamStatusResponse{OutputActive: true, OutputBytes: 1000, OutputDuration: 10000}}
	var s Statistics
	read := func() { s.Read(f, func() time.Time { return now }) }
	read()
	if s.Values(now)["stream.kbps"].Defined {
		t.Fatal("first sample")
	}
	if got := s.Values(now.Add(time.Second))["stream.elapsed"].Number; got != 11 {
		t.Fatal(got)
	}
	now = now.Add(2 * time.Second)
	f.status.OutputBytes = 3000
	read()
	if got := s.Values(now)["stream.kbps"].Number; got != 8 {
		t.Fatal(got)
	}
	f.fail = true
	now = now.Add(2 * time.Second)
	read()
	if s.Values(now)["stream.kbps"].Defined || s.Values(now)["stream.elapsed"].Defined || s.Values(now)["obs.fps"].Defined {
		t.Fatal("failed sample defined")
	}
	f.fail = false
	now = now.Add(2 * time.Second)
	f.status.OutputBytes = 7000
	read()
	if got := s.Values(now)["stream.kbps"].Number; got != 8 {
		t.Fatal(got)
	}
	f.epoch++
	now = now.Add(2 * time.Second)
	read()
	if s.Values(now)["stream.kbps"].Defined {
		t.Fatal("reconnect")
	}
	f.status.OutputBytes = 0
	now = now.Add(2 * time.Second)
	read()
	if s.Values(now)["stream.kbps"].Defined {
		t.Fatal("decrease")
	}
	f.status.OutputActive = false
	read()
	for _, n := range []string{"stream.kbps", "stream.elapsed", "stream.dropped", "stream.congestion"} {
		if s.Values(now)[n].Defined {
			t.Fatal(n)
		}
	}
	if !s.Values(now)["obs.fps"].Defined {
		t.Fatal("fps while stopped")
	}
	f.status.OutputActive = true
	read()
	if s.Values(now)["stream.kbps"].Defined {
		t.Fatal("restart")
	}
}
func TestUsesStatistics(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"stream.kbps", false}, {"{{stream.kbps}}", false}, {"{stream.kbps}", true}, {"{if obs.fps > 30}yes{end}", true}, {"{cam1.kbps}", false},
	} {
		if got := UsesStatistics(Source{Body: tc.text}); got != tc.want {
			t.Errorf("%q: %v", tc.text, got)
		}
	}
}
