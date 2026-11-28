package core

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// S2-68: 消したカメラの hidden 記録（spec 7章「hidden.json」）。
// カメラを消す保存は新しい世代（設定に cam2 がない Core）を作り、hidden.json を読んで渡す。
// 照合できる記録は restore の理由で戻して消し、照合できない記録は触らずに残す。同じ番号を作ると、
// 残った記録は新しいカメラのものとして扱う（OK なら ok、「すべて表示に戻す」なら manual_all）。削除時の別の書き込みはしない。

// delEntry は cam2 の項目（Camera の 2）の「隠した」記録。
func delEntry(mod func(e *switcher.HiddenEntry)) switcher.HiddenEntry {
	e := switcher.HiddenEntry{CameraID: subCam, Host: testEndpoint.Host, Port: testEndpoint.Port, Collection: testCollection,
		ContainerUUID: camera, ItemID: 2, SourceUUID: "SUB", State: switcher.HiddenH, WrittenAt: time.Unix(1_700_000_000, 0).UTC()}
	if mod != nil {
		mod(&e)
	}
	return e
}

// subHiddenMap は SUB（cam2 の項目）が OBS で非表示のままの地図。
func subHiddenMap() *scenemap.Map {
	m := hideMap()
	sc := m.Scenes[camera]
	sc.Items[1].Enabled = false
	m.Scenes[camera] = sc
	return m
}

// delGenHarness は世代を作る。cams が空なら cam1 だけ（cam2 を消した後）。entries は、前の実行が残した hidden.json の記録。
// ファイルにも同じ記録を置く（本体はファイルを読んで HiddenRestored として渡す）。書き込みの回数は writes に数える。
func delGenHarness(t *testing.T, path string, hw *hiddenWrites, writes *atomic.Int32, withCam2 bool, entries []switcher.HiddenEntry) *harness {
	t.Helper()
	count := func(_ *switcher.Config, d *Deps) {
		inner := hw.wrap(d.HiddenWrite)
		d.HiddenWrite = func(p string, b []byte) error { writes.Add(1); return inner(p, b) }
	}
	opts := []option{withEndpoint, withHiddenPath(path), withRestored(entries...), count}
	if withCam2 {
		opts = append(opts, twoCameras)
	}
	return newHarness(t, opts...)
}

// putHidden は hidden.json を置く。
func putHidden(t *testing.T, path string, es ...switcher.HiddenEntry) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, encodeHidden(es, time.Unix(1_700_000_100, 0).UTC())); err != nil {
		t.Fatal(err)
	}
}

// (a) 照合できる記録は、消したカメラの項目として表示に戻して消す。理由は restore。世代を作っただけでは書かない（接続のあとの書き込みは、最初の地図の照合による）。
func TestDeletedCameraRecordRestored(t *testing.T) {
	path, hw := trackedHiddenPath(t)
	var writes atomic.Int32
	e := delEntry(nil)
	putHidden(t, path, e)
	h := delGenHarness(t, path, hw, &writes, false, []switcher.HiddenEntry{e})
	if writes.Load() != 0 {
		t.Fatalf("世代を作っただけで %d 回書いた", writes.Load())
	}
	h.itemWorld()
	h.start()
	h.connectMap(true, camera, subHiddenMap())
	h.waitCall("item:Camera:2:true")
	waitFile(t, path, func(es []fileEntry) bool { return len(es) == 0 })
	h.waitLog("camera shown")
	if log := h.log.String(); !strings.Contains(log, "camera=cam2") || !strings.Contains(log, "reason=restore") {
		t.Errorf("camera shown の camera=cam2 reason=restore がない:\n%s", log)
	}
}

