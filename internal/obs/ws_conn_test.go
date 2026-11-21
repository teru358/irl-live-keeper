package obs

import (
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

func TestClassifyErrorMismatchedID(t *testing.T) {
	err := classifyError(errors.New("request SetCurrentProgramScene: mismatched ID: expected response with ID a, but got b"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("ID の合わない応答は ErrTimeout と同じ扱いのはずが %v", err)
	}
}

// 応答の ID がずれたときも、Timeout と同じに Client を捨ててつなぎ直す。
func TestMainMismatchedIDDiscardsClientAndReconnects(t *testing.T) {
	h := newWSHarness(t, nil)
	first := true
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "SetCurrentProgramScene" && first {
			first = false
			return auxReply{ID: "someone-else"}, true
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	if err := h.w.SetScene("A"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("ErrTimeout のはずが %v", err)
	}
	h.awaitDisconnected(1)
	h.awaitConnected(2)
	if err := h.w.SetScene("B"); err != nil {
		t.Fatal(err)
	}
}

// 並行の要求は goobs の中に1本ずつ入る。Timeout した1本目のあと、待っていた2本目は
// 相手に送られない（応答の列がずれた Client に入れない）。
func TestMainQueuedRequestIsNotSentAfterTimeout(t *testing.T) {
	h := newWSHarness(t, nil)
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "SetCurrentProgramScene" {
			return auxReply{Skip: true}, true
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	errs := make(chan error, 2)
	go func() { errs <- h.w.SetScene("A") }()
	waitFor(t, "1本目が届く", func() bool { return len(h.s.requestsOf("SetCurrentProgramScene")) == 1 })
	go func() { errs <- h.w.SetScene("B") }()
	for range 2 {
		select {
		case err := <-errs:
			if err == nil {
				t.Fatal("応答がないのに成功した")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("要求が戻らない")
		}
	}
	if n := len(h.s.requestsOf("SetCurrentProgramScene")); n != 1 {
		t.Fatalf("2本目が相手に送られた: %d 本", n)
	}
	h.awaitConnected(2)
}

func TestMainEventsCarrySceneUUID(t *testing.T) {
	h := newWSHarness(t, nil)
	evs := h.awaitConnected(1)
	c, _ := lastOf[switcher.OBSConnected](evs)
	if c.Scene != "Live" || c.SceneUUID != "uuid-live" {
		t.Fatalf("OBSConnected: %+v", c)
	}
	h.s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "B", "sceneUuid": "uuid-b"})
	evs = h.await("SceneChanged", func(evs []switcher.Event) bool { return countOf[switcher.SceneChanged](evs) == 1 })
	sc, _ := lastOf[switcher.SceneChanged](evs)
	if sc.Scene != "B" || sc.SceneUUID != "uuid-b" {
		t.Fatalf("SceneChanged: %+v", sc)
	}
}

// トランジションの取り直しの応答が遅くても、ほかのイベントは流れ続ける。
func TestTransitionRefetchDoesNotBlockEvents(t *testing.T) {
	h := newWSHarness(t, func(w *WS) { w.timeout = 3 * time.Second })
	calls := 0
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "GetCurrentSceneTransition" {
			calls++
			if calls == 2 { // 1回目は接続のとき。2回目が取り直し
				return auxReply{Delay: 1200 * time.Millisecond, Data: `{"transitionFixed":false,"transitionDuration":777}`}, true
			}
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	h.s.sendEvent(1, "CurrentSceneTransitionChanged", map[string]any{"transitionName": "Cut"})
	waitFor(t, "取り直しの要求が届く", func() bool { return len(h.s.requestsOf("GetCurrentSceneTransition")) == 2 })
	start := time.Now()
	h.s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "B", "sceneUuid": "uuid-b"})
	h.await("取り直しの応答を待たずに SceneChanged が流れる", func(evs []switcher.Event) bool {
		return countOf[switcher.SceneChanged](evs) == 1
	})
	if el := time.Since(start); el > 800*time.Millisecond {
		t.Fatalf("SceneChanged が取り直しの応答を待った: %v", el)
	}
	if n := countOf[switcher.TransitionChanged](h.events()); n != 0 {
		t.Fatalf("取り直しが終わる前に TransitionChanged が流れた: %d", n)
	}
	evs := h.await("取り直しの結果", func(evs []switcher.Event) bool { return countOf[switcher.TransitionChanged](evs) == 1 })
	tc, _ := lastOf[switcher.TransitionChanged](evs)
	if tc.TransitionMs == nil || *tc.TransitionMs != 777 {
		t.Fatalf("TransitionChanged: %+v", tc)
	}
}

// 取り直しが重なったら、最後の1回だけ（イベントごとに1回ずつ取り直さない）。
func TestTransitionRefetchCoalesces(t *testing.T) {
	h := newWSHarness(t, func(w *WS) { w.timeout = 3 * time.Second })
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "GetCurrentSceneTransition" && h.f.count("GetCurrentSceneTransition") == 2 {
			return auxReply{Delay: 300 * time.Millisecond}, true // 取り直しの1回目を遅らせる
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	h.s.sendEvent(1, "CurrentSceneTransitionChanged", map[string]any{"transitionName": "A"})
	waitFor(t, "1回目の取り直しが届く", func() bool { return len(h.s.requestsOf("GetCurrentSceneTransition")) == 2 })
	for range 5 {
		h.s.sendEvent(1, "CurrentSceneTransitionChanged", map[string]any{"transitionName": "B"})
	}
	h.f.setTransMs(555)
	h.await("取り直しが終わる", func(evs []switcher.Event) bool { return countOf[switcher.TransitionChanged](evs) >= 2 })
	time.Sleep(300 * time.Millisecond)
	if n := len(h.s.requestsOf("GetCurrentSceneTransition")); n != 3 {
		t.Fatalf("重なった5回は、最後に1回だけ取り直すはず: 接続のとき + 2回のはずが全部で %d 回", n)
	}
	tc, _ := lastOf[switcher.TransitionChanged](h.events())
	if tc.TransitionMs == nil || *tc.TransitionMs != 555 {
		t.Fatalf("最後の取り直しの値のはず: %+v", tc)
	}
}

// mainRequests は、本来の接続（1本目）に届いた要求の数。
func mainRequests(s *auxServer) int {
	n := 0
	for _, r := range s.requests() {
		if r.Conn == 1 {
			n++
		}
	}
	return n
}

func TestCollectionChangeClosesGateAndReconnects(t *testing.T) {
	h := newWSHarness(t, nil)
	h.awaitConnected(1)
	waitFor(t, "補助の接続が開く", func() bool { return h.s.accepted() == 2 })

	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	if n := h.s.waitClosed(t, "切替の間は補助の接続も閉じる"); n != 2 {
		t.Fatalf("閉じられたのは補助の接続（2本目）のはずが %d 本目", n)
	}
	before := mainRequests(h.s)
	if err := h.w.SetScene("X"); !errors.Is(err, errSwitching) {
		t.Fatalf("門が閉じている間は errSwitching のはずが %v", err)
	}
	if err := h.w.RestartMedia("Cam"); !errors.Is(err, errSwitching) {
		t.Fatalf("RestartMedia: %v", err)
	}
	if _, _, err := h.w.SceneTransitionOverride("X"); !errors.Is(err, errSwitching) {
		t.Fatalf("SceneTransitionOverride: %v", err)
	}
	if err := h.w.StartStream(); !errors.Is(err, errSwitching) {
		t.Fatalf("StartStream: %v", err)
	}
	// トランジションのイベントが来ても、切替の間は取り直さない。
	h.s.sendEvent(1, "CurrentSceneTransitionChanged", map[string]any{"transitionName": "Cut"})
	time.Sleep(250 * time.Millisecond)
	if n := mainRequests(h.s); n != before {
		t.Fatalf("門が閉じている間に要求を送った: %d 本", n-before)
	}
	if n := countOf[switcher.OBSConnected](h.events()); n != 1 {
		t.Fatalf("Changed の前に OBSConnected が流れた: %d", n)
	}
	if !slices.ContainsFunc(h.w.Warnings(), func(s string) bool { return strings.Contains(s, "切り替え中") }) {
		t.Fatalf("切替中の警告がない: %v", h.w.Warnings())
	}

	h.f.setScene("Second", "uuid-2")
	h.s.sendEvent(1, "CurrentSceneCollectionChanged", map[string]any{"sceneCollectionName": "New"})
	evs := h.awaitConnected(2)
	c, _ := lastOf[switcher.OBSConnected](evs)
	if c.Scene != "Second" || c.SceneUUID != "uuid-2" {
		t.Fatalf("つなぎ直しの OBSConnected は新しい読み取りのはず: %+v", c)
	}
	if n := h.s.accepted(); n < 3 {
		waitFor(t, "補助の接続が開き直る", func() bool { return h.s.accepted() == 3 })
	}
	if got := h.s.accepted(); got != 3 {
		t.Fatalf("websocket はつないだまま（本来 + 補助の開き直し）のはずが %d 本", got)
	}
	if err := h.w.SetScene("Second"); err != nil {
		t.Fatalf("門が開いたあとの SetScene: %v", err)
	}
	if slices.ContainsFunc(h.w.Warnings(), func(s string) bool { return strings.Contains(s, "切り替え中") }) {
		t.Fatalf("つなぎ直したのに切替中の警告が残っている: %v", h.w.Warnings())
	}
	if n := countOf[switcher.OBSDisconnected](h.events()); n != 1 {
		t.Fatalf("OBSDisconnected は1回のはずが %d", n)
	}
}

// 切替の間に届いたイベントのうち、Changed のあとのものは OBSConnected のあとに流す。それ以前のものは捨てる。
func TestCollectionChangeHoldsEventsAfterChanged(t *testing.T) {
	h := newWSHarness(t, nil)
	h.awaitConnected(1)
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	h.s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "Junk", "sceneUuid": "uuid-junk"})
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "GetCurrentSceneTransition" {
			return auxReply{Delay: 150 * time.Millisecond}, true // シーンを読んだあと、読み取りの間に次のイベントが来る
		}
		return auxReply{}, false
	})
	h.f.setScene("Second", "uuid-2")
	h.s.sendEvent(1, "CurrentSceneCollectionChanged", map[string]any{"sceneCollectionName": "New"})
	waitFor(t, "シーンの読み取りのあとのトランジションの読み取りが届く", func() bool { return h.f.count("GetCurrentSceneTransition") == 2 })
	h.s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "Third", "sceneUuid": "uuid-3"})
	evs := h.await("つなぎ直したあとの SceneChanged", func(evs []switcher.Event) bool {
		return countOf[switcher.OBSConnected](evs) == 2 && countOf[switcher.SceneChanged](evs) >= 1
	})
	time.Sleep(100 * time.Millisecond)
	evs = h.events()
	var names []string
	for _, e := range evs {
		switch e := e.(type) {
		case switcher.OBSConnected:
			names = append(names, "connected:"+e.Scene)
		case switcher.OBSDisconnected:
			names = append(names, "disconnected")
		case switcher.SceneChanged:
			names = append(names, "scene:"+e.Scene)
		}
	}
	want := []string{"connected:Live", "disconnected", "connected:Second", "scene:Third"}
	if !slices.Equal(names, want) {
		t.Fatalf("イベントの順: %v（期待 %v）", names, want)
	}
}

