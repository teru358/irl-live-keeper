package obs

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

var testCameras = []CameraConfig{{ID: "cam1", Name: "メイン"}, {ID: "cam2", Name: "サブ"}}

// mapWorld は地図の読み取りの元（シーン Live にカメラ2つ。cam2 はグループの中）。
func mapWorld(f *fakeOBS) {
	f.update(func(f *fakeOBS) {
		f.collection = "ライブ"
		f.scenes = []fakeScene{
			{UUID: "uuid-live", Name: "Live", Items: []fakeItem{
				{ID: 1, SourceUUID: "in1", SourceName: "Cam1", Kind: "input", Enabled: true},
				{ID: 2, SourceUUID: "g1", SourceName: "Group", Kind: "group", Enabled: true},
			}},
			{UUID: "uuid-wait", Name: "Wait"},
		}
		f.groups = map[string]fakeScene{"g1": {UUID: "g1", Name: "Group", Items: []fakeItem{
			{ID: 1, SourceUUID: "in2", SourceName: "Cam2", Kind: "input", Enabled: true},
		}}}
		f.inputs = []fakeInput{
			rtspInput("in1", "Cam1", "rtsp://127.0.0.1:8554/cam1"),
			rtspInput("in2", "Cam2", "rtsp://127.0.0.1:8554/cam2"),
		}
	})
}

func mapTune(w *WS) {
	w.cfg.Cameras = testCameras
	w.cfg.RTSPPort = 8554
	w.backoffMin, w.backoffMax = 20*time.Millisecond, 40*time.Millisecond
}

// requestsOnConn は n 本目の接続に届いた種類 typ の要求の数。
func requestsOnConn(s *auxServer, conn int, typ string) int {
	n := 0
	for _, r := range s.requests() {
		if r.Conn == conn && r.Type == typ {
			n++
		}
	}
	return n
}

func newMapHarness(t *testing.T, tune func(*WS)) *wsHarness {
	t.Helper()
	return newMapHarnessHook(t, nil, tune)
}

// newMapHarnessHook は、WS を始める前に相手の hook を置く。
func newMapHarnessHook(t *testing.T, hook func(auxReq) (auxReply, bool), tune func(*WS)) *wsHarness {
	t.Helper()
	s := newAuxServer(t)
	f := newFakeOBS()
	mapWorld(f)
	f.setHook(hook)
	s.setHandler(f.handle)
	return startWSHarness(t, s, f, func(w *WS) {
		mapTune(w)
		if tune != nil {
			tune(w)
		}
	})
}

func TestFirstMapIsReadOnMainConnectionAndCarriedByOBSConnected(t *testing.T) {
	h := newMapHarness(t, nil)
	evs := h.awaitConnected(1)
	c, _ := lastOf[switcher.OBSConnected](evs)
	if c.Map == nil {
		t.Fatal("OBSConnected に地図がない")
	}
	if c.Map.Collection != "ライブ" {
		t.Errorf("Collection = %q", c.Map.Collection)
	}
	if got := scenemap.CamerasIn(*c.Map, "uuid-live", nil, true); !slices.Equal(got, []string{"cam1", "cam2"}) {
		t.Errorf("Live に映るカメラ: %v", got)
	}
	if got := scenemap.CamerasIn(*c.Map, "uuid-wait", nil, true); len(got) != 0 {
		t.Errorf("Wait に映るカメラ: %v", got)
	}
	// 地図は本来の接続（1本目）で読む。グループは1回。
	for _, typ := range []string{"GetSceneCollectionList", "GetSceneItemList", "GetGroupSceneItemList", "GetInputList", "GetInputSettings"} {
		if requestsOnConn(h.s, 1, typ) == 0 {
			t.Errorf("本来の接続で %s を読んでいない", typ)
		}
	}
	if n := requestsOnConn(h.s, 1, "GetGroupSceneItemList"); n != 1 {
		t.Errorf("グループは1回だけ読む: %d", n)
	}
	// 引数は UUID
	for _, r := range h.s.requestsOf("GetSceneItemList") {
		if d := mustJSON(t, r.Data); d["sceneUuid"] == nil || d["sceneName"] != nil {
			t.Errorf("GetSceneItemList は UUID で送る: %s", r.Data)
		}
	}
	for _, r := range h.s.requestsOf("GetInputSettings") {
		if d := mustJSON(t, r.Data); d["inputUuid"] == nil || d["inputName"] != nil {
			t.Errorf("GetInputSettings は UUID で送る: %s", r.Data)
		}
	}
	// OBSConnected の前に読み終えている（OBSConnected のあとに本来の接続で読まない）
	if hasWarning(h.w, "見つかりません") {
		t.Errorf("カメラがあるのに警告: %v", h.w.Warnings())
	}
}

