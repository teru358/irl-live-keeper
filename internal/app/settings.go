package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"github.com/teru358/irl-live-keeper/internal/web"
)

func (b *pageBackend) SettingsTransaction(fn func(*web.SettingsStore)) error {
	a := b.app
	a.mu.Lock()
	defer a.mu.Unlock()
	path := filepath.Join(a.opts.BaseDir, configName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	current := b.senderConfig
	if a.liveConfig != nil {
		current = a.liveConfig
	}
	fn(&web.SettingsStore{Raw: raw, Current: current, Applying: a.apply.State == ApplyApplying,
		Owned: ownedSettingsPorts(current), Backup: func() error { return nil },
		Write: func(out []byte) error { return settings.WriteFile(path, out) }, ApplySaved: b.ApplySaved, StartApply: func() { a.settingsReady <- struct{}{} }})
	return nil
}

// ApplySaved は保存に成功したあと App.mu 内で呼ぶ。版と印を保持し、反映中にする。
func (b *pageBackend) ApplySaved(old, next *config.Config, summary settings.ChangeSummary, updateOBSInput bool) {
	b.app.pendingSettings = &savedSettings{old, next, summary, updateOBSInput}
	kind := ApplyKind("live")
	if summary.Apply == settings.ApplyRestart {
		kind = ApplyRestart
	}
	b.app.apply = Apply{State: ApplyApplying, Kind: kind}
	b.app.updateOBSInput = b.app.updateOBSInput || updateOBSInput
}

type savedSettings struct {
	Old, Next      *config.Config
	Summary        settings.ChangeSummary
	UpdateOBSInput bool
}

func ownedSettingsPorts(c *config.Config) []settings.ListeningPort {
	if c == nil {
		return nil
	}
	var out []settings.ListeningPort
	add := func(protocol string, port int) {
		out = append(out, settings.ListeningPort{Protocol: protocol, Address: fmt.Sprintf("0.0.0.0:%d", port)}, settings.ListeningPort{Protocol: protocol, Address: fmt.Sprintf(":%d", port)})
	}
	if c.MediaMTX.RTMPEnabled {
		add("tcp", c.MediaMTX.RTMPPort)
	}
	if c.MediaMTX.SRTEnabled {
		add("udp", c.MediaMTX.SRTPort)
	}
	add("tcp", c.MediaMTX.RTSPPort)
	return append(out, settings.ListeningPort{Protocol: "tcp", Address: fmt.Sprintf("127.0.0.1:%d", c.MediaMTX.APIPort)})
}

// 保存の callback は App.mu 内。開始通知は 202 の flush と unlock のあと。
func (b *pageBackend) KeepSettingsSession(token string) { b.app.savedSession = token }
func (b *pageBackend) AdminPassword() string {
	b.app.mu.Lock()
	defer b.app.mu.Unlock()
	return b.app.adminPassword
}
func (a *App) applyLive(g *generation, saved *savedSettings) {
	cfg := saved.Next
	d := coreDeps(cfg, buildCameraURLs(cfg, ""))
	change := core.SettingsChange{Scenes: &core.SceneSettings{Standby: d.StandbyScene, SceneFallbacks: d.SceneFallbacks, AutoEnabled: d.AutoEnabled, StartScene: d.StartScene, StudioModeHide: d.StudioModeHide}}
	for _, c := range d.Cameras {
		change.Cameras = append(change.Cameras, core.CameraSettings{ID: c.ID, Name: c.Name, OnBad: c.OnBad, Thresholds: c.Thresholds})
	}
	if saved.Old != nil {
		old := coreDeps(saved.Old, buildCameraURLs(saved.Old, ""))
		oldScenes := core.SceneSettings{Standby: old.StandbyScene, SceneFallbacks: old.SceneFallbacks, AutoEnabled: old.AutoEnabled, StartScene: old.StartScene, StudioModeHide: old.StudioModeHide}
		if reflect.DeepEqual(*change.Scenes, oldScenes) {
			change.Scenes = nil
		}
		var cameras []core.CameraSettings
		for _, c := range old.Cameras {
			cameras = append(cameras, core.CameraSettings{ID: c.ID, Name: c.Name, OnBad: c.OnBad, Thresholds: c.Thresholds})
		}
		if reflect.DeepEqual(change.Cameras, cameras) {
			change.Cameras = nil
		}
	}
	change.URLs = core.BuildSettingsURLs(cfg, func() string { h, _, _ := choosePublishHost("", systemInterfaces); return h })
	ctx, cancel := context.WithTimeout(g.ctx, 2*time.Second)
	defer cancel()
	var err error
	if target, ok := g.core.(interface {
		ApplySettings(context.Context, core.SettingsChange) error
	}); ok {
		err = target.ApplySettings(ctx, change)
	} else {
		err = errors.New("core settings unavailable")
	}
	if err == nil {
		// OBS への問い合わせ（補助の接続、最大2秒）は App.mu の外で。持ったままだと窓と管理ページの状態の読み取りが止まる
		g.obs.UpdateScenes(obsConfig(cfg).Scenes)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.apply = Apply{State: ApplyFailed, Kind: ApplyKind("live"), Reason: i18n.Render(a.windowLang, i18n.New("app.settings_apply_failed"))}
		return
	}
	a.rememberSecrets(cfg.Admin.Password)
	g.logger.AddSecrets(cfg.Admin.Password)
	if cfg.Admin.Password != a.adminPassword {
		a.auth.KeepSession(a.savedSession)
	}
	a.adminPassword = cfg.Admin.Password
	a.savedSession = ""
	if l, ok := g.logger.(interface{ SetKeepDays(int) }); ok {
		l.SetKeepDays(cfg.Log.KeepDays)
	}
	a.windowClose = cfg.GUI.CloseAction
	a.windowPasswordUnset = cfg.Admin.Password == ""
	if raw, e := os.ReadFile(filepath.Join(a.opts.BaseDir, configName)); e == nil {
		a.hash = sha256.Sum256(raw)
	}
	a.liveConfig = cfg
	a.apply = Apply{State: ApplyApplied, Kind: ApplyKind("live")}
}

// configureProcess は世代の作る順5。保存した Cookie の識別子はここまで保持する。
func (a *App) configureProcess(cfg *config.Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.adminPassword != cfg.Admin.Password {
		a.auth.KeepSession(a.savedSession)
	}
	a.adminPassword = cfg.Admin.Password
	a.savedSession = ""
	a.adminPort = cfg.Admin.Port
	a.windowClose = cfg.GUI.CloseAction
}
