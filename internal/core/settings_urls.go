package core

import (
	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
)

// BuildSettingsURLs は反映のたびに URL を作り直す。chooseHost は app の
// choosePublishHost と同じ選択を行い、PublishHost が空のときだけ呼ぶ。
// OS の照会を中核ループの外で済ませ、返した値を SettingsChange.URLs に渡す。
func BuildSettingsURLs(cfg *config.Config, chooseHost func() string) []CameraURLs {
	host := cfg.Stream.PublishHost
	if host == "" {
		host = chooseHost()
	}
	obsHost := cfg.Stream.ObsReadHost
	if obsHost == "" {
		obsHost = "127.0.0.1"
	}
	pub, read := supervisor.PublishURLs(cfg, host), supervisor.ReadURLs(cfg, obsHost)
	urls := make([]CameraURLs, len(cfg.Cameras))
	for i, cam := range cfg.Cameras {
		urls[i] = CameraURLs{ID: cam.ID, Name: cam.Name, PublishSRT: pub[i].SRT, PublishRTMP: pub[i].RTMP, OBSRead: read[i].RTSP}
	}
	return urls
}
