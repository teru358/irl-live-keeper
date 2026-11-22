package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestMain は、環境変数 FAKE_MEDIAMTX があるときだけ偽の mediamtx として振る舞う
// （テストのバイナリが自分自身を子プロセスとして起動する、ヘルパープロセスの手法）。
func TestMain(m *testing.M) {
	if mode := os.Getenv("FAKE_MEDIAMTX"); mode != "" {
		fakeMediaMTX(mode)
		return
	}
	os.Exit(m.Run())
}

func fakeMediaMTX(mode string) {
	switch mode {
	case "exit": // 何か出して、すぐ異常終了する
		fmt.Println("INF starting")
		os.Exit(3)
	case "dump": // 渡された設定ファイルの中身を、標準出力と標準エラーに出して終了する
		b, _ := os.ReadFile(os.Args[1])
		for i, l := range strings.Split(string(b), "\n") {
			if i%2 == 0 {
				fmt.Println("INF " + l)
			} else {
				fmt.Fprintln(os.Stderr, "INF "+l)
			}
		}
		os.Exit(0)
	case "infkill": // INF の行だけを出して、自分で SIGKILL される（kill -9 された場合の再現）
		fmt.Println("INF started with listener on 127.0.0.1:19997 (TCP/HTTP)")
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			_ = p.Kill()
		}
		time.Sleep(time.Minute)
	case "bindfail":
		fmt.Println("INF MediaMTX v1.21.4")
		fmt.Println("ERR listen tcp :1935: bind: address already in use")
		fmt.Println("INF shutting down")
		os.Exit(1)
	case "sleep": // 終了の要求（SIGTERM）で止まる
		fmt.Println("ready pid=" + strconv.Itoa(os.Getpid()))
		time.Sleep(time.Minute)
		os.Exit(0)
	case "ignoreterm": // 終了の要求を無視する（kill でしか止まらない）
		signal.Ignore(syscall.SIGTERM)
		fmt.Println("ready pid=" + strconv.Itoa(os.Getpid()))
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(99)
}

// syncBuf はログの出力先。複数の goroutine から書かれる。
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newSup(t *testing.T, mode string) (*Supervisor, *syncBuf) {
	t.Helper()
	t.Setenv("FAKE_MEDIAMTX", mode)
	buf := &syncBuf{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return New(freePorts(t), os.Args[0], t.TempDir(), log), buf
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	s, _ := newSup(t, "exit")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got []time.Duration
	s.sleep = func(_ context.Context, d time.Duration) error {
		got = append(got, d)
		if len(got) == 8 {
			cancel()
			return context.Canceled
		}
		return nil
	}
	s.Run(ctx)
	sec := time.Second
	want := []time.Duration{1 * sec, 2 * sec, 4 * sec, 8 * sec, 16 * sec, 30 * sec, 30 * sec, 30 * sec}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("待ち時間 = %v, want %v", got, want)
	}
}

func TestBackoffResetsAfterStableRun(t *testing.T) {
	s, _ := newSup(t, "exit")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 起動の時刻と終了の時刻を交互に返し、1回ごとの動作時間を決める。
	runs := []time.Duration{1 * time.Second, 1 * time.Second, 61 * time.Second, 1 * time.Second}
	var n int
	base := time.Unix(1000, 0)
	s.now = func() time.Time {
		i := n / 2
		n++
		if n%2 == 1 {
			return base
		}
		return base.Add(runs[i])
	}
	var got []time.Duration
	s.sleep = func(_ context.Context, d time.Duration) error {
		got = append(got, d)
		if len(got) == len(runs) {
			cancel()
			return context.Canceled
		}
		return nil
	}
	s.Run(ctx)
	sec := time.Second
	want := []time.Duration{1 * sec, 2 * sec, 1 * sec, 2 * sec}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("待ち時間 = %v, want %v", got, want)
	}
}