func TestSceneRemovedOfProgramSceneActsLikeChanging(t *testing.T) {
	h := newWSHarness(t, nil)
	h.awaitConnected(1)

	// 今の program でないシーンの削除は、何も起こさない。
	h.s.sendEvent(1, "SceneRemoved", map[string]any{"sceneName": "Other", "sceneUuid": "uuid-other", "isGroup": false})
	time.Sleep(200 * time.Millisecond)
	if n := countOf[switcher.OBSDisconnected](h.events()); n != 0 {
		t.Fatalf("ほかのシーンの削除で切断になった: %d", n)
	}

	h.s.sendEvent(1, "SceneRemoved", map[string]any{"sceneName": "Live", "sceneUuid": "uuid-live", "isGroup": false})
	h.awaitDisconnected(1)
	before := mainRequests(h.s)
	if err := h.w.SetScene("X"); !errors.Is(err, errSwitching) {
		t.Fatalf("errSwitching のはずが %v", err)
	}
	if n := mainRequests(h.s); n != before {
		t.Fatal("門が閉じているのに要求を送った")
	}
	h.f.setScene("Copy", "uuid-copy")
	h.s.sendEvent(1, "CurrentSceneCollectionChanged", map[string]any{"sceneCollectionName": "Copy"})
	evs := h.awaitConnected(2)
	if c, _ := lastOf[switcher.OBSConnected](evs); c.SceneUUID != "uuid-copy" {
		t.Fatalf("OBSConnected: %+v", c)
	}
	if err := h.w.SetScene("Copy"); err != nil {
		t.Fatal(err)
	}
	// Changed が来たあとの、同じシーンコレクションの Changing なしの Changed は二重に切断しない。
	if n := countOf[switcher.OBSDisconnected](h.events()); n != 1 {
		t.Fatalf("OBSDisconnected は1回のはず: %d", n)
	}
}