// 地図が読めなければ接続の失敗。続けて3回失敗したら、シーンの一覧だけの地図でつなぐ。
func TestMapReadFailureFailsConnectionThenEmptyMapAfterThreeFailures(t *testing.T) {
	h := newMapHarnessHook(t, func(r auxReq) (auxReply, bool) {
		if r.Type == "GetSceneItemList" {
			return auxReply{Code: 600}, true
		}
		return auxReply{}, false
	}, nil)
	evs := h.awaitConnected(1)
	if n := h.f.count("GetSceneItemList"); n != 3 {
		t.Errorf("3回目の失敗でつなぐはずが、地図の読み取りの失敗は %d 回", n)
	}
	if n := h.s.accepted(); n < 3 {
		t.Errorf("3回つなぎ直すはずが、接続は %d 本", n)
	}
	c, _ := lastOf[switcher.OBSConnected](evs)
	if c.Map == nil || c.Map.Scenes["uuid-live"].Name != "Live" || len(c.Map.Scenes["uuid-live"].Items) != 0 || len(c.Map.Inputs) != 0 {
		t.Fatalf("シーンの一覧だけの地図のはず: %+v", c.Map)
	}
	if !hasWarning(h.w, "シーンの地図を読めない") {
		t.Errorf("警告がない: %v", h.w.Warnings())
	}
	if hasWarning(h.w, "見つかりません") {
		t.Errorf("読めないときに、カメラごとの警告は出さない: %v", h.w.Warnings())
	}
	// 空の地図でつないだら、30秒を待たずに、補助の接続で本当の地図を読みにいく。
	// 本来の接続の読み取りは GetSceneCollectionList が 3 回（失敗）+ 空の地図の 1 回。
	waitFor(t, "補助の接続で読み直す", func() bool { return h.f.count("GetSceneCollectionList") >= 5 })
}

func TestMapReadSuccessResetsFailureCount(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	h := newMapHarnessHook(t, func(r auxReq) (auxReply, bool) {
		if failing.Load() && r.Type == "GetSceneItemList" {
			return auxReply{Code: 600}, true
		}
		return auxReply{}, false
	}, nil)
	waitFor(t, "2回失敗する", func() bool { return h.f.count("GetSceneItemList") >= 2 })
	failing.Store(false) // 3回目は成功
	evs := h.awaitConnected(1)
	if c, _ := lastOf[switcher.OBSConnected](evs); len(c.Map.Scenes["uuid-live"].Items) != 2 {
		t.Fatalf("成功したので、本当の地図のはず: %+v", c.Map)
	}
	if hasWarning(h.w, "シーンの地図を読めない") {
		t.Errorf("読めたのに警告が残っている: %v", h.w.Warnings())
	}
	// 切れてつなぎ直すとき、失敗の数は0から数える。空の地図でつなぐのは、そこから3回目の失敗のとき。
	failing.Store(true)
	before := h.f.count("GetSceneItemList")
	h.s.kill(mainConnNumber(h.s))
	h.awaitDisconnected(1)
	evs = h.awaitConnected(2)
	if n := h.f.count("GetSceneItemList") - before; n != 3 {
		t.Fatalf("成功で失敗の数は0に戻るはずが、つなぐまでの失敗が %d 回", n)
	}
	if c, _ := lastOf[switcher.OBSConnected](evs); len(c.Map.Scenes["uuid-live"].Items) != 0 {
		t.Fatalf("空の地図のはず: %+v", c.Map)
	}
}

