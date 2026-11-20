package supervisor

import (
	"flag"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/teru358/irl-live-keeper/internal/config"
)

var update = flag.Bool("update", false, "ゴールデンファイルを更新する")

// loadCfg は固定のパスワードを入れた config を、既定値つきで読み込む。
func loadCfg(t *testing.T, extra string) *config.Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	body := "admin:\n  password: adminPASS123\nstream:\n  publish_pass: pubPASS123\n  read_pass: readPASS123\n" + extra
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestGenerateYAMLGolden(t *testing.T) {
	got, err := GenerateYAML(loadCfg(t, ""), "APIUSER", "APIPASS")
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "golden-default.yml")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("ゴールデンと違います\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	s := string(got)
	if !strings.HasPrefix(s, "# 自動生成。編集しないこと\n") {
		t.Error("先頭の注意書きがありません")
	}
	for _, sw := range []string{"hls: false", "webrtc: false", "moq: false", "playback: false", "metrics: false", "pprof: false", "rtspTransports: [tcp]", "authMethod: internal"} {
		if !strings.Contains(s, sw) {
			t.Errorf("スイッチ %q が明示されていません", sw)
		}
	}
}

// 見本（実物の v1.21.4 で動作確認済み）と同じ入力から、意味として同じ文書になる。
func TestGenerateYAMLMatchesReference(t *testing.T) {
	cfg := loadCfg(t, "mediamtx:\n  rtmp_port: 11935\n  srt_port: 18890\n  rtsp_port: 18554\n  api_port: 19997\n")
	got, err := GenerateYAML(cfg, "APIUSER", "APIPASS")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := os.ReadFile(filepath.Join("testdata", "reference-v1.21.4.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var a, b map[string]any
	if err := yaml.Unmarshal(got, &a); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(ref, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("見本と違います\n got: %v\nwant: %v", a, b)
	}
}

func TestGenerateYAMLRejectsEmptyAPICreds(t *testing.T) {
	if _, err := GenerateYAML(loadCfg(t, ""), "", "x"); err == nil {
		t.Error("空の資格情報を受け付けた")
	}
}

func TestURLs(t *testing.T) {
	cfg := loadCfg(t, "")
	p := PublishURLs(cfg, "192.168.1.5")
	if want := "srt://192.168.1.5:8890?streamid=publish:live:irl:pubPASS123"; p.SRT != want {
		t.Errorf("SRT = %q", p.SRT)
	}
	if want := "rtmp://192.168.1.5:1935/live?user=irl&pass=pubPASS123"; p.RTMP != want {
		t.Errorf("RTMP = %q", p.RTMP)
	}
	r := ReadURLs(cfg, "127.0.0.1")
	if want := "rtsp://obs:readPASS123@127.0.0.1:8554/live"; r.RTSP != want {
		t.Errorf("RTSP = %q", r.RTSP)
	}
	if want := "rtmp://127.0.0.1:1935/live?user=obs&pass=readPASS123"; r.RTMP != want {
		t.Errorf("RTMP = %q", r.RTMP)
	}
	if want := "srt://127.0.0.1:8890?streamid=read:live:obs:readPASS123"; r.SRT != want {
		t.Errorf("SRT = %q", r.SRT)
	}
	if got := PublishURLs(cfg, "::1").RTMP; !strings.HasPrefix(got, "rtmp://[::1]:1935/") {
		t.Errorf("IPv6 の括弧がありません: %q", got)
	}
}

func TestPublishURLsPorts(t *testing.T) {
	cfg := loadCfg(t, "")
	cfg.Stream.PublishRTMPPort, cfg.Stream.PublishSRTPort = 21935, 28890
	p := PublishURLs(cfg, "example.com")
	if want := "srt://example.com:28890?streamid=publish:live:irl:pubPASS123"; p.SRT != want {
		t.Errorf("SRT = %q, want %q", p.SRT, want)
	}
	if want := "rtmp://example.com:21935/live?user=irl&pass=pubPASS123"; p.RTMP != want {
		t.Errorf("RTMP = %q, want %q", p.RTMP, want)
	}
	// 片方だけ上書き。0 の側は受信のポート
	cfg.Stream.PublishSRTPort = 0
	p = PublishURLs(cfg, "example.com")
	if want := "srt://example.com:8890?streamid=publish:live:irl:pubPASS123"; p.SRT != want {
		t.Errorf("SRT = %q, want %q", p.SRT, want)
	}
	// 読み出し側の候補は受信のポートのまま
	if want := "rtmp://example.com:1935/live?user=obs&pass=readPASS123"; ReadURLs(cfg, "example.com").RTMP != want {
		t.Errorf("読み出し側のポートが変わった")
	}
}

// freePorts は空いているポートを4つ選んで cfg に入れる。
func freePorts(t *testing.T) *config.Config {
	t.Helper()
	var nums [4]int
	for i := range nums {
		for {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			n := l.Addr().(*net.TCPAddr).Port
			l.Close()
			// srt は UDP なので UDP でも空いているものだけ使う。
			u, err := net.ListenPacket("udp", ":"+strconv.Itoa(n))
			if err != nil {
				continue
			}
			u.Close()
			dup := false
			for _, p := range nums[:i] {
				dup = dup || p == n
			}
			if !dup {
				nums[i] = n
				break
			}
		}
	}
	cfg := loadCfg(t, "")
	cfg.MediaMTX.RTMPPort, cfg.MediaMTX.SRTPort, cfg.MediaMTX.RTSPPort, cfg.MediaMTX.APIPort = nums[0], nums[1], nums[2], nums[3]
	return cfg
}

func TestCheckPorts(t *testing.T) {
	cfg := freePorts(t)
	if err := CheckPorts(cfg); err != nil {
		t.Fatalf("空きポートで失敗: %v", err)
	}
	// 各ポートを自分で塞いで、その名前がエラーに出ることを確かめる。
	hold := []struct {
		name string
		port *int
		udp  bool
		addr string
	}{
		{"rtmp_port", &cfg.MediaMTX.RTMPPort, false, ":"},
		{"srt_port", &cfg.MediaMTX.SRTPort, true, ":"},
		{"rtsp_port", &cfg.MediaMTX.RTSPPort, false, ":"},
		{"api_port", &cfg.MediaMTX.APIPort, false, "127.0.0.1:"},
	}
	for _, h := range hold {
		var closer interface{ Close() error }
		var err error
		if h.udp {
			closer, err = net.ListenPacket("udp", h.addr+strconv.Itoa(*h.port))
		} else {
			closer, err = net.Listen("tcp", h.addr+strconv.Itoa(*h.port))
		}
		if err != nil {
			t.Fatal(err)
		}
		err = CheckPorts(cfg)
		closer.Close()
		if err == nil {
			t.Errorf("%s: 使用中なのに成功した", h.name)
			continue
		}
		if !strings.Contains(err.Error(), h.name) || !strings.Contains(err.Error(), strconv.Itoa(*h.port)) || !strings.Contains(err.Error(), "mediamtx が残っていない") {
			t.Errorf("%s: エラー文が不十分: %v", h.name, err)
		}
	}
}

// 全インターフェースのポートは、ほかのプロセスが IPv4 の 0.0.0.0:port を使っているときにも
// 落ちて、そのポート名と tcp4 / udp4 がエラーに出ることを確かめる（Windows の実機で見つかった形）。
func TestCheckPortsIPv4Wildcard(t *testing.T) {
	cases := []struct {
		name, network string
		port          func(*config.Config) *int
	}{
		{"rtmp_port", "tcp4", func(c *config.Config) *int { return &c.MediaMTX.RTMPPort }},
		{"srt_port", "udp4", func(c *config.Config) *int { return &c.MediaMTX.SRTPort }},
		{"rtsp_port", "tcp4", func(c *config.Config) *int { return &c.MediaMTX.RTSPPort }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := freePorts(t)
			port := *c.port(cfg)
			addr := "0.0.0.0:" + strconv.Itoa(port)
			var closer interface{ Close() error }
			var err error
			if c.network == "udp4" {
				closer, err = net.ListenPacket(c.network, addr)
			} else {
				closer, err = net.Listen(c.network, addr)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer closer.Close()
			err = CheckPorts(cfg)
			if err == nil {
				t.Fatal("IPv4 の 0.0.0.0 で使用中なのに成功した")
			}
			for _, want := range []string{c.name, strconv.Itoa(port), "（" + c.network + "）", "mediamtx が残っていない"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("エラー文に %q がありません: %v", want, err)
				}
			}
		})
	}
}
