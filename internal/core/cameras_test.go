package core

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// ---- 1組の Sample ----

// drain は seen に溜まっている記録を読み出す（待たない）。
func (h *harness) drain() []any {
	var out []any
	for {
		select {
		case v := <-h.seen:
			out = append(out, v)
		default:
			return out
		}
	}
}

// settle は Tick を1つ流し、それが処理されるまでの記録を返す。直前に送った入力の処理が終わったことの確かめにも使う。
func (h *harness) settle() []any {
	h.t.Helper()
	h.tick(h.clk.Advance(time.Millisecond))
	return h.waitSeen("Tick", func(v any) bool { _, ok := v.(switcher.Tick); return ok })
}

func countOf[T any](seen []any) int {
	n := 0
	for _, v := range seen {
		if _, ok := v.(T); ok {
			n++
		}
	}
	return n
}

// 1組の Sample で変わったカメラが何台でも、CamerasChanged は1回。変わらなければ0回。
func TestSampleSetSendsCamerasChangedOnce(t *testing.T) {
	h := newHarness(t, twoCameras)
	h.start()

	h.sampleSet(goodKbps, goodKbps) // 2台とも DOWN → LOW
	seen := h.settle()
	if n := countOf[switcher.CamerasChanged](seen); n != 1 {
		t.Fatalf("2台が同時に変わった組の CamerasChanged = %d, want 1: %#v", n, seen)
	}
	var cc switcher.CamerasChanged
	for _, v := range seen {
		if e, ok := v.(switcher.CamerasChanged); ok {
			cc = e
		}
	}
	want := map[string]scenemap.CameraState{mainCam: scenemap.StateLow, subCam: scenemap.StateLow}
	if !reflect.DeepEqual(cc.States, want) {
		t.Errorf("States = %v, want 全カメラの今の状態 %v", cc.States, want)
	}

	for range 3 { // どちらも変わらない組
		h.sampleSet(goodKbps, goodKbps)
	}
	if n := countOf[switcher.CamerasChanged](h.settle()); n != 0 {
		t.Errorf("変わらない組で CamerasChanged = %d, want 0", n)
	}

	h.sampleSet(goodKbps, goodKbps) // 5秒たって2台とも OK
	if n := countOf[switcher.CamerasChanged](h.settle()); n != 1 {
		t.Errorf("OK へ戻った組の CamerasChanged = %d, want 1", n)
	}
	h.waitSnap("2台とも OK", func(s Snapshot) bool { return s.Cameras[0].State == "OK" && s.Cameras[1].State == "OK" })

	// 片方だけ変わる組。States には変わらない方も入る。
	h.sampleSet(goodKbps, 0)
	h.sampleSet(goodKbps, 0)
	h.sampleSet(goodKbps, 0) // sub は 0 が3秒で LOW
	seen = h.settle()
	if n := countOf[switcher.CamerasChanged](seen); n != 1 {
		t.Fatalf("CamerasChanged = %d, want 1", n)
	}
	for _, v := range seen {
		if e, ok := v.(switcher.CamerasChanged); ok {
			if e.States[mainCam] != scenemap.StateOK || e.States[subCam] != scenemap.StateLow {
				t.Errorf("States = %v", e.States)
			}
		}
	}
}

// 閾値はカメラごと。同じ kbps でも、上書きした閾値のカメラは違う状態になる。
func TestSampleSetUsesPerCameraThresholds(t *testing.T) {
	h := newHarness(t, func(_ *switcher.Config, d *Deps) {
		main := testCamera(mainCam, "メイン", scenemap.MotionStandby)
		sub := testCamera(subCam, "サブ", scenemap.MotionStandby)
		sub.Thresholds.FailKbps, sub.Thresholds.RecoverKbps = 400, 800
		d.Cameras = []Camera{main, sub}
	})
	h.start()
	for range 5 {
		h.sampleSet(goodKbps, goodKbps)
	}
	h.waitSnap("2台とも OK", func(s Snapshot) bool { return s.Cameras[0].State == "OK" && s.Cameras[1].State == "OK" })
	for range 3 { // 500 kbps は main の fail_kbps（1000）未満、sub の fail_kbps（400）以上
		h.sampleSet(500, 500)
	}
	s := h.waitSnap("main だけ LOW", func(s Snapshot) bool { return s.Cameras[0].State == "LOW" })
	if s.Cameras[1].State != "OK" {
		t.Errorf("sub = %s, want OK（自分の閾値では十分）", s.Cameras[1].State)
	}
}

