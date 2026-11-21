package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ip(v int) *int { return &v }

func cams(n int) []Camera {
	out := make([]Camera, n)
	for i := range out {
		out[i] = Camera{ID: fmt.Sprintf("cam%d", i), Name: fmt.Sprintf("カメラ%d", i), OnBad: "standby"}
	}
	return out
}

func TestValidateCameras(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Config)
		field  string // 空なら通ること
	}{
		{"0台 落ちる", func(c *Config) { c.Cameras = nil }, "cameras"},
		{"1台 通る", func(c *Config) { c.Cameras = cams(1) }, ""},
		{"8台 通る", func(c *Config) { c.Cameras = cams(8) }, ""},
		{"9台 落ちる", func(c *Config) { c.Cameras = cams(9) }, "cameras"},

		{"id 大文字 落ちる", func(c *Config) { c.Cameras[0].ID = "Main" }, "cameras[0].id"},
		{"id 先頭が - 落ちる", func(c *Config) { c.Cameras[0].ID = "-main" }, "cameras[0].id"},
		{"id 先頭が _ 落ちる", func(c *Config) { c.Cameras[0].ID = "_main" }, "cameras[0].id"},
		{"id 空 落ちる", func(c *Config) { c.Cameras[0].ID = "" }, "cameras[0].id"},
		{"id 32文字 通る", func(c *Config) { c.Cameras[0].ID = strings.Repeat("a", 32) }, ""},
		{"id 33文字 落ちる", func(c *Config) { c.Cameras[0].ID = strings.Repeat("a", 33) }, "cameras[0].id"},
		{"id 日本語 落ちる", func(c *Config) { c.Cameras[0].ID = "かめら" }, "cameras[0].id"},
		{"id 記号 通る", func(c *Config) { c.Cameras[0].ID = "a-b_c9" }, ""},
		{"id 数字始まり 通る", func(c *Config) { c.Cameras[0].ID = "1cam" }, ""},
		{"id スラッシュ 落ちる", func(c *Config) { c.Cameras[0].ID = "a/b" }, "cameras[0].id"},
		{"id all 落ちる", func(c *Config) { c.Cameras[0].ID = "all" }, "cameras[0].id"},
		{"id all_others 落ちる", func(c *Config) { c.Cameras[0].ID = "all_others" }, "cameras[0].id"},
		{"id all_x は通る", func(c *Config) { c.Cameras[0].ID = "all_x" }, ""},
		{"id 重複は後の番号で落ちる", func(c *Config) { c.Cameras = cams(3); c.Cameras[2].ID = "cam0" }, "cameras[2].id"},

		{"name 0文字 落ちる", func(c *Config) { c.Cameras[0].Name = "" }, "cameras[0].name"},
		{"name 1文字 通る", func(c *Config) { c.Cameras[0].Name = "あ" }, ""},
		{"name 日本語32文字 通る", func(c *Config) { c.Cameras[0].Name = strings.Repeat("あ", 32) }, ""},
		{"name 日本語33文字 落ちる", func(c *Config) { c.Cameras[0].Name = strings.Repeat("あ", 33) }, "cameras[0].name"},
		{"name 改行 落ちる", func(c *Config) { c.Cameras[0].Name = "a\nb" }, "cameras[0].name"},
		{"name タブ 落ちる", func(c *Config) { c.Cameras[0].Name = "a\tb" }, "cameras[0].name"},
		{"name NUL 落ちる", func(c *Config) { c.Cameras[0].Name = "a\x00b" }, "cameras[0].name"},
		{"name 重複 通る", func(c *Config) { c.Cameras = cams(2); c.Cameras[1].Name = c.Cameras[0].Name }, ""},

		{"on_bad standby 通る", func(c *Config) { c.Cameras[0].OnBad = "standby" }, ""},
		{"on_bad hide 通る", func(c *Config) { c.Cameras[0].OnBad = "hide" }, ""},
		{"on_bad 空 落ちる", func(c *Config) { c.Cameras[0].OnBad = "" }, "cameras[0].on_bad"},
		{"on_bad 別の値 落ちる", func(c *Config) { c.Cameras[0].OnBad = "off" }, "cameras[0].on_bad"},
		{"on_bad 2台目 落ちる", func(c *Config) { c.Cameras = cams(2); c.Cameras[1].OnBad = "Hide" }, "cameras[1].on_bad"},

		{"上書きなし 通る", func(c *Config) { c.Cameras[0].Switch = nil }, ""},
		{"上書き空 通る", func(c *Config) { c.Cameras[0].Switch = &SwitchOverride{} }, ""},
		{"上書き fail と recover 通る", func(c *Config) {
			c.Cameras[0].Switch = &SwitchOverride{FailKbps: ip(400), RecoverKbps: ip(800)}
		}, ""},
		{"上書きだけで recover<=fail 落ちる", func(c *Config) {
			c.Cameras[0].Switch = &SwitchOverride{FailKbps: ip(900), RecoverKbps: ip(900)}
		}, "cameras[0].switch.recover_kbps"},
		{"合わせると recover<=fail 落ちる", func(c *Config) {
			c.Cameras[0].Switch = &SwitchOverride{FailKbps: ip(2000)}
		}, "cameras[0].switch.recover_kbps"},
		{"合わせると通る（recover を上げれば）", func(c *Config) {
			c.Cameras[0].Switch = &SwitchOverride{FailKbps: ip(2000), RecoverKbps: ip(3000)}
		}, ""},
		{"low_sec 0 落ちる", func(c *Config) { c.Cameras[0].Switch = &SwitchOverride{LowSec: ip(0)} }, "cameras[0].switch.low_sec"},
		{"recover_sec 0 落ちる", func(c *Config) { c.Cameras[0].Switch = &SwitchOverride{RecoverSec: ip(0)} }, "cameras[0].switch.recover_sec"},
		{"down_sec 0 落ちる", func(c *Config) { c.Cameras[0].Switch = &SwitchOverride{DownSec: ip(0)} }, "cameras[0].switch.down_sec"},
		{"down_sec が共通の low_sec より大きい 落ちる", func(c *Config) {
			c.Cameras[0].Switch = &SwitchOverride{DownSec: ip(c.Switch.LowSec + 1)}
		}, "cameras[0].switch.down_sec"},
		{"low_sec を下げて共通の down_sec より小さい 落ちる", func(c *Config) {
			c.Cameras[0].Switch = &SwitchOverride{LowSec: ip(c.Switch.DownSec - 1)}
		}, "cameras[0].switch.down_sec"},
		{"low_sec と down_sec を合わせて下げる 通る", func(c *Config) {
			c.Cameras[0].Switch = &SwitchOverride{LowSec: ip(1), DownSec: ip(1)}
		}, ""},
		{"2台目の上書きの番号", func(c *Config) {
			c.Cameras = cams(2)
			c.Cameras[1].Switch = &SwitchOverride{LowSec: ip(0)}
		}, "cameras[1].switch.low_sec"},

		{"rtmp も srt も false 落ちる", func(c *Config) { c.MediaMTX.RTMPEnabled, c.MediaMTX.SRTEnabled = false, false }, "mediamtx.rtmp_enabled"},
		{"rtmp だけ false 通る", func(c *Config) { c.MediaMTX.RTMPEnabled = false }, ""},
		{"srt だけ false 通る", func(c *Config) { c.MediaMTX.SRTEnabled = false }, ""},
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
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("ValidationError でない: %v", err)
			}
			if _, ok := ve.ByField()[tt.field]; !ok {
				t.Fatalf("項目 %q がない: %v", tt.field, ve.ByField())
			}
		})
	}
}

