package main

import (
	"path/filepath"
	"reflect"
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
	cfg.Stream.Name, cfg.Stream.PublishUser, cfg.Stream.PublishPass = "live", "irl", "PUBPASS12"
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
