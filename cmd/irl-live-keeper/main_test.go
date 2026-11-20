package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
)

// syncBuffer は複数の goroutine から書かれる標準出力・標準エラーの代わり。
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// freeTCPPort は今空いている TCP ポートを返す。
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// freeUDPPort は今空いている UDP ポートを返す。
func freeUDPPort(t *testing.T) int {
	t.Helper()
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port
}

// useGenerate は config の生成を差し替える。生成した config.yaml のポートを空いているものに、
// mediamtx.binary を binary に書き換えて読み直す（この PC の 1935 などが使われていても動くように）。
// 戻り値は、生成された設定を受け取る関数。
func useGenerate(t *testing.T, binary string, tcpOverride map[string]int) func() *config.Config {
	t.Helper()
	var mu sync.Mutex
	var generated *config.Config
	orig := generate
	t.Cleanup(func() { generate = orig })
	generate = func(path string) (*config.Config, error) {
		if _, err := orig(path); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		s := string(raw)
		// 互いに重複しないポートを選ぶ（閉じた直後は同じ番号が返ることがある）
		used := map[int]bool{}
		pick := func(f func(*testing.T) int) int {
			for {
				if p := f(t); !used[p] {
					used[p] = true
					return p
				}
			}
		}
		ports := map[string]int{
			"rtmp_port": pick(freeTCPPort), "srt_port": pick(freeUDPPort),
			"rtsp_port": pick(freeTCPPort), "api_port": pick(freeTCPPort),
		}
		for k, v := range tcpOverride {
			ports[k] = v
		}
		for k, v := range ports {
			re := regexp.MustCompile(`(?m)^(\s+` + k + `:) \d+`)
			s = re.ReplaceAllString(s, fmt.Sprintf("${1} %d", v))
		}
		s = strings.Replace(s, `listen: "127.0.0.1:8787"`, fmt.Sprintf(`listen: "127.0.0.1:%d"`, pick(freeTCPPort)), 1)
		s = strings.Replace(s, `binary: ""`, fmt.Sprintf(`binary: %q`, binary), 1)
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			return nil, err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		generated = cfg
		mu.Unlock()
		return cfg, nil
	}
	return func() *config.Config {
		mu.Lock()
		defer mu.Unlock()
		return generated
	}
}

func runInBackground(ctx context.Context, base string, stdout, stderr *syncBuffer) <-chan int {
	done := make(chan int, 1)
	go func() { done <- run(ctx, base, stdout, stderr) }()
	return done
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("待ちきれなかった: %s", what)
}

func readLogs(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	return all.String()
}

