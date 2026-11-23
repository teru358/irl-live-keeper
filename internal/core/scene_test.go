package core

import (
	"errors"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"testing"
)

// 人の入力を偽 OBS と中核ループから通し、回復後も選択した管理外のシーンに残る。
func TestHumanSceneSelectionRemains(t *testing.T) {
	for _, mode := range []string{"通常", "auto", "manual"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t)
			h.start()
			if mode == "auto" {
				h.autoStandby(camera)
			} else {
				h.samplesN(5, goodKbps)
				h.connect(true, camera)
				h.waitSnap("接続", sceneIs(camera))
				if mode == "manual" {
					if err, _ := h.command(switcher.CmdStandbyOn); err != nil {
						t.Fatal(err)
					}
					h.waitSnap("待機", standbyIs("manual", camera))
				}
			}
			if err := h.core.SelectScene(h.ctx, talk); err != nil {
				t.Fatal(err)
			}
			h.waitSnap("人の切替", both(sceneIs(talk), standbyIs("none", "")))
			n := len(h.obs.Calls())
			h.samplesN(6, goodKbps)
			if got := len(h.obs.Calls()); got != n {
				t.Fatalf("人の選択後に操作: %v", h.obs.Calls())
			}
		})
	}
}

// 補助の要求は応答を返し、イベントを中核へ送る経路とは分離する。
func (f *fakeOBS) Scenes() ([]obs.Scene, error) {
	return []obs.Scene{{UUID: talk, Name: talk}, {UUID: standby, Name: standby}}, nil
}
func (f *fakeOBS) SelectScene(uuid string) error {
	if f.onSelect != nil {
		return f.onSelect(uuid)
	}
	f.emit(switcher.SceneChanged{At: f.clk.Now(), SceneUUID: uuid})
	return nil
}

// 補助の接続の切り替えが応答を待っている間も、中核ループは Sample と Command を受け続ける（止まらない）。
func TestSelectSceneDoesNotBlockCoreLoop(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.samplesN(5, goodKbps)
	h.connect(true, camera)
	h.waitSnap("接続", sceneIs(camera))
	release := make(chan struct{})
	entered := make(chan struct{})
	h.obs.onSelect = func(uuid string) error {
		close(entered)
		<-release
		h.obs.emit(switcher.SceneChanged{At: h.clk.Now(), SceneUUID: uuid})
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- h.core.SelectScene(h.ctx, talk) }()
	<-entered
	// 切り替えの応答を待っている間に、Sample と Command が受け付けられること
	h.samplesN(3, goodKbps)
	if err, _ := h.command(switcher.CmdStandbyOn); err != nil {
		t.Fatalf("待っている間の Command: %v", err)
	}
	h.waitSnap("応答待ちの間の待機操作", both(sceneIs(standby), standbyIs("manual", camera)))
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	h.waitSnap("人の切替", both(sceneIs(talk), standbyIs("none", "")))
}

// 通知と応答を別々に届け、成功・期限切れでは通知まで現在値を変えない。
func TestSelectSceneArrivalOrder(t *testing.T) {
	for _, mode := range []string{"通知が先", "応答が先", "期限切れ", "失敗"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t)
			h.start()
			h.autoStandby(camera)
			entered := make(chan struct{})
			release := make(chan struct{})
			h.obs.onSelect = func(uuid string) error {
				close(entered)
				<-release
				if mode == "期限切れ" {
					return obs.ErrAuxTimeout
				}
				if mode == "失敗" {
					return errors.New("OBS が拒否")
				}
				return nil
			}
			done := make(chan error, 1)
			go func() { done <- h.core.SelectScene(h.ctx, talk) }()
			<-entered
			if got := h.core.Snapshot().OBS.SceneUUID; got != standby {
				t.Fatalf("到着前に更新: %s", got)
			}
			if mode == "通知が先" {
				h.obsEvent(switcher.SceneChanged{At: h.clk.Now(), SceneUUID: talk})
			}
			close(release)
			err := <-done
			if (mode == "期限切れ" || mode == "失敗") != (err != nil) {
				t.Fatalf("応答: %v", err)
			}
			if mode == "失敗" {
				return
			}
			if mode != "通知が先" {
				if got := h.core.Snapshot().OBS.SceneUUID; got != standby {
					t.Fatalf("応答で更新: %s", got)
				}
				h.obsEvent(switcher.SceneChanged{At: h.clk.Now(), SceneUUID: talk})
			}
			h.waitSnap("人の到着", both(sceneIs(talk), standbyIs("none", "")))
		})
	}
}

func TestSelectSceneSameSkipsOBS(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.autoStandby(camera)
	h.obs.onSelect = func(string) error { t.Error("同じシーンへ要求した"); return nil }
	if err := h.core.SelectScene(h.ctx, standby); err != nil {
		t.Fatal(err)
	}
	h.waitSnap("同じシーンの人の操作", both(sceneIs(standby), standbyIs("none", "")))
}