// mainConnNumber は、いまの本来の接続の番号（最後に GetStreamStatus を送った接続）。
func mainConnNumber(s *auxServer) int {
	rs := s.requestsOf("GetStreamStatus")
	return rs[len(rs)-1].Conn
}

func TestLegacyVersionMapIsSceneListOnly(t *testing.T) {
	s := newAuxServer(t)
	f := newFakeOBS()
	f.version = "5.3.0"
	mapWorld(f)
	s.setHandler(f.handle)
	h := startWSHarness(t, s, f, mapTune)
	evs := h.awaitConnected(1)
	c, _ := lastOf[switcher.OBSConnected](evs)
	if c.Map == nil || c.Map.Scenes["Live"].Name != "Live" || c.Map.Scenes["Wait"].Name != "Wait" {
		t.Fatalf("古い版の地図は、名前が鍵のシーンの一覧: %+v", c.Map)
	}
	if len(c.Map.Inputs) != 0 || len(c.Map.Groups) != 0 || len(c.Map.Scenes["Live"].Items) != 0 {
		t.Fatalf("古い版の地図に項目とカメラの対応はない: %+v", c.Map)
	}
	for _, typ := range []string{"GetSceneItemList", "GetGroupSceneItemList", "GetInputSettings"} {
		if n := len(h.s.requestsOf(typ)); n != 0 {
			t.Errorf("古い版で %s を送った: %d", typ, n)
		}
	}
	if hasWarning(h.w, "見つかりません") {
		t.Errorf("古い版でカメラごとの警告は出さない: %v", h.w.Warnings())
	}
}

func TestCameraMediaSourceWarning(t *testing.T) {
	h := newMapHarness(t, func(w *WS) {
		w.cfg.Cameras = []CameraConfig{{ID: "cam1", Name: "メイン"}, {ID: "cam9", Name: "予備"}}
	})
	h.awaitConnected(1)
	if !hasWarning(h.w, "予備 のメディアソースが OBS に見つかりません") {
		t.Fatalf("警告がない: %v", h.w.Warnings())
	}
	if hasWarning(h.w, "メイン のメディアソース") {
		t.Fatalf("あるカメラの警告が出ている: %v", h.w.Warnings())
	}
}

// ---- 補助の接続の読み直し ----

type refreshHarness struct {
	*wsHarness
	clk *fakeClock
}

func newRefreshHarness(t *testing.T) *refreshHarness {
	t.Helper()
	clk := newFakeClock()
	h := newMapHarness(t, func(w *WS) { w.clock = clk })
	h.awaitConnected(1)
	waitFor(t, "補助の接続が開く", func() bool { return h.s.accepted() == 2 })
	waitFor(t, "30秒の待ちが置かれる", func() bool { return clk.pending() >= 1 })
	return &refreshHarness{h, clk}
}

// auxReads は、補助の接続（2本目）で地図を読んだ回数。
func (h *refreshHarness) auxReads() int { return requestsOnConn(h.s, 2, "GetSceneCollectionList") }

func (h *wsHarness) mapChanges() int { return countOf[switcher.SceneMapChanged](h.events()) }

