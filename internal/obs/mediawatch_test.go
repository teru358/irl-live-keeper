package obs

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	statePlaying = "OBS_MEDIA_STATE_PLAYING"
	stateEnded   = "OBS_MEDIA_STATE_ENDED"
)

// fakeMediaAux は MediaAux の偽物。状態は statusFn で差し替える。入れ直しは記録する。
type fakeMediaAux struct {
	mu       sync.Mutex
	statusFn func(uuid string, n int) (MediaStatus, error) // n はその UUID への何回目の読みか（1 から）
	reads    map[string]int
	settings map[string]map[string]any // uuid → 今の設定
	sets     []setCall
	getErr   error
}

type setCall struct {
	UUID     string
	Settings map[string]any
	Overlay  bool
}

func newFakeMediaAux() *fakeMediaAux {
	return &fakeMediaAux{reads: map[string]int{}, settings: map[string]map[string]any{}}
}

func (f *fakeMediaAux) GetMediaInputStatus(uuid string) (MediaStatus, error) {
	f.mu.Lock()
	f.reads[uuid]++
	n := f.reads[uuid]
	fn := f.statusFn
	f.mu.Unlock()
	return fn(uuid, n)
}

func (f *fakeMediaAux) GetInputSettings(uuid string) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.settings[uuid], nil
}

func (f *fakeMediaAux) SetInputSettings(uuid string, settings map[string]any, overlay bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets = append(f.sets, setCall{uuid, settings, overlay})
	return nil
}

func (f *fakeMediaAux) setCalls() []setCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]setCall(nil), f.sets...)
}

func (f *fakeMediaAux) readCount(uuid string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[uuid]
}

type mwHarness struct {
	t   *testing.T
	clk *fakeClock
	aux *fakeMediaAux
	mw  *MediaWatch
	log *mwLog
}

type mwLog struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *mwLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *mwLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func newMWHarness(t *testing.T) *mwHarness {
	t.Helper()
	h := &mwHarness{t: t, clk: newFakeClock(), aux: newFakeMediaAux(), log: &mwLog{}}
	h.aux.settings["u1"] = map[string]any{"input": "rtsp://127.0.0.1:8554/cam1", "other": 1.0}
	h.mw = NewMediaWatch(h.aux, slog.New(slog.NewTextHandler(h.log, nil)), h.clk)
	t.Cleanup(h.mw.Close)
	return h
}

// step は、時計の待ちが1つ以上できるのを待って、d だけ進める。
func (h *mwHarness) step(d time.Duration) {
	h.t.Helper()
	waitFor(h.t, "時計の待ち", func() bool { return h.clk.pending() > 0 })
	h.clk.Advance(d)
}

// moving は、読むたびに位置が進む映っているソース。
func moving(_ string, n int) (MediaStatus, error) {
	return MediaStatus{State: statePlaying, Cursor: float64(n) * 1000}, nil
}

// stuck は、PLAYING なのに位置が 0 のままのソース。
func stuck(string, int) (MediaStatus, error) {
	return MediaStatus{State: statePlaying, Cursor: 0}, nil
}

var src1 = []MediaSource{{UUID: "u1", Name: "IRL"}}

func TestMediaWatchMovingDoesNothing(t *testing.T) {
	h := newMWHarness(t)
	h.aux.statusFn = moving
	h.mw.Watch("cam1", src1)
	h.step(5 * time.Second)
	waitFor(t, "1回目の読み", func() bool { return h.aux.readCount("u1") == 1 })
	h.step(time.Second)
	waitFor(t, "2回目の読み", func() bool { return h.aux.readCount("u1") == 2 })
	h.mw.Close() // 終わるまで待つ
	if n := len(h.aux.setCalls()); n != 0 {
		t.Fatalf("映っているソースを入れ直した: %v", h.aux.setCalls())
	}
	if w := h.mw.Warnings(); len(w) != 0 {
		t.Fatalf("警告が出た: %v", w)
	}
}