func TestLogForwardingMasksSecrets(t *testing.T) {
	s, buf := newSup(t, "dump")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	s.Run(ctx)
	_, apiPass := s.APICredentials()
	out := buf.String()
	if !strings.Contains(out, "component=mediamtx") {
		t.Errorf("component がありません:\n%s", out)
	}
	if !strings.Contains(out, "***") {
		t.Errorf("置き換えが行われていません:\n%s", out)
	}
	for _, secret := range []string{"pubPASS123", "readPASS123", apiPass} {
		if strings.Contains(out, secret) {
			t.Errorf("ログに %q が出ています:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "authInternalUsers") {
		t.Errorf("標準出力か標準エラーの行が流れていません:\n%s", out)
	}
}

// MediaMTX の行の中で途中で重なる2つの秘密（publish_pass と read_pass）は、どちらの断片も残さない。
// 行（slog の msg）と、最後の ERR の行（status の last_error）の両方を確かめる。
func TestLineWriterMasksOverlappingSecrets(t *testing.T) {
	const pub, read = "abcdefgh", "fghijklm" // 8文字以上。途中の fgh が重なる
	buf := &syncBuf{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	lw := newLineWriter(log, pub, read)

	// 重なった文字列が1行に出る。ERR の行は last_error に使われる
	fmt.Fprint(lw, "INF conn pass=abcdefghijklm\n")
	fmt.Fprint(lw, "ERR auth failed for abcdefghijklm\n")

	out := buf.String()
	if !strings.Contains(out, `msg="INF conn pass=***"`) {
		t.Errorf("行が置き換わっていない:\n%s", out)
	}
	if got, want := lw.lastErrorLine(), "ERR auth failed for ***"; got != want {
		t.Errorf("lastErrorLine = %q, want %q", got, want)
	}
	// どちらの秘密の3文字以上の断片も、出力に残らない
	union := "abcdefghijklm"
	for i := 0; i+3 <= len(union); i++ {
		frag := union[i : i+3]
		if strings.Contains(out, frag) || strings.Contains(lw.lastErrorLine(), frag) {
			t.Errorf("断片 %q が残った:\n出力: %s\nlast_error: %s", frag, out, lw.lastErrorLine())
		}
	}
}

func TestEarlyExitSetsLastError(t *testing.T) {
	s, _ := newSup(t, "bindfail")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	s.Run(ctx)
	st := s.Status()
	if !strings.Contains(st.LastError, "address already in use") {
		t.Errorf("LastError = %q", st.LastError)
	}
	if st.State != StateDown {
		t.Errorf("State = %q", st.State)
	}
}

// ERR の行がないまま起動直後に落ちたときは、INF の行ではなく終了の文を last_error に出す。
func TestEarlyExitWithoutERRSetsExitMessage(t *testing.T) {
	s, _ := newSup(t, "infkill")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	s.Run(ctx)
	st := s.Status()
	if !strings.Contains(st.LastError, "mediamtx が終了しました") {
		t.Errorf("LastError = %q, want 終了の文", st.LastError)
	}
	if strings.Contains(st.LastError, "INF") {
		t.Errorf("LastError に INF の行が出ています: %q", st.LastError)
	}
	if st.State != StateDown {
		t.Errorf("State = %q", st.State)
	}
}

func TestPortInUseStopsStart(t *testing.T) {
	s, _ := newSup(t, "sleep")
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.MediaMTX.RTSPPort))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	s.Run(ctx)
	st := s.Status()
	if !strings.Contains(st.LastError, "rtsp_port") || st.State != StateDown {
		t.Errorf("Status = %+v", st)
	}
}

func TestAPICredentialsSameAcrossRestarts(t *testing.T) {
	s, _ := newSup(t, "exit")
	re := regexp.MustCompile(`^[A-Za-z0-9]+$`)
	// Poller は作成時に資格情報を受け取るので、Run の前から決まっている必要がある。
	u0, p0 := s.APICredentials()
	if !re.MatchString(u0) || !re.MatchString(p0) || len(p0) < 20 {
		t.Fatalf("資格情報の形が不正: %q %q", u0, p0)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	starts := 0
	s.sleep = func(context.Context, time.Duration) error {
		starts++
		if u, p := s.APICredentials(); u != u0 || p != p0 {
			t.Errorf("%d 回目の起動のあとで資格情報が変わった: %q %q", starts, u, p)
		}
		// MediaMTX に渡した設定にも、同じ資格情報が入っている。
		yml, err := os.ReadFile(filepath.Join(s.workDir, ymlName))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(yml), u0) || !strings.Contains(string(yml), p0) {
			t.Errorf("%d 回目の mediamtx.yml に資格情報がない:\n%s", starts, yml)
		}
		if starts == 3 {
			cancel()
			return context.Canceled
		}
		return nil
	}
	s.Run(ctx)
	if starts != 3 {
		t.Errorf("起動の回数 = %d, want 3", starts)
	}
	// 別の Supervisor は別の資格情報を持つ。
	other, _ := newSup(t, "exit")
	if _, p := other.APICredentials(); p == p0 {
		t.Errorf("別の Supervisor で同じパスワード: %q", p)
	}
}

// waitRunning は ready の行が出るまで待ち、子の pid を返す。
func waitReady(t *testing.T, s *Supervisor, buf *syncBuf) int {
	t.Helper()
	re := regexp.MustCompile(`ready pid=(\d+)`)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if m := re.FindStringSubmatch(buf.String()); m != nil && s.Status().State == StateRunning {
			pid, _ := strconv.Atoi(m[1])
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("子が起動しません:\n%s", buf.String())
	return 0
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func testStop(t *testing.T, mode string, grace time.Duration, limit time.Duration) {
	if runtime.GOOS == "windows" {
		t.Skip("シグナルの扱いが異なる")
	}
	s, buf := newSup(t, mode)
	s.grace = grace
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	pid := waitReady(t, s, buf)
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatal("Run が戻りません")
	}
	t.Logf("停止まで %v", time.Since(start))
	if alive(pid) {
		t.Errorf("Run が戻ったのに子 %d が残っています", pid)
	}
	if s.Status().State != StateDown {
		t.Errorf("State = %q", s.Status().State)
	}
}

func TestStopTerminatesChild(t *testing.T) { testStop(t, "sleep", 5*time.Second, 4*time.Second) }

func TestStopKillsAfterGrace(t *testing.T) {
	testStop(t, "ignoreterm", 300*time.Millisecond, 4*time.Second)
}

// 実物の MediaMTX で起動・API 応答・停止を確かめる。MEDIAMTX_BIN があるときだけ。
func TestRealMediaMTX(t *testing.T) {
	bin := os.Getenv("MEDIAMTX_BIN")
	if bin == "" {
		t.Skip("MEDIAMTX_BIN が未設定")
	}
	buf := &syncBuf{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	s := New(freePorts(t), bin, t.TempDir(), log)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	url := fmt.Sprintf("http://127.0.0.1:%d/v3/paths/list", s.cfg.MediaMTX.APIPort)
	ok := false
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline) && !ok; time.Sleep(200 * time.Millisecond) {
		u, p := s.APICredentials()
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.SetBasicAuth(u, p)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			ok = resp.StatusCode == 200
			resp.Body.Close()
		}
	}
	if !ok {
		t.Errorf("API が応答しません。status=%+v\n%s", s.Status(), buf.String())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run が戻りません")
	}
}

// 子が本体より一瞬早く終わり（Windows でコンソールの Ctrl+C や窓を閉じる操作は、同じコンソールの子にも同時に届く）、
// そのすぐあとに本体の終了（ctx）が来たときは、思いがけない終了として WARN を出さない（M-13 の Windows の確認）。
func TestExitJustBeforeShutdownIsNotWarned(t *testing.T) {
	s, buf := newSup(t, "exit")
	s.exitSettle = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// 子はすぐ終わる。本体の終了はそのあとに来る
		for s.Status().State != StateRunning {
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run が戻らない")
	}
	if strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("本体の終了の直前の子の終了で WARN が出た:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "MediaMTX が終了しました（本体の終了と同時）") {
		t.Errorf("終了の INFO の行がない:\n%s", buf.String())
	}
}

// 本体が終わらないまま子が終わったら、待ったあとで今までどおり WARN を出して起動し直す。
func TestUnexpectedExitStillWarnsAfterSettle(t *testing.T) {
	s, buf := newSup(t, "exit")
	s.exitSettle = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	s.Run(ctx)
	if !strings.Contains(buf.String(), `level=WARN msg="MediaMTX が終了しました"`) {
		t.Errorf("思いがけない終了の WARN がない:\n%s", buf.String())
	}
}
