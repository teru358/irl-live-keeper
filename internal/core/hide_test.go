package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// ---- 0b（M-12）の試験の道具 ----

const testCollection = "col"

var testEndpoint = switcher.Endpoint{Host: "127.0.0.1", Port: 4455}

// hideMap は sub（on_bad: hide）のメディアソース SUB を、Camera の項目 2 に置いた地図（main の IRL は項目 1）。
// Camera2 は main だけ。hideMap2 は Camera2 にも SUB を置く。
func hideMap() *scenemap.Map {
	return hideMapWith(false)
}

func hideMap2() *scenemap.Map { return hideMapWith(true) }

func hideMapWith(subInCamera2 bool) *scenemap.Map {
	cam := func(name string, withSub bool) scenemap.Container {
		items := []scenemap.Item{{ID: 1, SourceUUID: "IRL", SourceKind: scenemap.InputSource, Enabled: true}}
		if withSub {
			items = append(items, scenemap.Item{ID: 2, SourceUUID: "SUB", SourceKind: scenemap.InputSource, Enabled: true})
		}
		return scenemap.Container{Name: name, Items: items}
	}
	return &scenemap.Map{
		Collection: testCollection,
		Scenes: map[string]scenemap.Container{
			standby: {Name: standby}, camera: cam(camera, true), camera2: cam(camera2, subInCamera2), talk: {Name: talk},
		},
		Groups: map[string]scenemap.Container{},
		Inputs: map[string]scenemap.Input{
			"IRL": {Name: "IRL", CameraID: mainCam},
			"SUB": {Name: "SUB", CameraID: subCam},
		},
	}
}

func withEndpoint(c *switcher.Config, _ *Deps) { c.Endpoint = testEndpoint }

func withHiddenPath(p string) option {
	return func(_ *switcher.Config, d *Deps) { d.HiddenPath = p }
}

func withHiddenWrite(f func(path string, data []byte) error) option {
	return func(_ *switcher.Config, d *Deps) { d.HiddenWrite = f }
}

func withHiddenRetry(dur time.Duration) option {
	return func(_ *switcher.Config, d *Deps) { d.HiddenRetry = dur }
}

func withStopTimeout(dur time.Duration) option {
	return func(_ *switcher.Config, d *Deps) { d.StopTimeout = dur }
}

func withStudioHide(v string) option {
	return func(_ *switcher.Config, d *Deps) { d.StudioModeHide = v }
}

func withRestored(es ...switcher.HiddenEntry) option {
	return func(_ *switcher.Config, d *Deps) { d.HiddenRestored = es }
}

// hideHarness は main（standby）と sub（hide）の2台で、hidden.json を一時のフォルダに置く。
func hideHarness(t *testing.T, opts ...option) (*harness, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state", "hidden.json")
	all := append([]option{twoCameras, withEndpoint, withHiddenPath(path)}, opts...)
	return newHarness(t, all...), path
}

func (h *harness) connectMap(streaming bool, scene string, m *scenemap.Map) {
	h.t.Helper()
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Streaming: streaming, Scene: scene, SceneUUID: scene, Map: m})
}

// itemWorld は偽の OBS の項目の応答を、適用して ItemEnabledChanged を流す形にする。
func (h *harness) itemWorld() {
	h.obs.onItem = func(scene string, id int, en bool) error {
		h.obs.emit(switcher.ItemEnabledChanged{At: h.clk.Now(), SceneUUID: scene, ItemID: id, Enabled: en})
		return nil
	}
}

// mainOKSubDown は main を OK、sub を DOWN にする。
func (h *harness) mainOKSubDown() {
	h.t.Helper()
	for range 5 {
		h.sampleSet(goodKbps, 0)
	}
	h.waitSnap("main OK", linkIs("OK"))
}

// hideSub は配信中の Camera へつなぎ、sub の項目が隠れて hidden.json の記録が「隠した」になるまで進める。
func (h *harness) hideSub(path string) {
	h.t.Helper()
	h.itemWorld()
	h.start()
	h.mainOKSubDown()
	h.connectMap(true, camera, hideMap())
	h.waitCall("item:Camera:2:false")
	h.waitSnap("sub が隠れた", func(s Snapshot) bool { return s.Cameras[1].Hidden == 1 })
	waitFile(h.t, path, func(es []fileEntry) bool { return len(es) == 1 && es[0].State == "hidden" })
}