var refreshTriggers = []struct {
	typ  string
	data map[string]any
}{
	{"SceneCreated", map[string]any{"sceneName": "N", "sceneUuid": "uuid-n", "isGroup": false}},
	{"SceneRemoved", map[string]any{"sceneName": "Gone", "sceneUuid": "uuid-gone", "isGroup": false}},
	{"SceneNameChanged", map[string]any{"oldSceneName": "Wait", "sceneName": "Wait2", "sceneUuid": "uuid-wait"}},
	{"SceneItemCreated", map[string]any{"sceneName": "Live", "sceneUuid": "uuid-live", "sceneItemId": 9, "sourceName": "x"}},
	{"SceneItemRemoved", map[string]any{"sceneName": "Live", "sceneUuid": "uuid-live", "sceneItemId": 9, "sourceName": "x", "sourceUuid": "x"}},
	{"InputCreated", map[string]any{"inputName": "Cam3", "inputUuid": "in3", "inputKind": "ffmpeg_source"}},
	{"InputRemoved", map[string]any{"inputName": "Cam3", "inputUuid": "in3"}},
	{"InputNameChanged", map[string]any{"inputName": "Cam1b", "inputUuid": "in1", "oldInputName": "Cam1"}},
}

// 8つのイベントのそれぞれで、0.3秒まとめて、補助の接続で全部を読み直し、SceneMapChanged を流す。
func TestRefreshIsTriggeredByEachEventAfterDebounce(t *testing.T) {
	h := newRefreshHarness(t)
	for i, tr := range refreshTriggers {
		reads, changes := h.auxReads(), h.mapChanges()
		p := h.clk.pending()
		h.s.sendEvent(1, tr.typ, tr.data)
		waitFor(t, tr.typ+": まとめる待ちが置かれる", func() bool { return h.clk.pending() > p })
		h.clk.Advance(299 * time.Millisecond)
		time.Sleep(60 * time.Millisecond)
		if h.auxReads() != reads {
			t.Fatalf("%s: 0.3秒に足りないのに読んだ", tr.typ)
		}
		h.clk.Advance(time.Millisecond)
		waitFor(t, tr.typ+": 読み直す", func() bool { return h.mapChanges() == changes+1 })
		if got := h.auxReads(); got != reads+1 {
			t.Fatalf("%s: 読み直しは1回のはずが %d 回", tr.typ, got-reads)
		}
		_ = i
	}
	if n := requestsOnConn(h.s, 1, "GetSceneItemList"); n != 2 { // 本来の接続は最初の地図の2シーンだけ
		t.Fatalf("読み直しを本来の接続で読んだ: %d", n)
	}
	m, _ := lastOf[switcher.SceneMapChanged](h.events())
	if m.Map == nil || m.At.IsZero() {
		t.Fatalf("SceneMapChanged: %+v", m)
	}
}

func TestRefreshBatchesBurstOfEvents(t *testing.T) {
	h := newRefreshHarness(t)
	reads := h.auxReads()
	p := h.clk.pending()
	for _, tr := range refreshTriggers[:5] {
		h.s.sendEvent(1, tr.typ, tr.data)
	}
	waitFor(t, "待ちが置かれる", func() bool { return h.clk.pending() > p })
	time.Sleep(100 * time.Millisecond)
	h.clk.Advance(300 * time.Millisecond)
	waitFor(t, "読み直す", func() bool { return h.mapChanges() == 1 })
	time.Sleep(150 * time.Millisecond)
	if got := h.auxReads(); got != reads+1 {
		t.Fatalf("5つのイベントは1回の読み直しにまとまるはずが %d 回", got-reads)
	}
}

