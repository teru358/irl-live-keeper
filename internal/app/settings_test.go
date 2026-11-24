package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/core"
	"os"
	"path/filepath"
	"strings"
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
	if a.pendingSettings == nil || !a.pendingSettings.UpdateOBSInput || a.pendingSettings.Next.Log.KeepDays != 3 || a.apply.State != ApplyApplying {
		t.Fatal("missing handoff")
	}
	if err = b.SettingsTransaction(func(s *web.SettingsStore) {
		if !s.Applying {
			t.Fatal("save must start applying")
		}
		raw, _ := os.ReadFile(path)
		if settings.Hash(raw) != settings.Hash(s.Raw) {
			t.Fatal("stale bytes")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

type settingsCore struct {
	coreRunner
	change core.SettingsChange
	err    error
}

func (c *settingsCore) ApplySettings(_ context.Context, v core.SettingsChange) error {
	c.change = v
	return c.err
}

type settingsOBS struct {
	obsRunner
	names []string
}

func (o *settingsOBS) UpdateScenes(v []string) { o.names = v }

type settingsLog struct {
	logHandle
	secrets []string
	days    int
}

func (l *settingsLog) AddSecrets(v ...string) { l.secrets = append(l.secrets, v...) }
func (l *settingsLog) SetKeepDays(v int)      { l.days = v }
func TestApplyLiveDeliveryAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			cfg := testConfig(t)
			cfg.GUI.CloseAction = "exit"
			cfg.Log.KeepDays = 3
			cfg.OBS.StandbyScene = "new-scene"
			c := &settingsCore{}
			if fail {
				c.err = errors.New("secret must not appear")
			}
			o, l := &settingsOBS{}, &settingsLog{}
			a := &App{auth: web.NewAuth(nil), apply: Apply{State: ApplyApplying}, opts: Options{BaseDir: t.TempDir()}}
			g := &generation{ctx: context.Background(), core: c, obs: o, logger: l}
			a.applyLive(g, &savedSettings{Next: cfg})
			if fail {
				if a.Apply().State != ApplyFailed || strings.Contains(a.Apply().Reason, "secret") {
					t.Fatal(a.Apply())
				}
				return
			}
			if a.Apply().State != ApplyApplied || a.windowClose != "exit" || l.days != 3 || len(l.secrets) != 1 || o.names[0] != "new-scene" {
				t.Fatal("届け先の不一致")
			}
			if c.change.Scenes.Standby != "new-scene" || len(c.change.Cameras) != len(cfg.Cameras) || len(c.change.URLs) != len(cfg.Cameras) {
				t.Fatal("core の反映が不足")
			}
		})
	}
}

func TestSavedSettingsRoutesAndBusy(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprint(restart), func(t *testing.T) {
			old := testConfig(t)
			next := *old
			next.Admin.Password = "changed-password"
			next.Log.KeepDays = 3
			if restart {
				next.Admin.Port++
			}
			a := &App{running: true, restarts: make(chan struct{}, 1)}
			b := &pageBackend{app: a}
			b.ApplySaved(old, &next, settings.ClassifyChanges(old, &next, true), true)
			want := ApplyKind("live")
			if restart {
				want = ApplyRestart
			}
			if a.apply.State != ApplyApplying || a.apply.Kind != want || !a.updateOBSInput {
				t.Fatal(a.apply)
			}
			if err := a.Restart(); err != ErrApplying {
				t.Fatal("反映中の再起動", err)
			}
			if len(a.restarts) != 0 {
				t.Fatal("202 より前に実行を開始した")
			}
		})
	}
}
