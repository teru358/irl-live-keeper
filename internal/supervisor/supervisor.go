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
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
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
	ymlName      = "mediamtx.yml"
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

	mu      sync.Mutex
	status  Status
	apiUser string
	apiPass string
}

// New は Supervisor を作る。binPath は mediamtx の実行ファイル、workDir は mediamtx.yml を置く場所。
func New(cfg *config.Config, binPath, workDir string, log *slog.Logger) *Supervisor {
	if log == nil {
		log = slog.Default()
	}
	return &Supervisor{
		cfg: cfg, binPath: binPath, workDir: workDir, log: log,
		now:    time.Now,
		sleep:  sleepCtx,
		grace:  defaultGrace,
		status: Status{State: StateDown},
	}
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

// APICredentials は現在の API の資格情報を返す。起動のたびに変わる。
func (s *Supervisor) APICredentials() (user, pass string) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	user, err := randomString(12)
	if err != nil {
		return fail(err)
	}
	pass, err := randomString(24)
	if err != nil {
		return fail(err)
	}
	yml, err := GenerateYAML(s.cfg, user, pass)
	if err != nil {
		return fail(err)
	}
	ymlPath := filepath.Join(s.workDir, ymlName)
	if err := os.WriteFile(ymlPath, yml, 0o600); err != nil {
		return fail(fmt.Errorf("mediamtx.yml を書き込めません: %w", err))
	}
	s.mu.Lock()
	s.apiUser, s.apiPass = user, pass
	s.mu.Unlock()

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

	select {
	case <-c.done:
	case <-ctx.Done():
		s.stop(c)
	}
	lw.flush()
	ran := s.now().Sub(start)
	if ctx.Err() != nil {
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
	log      *slog.Logger
	replacer *strings.Replacer

	mu      sync.Mutex
	buf     bytes.Buffer
	lastErr string
	last    string
}

var _ io.Writer = (*lineWriter)(nil)

func newLineWriter(log *slog.Logger, secrets ...string) *lineWriter {
	var pairs []string
	for _, s := range secrets {
		if s != "" {
			pairs = append(pairs, s, "***")
		}
	}
	return &lineWriter{log: log, replacer: strings.NewReplacer(pairs...)}
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
}

func (w *lineWriter) emit(line string) {
	line = w.replacer.Replace(strings.TrimRight(line, "\r\n"))
	if line == "" {
		return
	}
	w.last = line
	if strings.Contains(line, "ERR") {
		w.lastErr = line
	}
	w.log.Info(line)
}

// lastErrorLine は最後の ERR の行を、なければ最後の行を返す。
func (w *lineWriter) lastErrorLine() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastErr != "" {
		return w.lastErr
	}
	return w.last
}