// program のシーンの SceneRemoved のあと、イベントが2秒途切れたら、Changed が来なくても読み直す。
func TestSceneRemovedReconnectsAfterQuietPeriod(t *testing.T) {
	clk := newFakeClock()
	h := newWSHarness(t, func(w *WS) { w.clock = clk })
	h.awaitConnected(1)
	h.s.sendEvent(1, "SceneRemoved", map[string]any{"sceneName": "Live", "sceneUuid": "uuid-live"})
	h.awaitDisconnected(1)
	waitFor(t, "途切れの待ちが置かれる", func() bool { return clk.pending() >= 1 })

	clk.Advance(1500 * time.Millisecond)
	// 途中でイベントが来たら、2秒は数え直す。
	p := clk.pending()
	h.s.sendEvent(1, "SceneRemoved", map[string]any{"sceneName": "Next", "sceneUuid": "uuid-next"})
	waitFor(t, "数え直しの待ちが置かれる", func() bool { return clk.pending() > p })
	clk.Advance(1 * time.Second) // 最初の起点から 2.5 秒。数え直しの起点から 1 秒
	time.Sleep(150 * time.Millisecond)
	if n := countOf[switcher.OBSConnected](h.events()); n != 1 {
		t.Fatalf("途切れが2秒に足りないのに読み直した")
	}
	h.f.setScene("After", "uuid-after")
	clk.Advance(1500 * time.Millisecond)
	evs := h.awaitConnected(2)
	if c, _ := lastOf[switcher.OBSConnected](evs); c.Scene != "After" {
		t.Fatalf("OBSConnected: %+v", c)
	}
}