func TestRunは生成した設定の秘密をログファイルに書かない(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("シェルスクリプトの偽の MediaMTX を使うため")
	}
	base := t.TempDir()
	bin := filepath.Join(base, "fake-mediamtx.sh")
	cfgOf := useGenerate(t, bin, nil)

	var stdout, stderr syncBuffer
	// 偽の MediaMTX は、生成された設定のパスワードを標準出力に書く。config の生成後にスクリプトを書き換えるため、
	// generate の差し替えのあとで、もう一度包む。
	inner := generate
	generate = func(path string) (*config.Config, error) {
		cfg, err := inner(path)
		if err != nil {
			return nil, err
		}
		script := fmt.Sprintf("#!/bin/sh\necho 'publish pass=%s'\necho 'read pass=%s'\necho 'admin pass=%s'\nexec sleep 30\n",
			cfg.Stream.PublishPass, cfg.Stream.ReadPass, cfg.Admin.Password)
		return cfg, os.WriteFile(bin, []byte(script), 0o700)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runInBackground(ctx, base, &stdout, &stderr)

	logDir := filepath.Join(base, "logs")
	// 偽の MediaMTX の出力が、置き換えられてログに載るまで待つ
	waitFor(t, "偽の MediaMTX の出力がログに載る", func() bool {
		return strings.Contains(readLogs2(logDir), "admin pass=***")
	})
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("終了コード = %d, want 0\nstderr: %s", code, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run が終わらない")
	}

	cfg := cfgOf()
	if cfg == nil {
		t.Fatal("config が生成されていない")
	}
	out := stdout.String()
	// 標準出力には、URL と生成した管理 API のパスワードが1回出る
	for _, want := range []string{cfg.Admin.Password, cfg.Stream.PublishPass, cfg.Stream.ReadPass, "srt://", "rtmp://", "rtsp://"} {
		if !strings.Contains(out, want) {
			t.Errorf("標準出力に %q がない", want)
		}
	}
	if n := strings.Count(out, cfg.Admin.Password); n != 1 {
		t.Errorf("管理 API のパスワードが標準出力に %d 回出た（1回のはず）", n)
	}
	// ログファイルには、秘密の文字列が1つも出ない
	logs := readLogs(t, logDir)
	if logs == "" {
		t.Fatal("ログファイルが空")
	}
	for name, secret := range map[string]string{
		"admin.password": cfg.Admin.Password, "publish_pass": cfg.Stream.PublishPass, "read_pass": cfg.Stream.ReadPass,
	} {
		if strings.Contains(logs, secret) {
			t.Errorf("ログファイルに %s が出ている", name)
		}
	}
	// 標準出力のうちログの行（起動の表示以外）にも出ない。偽の MediaMTX の行で確かめる
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "component=mediamtx") {
			for _, secret := range []string{cfg.Admin.Password, cfg.Stream.PublishPass, cfg.Stream.ReadPass} {
				if strings.Contains(line, secret) {
					t.Errorf("標準出力のログの行に秘密が出ている: %s", line)
				}
			}
		}
	}
}

func TestRunはOBSのパスワードをログファイルにも標準出力のログの行にも書かない(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("シェルスクリプトの偽の MediaMTX を使うため")
	}
	base := t.TempDir()
	bin := filepath.Join(base, "fake-mediamtx.sh")
	useGenerate(t, bin, nil)
	// 実機の OBS に届かないよう、閉じたポートにつなぐ
	obsPort := freeTCPPort(t)

	// 英数字だけの固定の文字列にする（YAML や slog の引用で形が変わらないように）
	const obsPass = "obsSecret8f2c1d4e"
	inner := generate
	generate = func(path string) (*config.Config, error) {
		if _, err := inner(path); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// 雛形で空の password は obs.password の1箇所だけ
		s := string(raw)
		if strings.Count(s, `password: ""`) != 1 {
			return nil, fmt.Errorf("config.yaml の空の password が1つではない")
		}
		s = strings.Replace(s, `password: ""`, fmt.Sprintf("password: %q", obsPass), 1)
		s = regexp.MustCompile(`(?m)^(\s+port: )4455`).ReplaceAllString(s, fmt.Sprintf("${1}%d", obsPort))
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			return nil, err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return nil, err
		}
		if cfg.OBS.Password != obsPass {
			return nil, fmt.Errorf("obs.password が書き換わっていない")
		}
		// 偽の MediaMTX は、OBS のパスワードを標準出力に書く
		script := fmt.Sprintf("#!/bin/sh\necho 'obs pass=%s'\nexec sleep 30\n", obsPass)
		if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	var stdout, stderr syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runInBackground(ctx, base, &stdout, &stderr)

	logDir := filepath.Join(base, "logs")
	// 偽の MediaMTX の行が届くまで待つ。置き換えの有無はここでは問わない（外れていれば下で落ちる）
	waitFor(t, "偽の MediaMTX の出力がログに載る", func() bool {
		return strings.Contains(readLogs2(logDir), "obs pass=")
	})
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("終了コード = %d, want 0\nstderr: %s", code, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run が終わらない")
	}

	logs := readLogs(t, logDir)
	// 置き換えられて *** として出ていること。出ていなければ、確かめられていない
	if !strings.Contains(logs, "obs pass=***") {
		t.Error("ログファイルに obs pass=*** がない")
	}
	if !strings.Contains(stdout.String(), "obs pass=***") {
		t.Error("標準出力に obs pass=*** がない")
	}
	if strings.Contains(logs, obsPass) {
		t.Error("ログファイルに obs.password が出ている")
	}
	// 起動の表示には OBS のパスワードは出ない。標準出力の全体で確かめる
	if strings.Contains(stdout.String(), obsPass) {
		t.Error("標準出力に obs.password が出ている")
	}
}

