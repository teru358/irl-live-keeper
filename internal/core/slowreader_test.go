package core

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

const slowWarnTail = "の映像を OBS が読み切れていません（MediaMTX が映像を捨てています）。送るビットレートや解像度を下げるか、OBS のメディアソースの「ハードウェアデコードを使用」を入れてください"

func TestSlowReaderWarnings(t *testing.T) {
	var mu sync.Mutex
	var slow []supervisor.SlowReader
	set := func(r ...supervisor.SlowReader) { mu.Lock(); slow = r; mu.Unlock() }
	h := newHarness(t, func(_ *switcher.Config, d *Deps) {
		c1 := testCamera(mainCam, "メイン", scenemap.MotionStandby)
		c1.Path = "live/main"
		c2 := testCamera("sub", "サブ", scenemap.MotionStandby)
		c2.Path = "sub" // app が空の1段の path
		d.Cameras = []Camera{c1, c2}
		d.Warnings = func() []string { return []string{"既存の警告"} }
		d.SlowReaders = func() []supervisor.SlowReader {
			mu.Lock()
			defer mu.Unlock()
			return slices.Clone(slow)
		}
	})
	h.start()

	// 追いつかない path がなければ、既存の警告だけ。
	h.tick(h.clk.Now())
	if w := h.core.Snapshot().OBS.Warnings; !slices.Equal(w, []string{"既存の警告"}) {
		t.Fatalf("warnings = %v", w)
	}

	set(supervisor.SlowReader{Path: "live/main", Count: 4})
	h.tick(h.clk.Now())
	s := h.waitSnap("メインの警告", func(s Snapshot) bool { return len(s.OBS.Warnings) == 2 })
	if s.OBS.Warnings[0] != "既存の警告" || s.OBS.Warnings[1] != "メイン "+slowWarnTail {
		t.Errorf("warnings = %v", s.OBS.Warnings)
	}
	// 健康には入れない
	if s.Cameras[0].State != s.Cameras[1].State {
		t.Errorf("健康が変わった: %+v", s.Cameras)
	}

	// 1段の path のカメラ、2台とも。出る順はカメラの順
	set(supervisor.SlowReader{Path: "sub", Count: 3}, supervisor.SlowReader{Path: "live/main", Count: 5})
	h.tick(h.clk.Now())
	s = h.waitSnap("2台の警告", func(s Snapshot) bool { return len(s.OBS.Warnings) == 3 })
	if s.OBS.Warnings[1] != "メイン "+slowWarnTail || s.OBS.Warnings[2] != "サブ "+slowWarnTail {
		t.Errorf("warnings = %v", s.OBS.Warnings)
	}

	// 解ける
	set()
	h.tick(h.clk.Now())
	h.waitSnap("警告が消える", func(s Snapshot) bool { return len(s.OBS.Warnings) == 1 })

	out := h.log.String()
	for _, want := range []string{
		"msg=\"obs reader too slow\" camera=main count=4",
		"msg=\"obs reader too slow\" camera=sub count=3",
		"msg=\"obs reader caught up\" camera=main",
		"msg=\"obs reader caught up\" camera=sub",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ログに %q がない:\n%s", want, out)
		}
	}
	// 続けて追いつかないあいだは、1行ずつ（繰り返さない）
	if n := strings.Count(out, "obs reader too slow camera=main"); n > 1 {
		t.Errorf("main の too slow が %d 回", n)
	}
}