func TestCameraSwitch(t *testing.T) {
	base := Switch{AutoEnabled: false, FailKbps: 800, LowSec: 5, RecoverKbps: 1500, RecoverSec: 10, DownSec: 2}
	tests := []struct {
		name string
		ov   *SwitchOverride
		want Switch
	}{
		{"上書きなし", nil, base},
		{"上書き空", &SwitchOverride{}, base},
		{"一部", &SwitchOverride{FailKbps: ip(400), RecoverKbps: ip(900)},
			Switch{AutoEnabled: false, FailKbps: 400, LowSec: 5, RecoverKbps: 900, RecoverSec: 10, DownSec: 2}},
		{"すべて", &SwitchOverride{FailKbps: ip(1), LowSec: ip(2), RecoverKbps: ip(3), RecoverSec: ip(4), DownSec: ip(1)},
			Switch{AutoEnabled: false, FailKbps: 1, LowSec: 2, RecoverKbps: 3, RecoverSec: 4, DownSec: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			c.Switch = base
			c.Cameras = cams(2)
			c.Cameras[1].Switch = tt.ov
			if got := c.CameraSwitch(1); got != tt.want {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
			if got := c.CameraSwitch(0); got != base {
				t.Fatalf("上書きのないカメラが変わった: %+v", got)
			}
			if c.Switch != base {
				t.Fatal("共通の値が書き換わった")
			}
		})
	}
}

func writeCfg(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const camMinimal = "admin:\n  password: adminpass\nstream:\n  publish_pass: publishpass\n  read_pass: readpass1\n"
const oneCam = "cameras:\n  - id: main\n    name: メイン\n    on_bad: standby\n"

func TestLoadCameras(t *testing.T) {
	t.Run("rtmp_enabled と srt_enabled の省略は true", func(t *testing.T) {
		cfg, err := Load(writeCfg(t, camMinimal+oneCam))
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.MediaMTX.RTMPEnabled || !cfg.MediaMTX.SRTEnabled {
			t.Fatalf("%+v", cfg.MediaMTX)
		}
	})
	t.Run("明示の false は false", func(t *testing.T) {
		cfg, err := Load(writeCfg(t, camMinimal+oneCam+"mediamtx:\n  srt_enabled: false\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.MediaMTX.RTMPEnabled || cfg.MediaMTX.SRTEnabled {
			t.Fatalf("%+v", cfg.MediaMTX)
		}
	})
	t.Run("カメラを読む（上書きは書いた項目だけ）", func(t *testing.T) {
		y := "cameras:\n  - id: main\n    name: メイン\n    on_bad: standby\n  - id: sub\n    name: サブ\n    on_bad: hide\n    switch:\n      fail_kbps: 400\n      recover_kbps: 800\n"
		cfg, err := Load(writeCfg(t, camMinimal+y))
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Cameras) != 2 || cfg.Cameras[0].Switch != nil || cfg.Cameras[1].OnBad != "hide" {
			t.Fatalf("%+v", cfg.Cameras)
		}
		sw := cfg.CameraSwitch(1)
		if sw.FailKbps != 400 || sw.RecoverKbps != 800 || sw.LowSec != 5 || sw.DownSec != 2 {
			t.Fatalf("%+v", sw)
		}
	})
	t.Run("cameras を省略すると不正", func(t *testing.T) {
		_, err := Load(writeCfg(t, camMinimal))
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%v", err)
		}
		if _, ok := ve.ByField()["cameras"]; !ok {
			t.Fatalf("%v", ve.ByField())
		}
	})
	t.Run("switch の下の知らない項目は不正", func(t *testing.T) {
		y := "cameras:\n  - id: main\n    name: メイン\n    on_bad: standby\n    switch:\n      fail_kb: 1\n"
		if _, err := Load(writeCfg(t, camMinimal+y)); err == nil {
			t.Fatal("通ってしまった")
		}
	})
	t.Run("switch の下の auto_enabled は不正（5つの閾値だけ）", func(t *testing.T) {
		y := "cameras:\n  - id: main\n    name: メイン\n    on_bad: standby\n    switch:\n      auto_enabled: false\n"
		if _, err := Load(writeCfg(t, camMinimal+y)); err == nil {
			t.Fatal("通ってしまった")
		}
	})
	t.Run("カメラの知らない項目は不正", func(t *testing.T) {
		y := "cameras:\n  - id: main\n    name: メイン\n    on_bad: standby\n    bogus: 1\n"
		if _, err := Load(writeCfg(t, camMinimal+y)); err == nil {
			t.Fatal("通ってしまった")
		}
	})
}

func TestLoadの古いstream_name(t *testing.T) {
	_, err := Load(writeCfg(t, "admin:\n  password: adminpass\nstream:\n  name: live\n  publish_pass: publishpass\n  read_pass: readpass1\n"+oneCam))
	if err == nil {
		t.Fatal("通ってしまった")
	}
	for _, want := range []string{"stream.name は使えなくなりました", "cameras", "id", "書き直して"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("エラーの文に %q がない: %v", want, err)
		}
	}
}

