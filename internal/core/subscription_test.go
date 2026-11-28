package core

import (
	"context"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

const subscriptionTimeout = 3 * time.Second

func waitValue[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value, ok := <-ch:
		if !ok {
			t.Fatal("チャネルが早く閉じた")
		}
		return value
	case <-time.After(subscriptionTimeout):
		t.Fatal("チャネルの受信が時間切れ")
		var zero T
		return zero
	}
}

func waitClosed[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	timer := time.NewTimer(subscriptionTimeout)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("チャネルが時間内に閉じなかった")
		}
	}
}

func waitSnap(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	waitValue(t, ch)
}

func waitDone(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(subscriptionTimeout):
		t.Fatal("完了通知が時間切れ")
	}
}

func assertNoValue[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("チャネルが閉じている")
		}
		t.Fatal("予期しない値を受信した")
	case <-time.After(time.Second):
	}
}

func TestEventSubscriptionSampleAndOverflow(t *testing.T) {
	h := newHarness(t)
	cam2 := h.core.SubscribeEvents(2)
	h.start()
	base := h.clk.Now()
	for i := 1; i <= 4; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		h.sendSet(ingest.SampleSet{At: at, Cameras: []ingest.Sample{{Camera: mainCam, At: at, Interval: time.Second, Online: true, Protocol: "rtmp", Kbps: float64(i * 1000)}}})
	}
	h.command(switcher.CmdStandbyOff) // 4件目まで中核ループが処理したことを同期する。
	if got := waitValue(t, cam2.C).(SampleEvent).Cameras[0].Kbps; got != 1000 {
		t.Fatalf("first kbps=%v", got)
	}
	at := base.Add(5 * time.Second)
	h.sendSet(ingest.SampleSet{At: at, Cameras: []ingest.Sample{{Camera: mainCam, At: at, Interval: time.Second, Online: true, Protocol: "rtmp", Kbps: 5000}}})
	h.command(switcher.CmdStandbyOff)
	if got := waitValue(t, cam2.C).(SampleEvent).Cameras[0].Kbps; got != 2000 {
		t.Fatalf("second kbps=%v", got)
	}
	drain := waitValue(t, cam2.C)
	if d, ok := drain.(DroppedEvent); !ok || d.N != 2 {
		t.Fatalf("dropped=%#v", drain)
	}
	at = base.Add(6 * time.Second)
	h.sendSet(ingest.SampleSet{At: at, Cameras: []ingest.Sample{{Camera: mainCam, At: at, Interval: time.Second, Online: true, Protocol: "rtmp", Kbps: 6000}}})
	h.command(switcher.CmdStandbyOff)
	if d := waitValue(t, cam2.C).(DroppedEvent); d.N != 1 {
		t.Fatalf("third dropped=%+v", d)
	}
	if got := waitValue(t, cam2.C).(SampleEvent).Cameras[0].Kbps; got != 6000 {
		t.Fatalf("last kbps=%v", got)
	}
}

func TestEventSubscriptionClosesWithRun(t *testing.T) {
	h := newHarness(t)
	cam2 := h.core.SubscribeEvents(1)
	snap := h.core.SubscribeSnapshots()
	h.start()
	for i := 0; i < 4; i++ {
		h.sample(1000)
	}
	h.command(switcher.CmdStandbyOff) // 最後の Sample まで処理してから停止する。
	h.cancel()
	waitDone(t, h.runDone)
	waitClosed(t, cam2.C)
	waitClosed(t, snap)
	if got := cam2.Dropped(); got != 3 {
		t.Fatalf("dropped=%d", got)
	}
}

func TestSnapshotSubscriptionCoalesces(t *testing.T) {
	h := newHarness(t)
	cam2 := h.core.SubscribeSnapshots()
	if cap(cam2) != 1 {
		t.Fatalf("snapshot capacity=%d, want 1", cap(cam2))
	}
	select {
	case <-cam2:
		t.Fatal("New 前の publish の知らせが届いた")
	default:
	}
	h.start()
	h.sample(1000)
	h.command(switcher.CmdStandbyOff)
	waitSnap(t, cam2)
	h.sample(1000)
	h.sample(1000)
	h.command(switcher.CmdStandbyOff)
	waitSnap(t, cam2)
	select {
	case <-cam2:
		t.Fatal("容量1のチャネルに余分な知らせが残っている")
	default:
	}
}

func TestSubscriptionsWithoutRunStayOpen(t *testing.T) {
	h := newHarness(t)
	cam2 := h.core.SubscribeSnapshots()
	events := h.core.SubscribeEvents(1)
	assertNoValue(t, cam2)
	assertNoValue(t, events.C)
}

func TestSubscribeAfterRunPanicsAndZeroCapacityPanics(t *testing.T) {
	h := newHarness(t)
	cam2 := h.core.SubscribeEvents(1)
	if cam2 == nil {
		t.Fatal("nil subscription")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("容量0で panic しない")
			}
		}()
		h.core.SubscribeEvents(0)
	}()
	h.start()
	h.cancel()
	waitDone(t, h.runDone)
	for name, subscribe := range map[string]func(){
		"SubscribeSnapshots": func() { h.core.SubscribeSnapshots() },
		"SubscribeEvents":    func() { h.core.SubscribeEvents(1) },
	} {
		name, subscribe := name, subscribe
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Run 後の登録で panic しない")
				}
			}()
			subscribe()
		})
	}
}

