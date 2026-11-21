package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// testConfig は検証を通る設定を返す（Generate を使わずに既定値と固定のパスワードで作る）。
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := config.Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestObsConfig(t *testing.T) {
	cfg := testConfig(t)
	cfg.OBS.Host, cfg.OBS.Port, cfg.OBS.Password = "10.0.0.2", 4456, "obspw"
	cfg.OBS.MediaSource = "Cam"
	cfg.OBS.StandbyScene = "Wait"
	cfg.OBS.MonitoredScenes = []string{"A", "B"}

	got := obsConfig(cfg)
	if got.Host != "10.0.0.2" || got.Port != 4456 || got.Password != "obspw" || got.MediaSource != "Cam" {
		t.Errorf("接続の設定が違う: %+v", got)
	}
	if want := []string{"Wait", "A", "B"}; !reflect.DeepEqual(got.Scenes, want) {
		t.Errorf("Scenes = %v, want %v", got.Scenes, want)
	}

	cfg.OnStreamStart.Enabled, cfg.OnStreamStart.Scene = true, "Intro"
	if want := []string{"Wait", "A", "B", "Intro"}; !reflect.DeepEqual(obsConfig(cfg).Scenes, want) {
		t.Errorf("開始時のシーンが入っていない: %v", obsConfig(cfg).Scenes)
	}

	// 開始時のシーンが監視対象と同じでも、名前は1回だけ
	cfg.OnStreamStart.Scene = "B"
	if want := []string{"Wait", "A", "B"}; !reflect.DeepEqual(obsConfig(cfg).Scenes, want) {
		t.Errorf("重複を除いていない: %v", obsConfig(cfg).Scenes)
	}

	// 無効のときは scene に値があっても入れない
	cfg.OnStreamStart.Enabled, cfg.OnStreamStart.Scene = false, "Intro"
	if want := []string{"Wait", "A", "B"}; !reflect.DeepEqual(obsConfig(cfg).Scenes, want) {
		t.Errorf("無効なのに開始時のシーンが入っている: %v", obsConfig(cfg).Scenes)
	}
}

func TestCoreDeps(t *testing.T) {
	cfg := testConfig(t)
	cfg.OBS.StandbyScene = "Wait"
	cfg.OBS.MonitoredScenes = []string{"A", "B"}
	cfg.OBS.MediaSource = "Cam"
	cfg.Switch.AutoEnabled = false
	urls := core.URLs{PublishSRT: "s", PublishRTMP: "r", OBSRead: "o"}

	d := coreDeps(cfg, urls)
	if d.StandbyScene != "Wait" || !reflect.DeepEqual(d.MonitoredScenes, []string{"A", "B"}) ||
		d.AutoEnabled || d.MediaSource != "Cam" || d.URLs != urls {
		t.Errorf("詰め替えが違う: %+v", d)
	}
	if d.StartScene != nil {
		t.Errorf("開始時の切替が無効なのに StartScene がある: %q", *d.StartScene)
	}

	cfg.OnStreamStart.Enabled, cfg.OnStreamStart.Scene = true, "Intro"
	cfg.Switch.AutoEnabled = true
	d = coreDeps(cfg, urls)
	if d.StartScene == nil || *d.StartScene != "Intro" || !d.AutoEnabled {
		t.Errorf("StartScene/AutoEnabled が違う: %+v", d)
	}

	// 元の config の配列を共有しない
	d.MonitoredScenes[0] = "X"
	if cfg.OBS.MonitoredScenes[0] != "A" {
		t.Error("MonitoredScenes を共有している")
	}
}

