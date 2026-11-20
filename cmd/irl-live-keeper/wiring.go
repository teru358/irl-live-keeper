package main

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
)

// resolvePath は相対パスを base からの相対として解決する。絶対パスはそのまま返す。
func resolvePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// mediaMTXBinary は MediaMTX の実行ファイルのパスを返す。空なら base の mediamtx（Windows は .exe）。
func mediaMTXBinary(base, configured, goos string) string {
	if configured != "" {
		return resolvePath(base, configured)
	}
	name := "mediamtx"
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(base, name)
}

// obsConfig は config から obs.Config を作る。Scenes は OBS に存在を確かめる名前（待機シーン、
// 監視対象のシーン、有効なら開始時のシーン）で、重複は除く。
func obsConfig(cfg *config.Config) obs.Config {
	scenes := []string{cfg.OBS.StandbyScene}
	scenes = append(scenes, cfg.OBS.MonitoredScenes...)
	if cfg.OnStreamStart.Enabled {
		scenes = append(scenes, cfg.OnStreamStart.Scene)
	}
	seen := map[string]bool{}
	scenes = slices.DeleteFunc(scenes, func(s string) bool {
		dup := seen[s]
		seen[s] = true
		return dup
	})
	return obs.Config{
		Host:        cfg.OBS.Host,
		Port:        cfg.OBS.Port,
		Password:    cfg.OBS.Password,
		MediaSource: cfg.OBS.MediaSource,
		Scenes:      scenes,
	}
}

// coreDeps は config から core.Deps のうち、設定に由来する部分を作る。
// 部品（Machine、Judge、OBS など）は呼び出し側が足す。
func coreDeps(cfg *config.Config, urls core.URLs) core.Deps {
	d := core.Deps{
		StandbyScene:    cfg.OBS.StandbyScene,
		MonitoredScenes: slices.Clone(cfg.OBS.MonitoredScenes),
		AutoEnabled:     cfg.Switch.AutoEnabled,
		MediaSource:     cfg.OBS.MediaSource,
		URLs:            urls,
	}
	if cfg.OnStreamStart.Enabled {
		scene := cfg.OnStreamStart.Scene
		d.StartScene = &scene
	}
	return d
}

// judgeThresholds は config から Judge の閾値を作る。
func judgeThresholds(cfg *config.Config) ingest.Thresholds {
	sec := func(n int) time.Duration { return time.Duration(n) * time.Second }
	return ingest.Thresholds{
		FailKbps:    float64(cfg.Switch.FailKbps),
		RecoverKbps: float64(cfg.Switch.RecoverKbps),
		LowSec:      sec(cfg.Switch.LowSec),
		RecoverSec:  sec(cfg.Switch.RecoverSec),
		DownSec:     sec(cfg.Switch.DownSec),
	}
}

// obsReadHost は OBS 用 URL の host。OBS は同じ PC で動く。OBS 用 URL は RTSP（Windows の OBS で遅延は約1秒）。
const obsReadHost = "127.0.0.1"

// buildURLs は status と起動時の表示に使う URL を作る。送信用は host、OBS 用は 127.0.0.1。
func buildURLs(cfg *config.Config, publishHost string) core.URLs {
	pub := supervisor.PublishURLs(cfg, publishHost)
	return core.URLs{
		PublishSRT:  pub.SRT,
		PublishRTMP: pub.RTMP,
		OBSRead:     supervisor.ReadURLs(cfg, obsReadHost).RTSP,
	}
}