// 読み直しの間に次のイベントが来たら、終わったあとにもう一度読む。
func TestRefreshReadsAgainWhenEventArrivesDuringRead(t *testing.T) {
	for _, during := range []struct {
		name string
		typ  string
		data map[string]any
	}{
		{"SceneCreated", refreshTriggers[0].typ, refreshTriggers[0].data},
		{"InputSettingsChanged", "InputSettingsChanged", map[string]any{"inputName": "Cam1", "inputUuid": "in1", "inputSettings": map[string]any{"input": "rtsp://127.0.0.1:8554/cam2", "is_local_file": false}}},
	} {
		t.Run(during.name, func(t *testing.T) {
			h := newRefreshHarness(t)
			reads := h.auxReads()
			h.f.setHook(func(r auxReq) (auxReply, bool) {
				if r.Conn == 2 && r.Type == "GetSceneList" {
					return auxReply{Delay: 300 * time.Millisecond}, true // 読んでいる間
				}
				return auxReply{}, false
			})
			p := h.clk.pending()
			h.s.sendEvent(1, "SceneCreated", map[string]any{"sceneName": "N", "sceneUuid": "uuid-n"})
			waitFor(t, "待ちが置かれる", func() bool { return h.clk.pending() > p })
			h.clk.Advance(300 * time.Millisecond)
			waitFor(t, "1回目の読み取りが始まる", func() bool { return h.auxReads() == reads+1 })
			h.s.sendEvent(1, during.typ, during.data) // 読んでいる最中
			waitFor(t, "1回目が終わる", func() bool { return h.mapChanges() >= 1 })
			p = h.clk.pending()
			waitFor(t, "もう一度の待ちが置かれる", func() bool { return h.clk.pending() >= p })
			h.clk.Advance(300 * time.Millisecond)
			waitFor(t, "もう一度読む", func() bool { return h.auxReads() == reads+2 })
			waitFor(t, "2回目が終わる", func() bool { return h.mapChanges() >= 2 })
			time.Sleep(100 * time.Millisecond)
			h.clk.Advance(300 * time.Millisecond)
			time.Sleep(100 * time.Millisecond)
			if got := h.auxReads(); got != reads+2 {
				t.Fatalf("読み直しは2回で止まるはずが %d 回", got-reads)
			}
		})
	}
}

// 読み取りが失敗したら、その回の結果は流さず、1秒あけて読み直す。
func TestRefreshFailureRetriesAfterOneSecond(t *testing.T) {
	h := newRefreshHarness(t)
	reads := h.auxReads()
	failing := true
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Conn == 2 && r.Type == "GetSceneItemList" && failing {
			failing = false // 1回だけ失敗
			return auxReply{Code: 600}, true
		}
		return auxReply{}, false
	})
	p := h.clk.pending()
	h.s.sendEvent(1, "SceneCreated", map[string]any{"sceneName": "N", "sceneUuid": "uuid-n"})
	waitFor(t, "待ちが置かれる", func() bool { return h.clk.pending() > p })
	h.clk.Advance(300 * time.Millisecond)
	waitFor(t, "1回目の読み取りが失敗する", func() bool { return h.auxReads() == reads+1 && h.f.count("GetSceneItemList") >= 3 })
	p = h.clk.pending()
	waitFor(t, "1秒の待ちが置かれる", func() bool { return h.clk.pending() > p-1 && h.clk.pending() >= 2 })
	time.Sleep(100 * time.Millisecond)
	if h.mapChanges() != 0 {
		t.Fatal("失敗した回の結果を流した")
	}
	h.clk.Advance(999 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	if h.auxReads() != reads+1 {
		t.Fatal("1秒に足りないのに読み直した")
	}
	h.clk.Advance(time.Millisecond)
	waitFor(t, "読み直して流す", func() bool { return h.mapChanges() == 1 })
	if got := h.auxReads(); got != reads+2 {
		t.Fatalf("読み取り: %d", got-reads)
	}
}

// つながっている間は、イベントがなくても30秒ごとに読み直す。
func TestRefreshEvery30Seconds(t *testing.T) {
	h := newRefreshHarness(t)
	reads := h.auxReads()
	h.clk.Advance(29 * time.Second)
	time.Sleep(80 * time.Millisecond)
	if h.auxReads() != reads {
		t.Fatal("30秒に足りないのに読んだ")
	}
	h.clk.Advance(time.Second)
	waitFor(t, "30秒で読む", func() bool { return h.mapChanges() == 1 })
	waitFor(t, "次の30秒の待ちが置かれる", func() bool { return h.clk.pending() >= 1 })
	h.clk.Advance(30 * time.Second)
	waitFor(t, "次の30秒で読む", func() bool { return h.mapChanges() == 2 })
}