// fileEntry は hidden.json の1件（試験は core の型を使わず、ファイルの形を読む）。
type fileEntry struct {
	Camera     string  `json:"camera"`
	Host       string  `json:"host"`
	Port       int     `json:"port"`
	Collection string  `json:"collection"`
	Container  string  `json:"container"`
	Item       int     `json:"item"`
	Source     string  `json:"source"`
	State      string  `json:"state"`
	SettledAt  *string `json:"settled_at"`
	WrittenAt  string  `json:"written_at"`
}

func readFile(t *testing.T, path string) ([]fileEntry, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	es, err := parseHiddenFile(b)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return es, true
}

// parseHiddenFile は hidden.json の中身を、試験側の型で読む（core の型を使わない）。
func parseHiddenFile(b []byte) ([]fileEntry, error) {
	var f struct {
		Version int         `json:"version"`
		Entries []fileEntry `json:"entries"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("hidden.json が JSON でない: %v: %s", err, b)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("version = %d", f.Version)
	}
	return f.Entries, nil
}

func waitFile(t *testing.T, path string, pred func([]fileEntry) bool) []fileEntry {
	t.Helper()
	deadline := time.Now().Add(wait)
	for {
		es, _ := readFile(t, path)
		if pred(es) {
			return es
		}
		if time.Now().After(deadline) {
			t.Fatalf("hidden.json が条件を満たさない: %+v", es)
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *harness) waitLog(sub string) {
	h.t.Helper()
	deadline := time.Now().Add(wait)
	for !strings.Contains(h.log.String(), sub) {
		if time.Now().After(deadline) {
			h.t.Fatalf("ログに %q が出ない:\n%s", sub, h.log.String())
		}
		time.Sleep(time.Millisecond)
	}
}

func hasItemCall(calls []string, prefix string) bool {
	return slices.ContainsFunc(calls, func(c string) bool { return strings.HasPrefix(c, prefix) })
}

// ---- つなぎを外した ----

// on_bad: hide が standby に直されず、false を送る前に hidden.json に「隠しかけ」を書き、そのあと「隠した」になる。
func TestHideWritesRecordBeforeFalse(t *testing.T) {
	h, path := hideHarness(t)
	var mu sync.Mutex
	var atCall []fileEntry
	var seenFalse bool
	h.obs.onItem = func(scene string, id int, en bool) error {
		if !en && !seenFalse {
			mu.Lock()
			seenFalse = true
			atCall, _ = readFile(t, path)
			mu.Unlock()
		}
		h.obs.emit(switcher.ItemEnabledChanged{At: h.clk.Now(), SceneUUID: scene, ItemID: id, Enabled: en})
		return nil
	}
	h.start()
	h.mainOKSubDown()
	h.connectMap(true, camera, hideMap())
	h.waitCall("item:Camera:2:false")
	waitFile(t, path, func(es []fileEntry) bool { return len(es) == 1 && es[0].State == "hidden" })

	mu.Lock()
	got := atCall
	mu.Unlock()
	if len(got) != 1 || got[0].State != "pending" || got[0].Camera != subCam || got[0].Container != camera ||
		got[0].Item != 2 || got[0].Source != "SUB" || got[0].Collection != testCollection ||
		got[0].Host != testEndpoint.Host || got[0].Port != testEndpoint.Port {
		t.Fatalf("false を送った時点の hidden.json = %+v, want sub の項目 (Camera,2) の pending", got)
	}
	if calls := h.obs.Calls(); hasItemCall(calls, "scene:") && countPrefix(calls, "scene:") > 0 {
		t.Errorf("sub を隠す動きで、シーンの切替も出た: %v", calls)
	}
}

// ---- 実行役: 結果の分類と ReqID ----

func TestExecutorClassifiesItemResult(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want switcher.ItemResult
	}{
		{"OK", nil, switcher.ItemOK},
		{"Timeout", fmt.Errorf("%w: SetSceneItemEnabled", obs.ErrTimeout), switcher.ItemTimeout},
		{"NotFound", errors.New("request SetSceneItemEnabled: ResourceNotFound (600): no item"), switcher.ItemNotFound},
		{"Error", errors.New("boom"), switcher.ItemError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, _ := hideHarness(t)
			h.obs.onItem = func(string, int, bool) error { return c.err }
			h.start()
			h.mainOKSubDown()
			h.connectMap(true, camera, hideMap())
			got := h.waitSeen("ItemRequestDone", func(v any) bool { _, ok := v.(switcher.ItemRequestDone); return ok })
			var done switcher.ItemRequestDone
			var proceed switcher.Proceed
			for _, v := range got {
				switch e := v.(type) {
				case switcher.ItemRequestDone:
					done = e
				case approval:
					if p, ok := e.Result.(switcher.Proceed); ok {
						if _, isItem := e.Action.(switcher.SetItemEnabled); isItem {
							proceed = p
						}
					}
				}
			}
			if done.Result != c.want {
				t.Errorf("Result = %v, want %v", done.Result, c.want)
			}
			if proceed.ItemReqID == 0 || done.ReqID != proceed.ItemReqID {
				t.Errorf("ReqID = %d, Proceed.ItemReqID = %d", done.ReqID, proceed.ItemReqID)
			}
		})
	}
}

// 応答が来ないうちに切れたら、OBS が別のエラーを返しても Timeout にする（適用されたかわからない）。
func TestExecutorDisconnectMidRequestIsTimeout(t *testing.T) {
	h, _ := hideHarness(t)
	release := make(chan struct{})
	h.obs.onItem = func(string, int, bool) error {
		h.obs.gate(release)
		return errors.New("use of closed network connection")
	}
	h.start()
	h.mainOKSubDown()
	h.connectMap(true, camera, hideMap())
	h.waitCall("item:Camera:2:false")
	h.obsEvent(switcher.OBSDisconnected{At: h.clk.Now()})
	h.settle()
	close(release)
	got := h.waitSeen("ItemRequestDone", func(v any) bool { _, ok := v.(switcher.ItemRequestDone); return ok })
	done := got[len(got)-1].(switcher.ItemRequestDone)
	if done.Result != switcher.ItemTimeout {
		t.Fatalf("Result = %v, want Timeout", done.Result)
	}
}

// 1つの項目が失敗しても打ち切らず次の項目へ進み、BatchDone は Completed で Err を持つ。
func TestExecutorContinuesAfterItemFailure(t *testing.T) {
	h, _ := hideHarness(t)
	h.obs.onItem = func(scene string, id int, en bool) error {
		if scene == camera {
			return errors.New("boom")
		}
		h.obs.emit(switcher.ItemEnabledChanged{At: h.clk.Now(), SceneUUID: scene, ItemID: id, Enabled: en})
		return nil
	}
	h.start()
	h.mainOKSubDown()
	h.connectMap(true, camera, hideMap2())
	got := h.waitSeen("BatchDone", func(v any) bool {
		bd, ok := v.(switcher.BatchDone)
		return ok && bd.Result == switcher.BatchCompleted
	})
	bd := got[len(got)-1].(switcher.BatchDone)
	if bd.Err == nil {
		t.Errorf("BatchDone.Err = nil, want 失敗を持つ")
	}
	calls := h.obs.Calls()
	if !slices.Contains(calls, "item:Camera:2:false") || !slices.Contains(calls, "item:Camera2:2:false") {
		t.Errorf("両方の項目を送らなかった: %v", calls)
	}
}

// ---- hidden.json を書けないとき ----

func TestMustPersistFailureRejectsAndWarns(t *testing.T) {
	h, _ := hideHarness(t, withHiddenWrite(func(string, []byte) error { return errors.New("disk full") }))
	h.start()
	h.mainOKSubDown()
	h.connectMap(true, camera, hideMap())
	h.waitSeen("却下", isBatchDone(switcher.BatchRejected))
	s := h.waitSnap("last_error", func(s Snapshot) bool { return s.LastError != nil })
	if !strings.Contains(s.LastError.Message, "隠した項目を記録できないため、隠すのを止めました") {
		t.Errorf("last_error = %q", s.LastError.Message)
	}
	if !slices.ContainsFunc(s.OBS.Warnings, func(w string) bool { return strings.Contains(w, "hidden.json") }) {
		t.Errorf("warnings = %v, want hidden.json を書けない旨", s.OBS.Warnings)
	}
	if calls := h.obs.Calls(); hasItemCall(calls, "item:") {
		t.Errorf("記録できないのに OBS へ送った: %v", calls)
	}
}

// 同期の保存（MustPersist）は上限1秒。書き込みが戻らなくても1秒で諦めて Reject にし、そのあとも中核ループは動く。
func TestMustPersistTimeoutIs1s(t *testing.T) {
	block := make(chan struct{})
	h, _ := hideHarness(t, withHiddenWrite(func(string, []byte) error { <-block; return nil }))
	t.Cleanup(func() { close(block) })
	h.start()
	h.mainOKSubDown()
	start := time.Now()
	h.connectMap(true, camera, hideMap())
	h.waitSeen("却下", isBatchDone(switcher.BatchRejected))
	if d := time.Since(start); d < 900*time.Millisecond || d > 1500*time.Millisecond {
		t.Errorf("却下まで %v かかった（上限1秒のはず）", d)
	}
	if calls := h.obs.Calls(); hasItemCall(calls, "item:") {
		t.Errorf("保存できないのに送った: %v", calls)
	}
	// 諦めたあとは、中核ループは通常どおり応答する。
	t0 := time.Now()
	h.settle()
	if d := time.Since(t0); d > 300*time.Millisecond {
		t.Errorf("諦めたあとの Tick の処理に %v", d)
	}
	s := h.waitSnap("警告", func(s Snapshot) bool { return s.LastError != nil })
	if !strings.Contains(s.LastError.Message, "隠した項目を記録できないため") {
		t.Errorf("last_error = %q", s.LastError.Message)
	}
}

// 通常の保存（「隠した」への更新）が失敗しても警告を出し、一定の間隔で書き直す（H-06）。
func TestAsyncPersistFailureRetries(t *testing.T) {
	var mu sync.Mutex
	failed := 0 // 失敗させた書き込みの数
	var failing bool
	var real func(string, []byte) error = atomicWrite
	// 失敗させるのは「隠した」を含む中身の書き込みだけ。隠す前の同期の保存（pending）は通す。
	// 書き込みの順や回数には頼らない（非同期の保存と同期の保存の順は決まっていない）。
	h, path := hideHarness(t, withHiddenRetry(20*time.Millisecond), withHiddenWrite(func(p string, b []byte) error {
		es, err := decodeHidden(b)
		mu.Lock()
		f := failing
		hiddenState := err == nil && slices.ContainsFunc(es, func(e switcher.HiddenEntry) bool { return e.State == switcher.HiddenH })
		if f && hiddenState {
			failed++
			mu.Unlock()
			return errors.New("disk full")
		}
		mu.Unlock()
		return real(p, b)
	}))
	mu.Lock()
	failing = true
	mu.Unlock()
	h.itemWorld()
	h.start()
	h.mainOKSubDown()
	h.connectMap(true, camera, hideMap())
	h.waitCall("item:Camera:2:false")
	h.waitSnap("書けない警告", func(s Snapshot) bool {
		h.tick(h.clk.Advance(time.Millisecond)) // 警告は Snapshot の作り直しで付く（保存の失敗だけでは作り直されない）
		return slices.ContainsFunc(s.OBS.Warnings, func(w string) bool { return strings.Contains(w, "hidden.json") })
	})
	// 書き直し（再試行）が実際に起きる（失敗が2回以上）まで待ってから、書けるようにする
	deadline := time.Now().Add(wait)
	for {
		mu.Lock()
		n := failed
		mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("書き直しが起きない: 失敗 %d 回", n)
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	failing = false
	mu.Unlock()
	h.tick(h.clk.Advance(time.Second)) // 警告は Snapshot の作り直しで消える
	waitFile(t, path, func(es []fileEntry) bool { return len(es) == 1 && es[0].State == "hidden" })
	h.waitSnap("警告が消える", func(s Snapshot) bool {
		h.tick(h.clk.Advance(time.Millisecond))
		return !slices.ContainsFunc(s.OBS.Warnings, func(w string) bool { return strings.Contains(w, "hidden.json") })
	})
}

// ---- hidden.json の読み込み ----

func TestLoadHidden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "hidden.json")

	if es, w := LoadHidden(path); es != nil || w != "" {
		t.Errorf("ファイルがない: %v, %q", es, w)
	}

	settled := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	want := []switcher.HiddenEntry{
		{CameraID: "sub", Host: "h", Port: 4455, Collection: "c", ContainerUUID: "S1", ItemID: 2, SourceUUID: "U1",
			State: switcher.HiddenH, WrittenAt: settled},
		{CameraID: "old", Host: "h", Port: 4455, Collection: "c", ContainerUUID: "S2", ItemID: 7, SourceUUID: "U2",
			State: switcher.HiddenU, SettledAt: &settled, WrittenAt: settled},
		{CameraID: "sub", Host: "h", Port: 4455, Collection: "c", ContainerUUID: "S3", ItemID: 1, SourceUUID: "U3",
			State: switcher.HiddenP, WrittenAt: settled},
	}
	if err := atomicWrite(path, encodeHidden(want, settled)); err != nil {
		t.Fatal(err)
	}
	es, w := LoadHidden(path)
	if w != "" || len(es) != 3 {
		t.Fatalf("読めない: %v, %q", es, w)
	}
	for i := range want {
		if es[i].CameraID != want[i].CameraID || es[i].Key() != want[i].Key() || es[i].State != want[i].State ||
			es[i].SourceUUID != want[i].SourceUUID || es[i].Collection != want[i].Collection ||
			es[i].Host != want[i].Host || es[i].Port != want[i].Port || !es[i].WrittenAt.Equal(want[i].WrittenAt) ||
			(es[i].SettledAt == nil) != (want[i].SettledAt == nil) {
			t.Errorf("[%d] = %+v, want %+v", i, es[i], want[i])
		}
	}

	// 壊れたファイルは空として扱い、警告を返し、hidden.json.broken に名前を替えて残す。
	if err := os.WriteFile(path, []byte(`{"version":1,"entries":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	es, w = LoadHidden(path)
	if es != nil || w == "" {
		t.Errorf("壊れたファイル: %v, %q", es, w)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("壊れたファイルが元の名前に残っている: %v", err)
	}
	if b, err := os.ReadFile(path + ".broken"); err != nil || !strings.Contains(string(b), `"entries":[`) {
		t.Errorf(".broken = %q, %v", b, err)
	}
}

// 知らない version のファイルも壊れたものとして扱う（空で始め、元の中身は .broken に残す）。
func TestLoadHiddenUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hidden.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	es, w := LoadHidden(path)
	if es != nil || w == "" {
		t.Errorf("= %v, %q", es, w)
	}
	if _, err := os.Stat(path + ".broken"); err != nil {
		t.Errorf(".broken がない: %v", err)
	}
}

// 起動で読んだ記録は HiddenRestored として渡り、最初の地図で照合される。
// sub が OK なら「戻してよい」ので、非表示のままの項目は表示に戻る。
func TestRestoredEntryIsShownBack(t *testing.T) {
	entry := switcher.HiddenEntry{CameraID: subCam, Host: testEndpoint.Host, Port: testEndpoint.Port, Collection: testCollection,
		ContainerUUID: camera, ItemID: 2, SourceUUID: "SUB", State: switcher.HiddenH, WrittenAt: time.Now()}
	h, path := hideHarness(t, withRestored(entry))
	// 前回の実行で書いたファイル（すでに外した記録）を、同じ内容で置いておく。
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, encodeHidden([]switcher.HiddenEntry{entry}, time.Now())); err != nil {
		t.Fatal(err)
	}
	h.itemWorld()
	h.start()
	for range 5 {
		h.sampleSet(goodKbps, goodKbps)
	}
	h.waitSnap("sub OK", func(s Snapshot) bool { return s.Cameras[1].State == "OK" })
	m := hideMap()
	sc := m.Scenes[camera]
	sc.Items[1].Enabled = false // 落ちる前に隠した状態のまま
	m.Scenes[camera] = sc
	h.connectMap(true, camera, m)
	h.waitCall("item:Camera:2:true")
	waitFile(t, path, func(es []fileEntry) bool { return len(es) == 0 })
	h.waitLog("camera shown")
}

