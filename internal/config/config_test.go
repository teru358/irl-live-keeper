package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// valid は検証を通る基準の設定を返す。
func valid() *Config {
	c := defaults()
	c.Admin.Password = "adminpass"
	c.Stream.ReadPass = "readpass1"
	c.Cameras = []Camera{{ID: "cam1", PublishPass: "publishpass", Name: "メインカメラ", OnBad: "standby"}}
	return c
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Config)
		field  string // 空なら通ること
	}{
		{"基準は通る", func(c *Config) {}, ""},
		{"language auto", func(c *Config) { c.Language = "auto" }, ""},
		{"language ja", func(c *Config) { c.Language = "ja" }, ""},
		{"language en", func(c *Config) { c.Language = "en" }, ""},
		{"language 不正", func(c *Config) { c.Language = "fr" }, "language"},

		{"recover>fail 通る", func(c *Config) { c.Switch.FailKbps = 800; c.Switch.RecoverKbps = 801 }, ""},
		{"recover==fail 落ちる", func(c *Config) { c.Switch.RecoverKbps = c.Switch.FailKbps }, "switch.recover_kbps"},
		{"recover<fail 落ちる", func(c *Config) { c.Switch.RecoverKbps = 100 }, "switch.recover_kbps"},

		{"秒が1 通る", func(c *Config) { c.Switch.LowSec, c.Switch.RecoverSec, c.Switch.DownSec = 1, 1, 1 }, ""},
		{"low_sec 0 落ちる", func(c *Config) { c.Switch.LowSec = 0 }, "switch.low_sec"},
		{"recover_sec 0 落ちる", func(c *Config) { c.Switch.RecoverSec = 0 }, "switch.recover_sec"},
		{"down_sec 0 落ちる", func(c *Config) { c.Switch.DownSec = 0 }, "switch.down_sec"},
		{"down_sec==low_sec 通る", func(c *Config) { c.Switch.DownSec = c.Switch.LowSec }, ""},
		{"down_sec>low_sec 落ちる", func(c *Config) { c.Switch.DownSec = c.Switch.LowSec + 1 }, "switch.down_sec"},

		{"ポート 1 と 65535 通る", func(c *Config) { c.MediaMTX.RTMPPort = 1; c.MediaMTX.SRTPort = 65535 }, ""},
		{"rtmp_port 0 落ちる", func(c *Config) { c.MediaMTX.RTMPPort = 0 }, "mediamtx.rtmp_port"},
		{"srt_port 65536 落ちる", func(c *Config) { c.MediaMTX.SRTPort = 65536 }, "mediamtx.srt_port"},
		{"rtsp_port 負 落ちる", func(c *Config) { c.MediaMTX.RTSPPort = -1 }, "mediamtx.rtsp_port"},
		{"api_port 0 落ちる", func(c *Config) { c.MediaMTX.APIPort = 0 }, "mediamtx.api_port"},
		{"studio_mode_hide standby 通る", func(c *Config) { c.OBS.StudioModeHide = "standby" }, ""},
		{"studio_mode_hide ignore 通る", func(c *Config) { c.OBS.StudioModeHide = "ignore" }, ""},
		{"studio_mode_hide hide 通る", func(c *Config) { c.OBS.StudioModeHide = "hide" }, ""},
		{"studio_mode_hide 空 落ちる", func(c *Config) { c.OBS.StudioModeHide = "" }, "obs.studio_mode_hide"},
		{"studio_mode_hide Hide 落ちる", func(c *Config) { c.OBS.StudioModeHide = "Hide" }, "obs.studio_mode_hide"},
		{"studio_mode_hide off 落ちる", func(c *Config) { c.OBS.StudioModeHide = "off" }, "obs.studio_mode_hide"},
		{"obs.port 0 落ちる", func(c *Config) { c.OBS.Port = 0 }, "obs.port"},
		{"mediamtx 同士の重複 落ちる", func(c *Config) { c.MediaMTX.SRTPort = c.MediaMTX.RTMPPort }, "mediamtx.srt_port"},
		{"admin.port 1 通る", func(c *Config) { c.Admin.Port = 1 }, ""},
		{"admin.port 65535 通る", func(c *Config) { c.Admin.Port = 65535 }, ""},
		{"admin.port 0 落ちる", func(c *Config) { c.Admin.Port = 0 }, "admin.port"},
		{"admin.port 65536 落ちる", func(c *Config) { c.Admin.Port = 65536 }, "admin.port"},
		{"admin.port 負 落ちる", func(c *Config) { c.Admin.Port = -1 }, "admin.port"},
		{"admin.port が mediamtx と重複 落ちる", func(c *Config) { c.Admin.Port = c.MediaMTX.APIPort }, "admin.port"},
		{"admin.port が rtmp_port と重複 落ちる", func(c *Config) { c.Admin.Port = c.MediaMTX.RTMPPort }, "admin.port"},
		{"admin.port 重複なし 通る", func(c *Config) { c.Admin.Port = 18787 }, ""},
		{"admin.lan true 通る", func(c *Config) { c.Admin.LAN = true }, ""},

		{"close_action minimize 通る", func(c *Config) { c.GUI.CloseAction = "minimize" }, ""},
		{"close_action confirm 通る", func(c *Config) { c.GUI.CloseAction = "confirm" }, ""},
		{"close_action quit 通る", func(c *Config) { c.GUI.CloseAction = "quit" }, ""},
		{"close_action 空 落ちる", func(c *Config) { c.GUI.CloseAction = "" }, "gui.close_action"},
		{"close_action Quit 落ちる", func(c *Config) { c.GUI.CloseAction = "Quit" }, "gui.close_action"},
		{"close_action close 落ちる", func(c *Config) { c.GUI.CloseAction = "close" }, "gui.close_action"},
		{"close_action 空白つき 落ちる", func(c *Config) { c.GUI.CloseAction = " quit" }, "gui.close_action"},

		{"publish_host 空 通る", func(c *Config) { c.Stream.PublishHost = "" }, ""},
		{"publish_host IPv4 通る", func(c *Config) { c.Stream.PublishHost = "192.168.1.10" }, ""},
		{"publish_host IPv6 通る", func(c *Config) { c.Stream.PublishHost = "::1" }, ""},
		{"publish_host 名前 通る", func(c *Config) { c.Stream.PublishHost = "example.com" }, ""},
		{"publish_host .local 通る", func(c *Config) { c.Stream.PublishHost = "my-host.local" }, ""},
		{"publish_host 253 文字 通る", func(c *Config) { c.Stream.PublishHost = strings.Repeat("a", 253) }, ""},
		{"publish_host スキーム 落ちる", func(c *Config) { c.Stream.PublishHost = "srt://x" }, "stream.publish_host"},
		{"publish_host ポート 落ちる", func(c *Config) { c.Stream.PublishHost = "host:8890" }, "stream.publish_host"},
		{"publish_host 角括弧 落ちる", func(c *Config) { c.Stream.PublishHost = "[::1]" }, "stream.publish_host"},
		{"publish_host スラッシュ 落ちる", func(c *Config) { c.Stream.PublishHost = "a/b" }, "stream.publish_host"},
		{"publish_host 空白 落ちる", func(c *Config) { c.Stream.PublishHost = "a b" }, "stream.publish_host"},
		{"publish_host 254 文字 落ちる", func(c *Config) { c.Stream.PublishHost = strings.Repeat("a", 254) }, "stream.publish_host"},

		{"obs_read_host 空 通る", func(c *Config) { c.Stream.ObsReadHost = "" }, ""},
		{"obs_read_host IPv4 通る", func(c *Config) { c.Stream.ObsReadHost = "192.168.1.20" }, ""},
		{"obs_read_host IPv6 通る", func(c *Config) { c.Stream.ObsReadHost = "::1" }, ""},
		{"obs_read_host 名前 通る", func(c *Config) { c.Stream.ObsReadHost = "relay.local" }, ""},
		{"obs_read_host スキーム 落ちる", func(c *Config) { c.Stream.ObsReadHost = "rtsp://x" }, "stream.obs_read_host"},
		{"obs_read_host ポート 落ちる", func(c *Config) { c.Stream.ObsReadHost = "host:8554" }, "stream.obs_read_host"},
		{"obs_read_host 角括弧 落ちる", func(c *Config) { c.Stream.ObsReadHost = "[::1]" }, "stream.obs_read_host"},
		{"obs_read_host 254 文字 落ちる", func(c *Config) { c.Stream.ObsReadHost = strings.Repeat("a", 254) }, "stream.obs_read_host"},
		{"publish ポート 0 通る", func(c *Config) { c.Stream.PublishRTMPPort, c.Stream.PublishSRTPort = 0, 0 }, ""},
		{"publish ポート 1 と 65535 通る", func(c *Config) { c.Stream.PublishRTMPPort, c.Stream.PublishSRTPort = 1, 65535 }, ""},
		{"publish ポート 受信と同じでも通る", func(c *Config) {
			c.Stream.PublishRTMPPort, c.Stream.PublishSRTPort = c.MediaMTX.RTMPPort, c.MediaMTX.SRTPort
		}, ""},
		{"publish_rtmp_port 負 落ちる", func(c *Config) { c.Stream.PublishRTMPPort = -1 }, "stream.publish_rtmp_port"},
		{"publish_rtmp_port 65536 落ちる", func(c *Config) { c.Stream.PublishRTMPPort = 65536 }, "stream.publish_rtmp_port"},
		{"publish_srt_port 負 落ちる", func(c *Config) { c.Stream.PublishSRTPort = -1 }, "stream.publish_srt_port"},
		{"publish_srt_port 65536 落ちる", func(c *Config) { c.Stream.PublishSRTPort = 65536 }, "stream.publish_srt_port"},

		{"publish_pass 空 落ちる", func(c *Config) { c.Cameras[0].PublishPass = "" }, "cameras[cam1].publish_pass"},
		{"publish_pass # 落ちる", func(c *Config) { c.Cameras[0].PublishPass = "abcdefg#h" }, "cameras[cam1].publish_pass"},
		{"read_user & 落ちる", func(c *Config) { c.Stream.ReadUser = "a&b" }, "stream.read_user"},
		{"read_user 空 落ちる", func(c *Config) { c.Stream.ReadUser = "" }, "stream.read_user"},
		{"read_pass 空白 落ちる", func(c *Config) { c.Stream.ReadPass = "abcd efgh" }, "stream.read_pass"},
		{"read_pass 空 落ちる", func(c *Config) { c.Stream.ReadPass = "" }, "stream.read_pass"},

		{"publish_pass 8文字 通る", func(c *Config) { c.Cameras[0].PublishPass = "12345678" }, ""},
		{"publish_pass 7文字 落ちる", func(c *Config) { c.Cameras[0].PublishPass = "1234567" }, "cameras[cam1].publish_pass"},
		{"read_pass 7文字 落ちる", func(c *Config) { c.Stream.ReadPass = "1234567" }, "stream.read_pass"},

		{"read_user any 落ちる", func(c *Config) { c.Stream.ReadUser = "any" }, "stream.read_user"},

		{"standby_scene 空 落ちる", func(c *Config) { c.OBS.StandbyScene = "" }, "obs.standby_scene"},
		{"standby_scene が監視対象と別 通る", func(c *Config) { c.OBS.StandbyScene = "待機" }, ""},
		{"on_stream_start 無効なら scene 空でも通る", func(c *Config) { c.OnStreamStart = OnStreamStart{Enabled: false, Scene: ""} }, ""},
		{"on_stream_start 有効で scene 空 落ちる", func(c *Config) { c.OnStreamStart = OnStreamStart{Enabled: true, Scene: ""} }, "on_stream_start.scene"},
		{"on_stream_start 有効で scene が待機と同じ 落ちる", func(c *Config) { c.OnStreamStart = OnStreamStart{Enabled: true, Scene: c.OBS.StandbyScene} }, "on_stream_start.scene"},
		{"on_stream_start 有効で scene が待機と別 通る", func(c *Config) { c.OnStreamStart = OnStreamStart{Enabled: true, Scene: "Camera"} }, ""},

		{"admin.password 空 通る", func(c *Config) { c.Admin.Password = "" }, ""},
		{"admin.password 短い", func(c *Config) { c.Admin.Password = "short" }, "admin.password"},
		{"obs.password 空 通る", func(c *Config) { c.OBS.Password = "" }, ""},

		{"log.dir 空 落ちる", func(c *Config) { c.Log.Dir = "" }, "log.dir"},
		{"log.dir 指定 通る", func(c *Config) { c.Log.Dir = "logs/irl" }, ""},
		{"log.keep_days 14 通る", func(c *Config) { c.Log.KeepDays = 14 }, ""},
		{"log.keep_days 0 通る", func(c *Config) { c.Log.KeepDays = 0 }, ""},
		{"log.keep_days -1 落ちる", func(c *Config) { c.Log.KeepDays = -1 }, "log.keep_days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			tt.mutate(c)
			err := c.Validate()
			if tt.field == "" {
				if err != nil {
					t.Fatalf("通るはずが落ちた: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("落ちるはずが通った（%s）", tt.field)
			}
			if !strings.Contains(err.Error(), tt.field+":") {
				t.Fatalf("エラーに項目名 %s がない: %v", tt.field, err)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("*ValidationError でない: %T", err)
			}
			if _, ok := ve.ByField()[tt.field]; !ok {
				t.Fatalf("ByField に %s がない: %v", tt.field, ve.ByField())
			}
		})
	}
}

