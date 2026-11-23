package supervisor

import (
	"flag"
	"github.com/teru358/irl-live-keeper/internal/i18n"
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

// camsOne は1台のカメラ（id は live）の cameras の書き方。
const camsOne = "cameras:\n  - id: live\n    name: ライブ\n    on_bad: standby\n"

// loadCfgWith は固定のパスワードを入れた config を、既定値つきで読み込む。cams は cameras の区分の全体、extra は mediamtx などの追加。
func loadCfgWith(t *testing.T, cams, extra string) *config.Config {
	t.Helper()
	return loadCfgApp(t, "", cams, extra)
}

// loadCfgApp は loadCfgWith に stream の追加の行（例: `  app: ""\n`）をつけたもの。
func loadCfgApp(t *testing.T, streamExtra, cams, extra string) *config.Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	body := "admin:\n  password: adminPASS123\nstream:\n  publish_pass: pubPASS123\n  read_pass: readPASS123\n" + streamExtra + cams + extra
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// loadCfg は id が live の1台の config を、既定値つきで読み込む。
func loadCfg(t *testing.T, extra string) *config.Config {
	t.Helper()
	return loadCfgWith(t, camsOne, extra)
}

// camsThree は id が main、sub、hand の3台（main と hand は standby、sub は hide）。
const camsThree = "cameras:\n  - id: main\n    name: メインカメラ\n    on_bad: standby\n  - id: sub\n    name: サブカメラ\n    on_bad: hide\n  - id: hand\n    name: 手元カメラ\n    on_bad: standby\n"

// camsTwo は id が main と sub の2台。
const camsTwo = "cameras:\n  - id: main\n    name: メイン\n    on_bad: standby\n  - id: sub\n    name: サブ\n    on_bad: hide\n"

// TestGenerateYAMLGoldenVariants は、カメラの台数（1台と3台）と受付の組み合わせ（両方、RTMP だけ、SRT だけ）ごとに、
// 手で書いたゴールデンと一致することを確かめる。ゴールデンは -update で作らない。
func TestGenerateYAMLGoldenVariants(t *testing.T) {
	tests := []struct {
		name   string
		cams   string
		extra  string
		golden string
		ids    []string
	}{
		{"3台、両方有効", camsThree, "", "golden-3cams.yml", []string{"live/main", "live/sub", "live/hand"}},
		{"RTMP だけ（SRT を止める）", camsOne, "mediamtx:\n  srt_enabled: false\n", "golden-rtmp-only.yml", []string{"live/live"}},
		{"SRT だけ（RTMP を止める）", camsOne, "mediamtx:\n  rtmp_enabled: false\n", "golden-srt-only.yml", []string{"live/live"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateYAML(loadCfgWith(t, tt.cams, tt.extra), "APIUSER", "APIPASS")
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tt.golden))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("ゴールデン %s と違います\n--- got ---\n%s", tt.golden, got)
			}
			// ゴールデンの手書きが正しいことを、構造でも確かめる: paths と publish / read の path が id の列と同じ。
			var doc struct {
				Paths map[string]any `yaml:"paths"`
				Users []struct {
					User        string `yaml:"user"`
					Permissions []struct {
						Action string `yaml:"action"`
						Path   string `yaml:"path"`
					} `yaml:"permissions"`
				} `yaml:"authInternalUsers"`
			}
			if err := yaml.Unmarshal(got, &doc); err != nil {
				t.Fatal(err)
			}
			if len(doc.Paths) != len(tt.ids) {
				t.Errorf("paths の数 = %d, want %d", len(doc.Paths), len(tt.ids))
			}
			for _, id := range tt.ids {
				if _, ok := doc.Paths[id]; !ok {
					t.Errorf("paths に %q がない", id)
				}
			}
			for _, u := range doc.Users {
				var paths []string
				for _, p := range u.Permissions {
					if p.Action == "publish" || p.Action == "read" {
						paths = append(paths, p.Path)
					}
				}
				if u.User == "APIUSER" {
					continue
				}
				if !reflect.DeepEqual(paths, tt.ids) {
					t.Errorf("ユーザー %s の path = %v, want %v", u.User, paths, tt.ids)
				}
			}
		})
	}
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
	p := PublishURLs(cfg, "192.168.1.5")[0]
	if want := "srt://192.168.1.5:8890?streamid=publish:live/live:irl:pubPASS123"; p.SRT != want {
		t.Errorf("SRT = %q", p.SRT)
	}
	if want := "rtmp://192.168.1.5:1935/live/live?user=irl&pass=pubPASS123"; p.RTMP != want {
		t.Errorf("RTMP = %q", p.RTMP)
	}
	r := ReadURLs(cfg, "127.0.0.1")[0]
	if want := "rtsp://obs:readPASS123@127.0.0.1:8554/live/live"; r.RTSP != want {
		t.Errorf("RTSP = %q", r.RTSP)
	}
	if want := "rtmp://127.0.0.1:1935/live/live?user=obs&pass=readPASS123"; r.RTMP != want {
		t.Errorf("RTMP = %q", r.RTMP)
	}
	if want := "srt://127.0.0.1:8890?streamid=read:live/live:obs:readPASS123"; r.SRT != want {
		t.Errorf("SRT = %q", r.SRT)
	}
	if got := PublishURLs(cfg, "::1")[0].RTMP; !strings.HasPrefix(got, "rtmp://[::1]:1935/") {
		t.Errorf("IPv6 の括弧がありません: %q", got)
	}
}

