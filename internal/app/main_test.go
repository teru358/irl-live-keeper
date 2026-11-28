package app

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
		s = strings.Replace(s, "admin:\n  port: 8787", fmt.Sprintf("admin:\n  port: %d", pick(freeTCPPort)), 1)
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
		// 管理ページのパスワードは初回の生成では空（S2-61）なので、送信と読み取りのパスワードで確かめる
		script := fmt.Sprintf("#!/bin/sh\necho 'publish pass=%s'\necho 'read pass=%s'\nexec sleep 30\n",
			cfg.Cameras[0].PublishPass, cfg.Stream.ReadPass)
		return cfg, os.WriteFile(bin, []byte(script), 0o700)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runInBackground(ctx, base, &stdout, &stderr)

	logDir := filepath.Join(base, "logs")
	// 偽の MediaMTX の出力が、置き換えられてログに載るまで待つ
	waitFor(t, "偽の MediaMTX の出力がログに載る", func() bool {
		logs := readLogs2(logDir)
		return strings.Contains(logs, "publish pass=***") && strings.Contains(logs, "read pass=***")
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
	// 管理ページのパスワードは初回の生成では空（S2-61。管理ページか config.yaml で決める）
	if cfg.Admin.Password != "" {
		t.Fatal("初回の生成で admin.password が空でない")
	}
	// 標準出力には、URL と生成した送信・読み取りのパスワードが出る
	for _, want := range []string{cfg.Cameras[0].PublishPass, cfg.Stream.ReadPass, "srt://", "rtmp://", "rtsp://"} {
		if !strings.Contains(out, want) {
			t.Errorf("標準出力に %q がない", want)
		}
	}
	// ログファイルには、秘密の文字列が1つも出ない
	logs := readLogs(t, logDir)
	if logs == "" {
		t.Fatal("ログファイルが空")
	}
	// 止める操作を受けた時刻と、止まり終えた時刻がログでわかる（Windows の確認で、Ctrl+C のときに1行も出なかった）
	for _, want := range []string{`msg="Shutdown requested"`, `msg="shutdown complete" code=0`} {
		if !strings.Contains(logs, want) {
			t.Errorf("ログに %s がない", want)
		}
	}
	// ログは英語だけ（spec v0.11 第14節）。MediaMTX の行はそのまま流すので除く
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "component=mediamtx") {
			continue
		}
		if regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`).MatchString(line) {
			t.Errorf("ログに日本語の行がある: %s", line)
		}
	}
	for name, secret := range map[string]string{
		"publish_pass": cfg.Cameras[0].PublishPass, "read_pass": cfg.Stream.ReadPass,
	} {
		if strings.Contains(logs, secret) {
			t.Errorf("ログファイルに %s が出ている", name)
		}
	}
	// 標準出力のうちログの行（起動の表示以外）にも出ない。偽の MediaMTX の行で確かめる
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "component=mediamtx") {
			for _, secret := range []string{cfg.Cameras[0].PublishPass, cfg.Stream.ReadPass} {
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
		if strings.Count(s, `password: ""`) != 2 {
			return nil, fmt.Errorf("config.yaml の空の password が1つではない")
		}
		s = strings.Replace(s, `password: ""               # 空なら`, fmt.Sprintf("password: %q               # 空なら", obsPass), 1)
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
	bad := "admin:\n  password: short\nswitch:\n  low_sec: 0\n  recover_kbps: 100\n"
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
	if !strings.Contains(stderr.String(), "rtmp_port") || !strings.Contains(stderr.String(), "ほかのプログラム") {
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
		s := regexp.MustCompile(`(?m)^admin:\n  port: \d+`).ReplaceAllString(string(raw), fmt.Sprintf("admin:\n  port: %d", busy))
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
	if !strings.Contains(stderr.String(), "admin.port") {
		t.Errorf("標準エラーに admin.port がない:\n%s", stderr.String())
	}
	got := readLogs(t, filepath.Join(base, "logs"))
	if !strings.Contains(got, "admin.port") {
		t.Errorf("ログファイルに admin.port がない:\n%s", got)
	}
	// 終わりのログの code は、実際の終了コードと同じ（S2-5 のレビュー）
	if strings.Contains(got, "code=0") || !strings.Contains(got, "code=1") {
		t.Errorf("終わりのログの code が 1 でない:\n%s", got)
	}
}

// ---- 0b: hidden.json と Stopping ----

func TestHiddenPathはbaseのstate下(t *testing.T) {
	got := hiddenPath(filepath.Join("a", "b"))
	if want := filepath.Join("a", "b", "state", "hidden.json"); got != want {
		t.Errorf("hiddenPath = %q, want %q", got, want)
	}
}

// 終わる道では、HTTP サーバーと MediaMTX を止める前に core に Stopping を渡す。待ちは2秒を超えない。
type orderRecorder struct {
	mu     sync.Mutex
	events []string
	dl     time.Duration
}

func (r *orderRecorder) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, s)
}