func TestValidateは違反をすべて集める(t *testing.T) {
	c := valid()
	c.Switch.RecoverKbps = 1
	c.Admin.Password = "short"
	c.OBS.StandbyScene = ""
	err := c.Validate()
	if err == nil {
		t.Fatal("落ちるはず")
	}
	for _, f := range []string{"switch.recover_kbps", "admin.password", "obs.standby_scene"} {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("%s が含まれない: %v", f, err)
		}
	}
}

func TestValidationErrorの形(t *testing.T) {
	c := valid()
	c.Admin.Port = 0
	c.GUI.CloseAction = "x"
	c.Switch.RecoverKbps = 1
	err := c.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("*ValidationError でない: %T", err)
	}
	var fields []string
	for _, f := range ve.Fields {
		fields = append(fields, f.Field)
	}
	// 見つけた順（Validate の確かめる順）
	if want := []string{"switch.recover_kbps", "admin.port", "gui.close_action"}; !reflect.DeepEqual(fields, want) {
		t.Fatalf("Fields の順 = %v, want %v", fields, want)
	}
	if len(ve.ByField()) != 3 {
		t.Fatalf("ByField = %v", ve.ByField())
	}
	lines := strings.Split(strings.TrimSpace(ve.Error()), "\n")
	for _, f := range fields {
		found := false
		for _, l := range lines {
			if strings.Contains(l, f+": ") {
				found = true
			}
		}
		if !found {
			t.Errorf("Error() に %s の行がない: %s", f, ve.Error())
		}
	}
	if c := valid(); c.Validate() != nil {
		t.Error("問題がないのに nil でない")
	}
}