// 組に入っていないカメラは更新しない。設定にない id の Sample は無視する。
func TestSampleSetIgnoresUnknownAndMissingCameras(t *testing.T) {
	h := newHarness(t, twoCameras)
	h.start()
	at := h.clk.Advance(time.Second)
	h.sendSet(ingest.SampleSet{At: at, Cameras: []ingest.Sample{
		{Camera: "ghost", At: at, Interval: time.Second, Online: true, Kbps: goodKbps},
		{Camera: subCam, At: at, Interval: time.Second, Online: true, Kbps: goodKbps},
	}})
	s := h.waitSnap("sub が LOW", func(s Snapshot) bool { return s.Cameras[1].State == "LOW" })
	if s.Cameras[0].State != "DOWN" || len(s.Cameras) != 2 {
		t.Errorf("cameras = %+v", s.Cameras)
	}
}

// カメラの状態の変化は camera changed の1行（camera、from、to、kbps）。link changed は出さない。
func TestCameraChangedLog(t *testing.T) {
	h := newHarness(t, twoCameras)
	h.start()
	h.sampleSet(goodKbps, 0)
	h.waitSnap("main が LOW", func(s Snapshot) bool { return s.Cameras[0].State == "LOW" })
	out := h.log.String()
	if !strings.Contains(out, `msg="camera changed" camera=main from=DOWN to=LOW kbps=3000`) {
		t.Errorf("camera changed の行がない:\n%s", out)
	}
	if strings.Contains(out, "camera=sub") || strings.Contains(out, "link changed") {
		t.Errorf("変わらないカメラや古い行が出た:\n%s", out)
	}
}

// ---- 複数のカメラの切替 ----

// 2台が映るシーンで悪い方が代表になり、scene switch に camera と、そのカメラの kbps が出る。
func TestSceneSwitchLogsBadCamera(t *testing.T) {
	h := newHarness(t, twoCameras)
	h.start()
	for range 5 {
		h.sampleSet(goodKbps, goodKbps)
	}
	h.waitSnap("2台とも OK", func(s Snapshot) bool { return s.Cameras[0].State == "OK" && s.Cameras[1].State == "OK" })
	m := testMap()
	m.Inputs["IRL2"] = scenemap.Input{Name: "IRL2", CameraID: subCam}
	c := m.Scenes[camera]
	c.Items = append(c.Items, scenemap.Item{ID: 2, SourceUUID: "IRL2", SourceKind: scenemap.InputSource, Enabled: true})
	m.Scenes[camera] = c
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Streaming: true, Scene: camera, SceneUUID: camera, Map: m})
	s := h.waitSnap("camera", sceneIs(camera))
	if !reflect.DeepEqual(s.OBS.SceneCameras, []string{mainCam, subCam}) || s.OBS.SceneHealth == nil || *s.OBS.SceneHealth != "OK" {
		t.Fatalf("scene_cameras = %v, scene_health = %v", s.OBS.SceneCameras, s.OBS.SceneHealth)
	}
	for range 3 { // sub だけ 0 が3秒続いて LOW
		h.sampleSet(goodKbps, 0)
	}
	h.waitCall("scene:" + standby)
	h.waitSeen("BatchDone", isBatchDone(switcher.BatchCompleted))
	want := `msg="scene switch" from=Camera to=Standby reason=low camera=sub kbps=0`
	if out := h.log.String(); !strings.Contains(out, want) {
		t.Errorf("ログに %q がない:\n%s", want, out)
	}
}

