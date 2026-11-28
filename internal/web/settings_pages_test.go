package web

import (
	"encoding/json"
	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"testing"
)

func TestSettingsPageRequests(t *testing.T) {
	for _, tc := range []struct {
		name, page, hash, values string
		code                     int
	}{
		{"preview", "settings", "current", `{"log":{"keep_days":3}}`, 200},
		{"same page conflict", "settings", "stale", `{"log":{"keep_days":3}}`, 409},
		{"foreign unchanged leaf", "settings", "current", `{"switch":{"auto_enabled":true}}`, 400},
		{"missing page", "", "current", `{"log":{"keep_days":3}}`, 400},
		{"unknown page", "invalid", "current", `{"log":{"keep_days":3}}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := settingsFixture(t)
			cfg, _ := config.Parse(b.store.Raw)
			hash := tc.hash
			if hash == "current" {
				hash = settings.PageHash(cfg, tc.page)
			}
			body, _ := json.Marshal(map[string]any{"page": tc.page, "hash": hash, "values": json.RawMessage(tc.values)})
			w := settingsCall(b, "POST", "/api/settings/preview", string(body), "127.0.0.1:4", true)
			if w.Code != tc.code {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPageSavesIndependent(t *testing.T) {
	b := settingsFixture(t)
	cfg, _ := config.Parse(b.store.Raw)
	cameraHash := settings.PageHash(cfg, "cameras")
	settingsHash := settings.PageHash(cfg, "settings")
	call := func(method, path, page, hash, values string) int {
		body, _ := json.Marshal(map[string]any{"page": page, "hash": hash, "values": json.RawMessage(values)})
		return settingsCall(b, method, path, string(body), "127.0.0.1:4", true).Code
	}
	if code := call("PUT", "/api/settings", "settings", settingsHash, `{"log":{"keep_days":3}}`); code != 202 {
		t.Fatalf("settings save: %d", code)
	}
	if code := call("POST", "/api/settings/preview", "cameras", cameraHash, `{"switch":{"fail_kbps":999}}`); code != 200 {
		t.Fatalf("independent preview: %d", code)
	}
	if code := call("PUT", "/api/settings", "cameras", cameraHash, `{"switch":{"fail_kbps":999}}`); code != 202 {
		t.Fatalf("independent save: %d", code)
	}
	saved, _ := config.Parse(b.store.Raw)
	if saved.Log.KeepDays != 3 || saved.Switch.FailKbps != 999 {
		t.Fatal("page save overwrote other values")
	}
	if code := call("PUT", "/api/settings", "cameras", cameraHash, `{"switch":{"fail_kbps":998}}`); code != 409 {
		t.Fatalf("same page: %d", code)
	}
}