func TestSampleEventValues(t *testing.T) {
	h := newHarness(t)
	cam2 := h.core.SubscribeEvents(2)
	h.start()
	at := h.clk.Advance(time.Second)
	h.sendSet(ingest.SampleSet{At: at, Cameras: []ingest.Sample{{Camera: mainCam, At: at, Interval: 1500 * time.Millisecond, Online: true, Protocol: "srt", Kbps: 1234.5, SRT: &ingest.SRTStats{RTTMs: 9, LossRate: 2, RecvMbps: 3}, Err: context.Canceled}}})
	ev := waitValue(t, cam2.C).(SampleEvent)
	if ev.At != at || len(ev.Cameras) != 1 || ev.Cameras[0].ID != mainCam || ev.Cameras[0].IVMs != 1500 || ev.Cameras[0].State != "DOWN" || !ev.Cameras[0].Err || ev.Cameras[0].RTTMs == nil || *ev.Cameras[0].RTTMs != 9 || ev.Cameras[0].Loss == nil || *ev.Cameras[0].Loss != 2 || ev.Cameras[0].Mbps == nil || *ev.Cameras[0].Mbps != 3 {
		t.Fatalf("sample event=%+v", ev)
	}
	at = at.Add(time.Second)
	h.sendSet(ingest.SampleSet{At: at, Cameras: []ingest.Sample{{Camera: mainCam, At: at, Interval: time.Second, Online: true, Protocol: "rtmp", SRT: &ingest.SRTStats{RTTMs: 9, LossRate: 2, RecvMbps: 3}}}})
	rtmp := waitValue(t, cam2.C).(SampleEvent)
	if len(rtmp.Cameras) != 1 || rtmp.Cameras[0].RTTMs != nil || rtmp.Cameras[0].Loss != nil || rtmp.Cameras[0].Mbps != nil {
		t.Fatalf("RTMP event has SRT values: %+v", rtmp.Cameras)
	}
}

func TestSlowSubscribersDoNotBlockCore(t *testing.T) {
	h := newHarness(t)
	h.core.SubscribeEvents(1)
	h.core.SubscribeSnapshots()
	h.start()
	for i := 0; i < 40; i++ {
		h.sample(float64(1000 + i))
		h.command(switcher.CmdStandbyOff)
	}
}

func TestSampleEventTwoCamerasAndJudgeStates(t *testing.T) {
	h := newHarness(t, func(_ *switcher.Config, d *Deps) {
		d.Cameras = []Camera{testCamera(mainCam, "SRT", scenemap.MotionStandby), testCamera(subCam, "RTMP", scenemap.MotionStandby)}
	})
	cam2 := h.core.SubscribeEvents(16)
	h.start()
	base := h.clk.Now()
	send := func(i int, kbps float64, online, failed bool) SampleEvent {
		at := base.Add(time.Duration(i) * time.Second)
		h.sendSet(ingest.SampleSet{At: at, Cameras: []ingest.Sample{
			{Camera: mainCam, At: at, Interval: time.Second, Online: online, Protocol: "srt", Kbps: kbps, SRT: &ingest.SRTStats{RTTMs: 12, LossRate: 1.5, RecvMbps: 2.5}},
			{Camera: subCam, At: at, Interval: time.Second, Online: online, Protocol: "rtmp", Kbps: kbps, Err: func() error {
				if failed {
					return context.Canceled
				}
				return nil
			}()},
		}})
		return waitValue(t, cam2.C).(SampleEvent)
	}
	for i := 1; i <= 2; i++ {
		kbps, online, failed, want := 0.0, false, true, "DOWN"
		if i == 2 {
			kbps, online, failed, want = 500, true, false, "LOW"
		}
		ev := send(i, kbps, online, failed)
		if ev.Time() != base.Add(time.Duration(i)*time.Second) || len(ev.Cameras) != 2 || ev.Cameras[0].ID != mainCam || ev.Cameras[1].ID != subCam || ev.Cameras[0].State != want || ev.Cameras[1].State != want {
			t.Fatalf("LOW transition event %d: %+v", i, ev)
		}
		if ev.Cameras[0].IVMs != 1000 || ev.Cameras[0].Kbps != kbps || ev.Cameras[0].Online != online || ev.Cameras[0].Protocol != "srt" || ev.Cameras[0].RTTMs == nil || *ev.Cameras[0].RTTMs != 12 || ev.Cameras[0].Loss == nil || *ev.Cameras[0].Loss != 1.5 || ev.Cameras[0].Mbps == nil || *ev.Cameras[0].Mbps != 2.5 || ev.Cameras[0].Err {
			t.Fatalf("SRT values: %+v", ev.Cameras[0])
		}
		if ev.Cameras[1].RTTMs != nil || ev.Cameras[1].Loss != nil || ev.Cameras[1].Mbps != nil || ev.Cameras[1].Err != failed {
			t.Fatalf("RTMP values: %+v", ev.Cameras[1])
		}
	}
	for i := 3; i <= 7; i++ {
		ev := send(i, 3000, true, false)
		want := "LOW"
		if i == 7 {
			want = "OK"
		}
		if ev.Cameras[0].State != want || ev.Cameras[1].State != want {
			t.Fatalf("OK transition event %d: %+v", i, ev.Cameras)
		}
	}
}