// Changing のあと Changed が来ないまま長く経ったら、読み直す（切断のままにしない）。
func TestChangingWithoutChangedReconnectsAfterLongQuiet(t *testing.T) {
	clk := newFakeClock()
	h := newWSHarness(t, func(w *WS) { w.clock = clk })
	h.awaitConnected(1)
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	waitFor(t, "待ちが置かれる", func() bool { return clk.pending() >= 1 })
	clk.Advance(5 * time.Second)
	time.Sleep(150 * time.Millisecond)
	if n := countOf[switcher.OBSConnected](h.events()); n != 1 {
		t.Fatal("Changing のあとの2秒では読み直さない（切替は長くかかる）")
	}
	clk.Advance(30 * time.Second)
	h.awaitConnected(2)
}

func TestIsLegacyVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"5.3.0": true, "5.3.3": true, "5.3.99": true, "4.9.1": true, "5.0.0": true,
		"5.4.0": false, "5.4.1": false, "5.5.0": false, "5.7.6": false, "6.0.0": false, "5.10.0": false,
		"5.4.0-beta1": false, "5.3.9-rc1": true,
		"": true, "garbage": true, "5": true,
	} {
		if got := isLegacyVersion(v); got != want {
			t.Errorf("isLegacyVersion(%q) = %v のはずが %v", v, want, got)
		}
	}
}

func hasWarning(w *WS, sub string) bool {
	return slices.ContainsFunc(w.Warnings(), func(s string) bool { return strings.Contains(s, sub) })
}

const legacyWarningText = "OBS が古いため、自動の切替と隠す・戻すを止めています"

