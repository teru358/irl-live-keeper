package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

func TestApplySettings(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle", true: "streaming"}[streaming], func(t *testing.T) {
			h := newHarness(t)
			sub := h.core.SubscribeEvents(10)
			h.start()
			h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Streaming: streaming, Scene: camera, SceneUUID: camera, Map: testMap()})
			h.settle()
			before := h.core.Settings()
			scenes := before.Scenes
			scenes.Standby = "new standby"
			cams := before.Cameras
			cams[0].Name = "new name"
			cams[0].Thresholds.FailKbps++
			ctx, cancel := context.WithTimeout(context.Background(), wait)
			defer cancel()
			if err := h.core.ApplySettings(ctx, SettingsChange{Scenes: &scenes, Cameras: cams, URLs: []CameraURLs{{ID: cams[0].ID, OBSRead: "secret-url"}}}); err != nil {
				t.Fatal(err)
			}
			if got := h.core.Settings(); got.Scenes.Standby != scenes.Standby || got.Cameras[0].Name != "new name" {
				t.Fatalf("settings: %+v", got)
			}
			if h.core.Snapshot().URLs[0].OBSRead != "secret-url" {
				t.Fatal("URL missing")
			}
			if streaming {
				select {
				case e := <-sub.C:
					b, _ := json.Marshal(e)
					if e.Kind() != "settings_applied" || strings.Contains(string(b), "secret-url") {
						t.Fatalf("event: %s", b)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			} else {
				select {
				case e := <-sub.C:
					t.Fatalf("unexpected %v", e)
				default:
				}
			}
			// Returned settings are independent copies.
			got := h.core.Settings()
			got.Cameras[0].Name = "mutated"
			if h.core.Settings().Cameras[0].Name != "new name" {
				t.Fatal("aliased")
			}
		})
	}
}

func TestApplySettingsContextWithoutLoop(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := h.core.ApplySettings(ctx, SettingsChange{}); err != context.DeadlineExceeded {
		t.Fatal(err)
	}
}

func TestBuildSettingsURLsReselectsHost(t *testing.T) {
	cfg := &config.Config{Cameras: []config.Camera{{ID: "main"}, {ID: "sub"}}}
	calls := 0
	choose := func() string { calls++; return "192.168.1.2" }
	a := BuildSettingsURLs(cfg, choose)
	BuildSettingsURLs(cfg, choose)
	if calls != 2 || len(a) != 2 || a[0].OBSRead == a[1].OBSRead {
		t.Fatalf("calls=%d urls=%v", calls, a)
	}
	cfg.Stream.PublishHost = "manual"
	BuildSettingsURLs(cfg, choose)
	if calls != 2 {
		t.Fatal("manual host reselected")
	}
}

// 中核ループが処理中でも呼び手は ctx で戻り、その後の要求も処理できる。
func TestApplySettingsBusyLoop(t *testing.T) {
	h := newHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	original := h.core.observe
	h.core.observe = func(v any) {
		if _, ok := v.(switcher.Tick); ok {
			close(entered)
			<-release
		}
		if original != nil {
			original(v)
		}
	}
	h.start()
	h.ticks <- h.clk.Now()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	err := h.core.ApplySettings(ctx, SettingsChange{})
	cancel()
	close(release)
	if err != context.DeadlineExceeded {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := h.core.ApplySettings(ctx, SettingsChange{}); err != nil {
		t.Fatal(err)
	}
}

func TestApplySettingsHungExecutor(t *testing.T) {
	h := newHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	h.obs.onStart = func() error { close(entered); <-release; return nil }
	defer close(release)
	h.start()
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Scene: camera, SceneUUID: camera, Map: testMap()})
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := h.core.Command(ctx, switcher.CmdStreamStart); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := h.core.ApplySettings(ctx, SettingsChange{}); err != nil {
		t.Fatal(err)
	}
}

func TestApplySettingsPublishesOnceAndDeliversScenes(t *testing.T) {
	h := newHarness(t)
	notifications := make(chan struct{}, 10)
	h.core.snapshotSubs = append(h.core.snapshotSubs, notifications)
	h.start()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	scenes := h.core.Settings().Scenes
	scenes.Standby = "changed"
	if err := h.core.ApplySettings(ctx, SettingsChange{Scenes: &scenes}); err != nil {
		t.Fatal(err)
	}
	if n := len(notifications); n != 1 {
		t.Fatalf("snapshot publications=%d", n)
	}
	select {
	case v := <-h.seen:
		e, ok := v.(switcher.ScenesConfigured)
		if !ok || e.Standby != "changed" {
			t.Fatalf("Machine event=%#v", v)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