// 読めなかったときの警告（Deps.HiddenLoadWarning）は status の warnings に出る。
func TestLoadWarningShownInStatus(t *testing.T) {
	h, _ := hideHarness(t, func(_ *switcher.Config, d *Deps) {
		d.HiddenLoadWarning = "hidden.json を読めなかったため、空として扱いました"
	})
	s := h.core.Snapshot()
	if !slices.ContainsFunc(s.OBS.Warnings, func(w string) bool { return strings.Contains(w, "hidden.json を読めなかった") }) {
		t.Errorf("warnings = %v", s.OBS.Warnings)
	}
}

// ---- バッチの列の入れ方 ----

func TestEnqueueOrder(t *testing.T) {
	b := func(p switcher.Purpose, scope switcher.RestoreScope) *switcher.Batch {
		return &switcher.Batch{Purpose: p, RestoreScope: scope}
	}
	var names []string
	order := func(c *Core) string {
		names = names[:0]
		for _, q := range c.queue {
			names = append(names, q.Purpose.String())
		}
		return strings.Join(names, ",")
	}
	c := &Core{}
	c.enqueue(b(switcher.PurposeHideCamera, switcher.RestoreNone))
	c.enqueue(b(switcher.PurposeAutoFail, switcher.RestoreNone))
	if got := order(c); got != "AutoFail,HideCamera" {
		t.Errorf("シーンの切替は隠すより前: %s", got)
	}
	c.enqueue(b(switcher.PurposeShowCamera, switcher.RestoreNone))
	c.enqueue(b(switcher.PurposeStandbyOn, switcher.RestoreNone))
	if got := order(c); got != "AutoFail,StandbyOn,HideCamera,ShowCamera" {
		t.Errorf("= %s", got)
	}
	c.enqueue(b(switcher.PurposeRestoreAll, switcher.RestoreTrackedItems))
	if got := order(c); got != "RestoreAll,AutoFail,StandbyOn,HideCamera,ShowCamera" {
		t.Errorf("停止の RestoreAll は先頭: %s", got)
	}
	c.enqueue(b(switcher.PurposeRestoreAll, switcher.RestoreAllCameraItems))
	if got := order(c); got != "RestoreAll,AutoFail,StandbyOn,HideCamera,ShowCamera,RestoreAll" {
		t.Errorf("手動の RestoreAll は隠す・戻すの末尾: %s", got)
	}
}