func TestByFieldは同じ項目の理由を句点でつなぐ(t *testing.T) {
	ve := &ValidationError{Fields: []FieldError{{Field: "a", Reason: "一つ目"}, {Field: "b", Reason: "別"}, {Field: "a", Reason: "二つ目"}}}
	got := ve.ByField()
	if got["a"] != "一つ目。二つ目" || got["b"] != "別" || len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestLoadの検証のエラーはValidationError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("admin:\n  port: 0\n  password: adminpass\nstream:\n  read_pass: readpass1\ncameras:\n  - id: cam1\n    publish_pass: publishpass\n    name: メイン\n    on_bad: standby\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("errors.As できない: %v", err)
	}
	if _, ok := ve.ByField()["admin.port"]; !ok {
		t.Fatalf("%v", ve.ByField())
	}
}

func TestGenerateとLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("生成した設定が検証を通らない: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("読み込み直した値が一致しない: %+v vs %+v", loaded, cfg)
	}
	if d := defaults(); loaded.MediaMTX != d.MediaMTX || loaded.Switch != d.Switch || loaded.Admin.Port != d.Admin.Port || loaded.Admin.LAN != d.Admin.LAN || loaded.GUI != d.GUI {
		t.Fatalf("既定値と異なる: %+v", loaded)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Errorf("ファイルの権限が 0600 でない: %o", perm)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "# 管理 API と管理ページのポート") {
		t.Error("説明コメントが残っていない")
	}
	for _, want := range []string{
		"# 待機シーン（1つ）。回線が悪い間に出す OBS のシーン名。既定値は Standby",
		"# 配信開始時の切替（安全機能）",
		"# true にすると、配信が始まったときに scene へ切り替える",
		"# 自動切替。あえて乱れた映像を出したいときに false にする",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("テンプレートのコメントがない: %s", want)
		}
	}
	if strings.Contains(string(raw), "scenes:") && strings.Contains(string(raw), "intro:") {
		t.Error("旧 obs.scenes が残っている")
	}
	if !strings.Contains(string(raw), "publish_host: \"\"") || !strings.Contains(string(raw), "# 送信用 URL の host。空なら起動時に LAN の IPv4 を自動で調べる（自動）") {
		t.Error("publish_host の行とコメントがない")
	}
	for _, want := range []string{
		"obs_read_host: \"\"",
		"# OBS 用 URL の host。空なら 127.0.0.1（本ツールを配信 PC に置く場合）",
		"publish_rtmp_port: 0",
		"# 送信用 URL に出す RTMP のポート。0 なら mediamtx.rtmp_port と同じ",
		"publish_srt_port: 0",
		"# 送信用 URL に出す SRT のポート。0 なら mediamtx.srt_port と同じ",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("生成した config に項目やコメントがない: %s", want)
		}
	}
	if strings.Index(string(raw), "obs_read_host:") > strings.Index(string(raw), "publish_host:") {
		t.Error("obs_read_host が publish_host の前にない")
	}
	if cfg.Stream.ObsReadHost != "" || cfg.Stream.PublishRTMPPort != 0 || cfg.Stream.PublishSRTPort != 0 {
		t.Errorf("新しい項目の既定値が空と 0 でない: %+v", cfg.Stream)
	}
	if cfg.Stream.PublishHost != "" {
		t.Errorf("publish_host の既定値が空でない: %q", cfg.Stream.PublishHost)
	}
	if strings.Contains(string(raw), `"*"`) {
		t.Error("* が置き換わっていない")
	}

	re := regexp.MustCompile(`^[A-Za-z0-9]{24}$`)
	if cfg.Admin.Password != "" {
		t.Fatal("initial password must be empty")
	}
	secrets := []string{cfg.Cameras[0].PublishPass, cfg.Stream.ReadPass}
	for _, s := range secrets {
		if !re.MatchString(s) {
			t.Errorf("生成した値が文字種・長さに合わない: %q", s)
		}
	}
	if secrets[0] == secrets[1] {
		t.Error("生成した値が重複している")
	}
}