func TestLoadの古いobsの項目(t *testing.T) {
	for _, c := range []struct {
		name, yaml string
		wants      []string
	}{
		{"media_source", "obs:\n  media_source: IRL\n", []string{"obs.media_source は使えなくなりました", "OBS", "メディアソース", "書き直して"}},
		{"monitored_scenes", "obs:\n  monitored_scenes: [Camera]\n", []string{"obs.monitored_scenes は使えなくなりました", "OBS", "シーン", "書き直して"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(writeCfg(t, camMinimal+oneCam+c.yaml))
			if err == nil {
				t.Fatal("通ってしまった")
			}
			for _, want := range c.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("エラーの文に %q がない: %v", want, err)
				}
			}
		})
	}
}

func TestGenerateのカメラと受付(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Cameras) != 1 || cfg.Cameras[0] != (Camera{ID: "main", Name: "メインカメラ", OnBad: "standby"}) {
		t.Fatalf("%+v", cfg.Cameras)
	}
	if !cfg.MediaMTX.RTMPEnabled || !cfg.MediaMTX.SRTEnabled {
		t.Fatalf("%+v", cfg.MediaMTX)
	}
	raw, _ := os.ReadFile(path)
	for _, want := range []string{
		"cameras:",
		"id: main",
		"name: メインカメラ",
		"on_bad: standby",
		"省略可。書いた項目だけ共通の switch を上書きする",
		"rtmp_enabled: true",
		"srt_enabled: true",
		"使わないなら false にできる",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ひな形に %q がない", want)
		}
	}
	if strings.Contains(string(raw), "  name: live") {
		t.Error("stream.name が残っている")
	}
	for _, gone := range []string{"media_source", "monitored_scenes"} {
		if strings.Contains(string(raw), gone) {
			t.Errorf("ひな形に古い項目 %s が残っている", gone)
		}
	}
}
