package web

import (
	"encoding/json"
	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"testing"
)

func TestCameraSaveAndRevertKeepCurrentCredentials(t *testing.T) {
	b := settingsFixture(t)
	before := b.store.Current.Cameras[0].PublishPass
	payload := map[string]any{"page": "cameras", "hash": settings.PageHash(b.store.Current, "cameras"), "values": map[string]any{"cameras": []any{map[string]any{"id": "cam1", "name": "One", "on_bad": "standby"}, map[string]any{"id": "cam2", "name": "Two", "on_bad": "hide"}}}}
	raw, _ := json.Marshal(payload)
	preview := settingsCall(b, "POST", "/api/settings/preview", string(raw), "127.0.0.1:4", true)
	if preview.Code != 200 || b.writes != 0 {
		t.Fatal("preview wrote or failed")
	}
	w := settingsCall(b, "PUT", "/api/settings", string(raw), "127.0.0.1:4", true)
	if w.Code != 202 {
		t.Fatal("camera save failed")
	}
	next, err := config.Parse(b.store.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if next.Cameras[0].PublishPass != before || len(next.Cameras[1].PublishPass) != 24 {
		t.Fatal("wrong camera credentials")
	}
	backup := *next
	backup.Log.KeepDays++
	b.store.BackupRaw = configBytes(t, &backup)
	raw, _ = json.Marshal(map[string]any{"target": "publish", "camera": "cam1"})
	w = settingsCall(b, "POST", "/api/settings/regenerate", string(raw), "127.0.0.1:4", true)
	if w.Code != 202 {
		t.Fatal("regeneration failed")
	}
	regenerated, err := config.Parse(b.store.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if regenerated.Cameras[0].PublishPass == before || regenerated.Cameras[1].PublishPass != next.Cameras[1].PublishPass {
		t.Fatal("wrong regeneration target")
	}
	raw, _ = json.Marshal(map[string]any{"file_hash": settings.Hash(b.store.Raw)})
	w = settingsCall(b, "POST", "/api/settings/revert", string(raw), "127.0.0.1:4", true)
	// With no noncredential changes, revert is intentionally a no-op conflict.
	if w.Code != 202 {
		t.Fatal("revert failed")
	}
	final, err := config.Parse(b.store.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if final.Cameras[0].PublishPass != regenerated.Cameras[0].PublishPass {
		t.Fatal("revoked credential restored")
	}
}