// 実行役は UUID のメソッドで送り、ログと status のシーンは地図の名前で書く。
func TestExecutorUsesUUIDsAndNames(t *testing.T) {
	h := newHarness(t)
	h.start()
	m := &scenemap.Map{
		Scenes: map[string]scenemap.Container{
			"u-standby": {Name: standby},
			"u-camera": {Name: camera, Items: []scenemap.Item{
				{ID: 1, SourceUUID: "u-irl", SourceKind: scenemap.InputSource, Enabled: true}}},
		},
		Groups: map[string]scenemap.Container{},
		Inputs: map[string]scenemap.Input{"u-irl": {Name: "IRL", CameraID: mainCam}},
	}
	h.samplesN(5, goodKbps)
	h.waitSnap("OK", linkIs("OK"))
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Streaming: true, Scene: camera, SceneUUID: "u-camera", Map: m})
	h.waitSnap("camera", sceneIs(camera))
	h.samplesN(3, 0)
	h.waitCall("scene:u-standby")
	h.waitSeen("AutoFail の BatchDone", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("自動の待機", both(sceneIs(standby), standbyIs("auto", camera)))
	if calls := h.obs.Calls(); !reflect.DeepEqual(calls, []string{"override:u-standby", "scene:u-standby"}) {
		t.Errorf("calls = %v", calls)
	}
	if want := `msg="scene switch" from=Camera to=Standby reason=low camera=main kbps=0`; !strings.Contains(h.log.String(), want) {
		t.Errorf("ログに %q がない:\n%s", want, h.log.String())
	}
	n := len(h.obs.Calls())
	h.samplesN(5, goodKbps)
	h.waitSeen("AutoRecover の BatchDone", isBatchDone(switcher.BatchCompleted))
	if calls := callsSince(h.obs, n); !reflect.DeepEqual(calls, []string{"restart:u-irl", "override:u-camera", "scene:u-camera"}) {
		t.Errorf("復帰の calls = %v", calls)
	}
	if want := `msg="scene switch" from=Standby to=Camera reason=ok`; !strings.Contains(h.log.String(), want) {
		t.Errorf("ログに %q がない:\n%s", want, h.log.String())
	}
}

// ---- 地図の Enabled の引き継ぎ ----

func (h *harness) item(scene string, id int, enabled bool) {
	h.t.Helper()
	h.obsEvent(switcher.ItemEnabledChanged{At: h.clk.Now(), SceneUUID: scene, ItemID: id, Enabled: enabled})
}

func (h *harness) mapChanged(m *scenemap.Map) {
	h.t.Helper()
	h.obsEvent(switcher.SceneMapChanged{At: h.clk.Now(), Map: m})
}

// lastMap は、Machine に渡した最後の地図（SceneMapChanged か OBSConnected）。
func lastMap(seen []any) *scenemap.Map {
	var m *scenemap.Map
	for _, v := range seen {
		switch e := v.(type) {
		case switcher.SceneMapChanged:
			m = e.Map
		case switcher.OBSConnected:
			m = e.Map
		}
	}
	return m
}

func enabledOf(m *scenemap.Map, scene string, id int) (enabled, found bool) {
	for _, it := range m.Scenes[scene].Items {
		if it.ID == id {
			return it.Enabled, true
		}
	}
	return false, false
}

// withItem は testMap に、Camera の項目 id を足した地図（Enabled は引数）。
func withItem(id int, enabled bool) *scenemap.Map {
	m := testMap()
	c := m.Scenes[camera]
	c.Items = append(c.Items, scenemap.Item{ID: id, SourceUUID: "IRL", SourceKind: scenemap.InputSource, Enabled: enabled})
	m.Scenes[camera] = c
	return m
}

