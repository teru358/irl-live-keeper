package core

import (
	"context"
	"slices"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// SettingsChange は検証済みのすぐ反映の要求。nil の区分は変更しない。
// カメラの追加・削除・ID変更は世代の再起動で扱う。
// URLs は app が host を再選択して作り直したカメラごとの URL。
type SettingsChange struct {
	Scenes  *SceneSettings
	Cameras []CameraSettings // nil は変更なし。name/on_bad と実効閾値をまとめて渡す。
	URLs    []CameraURLs
}

// SceneSettings は名前で指定するシーンの設定。
type SceneSettings struct {
	Standby        string
	SceneFallbacks []switcher.FallbackRow
	AutoEnabled    bool
	StartScene     *string
	StudioModeHide string
}

// CameraSettings は秘密情報を持たないカメラの設定。閾値は上書きを合成済み。
type CameraSettings struct {
	ID         string
	Name       string
	OnBad      scenemap.Motion
	Thresholds ingest.Thresholds
}

// Settings は履歴の meta と settings_applied に使う現在の設定。
type Settings struct {
	Scenes  SceneSettings
	Cameras []CameraSettings
}

type settingsReq struct {
	change SettingsChange
	reply  chan error
}

// ApplySettings は実行役の完了を待たず、差し替えと Snapshot の公開を待つ。
// ctx が終わっても、受理済みの要求は適用されることがある。
func (c *Core) ApplySettings(ctx context.Context, change SettingsChange) error {
	change = cloneChange(change)
	req := settingsReq{change: change, reply: make(chan error, 1)}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stopped:
		return ErrStopped
	case c.settingsCh <- req:
	}
	select {
	case err := <-req.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stopped:
		return ErrStopped
	}
}

func cloneScenes(s SceneSettings) SceneSettings {
	s.SceneFallbacks = slices.Clone(s.SceneFallbacks)
	if s.StartScene != nil {
		v := *s.StartScene
		s.StartScene = &v
	}
	return s
}
func cloneChange(v SettingsChange) SettingsChange {
	if v.Scenes != nil {
		s := cloneScenes(*v.Scenes)
		v.Scenes = &s
	}
	v.Cameras = slices.Clone(v.Cameras)
	v.URLs = slices.Clone(v.URLs)
	return v
}

// Settings はどの goroutine からでも読める、独立した写しを返す。
func (c *Core) Settings() Settings {
	s := *c.settings.Load()
	s.Scenes = cloneScenes(s.Scenes)
	s.Cameras = slices.Clone(s.Cameras)
	return s
}
func (c *Core) currentSettings(auto bool, start *string) *Settings {
	s := &Settings{Scenes: cloneScenes(SceneSettings{Standby: c.standbyName, SceneFallbacks: c.fallbackRows, AutoEnabled: auto, StartScene: start, StudioModeHide: c.studioHide})}
	for _, cam := range c.cams {
		s.Cameras = append(s.Cameras, CameraSettings{cam.cfg.ID, cam.cfg.Name, cam.cfg.OnBad, cam.cfg.Thresholds})
	}
	return s
}
func (c *Core) applySettings(v SettingsChange) {
	previous := c.Settings()
	scenes := previous.Scenes
	if v.Scenes != nil {
		scenes = *v.Scenes
	}
	for _, cfg := range v.Cameras {
		if cam := c.camByID(cfg.ID); cam != nil {
			cam.cfg.Name, cam.cfg.OnBad = cfg.Name, cfg.OnBad
			if cam.cfg.Thresholds != cfg.Thresholds {
				cam.cfg.Thresholds = cfg.Thresholds
				cam.judge.SetThresholds(cfg.Thresholds)
			}
		}
	}
	for _, u := range v.URLs {
		if cam := c.camByID(u.ID); cam != nil {
			cam.cfg.URLs = u
		}
	}
	c.standbyName, c.fallbackRows, c.studioHide = scenes.Standby, slices.Clone(scenes.SceneFallbacks), scenes.StudioModeHide
	if v.Scenes != nil || v.Cameras != nil {
		cfgs := make([]switcher.CameraConfig, 0, len(c.cams))
		c.anyHide = false
		for _, cam := range c.cams {
			cfgs = append(cfgs, switcher.CameraConfig{ID: cam.cfg.ID, OnBad: cam.cfg.OnBad})
			c.anyHide = c.anyHide || cam.cfg.OnBad == scenemap.MotionHide
		}
		c.handle(switcher.ScenesConfigured{At: c.now(), Standby: scenes.Standby, SceneFallbacks: slices.Clone(scenes.SceneFallbacks), Cameras: cfgs, AutoEnabled: scenes.AutoEnabled, StartScene: scenes.StartScene, StudioModeHide: scenes.StudioModeHide})
	}
	c.settings.Store(c.currentSettings(scenes.AutoEnabled, scenes.StartScene))
	if c.m.Snapshot().Streaming {
		c.publishEvent(SettingsAppliedEvent{At: c.now(), Settings: c.Settings()})
	}
}

// SettingsAppliedEvent は秘密情報も URL も持たない。
type SettingsAppliedEvent struct {
	At       time.Time
	Settings Settings
}

func (SettingsAppliedEvent) Kind() string      { return "settings_applied" }
func (e SettingsAppliedEvent) Time() time.Time { return e.At }