// ---- Stopping ----

func TestStoppingRestoresAndWritesFile(t *testing.T) {
	h, path := hideHarness(t)
	h.hideSub(path)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	h.core.Stopping(ctx)
	if d := time.Since(start); d > time.Second {
		t.Errorf("Stopping が %v かかった（RestoreAll の完了で戻るはず）", d)
	}
	if !slices.Contains(h.obs.Calls(), "item:Camera:2:true") {
		t.Errorf("true を送っていない: %v", h.obs.Calls())
	}
	if es, ok := readFile(t, path); !ok || len(es) != 0 {
		t.Errorf("戻ったあとの hidden.json = %+v, ok=%v", es, ok)
	}
	if err, _ := h.command(switcher.CmdStandbyOn); !errors.As(err, new(switcher.ErrInvalidState)) {
		t.Errorf("終了中の Command = %v, want ErrInvalidState", err)
	}
	if !strings.Contains(h.log.String(), "reason=shutdown") {
		t.Errorf("camera shown の reason=shutdown がない:\n%s", h.log.String())
	}
}

// OBS が true に応えなければ、最大 StopTimeout で戻る。記録は hidden.json に残る。
func TestStoppingGivesUpAfterTimeout(t *testing.T) {
	h, path := hideHarness(t, withStopTimeout(150*time.Millisecond))
	h.hideSub(path)
	h.obs.onItem = func(string, int, bool) error { h.obs.gate(nil); return nil }
	start := time.Now()
	h.core.Stopping(context.Background())
	d := time.Since(start)
	if d < 100*time.Millisecond || d > time.Second {
		t.Errorf("Stopping の待ち = %v, want 約150ms", d)
	}
	if es, _ := readFile(t, path); len(es) != 1 {
		t.Errorf("hidden.json = %+v, want 記録が残る", es)
	}
}