func (r *orderRecorder) Stopping(ctx context.Context) {
	if d, ok := ctx.Deadline(); ok {
		r.dl = time.Until(d)
	}
	r.add("stopping")
}

func (r *orderRecorder) Shutdown(context.Context) error { r.add("http shutdown"); return nil }
func (r *orderRecorder) Close() error                   { r.add("http close"); return nil }

func TestShutdownはHTTPとMediaMTXより前にStoppingを渡す(t *testing.T) {
	r := &orderRecorder{}
	var wg sync.WaitGroup
	wg.Go(func() { time.Sleep(10 * time.Millisecond); r.add("parts stopped") })
	shutdown(r, r, func() { r.add("cancel") }, &wg)
	want := []string{"stopping", "http shutdown", "cancel", "parts stopped"}
	if fmt.Sprint(r.events) != fmt.Sprint(want) {
		t.Errorf("順 = %v, want %v", r.events, want)
	}
	if r.dl <= 0 || r.dl > 2*time.Second {
		t.Errorf("Stopping の待ちの上限 = %v, want 2秒以内", r.dl)
	}
}

// 壊れた hidden.json は、警告を出して空として始まり、hidden.json.broken に残る。
func TestRunは壊れたhidden_jsonを警告して残す(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("シェルスクリプトの偽の MediaMTX を使うため")
	}
	base := t.TempDir()
	bin := filepath.Join(base, "fake-mediamtx.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	useGenerate(t, bin, nil)
	if err := os.MkdirAll(filepath.Join(base, "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hiddenPath(base), []byte("{壊れた"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runInBackground(ctx, base, &stdout, &stderr)
	logDir := filepath.Join(base, "logs")
	waitFor(t, "警告がログに載る", func() bool { return strings.Contains(readLogs2(logDir), "Cannot read hidden.json") })
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("終了コード = %d\nstderr: %s", code, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run が終わらない")
	}
	if b, err := os.ReadFile(hiddenPath(base) + ".broken"); err != nil || string(b) != "{壊れた" {
		t.Errorf(".broken = %q, %v", b, err)
	}
	if _, err := os.Stat(hiddenPath(base)); err == nil {
		t.Errorf("元の hidden.json が残っている（空で始めたのだから、読み直されないよう名前を替えるはず）")
	}
}

// 部品の context はシグナルの ctx から独立している: 親を取り消しても、Stopping と HTTP の停止の前に
// 部品へ取り消しが伝わらない（RestoreAll を送れるように）。部品を止めるのは cancel だけ。
func TestPartsContextはシグナルの取り消しで止まらない(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	parts, cancel := partsContext(parent)
	defer cancel()
	stop()
	time.Sleep(20 * time.Millisecond)
	if parts.Err() != nil {
		t.Fatalf("親の取り消しが部品の context に伝わった: %v", parts.Err())
	}
	cancel()
	if parts.Err() == nil {
		t.Fatal("cancel しても部品の context が止まらない")
	}
}

// 終わる道全体は、Windows のコンソールを閉じるときの猶予（数秒）に収まる。Stopping も HTTP の停止も止まったままでも。
func TestShutdown全体は数秒に収まる(t *testing.T) {
	hang := &hangRecorder{}
	var wg sync.WaitGroup
	start := time.Now()
	shutdown(hang, hang, func() {}, &wg)
	if d := time.Since(start); d > 3500*time.Millisecond {
		t.Fatalf("終わる道が %v かかった（Stopping 2秒 + HTTP 1秒 + 余裕のはず）", d)
	}
}

type hangRecorder struct{}

func (hangRecorder) Stopping(ctx context.Context)       { <-ctx.Done() }
func (hangRecorder) Shutdown(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
func (hangRecorder) Close() error                       { return nil }