func TestLegacyVersionUsesNamesAsUUIDs(t *testing.T) {
	s := newAuxServer(t)
	f := newFakeOBS()
	f.version = "5.3.0"
	s.setHandler(f.handle)
	h := startWSHarness(t, s, f, nil)
	evs := h.awaitConnected(1)
	c, _ := lastOf[switcher.OBSConnected](evs)
	if c.Scene != "Live" || c.SceneUUID != "Live" {
		t.Fatalf("古い版では SceneUUID に名前を入れる: %+v", c)
	}
	if !hasWarning(h.w, legacyWarningText) {
		t.Fatalf("古い版の警告がない: %v", h.w.Warnings())
	}
	h.s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "B", "sceneUuid": "ignored"})
	h.await("SceneChanged", func(evs []switcher.Event) bool { return countOf[switcher.SceneChanged](evs) == 1 })
	sc, _ := lastOf[switcher.SceneChanged](h.events())
	if sc.Scene != "B" || sc.SceneUUID != "B" {
		t.Fatalf("SceneChanged: %+v", sc)
	}

	if err := h.w.SetSceneUUID("B"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.w.SceneTransitionOverrideUUID("B"); err != nil {
		t.Fatal(err)
	}
	if err := h.w.RestartMediaInputs([]string{"Cam"}); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"SetCurrentProgramScene", "GetSceneSceneTransitionOverride"} {
		r := h.s.requestsOf(typ)
		if len(r) != 1 {
			t.Fatalf("%s: %d 本", typ, len(r))
		}
		d := mustJSON(t, r[0].Data)
		if d["sceneName"] != "B" || d["sceneUuid"] != nil {
			t.Errorf("%s: 古い版は名前で送る: %s", typ, r[0].Data)
		}
	}
	r := h.s.requestsOf("TriggerMediaInputAction")
	if len(r) != 1 {
		t.Fatalf("TriggerMediaInputAction: %d 本", len(r))
	}
	if d := mustJSON(t, r[0].Data); d["inputName"] != "Cam" || d["inputUuid"] != nil {
		t.Errorf("古い版は名前で送る: %s", r[0].Data)
	}
}

func TestNewVersionSendsUUIDs(t *testing.T) {
	for _, v := range []string{"5.4.0", "5.7.6"} {
		t.Run(v, func(t *testing.T) {
			s := newAuxServer(t)
			f := newFakeOBS()
			f.version = v
			s.setHandler(f.handle)
			h := startWSHarness(t, s, f, nil)
			h.awaitConnected(1)
			if hasWarning(h.w, legacyWarningText) {
				t.Fatalf("%s は古くない: %v", v, h.w.Warnings())
			}
			if err := h.w.SetSceneUUID("uuid-b"); err != nil {
				t.Fatal(err)
			}
			if _, _, err := h.w.SceneTransitionOverrideUUID("uuid-b"); err != nil {
				t.Fatal(err)
			}
			if err := h.w.RestartMediaInputs([]string{"in-1", "in-2"}); err != nil {
				t.Fatal(err)
			}
			for _, typ := range []string{"SetCurrentProgramScene", "GetSceneSceneTransitionOverride"} {
				r := h.s.requestsOf(typ)
				if len(r) != 1 {
					t.Fatalf("%s: %d 本", typ, len(r))
				}
				d := mustJSON(t, r[0].Data)
				if d["sceneUuid"] != "uuid-b" || d["sceneName"] != nil {
					t.Errorf("%s: UUID で送る: %s", typ, r[0].Data)
				}
			}
			r := h.s.requestsOf("TriggerMediaInputAction")
			if len(r) != 2 {
				t.Fatalf("TriggerMediaInputAction: %d 本", len(r))
			}
			for i, want := range []string{"in-1", "in-2"} {
				d := mustJSON(t, r[i].Data)
				if d["inputUuid"] != want || d["inputName"] != nil || d["mediaAction"] != "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART" {
					t.Errorf("%d 本目: %s", i, r[i].Data)
				}
			}
		})
	}
}