func TestStoppingWhenNotConnectedReturnsImmediately(t *testing.T) {
	h, _ := hideHarness(t)
	h.start()
	start := time.Now()
	h.core.Stopping(context.Background())
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("未接続の Stopping が %v かかった", d)
	}
	if _, err := h.core.Snapshot(), error(nil); err != nil {
		t.Fatal(err)
	}
	// 2回目は何もしない。Run が止まったあとも戻る。
	h.core.Stopping(context.Background())
	h.cancel()
	<-h.runDone
	h.core.Stopping(context.Background())
}

// ---- すべて表示に戻す ----

func TestShowAllCommand(t *testing.T) {
	h, path := hideHarness(t)
	if err, _ := h.command(switcher.CmdShowAllCameras); !errors.As(err, new(switcher.ErrNotConnected)) {
		// Run の前は ErrStopped（まだ動いていない）。起動してから確かめる。
		t.Logf("Run 前: %v", err)
	}
	h.hideSub(path)
	if err, _ := h.command(switcher.CmdShowAllCameras); err != nil {
		t.Fatalf("ShowAll: %v", err)
	}
	h.waitCall("item:Camera:2:true")
	h.waitLog("camera shown")
	if !strings.Contains(h.log.String(), "reason=manual_all") {
		t.Errorf("reason=manual_all がない:\n%s", h.log.String())
	}
	h.obsEvent(switcher.OBSDisconnected{At: h.clk.Now()})
	h.waitSnap("切断", func(s Snapshot) bool { return !s.OBS.Connected })
	if err, _ := h.command(switcher.CmdShowAllCameras); !errors.As(err, new(switcher.ErrNotConnected)) {
		t.Errorf("未接続の ShowAll = %v, want ErrNotConnected", err)
	}
}