// (b) 照合できない記録（OBS 未接続、空の地図、別のコレクション、別の接続先）は、触らずに残す。要求も出さない。
func TestDeletedCameraRecordKept(t *testing.T) {
	cases := []struct {
		name string
		mod  func(e *switcher.HiddenEntry)
		// connect が nil なら OBS につながない。
		connect func(h *harness)
		// rewrites は、同じ中身の書き直しを許すか（空の地図では、最初の接続で記録の置き場所が変わるだけで中身は同じ）。
		rewrites bool
	}{
		{name: "OBS 未接続"},
		{name: "空の地図", rewrites: true, connect: func(h *harness) {
			h.connectMap(true, camera, &scenemap.Map{Collection: testCollection})
		}},
		{name: "別のシーンコレクション", mod: func(e *switcher.HiddenEntry) { e.Collection = "other-collection" },
			connect: func(h *harness) { h.connectMap(true, camera, subHiddenMap()) }},
		{name: "別の接続先", mod: func(e *switcher.HiddenEntry) { e.Host = "other.local" },
			connect: func(h *harness) { h.connectMap(true, camera, subHiddenMap()) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path, hw := trackedHiddenPath(t)
			var writes atomic.Int32
			e := delEntry(c.mod)
			putHidden(t, path, e)
			before, _ := readFile(t, path)
			h := delGenHarness(t, path, hw, &writes, false, []switcher.HiddenEntry{e})
			h.itemWorld()
			h.start()
			if c.connect != nil {
				c.connect(h)
				h.waitSnap("接続", func(s Snapshot) bool { return s.OBS.Connected })
			}
			// 中核ループが記録を処理し終えて、書き込みがあればそれも終わるまで待つ。
			h.core.Snapshot()
			time.Sleep(50 * time.Millisecond)
			hw.waitIdle()
			if slices.ContainsFunc(h.obs.Calls(), func(s string) bool { return strings.HasPrefix(s, "item:") }) {
				t.Errorf("項目の要求を出した: %v", h.obs.Calls())
			}
			after, ok := readFile(t, path)
			if !ok || !reflect.DeepEqual(before, after) {
				t.Errorf("記録が変わった: before=%+v after=%+v", before, after)
			}
			if !c.rewrites && writes.Load() != 0 {
				t.Errorf("書き込みが %d 回あった", writes.Load())
			}
			if strings.Contains(h.log.String(), "camera shown") || strings.Contains(h.log.String(), "hidden item dropped") {
				t.Errorf("戻した・捨てたログがある:\n%s", h.log.String())
			}
		})
	}
}

// (c)(d) 残った記録のあとで、同じ番号（cam2）のカメラをまた作る。記録は新しいカメラのものとして扱う。
func TestRecreatedCameraInheritsKeptRecord(t *testing.T) {
	// 消した世代（cam2 なし）が、空の地図で照合できずに記録を残し、止まる。次の世代はファイルを読み直して作る。
	// 作り直した世代は cam2 が DOWN（hide）で配信中。記録は「悪いあいだは隠したまま」のものになる。
	setup := func(t *testing.T) (*harness, string) {
		path, hw := trackedHiddenPath(t)
		var writes atomic.Int32
		e := delEntry(nil)
		putHidden(t, path, e)
		gen1 := delGenHarness(t, path, hw, &writes, false, []switcher.HiddenEntry{e})
		gen1.itemWorld()
		gen1.start()
		gen1.connectMap(true, camera, &scenemap.Map{Collection: testCollection})
		gen1.waitSnap("接続", func(s Snapshot) bool { return s.OBS.Connected })
		gen1.cancel()
		<-gen1.runDone
		hw.waitIdle()
		kept, w := LoadHidden(path)
		if w != "" || len(kept) != 1 || kept[0].CameraID != subCam {
			t.Fatalf("残った記録 = %+v, %q", kept, w)
		}

		gen2 := delGenHarness(t, path, hw, &writes, true, kept)
		gen2.itemWorld()
		gen2.start()
		gen2.sampleSet(goodKbps, 0)
		gen2.connectMap(true, camera, subHiddenMap())
		for range 4 {
			gen2.sampleSet(goodKbps, 0)
		}
		gen2.waitSnap("cam1 OK", linkIs("OK"))
		gen2.waitSnap("cam2 の記録を引き継いだ", func(s Snapshot) bool { return len(s.Cameras) == 2 && s.Cameras[1].Hidden == 1 })
		if hasItemCall(gen2.obs.Calls(), "item:Camera:2:true") {
			t.Fatalf("cam2 が悪いのに戻した: %v", gen2.obs.Calls())
		}
		if es, _ := readFile(t, path); len(es) != 1 || es[0].Camera != subCam {
			t.Fatalf("hidden.json = %+v", es)
		}
		return gen2, path
	}

	t.Run("c OK になれば通常の理由 ok で戻して消す", func(t *testing.T) {
		h, path := setup(t)
		for range 5 {
			h.sampleSet(goodKbps, goodKbps)
		}
		h.waitSnap("cam2 OK", func(s Snapshot) bool { return s.Cameras[1].State == "OK" })
		h.waitCall("item:Camera:2:true")
		waitFile(t, path, func(es []fileEntry) bool { return len(es) == 0 })
		h.waitLog("camera shown")
		log := h.log.String()
		if !strings.Contains(log, "camera=cam2") || !strings.Contains(log, "reason=ok") || strings.Contains(log, "reason=restore") {
			t.Errorf("reason=ok のはず:\n%s", log)
		}
	})

	t.Run("d すべて表示に戻すでも消える", func(t *testing.T) {
		h, path := setup(t)
		if err, _ := h.command(switcher.CmdShowAllCameras); err != nil {
			t.Fatalf("ShowAll: %v", err)
		}
		h.waitCall("item:Camera:2:true")
		waitFile(t, path, func(es []fileEntry) bool { return len(es) == 0 })
		h.waitLog("camera shown")
		if log := h.log.String(); !strings.Contains(log, "camera=cam2") || !strings.Contains(log, "reason=manual_all") {
			t.Errorf("reason=manual_all のはず:\n%s", log)
		}
	})
}
