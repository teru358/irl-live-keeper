package core

import (
	"sync/atomic"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// Event は記録に使う、時刻と種類を持つできごと。
type Event interface {
	Kind() string
	Time() time.Time
}

// CameraSample は sample のできごとに含めるカメラごとの値。
type CameraSample struct {
	ID       string
	IVMs     int64
	Kbps     float64
	State    string
	Online   bool
	Protocol string
	RTTMs    *float64
	Loss     *float64
	Mbps     *float64
	Err      bool
}

// SampleEvent は Poller が出した SampleSet 1組分の値。
type SampleEvent struct {
	At      time.Time
	Cameras []CameraSample
}

func (SampleEvent) Kind() string      { return "sample" }
func (e SampleEvent) Time() time.Time { return e.At }

// DroppedEvent は購読者が受け取れなかったできごとの数。
type DroppedEvent struct {
	At time.Time
	N  int
}

func (DroppedEvent) Kind() string      { return "dropped" }
func (e DroppedEvent) Time() time.Time { return e.At }

// EventSub はできごとの受け取り口と、未通知の落とした数を持つ。
type EventSub struct {
	C       <-chan Event
	ch      chan Event
	dropped atomic.Int64
}

// Dropped はまだ dropped できごととして送れていない件数を返す。
func (s *EventSub) Dropped() int { return int(s.dropped.Load()) }

// SubscribeSnapshots は Snapshot が公開された知らせを容量1で受け取る。
func (c *Core) SubscribeSnapshots() <-chan struct{} {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	if c.runStarted {
		panic("Run のあとには購読を登録できません")
	}
	ch := make(chan struct{}, 1)
	c.snapshotSubs = append(c.snapshotSubs, ch)
	return ch
}

// SubscribeEvents は容量 n のできごと購読を登録する。n は1以上。
func (c *Core) SubscribeEvents(n int) *EventSub {
	if n < 1 {
		panic("できごと購読の容量は1以上です")
	}
	c.subMu.Lock()
	defer c.subMu.Unlock()
	if c.runStarted {
		panic("Run のあとには購読を登録できません")
	}
	ch := make(chan Event, n)
	s := &EventSub{C: ch, ch: ch}
	c.eventSubs = append(c.eventSubs, s)
	return s
}

func (c *Core) emitSample(set ingest.SampleSet) {
	e := SampleEvent{At: set.At, Cameras: make([]CameraSample, 0, len(c.cams))}
	byID := make(map[string]ingest.Sample, len(set.Cameras))
	for _, s := range set.Cameras {
		byID[s.Camera] = s
	}
	for _, cam := range c.cams {
		s, ok := byID[cam.cfg.ID]
		if !ok {
			continue
		}
		v := CameraSample{ID: cam.cfg.ID, IVMs: s.Interval.Milliseconds(), Kbps: s.Kbps,
			State: cam.judge.State().String(), Online: s.Online, Protocol: s.Protocol, Err: s.Err != nil}
		if s.Protocol == "srt" && s.SRT != nil {
			rtt, loss, mbps := s.SRT.RTTMs, s.SRT.LossRate, s.SRT.RecvMbps
			v.RTTMs, v.Loss, v.Mbps = &rtt, &loss, &mbps
		}
		e.Cameras = append(e.Cameras, v)
	}
	c.publishEvent(e)
}

func (c *Core) publishEvent(e Event) {
	for _, s := range c.eventSubs {
		if n := s.dropped.Load(); n > 0 {
			select {
			case s.ch <- DroppedEvent{At: c.now(), N: int(n)}:
				s.dropped.Store(0)
			default:
				s.dropped.Add(1)
				continue
			}
		}
		select {
		case s.ch <- e:
		default:
			s.dropped.Add(1)
		}
	}
}

func (c *Core) notifySnapshot() {
	for _, ch := range c.snapshotSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (c *Core) closeSubscriptions() {
	for _, ch := range c.snapshotSubs {
		close(ch)
	}
	for _, s := range c.eventSubs {
		close(s.ch)
	}
}
