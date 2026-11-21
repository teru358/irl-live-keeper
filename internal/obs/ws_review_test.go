package obs

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 要求 A の応答待ちの間に、要求 B が gate 待ちになり、そのあとコレクションの切替が始まったら、
// A が正常に終わっても B は相手に送らない。
func TestQueuedRequestIsNotSentAfterGateCloses(t *testing.T) {
	h := newWSHarness(t, func(w *WS) { w.timeout = 3 * time.Second })
	first := true
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "SetCurrentProgramScene" && first {
			first = false
			return auxReply{Delay: 600 * time.Millisecond}, true
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	aErr, bErr := make(chan error, 1), make(chan error, 1)
	go func() { aErr <- h.w.SetScene("A") }()
	waitFor(t, "A が届く", func() bool { return len(h.s.requestsOf("SetCurrentProgramScene")) == 1 })
	go func() { bErr <- h.w.SetScene("B") }()
	time.Sleep(100 * time.Millisecond) // B は gate 待ち
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	if err := <-aErr; err != nil {
		t.Fatalf("A は正常に終わるはず: %v", err)
	}
	err := <-bErr
	if !errors.Is(err, errSwitching) && !errors.Is(err, errConnDropped) {
		t.Fatalf("B は送らずに失敗するはずが %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if n := len(h.s.requestsOf("SetCurrentProgramScene")); n != 1 {
		t.Fatalf("切替の間に B が相手に届いた: %d 本", n)
	}
}

func sendItemEvents(h *wsHarness, from, to int) {
	for i := from; i <= to; i++ {
		h.s.sendEvent(1, "SceneItemEnableStateChanged", map[string]any{"sceneUuid": "uuid-live", "sceneItemId": i, "sceneItemEnabled": i%2 == 0})
	}
}

// core が詰まって Events を読まなくても、goobs のイベントの列（100）と w.events（64）を超えるイベントを
// 失わない。最後の ItemEnabledChanged と、そのあとの切替の知らせも順に届く。
func TestEventsAreNotLostWhenConsumerStalls(t *testing.T) {
	h := newMapHarness(t, nil)
	h.awaitConnected(1)
	h.paused.Store(true)
	sendItemEvents(h, 1, 400)
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	time.Sleep(300 * time.Millisecond)
	h.paused.Store(false)
	evs := h.awaitDisconnected(1)
	var ids []int
	for _, e := range evs {
		if it, ok := e.(switcher.ItemEnabledChanged); ok {
			ids = append(ids, it.ItemID)
		}
	}
	if len(ids) != 400 {
		t.Fatalf("ItemEnabledChanged は 400 件のはずが %d 件", len(ids))
	}
	for i, id := range ids {
		if id != i+1 {
			t.Fatalf("順が崩れた: %d 番目が %d", i, id)
		}
	}
}

func hookWithData(f *fakeOBS, typ string, match func(map[string]any) bool, rep auxReply) {
	f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type != typ {
			return auxReply{}, false
		}
		var d map[string]any
		_ = json.Unmarshal(r.Data, &d)
		if match == nil || match(d) {
			return rep, true
		}
		return auxReply{}, false
	})
}

// 読み取りの応答より前に届いたイベントは、OBSConnected のあとに流さない。応答のあとに届いたものは流す。
func TestHeldSceneEventsOlderThanSnapshotAreDropped(t *testing.T) {
	s := newAuxServer(t)
	f := newFakeOBS()
	f.setScene("A", "uuid-a")
	f.setHook(func(r auxReq) (auxReply, bool) {
		switch r.Type {
		case "GetVersion":
			return auxReply{Delay: 300 * time.Millisecond}, true // この間に A→B→C
		case "GetCurrentSceneTransition":
			return auxReply{Delay: 300 * time.Millisecond}, true // シーンの応答のあとの変化 D
		}
		return auxReply{}, false
	})
	s.setHandler(f.handle)
	h := startWSHarness(t, s, f, func(w *WS) { w.timeout = 3 * time.Second })
	waitFor(t, "GetVersion が届く", func() bool { return f.count("GetVersion") == 1 })
	s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "B", "sceneUuid": "uuid-b"})
	s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "C", "sceneUuid": "uuid-c"})
	f.setScene("C", "uuid-c")
	waitFor(t, "トランジションの読み取りが届く", func() bool { return f.count("GetCurrentSceneTransition") == 1 })
	f.setScene("D", "uuid-d")
	s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "D", "sceneUuid": "uuid-d"})
	h.awaitConnected(1)
	h.await("D が流れる", func(evs []switcher.Event) bool { return countOf[switcher.SceneChanged](evs) >= 1 })
	time.Sleep(100 * time.Millisecond)
	var got []string
	for _, e := range h.events() {
		switch e := e.(type) {
		case switcher.OBSConnected:
			got = append(got, "connected:"+e.Scene)
		case switcher.SceneChanged:
			got = append(got, "scene:"+e.Scene)
		}
	}
	if len(got) != 2 || got[0] != "connected:C" || got[1] != "scene:D" {
		t.Fatalf("イベントの順: %v（connected:C, scene:D のはず）", got)
	}
}