// 今の地図にある項目の Enabled は、読み直しの結果の値ではなく、本来の接続のイベントで知った値を使う。
func TestMapChangedKeepsEnabledOfKnownItems(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.connect(false, camera)
	h.waitSnap("camera", sceneIs(camera))
	if s := h.core.Snapshot(); !reflect.DeepEqual(s.OBS.SceneCameras, []string{mainCam}) {
		t.Fatalf("scene_cameras = %v", s.OBS.SceneCameras)
	}
	h.item(camera, 1, false) // 人が隠した
	h.waitSnap("映るカメラなし", func(s Snapshot) bool { return len(s.OBS.SceneCameras) == 0 })
	h.drain()

	h.mapChanged(testMap()) // 古い読み取りの値（項目1は表示）
	seen := h.settle()
	m := lastMap(seen)
	if m == nil {
		t.Fatalf("SceneMapChanged を Machine に渡していない: %#v", seen)
	}
	if en, found := enabledOf(m, camera, 1); !found || en {
		t.Errorf("項目1の Enabled = %v（found=%v）, want 写しの値 false", en, found)
	}
	if s := h.core.Snapshot(); len(s.OBS.SceneCameras) != 0 {
		t.Errorf("Machine の地図で映るカメラ = %v, want なし", s.OBS.SceneCameras)
	}
	if n := countOf[switcher.SceneMapChanged](seen); n != 1 {
		t.Errorf("SceneMapChanged = %d 回, want 1", n)
	}
}

// 今の地図にない項目の変化は覚えて、新しい地図の新しい項目に重ねる。ItemEnabledChanged は届いた時点で1回 Machine に渡す。
func TestMapChangedOverlaysRememberedUnknownItems(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.connect(false, camera)
	h.waitSnap("camera", sceneIs(camera))
	h.drain()

	h.item(camera, 7, false) // まだ地図にない項目
	seen := h.settle()
	if n := countOf[switcher.ItemEnabledChanged](seen); n != 1 {
		t.Errorf("ItemEnabledChanged を Machine に渡した回数 = %d, want 1", n)
	}
	h.mapChanged(withItem(7, true)) // 読み取りの時点では表示だった
	m := lastMap(h.settle())
	if en, found := enabledOf(m, camera, 7); !found || en {
		t.Errorf("新しい項目7の Enabled = %v（found=%v）, want 覚えた値 false", en, found)
	}
	if en, _ := enabledOf(m, camera, 1); !en {
		t.Errorf("項目1は読み直しの値（true）のまま")
	}

	// 覚えた値は地図ができたら空になる。次の地図の新しい項目9は、読み取りの値のまま。
	h.item(camera, 9, false)
	h.mapChanged(testMap()) // 項目9を含まない地図。ここで覚えた値を捨てる
	h.settle()
	h.mapChanged(withItem(9, true))
	m = lastMap(h.settle())
	if en, found := enabledOf(m, camera, 9); !found || !en {
		t.Errorf("項目9の Enabled = %v（found=%v）, want 読み取りの値 true（覚えた値は前の地図で捨てた）", en, found)
	}
}

// 覚えた値は切断で空になる。新しい接続の最初の地図（OBSConnected）はそのまま使い、以後の写しもそれから始める。
func TestOBSConnectedMapIsUsedAsIs(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.connect(false, camera)
	h.waitSnap("camera", sceneIs(camera))
	h.item(camera, 1, false)
	h.item(camera, 7, false) // 覚える
	h.drain()

	h.obsEvent(switcher.OBSDisconnected{At: h.clk.Now()})
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Scene: camera, SceneUUID: camera, Map: withItem(7, true)})
	m := lastMap(h.settle())
	if en, _ := enabledOf(m, camera, 1); !en {
		t.Errorf("OBSConnected の項目1 = %v, want 地図のまま true", en)
	}
	if en, _ := enabledOf(m, camera, 7); !en {
		t.Errorf("OBSConnected の項目7 = %v, want 地図のまま true", en)
	}
	// 写しは新しい接続の地図から。古い接続の値は引き継がない。
	h.mapChanged(withItem(7, false))
	m = lastMap(h.settle())
	if en, _ := enabledOf(m, camera, 1); !en {
		t.Errorf("項目1 = %v, want 新しい接続の値 true", en)
	}
	if en, _ := enabledOf(m, camera, 7); !en {
		t.Errorf("項目7 = %v, want 新しい接続の地図の値 true を引き継ぐ", en)
	}

	// 切断で覚えた値が空になる: 切断の前に覚えた項目は、つなぎ直したあとの地図の新しい項目に重ならない。
	h.item(camera, 11, false)
	h.obsEvent(switcher.OBSDisconnected{At: h.clk.Now()})
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Scene: camera, SceneUUID: camera, Map: testMap()})
	h.settle()
	h.mapChanged(withItem(11, true))
	m = lastMap(h.settle())
	if en, found := enabledOf(m, camera, 11); !found || !en {
		t.Errorf("項目11 = %v（found=%v）, want 読み取りの値 true（切断で覚えた値を捨てた）", en, found)
	}
}