// URL の表: カメラの数、止めたプロトコル、publish_host・publish_*_port・obs_read_host の組み合わせ。
// 期待の文字列は手で書く（生成の関数と同じ式を使わない）。
func TestURLTable(t *testing.T) {
	tests := []struct {
		name              string
		cams, extra       string
		pubHost           string
		rtmpPort, srtPort int
		obsHost           string
		wantPub           []PublishURLSet
		wantRead          []ReadURLSet
	}{
		{
			name:    "2台、両方",
			cams:    camsTwo,
			pubHost: "192.168.1.5",
			obsHost: "127.0.0.1",
			wantPub: []PublishURLSet{
				{ID: "main", SRT: "srt://192.168.1.5:8890?streamid=publish:live/main:irl:pubPASS123", RTMP: "rtmp://192.168.1.5:1935/live/main?user=irl&pass=pubPASS123"},
				{ID: "sub", SRT: "srt://192.168.1.5:8890?streamid=publish:live/sub:irl:pubPASS123", RTMP: "rtmp://192.168.1.5:1935/live/sub?user=irl&pass=pubPASS123"},
			},
			wantRead: []ReadURLSet{
				{ID: "main", RTSP: "rtsp://obs:readPASS123@127.0.0.1:8554/live/main", RTMP: "rtmp://127.0.0.1:1935/live/main?user=obs&pass=readPASS123", SRT: "srt://127.0.0.1:8890?streamid=read:live/main:obs:readPASS123"},
				{ID: "sub", RTSP: "rtsp://obs:readPASS123@127.0.0.1:8554/live/sub", RTMP: "rtmp://127.0.0.1:1935/live/sub?user=obs&pass=readPASS123", SRT: "srt://127.0.0.1:8890?streamid=read:live/sub:obs:readPASS123"},
			},
		},
		{
			// RTMP を止めたら、publish_rtmp_port を書いてあっても RTMP の URL は作らない。OBS 用の RTSP は作る。
			name:     "2台、RTMP を止める",
			cams:     camsTwo,
			extra:    "mediamtx:\n  rtmp_enabled: false\n",
			pubHost:  "::1",
			rtmpPort: 21935,
			obsHost:  "10.0.0.9",
			wantPub: []PublishURLSet{
				{ID: "main", SRT: "srt://[::1]:8890?streamid=publish:live/main:irl:pubPASS123", RTMP: ""},
				{ID: "sub", SRT: "srt://[::1]:8890?streamid=publish:live/sub:irl:pubPASS123", RTMP: ""},
			},
			wantRead: []ReadURLSet{
				{ID: "main", RTSP: "rtsp://obs:readPASS123@10.0.0.9:8554/live/main", RTMP: "", SRT: "srt://10.0.0.9:8890?streamid=read:live/main:obs:readPASS123"},
				{ID: "sub", RTSP: "rtsp://obs:readPASS123@10.0.0.9:8554/live/sub", RTMP: "", SRT: "srt://10.0.0.9:8890?streamid=read:live/sub:obs:readPASS123"},
			},
		},
		{
			// SRT を止めたら、publish_srt_port を書いてあっても SRT の URL は作らない。
			name:     "1台、SRT を止める",
			cams:     camsOne,
			extra:    "mediamtx:\n  srt_enabled: false\n",
			pubHost:  "example.com",
			rtmpPort: 21935,
			srtPort:  28890,
			obsHost:  "127.0.0.1",
			wantPub: []PublishURLSet{
				{ID: "live", SRT: "", RTMP: "rtmp://example.com:21935/live/live?user=irl&pass=pubPASS123"},
			},
			wantRead: []ReadURLSet{
				{ID: "live", RTSP: "rtsp://obs:readPASS123@127.0.0.1:8554/live/live", RTMP: "rtmp://127.0.0.1:1935/live/live?user=obs&pass=readPASS123", SRT: ""},
			},
		},
		{
			name:     "1台、両方、publish のポートを上書き、OBS の host は LAN のアドレス",
			cams:     camsOne,
			pubHost:  "example.com",
			rtmpPort: 21935,
			srtPort:  28890,
			obsHost:  "192.168.1.20",
			wantPub: []PublishURLSet{
				{ID: "live", SRT: "srt://example.com:28890?streamid=publish:live/live:irl:pubPASS123", RTMP: "rtmp://example.com:21935/live/live?user=irl&pass=pubPASS123"},
			},
			wantRead: []ReadURLSet{
				{ID: "live", RTSP: "rtsp://obs:readPASS123@192.168.1.20:8554/live/live", RTMP: "rtmp://192.168.1.20:1935/live/live?user=obs&pass=readPASS123", SRT: "srt://192.168.1.20:8890?streamid=read:live/live:obs:readPASS123"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadCfgWith(t, tt.cams, tt.extra)
			cfg.Stream.PublishRTMPPort, cfg.Stream.PublishSRTPort = tt.rtmpPort, tt.srtPort
			if got := PublishURLs(cfg, tt.pubHost); !reflect.DeepEqual(got, tt.wantPub) {
				t.Errorf("送信用 URL\n got: %+v\nwant: %+v", got, tt.wantPub)
			}
			if got := ReadURLs(cfg, tt.obsHost); !reflect.DeepEqual(got, tt.wantRead) {
				t.Errorf("読み出し用 URL\n got: %+v\nwant: %+v", got, tt.wantRead)
			}
		})
	}
}

func TestPublishURLsPorts(t *testing.T) {
	cfg := loadCfg(t, "")
	cfg.Stream.PublishRTMPPort, cfg.Stream.PublishSRTPort = 21935, 28890
	p := PublishURLs(cfg, "example.com")[0]
	if want := "srt://example.com:28890?streamid=publish:live/live:irl:pubPASS123"; p.SRT != want {
		t.Errorf("SRT = %q, want %q", p.SRT, want)
	}
	if want := "rtmp://example.com:21935/live/live?user=irl&pass=pubPASS123"; p.RTMP != want {
		t.Errorf("RTMP = %q, want %q", p.RTMP, want)
	}
	// 片方だけ上書き。0 の側は受信のポート
	cfg.Stream.PublishSRTPort = 0
	p = PublishURLs(cfg, "example.com")[0]
	if want := "srt://example.com:8890?streamid=publish:live/live:irl:pubPASS123"; p.SRT != want {
		t.Errorf("SRT = %q, want %q", p.SRT, want)
	}
	// 読み出し側の候補は受信のポートのまま
	if want := "rtmp://example.com:1935/live/live?user=obs&pass=readPASS123"; ReadURLs(cfg, "example.com")[0].RTMP != want {
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
		ja := i18n.Render(i18n.JA, i18n.Reason(err)) // 画面とコンソールに出す日本語の文（Error() はログ用の英語）
		if !strings.Contains(ja, h.name) || !strings.Contains(ja, strconv.Itoa(*h.port)) || !strings.Contains(ja, "mediamtx が残っていない") {
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
				if ja := i18n.Render(i18n.JA, i18n.Reason(err)); !strings.Contains(ja, want) {
					t.Errorf("エラー文に %q がありません: %v", want, ja)
				}
			}
		})
	}
}