// 切断のあとは、前の世代で始めた読み直しの結果を流さない。
func TestRefreshResultOfOldGenerationIsNotEmitted(t *testing.T) {
	h := newRefreshHarness(t)
	reads := h.auxReads()
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Conn == 2 && r.Type == "GetSceneList" {
			return auxReply{Delay: 400 * time.Millisecond}, true
		}
		return auxReply{}, false
	})
	p := h.clk.pending()
	h.s.sendEvent(1, "SceneCreated", map[string]any{"sceneName": "N", "sceneUuid": "uuid-n"})
	waitFor(t, "待ちが置かれる", func() bool { return h.clk.pending() > p })
	h.clk.Advance(300 * time.Millisecond)
	waitFor(t, "読み取りが始まる", func() bool { return h.auxReads() == reads+1 })
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	time.Sleep(700 * time.Millisecond)
	if h.mapChanges() != 0 {
		t.Fatal("切断のあとに、前の世代の読み直しの結果を流した")
	}
}

func TestEmitEpochAfterCloseDropsEvent(t *testing.T) {
	h := newMapHarness(t, nil)
	h.awaitConnected(1)
	ep := &epoch{}
	ep.ctx, ep.cancel = contextWithCancel()
	h.w.closeEpoch(ep)
	if h.w.emitEpoch(ep, switcher.SceneMapChanged{}) {
		t.Fatal("閉じたエポックの結果を流した")
	}
	time.Sleep(50 * time.Millisecond)
	if h.mapChanges() != 0 {
		t.Fatal("流れた")
	}
}

// InputSettingsChanged は、地図で ffmpeg_source とわかっている入力だけ。全部は読み直さず、URL の照合だけやり直す。
func TestInputSettingsChangedRematchesOnlyThatInput(t *testing.T) {
	h := newRefreshHarness(t)
	reads := h.auxReads()
	// 知らない入力と、ffmpeg_source でない入力（地図にない）は何もしない。
	h.s.sendEvent(1, "InputSettingsChanged", map[string]any{"inputName": "Mic", "inputUuid": "mic", "inputSettings": map[string]any{"input": "rtsp://127.0.0.1:8554/cam1", "is_local_file": false}})
	time.Sleep(150 * time.Millisecond)
	if h.mapChanges() != 0 {
		t.Fatal("地図にない入力で SceneMapChanged を流した")
	}
	// URL を cam2 に変える。
	h.s.sendEvent(1, "InputSettingsChanged", map[string]any{"inputName": "Cam1", "inputUuid": "in1", "inputSettings": map[string]any{"input": "rtsp://127.0.0.1:8554/cam2", "is_local_file": false}})
	waitFor(t, "SceneMapChanged", func() bool { return h.mapChanges() == 1 })
	m, _ := lastOf[switcher.SceneMapChanged](h.events())
	if got := m.Map.Inputs["in1"]; got.CameraID != "cam2" || got.Name != "Cam1" {
		t.Fatalf("in1: %+v", got)
	}
	if got := m.Map.Inputs["in2"]; got.CameraID != "cam2" {
		t.Fatalf("in2 は変えない: %+v", got)
	}
	if got := h.auxReads(); got != reads {
		t.Fatalf("全部を読み直した: %d 回", got-reads)
	}
	if n := len(h.s.requestsOf("GetInputSettings")); n != 2 {
		t.Fatalf("イベントの inputSettings で照合するので、要求は増えない: %d", n)
	}
	// ローカルファイルに戻す（is_local_file がなければ既定の true）。
	h.s.sendEvent(1, "InputSettingsChanged", map[string]any{"inputName": "Cam1", "inputUuid": "in1", "inputSettings": map[string]any{"input": "rtsp://127.0.0.1:8554/cam1"}})
	waitFor(t, "SceneMapChanged", func() bool { return h.mapChanges() == 2 })
	m, _ = lastOf[switcher.SceneMapChanged](h.events())
	if got := m.Map.Inputs["in1"]; got.CameraID != "" {
		t.Fatalf("ローカルファイルは対象外: %+v", got)
	}
	// 変わらなければ流さない。
	h.s.sendEvent(1, "InputSettingsChanged", map[string]any{"inputName": "Cam1", "inputUuid": "in1", "inputSettings": map[string]any{"input": "rtsp://127.0.0.1:8554/cam1"}})
	time.Sleep(150 * time.Millisecond)
	if h.mapChanges() != 2 {
		t.Fatal("照合が変わらないのに流した")
	}
	// 警告も照合に合わせて変わる（cam1 のメディアソースがなくなった）。
	if !hasWarning(h.w, "メイン のメディアソースが OBS に見つかりません") {
		t.Fatalf("警告: %v", h.w.Warnings())
	}
}