func TestGenerateは上書きしない(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(path); err == nil {
		t.Fatal("既存ファイルがあるのに成功した")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "keep" {
		t.Fatalf("既存ファイルが書き換わった: %q", got)
	}
}

func TestLoad(t *testing.T) {
	write := func(t *testing.T, s string) string {
		p := filepath.Join(t.TempDir(), "c.yaml")
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	const minimal = "admin:\n  password: adminpass\nstream:\n  read_pass: readpass1\ncameras:\n  - id: cam1\n    publish_pass: publishpass\n    name: メイン\n    on_bad: standby\n"

	t.Run("省略した項目は既定値", func(t *testing.T) {
		cfg, err := Load(write(t, minimal))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Admin.Port != 8787 || cfg.Admin.LAN || cfg.GUI.CloseAction != "confirm" || cfg.Switch.FailKbps != 800 || cfg.OBS.StandbyScene != "Standby" || cfg.OnStreamStart.Enabled || !cfg.Switch.AutoEnabled || cfg.Log.KeepDays != 14 {
			t.Fatalf("既定値が入っていない: %+v", cfg)
		}
	})
	t.Run("auto_enabled 省略は true", func(t *testing.T) {
		cfg, err := Load(write(t, minimal+"switch:\n  fail_kbps: 500\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.Switch.AutoEnabled {
			t.Fatal("省略したのに false になった")
		}
	})
	t.Run("auto_enabled 明示の false は false のまま", func(t *testing.T) {
		cfg, err := Load(write(t, minimal+"switch:\n  auto_enabled: false\n"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Switch.AutoEnabled {
			t.Fatal("false を指定したのに true になった")
		}
	})
	t.Run("シーンの設定を読む", func(t *testing.T) {
		cfg, err := Load(write(t, minimal+"obs:\n  standby_scene: 待機\non_stream_start:\n  enabled: true\n  scene: Cam1\n"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.OBS.StandbyScene != "待機" || !cfg.OnStreamStart.Enabled || cfg.OnStreamStart.Scene != "Cam1" {
			t.Fatalf("%+v %+v", cfg.OBS, cfg.OnStreamStart)
		}
	})
	t.Run("旧 obs.scenes は未知のキーとして拒否", func(t *testing.T) {
		if _, err := Load(write(t, minimal+"obs:\n  scenes:\n    intro: A\n")); err == nil {
			t.Fatal("通ってしまった")
		}
	})
	t.Run("指定した値は既定値を上書きする", func(t *testing.T) {
		cfg, err := Load(write(t, minimal+"switch:\n  fail_kbps: 500\n"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Switch.FailKbps != 500 || cfg.Switch.RecoverKbps != 1500 {
			t.Fatalf("%+v", cfg.Switch)
		}
	})
	t.Run("未知のキーは拒否", func(t *testing.T) {
		if _, err := Load(write(t, minimal+"bogus: 1\n")); err == nil {
			t.Fatal("通ってしまった")
		}
		if _, err := Load(write(t, minimal+"switch:\n  fail_kb: 1\n")); err == nil {
			t.Fatal("入れ子の未知キーが通ってしまった")
		}
	})
	t.Run("検証に落ちる", func(t *testing.T) {
		_, err := Load(write(t, "admin:\n  password: x\n"))
		if err == nil || !strings.Contains(err.Error(), "stream.read_pass") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("空ファイルは検証で落ちる", func(t *testing.T) {
		if _, err := Load(write(t, "")); err == nil {
			t.Fatal("通ってしまった")
		}
	})
	t.Run("ファイルなし", func(t *testing.T) {
		if _, err := Load(filepath.Join(t.TempDir(), "none.yaml")); err == nil {
			t.Fatal("通ってしまった")
		}
	})
}

func TestWarnings(t *testing.T) {
	c := valid()
	if w := c.Warnings(); len(w) != 0 {
		t.Fatalf("lan=false で警告がある: %v", w)
	}
	c.Admin.LAN = true
	w := c.Warnings()
	if len(w) != 1 {
		t.Fatalf("lan=true の警告 = %v", w)
	}
	for _, want := range []string{"LAN に公開", "平文", "盗聴", "パスワード", "設定の変更", "転送しない"} {
		if !strings.Contains(w[0], want) {
			t.Errorf("警告文に %q がない: %s", want, w[0])
		}
	}
}

func TestLoadの古いadmin_listen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("admin:\n  listen: \"127.0.0.1:8787\"\n  password: adminpass\nstream:\n  read_pass: readpass1\ncameras:\n  - id: cam1\n    publish_pass: publishpass\n    name: メイン\n    on_bad: standby\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	if err == nil {
		t.Fatal("通ってしまった")
	}
	for _, want := range []string{"admin.listen は使えなくなりました", "admin.port", "admin.lan", "書き直して"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("エラーの文に %q がない: %v", want, err)
		}
	}
	// ほかの知らない項目は今のまま（書き直しの案内を足さない）
	p2 := filepath.Join(t.TempDir(), "c2.yaml")
	os.WriteFile(p2, []byte("bogus: 1\n"), 0o600)
	_, err = Load(p2)
	if err == nil || strings.Contains(err.Error(), "admin.port") {
		t.Fatalf("err=%v", err)
	}
}

func TestStudioModeHideの既定とエラー文(t *testing.T) {
	const minimal = "admin:\n  password: adminpass\nstream:\n  read_pass: readpass1\ncameras:\n  - id: cam1\n    publish_pass: publishpass\n    name: メイン\n    on_bad: standby\n"
	cfg, err := parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OBS.StudioModeHide != "standby" {
		t.Fatalf("既定 = %q, want standby", cfg.OBS.StudioModeHide)
	}
	for _, v := range []string{"standby", "ignore", "hide"} {
		cfg, err := parse([]byte(minimal + "obs:\n  studio_mode_hide: " + v + "\n"))
		if err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		if cfg.OBS.StudioModeHide != v {
			t.Fatalf("%s を読んだ結果 = %q", v, cfg.OBS.StudioModeHide)
		}
	}
	_, err = parse([]byte(minimal + "obs:\n  studio_mode_hide: nope\n"))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("ValidationError のはずが %v", err)
	}
	msg := ve.ByField()["obs.studio_mode_hide"]
	for _, want := range []string{"standby", "ignore", "hide", `"nope"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("エラー文 %q に %q がない", msg, want)
		}
	}
}

func Testひな形にStudioModeHideのコメントがある(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := Generate(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("ひな形が検証を通らない: %v", err)
	}
	if loaded.OBS.StudioModeHide != "standby" {
		t.Fatalf("ひな形の値 = %q", loaded.OBS.StudioModeHide)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "studio_mode_hide:") {
			for _, want := range []string{"standby", "ignore", "hide", "シーンを複製"} {
				if !strings.Contains(line, want) {
					t.Errorf("ひな形のコメントに %q がない: %s", want, line)
				}
			}
			return
		}
	}
	t.Fatal("ひな形に studio_mode_hide の行がない")
}

func TestStreamApp(t *testing.T) {
	if got := defaults().Stream.App; got != "live" {
		t.Errorf("既定の app = %q, want live", got)
	}
	tests := []struct {
		app   string
		field string
	}{
		{"live", ""}, {"", ""}, {"app-1_x", ""},
		{"a/b", "stream.app"}, {"/live", "stream.app"}, {"a b", "stream.app"}, {" ", "stream.app"},
		{"a?b", "stream.app"}, {"a#b", "stream.app"}, {"a:b", "stream.app"}, {"ライブ", "stream.app"},
	}
	for _, tt := range tests {
		t.Run(tt.app, func(t *testing.T) {
			c := valid()
			c.Stream.App = tt.app
			err := c.Validate()
			if tt.field == "" {
				if err != nil {
					t.Fatalf("通るはずが落ちた: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("%q: %s で落ちるはずが %v", tt.app, tt.field, err)
			}
		})
	}
}

func TestCameraPath(t *testing.T) {
	c := valid()
	if got := c.CameraPath("cam1"); got != "live/cam1" {
		t.Errorf("既定: %q", got)
	}
	c.Stream.App = "x"
	if got := c.CameraPath("cam1"); got != "x/cam1" {
		t.Errorf("app=x: %q", got)
	}
	c.Stream.App = ""
	if got := c.CameraPath("cam1"); got != "cam1" {
		t.Errorf("app が空: %q", got)
	}
}

func TestLoadのstream_app(t *testing.T) {
	load := func(streamApp string) (*Config, error) {
		p := filepath.Join(t.TempDir(), "config.yaml")
		body := "admin:\n  password: adminPASS123\nstream:\n" + streamApp + "  read_pass: readPASS123\ncameras:\n  - id: cam1\n    publish_pass: publishpass\n    name: m\n    on_bad: standby\n"
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return Load(p)
	}
	if c, err := load(""); err != nil || c.Stream.App != "live" {
		t.Errorf("省略は live: %v %+v", err, c)
	}
	if c, err := load("  app: \"\"\n"); err != nil || c.Stream.App != "" || c.CameraPath("cam1") != "cam1" {
		t.Errorf("app: \"\" は1段: %v %+v", err, c)
	}
	if _, err := load("  app: a/b\n"); err == nil {
		t.Error("a/b が通った")
	}
}