// 名前で送る今のメソッドは、そのまま名前で送る。
func TestNameRequestsStillSendNames(t *testing.T) {
	h := newWSHarness(t, nil)
	h.awaitConnected(1)
	if err := h.w.SetScene("B"); err != nil {
		t.Fatal(err)
	}
	if err := h.w.RestartMedia("Cam"); err != nil {
		t.Fatal(err)
	}
	if d := mustJSON(t, h.s.requestsOf("SetCurrentProgramScene")[0].Data); d["sceneName"] != "B" || d["sceneUuid"] != nil {
		t.Errorf("SetScene は名前で送る: %v", d)
	}
	if d := mustJSON(t, h.s.requestsOf("TriggerMediaInputAction")[0].Data); d["inputName"] != "Cam" {
		t.Errorf("RestartMedia は名前で送る: %v", d)
	}
}

// 補助の接続は、OBSConnected のときに開き、OBSDisconnected で閉じる。
func TestAuxFollowsMainConnection(t *testing.T) {
	h := newWSHarness(t, nil)
	if n := h.s.accepted(); n > 1 {
		t.Fatalf("OBSConnected の前に補助の接続が開いた")
	}
	h.awaitConnected(1)
	waitFor(t, "補助の接続が開く", func() bool { return h.s.accepted() == 2 })
	h.s.kill(1) // OBS から本来の接続が切られた
	h.awaitDisconnected(1)
	h.s.waitClosed(t, "切れたら補助も閉じる")
	h.awaitConnected(2)
	waitFor(t, "補助の接続が開き直る", func() bool { return h.s.accepted() == 4 })
}

// Run を止めたら、goroutine も接続も残らない。
func TestRunLeavesNothingBehind(t *testing.T) {
	s := newAuxServer(t)
	f := newFakeOBS()
	s.setHandler(f.handle)
	runtime.GC()
	base := runtime.NumGoroutine()
	h := startWSHarness(t, s, f, nil)
	h.awaitConnected(1)
	waitFor(t, "補助の接続が開く", func() bool { return s.accepted() == 2 })
	// 切替と、Timeout からのつなぎ直しを経ておく。
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	h.s.sendEvent(1, "CurrentSceneCollectionChanged", map[string]any{"sceneCollectionName": "New"})
	h.awaitConnected(2)
	h.s.sendEvent(1, "CurrentSceneTransitionChanged", map[string]any{"transitionName": "Cut"})
	first := true
	f.hook = func(r auxReq) (auxReply, bool) {
		if r.Type == "SetCurrentProgramScene" && first {
			first = false
			return auxReply{Skip: true}, true
		}
		return auxReply{}, false
	}
	if err := h.w.SetScene("A"); !errors.Is(err, ErrTimeout) {
		t.Fatal(err)
	}
	h.awaitConnected(3)
	waitFor(t, "補助の接続が開き直る", func() bool { return s.accepted() >= 5 })

	h.stop()
	n := s.accepted()
	for range n {
		s.waitClosed(t, "Run を止めたあと")
	}
	waitFor(t, "goroutine が基準まで戻る", func() bool { return runtime.NumGoroutine() <= base })
}

// goobs の中で止まった Client（受け手のいない応答で handleOpcodes が止まり、要求は送る段階で戻らない。
// goobs 自身の応答待ちの期限も効かない）でも、呼び出し側の期限で session が終わり、つなぎ直し続け、
// Run は上限内に止まる。止まった goobs の goroutine は外から戻せないので、このテストのあとに残る。
func TestStuckGoobsDoesNotStallRunAndEveryConnectionIsClosed(t *testing.T) {
	s := newAuxServer(t)
	s.ghost = true
	f := newFakeOBS()
	s.setHandler(f.handle)
	h := startWSHarness(t, s, f, func(w *WS) { w.backoffMin, w.backoffMax = 20*time.Millisecond, 40*time.Millisecond })

	waitFor(t, "止まった Client を捨てて、3回つなぎ直す", func() bool { return s.accepted() >= 3 })
	if n := countOf[switcher.OBSConnected](h.events()); n != 0 {
		t.Fatalf("止まっているのに OBSConnected を流した: %d", n)
	}
	start := time.Now()
	h.stop()
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("Run が止まるまでに %v かかった", el)
	}
	n := s.accepted()
	for i := range n {
		select {
		case <-s.closed:
		case <-time.After(3 * time.Second):
			t.Fatalf("接続 %d 本のうち %d 本しか閉じられていない", n, i)
		}
	}
}
