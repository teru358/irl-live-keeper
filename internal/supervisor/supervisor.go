package supervisor

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/redact"
)

// MediaMTX の状態。status の mediamtx に使う。
const (
	StateRunning  = "running"
	StateDown     = "down"
	StateStarting = "starting"
)

const (
	minBackoff   = 1 * time.Second
	maxBackoff   = 30 * time.Second
	stableAfter  = 60 * time.Second // これだけ動けば待ち時間を戻す
	earlyExit    = 5 * time.Second  // これより早く落ちたら起動失敗として扱う
	defaultGrace = 5 * time.Second  // 終了を求めてから kill するまで
	// defaultExitSettle は、子が思いがけず終わったときに、本体の終了が続いて来るかを待つ長さ。
	// Windows でコンソールの Ctrl+C や窓を閉じる操作は、同じコンソールの子にも同時に届き、子が本体より一瞬早く終わる。
	defaultExitSettle = time.Second
	ymlName           = "mediamtx.yml"
)

// Status は MediaMTX の現在の状態。
type Status struct {
	State     string
	LastError string
}

// Supervisor は MediaMTX の子プロセスを起動し、落ちたら再起動する。
type Supervisor struct {
	cfg     *config.Config
	binPath string
	workDir string
	log     *slog.Logger

	// テストで差し替える。
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
	grace time.Duration
	// exitSettle は defaultExitSettle（テストで差し替える）。
	exitSettle time.Duration

	// api の資格情報。New で1回だけ作り、プロセスが終わるまで変えない
	// （Poller は作成時に受け取った資格情報で API を呼び続けるため）。
	apiUser string
	apiPass string
	credErr error // 作れなかったときの理由。起動のたびにこれで失敗する

	mu     sync.Mutex
	status Status
}

// New は Supervisor を作る。binPath は mediamtx の実行ファイル、workDir は mediamtx.yml を置く場所。
func New(cfg *config.Config, binPath, workDir string, log *slog.Logger) *Supervisor {
	if log == nil {
		log = slog.Default()
	}
	s := &Supervisor{
		cfg: cfg, binPath: binPath, workDir: workDir, log: log,
		now:        time.Now,
		sleep:      sleepCtx,
		grace:      defaultGrace,
		exitSettle: defaultExitSettle,
		status:     Status{State: StateDown},
	}
	s.apiUser, s.apiPass, s.credErr = newCredentials()
	return s
}

