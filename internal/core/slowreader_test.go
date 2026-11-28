package core

import (
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

const slowWarnTail = "の映像を OBS が読み切れていません（MediaMTX が映像を捨てています）。つなぎ直した直後なら、しばらくで消えることがあります。続くときは、送るビットレートや解像度を下げてください。OBS のメディアソースの「ハードウェアデコードを使用」が切れていれば、入れてください。"

func TestSlowReaderWarnings(t *testing.T) {
	var mu sync.Mutex
	var slow []supervisor.SlowReader
	set := func(r ...supervisor.SlowReader) { mu.Lock(); slow = r; mu.Unlock() }
	h := newHarness(t, func(_ *switcher.Config, d *Deps) {
		c1 := testCamera(mainCam, "メイン", scenemap.MotionStandby)
		c1.Path = "live/cam1"
		c2 := testCamera("cam2", "サブ", scenemap.MotionStandby)
		c2.Path = "cam2" // app が空の1段の path
		d.Cameras = []Camera{c1, c2}
		d.WarningMessages = func() []i18n.Msg { return []i18n.Msg{i18n.Text("existing warning")} }
		d.SlowReaders = func() []supervisor.SlowReader {
			mu.Lock()
			defer mu.Unlock()
			return slices.Clone(slow)
		}
	})
	h.start()
	// 2台とも受信している（DOWN のカメラには警告を出さないため）
	h.sampleSet(3000, 3000)

	// 追いつかない path がなければ、既存の警告だけ。
	h.tick(h.clk.Now())
	if w := i18n.Strings(i18n.JA, h.core.Snapshot().OBS.Warnings); !slices.Equal(w, []string{"existing warning"}) {
		t.Fatalf("warnings = %v", w)
	}

	set(supervisor.SlowReader{Path: "live/cam1", Count: 4, Last: h.clk.Now()})
	h.tick(h.clk.Now())
	s := h.waitSnap("メインの警告", func(s Snapshot) bool { return len(s.OBS.Warnings) == 2 })
	if i18n.Strings(i18n.JA, s.OBS.Warnings)[0] != "existing warning" || i18n.Strings(i18n.JA, s.OBS.Warnings)[1] != "メイン"+slowWarnTail {
		t.Errorf("warnings = %v", i18n.Strings(i18n.JA, s.OBS.Warnings))
	}
	// 健康には入れない
	if s.Cameras[0].State != s.Cameras[1].State {
		t.Errorf("健康が変わった: %+v", s.Cameras)
	}

	// 1段の path のカメラ、2台とも。出る順はカメラの順
	set(supervisor.SlowReader{Path: "cam2", Count: 3, Last: h.clk.Now()}, supervisor.SlowReader{Path: "live/cam1", Count: 5, Last: h.clk.Now()})
	h.tick(h.clk.Now())
	s = h.waitSnap("2台の警告", func(s Snapshot) bool { return len(s.OBS.Warnings) == 3 })
	if i18n.Strings(i18n.JA, s.OBS.Warnings)[1] != "メイン"+slowWarnTail || i18n.Strings(i18n.JA, s.OBS.Warnings)[2] != "サブ"+slowWarnTail {
		t.Errorf("warnings = %v", i18n.Strings(i18n.JA, s.OBS.Warnings))
	}

	// 解ける
	set()
	h.tick(h.clk.Now())
	h.waitSnap("警告が消える", func(s Snapshot) bool { return len(s.OBS.Warnings) == 1 })

	out := h.log.String()
	for _, want := range []string{
		"msg=\"obs reader too slow\" camera=cam1 count=4",
		"msg=\"obs reader too slow\" camera=cam2 count=3",
		"msg=\"obs reader caught up\" camera=cam1 count=0",
		"msg=\"obs reader caught up\" camera=cam2 count=0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ログに %q がない:\n%s", want, out)
		}
	}
	// 続けて追いつかないあいだは、1行ずつ（繰り返さない）
	if n := strings.Count(out, "obs reader too slow camera=cam1"); n > 1 {
		t.Errorf("cam1 の too slow が %d 回", n)
	}
}

// 止まった（DOWN の）カメラには警告を出さない。DOWN のあいだに残っていた古い記録では、つなぎ直したあとにも出さない。
// つなぎ直したあとに新しい reader is too slow が来たら、また出す（T-15 の2日目: カメラが止まっても約60秒残った）。
func TestSlowReaderWarningClearsWhenCameraDown(t *testing.T) {
	var mu sync.Mutex
	var slow []supervisor.SlowReader
	set := func(r ...supervisor.SlowReader) { mu.Lock(); slow = r; mu.Unlock() }
	h := newHarness(t, func(_ *switcher.Config, d *Deps) {
		c1 := testCamera(mainCam, "メイン", scenemap.MotionStandby)
		c1.Path = "live/cam1"
		d.Cameras = []Camera{c1}
		d.SlowReaders = func() []supervisor.SlowReader {
			mu.Lock()
			defer mu.Unlock()
			return slices.Clone(slow)
		}
	})
	h.start()

	// DOWN のまま（まだ受信していない）なら出さない
	set(supervisor.SlowReader{Path: "live/cam1", Count: 3, Last: h.clk.Now()})
	h.tick(h.clk.Now())
	if w := i18n.Strings(i18n.JA, h.core.Snapshot().OBS.Warnings); len(w) != 0 {
		t.Fatalf("DOWN のカメラに警告: %v", w)
	}

	// 受信して LOW になると出る。DOWN の間の publish（downAt を書く）が時計を進めたあとに遅れて走ることがあるので、
	// LOW が公開されたのを見てから時計を進め、それより新しい記録にする
	h.sample(3000)
	h.waitSnap("LOW", func(s Snapshot) bool { return s.Cameras[0].State == "LOW" })
	h.clk.Advance(time.Second)
	set(supervisor.SlowReader{Path: "live/cam1", Count: 3, Last: h.clk.Now()})
	h.tick(h.clk.Now())
	h.waitSnap("警告が出る", func(s Snapshot) bool { return len(s.OBS.Warnings) == 1 })

	// 止まって DOWN になると、記録が残っていても消える
	h.samplesN(5, 0)
	h.waitSnap("DOWN で消える", func(s Snapshot) bool {
		return s.Cameras[0].State == "DOWN" && len(s.OBS.Warnings) == 0
	})
	if out := h.log.String(); !strings.Contains(out, `msg="obs reader caught up" camera=cam1 count=0 reason=camera_down`) {
		t.Errorf("ログに camera_down の caught up がない:\n%s", out)
	}

	// つなぎ直しても、古い記録（Last が DOWN の前）では出さない
	h.sample(3000)
	h.tick(h.clk.Now())
	if s := h.waitSnap("つなぎ直し", func(s Snapshot) bool { return s.Cameras[0].State == "LOW" }); len(s.OBS.Warnings) != 0 {
		t.Fatalf("古い記録で警告: %v", i18n.Strings(i18n.JA, s.OBS.Warnings))
	}

	// 新しい reader is too slow が来たら、また出る
	h.clk.Advance(time.Second)
	set(supervisor.SlowReader{Path: "live/cam1", Count: 4, Last: h.clk.Now()})
	h.tick(h.clk.Now())
	h.waitSnap("新しい記録で出る", func(s Snapshot) bool { return len(s.OBS.Warnings) == 1 })
	if n := strings.Count(h.log.String(), `msg="obs reader too slow" camera=cam1`); n != 2 {
		t.Errorf("too slow のログが %d 回（2回のはず）", n)
	}
}
