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
	"sort"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"github.com/teru358/irl-live-keeper/internal/web"
)

// updateOBSInputs handles a main-connection notification. Aux calls run with
// no App.mu held, so a slow OBS response cannot block settings or status reads.
func (a *App) updateOBSInputs(client any, connected switcher.OBSConnected) {
	a.mu.Lock()
	targets := make(map[string]string, len(a.obsInputURLs))
	for id, url := range a.obsInputURLs {
		targets[id] = url
	}
	oldURLs := make(map[string]string, len(a.obsInputOldURLs))
	for id, url := range a.obsInputOldURLs {
		oldURLs[id] = url
	}
	pinned := make(map[string][]string, len(a.obsInputUUIDs))
	for id, uuids := range a.obsInputUUIDs {
		pinned[id] = append([]string(nil), uuids...)
	}
	a.mu.Unlock()
	if len(targets) == 0 || connected.Map == nil {
		return
	}
	provider, ok := client.(interface{ SetMediaInputURL(string, string) error })
	if !ok {
		return
	}
	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	missing, failed := false, false
	for _, id := range ids {
		uuids := pinned[id]
		if len(uuids) > 0 {
			present := uuids[:0]
			for _, uuid := range uuids {
				if _, ok := connected.Map.Inputs[uuid]; ok {
					present = append(present, uuid)
				}
			}
			uuids = present
		}
		if len(uuids) == 0 {
			for uuid, in := range connected.Map.Inputs {
				if in.CameraID == id {
					uuids = append(uuids, uuid)
				}
			}
		}
		sort.Strings(uuids)
		if len(uuids) == 0 {
			if oldURL := oldURLs[id]; oldURL != "" {
				if lookup, ok := client.(interface {
					SetMediaInputURLByOldURL(string, string) (bool, bool, error)
				}); ok {
					found, media, err := lookup.SetMediaInputURLByOldURL(oldURL, targets[id])
					if err != nil {
						a.windowLogMessage("obs media input update failed", "camera", id)
						failed = true
						continue
					}
					if found && media {
						a.windowLogMessage("obs media input updated", "camera", id, "count", 1)
						a.dropOBSInputMarker(id)
						continue
					}
				}
			}
			missing = true
			a.dropOBSInputMarker(id)
			continue
		}
		inputFailed := false
		for _, uuid := range uuids {
			if err := provider.SetMediaInputURL(uuid, targets[id]); err != nil {
				a.windowLogMessage("obs media input update failed", "camera", id)
				failed = true
				inputFailed = true
				continue
			}
		}
		if !inputFailed {
			a.windowLogMessage("obs media input updated", "camera", id, "count", len(uuids))
			a.dropOBSInputMarker(id)
		}
	}
	a.mu.Lock()
	switch {
	case failed:
		a.obsInputWarning.Store(i18n.New("app.obs_url_pending"))
	case missing:
		a.obsInputWarning.Store(i18n.New("app.obs_url_missing"))
	default:
		a.obsInputWarning.Store(i18n.Msg{})
	}
	a.mu.Unlock()
}

func (a *App) dropOBSInputMarker(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.obsInputURLs, id)
	delete(a.obsInputOldURLs, id)
	delete(a.obsInputUUIDs, id)
	if len(a.obsInputURLs) == 0 {
		a.updateOBSInput = false
	}
}

// captureOBSInputUUIDs preserves the save-time camera-to-input association when
// a camera ID changes and the next generation's map can no longer match the old URL.
func (a *App) captureOBSInputUUIDs(g *generation, saved *savedSettings) {
	if saved.Old == nil || saved.Next == nil {
		return
	}
	oldMapProvider, ok := g.obs.(interface{ ConnectedMap() (*scenemap.Map, bool) })
	if !ok {
		return
	}
	m, connected := oldMapProvider.ConnectedMap()
	if !connected || m == nil {
		return
	}
	if len(saved.Old.Cameras) != len(saved.Next.Cameras) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.obsInputUUIDs = make(map[string][]string)
	for i, oldCam := range saved.Old.Cameras {
		newID := saved.Next.Cameras[i].ID
		for uuid, in := range m.Inputs {
			if in.CameraID == oldCam.ID {
				a.obsInputUUIDs[newID] = append(a.obsInputUUIDs[newID], uuid)
			}
		}
	}
}

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
	bakPath := path + ".bak"
	bak, _ := os.ReadFile(bakPath)
	fn(&web.SettingsStore{Raw: raw, Current: current, Applying: a.apply.State == ApplyApplying, BackupRaw: bak,
		Owned: ownedSettingsPorts(current), Backup: func() error {
			if sha256.Sum256(raw) != a.hash {
				return nil
			}
			return settings.WriteFile(bakPath, raw)
		},
		Write: func(out []byte) error { return settings.WriteFile(path, out) }, MarkHash: func(raw []byte) { a.hash = sha256.Sum256(raw) },
		ApplySaved: b.ApplySaved, StartApply: func() { a.settingsReady <- struct{}{} }})
	return nil
}

func (b *pageBackend) MarkRevert() { b.app.apply.Kind = ApplyKind("settings_revert") }

// ApplySaved は保存に成功したあと App.mu 内で呼ぶ。版と印を保持し、反映中にする。
func (b *pageBackend) ApplySaved(old, next *config.Config, summary settings.ChangeSummary, updateOBSInput bool) {
	b.app.pendingSettings = &savedSettings{old, next, summary, updateOBSInput}
	kind := ApplyKind("live")
	if summary.Apply == settings.ApplyRestart {
		kind = ApplyRestart
	}
	b.app.apply = Apply{State: ApplyApplying, Kind: kind}
	// The marker belongs to the process and the latest saved URL. A later unchecked
	// save cancels it; a checked save replaces it with the current camera URLs.
	b.app.updateOBSInput = updateOBSInput
	if !updateOBSInput {
		b.app.obsInputURLs = nil
		b.app.obsInputOldURLs = nil
		b.app.obsInputUUIDs = nil
		b.app.obsInputWarning.Store(i18n.Msg{})
	} else {
		host, _, _ := choosePublishHost("", systemInterfaces)
		urls := buildCameraURLs(next, host)
		b.app.obsInputURLs = make(map[string]string, len(urls))
		b.app.obsInputOldURLs = make(map[string]string)
		for _, u := range urls {
			b.app.obsInputURLs[u.ID] = u.OBSRead
		}
		if old != nil && len(old.Cameras) == len(next.Cameras) {
			oldURLs := buildCameraURLs(old, "")
			for i, u := range urls {
				b.app.obsInputOldURLs[u.ID] = oldURLs[i].OBSRead
			}
		}
		b.app.obsInputWarning.Store(i18n.New("app.obs_url_pending"))
	}
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
		if current, ok := g.obs.(interface{ ConnectedMap() (*scenemap.Map, bool) }); ok {
			if m, connected := current.ConnectedMap(); connected {
				a.updateOBSInputs(g.obs, switcher.OBSConnected{Map: m})
			}
		}
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
	kind := a.apply.Kind
	a.apply = Apply{State: ApplyApplied, Kind: kind}
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

// msgBox は1つの i18n.Msg を、ほかの lock と独立に読み書きする箱。
type msgBox struct {
	mu sync.Mutex
	m  i18n.Msg
}

func (b *msgBox) Load() i18n.Msg {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.m
}

func (b *msgBox) Store(m i18n.Msg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.m = m
}