func TestMediaWatchStuckAtZeroRefreshes(t *testing.T) {
	h := newMWHarness(t)
	h.aux.statusFn = stuck
	h.mw.Watch("cam1", src1)
	h.step(5 * time.Second)
	h.step(time.Second)
	waitFor(t, "入れ直し", func() bool { return len(h.aux.setCalls()) == 1 })
	c := h.aux.setCalls()[0]
	if c.UUID != "u1" || c.Settings["input"] != "rtsp://127.0.0.1:8554/cam1" || !c.Overlay {
		t.Fatalf("入れ直しの内容が違う: %+v", c)
	}
	waitFor(t, "ログ", func() bool { return strings.Contains(h.log.String(), "media source refreshed") })
	if l := h.log.String(); !strings.Contains(l, "camera=cam1") || !strings.Contains(l, "source=IRL") {
		t.Fatalf("ログに camera と source がない: %s", l)
	}
}

func TestMediaWatchEndedRefreshes(t *testing.T) {
	h := newMWHarness(t)
	h.aux.statusFn = func(string, int) (MediaStatus, error) { return MediaStatus{State: stateEnded}, nil }
	h.mw.Watch("cam1", src1)
	h.step(5 * time.Second)
	h.step(time.Second)
	waitFor(t, "入れ直し", func() bool { return len(h.aux.setCalls()) == 1 })
}

func TestMediaWatchRefreshThenRecovers(t *testing.T) {
	h := newMWHarness(t)
	// 入れ直したあとは位置が進む。
	h.aux.statusFn = func(uuid string, n int) (MediaStatus, error) {
		if len(h.aux.setCalls()) == 0 {
			return stuck(uuid, n)
		}
		return moving(uuid, n)
	}
	h.mw.Watch("cam1", src1)
	h.step(5 * time.Second)
	h.step(time.Second)
	waitFor(t, "入れ直し", func() bool { return len(h.aux.setCalls()) == 1 })
	h.step(10 * time.Second)
	h.step(time.Second)
	h.mw.Close()
	if n := len(h.aux.setCalls()); n != 1 {
		t.Fatalf("戻ったのに入れ直した: %d 回", n)
	}
	if w := h.mw.Warnings(); len(w) != 0 {
		t.Fatalf("警告が出た: %v", w)
	}
}

func TestMediaWatchRefreshesTwiceThenWarnsUntilNextWatch(t *testing.T) {
	h := newMWHarness(t)
	h.aux.statusFn = stuck
	h.mw.Watch("cam1", src1)
	h.step(5 * time.Second)
	h.step(time.Second)
	waitFor(t, "1回目の入れ直し", func() bool { return len(h.aux.setCalls()) == 1 })
	if w := h.mw.Warnings(); len(w) != 0 {
		t.Fatalf("1回入れ直しただけで警告が出た: %v", w)
	}
	h.step(10 * time.Second)
	h.step(time.Second)
	waitFor(t, "2回目の入れ直し", func() bool { return len(h.aux.setCalls()) == 2 })
	if w := h.mw.Warnings(); len(w) != 0 {
		t.Fatalf("2回目の入れ直しの直後に警告が出た: %v", w)
	}
	h.step(10 * time.Second)
	h.step(time.Second)
	waitFor(t, "警告", func() bool { return len(h.mw.Warnings()) == 1 })
	if w := h.mw.Warnings(); w[0] != "IRL が映っていない可能性があります" {
		t.Fatalf("警告の文が違う: %v", w)
	}
	if n := len(h.aux.setCalls()); n != 2 {
		t.Fatalf("入れ直しは2回まで: %d", n)
	}
	// 時間がたっても警告は残る。
	h.clk.Advance(time.Hour)
	if len(h.mw.Warnings()) != 1 {
		t.Fatal("警告が消えた")
	}
	// 別のカメラの Watch では消えない。
	h.mw.Watch("other", nil)
	if len(h.mw.Warnings()) != 1 {
		t.Fatal("別のカメラの見張りで警告が消えた")
	}
	// 次の Online（同じカメラの Watch）で消える。
	h.aux.statusFn = moving
	h.mw.Watch("cam1", src1)
	if w := h.mw.Warnings(); len(w) != 0 {
		t.Fatalf("次の Watch で警告が消えない: %v", w)
	}
}

