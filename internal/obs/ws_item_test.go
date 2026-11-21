package obs

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

type itemReq struct {
	SceneUUID        *string `json:"sceneUuid"`
	SceneName        *string `json:"sceneName"`
	SceneItemID      int     `json:"sceneItemId"`
	SceneItemEnabled *bool   `json:"sceneItemEnabled"`
}

func lastItemReq(t *testing.T, h *wsHarness) itemReq {
	t.Helper()
	rs := h.s.requestsOf("SetSceneItemEnabled")
	if len(rs) == 0 {
		t.Fatal("SetSceneItemEnabled が届いていない")
	}
	var q itemReq
	if err := json.Unmarshal(rs[len(rs)-1].Data, &q); err != nil {
		t.Fatal(err)
	}
	return q
}

func TestSetSceneItemEnabledSendsUUIDItemAndValue(t *testing.T) {
	h := newWSHarness(t, nil)
	h.awaitConnected(1)
	if err := h.w.SetSceneItemEnabled("uuid-live", 7, false); err != nil {
		t.Fatal(err)
	}
	q := lastItemReq(t, h)
	if q.SceneUUID == nil || *q.SceneUUID != "uuid-live" || q.SceneName != nil || q.SceneItemID != 7 || q.SceneItemEnabled == nil || *q.SceneItemEnabled {
		t.Fatalf("送った内容: %+v", q)
	}
	// false を省略せずに送ること（omitempty で落ちない）と、true も送ること
	if err := h.w.SetSceneItemEnabled("uuid-live", 8, true); err != nil {
		t.Fatal(err)
	}
	q = lastItemReq(t, h)
	if q.SceneItemID != 8 || q.SceneItemEnabled == nil || !*q.SceneItemEnabled {
		t.Fatalf("true: %+v", q)
	}
}

// グループの中の項目は、グループの UUID を sceneUuid に渡せば送れる（グループはシーンとして扱える）。
func TestSetSceneItemEnabledAcceptsGroupUUID(t *testing.T) {
	h := newMapHarness(t, nil)
	h.awaitConnected(1)
	if err := h.w.SetSceneItemEnabled("g1", 1, false); err != nil {
		t.Fatal(err)
	}
	q := lastItemReq(t, h)
	if q.SceneUUID == nil || *q.SceneUUID != "g1" || q.SceneItemID != 1 {
		t.Fatalf("グループの UUID で送られていない: %+v", q)
	}
}

// 古い OBS（5.4.0 未満）では、名前で送る。
func TestSetSceneItemEnabledLegacyUsesName(t *testing.T) {
	s := newAuxServer(t)
	f := newFakeOBS()
	f.version = "5.3.0"
	s.setHandler(f.handle)
	h := startWSHarness(t, s, f, nil)
	h.awaitConnected(1)
	if err := h.w.SetSceneItemEnabled("Live", 2, true); err != nil {
		t.Fatal(err)
	}
	q := lastItemReq(t, h)
	if q.SceneName == nil || *q.SceneName != "Live" || q.SceneUUID != nil || q.SceneItemID != 2 {
		t.Fatalf("古い OBS は名前で送る: %+v", q)
	}
}

func TestSetSceneItemEnabledNotConnectedAndGateClosed(t *testing.T) {
	h := newWSHarness(t, nil)
	if err := h.w.SetSceneItemEnabled("u", 1, false); !errors.Is(err, errNotConnected) {
		t.Fatalf("未接続: %v", err)
	}
	h.awaitConnected(1)
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	if err := h.w.SetSceneItemEnabled("u", 1, false); !errors.Is(err, errSwitching) {
		t.Fatalf("切替中: %v", err)
	}
	if n := len(h.s.requestsOf("SetSceneItemEnabled")); n != 0 {
		t.Fatalf("門が閉じている間に送った: %d", n)
	}
}

// 応答待ちの超過は ErrTimeout で、Client を捨ててつなぎ直す（SetSceneUUID と同じ）。
func TestSetSceneItemEnabledTimeoutDiscardsClient(t *testing.T) {
	h := newWSHarness(t, nil)
	first := true
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "SetSceneItemEnabled" && first {
			first = false
			return auxReply{Skip: true}, true
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	err := h.w.SetSceneItemEnabled("uuid-live", 1, false)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("ErrTimeout のはずが %v", err)
	}
	if got := ClassifyItemResult(err); got != switcher.ItemTimeout {
		t.Fatalf("分類 = %v", got)
	}
	h.awaitDisconnected(1)
	h.awaitConnected(2)
	if err := h.w.SetSceneItemEnabled("uuid-live", 1, false); err != nil {
		t.Fatal(err)
	}
}

