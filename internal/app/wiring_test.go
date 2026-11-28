package app

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"github.com/teru358/irl-live-keeper/internal/switcher"
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
	cfg.OBS.StandbyScene = "Wait"
	cfg.MediaMTX.RTSPPort = 18554
	cfg.Cameras = []config.Camera{{ID: "cam1", PublishPass: "publishpass", Name: "メイン", OnBad: "standby"}, {ID: "cam2", PublishPass: "publishpass", Name: "サブ", OnBad: "hide"}}

	got := obsConfig(cfg)
	if got.Host != "10.0.0.2" || got.Port != 4456 || got.Password != "obspw" {
		t.Errorf("接続の設定が違う: %+v", got)
	}
	if want := []string{"Wait"}; !reflect.DeepEqual(got.Scenes, want) {
		t.Errorf("Scenes = %v, want %v", got.Scenes, want)
	}
	if want := []obs.CameraConfig{{ID: "cam1", Name: "メイン", Path: "live/cam1"}, {ID: "cam2", Name: "サブ", Path: "live/cam2"}}; !reflect.DeepEqual(got.Cameras, want) {
		t.Errorf("Cameras = %v, want %v（設定の順）", got.Cameras, want)
	}
	if got.RTSPPort != 18554 {
		t.Errorf("RTSPPort = %d, want 18554", got.RTSPPort)
	}

	cfg.OnStreamStart.Enabled, cfg.OnStreamStart.Scene = true, "Intro"
	if want := []string{"Wait", "Intro"}; !reflect.DeepEqual(obsConfig(cfg).Scenes, want) {
		t.Errorf("開始時のシーンが入っていない: %v", obsConfig(cfg).Scenes)
	}

	// 開始時のシーンが待機シーンと同じ名前でも、名前は1回だけ
	cfg.OnStreamStart.Scene = "Wait"
	if want := []string{"Wait"}; !reflect.DeepEqual(obsConfig(cfg).Scenes, want) {
		t.Errorf("重複を除いていない: %v", obsConfig(cfg).Scenes)
	}

	// 無効のときは scene に値があっても入れない
	cfg.OnStreamStart.Enabled, cfg.OnStreamStart.Scene = false, "Intro"
	if want := []string{"Wait"}; !reflect.DeepEqual(obsConfig(cfg).Scenes, want) {
		t.Errorf("無効なのに開始時のシーンが入っている: %v", obsConfig(cfg).Scenes)
	}
}

func TestResolvePathsForDir(t *testing.T) {
	dataDir, exeDir := filepath.Join("/opt", "keeper-data"), filepath.Join("/opt", "keeper")
	if got, want := resolvePath(dataDir, "logs"), filepath.Join(dataDir, "logs"); got != want {
		t.Fatalf("relative log path = %q, want %q", got, want)
	}
	if got, want := resolvePath(dataDir, "/var/log/keeper"), "/var/log/keeper"; got != want {
		t.Fatalf("absolute log path = %q, want %q", got, want)
	}
	if got, want := mediaMTXBinary(exeDir, "", "linux"), filepath.Join(exeDir, "mediamtx"); got != want {
		t.Fatalf("default binary = %q, want %q", got, want)
	}
	if got, want := mediaMTXBinary(dataDir, "bin/mediamtx", "linux"), filepath.Join(dataDir, "bin", "mediamtx"); got != want {
		t.Fatalf("relative binary = %q, want %q", got, want)
	}
	if got, want := mediaMTXBinary(dataDir, "/usr/bin/mediamtx", "linux"), "/usr/bin/mediamtx"; got != want {
		t.Fatalf("absolute binary = %q, want %q", got, want)
	}
}