// MC-MAP-15、MC-ITEM-11: 切断中（と最初の接続の前）の地図の読み直しの結果と項目のイベントは、Machine に渡さず捨てる。
func TestDisconnectedMapAndItemEventsAreDropped(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.mapChanged(testMap()) // 最初の接続の前
	h.item(camera, 1, false)
	if seen := h.settle(); countOf[switcher.SceneMapChanged](seen)+countOf[switcher.ItemEnabledChanged](seen) != 0 {
		t.Fatalf("未接続の間のイベントを Machine に渡した: %#v", seen)
	}

	h.connect(false, camera)
	h.waitSnap("camera", sceneIs(camera))
	h.obsEvent(switcher.OBSDisconnected{At: h.clk.Now()})
	h.drain()
	h.mapChanged(withItem(5, true))
	h.item(camera, 1, false)
	seen := h.settle()
	if n := countOf[switcher.SceneMapChanged](seen) + countOf[switcher.ItemEnabledChanged](seen); n != 0 {
		t.Fatalf("切断中のイベントを Machine に渡した: %#v", seen)
	}
	// 捨てた項目のイベントは、つなぎ直した地図を書き換えない。
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Scene: camera, SceneUUID: camera, Map: testMap()})
	h.waitSnap("つなぎ直し", func(s Snapshot) bool { return s.OBS.Connected && len(s.OBS.SceneCameras) == 1 })
}

// AutoRecover の scene switch にも camera と kbps を出す。代表は、戻り先に映るカメラのうち kbps がいちばん低いもの。
func TestAutoRecoverLogsLowestVisibleCamera(t *testing.T) {
	h := newHarness(t, twoCameras)
	h.start()
	m := testMap()
	m.Inputs["IRL2"] = scenemap.Input{Name: "IRL2", CameraID: subCam}
	c := m.Scenes[camera]
	c.Items = append(c.Items, scenemap.Item{ID: 2, SourceUUID: "IRL2", SourceKind: scenemap.InputSource, Enabled: true})
	m.Scenes[camera] = c
	for range 5 {
		h.sampleSet(goodKbps, goodKbps)
	}
	h.waitSnap("2台とも OK", func(s Snapshot) bool { return s.Cameras[0].State == "OK" && s.Cameras[1].State == "OK" })
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Streaming: true, Scene: camera, SceneUUID: camera, Map: m})
	h.waitSnap("camera", sceneIs(camera))
	for range 3 {
		h.sampleSet(goodKbps, 0)
	}
	h.waitSeen("AutoFail の BatchDone", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("自動の待機", both(sceneIs(standby), standbyIs("auto", camera)))
	for range 5 { // 回復。sub の方が低い
		h.sampleSet(goodKbps, 2500)
	}
	h.waitSeen("AutoRecover の BatchDone", isBatchDone(switcher.BatchCompleted))
	want := `msg="scene switch" from=Standby to=Camera reason=ok camera=sub kbps=2500`
	if out := h.log.String(); !strings.Contains(out, want) {
		t.Errorf("ログに %q がない:\n%s", want, out)
	}
}