// readLogs2 は失敗しても空を返す readLogs（待ちの途中で使う）。
func readLogs2(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var all strings.Builder
	for _, e := range entries {
		if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			all.Write(b)
		}
	}
	return all.String()
}

func TestRunは不正な設定で終了コード1と項目名を出す(t *testing.T) {
	base := t.TempDir()
	bad := "switch:\n  low_sec: 0\n  recover_kbps: 100\n"
	if err := os.WriteFile(filepath.Join(base, "config.yaml"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr syncBuffer
	code := run(context.Background(), base, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("終了コード = %d, want 1", code)
	}
	for _, field := range []string{"switch.low_sec", "switch.recover_kbps", "admin.password"} {
		if !strings.Contains(stderr.String(), field) {
			t.Errorf("標準エラーに項目名 %s がない:\n%s", field, stderr.String())
		}
	}
	if strings.Contains(stdout.String(), "srt://") {
		t.Error("不正な設定なのに起動の表示が出ている")
	}
	if _, err := os.Stat(filepath.Join(base, "logs")); err == nil {
		t.Error("不正な設定なのにログのディレクトリを作った")
	}
}

func TestRunはポートが使用中なら終了コード1(t *testing.T) {
	base := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	busy := l.Addr().(*net.TCPAddr).Port
	useGenerate(t, filepath.Join(base, "none"), map[string]int{"rtmp_port": busy})

	var stdout, stderr syncBuffer
	code := run(context.Background(), base, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("終了コード = %d, want 1\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "rtmp_port") || !strings.Contains(stderr.String(), "前回の mediamtx") {
		t.Errorf("標準エラーにポートの名前と案内がない:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "srt://") {
		t.Error("起動を止めたのに起動の表示が出ている")
	}
	if got := readLogs(t, filepath.Join(base, "logs")); !strings.Contains(got, "rtmp_port") {
		t.Errorf("ログファイルにポートの名前がない:\n%s", got)
	}
}

func TestRunは管理APIの待ち受けに失敗したら終了コード1(t *testing.T) {
	base := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	busy := l.Addr().(*net.TCPAddr).Port

	cfgOf := useGenerate(t, filepath.Join(base, "none"), nil)
	// 管理 API のポートを使用中にするため、生成後の config を書き換える
	inner := generate
	generate = func(path string) (*config.Config, error) {
		if _, err := inner(path); err != nil {
			return nil, err
		}
		raw, _ := os.ReadFile(path)
		s := regexp.MustCompile(`listen: "127.0.0.1:\d+"`).ReplaceAllString(string(raw), fmt.Sprintf(`listen: "127.0.0.1:%d"`, busy))
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			return nil, err
		}
		return config.Load(path)
	}
	_ = cfgOf

	var stdout, stderr syncBuffer
	done := runInBackground(context.Background(), base, &stdout, &stderr)
	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("終了コード = %d, want 1\nstderr: %s", code, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run が終わらない")
	}
	if !strings.Contains(stderr.String(), "admin.listen") {
		t.Errorf("標準エラーに admin.listen がない:\n%s", stderr.String())
	}
	if got := readLogs(t, filepath.Join(base, "logs")); !strings.Contains(got, "admin.listen") {
		t.Errorf("ログファイルに admin.listen がない:\n%s", got)
	}
}