// ItemEnabledChanged も同じ形: そのシーンの項目の一覧の応答より前に届いたものは流さない。
func TestHeldItemEventsOlderThanItemListAreDropped(t *testing.T) {
	h := newMapHarness(t, nil)
	h.awaitConnected(1)
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		var d map[string]any
		_ = json.Unmarshal(r.Data, &d)
		switch {
		case r.Type == "GetStreamStatus":
			return auxReply{Delay: 250 * time.Millisecond}, true // この間に項目1が false→true（世界は true）
		case r.Type == "GetSceneItemList" && d["sceneUuid"] == "uuid-wait":
			return auxReply{Delay: 250 * time.Millisecond}, true // uuid-live の応答のあと
		}
		return auxReply{}, false
	})
	h.s.sendEvent(1, "CurrentSceneCollectionChanged", map[string]any{"sceneCollectionName": "New"})
	waitFor(t, "読み取りが始まる", func() bool { return h.f.count("GetStreamStatus") == 2 })
	h.s.sendEvent(1, "SceneItemEnableStateChanged", map[string]any{"sceneUuid": "uuid-live", "sceneItemId": 1, "sceneItemEnabled": false})
	h.s.sendEvent(1, "SceneItemEnableStateChanged", map[string]any{"sceneUuid": "uuid-live", "sceneItemId": 1, "sceneItemEnabled": true})
	waitFor(t, "uuid-wait の項目の読み取りが届く", func() bool {
		for _, r := range h.s.requestsOf("GetSceneItemList") {
			var d map[string]any
			_ = json.Unmarshal(r.Data, &d)
			if d["sceneUuid"] == "uuid-wait" && r.Conn == 1 && h.f.count("GetSceneItemList") > 2 {
				return true
			}
		}
		return false
	})
	h.s.sendEvent(1, "SceneItemEnableStateChanged", map[string]any{"sceneUuid": "uuid-live", "sceneItemId": 1, "sceneItemEnabled": false})
	h.awaitConnected(2)
	h.await("応答のあとの変化が流れる", func(evs []switcher.Event) bool { return countOf[switcher.ItemEnabledChanged](evs) >= 1 })
	time.Sleep(100 * time.Millisecond)
	var got []bool
	for _, e := range h.events() {
		if it, ok := e.(switcher.ItemEnabledChanged); ok {
			got = append(got, it.Enabled)
		}
	}
	if len(got) != 1 || got[0] {
		t.Fatalf("応答のあとの false だけが流れるはずが %v", got)
	}
}

// 取っておく上限を超えたら、取りこぼした状態で動き続けず、接続を捨てて OBSDisconnected を流し、
// つなぎ直して全部を読み直す。
func TestEventQueueOverflowDiscardsConnectionAndReconnects(t *testing.T) {
	h := newMapHarness(t, func(w *WS) { w.eventQueueMax = 50 })
	h.awaitConnected(1)
	h.paused.Store(true)
	sendItemEvents(h, 1, 400)
	time.Sleep(300 * time.Millisecond)
	h.paused.Store(false)
	h.awaitDisconnected(1)
	h.awaitConnected(2)
	if n := len(h.s.requestsOf("GetStreamStatus")); n < 2 {
		t.Fatalf("つなぎ直して読み直していない: %d", n)
	}
}
