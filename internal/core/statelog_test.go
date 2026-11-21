package core

import (
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

func logCount(h *harness, sub string) int { return strings.Count(h.log.String(), sub) }

// 起動直後の最初の Snapshot では状態の変化を書かない。
func TestStateLogFirstSnapshotIsSilent(t *testing.T) {
	h := newHarness(t)
	out := h.log.String()
	for _, k := range []string{"stream state", "standby changed", "scene changed by user"} {
		if strings.Contains(out, k) {
			t.Errorf("最初の Snapshot で %q を書いた:\n%s", k, out)
		}
	}
}

// 配信の開始と停止は、変わったときに1行ずつ。変わらない入力では書かない。
func TestStateLogStreamState(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.samplesN(5, goodKbps)
	h.connect(true, camera)
	h.waitSnap("camera", sceneIs(camera))
	h.samplesN(3, goodKbps) // 変化なし
	if n := logCount(h, `msg="stream state" streaming=true`); n != 1 {
		t.Errorf("開始の行 = %d, want 1:\n%s", n, h.log.String())
	}
	h.obsEvent(switcher.StreamStateChanged{At: h.clk.Now(), Streaming: false})
	h.waitSnap("停止", func(s Snapshot) bool { return !s.OBS.Streaming })
	h.samplesN(2, goodKbps)
	if n := logCount(h, `msg="stream state" streaming=false`); n != 1 {
		t.Errorf("停止の行 = %d, want 1:\n%s", n, h.log.String())
	}
	if n := logCount(h, `msg="stream state"`); n != 2 {
		t.Errorf("stream state の行 = %d, want 2", n)
	}
}

// 人の切替は1行。本体の自動切替では書かない。
func TestStateLogSceneChangedByUser(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.samplesN(5, goodKbps)
	h.connect(true, camera)
	h.waitSnap("camera", sceneIs(camera))
	h.obsEvent(switcher.SceneChanged{At: h.clk.Now(), Scene: talk, SceneUUID: talk})
	h.waitSnap("talk", sceneIs(talk))
	h.samplesN(2, goodKbps)
	if n := logCount(h, `msg="scene changed by user" from=Camera to=Talk`); n != 1 {
		t.Errorf("人の切替の行 = %d, want 1:\n%s", n, h.log.String())
	}
	if n := logCount(h, `msg="scene changed by user"`); n != 1 {
		t.Errorf("人の切替の行の総数 = %d, want 1", n)
	}
}

// 自動の待機に入ると、持ち主と戻り先の行が1回。人の切替の行は出ない。
func TestStateLogStandbyChanged(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.autoStandby(camera)
	h.samplesN(1, 0)
	if n := logCount(h, `msg="standby changed" owner=auto return_scene=Camera`); n != 1 {
		t.Errorf("待機の行 = %d, want 1:\n%s", n, h.log.String())
	}
	if strings.Contains(h.log.String(), "scene changed by user") {
		t.Errorf("本体の切替を人の切替として書いた:\n%s", h.log.String())
	}
	// 回復して戻り先へ戻ると、持ち主が none になり戻り先は空。
	h.samplesN(5, goodKbps)
	h.waitSeen("AutoRecover の BatchDone", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("戻り先", both(sceneIs(camera), standbyIs("none", "")))
	if n := logCount(h, `msg="standby changed" owner=none return_scene=""`); n != 1 {
		t.Errorf("待機の解除の行 = %d, want 1:\n%s", n, h.log.String())
	}
}