// ---- status とログ ----

func TestStatusHiddenFields(t *testing.T) {
	h, path := hideHarness(t)
	h.hideSub(path)
	s := h.core.Snapshot()
	sub := s.Cameras[1]
	if sub.Hidden != 1 || sub.UserShown || sub.Unusable || !slices.Equal(sub.Sources, []string{"SUB"}) {
		t.Errorf("sub = %+v", sub)
	}
	if main := s.Cameras[0]; main.Hidden != 0 || !slices.Equal(main.Sources, []string{"IRL"}) {
		t.Errorf("main = %+v", main)
	}
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	c1 := m["cameras"].([]any)[1].(map[string]any)
	for _, k := range []string{"sources", "hidden", "user_shown", "unusable"} {
		if _, ok := c1[k]; !ok {
			t.Errorf("cameras[1] に %q がない: %v", k, c1)
		}
	}
	if v, ok := m["obs"].(map[string]any)["studio_mode"]; !ok || v != false {
		t.Errorf("obs.studio_mode = %v, %v", v, ok)
	}
	// ソースのないカメラは [] （null にしない）
	h2 := newHarness(t, twoCameras)
	b, _ = json.Marshal(h2.core.Snapshot())
	if !strings.Contains(string(b), `"sources":[]`) {
		t.Errorf("sources が [] でない: %s", b)
	}
}

