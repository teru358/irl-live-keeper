package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"github.com/teru358/irl-live-keeper/internal/web"
)

func TestSettingsTransactionAndHandoff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configName)
	cfg, err := config.Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{opts: Options{BaseDir: dir}}
	b := &pageBackend{app: a, senderConfig: cfg}
	err = b.SettingsTransaction(func(s *web.SettingsStore) {
		out, e := settings.Rewrite(s.Raw, []settings.Change{{Path: "log.keep_days", Value: 3}})
		if e != nil {
			t.Fatal(e)
		}
		if e = s.Write(out); e != nil {
			t.Fatal(e)
		}
		next, e := config.Parse(out)
		if e != nil {
			t.Fatal(e)
		}
		s.ApplySaved(cfg, next, settings.ClassifyChanges(cfg, next, true), true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.pendingSettings == nil || !a.pendingSettings.UpdateOBSInput || a.pendingSettings.Next.Log.KeepDays != 3 || a.apply.State == ApplyApplying {
		t.Fatal("missing handoff")
	}
	if err = b.SettingsTransaction(func(s *web.SettingsStore) {
		if s.Applying {
			t.Fatal("save must not start applying")
		}
		raw, _ := os.ReadFile(path)
		if settings.Hash(raw) != settings.Hash(s.Raw) {
			t.Fatal("stale bytes")
		}
	}); err != nil {
		t.Fatal(err)
	}
}