// ID の合わない応答も Timeout と同じ。
func TestSetSceneItemEnabledMismatchedIDIsTimeout(t *testing.T) {
	h := newWSHarness(t, nil)
	first := true
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "SetSceneItemEnabled" && first {
			first = false
			return auxReply{ID: "someone-else"}, true
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	err := h.w.SetSceneItemEnabled("uuid-live", 1, false)
	if !errors.Is(err, ErrTimeout) || ClassifyItemResult(err) != switcher.ItemTimeout {
		t.Fatalf("Timeout のはずが %v", err)
	}
	h.awaitDisconnected(1)
	h.awaitConnected(2)
}

// gate: 前の要求が終わらない間は、次の要求を goobs に送らない。
func TestSetSceneItemEnabledQueuedIsNotSentAfterTimeout(t *testing.T) {
	h := newWSHarness(t, nil)
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "SetSceneItemEnabled" {
			return auxReply{Skip: true}, true
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	errs := make(chan error, 2)
	go func() { errs <- h.w.SetSceneItemEnabled("uuid-live", 1, false) }()
	waitFor(t, "1本目が届く", func() bool { return len(h.s.requestsOf("SetSceneItemEnabled")) == 1 })
	go func() { errs <- h.w.SetSceneItemEnabled("uuid-live", 2, false) }()
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
	if n := len(h.s.requestsOf("SetSceneItemEnabled")); n != 1 {
		t.Fatalf("2本目が相手に送られた: %d 本", n)
	}
}

func TestSetSceneItemEnabledStatus600IsNotFoundAndKeepsConnection(t *testing.T) {
	h := newWSHarness(t, nil)
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "SetSceneItemEnabled" {
			var q itemReq
			_ = json.Unmarshal(r.Data, &q)
			switch q.SceneItemID {
			case 1:
				return auxReply{Code: 600}, true
			case 2:
				return auxReply{Code: 400}, true
			}
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	err1 := h.w.SetSceneItemEnabled("uuid-live", 1, false)
	if err1 == nil || ClassifyItemResult(err1) != switcher.ItemNotFound {
		t.Fatalf("600 は NotFound のはずが %v (%v)", err1, ClassifyItemResult(err1))
	}
	err2 := h.w.SetSceneItemEnabled("uuid-live", 2, false)
	if err2 == nil || ClassifyItemResult(err2) != switcher.ItemError {
		t.Fatalf("400 は Error のはずが %v (%v)", err2, ClassifyItemResult(err2))
	}
	// 失敗の応答では接続を捨てない
	if err := h.w.SetSceneItemEnabled("uuid-live", 3, false); err != nil {
		t.Fatal(err)
	}
	if n := countOf[switcher.OBSDisconnected](h.events()); n != 0 {
		t.Fatalf("接続を捨てた: %d", n)
	}
}

func TestClassifyItemResult(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want switcher.ItemResult
	}{
		{"nil", nil, switcher.ItemOK},
		{"ErrTimeout", ErrTimeout, switcher.ItemTimeout},
		{"ErrTimeout を包む", fmt.Errorf("x: %w", ErrTimeout), switcher.ItemTimeout},
		{"未接続", errNotConnected, switcher.ItemError},
		{"切替中", errSwitching, switcher.ItemError},
		{"捨てた", errConnDropped, switcher.ItemError},
		{"ほかの失敗", errors.New("boom"), switcher.ItemError},
		{"文字に 600 を含むだけ", errors.New("request X: foo (6001)"), switcher.ItemError},
	} {
		if got := ClassifyItemResult(c.err); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// ---- Studio Mode ----

func TestOBSConnectedCarriesStudioMode(t *testing.T) {
	for _, want := range []bool{false, true} {
		s := newAuxServer(t)
		f := newFakeOBS()
		f.studio = want
		s.setHandler(f.handle)
		h := startWSHarness(t, s, f, nil)
		c, _ := lastOf[switcher.OBSConnected](h.awaitConnected(1))
		if c.StudioMode != want {
			t.Fatalf("StudioMode = %v, want %v", c.StudioMode, want)
		}
	}
}

// 読めなければ、GetStreamStatus などと同じに、接続の失敗としてつなぎ直す（OBSConnected を流さない）。
func TestStudioModeReadFailureRetriesConnection(t *testing.T) {
	h := newMapHarnessHook(t, func(r auxReq) (auxReply, bool) {
		if r.Type == "GetStudioModeEnabled" && r.Conn == 1 {
			return auxReply{Code: 500}, true
		}
		return auxReply{}, false
	}, nil)
	evs := h.awaitConnected(1)
	if len(h.s.requestsOf("GetStudioModeEnabled")) < 2 {
		t.Fatalf("読み直していない")
	}
	if c, _ := lastOf[switcher.OBSConnected](evs); c.Map == nil {
		t.Fatal("2本目の接続で OBSConnected が流れるはず")
	}
}

func TestStudioModeStateChangedIsEmitted(t *testing.T) {
	h := newWSHarness(t, nil)
	h.awaitConnected(1)
	h.s.sendEvent(1, "StudioModeStateChanged", map[string]any{"studioModeEnabled": true})
	evs := h.await("StudioModeChanged", func(evs []switcher.Event) bool { return countOf[switcher.StudioModeChanged](evs) == 1 })
	e, _ := lastOf[switcher.StudioModeChanged](evs)
	if !e.Enabled || e.At.IsZero() {
		t.Fatalf("StudioModeChanged: %+v", e)
	}
	h.s.sendEvent(1, "StudioModeStateChanged", map[string]any{"studioModeEnabled": false})
	evs = h.await("2つ目", func(evs []switcher.Event) bool { return countOf[switcher.StudioModeChanged](evs) == 2 })
	if e, _ := lastOf[switcher.StudioModeChanged](evs); e.Enabled {
		t.Fatalf("2つ目: %+v", e)
	}
}

// 順序は今のイベントの列の規則どおり: 同じ接続の中では届いた順。
func TestStudioModeChangedKeepsOrderWithSceneChanged(t *testing.T) {
	h := newWSHarness(t, nil)
	h.awaitConnected(1)
	h.s.sendEvent(1, "StudioModeStateChanged", map[string]any{"studioModeEnabled": true})
	h.s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "B", "sceneUuid": "uuid-b"})
	h.s.sendEvent(1, "StudioModeStateChanged", map[string]any{"studioModeEnabled": false})
	h.await("3つ", func(evs []switcher.Event) bool {
		return countOf[switcher.StudioModeChanged](evs) == 2 && countOf[switcher.SceneChanged](evs) == 1
	})
	var order []string
	for _, e := range h.events() {
		switch v := e.(type) {
		case switcher.StudioModeChanged:
			order = append(order, fmt.Sprint("studio=", v.Enabled))
		case switcher.SceneChanged:
			order = append(order, "scene")
		}
	}
	if fmt.Sprint(order) != "[studio=true scene studio=false]" {
		t.Fatalf("順序: %v", order)
	}
}

// 読み取りの最中に届いた変化は、OBSConnected のあとに流す。読んだ値より前の変化は流さない。
func TestStudioModeChangedDuringReloadIsHeld(t *testing.T) {
	h := newWSHarness(t, nil)
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "GetCurrentSceneTransition" {
			return auxReply{Delay: 150 * time.Millisecond}, true
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)
	h.s.sendEvent(1, "CurrentSceneCollectionChanged", map[string]any{"sceneCollectionName": "New"})
	waitFor(t, "読み取りが始まる", func() bool { return h.f.count("GetStudioModeEnabled") == 2 })
	h.f.setStudio(true) // 2回目の読み取りの値は false のまま。そのあとに届いた変化は流れる
	h.s.sendEvent(1, "StudioModeStateChanged", map[string]any{"studioModeEnabled": true})
	h.awaitConnected(2)
	h.await("取っておいた変化が流れる", func(evs []switcher.Event) bool { return countOf[switcher.StudioModeChanged](evs) == 1 })
	var order []string
	for _, e := range h.events() {
		switch e.(type) {
		case switcher.OBSConnected:
			order = append(order, "connected")
		case switcher.OBSDisconnected:
			order = append(order, "disconnected")
		case switcher.StudioModeChanged:
			order = append(order, "studio")
		}
	}
	if fmt.Sprint(order) != "[connected disconnected connected studio]" {
		t.Fatalf("順序: %v", order)
	}
}

// GetStudioModeEnabled の応答より前に届いた StudioModeStateChanged は、読んだ値に含まれるので流さない。
// 応答より後に届いたものは、OBSConnected のあとに流す。
func TestStudioModeChangedBeforeAndAfterSnapshot(t *testing.T) {
	s := newAuxServer(t)
	f := newFakeOBS()
	f.studio = true
	s.setHandler(f.handle)
	f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Conn != 1 {
			return auxReply{}, false
		}
		switch r.Type {
		case "GetStudioModeEnabled":
			// 応答より前にイベントを送る（同じ接続で、イベント、応答の順に届く）
			s.sendEvent(1, "StudioModeStateChanged", map[string]any{"studioModeEnabled": true})
		case "GetCurrentSceneTransition":
			// Studio Mode の応答より後
			s.sendEvent(1, "StudioModeStateChanged", map[string]any{"studioModeEnabled": false})
		}
		return auxReply{}, false
	})
	h := startWSHarness(t, s, f, nil)
	h.awaitConnected(1)
	h.await("応答より後の変化", func(evs []switcher.Event) bool { return countOf[switcher.StudioModeChanged](evs) >= 1 })
	time.Sleep(200 * time.Millisecond) // 余分な変化が流れないことを見る
	var order []string
	for _, e := range h.events() {
		switch v := e.(type) {
		case switcher.OBSConnected:
			order = append(order, fmt.Sprint("connected studio=", v.StudioMode))
		case switcher.StudioModeChanged:
			order = append(order, fmt.Sprint("changed=", v.Enabled))
		}
	}
	if fmt.Sprint(order) != "[connected studio=true changed=false]" {
		t.Fatalf("順序: %v", order)
	}
}