func TestStudioModeWarnings(t *testing.T) {
	cases := []struct{ mode, want string }{
		{"", "屋外配信では Studio Mode を勧めません。Studio Mode のため、隠す動きの代わりに待機シーンへ送っています"},
		{switcher.StudioHideStandby, "屋外配信では Studio Mode を勧めません。Studio Mode のため、隠す動きの代わりに待機シーンへ送っています"},
		{switcher.StudioHideIgnore, "屋外配信では Studio Mode を勧めません。Studio Mode のため、隠す動きを止めています"},
		{switcher.StudioHideHide, "屋外配信では Studio Mode を勧めません。Studio Mode でも隠しています（OBS の『シーンを複製』を切っていないと配信に効きません）"},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			h, _ := hideHarness(t, withStudioHide(c.mode))
			h.start()
			h.connectMap(false, camera, hideMap())
			h.waitSnap("接続", func(s Snapshot) bool { return s.OBS.Connected })
			if s := h.core.Snapshot(); s.OBS.StudioMode || len(s.OBS.Warnings) != 0 {
				t.Errorf("Studio Mode オフなのに: %+v", s.OBS)
			}
			h.obsEvent(switcher.StudioModeChanged{At: h.clk.Now(), Enabled: true})
			s := h.waitSnap("Studio Mode", func(s Snapshot) bool { return s.OBS.StudioMode })
			if !slices.Contains(s.OBS.Warnings, c.want) {
				t.Errorf("warnings = %v, want %q", s.OBS.Warnings, c.want)
			}
			h.obsEvent(switcher.StudioModeChanged{At: h.clk.Now(), Enabled: false})
			s = h.waitSnap("Studio Mode オフ", func(s Snapshot) bool { return !s.OBS.StudioMode })
			if len(s.OBS.Warnings) != 0 {
				t.Errorf("オフに戻ったのに警告: %v", s.OBS.Warnings)
			}
		})
	}
	// OBSConnected の StudioMode も Machine に渡る。
	h, _ := hideHarness(t)
	h.start()
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Scene: camera, SceneUUID: camera, Map: hideMap(), StudioMode: true})
	h.waitSnap("接続時の Studio Mode", func(s Snapshot) bool { return s.OBS.StudioMode })
}