func TestDirectoryAndBinaryPathMatrix(t *testing.T) {
	exeDir := filepath.Join("/opt", "keeper")
	for _, withDir := range []bool{false, true} {
		dataDir := exeDir
		if withDir {
			dataDir = filepath.Join("/var", "lib", "keeper")
		}
		for _, binary := range []struct {
			name, value, want string
		}{
			{name: "empty", want: filepath.Join(exeDir, "mediamtx")},
			{name: "relative", value: "bin/mediamtx", want: filepath.Join(dataDir, "bin", "mediamtx")},
			{name: "absolute", value: "/usr/bin/mediamtx", want: "/usr/bin/mediamtx"},
		} {
			for _, logDir := range []struct {
				value, want string
			}{
				{value: "logs", want: filepath.Join(dataDir, "logs")},
				{value: "/var/log/keeper", want: "/var/log/keeper"},
			} {
				if got := configuredMediaMTXBinary(dataDir, exeDir, binary.value, "linux"); got != binary.want {
					t.Errorf("withDir=%v binary=%s: got %q, want %q", withDir, binary.name, got, binary.want)
				}
				if got := resolvePath(dataDir, logDir.value); got != logDir.want {
					t.Errorf("withDir=%v log.dir=%q: got %q, want %q", withDir, logDir.value, got, logDir.want)
				}
			}
		}
	}
}

