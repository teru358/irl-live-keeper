package core

import (
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

type watchCall struct {
	Camera  string
	Sources []obs.MediaSource
}

// fakeWatcher は MediaWatcher の偽物。block を閉じるまで Watch が戻らないようにもできる（見張りが詰まった想定）。
type fakeWatcher struct {
	watches chan watchCall
	cancels atomic.Int32
	block   chan struct{} // nil なら止まらない

	mu   sync.Mutex
	warn []string
}

func newFakeWatcher() *fakeWatcher { return &fakeWatcher{watches: make(chan watchCall, 100)} }

func (f *fakeWatcher) Watch(camera string, sources []obs.MediaSource) {
	if f.block != nil {
		<-f.block
	}
	f.watches <- watchCall{camera, sources}
}
func (f *fakeWatcher) CancelAll() { f.cancels.Add(1) }
func (f *fakeWatcher) Warnings() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.warn...)
}
func (f *fakeWatcher) setWarnings(w ...string) { f.mu.Lock(); f.warn = w; f.mu.Unlock() }

func withMediaWatch(w MediaWatcher) option {
	return func(_ *switcher.Config, d *Deps) { d.MediaWatch = w }
}

// expectWatch は Watch が1回頼まれるのを待つ。
func (f *fakeWatcher) expectWatch(t *testing.T) watchCall {
	t.Helper()
	select {
	case w := <-f.watches:
		return w
	case <-time.After(wait):
		t.Fatal("見張りが頼まれない")
		return watchCall{}
	}
}

// expectNone は、いま溜まっている頼みがないことを確かめる。
func (f *fakeWatcher) expectNone(t *testing.T, what string) {
	t.Helper()
	select {
	case w := <-f.watches:
		t.Fatalf("%s: 頼まれるはずがない: %+v", what, w)
	case <-time.After(50 * time.Millisecond):
	}
}

var wantIRL = []obs.MediaSource{{UUID: "IRL", Name: "IRL"}}

func TestMediaWatchRequestedOnOnlineRise(t *testing.T) {
	w := newFakeWatcher()
	h := newHarness(t, withMediaWatch(w))
	h.start()
	h.connect(false, standby)
	h.waitSnap("接続", func(s Snapshot) bool { return s.OBS.Connected })
	w.expectNone(t, "届いていないカメラで接続しただけ")

	h.sample(goodKbps) // false → true
	got := w.expectWatch(t)
	if got.Camera != mainCam || !reflect.DeepEqual(got.Sources, wantIRL) {
		t.Fatalf("頼みが違う: %+v", got)
	}
	h.samplesN(3, goodKbps) // true → true
	h.settle()
	w.expectNone(t, "true → true")

	h.sample(0) // true → false
	h.settle()
	w.expectNone(t, "true → false")
	h.sample(goodKbps) // 再び false → true
	if got := w.expectWatch(t); got.Camera != mainCam {
		t.Fatalf("2回目の頼みが違う: %+v", got)
	}
}

func TestMediaWatchRequestedOnOBSConnectedForOnlineCameras(t *testing.T) {
	w := newFakeWatcher()
	h := newHarness(t, withMediaWatch(w), twoCameras)
	h.start()
	h.sampleSet(goodKbps, 0) // cam1 だけ Online。OBS は未接続なので頼まない
	h.settle()
	w.expectNone(t, "未接続")

	h.connect(false, standby)
	got := w.expectWatch(t)
	if got.Camera != mainCam {
		t.Fatalf("Online の cam1 だけ頼むはずが %+v", got)
	}
	w.expectNone(t, "cam2 は Online でない")
}

func TestMediaWatchCancelledOnOBSDisconnected(t *testing.T) {
	w := newFakeWatcher()
	h := newHarness(t, withMediaWatch(w))
	h.start()
	h.connect(false, standby)
	h.obsEvent(switcher.OBSDisconnected{At: h.clk.Now()})
	waitFor(t, func() bool { return w.cancels.Load() >= 1 }, "CancelAll")
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("待っても成り立たない: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// 見張りの部品が詰まっても、中核ループは止まらない。
func TestMediaWatchBlockedDoesNotStallCore(t *testing.T) {
	w := newFakeWatcher()
	w.block = make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(w.block) }) }
	h := newHarness(t, withMediaWatch(w))
	t.Cleanup(release) // Run を止める前に解く
	h.start()
	h.connect(false, standby)
	h.sample(goodKbps) // 頼むが、見張りは詰まっている
	for range 5 {
		h.sample(goodKbps)
	}
	h.waitSnap("OK", linkIs("OK"))
	if err, d := h.command(switcher.CmdStandbyOn); d > time.Second {
		t.Fatalf("Command が詰まった: %v %v", err, d)
	}
	h.settle() // Tick も進む
	// 詰まりが解けると、頼みが届く（同じカメラの頼みは最新の1つにまとまる）。
	release()
	w.expectWatch(t)
}

func TestMediaWatchWarningsMerged(t *testing.T) {
	w := newFakeWatcher()
	h := newHarness(t, withMediaWatch(w), func(_ *switcher.Config, d *Deps) {
		d.WarningMessages = func() []i18n.Msg { return []i18n.Msg{i18n.Text("obs warning")} }
	})
	h.start()
	h.sample(goodKbps)
	h.waitSnap("obs の警告のみ", func(s Snapshot) bool {
		return reflect.DeepEqual(i18n.Strings(i18n.JA, s.OBS.Warnings), []string{"obs warning"})
	})
	w.setWarnings("IRL が映っていない可能性があります。")
	h.sample(goodKbps)
	h.waitSnap("合わせた警告", func(s Snapshot) bool {
		return reflect.DeepEqual(i18n.Strings(i18n.JA, s.OBS.Warnings), []string{"obs warning", "IRL が映っていない可能性があります。"})
	})
}

// ログの kbps は小数1桁に丸める（M-9 の Windows の確認で、長い小数が読みにくかった）。
func TestRoundKbps(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{{2227.8806111059994, 2227.9}, {0, 0}, {1500.04, 1500}, {1500.05, 1500.1}} {
		if got := roundKbps(c.in); got != c.want {
			t.Errorf("roundKbps(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func (f *fakeWatcher) WarningMessages() []i18n.Msg {
	if len(f.Warnings()) == 0 {
		return nil
	}
	return []i18n.Msg{i18n.New("obs.media", "name", "IRL")}
}