func TestJudgeThresholds(t *testing.T) {
	cfg := testConfig(t)
	cfg.Switch.FailKbps, cfg.Switch.RecoverKbps = 700, 1600
	cfg.Switch.LowSec, cfg.Switch.RecoverSec, cfg.Switch.DownSec = 6, 12, 3
	want := ingest.Thresholds{
		FailKbps: 700, RecoverKbps: 1600,
		LowSec: 6 * time.Second, RecoverSec: 12 * time.Second, DownSec: 3 * time.Second,
	}
	if got := judgeThresholds(cfg); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResolvePath(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "opt", "ilk")
	abs := filepath.Join(string(filepath.Separator), "var", "log")
	tests := []struct{ in, want string }{
		{"logs", filepath.Join(base, "logs")},
		{abs, abs},
		{filepath.Join("a", "b"), filepath.Join(base, "a", "b")},
	}
	for _, tt := range tests {
		if got := resolvePath(base, tt.in); got != tt.want {
			t.Errorf("resolvePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMediaMTXBinary(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "opt", "ilk")
	if got, want := mediaMTXBinary(base, "", "linux"), filepath.Join(base, "mediamtx"); got != want {
		t.Errorf("空(linux) = %q, want %q", got, want)
	}
	if got, want := mediaMTXBinary(base, "", "windows"), filepath.Join(base, "mediamtx.exe"); got != want {
		t.Errorf("空(windows) = %q, want %q", got, want)
	}
	if got, want := mediaMTXBinary(base, "bin/mtx", "linux"), filepath.Join(base, "bin", "mtx"); got != want {
		t.Errorf("相対 = %q, want %q", got, want)
	}
	abs := filepath.Join(string(filepath.Separator), "x", "mtx")
	if got := mediaMTXBinary(base, abs, "linux"); got != abs {
		t.Errorf("絶対 = %q, want %q", got, abs)
	}
}

func TestBuildURLs(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cameras = []config.Camera{{ID: "live", Name: "live", OnBad: "standby"}}
	cfg.Stream.PublishUser, cfg.Stream.PublishPass = "irl", "PUBPASS12"
	cfg.Stream.ReadUser, cfg.Stream.ReadPass = "obs", "READPASS1"
	u := buildURLs(cfg, "192.168.1.10")
	if want := "srt://192.168.1.10:8890?streamid=publish:live:irl:PUBPASS12"; u.PublishSRT != want {
		t.Errorf("SRT = %q, want %q", u.PublishSRT, want)
	}
	if want := "rtmp://192.168.1.10:1935/live?user=irl&pass=PUBPASS12"; u.PublishRTMP != want {
		t.Errorf("RTMP = %q, want %q", u.PublishRTMP, want)
	}
	// OBS 用は host によらず 127.0.0.1
	if want := "rtsp://obs:READPASS1@127.0.0.1:8554/live"; u.OBSRead != want {
		t.Errorf("OBS = %q, want %q", u.OBSRead, want)
	}
}

func TestBuildURLsOverrides(t *testing.T) {
	tests := []struct {
		name                           string
		mod                            func(*config.Config)
		publishHost                    string
		wantSRT, wantRTMP, wantOBSRead string
	}{
		{"既定", func(c *config.Config) {}, "192.168.1.10",
			"srt://192.168.1.10:8890?streamid=publish:live:irl:PUBPASS12", "rtmp://192.168.1.10:1935/live?user=irl&pass=PUBPASS12", "rtsp://obs:READPASS1@127.0.0.1:8554/live"},
		{"obs_read_host 指定", func(c *config.Config) { c.Stream.ObsReadHost = "192.168.1.20" }, "192.168.1.10",
			"srt://192.168.1.10:8890?streamid=publish:live:irl:PUBPASS12", "rtmp://192.168.1.10:1935/live?user=irl&pass=PUBPASS12", "rtsp://obs:READPASS1@192.168.1.20:8554/live"},
		{"ポート上書き", func(c *config.Config) { c.Stream.PublishRTMPPort, c.Stream.PublishSRTPort = 21935, 28890 }, "example.com",
			"srt://example.com:28890?streamid=publish:live:irl:PUBPASS12", "rtmp://example.com:21935/live?user=irl&pass=PUBPASS12", "rtsp://obs:READPASS1@127.0.0.1:8554/live"},
		{"ポート 0 は受信のポート", func(c *config.Config) { c.MediaMTX.RTMPPort, c.MediaMTX.SRTPort = 11935, 18890 }, "example.com",
			"srt://example.com:18890?streamid=publish:live:irl:PUBPASS12", "rtmp://example.com:11935/live?user=irl&pass=PUBPASS12", "rtsp://obs:READPASS1@127.0.0.1:8554/live"},
		{"IPv6", func(c *config.Config) {
			c.Stream.ObsReadHost = "fd00::20"
			c.Stream.PublishRTMPPort = 21935
		}, "fd00::10",
			"srt://[fd00::10]:8890?streamid=publish:live:irl:PUBPASS12", "rtmp://[fd00::10]:21935/live?user=irl&pass=PUBPASS12", "rtsp://obs:READPASS1@[fd00::20]:8554/live"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.Cameras = []config.Camera{{ID: "live", Name: "live", OnBad: "standby"}}
			cfg.Stream.PublishUser, cfg.Stream.PublishPass = "irl", "PUBPASS12"
			cfg.Stream.ReadUser, cfg.Stream.ReadPass = "obs", "READPASS1"
			tt.mod(cfg)
			u := buildURLs(cfg, tt.publishHost)
			if u.PublishSRT != tt.wantSRT {
				t.Errorf("SRT = %q, want %q", u.PublishSRT, tt.wantSRT)
			}
			if u.PublishRTMP != tt.wantRTMP {
				t.Errorf("RTMP = %q, want %q", u.PublishRTMP, tt.wantRTMP)
			}
			if u.OBSRead != tt.wantOBSRead {
				t.Errorf("OBS = %q, want %q", u.OBSRead, tt.wantOBSRead)
			}
		})
	}
}

func TestBuildCameraURLs(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cameras = []config.Camera{
		{ID: "main", Name: "メインカメラ", OnBad: "standby"},
		{ID: "sub", Name: "サブカメラ", OnBad: "hide"},
	}
	cfg.Stream.PublishUser, cfg.Stream.PublishPass = "irl", "PUBPASS12"
	cfg.Stream.ReadUser, cfg.Stream.ReadPass = "obs", "READPASS1"

	// obs_read_host が空なら、どのカメラの OBS 用も 127.0.0.1
	got := buildCameraURLs(cfg, "192.168.1.10")
	want := []camURLs{
		{
			Name: "メインカメラ", ID: "main",
			PublishSRT:  "srt://192.168.1.10:8890?streamid=publish:main:irl:PUBPASS12",
			PublishRTMP: "rtmp://192.168.1.10:1935/main?user=irl&pass=PUBPASS12",
			OBSRead:     "rtsp://obs:READPASS1@127.0.0.1:8554/main",
		},
		{
			Name: "サブカメラ", ID: "sub",
			PublishSRT:  "srt://192.168.1.10:8890?streamid=publish:sub:irl:PUBPASS12",
			PublishRTMP: "rtmp://192.168.1.10:1935/sub?user=irl&pass=PUBPASS12",
			OBSRead:     "rtsp://obs:READPASS1@127.0.0.1:8554/sub",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("カメラごとの URL\n got: %+v\nwant: %+v", got, want)
	}

	// obs_read_host を書けば、すべてのカメラの OBS 用がその host になる
	cfg.Stream.ObsReadHost = "192.168.1.20"
	for i, c := range buildCameraURLs(cfg, "192.168.1.10") {
		if !strings.HasPrefix(c.OBSRead, "rtsp://obs:READPASS1@192.168.1.20:8554/") {
			t.Errorf("カメラ %d の OBS 用の host: %q", i, c.OBSRead)
		}
	}
}

// 止めたプロトコルの送信用は空文字。OBS 用の RTSP は止めていても作る。
func TestBuildCameraURLsStopped(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cameras = []config.Camera{{ID: "main", Name: "メイン", OnBad: "standby"}, {ID: "sub", Name: "サブ", OnBad: "standby"}}
	cfg.MediaMTX.RTMPEnabled = false
	for i, c := range buildCameraURLs(cfg, "192.168.1.10") {
		if c.PublishRTMP != "" || c.PublishSRT == "" || c.OBSRead == "" {
			t.Errorf("RTMP を止めたときのカメラ %d: %+v", i, c)
		}
	}
	cfg.MediaMTX.RTMPEnabled, cfg.MediaMTX.SRTEnabled = true, false
	for i, c := range buildCameraURLs(cfg, "192.168.1.10") {
		if c.PublishSRT != "" || c.PublishRTMP == "" || c.OBSRead == "" {
			t.Errorf("SRT を止めたときのカメラ %d: %+v", i, c)
		}
	}
}

// status の urls は先頭のカメラ。止めた方は空文字。
func TestBuildURLsStatusUsesFirstCamera(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cameras = []config.Camera{{ID: "main", Name: "メイン", OnBad: "standby"}, {ID: "sub", Name: "サブ", OnBad: "standby"}}
	cfg.Stream.PublishUser, cfg.Stream.PublishPass = "irl", "PUBPASS12"
	cfg.Stream.ReadUser, cfg.Stream.ReadPass = "obs", "READPASS1"
	cfg.MediaMTX.SRTEnabled = false
	got := buildURLs(cfg, "192.168.1.10")
	want := core.URLs{
		PublishSRT:  "",
		PublishRTMP: "rtmp://192.168.1.10:1935/main?user=irl&pass=PUBPASS12",
		OBSRead:     "rtsp://obs:READPASS1@127.0.0.1:8554/main",
	}
	if got != want {
		t.Errorf("status の URL = %+v, want %+v", got, want)
	}
}