func TestCoreDeps(t *testing.T) {
	cfg := testConfig(t)
	cfg.OBS.StandbyScene = "Wait"
	cfg.OBS.StudioModeHide = "ignore"
	cfg.Switch.AutoEnabled = false
	cfg.Switch.FailKbps, cfg.Switch.RecoverKbps = 700, 1600
	cfg.Switch.LowSec, cfg.Switch.RecoverSec, cfg.Switch.DownSec = 6, 12, 3
	fail := 300
	cfg.Cameras = []config.Camera{
		{ID: "cam1", PublishPass: "publishpass", Name: "メイン", OnBad: "standby"},
		{ID: "cam2", PublishPass: "publishpass", Name: "サブ", OnBad: "hide", Switch: &config.SwitchOverride{FailKbps: &fail}},
	}
	urls := []camURLs{
		{ID: "cam1", Name: "メイン", PublishSRT: "s1", PublishRTMP: "r1", OBSRead: "o1"},
		{ID: "cam2", Name: "サブ", PublishSRT: "s2", PublishRTMP: "r2", OBSRead: "o2"},
	}

	cfg.OBS.SceneFallbacks = []config.SceneFallback{{Scene: " A ", Fallback: "B"}, {Scene: "C", Fallback: "Wait"}}

	d := coreDeps(cfg, urls)
	wantFB := []switcher.FallbackRow{{Scene: " A ", Fallback: "B"}, {Scene: "C", Fallback: "Wait"}}
	if !reflect.DeepEqual(d.SceneFallbacks, wantFB) {
		t.Errorf("SceneFallbacks = %+v, want %+v（名前のまま、順のまま渡す）", d.SceneFallbacks, wantFB)
	}
	if d.StandbyScene != "Wait" || d.AutoEnabled {
		t.Errorf("詰め替えが違う: %+v", d)
	}
	if d.StudioModeHide != "ignore" {
		t.Errorf("StudioModeHide = %q, want ignore（obs.studio_mode_hide を渡す）", d.StudioModeHide)
	}
	common := ingest.Thresholds{FailKbps: 700, RecoverKbps: 1600, LowSec: 6 * time.Second, RecoverSec: 12 * time.Second, DownSec: 3 * time.Second}
	override := common
	override.FailKbps = 300
	want := []core.Camera{
		{ID: "cam1", Name: "メイン", Path: "live/cam1", OnBad: scenemap.MotionStandby, Thresholds: common,
			URLs: core.CameraURLs{PublishSRT: "s1", PublishRTMP: "r1", OBSRead: "o1"}},
		{ID: "cam2", Name: "サブ", Path: "live/cam2", OnBad: scenemap.MotionHide, Thresholds: override,
			URLs: core.CameraURLs{PublishSRT: "s2", PublishRTMP: "r2", OBSRead: "o2"}},
	}
	if !reflect.DeepEqual(d.Cameras, want) {
		t.Errorf("Cameras\n got: %+v\nwant: %+v", d.Cameras, want)
	}
	cfg.Stream.App = "" // app が空なら1段の path
	if p := coreDeps(cfg, urls).Cameras[1].Path; p != "cam2" {
		t.Errorf("app が空のときの Path = %q, want cam2", p)
	}
	cfg.Stream.App = "live"
	if d.StartScene != nil {
		t.Errorf("開始時の切替が無効なのに StartScene がある: %q", *d.StartScene)
	}

	cfg.OnStreamStart.Enabled, cfg.OnStreamStart.Scene = true, "Intro"
	cfg.Switch.AutoEnabled = true
	d = coreDeps(cfg, urls)
	if d.StartScene == nil || *d.StartScene != "Intro" || !d.AutoEnabled {
		t.Errorf("StartScene/AutoEnabled が違う: %+v", d)
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

func TestBuildCameraURLsOne(t *testing.T) {
	cfg := testConfig(t)
	cfg.Cameras = []config.Camera{{ID: "cam1", PublishPass: "publishpass", Name: "live", OnBad: "standby"}}
	cfg.Cameras[0].PublishPass = "PUBPASS12"
	cfg.Stream.ReadUser, cfg.Stream.ReadPass = "obs", "READPASS1"
	u := buildCameraURLs(cfg, "192.168.1.10")[0]
	if want := "srt://192.168.1.10:8890?streamid=publish:live/cam1:cam1:PUBPASS12"; u.PublishSRT != want {
		t.Errorf("SRT = %q, want %q", u.PublishSRT, want)
	}
	if want := "rtmp://192.168.1.10:1935/live/cam1?user=cam1&pass=PUBPASS12"; u.PublishRTMP != want {
		t.Errorf("RTMP = %q, want %q", u.PublishRTMP, want)
	}
	// OBS 用は host によらず 127.0.0.1
	if want := "rtsp://obs:READPASS1@127.0.0.1:8554/live/cam1"; u.OBSRead != want {
		t.Errorf("OBS = %q, want %q", u.OBSRead, want)
	}
}

func TestBuildCameraURLsOverrides(t *testing.T) {
	tests := []struct {
		name                           string
		mod                            func(*config.Config)
		publishHost                    string
		wantSRT, wantRTMP, wantOBSRead string
	}{
		{"既定", func(c *config.Config) {}, "192.168.1.10",
			"srt://192.168.1.10:8890?streamid=publish:live/cam1:cam1:PUBPASS12", "rtmp://192.168.1.10:1935/live/cam1?user=cam1&pass=PUBPASS12", "rtsp://obs:READPASS1@127.0.0.1:8554/live/cam1"},
		{"obs_read_host 指定", func(c *config.Config) { c.Stream.ObsReadHost = "192.168.1.20" }, "192.168.1.10",
			"srt://192.168.1.10:8890?streamid=publish:live/cam1:cam1:PUBPASS12", "rtmp://192.168.1.10:1935/live/cam1?user=cam1&pass=PUBPASS12", "rtsp://obs:READPASS1@192.168.1.20:8554/live/cam1"},
		{"ポート上書き", func(c *config.Config) { c.Stream.PublishRTMPPort, c.Stream.PublishSRTPort = 21935, 28890 }, "example.com",
			"srt://example.com:28890?streamid=publish:live/cam1:cam1:PUBPASS12", "rtmp://example.com:21935/live/cam1?user=cam1&pass=PUBPASS12", "rtsp://obs:READPASS1@127.0.0.1:8554/live/cam1"},
		{"ポート 0 は受信のポート", func(c *config.Config) { c.MediaMTX.RTMPPort, c.MediaMTX.SRTPort = 11935, 18890 }, "example.com",
			"srt://example.com:18890?streamid=publish:live/cam1:cam1:PUBPASS12", "rtmp://example.com:11935/live/cam1?user=cam1&pass=PUBPASS12", "rtsp://obs:READPASS1@127.0.0.1:8554/live/cam1"},
		{"IPv6", func(c *config.Config) {
			c.Stream.ObsReadHost = "fd00::20"
			c.Stream.PublishRTMPPort = 21935
		}, "fd00::10",
			"srt://[fd00::10]:8890?streamid=publish:live/cam1:cam1:PUBPASS12", "rtmp://[fd00::10]:21935/live/cam1?user=cam1&pass=PUBPASS12", "rtsp://obs:READPASS1@[fd00::20]:8554/live/cam1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.Cameras = []config.Camera{{ID: "cam1", PublishPass: "publishpass", Name: "live", OnBad: "standby"}}
			cfg.Cameras[0].PublishPass = "PUBPASS12"
			cfg.Stream.ReadUser, cfg.Stream.ReadPass = "obs", "READPASS1"
			tt.mod(cfg)
			u := buildCameraURLs(cfg, tt.publishHost)[0]
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
		{ID: "cam1", PublishPass: "publishpass", Name: "メインカメラ", OnBad: "standby"},
		{ID: "cam2", PublishPass: "publishpass", Name: "サブカメラ", OnBad: "hide"},
	}
	cfg.Cameras[0].PublishPass = "PUBPASS12"
	cfg.Stream.ReadUser, cfg.Stream.ReadPass = "obs", "READPASS1"

	// obs_read_host が空なら、どのカメラの OBS 用も 127.0.0.1
	got := buildCameraURLs(cfg, "192.168.1.10")
	want := []camURLs{
		{
			Name: "メインカメラ", ID: "cam1",
			PublishSRT:  "srt://192.168.1.10:8890?streamid=publish:live/cam1:cam1:PUBPASS12",
			PublishRTMP: "rtmp://192.168.1.10:1935/live/cam1?user=cam1&pass=PUBPASS12",
			OBSRead:     "rtsp://obs:READPASS1@127.0.0.1:8554/live/cam1",
		},
		{
			Name: "サブカメラ", ID: "cam2",
			PublishSRT:  "srt://192.168.1.10:8890?streamid=publish:live/cam2:cam2:publishpass",
			PublishRTMP: "rtmp://192.168.1.10:1935/live/cam2?user=cam2&pass=publishpass",
			OBSRead:     "rtsp://obs:READPASS1@127.0.0.1:8554/live/cam2",
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
	cfg.Cameras = []config.Camera{{ID: "cam1", PublishPass: "publishpass", Name: "メイン", OnBad: "standby"}, {ID: "cam2", PublishPass: "publishpass", Name: "サブ", OnBad: "standby"}}
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

// All downstream lists inherit the normalized order from a manually edited file.
func TestLoadedCameraOrderThroughSnapshotAndValues(t *testing.T) {
	cfg, err := config.Parse([]byte("stream:\n  read_pass: readpass1\ncameras:\n  - id: cam3\n    publish_pass: publishpass\n    name: Three\n    on_bad: hide\n  - id: cam1\n    publish_pass: publishpass\n    name: One\n    on_bad: standby\n  - id: cam2\n    publish_pass: publishpass\n    name: Two\n    on_bad: standby\n"))
	if err != nil {
		t.Fatal(err)
	}
	urls := buildCameraURLs(cfg, "")
	deps := coreDeps(cfg, urls)
	deps.Machine = switcher.New(switcher.Config{})
	snapshot := core.New(deps).Snapshot()
	groups, err := sources.ValueGroups(snapshot, sources.Values(snapshot, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	var valueIDs []string
	for _, g := range groups {
		if g.Camera != nil {
			valueIDs = append(valueIDs, g.Camera.ID)
		}
	}
	want := []string{"cam1", "cam2", "cam3"}
	var snapshotIDs, urlIDs []string
	for _, cam := range snapshot.Cameras {
		snapshotIDs = append(snapshotIDs, cam.ID)
	}
	for _, cam := range urls {
		urlIDs = append(urlIDs, cam.ID)
	}
	for _, ids := range [][]string{snapshotIDs, urlIDs, valueIDs, cameraIDs(cfg)} {
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("order=%v", ids)
		}
	}
}
