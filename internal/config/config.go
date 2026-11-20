// Package config は、config.yaml の読み込み、検証、初回生成を行う。
package config

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config は config.yaml 全体。
type Config struct {
	Admin         Admin         `yaml:"admin"`
	MediaMTX      MediaMTX      `yaml:"mediamtx"`
	Stream        Stream        `yaml:"stream"`
	OBS           OBS           `yaml:"obs"`
	OnStreamStart OnStreamStart `yaml:"on_stream_start"`
	Switch        Switch        `yaml:"switch"`
	Log           Log           `yaml:"log"`
	GUI           GUI           `yaml:"gui"`
}

// Admin は管理 API の設定。
type Admin struct {
	// Port は管理 API と管理ページのポート。
	Port int `yaml:"port"`
	// LAN が false なら 127.0.0.1 だけで待ち受け、true なら すべてのインターフェースで待ち受ける。
	LAN      bool   `yaml:"lan"`
	Password string `yaml:"password"`
}

// GUI は小さな窓の設定。
type GUI struct {
	// CloseAction は窓の「×」の動き。minimize / confirm / quit。
	CloseAction string `yaml:"close_action"`
}

// close_action に書ける値。
const (
	CloseMinimize = "minimize"
	CloseConfirm  = "confirm"
	CloseQuit     = "quit"
)

// MediaMTX は MediaMTX の設定。
type MediaMTX struct {
	Binary   string `yaml:"binary"`
	RTMPPort int    `yaml:"rtmp_port"`
	SRTPort  int    `yaml:"srt_port"`
	RTSPPort int    `yaml:"rtsp_port"`
	APIPort  int    `yaml:"api_port"`
}

// Stream は配信（path と資格情報）の設定。
type Stream struct {
	Name        string `yaml:"name"`
	PublishUser string `yaml:"publish_user"`
	PublishPass string `yaml:"publish_pass"`
	ReadUser    string `yaml:"read_user"`
	ReadPass    string `yaml:"read_pass"`
	// ObsReadHost は OBS 用 URL の host。空なら 127.0.0.1。LAN の中継 PC に置くときは中継 PC の LAN のアドレス。
	ObsReadHost string `yaml:"obs_read_host"`
	// PublishRTMPPort / PublishSRTPort は送信用 URL に出すポート。0 なら受信のポート（mediamtx.*）と同じ。
	PublishRTMPPort int `yaml:"publish_rtmp_port"`
	PublishSRTPort  int `yaml:"publish_srt_port"`
	// PublishHost は送信用 URL の host。空なら起動時に LAN の IPv4 を自動で調べる。
	PublishHost string `yaml:"publish_host"`
}

// OBS は OBS への接続設定。
type OBS struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Password    string `yaml:"password"`
	MediaSource string `yaml:"media_source"`
	// StandbyScene は回線が悪い間に出す待機シーン名（1つ）。
	StandbyScene string `yaml:"standby_scene"`
	// MonitoredScenes は監視対象のシーン名（1つ以上）。カメラ映像を使うシーン。
	MonitoredScenes []string `yaml:"monitored_scenes"`
}

// OnStreamStart は配信開始時の切替（安全機能）の設定。
type OnStreamStart struct {
	Enabled bool   `yaml:"enabled"`
	Scene   string `yaml:"scene"`
}

// Switch はシーン切り替えのしきい値。秒は整数の秒、ビットレートは kbps。
type Switch struct {
	AutoEnabled bool `yaml:"auto_enabled"`
	FailKbps    int  `yaml:"fail_kbps"`
	LowSec      int  `yaml:"low_sec"`
	RecoverKbps int  `yaml:"recover_kbps"`
	RecoverSec  int  `yaml:"recover_sec"`
	DownSec     int  `yaml:"down_sec"`
}

// Log はログの出力設定。
type Log struct {
	Dir      string `yaml:"dir"`
	KeepDays int    `yaml:"keep_days"`
}

// defaults は未指定の項目に使う既定値。パスワード類は既定値を持たない（空のまま検証で落ちる）。
func defaults() *Config {
	return &Config{
		Admin: Admin{Port: 8787},
		MediaMTX: MediaMTX{
			RTMPPort: 1935, SRTPort: 8890, RTSPPort: 8554, APIPort: 9997,
		},
		Stream: Stream{Name: "live", PublishUser: "irl", ReadUser: "obs"},
		OBS: OBS{
			Host: "127.0.0.1", Port: 4455, MediaSource: "IRL",
			StandbyScene: "Standby", MonitoredScenes: []string{"Camera"},
		},
		Switch: Switch{AutoEnabled: true, FailKbps: 800, LowSec: 5, RecoverKbps: 1500, RecoverSec: 10, DownSec: 2},
		Log:    Log{Dir: "logs", KeepDays: 14},
		GUI:    GUI{CloseAction: CloseConfirm},
	}
}

