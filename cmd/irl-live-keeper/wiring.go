package main

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
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

// obsConfig は config から obs.Config を作る。Scenes は OBS に存在を確かめる名前（待機シーン、有効なら
// 開始時のシーン）で、重複は除く。Cameras と RTSPPort は、地図でメディアソースの URL を照合するための設定。
func obsConfig(cfg *config.Config) obs.Config {
	scenes := []string{cfg.OBS.StandbyScene}
	if cfg.OnStreamStart.Enabled {
		scenes = append(scenes, cfg.OnStreamStart.Scene)
	}
	seen := map[string]bool{}
	scenes = slices.DeleteFunc(scenes, func(s string) bool {
		dup := seen[s]
		seen[s] = true
		return dup
	})
	cams := make([]obs.CameraConfig, len(cfg.Cameras))
	for i, c := range cfg.Cameras {
		cams[i] = obs.CameraConfig{ID: c.ID, Name: c.Name}
	}
	return obs.Config{
		Host:     cfg.OBS.Host,
		Port:     cfg.OBS.Port,
		Password: cfg.OBS.Password,
		Scenes:   scenes,
		Cameras:  cams,
		RTSPPort: cfg.MediaMTX.RTSPPort,
	}
}

// coreDeps は config から core.Deps のうち、設定に由来する部分を作る。カメラごとの閾値は、共通の switch に
// カメラの上書きを合わせたもの。urls はカメラごとの URL（buildCameraURLs の列）。
// 部品（Machine、OBS など）は呼び出し側が足す。
func coreDeps(cfg *config.Config, urls []camURLs) core.Deps {
	d := core.Deps{
		StandbyScene: cfg.OBS.StandbyScene,
		AutoEnabled:  cfg.Switch.AutoEnabled,
		// Studio Mode の間の on_bad: hide のカメラの動き（standby / ignore / hide）
		StudioModeHide: cfg.OBS.StudioModeHide,
	}
	for i, c := range cfg.Cameras {
		d.Cameras = append(d.Cameras, core.Camera{
			ID: c.ID, Name: c.Name, OnBad: scenemap.Motion(c.OnBad),
			Thresholds: switchThresholds(cfg.CameraSwitch(i)),
			URLs:       core.CameraURLs{PublishSRT: urls[i].PublishSRT, PublishRTMP: urls[i].PublishRTMP, OBSRead: urls[i].OBSRead},
		})
	}
	if cfg.OnStreamStart.Enabled {
		scene := cfg.OnStreamStart.Scene
		d.StartScene = &scene
	}
	return d
}

// switchThresholds は switch の値から Judge の閾値を作る。
func switchThresholds(sw config.Switch) ingest.Thresholds {
	sec := func(n int) time.Duration { return time.Duration(n) * time.Second }
	return ingest.Thresholds{
		FailKbps:    float64(sw.FailKbps),
		RecoverKbps: float64(sw.RecoverKbps),
		LowSec:      sec(sw.LowSec),
		RecoverSec:  sec(sw.RecoverSec),
		DownSec:     sec(sw.DownSec),
	}
}

// defaultObsReadHost は stream.obs_read_host が空のときの OBS 用 URL の host（本ツールを配信 PC に置く場合）。
const defaultObsReadHost = "127.0.0.1"

// camURLs は1台のカメラの URL。止めたプロトコルの送信用は空文字。
type camURLs struct {
	Name, ID                         string
	PublishSRT, PublishRTMP, OBSRead string
}

// buildCameraURLs はカメラごとの URL を設定の順に作る。送信用は publishHost とポートの上書き、
// OBS 用は stream.obs_read_host（空なら 127.0.0.1）の RTSP（Windows の OBS で遅延は約1秒）。
// 起動時の表示は、この列をそのまま使う。
func buildCameraURLs(cfg *config.Config, publishHost string) []camURLs {
	obsHost := cfg.Stream.ObsReadHost
	if obsHost == "" {
		obsHost = defaultObsReadHost
	}
	pub := supervisor.PublishURLs(cfg, publishHost)
	read := supervisor.ReadURLs(cfg, obsHost)
	out := make([]camURLs, len(cfg.Cameras))
	for i, cam := range cfg.Cameras {
		out[i] = camURLs{
			Name:        cam.Name,
			ID:          cam.ID,
			PublishSRT:  pub[i].SRT,
			PublishRTMP: pub[i].RTMP,
			OBSRead:     read[i].RTSP,
		}
	}
	return out
}

// cameraIDs は設定の順のカメラの id の列。
func cameraIDs(cfg *config.Config) []string {
	ids := make([]string, len(cfg.Cameras))
	for i, c := range cfg.Cameras {
		ids[i] = c.ID
	}
	return ids
}