// 止めたプロトコルのポートは確かめない。使われていても止めず、有効なほうのポートが使われていれば、その名前で落ちる。
func TestCheckPortsStoppedProtocol(t *testing.T) {
	modes := []struct {
		name      string
		rtmp, srt bool
	}{
		{"両方", true, true},
		{"RTMP だけ（SRT を止める）", true, false},
		{"SRT だけ（RTMP を止める）", false, true},
	}
	protos := []struct {
		name, network string
		port          func(*config.Config) *int
	}{
		{"rtmp_port", "tcp", func(c *config.Config) *int { return &c.MediaMTX.RTMPPort }},
		{"srt_port", "udp", func(c *config.Config) *int { return &c.MediaMTX.SRTPort }},
	}
	for _, m := range modes {
		for _, p := range protos {
			t.Run(m.name+"/"+p.name, func(t *testing.T) {
				cfg := freePorts(t)
				cfg.MediaMTX.RTMPEnabled, cfg.MediaMTX.SRTEnabled = m.rtmp, m.srt
				port := *p.port(cfg)
				var closer interface{ Close() error }
				var err error
				if p.network == "udp" {
					closer, err = net.ListenPacket("udp", ":"+strconv.Itoa(port))
				} else {
					closer, err = net.Listen("tcp", ":"+strconv.Itoa(port))
				}
				if err != nil {
					t.Fatal(err)
				}
				defer closer.Close()
				err = CheckPorts(cfg)
				enabled := (p.name == "rtmp_port" && m.rtmp) || (p.name == "srt_port" && m.srt)
				switch {
				case enabled && err == nil:
					t.Errorf("有効な %s が使用中なのに成功した", p.name)
				case enabled && !strings.Contains(err.Error(), p.name):
					t.Errorf("エラー文に %s がない: %v", p.name, err)
				case !enabled && err != nil:
					t.Errorf("止めたプロトコルの %s を確かめた: %v", p.name, err)
				}
			})
		}
	}
}