func TestMediaWatchOverlapKeepsOnlyNewest(t *testing.T) {
	h := newMWHarness(t)
	h.aux.statusFn = stuck
	h.mw.Watch("cam1", src1)
	waitFor(t, "1本目の待ち", func() bool { return h.clk.pending() == 1 })
	h.mw.Watch("cam1", src1)
	waitFor(t, "2本目の待ち", func() bool { return h.clk.pending() == 2 })
	h.clk.Advance(5 * time.Second) // 古い方も起きるが、取り消されている
	waitFor(t, "新しい方の1回目の読み", func() bool { return h.aux.readCount("u1") >= 1 })
	h.step(time.Second)
	waitFor(t, "入れ直し", func() bool { return len(h.aux.setCalls()) >= 1 })
	h.mw.Close()
	if n := h.aux.readCount("u1"); n != 2 {
		t.Fatalf("読みは新しい方の2回だけのはずが %d 回", n)
	}
	if n := len(h.aux.setCalls()); n != 1 {
		t.Fatalf("入れ直しは1回のはずが %d 回", n)
	}
}

func TestMediaWatchAuxUnavailableDoesNothing(t *testing.T) {
	h := newMWHarness(t)
	h.aux.statusFn = func(string, int) (MediaStatus, error) {
		return MediaStatus{}, fmt.Errorf("GetMediaInputStatus: %w", ErrAuxUnavailable)
	}
	h.mw.Watch("cam1", src1)
	h.step(5 * time.Second)
	waitFor(t, "読み", func() bool { return h.aux.readCount("u1") == 1 })
	h.mw.Close() // 見張りは終わっている（時計を進めなくても Close が戻る）
	if n := len(h.aux.setCalls()); n != 0 {
		t.Fatalf("補助の接続が使えないのに入れ直した: %d", n)
	}
	if w := h.mw.Warnings(); len(w) != 0 {
		t.Fatalf("補助の接続が使えないのに警告が出た: %v", w)
	}
}

func TestMediaWatchSettingsReadFailureDoesNothing(t *testing.T) {
	h := newMWHarness(t)
	h.aux.statusFn = stuck
	h.aux.getErr = errors.New("boom")
	h.mw.Watch("cam1", src1)
	h.step(5 * time.Second)
	h.step(time.Second)
	h.mw.Close()
	if n := len(h.aux.setCalls()); n != 0 {
		t.Fatalf("設定を読めないのに入れ直した: %d", n)
	}
}

func TestMediaWatchCancelAllAndClose(t *testing.T) {
	h := newMWHarness(t)
	h.aux.statusFn = stuck
	h.mw.Watch("cam1", src1)
	waitFor(t, "待ち", func() bool { return h.clk.pending() == 1 })
	h.mw.CancelAll()
	h.clk.Advance(time.Hour)
	h.mw.Close()
	if h.aux.readCount("u1") != 0 || len(h.aux.setCalls()) != 0 {
		t.Fatal("CancelAll のあとに動いた")
	}
	// Close のあとの Watch は何もしない（goroutine を作らない）。
	h.mw.Watch("cam1", src1)
	h.clk.Advance(time.Hour)
	if h.clk.pending() != 0 {
		t.Fatal("Close のあとに見張りが始まった")
	}
}

func TestMediaWatchSeveralSourcesIndependent(t *testing.T) {
	h := newMWHarness(t)
	h.aux.settings["u2"] = map[string]any{"input": "rtsp://x/2"}
	h.aux.statusFn = func(uuid string, n int) (MediaStatus, error) {
		if uuid == "u1" {
			return moving(uuid, n)
		}
		return stuck(uuid, n)
	}
	h.mw.Watch("cam1", []MediaSource{{UUID: "u1", Name: "A"}, {UUID: "u2", Name: "B"}})
	waitFor(t, "待ち2本", func() bool { return h.clk.pending() == 2 })
	h.clk.Advance(5 * time.Second)
	waitFor(t, "待ち2本", func() bool { return h.clk.pending() == 2 })
	h.clk.Advance(time.Second)
	waitFor(t, "B の入れ直し", func() bool { return len(h.aux.setCalls()) == 1 })
	if c := h.aux.setCalls()[0]; c.UUID != "u2" {
		t.Fatalf("入れ直したのは %s", c.UUID)
	}
}