// newCredentials は api のユーザー名とパスワードをランダムに作る。
func newCredentials() (user, pass string, err error) {
	if user, err = randomString(12); err != nil {
		return "", "", fmt.Errorf("api の資格情報を作れません: %w", err)
	}
	if pass, err = randomString(24); err != nil {
		return "", "", fmt.Errorf("api の資格情報を作れません: %w", err)
	}
	return user, pass, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Status は現在の状態を返す。
func (s *Supervisor) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// APICredentials は API の資格情報を返す。New で決まり、MediaMTX を再起動しても変わらない。
func (s *Supervisor) APICredentials() (user, pass string) {
	return s.apiUser, s.apiPass
}

func (s *Supervisor) setStatus(state, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.State = state
	if lastErr != "" {
		s.status.LastError = lastErr
	}
}

const credChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randomString(n int) (string, error) {
	max := big.NewInt(int64(len(credChars)))
	b := make([]byte, n)
	for i := range b {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = credChars[v.Int64()]
	}
	return string(b), nil
}

// Run は ctx が終わるまで MediaMTX を起動し続ける。戻る前に子プロセスを止める。
func (s *Supervisor) Run(ctx context.Context) {
	backoff := minBackoff
	for ctx.Err() == nil {
		ran, err := s.runOnce(ctx)
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			s.log.Warn("MediaMTX を起動できません", "error", err)
		}
		if ran >= stableAfter {
			backoff = minBackoff
		}
		s.setStatus(StateDown, "")
		if s.sleep(ctx, backoff) != nil {
			break
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	s.setStatus(StateDown, "")
}

// runOnce は MediaMTX を1回起動して終了まで待つ。動いていた時間を返す。
func (s *Supervisor) runOnce(ctx context.Context) (time.Duration, error) {
	s.setStatus(StateStarting, "")
	fail := func(err error) (time.Duration, error) {
		s.setStatus(StateDown, err.Error())
		return 0, err
	}
	if err := CheckPorts(s.cfg); err != nil {
		return fail(err)
	}
	if s.credErr != nil {
		return fail(s.credErr)
	}
	pass := s.apiPass
	yml, err := GenerateYAML(s.cfg, s.apiUser, pass)
	if err != nil {
		return fail(err)
	}
	ymlPath := filepath.Join(s.workDir, ymlName)
	if err := os.WriteFile(ymlPath, yml, 0o600); err != nil {
		return fail(fmt.Errorf("mediamtx.yml を書き込めません: %w", err))
	}
	lw := newLineWriter(s.log.With("component", "mediamtx"), s.cfg.Stream.PublishPass, s.cfg.Stream.ReadPass, pass)
	cmd := exec.Command(s.binPath, ymlPath)
	cmd.Dir = s.workDir
	cmd.Stdout = lw
	cmd.Stderr = lw

	start := s.now()
	c, err := spawn(cmd)
	if err != nil {
		return fail(fmt.Errorf("mediamtx を起動できません: %w", err))
	}
	s.setStatus(StateRunning, "")

	stopped := false
	select {
	case <-c.done:
	case <-ctx.Done():
		s.stop(c)
		stopped = true
	}
	lw.flush()
	ran := s.now().Sub(start)
	if !stopped && ctx.Err() == nil && s.exitSettle > 0 {
		// 子が先に終わった。本体の終了が少し遅れて来ることがあるので、少しの間待つ。
		t := time.NewTimer(s.exitSettle)
		select {
		case <-ctx.Done():
		case <-t.C:
		}
		t.Stop()
	}
	if ctx.Err() != nil {
		if !stopped {
			s.log.Info("MediaMTX が終了しました（本体の終了と同時）", "after", ran.String(), "error", c.err)
		}
		return ran, nil
	}

	msg := fmt.Sprintf("mediamtx が終了しました: %v", c.err)
	if c.err == nil {
		msg = "mediamtx が終了しました"
	}
	if ran < earlyExit {
		if l := lw.lastErrorLine(); l != "" {
			msg = l
		}
	}
	s.setStatus(StateDown, msg)
	s.log.Warn("MediaMTX が終了しました", "after", ran.String(), "error", c.err)
	return ran, nil
}

// stop は子に終了を求め、grace を過ぎたら kill して、終了を待つ。
func (s *Supervisor) stop(c *child) {
	if err := interrupt(c.cmd.Process); err != nil {
		_ = c.cmd.Process.Kill()
	}
	t := time.NewTimer(s.grace)
	defer t.Stop()
	select {
	case <-c.done:
	case <-t.C:
		_ = c.cmd.Process.Kill()
		<-c.done
	}
}

// child は起動した子プロセス。done は終了（Wait が返った）で閉じる。
type child struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error // done が閉じたあとに読む
}

// spawn は子を起動する。起動から Wait までを1本の goroutine で行う。
// lockThread のときは、子が生きている間その goroutine を OS スレッドに固定したままにする。
func spawn(cmd *exec.Cmd) (*child, error) {
	c := &child{cmd: cmd, done: make(chan struct{})}
	started := make(chan error, 1)
	go func() {
		if lockThread {
			// 解除しない。子が終わって goroutine が戻ると、スレッドごと破棄される。
			runtime.LockOSThread()
		}
		prepare(cmd)
		if err := cmd.Start(); err != nil {
			started <- err
			return
		}
		afterStart(cmd)
		started <- nil
		c.err = cmd.Wait()
		close(c.done)
	}()
	if err := <-started; err != nil {
		return nil, err
	}
	return c, nil
}

// lineWriter は MediaMTX の出力を行ごとに slog へ流す。秘密の文字列は *** に置き換える。
type lineWriter struct {
	log    *slog.Logger
	masker *redact.Masker

	now func() time.Time // テストで差し替える。nil なら time.Now

	mu      sync.Mutex
	buf     bytes.Buffer
	lastErr string
	held    []heldLine // 書くのを待っている OBS の読み直しの opened の行（古い順）
}

// heldLine は、読み直しかもしれないので、すぐには書かずに持っている opened の行。
type heldLine struct {
	addr string // 接続元（127.0.0.1:<port>）
	line string // 秘密の置き換えのあと
	at   time.Time
}

// holdFor を過ぎても次が来なければ、持っている行は書く（次に何か行が来たときに書き出す）。
const holdFor = 2 * time.Second

const noStreamClosed = "closed: no stream is available on path"

var (
	// MediaMTX の行は「日時 水準 本文」。日時がなければ先頭の語を水準とみなす。
	levelRe = regexp.MustCompile(`^(?:\d{4}/\d\d/\d\d \d\d:\d\d:\d\d )?([A-Z]{3}) (.*)$`)
	// OBS のメディアソースの読み直しで出る、loopback からの接続の opened。
	openedRe = regexp.MustCompile(`^\[RTSP\] \[conn (127\.0\.0\.1:\d+)\] opened$`)
	// 読み出しの session の作成。接続元を持つ。
	createdRe = regexp.MustCompile(`^\[RTSP\] \[session \w+\] created by (127\.0\.0\.1:\d+)`)
)

var _ io.Writer = (*lineWriter)(nil)

func newLineWriter(log *slog.Logger, secrets ...string) *lineWriter {
	return &lineWriter{log: log, masker: redact.New(secrets...)}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		line := string(w.buf.Next(i + 1))
		w.emit(line)
	}
	return len(p), nil
}