// YAML で真偽や数に読まれうる id（on、no、1）も、path の名前と権限の path が文字列のまま MediaMTX に渡ること。
func TestGenerateYAMLQuotesAmbiguousIDs(t *testing.T) {
	cams := "cameras:\n  - id: \"on\"\n    name: a\n    on_bad: standby\n  - id: \"no\"\n    name: b\n    on_bad: standby\n  - id: \"1\"\n    name: c\n    on_bad: standby\n"
	cfg := loadCfgWith(t, cams, "")
	out, err := GenerateYAML(cfg, "apiU", "apiP")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		AuthInternalUsers []struct {
			Permissions []struct {
				Action string `yaml:"action"`
				Path   any    `yaml:"path"`
			} `yaml:"permissions"`
		} `yaml:"authInternalUsers"`
		Paths map[any]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"live/on", "live/no", "live/1"} {
		if _, ok := doc.Paths[id]; !ok {
			t.Errorf("paths に文字列の %q がない（%v）", id, doc.Paths)
		}
	}
	for _, u := range doc.AuthInternalUsers {
		for _, p := range u.Permissions {
			if p.Path == nil {
				continue
			}
			if _, ok := p.Path.(string); !ok {
				t.Errorf("%s の path %v（%T）が文字列でない", p.Action, p.Path, p.Path)
			}
		}
	}
}

// app を空にすると、今までの1段の path（<カメラの id> だけ）で生成する。
func TestGenerateYAMLAppEmpty(t *testing.T) {
	cfg := loadCfgApp(t, "  app: \"\"\n", camsTwo, "")
	got, err := GenerateYAML(cfg, "APIUSER", "APIPASS")
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{"  \"main\": {}\n", "  \"sub\": {}\n", "path: \"main\"\n", "path: \"sub\"\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q がない:\n%s", want, s)
		}
	}
	if strings.Contains(s, "live/") {
		t.Errorf("app が空なのに2段の path:\n%s", s)
	}
	if want := "rtmp://h:1935/main?user=irl&pass=pubPASS123"; PublishURLs(cfg, "h")[0].RTMP != want {
		t.Errorf("RTMP = %q", PublishURLs(cfg, "h")[0].RTMP)
	}
	if want := "srt://h:8890?streamid=publish:main:irl:pubPASS123"; PublishURLs(cfg, "h")[0].SRT != want {
		t.Errorf("SRT = %q", PublishURLs(cfg, "h")[0].SRT)
	}
	if want := "rtsp://obs:readPASS123@h:8554/main"; ReadURLs(cfg, "h")[0].RTSP != want {
		t.Errorf("RTSP = %q", ReadURLs(cfg, "h")[0].RTSP)
	}
}

// app を変えると、その名前の2段の path で生成する。
func TestGenerateYAMLCustomApp(t *testing.T) {
	cfg := loadCfgApp(t, "  app: show\n", camsOne, "")
	got, err := GenerateYAML(cfg, "APIUSER", "APIPASS")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "  \"show/live\": {}\n") {
		t.Errorf("show/live の path がない:\n%s", got)
	}
	if want := "rtmp://h:1935/show/live?user=irl&pass=pubPASS123"; PublishURLs(cfg, "h")[0].RTMP != want {
		t.Errorf("RTMP = %q", PublishURLs(cfg, "h")[0].RTMP)
	}
}