// 別のコレクションの記録は触らずに残し、件数を警告に出す。
func TestRetainedEntriesWarning(t *testing.T) {
	other := switcher.HiddenEntry{CameraID: subCam, Host: testEndpoint.Host, Port: testEndpoint.Port, Collection: "別のコレクション",
		ContainerUUID: "X", ItemID: 1, SourceUUID: "Y", State: switcher.HiddenH, WrittenAt: time.Now()}
	h, path := hideHarness(t, withRestored(other))
	h.start()
	h.connectMap(false, camera, hideMap())
	s := h.waitSnap("接続", func(s Snapshot) bool { return s.OBS.Connected })
	if !slices.ContainsFunc(s.OBS.Warnings, func(w string) bool { return strings.Contains(w, "記録が 1 件") }) {
		t.Errorf("warnings = %v", s.OBS.Warnings)
	}
	if es, ok := readFile(t, path); ok && len(es) != 0 && es[0].Collection != "別のコレクション" {
		t.Errorf("hidden.json = %+v", es)
	}
}

func TestHideShowLogs(t *testing.T) {
	h, path := hideHarness(t)
	h.hideSub(path)
	if !strings.Contains(h.log.String(), `msg="camera hidden" camera=sub items=1 reason=down`) {
		t.Errorf("camera hidden の行がない:\n%s", h.log.String())
	}
	// 配信の停止で戻る（reason=stream_stop）。Hide と Show は前回の送出から5秒あける。
	h.clk.Advance(6 * time.Second)
	h.obsEvent(switcher.StreamStateChanged{At: h.clk.Now(), Streaming: false})
	h.waitCall("item:Camera:2:true")
	h.waitLog(`msg="camera shown" camera=sub items=1 reason=stream_stop`)
}

func TestShowLogReasonOK(t *testing.T) {
	h, path := hideHarness(t)
	h.hideSub(path)
	for range 5 {
		h.sampleSet(goodKbps, goodKbps)
	}
	h.waitCall("item:Camera:2:true")
	h.waitLog(`msg="camera shown" camera=sub items=1 reason=ok`)
}

// SetItemEnabled の失敗でも、中核ループは止まらず、goroutine は残らない（Run が戻る）。
func TestHideDoesNotLeakOnShutdown(t *testing.T) {
	h, _ := hideHarness(t)
	h.obs.onItem = func(string, int, bool) error { h.obs.gate(nil); return nil }
	h.start()
	h.mainOKSubDown()
	h.connectMap(true, camera, hideMap())
	h.waitCall("item:Camera:2:false")
	h.cancel()
	select {
	case <-h.runDone:
	case <-time.After(wait):
		t.Fatal("OBS の応答待ちの間に ctx を止めても Run が戻らない")
	}
}

var _ = ingest.SampleSet{}

// 非同期の保存が止まったままでも、取り消したあと Run は期限の内に戻る（ディスクの処理だけが残る）。
func TestRunReturnsWhenAsyncWriteIsStuck(t *testing.T) {
	var n atomic.Int32
	var active atomic.Int32 // 書き込みの関数の中にいる数（止められているものを含む）
	block := make(chan struct{})
	h, path := hideHarness(t, withHiddenWrite(func(p string, b []byte) error {
		active.Add(1)
		defer active.Add(-1)
		if n.Add(1) > 1 { // 最初の同期の保存だけ通し、あとの非同期の保存は止める
			<-block
		}
		return atomicWrite(p, b)
	}))
	// 後片づけは逆順に走る。TempDir の削除（先に登録済み）より前に、止めた書き込みを解いて、
	// 終わるのを待つ（待たないと、解かれた書き込みが削除の最中にファイルを作り、RemoveAll が落ちる）。
	t.Cleanup(func() {
		close(block)
		deadline := time.Now().Add(wait)
		for active.Load() > 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if active.Load() > 0 {
			t.Error("止めた書き込みが終わらない")
		}
	})
	_ = path
	h.itemWorld()
	h.start()
	h.mainOKSubDown()
	h.connectMap(true, camera, hideMap())
	h.waitCall("item:Camera:2:false")
	h.waitSnap("隠した", func(s Snapshot) bool { return s.Cameras[1].Hidden == 1 })
	deadline := time.Now().Add(wait)
	for n.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	h.cancel()
	select {
	case <-h.runDone:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("非同期の保存が止まっていると Run が戻らない")
	}
}