// flush は改行で終わらなかった残りを出す。
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf.Len() > 0 {
		w.emit(w.buf.String())
		w.buf.Reset()
	}
	for _, h := range w.held {
		w.write(h.line)
	}
	w.held = nil
}

func (w *lineWriter) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

// parseLine は行を水準の語と本文に分ける。水準がなければ水準は空。
func parseLine(line string) (level, rest string) {
	if m := levelRe.FindStringSubmatch(line); m != nil {
		return m[1], m[2]
	}
	return "", line
}

func (w *lineWriter) emit(line string) {
	line = w.masker.Mask(strings.TrimRight(line, "\r\n"))
	if line == "" {
		return
	}
	now := w.clock()
	// 古くなった行は先に書く。
	for len(w.held) > 0 && now.Sub(w.held[0].at) >= holdFor {
		w.write(w.held[0].line)
		w.held = w.held[1:]
	}
	_, rest := parseLine(line)
	if m := openedRe.FindStringSubmatch(rest); m != nil {
		w.release(m[1]) // 同じ接続の古い行が残っていれば先に書く
		w.held = append(w.held, heldLine{addr: m[1], line: line, at: now})
		return
	}
	for _, h := range w.held {
		prefix := "[RTSP] [conn " + h.addr + "] "
		if strings.HasPrefix(rest, prefix) {
			if strings.HasPrefix(rest[len(prefix):], noStreamClosed) {
				w.drop(h.addr) // 読み直し。opened とこの行を書かない
				return
			}
			w.release(h.addr)
			break
		}
		if m := createdRe.FindStringSubmatch(rest); m != nil && m[1] == h.addr {
			w.release(h.addr) // 読み出しが始まった。opened を先に書く
			break
		}
	}
	w.write(line)
}

// release は addr の持っている行を書き、持つのをやめる。
func (w *lineWriter) release(addr string) {
	for i, h := range w.held {
		if h.addr == addr {
			w.write(h.line)
			w.held = append(w.held[:i], w.held[i+1:]...)
			return
		}
	}
}

// drop は addr の持っている行を書かずに捨てる。
func (w *lineWriter) drop(addr string) {
	for i, h := range w.held {
		if h.addr == addr {
			w.held = append(w.held[:i], w.held[i+1:]...)
			return
		}
	}
}

// write は水準（ERR は Error、WAR は Warn、それ以外は Info）に合わせて書く。
// ERR の行は last_error に使う。
func (w *lineWriter) write(line string) {
	level, _ := parseLine(line)
	switch level {
	case "ERR":
		w.lastErr = line
		w.log.Error(line)
	case "WAR":
		w.log.Warn(line)
	default:
		w.log.Info(line)
	}
}

// lastErrorLine は最後の ERR の行を返す。ERR の行がなければ空文字列を返す
// （INF の行は起動の失敗の理由にならないので使わない）。
func (w *lineWriter) lastErrorLine() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastErr
}