// Load は path の config.yaml を読み、未指定の項目に既定値を入れ、未知のキーを拒否し、検証する。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("設定ファイルを読めません: %w", err)
	}
	return parse(data)
}

func parse(data []byte) (*Config, error) {
	cfg := defaults()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		if strings.Contains(err.Error(), "field listen not found in type config.Admin") {
			return nil, fmt.Errorf("設定ファイルの形式が不正です: admin.listen は使えなくなりました。admin.port（ポート番号）と admin.lan（LAN に公開するなら true）に書き直してください: %w", err)
		}
		return nil, fmt.Errorf("設定ファイルの形式が不正です: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

const secretChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const secretLen = 24

func randomSecret() (string, error) {
	max := big.NewInt(int64(len(secretChars)))
	b := make([]byte, secretLen)
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = secretChars[n.Int64()]
	}
	return string(b), nil
}

// defaultTemplate は初回生成するファイルの雛形。"*" は生成時にランダムな値へ置き換える。
const defaultTemplate = `admin:
  port: 8787                 # 管理 API と管理ページのポート
  lan: false                 # false なら 127.0.0.1 だけで待ち受ける。true ならすべてのインターフェース（IPv4 と IPv6）。別 PC から操作するときと、OBS と別の PC（中継 PC）に置くときだけ true にする（通信は平文。ルーターで転送しない）
  password: "*"              # Basic 認証のパスワード（ユーザー名は admin）

mediamtx:
  binary: ""                 # 空なら実行ファイルと同じディレクトリの mediamtx(.exe)
  rtmp_port: 1935
  srt_port: 8890
  rtsp_port: 8554            # OBS からの読み出し用（spike の結果で見直す）
  api_port: 9997             # 127.0.0.1 にだけ公開する

stream:
  name: live                 # MediaMTX の path 名
  publish_user: irl
  publish_pass: "*"
  read_user: obs             # OBS が読み出すときの資格情報（OBS 用 URL に埋め込む）
  read_pass: "*"
  obs_read_host: ""          # OBS 用 URL の host。空なら 127.0.0.1（本ツールを配信 PC に置く場合）。LAN の中継 PC に置くときは、中継 PC の LAN のアドレスを書く
  publish_rtmp_port: 0       # 送信用 URL に出す RTMP のポート。0 なら mediamtx.rtmp_port と同じ。ルーターのポート転送で外側のポートを変えたときに書く
  publish_srt_port: 0        # 送信用 URL に出す SRT のポート。0 なら mediamtx.srt_port と同じ
  publish_host: ""           # 送信用 URL の host。空なら起動時に LAN の IPv4 を自動で調べる（自動）。ポート転送やトンネル、VPN のアドレスが決まっていれば、そのホスト名か IP を書く（手動）

obs:
  host: 127.0.0.1
  port: 4455
  password: ""               # 空なら認証なしで接続する（OBS 側で認証を無効にしている場合）
  media_source: IRL          # MediaMTX を読むメディアソースの入力名
  standby_scene: Standby     # 待機シーン（1つ）。回線が悪い間に出す OBS のシーン名。既定値は Standby
  monitored_scenes:          # 監視対象のシーン（1つ以上）。カメラ映像（メディアソース）を使うシーン。既定値は Camera の1つ
    - Camera

on_stream_start:             # 配信開始時の切替（安全機能）
  enabled: false             # true にすると、配信が始まったときに scene へ切り替える
  scene: ""

switch:
  auto_enabled: true         # 自動切替。あえて乱れた映像を出したいときに false にする
  fail_kbps: 800             # これ未満が low_sec 続いたら LOW
  low_sec: 5
  recover_kbps: 1500         # これ以上が recover_sec 続いたら OK
  recover_sec: 10
  down_sec: 2                # 受信0または未接続がこれだけ続いたら DOWN

log:
  dir: logs
  keep_days: 14

gui:
  close_action: confirm      # 窓の「×」の動き（Windows の小さな窓だけ。第2段階）。minimize / confirm / quit。Linux と macOS では読むが使わない
`

// Generate は既定値の config.yaml を path に書き出して読み込み直す。
// パスワード類はランダムに作る。既にファイルがあるときは上書きせずエラーにする。
func Generate(path string) (*Config, error) {
	content := defaultTemplate
	for strings.Contains(content, `"*"`) {
		s, err := randomSecret()
		if err != nil {
			return nil, fmt.Errorf("乱数を生成できません: %w", err)
		}
		content = strings.Replace(content, `"*"`, `"`+s+`"`, 1)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("設定ファイルが既にあるため上書きしません: %s", path)
		}
		return nil, fmt.Errorf("設定ファイルを作成できません: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return nil, fmt.Errorf("設定ファイルを書き込めません: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("設定ファイルを閉じられません: %w", err)
	}
	return Load(path)
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// hostNameRe は publish_host に書けるホスト名の文字。
var hostNameRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// maxHostLen はホスト名の長さの上限。
const maxHostLen = 253

// FieldError は検証に落ちた項目1つ分。
type FieldError struct{ Field, Reason string }

// ValidationError は検証に落ちた項目すべて（見つけた順）。
type ValidationError struct{ Fields []FieldError }

// Error は1項目1行の "field: reason" をつないだ文を返す。
func (e *ValidationError) Error() string {
	lines := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		lines[i] = f.Field + ": " + f.Reason
	}
	return "設定が不正です:\n  - " + strings.Join(lines, "\n  - ")
}

// ByField は項目名 → 理由。同じ項目に複数あれば「。」でつなぐ。
func (e *ValidationError) ByField() map[string]string {
	m := make(map[string]string, len(e.Fields))
	for _, f := range e.Fields {
		if prev, ok := m[f.Field]; ok {
			m[f.Field] = prev + "。" + f.Reason
		} else {
			m[f.Field] = f.Reason
		}
	}
	return m
}

// Validate は設定値を検証する。違反はすべて集めて、項目名と理由を *ValidationError にまとめる。問題がなければ nil。
func (c *Config) Validate() error {
	var errs []FieldError
	add := func(field, reason string) {
		errs = append(errs, FieldError{field, reason})
	}

	// 閾値
	if c.Switch.RecoverKbps <= c.Switch.FailKbps {
		add("switch.recover_kbps", fmt.Sprintf("switch.fail_kbps（%d）より大きくしてください（現在 %d）", c.Switch.FailKbps, c.Switch.RecoverKbps))
	}
	secs := []struct {
		field string
		v     int
	}{
		{"switch.low_sec", c.Switch.LowSec},
		{"switch.recover_sec", c.Switch.RecoverSec},
		{"switch.down_sec", c.Switch.DownSec},
	}
	for _, s := range secs {
		if s.v < 1 {
			add(s.field, fmt.Sprintf("1 以上にしてください（現在 %d）", s.v))
		}
	}
	if c.Switch.DownSec > c.Switch.LowSec {
		add("switch.down_sec", fmt.Sprintf("switch.low_sec（%d）以下にしてください（現在 %d）", c.Switch.LowSec, c.Switch.DownSec))
	}

	// ポート
	type portEntry struct {
		field string
		port  int
	}
	var ports []portEntry
	for _, p := range []portEntry{
		{"mediamtx.rtmp_port", c.MediaMTX.RTMPPort},
		{"mediamtx.srt_port", c.MediaMTX.SRTPort},
		{"mediamtx.rtsp_port", c.MediaMTX.RTSPPort},
		{"mediamtx.api_port", c.MediaMTX.APIPort},
	} {
		if p.port < 1 || p.port > 65535 {
			add(p.field, fmt.Sprintf("1〜65535 の範囲にしてください（現在 %d）", p.port))
			continue
		}
		ports = append(ports, p)
	}
	if c.OBS.Port < 1 || c.OBS.Port > 65535 {
		add("obs.port", fmt.Sprintf("1〜65535 の範囲にしてください（現在 %d）", c.OBS.Port))
	}
	if c.Admin.Port < 1 || c.Admin.Port > 65535 {
		add("admin.port", fmt.Sprintf("1〜65535 の範囲にしてください（現在 %d）", c.Admin.Port))
	} else {
		ports = append(ports, portEntry{"admin.port", c.Admin.Port})
	}
	for i := range ports {
		for j := i + 1; j < len(ports); j++ {
			if ports[i].port == ports[j].port {
				add(ports[j].field, fmt.Sprintf("%s と同じポート %d を使っています。重複しないようにしてください", ports[i].field, ports[i].port))
			}
		}
	}

	// 資格情報と path 名
	for _, f := range []struct{ field, v string }{
		{"stream.name", c.Stream.Name},
		{"stream.publish_user", c.Stream.PublishUser},
		{"stream.publish_pass", c.Stream.PublishPass},
		{"stream.read_user", c.Stream.ReadUser},
		{"stream.read_pass", c.Stream.ReadPass},
	} {
		if f.v == "" {
			add(f.field, "空にできません")
		} else if !nameRe.MatchString(f.v) {
			add(f.field, "使える文字は英数字・_・- だけです（URL が壊れるため）")
		}
	}
	for _, f := range []struct{ field, v string }{
		{"stream.publish_pass", c.Stream.PublishPass},
		{"stream.read_pass", c.Stream.ReadPass},
	} {
		if f.v != "" && len(f.v) < 8 {
			add(f.field, fmt.Sprintf("8 文字以上にしてください（現在 %d 文字）", len(f.v)))
		}
	}
	for _, f := range []struct{ field, v string }{
		{"stream.publish_user", c.Stream.PublishUser},
		{"stream.read_user", c.Stream.ReadUser},
	} {
		if f.v == "any" {
			add(f.field, "any は使えません（MediaMTX が資格情報を照合せずに通すため）")
		}
	}
	if c.Stream.PublishUser != "" && c.Stream.PublishUser == c.Stream.ReadUser {
		add("stream.read_user", "stream.publish_user と異なる名前にしてください")
	}

	// 送信用 URL と OBS 用 URL の host。IP アドレスか、スキーム・ポート・パス・角括弧を含まないホスト名
	for _, f := range []struct{ field, v string }{
		{"stream.publish_host", c.Stream.PublishHost},
		{"stream.obs_read_host", c.Stream.ObsReadHost},
	} {
		if h := f.v; h != "" && net.ParseIP(h) == nil && (len(h) > maxHostLen || !hostNameRe.MatchString(h)) {
			add(f.field, "空か、ホスト名か IP アドレスにしてください（スキーム、ポート、パス、[ ] は含めず、名前は英数字・.・- の 253 文字以内）")
		}
	}
	// 送信用 URL に出すポート。0 は受信のポートと同じ。外側のポートなので受信のポートとの重複は問わない
	for _, f := range []struct {
		field string
		v     int
	}{
		{"stream.publish_rtmp_port", c.Stream.PublishRTMPPort},
		{"stream.publish_srt_port", c.Stream.PublishSRTPort},
	} {
		if f.v < 0 || f.v > 65535 {
			add(f.field, fmt.Sprintf("0（受信のポートと同じ）か 1〜65535 の範囲にしてください（現在 %d）", f.v))
		}
	}

	// シーン名
	if c.OBS.StandbyScene == "" {
		add("obs.standby_scene", "空にできません")
	}
	if len(c.OBS.MonitoredScenes) == 0 {
		add("obs.monitored_scenes", "1つ以上指定してください")
	}
	for i, s := range c.OBS.MonitoredScenes {
		field := fmt.Sprintf("obs.monitored_scenes[%d]", i)
		if s == "" {
			add(field, "空にできません")
			continue
		}
		if s == c.OBS.StandbyScene {
			add("obs.standby_scene", fmt.Sprintf("obs.monitored_scenes に含まれないシーンにしてください（%q は %s にあります）", s, field))
		}
		for j, prev := range c.OBS.MonitoredScenes[:i] {
			if prev == s {
				add(field, fmt.Sprintf("obs.monitored_scenes[%d] と同じシーン名 %q です。互いに異なる名前にしてください", j, s))
				break
			}
		}
	}
	if c.OnStreamStart.Enabled {
		if c.OnStreamStart.Scene == "" {
			add("on_stream_start.scene", "on_stream_start.enabled が true のときは空にできません")
		} else if c.OnStreamStart.Scene == c.OBS.StandbyScene {
			add("on_stream_start.scene", "obs.standby_scene と異なるシーンにしてください（待機シーンで配信が始まると自動で戻らないため）")
		}
	}

	if c.Admin.Password == "" {
		add("admin.password", "空にできません")
	}

	// ログ
	if c.Log.Dir == "" {
		add("log.dir", "空にできません")
	}
	if c.Log.KeepDays < 0 {
		add("log.keep_days", fmt.Sprintf("0 以上にしてください（現在 %d）", c.Log.KeepDays))
	}

	// GUI
	switch c.GUI.CloseAction {
	case CloseMinimize, CloseConfirm, CloseQuit:
	default:
		add("gui.close_action", fmt.Sprintf("minimize、confirm、quit のどれかにしてください（現在 %q）", c.GUI.CloseAction))
	}

	if len(errs) == 0 {
		return nil
	}
	return &ValidationError{Fields: errs}
}

// Warnings は起動のたびにログへ出す警告を返す。admin.lan が true のときだけ警告がある。
func (c *Config) Warnings() []string {
	if !c.Admin.LAN {
		return nil
	}
	return []string{"admin.lan が true のため、管理 API と管理ページを LAN に公開しています。通信は平文のため、LAN 内で盗聴されるおそれがあります。パスワードを取られると、LAN の機器から、すべてのパスワードの表示と設定の変更ができます。ルーターでこのポートを転送しないでください"}
}