// groupMap は testMap に、グループ G（項目 1 = IRL）を足し、Camera の項目をグループ G だけにした地図。
func groupMap(inGroup ...scenemap.Item) *scenemap.Map {
	m := testMap()
	m.Groups["G"] = scenemap.Container{Name: "G", Items: inGroup}
	m.Scenes[camera] = scenemap.Container{Name: camera, Items: []scenemap.Item{
		{ID: 1, SourceUUID: "G", SourceKind: scenemap.GroupSource, Enabled: true}}}
	return m
}

func irlItem(id int, enabled bool) scenemap.Item {
	return scenemap.Item{ID: id, SourceUUID: "IRL", SourceKind: scenemap.InputSource, Enabled: enabled}
}

// 地図にない同じ項目への false → true は、最後の true が勝つ（読み直しの地図が false で届いても）。
func TestUnknownItemLastValueWins(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.connect(false, camera)
	h.waitSnap("camera", sceneIs(camera))
	h.item(talk, 3, false)
	h.item(talk, 3, true)
	m := testMap()
	tk := m.Scenes[talk]
	tk.Items = []scenemap.Item{irlItem(3, false)}
	m.Scenes[talk] = tk
	h.mapChanged(m)
	got := lastMap(h.settle())
	if en, found := enabledOf(got, talk, 3); !found || !en {
		t.Errorf("渡した地図の項目3 = %v（found=%v）, want 最後の値 true", en, found)
	}
	// Machine の側でも Talk にカメラが映る。
	h.obsEvent(switcher.SceneChanged{At: h.clk.Now(), Scene: talk, SceneUUID: talk})
	h.waitSnap("talk にカメラ", func(s Snapshot) bool { return s.OBS.Scene == talk && len(s.OBS.SceneCameras) == 1 })
}

// グループの中の項目でも、既知の項目と新しい項目の両方で引き継ぎが効く。
func TestGroupItemsCarryOver(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Scene: camera, SceneUUID: camera, Map: groupMap(irlItem(1, true))})
	h.waitSnap("camera", func(s Snapshot) bool { return sceneIs(camera)(s) && len(s.OBS.SceneCameras) == 1 })

	// 既知の項目: グループの項目1を人が隠す。古い読み取り（表示）が来ても隠れたまま。
	h.item("G", 1, false)
	h.waitSnap("映るカメラなし", func(s Snapshot) bool { return len(s.OBS.SceneCameras) == 0 })
	h.mapChanged(groupMap(irlItem(1, true)))
	got := lastMap(h.settle())
	if en, found := func() (bool, bool) {
		for _, it := range got.Groups["G"].Items {
			if it.ID == 1 {
				return it.Enabled, true
			}
		}
		return false, false
	}(); !found || en {
		t.Errorf("グループの既知の項目1 = %v（found=%v）, want false", en, found)
	}
	if s := h.core.Snapshot(); len(s.OBS.SceneCameras) != 0 {
		t.Errorf("既知: scene_cameras = %v, want なし", s.OBS.SceneCameras)
	}

	// 新しい項目: グループの項目2は地図にまだない。false を覚えてから、読み取りが true で届く。項目1は表示に戻す。
	h.item("G", 2, false)
	h.item("G", 1, true)
	h.waitSnap("項目1が映る", func(s Snapshot) bool { return len(s.OBS.SceneCameras) == 1 })
	h.item("G", 1, false)
	h.waitSnap("また隠れる", func(s Snapshot) bool { return len(s.OBS.SceneCameras) == 0 })
	h.mapChanged(groupMap(irlItem(2, true)))
	got = lastMap(h.settle())
	for _, it := range got.Groups["G"].Items {
		if it.ID == 2 && it.Enabled {
			t.Errorf("グループの新しい項目2 = true, want 覚えた値 false")
		}
	}
	if s := h.core.Snapshot(); len(s.OBS.SceneCameras) != 0 {
		t.Errorf("新しい項目: scene_cameras = %v, want なし（項目2は false）", s.OBS.SceneCameras)
	}
}
