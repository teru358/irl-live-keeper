package core

import (
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 応答の期限を過ぎても、接続中に適用した変更のイベントは失わない。
func TestIntegrationHungApplyEmitsEvent(t *testing.T) {
	w := newWorld(1, newClock(), "", true)
	defer w.close()
	var seed int64
	for ; ; seed++ {
		w.rng = rand.New(rand.NewSource(seed))
		if w.pickMode() == modeHang && w.rng.Intn(2) == 0 {
			break
		}
	}
	w.rng = rand.New(rand.NewSource(seed))
	w.cont[camera].find(2).enabled = false
	g := &genClient{w: w, gen: w.gen}
	done := make(chan error, 1)
	go func() { done <- g.SetSceneItemEnabled(camera, 2, true) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w.mu.Lock()
		hung := len(w.hung) > 0
		w.mu.Unlock()
		if hung {
			w.releaseHungDue(w.clk.Advance(5 * time.Second))
			break
		}
		if time.Now().After(deadline) {
			w.close()
			<-done
			t.Fatal("要求が始まらない")
		}
		runtime.Gosched()
	}
	<-done
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		t.Fatal("接続中に適用した true のイベントが欠落")
	}
	ev, ok := w.pending[0].ev.(switcher.ItemEnabledChanged)
	if !ok || !ev.Enabled || ev.SceneUUID != camera || ev.ItemID != 2 {
		t.Fatalf("event = %#v", w.pending[0].ev)
	}
}