func TestItemEnabledChangedIsEmitted(t *testing.T) {
	h := newMapHarness(t, nil)
	h.awaitConnected(1)
	h.s.sendEvent(1, "SceneItemEnableStateChanged", map[string]any{"sceneName": "Group", "sceneUuid": "g1", "sceneItemId": 1, "sceneItemEnabled": false})
	evs := h.await("ItemEnabledChanged", func(evs []switcher.Event) bool { return countOf[switcher.ItemEnabledChanged](evs) == 1 })
	e, _ := lastOf[switcher.ItemEnabledChanged](evs)
	if e.SceneUUID != "g1" || e.ItemID != 1 || e.Enabled || e.At.IsZero() {
		t.Fatalf("ItemEnabledChanged: %+v", e)
	}
	h.s.sendEvent(1, "SceneItemEnableStateChanged", map[string]any{"sceneName": "Live", "sceneUuid": "uuid-live", "sceneItemId": 2, "sceneItemEnabled": true})
	evs = h.await("2つ目", func(evs []switcher.Event) bool { return countOf[switcher.ItemEnabledChanged](evs) == 2 })
	if e, _ := lastOf[switcher.ItemEnabledChanged](evs); !e.Enabled || e.SceneUUID != "uuid-live" || e.ItemID != 2 {
		t.Fatalf("ItemEnabledChanged: %+v", e)
	}
	if n := countOf[switcher.SceneMapChanged](h.events()); n != 0 {
		t.Fatalf("表示・非表示は読み直さない: %d", n)
	}
}

// 読み取りの最中に届いた表示・非表示の変化は、OBSConnected のあとに流す。
func TestItemEnabledChangedDuringReloadIsHeld(t *testing.T) {
	h := newMapHarness(t, nil)
	h.awaitConnected(1)
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Conn == 1 && r.Type == "GetInputList" {
			return auxReply{Delay: 150 * time.Millisecond}, true // 項目の一覧を読んだあと
		}
		return auxReply{}, false
	})
	h.s.sendEvent(1, "CurrentSceneCollectionChanged", map[string]any{"sceneCollectionName": "New"})
	waitFor(t, "項目の一覧のあとの読み取りが届く", func() bool { return h.f.count("GetInputList") == 2 })
	h.s.sendEvent(1, "SceneItemEnableStateChanged", map[string]any{"sceneUuid": "uuid-live", "sceneItemId": 1, "sceneItemEnabled": false})
	h.awaitConnected(2)
	h.await("取っておいた変化が流れる", func(evs []switcher.Event) bool { return countOf[switcher.ItemEnabledChanged](evs) == 1 })
	var order []string
	for _, e := range h.events() {
		switch e.(type) {
		case switcher.OBSConnected:
			order = append(order, "connected")
		case switcher.OBSDisconnected:
			order = append(order, "disconnected")
		case switcher.ItemEnabledChanged:
			order = append(order, "item")
		}
	}
	if !slices.Equal(order, []string{"connected", "disconnected", "connected", "item"}) {
		t.Fatalf("イベントの順: %v", order)
	}
	// 読み取りの最中に地図のイベントが来たら、読み取りのあとに読み直す（取りこぼさない）。
}

func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
