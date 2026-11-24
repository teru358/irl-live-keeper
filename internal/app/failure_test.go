package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"github.com/teru358/irl-live-keeper/internal/obs"
)

// failureFixture は実ソケットや子プロセスを使わず、Run の開始と回収を記録する。
type failureFixture struct {
	a           *App
	r           *generationRecord
	parts       []*generationPart
	out, stderr strings.Builder
	h           *failureHTTP
	stop        func(context.Context)
}

type failureHTTP struct {
	generationHTTP
	serveError error
	shutdown   func(context.Context) error
}

func (h *failureHTTP) Serve(net.Listener) error {
	close(h.started)
	<-h.done
	h.r.add("http.return")
	return h.serveError
}
func (h *failureHTTP) Shutdown(ctx context.Context) error {
	h.r.add("http.shutdown.call")
	if h.shutdown != nil {
		return h.shutdown(ctx)
	}
	close(h.done)
	return nil
}
func (h *failureHTTP) Close() error {
	h.r.add("http.close")
	// Serve が先に終了している場合も Close は呼べる。
	select {
	case <-h.done:
	default:
		close(h.done)
	}
	return nil
}

func newFailureFixture(t *testing.T) *failureFixture {
	t.Helper()
	f := &failureFixture{r: &generationRecord{}}
	base := t.TempDir()
	cfg, _, raw, err := loadConfigBytes(base)
	cfg.Admin.Password = "existing-password"
	raw = []byte(strings.Replace(string(raw), `password: ""`, `password: "existing-password"`, 1))
	if e := os.WriteFile(filepath.Join(base, configName), raw, 0600); e != nil {
		t.Fatal(e)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"sup", "poll", "obs", "core"} {
		f.parts = append(f.parts, &generationPart{n, f.r, make(chan context.Context, 1)})
	}
	f.h = &failureHTTP{generationHTTP: generationHTTP{make(chan struct{}), make(chan struct{}), f.r}, serveError: http.ErrServerClosed}
	logger := &generationLog{log: slog.New(slog.NewTextHandler(io.Discard, nil)), r: f.r}
	f.a = newApp(Options{BaseDir: base, Stdout: &f.out, Stderr: &f.stderr}, appDeps{
		initJob: func() error { return nil }, stage: f.r.add,
		loadConfig:    func(string) (*config.Config, bool, []byte, error) { return cfg, false, raw, nil },
		openLogger:    func(logging.Options) (logHandle, error) { return logger, nil },
		checkPorts:    func(*config.Config) error { return nil },
		newSupervisor: func(*config.Config, string, string, *slog.Logger) supervisorRunner { return &generationSup{f.parts[0]} },
		newPoller:     func(string, string, string, []string, []string) pollRunner { return &generationPoll{f.parts[1]} },
		newOBS:        func(obs.Config, *slog.Logger) obsRunner { return &generationOBS{generationPart: f.parts[2]} },
		newMediaWatch: func(obsRunner, *slog.Logger) mediaWatcher { return &generationWatch{r: f.r} },
		newCore: func(core.Deps) coreRunner {
			return &generationCore{generationPart: f.parts[3], stopping: func(ctx context.Context) {
				if f.stop != nil {
					f.stop(ctx)
				}
			}}
		},
		listen:  func(string, string) (net.Listener, error) { return nil, nil },
		newHTTP: func(coreRunner, string, i18n.Lang) servingHTTP { return f.h },
	})
	return f
}

func (f *failureFixture) stages() []string {
	var result []string
	for _, s := range f.r.events {
		if strings.HasSuffix(s, ".return") || s == "log.closed" || s == "http.close" || s == "http.shutdown.call" {
			continue
		}
		result = append(result, s)
	}
	return result
}
func (f *failureFixture) failed(t *testing.T, want []string) {
	t.Helper()
	if f.a.current != nil || f.a.hash != ([32]byte{}) {
		t.Fatal("失敗した世代またはハッシュが確定した")
	}
	v := f.a.Apply()
	if v.State != ApplyFailed || v.Kind != "" || v.Reason == "" {
		t.Fatalf("状態=%+v", v)
	}
	if !reflect.DeepEqual(f.stages(), want) {
		t.Fatalf("段=%v、期待=%v", f.stages(), want)
	}
}

func TestGeneration失敗四経路(t *testing.T) {
	for _, mode := range []string{"読取", "YAML", "検証", "logger", "ports", "Listen"} {
		t.Run(mode, func(t *testing.T) {
			f := newFailureFixture(t)
			want := []string{"config.load"}
			switch mode {
			case "読取", "YAML", "検証":
				f.a.deps.loadConfig = loadConfigBytes
				path := filepath.Join(f.a.opts.BaseDir, configName)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "読取":
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				case "YAML":
					if err := os.WriteFile(path, []byte("not: [valid"), 0600); err != nil {
						t.Fatal(err)
					}
				case "検証":
					if err := os.WriteFile(path, []byte("admin:\n  port: -1\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "logger":
				want = append(want, "logger.open")
				f.a.deps.openLogger = func(o logging.Options) (logHandle, error) { return nil, errors.New(o.Secrets[0]) }
			case "ports":
				want = append(want, "logger.open", "ports.check", "logger.close")
				f.a.deps.checkPorts = func(c *config.Config) error { return errors.New(c.Admin.Password) }
			case "Listen":
				want = append(want, "logger.open", "ports.check", "banner.write", "process.configure", "parts.create", "hidden.load", "core.new", "receivers.register", "parts.start", "http.listen", "core.stopping", "sse.close", "generation.cancel", "parts.wait", "history.finish", "logger.close")
				f.a.deps.listen = func(string, string) (net.Listener, error) {
					// 偽 MediaMTX を含む全 Run の開始を確認してから失敗する。
					for _, p := range f.parts {
						<-p.started
					}
					return nil, errors.New("secret-api")
				}
			}
			if code := f.a.run(context.Background()); code != 1 {
				t.Fatalf("終了コード=%d", code)
			}
			f.failed(t, append(want, "apply.failed"))
			if mode == "logger" || mode == "ports" || mode == "Listen" {
				if strings.Contains(f.a.Apply().Reason, "secret-api") {
					t.Fatal("API の秘密が残った")
				}
				cfg, _, _, _ := loadConfigBytes(f.a.opts.BaseDir)
				if strings.Contains(f.a.Apply().Reason, cfg.Admin.Password) || strings.Contains(f.stderr.String(), cfg.Admin.Password) {
					t.Fatal("管理パスワードが残った")
				}
			}
			if mode == "Listen" {
				for _, n := range []string{"sup", "poll", "obs", "core", "watch"} {
					if !strings.Contains(strings.Join(f.r.events, ","), n+".return") {
						t.Errorf("%s が未回収", n)
					}
				}
			}
		})
	}
}

func TestGeneration生成直後の失敗表示(t *testing.T) {
	for _, mode := range []string{"logger", "ports"} {
		t.Run(mode, func(t *testing.T) {
			f := newFailureFixture(t)
			if err := os.Remove(filepath.Join(f.a.opts.BaseDir, configName)); err != nil {
				t.Fatal(err)
			}
			f.a.deps.loadConfig = loadConfigBytes
			if mode == "logger" {
				f.a.deps.openLogger = func(logging.Options) (logHandle, error) { return nil, errors.New("ログ失敗") }
			} else {
				f.a.deps.checkPorts = func(*config.Config) error { return errors.New("ポート失敗") }
			}
			if code := f.a.run(context.Background()); code != 1 {
				t.Fatal(code)
			}
			cfg, generated, _, err := loadConfigBytes(f.a.opts.BaseDir)
			if err != nil || generated {
				t.Fatalf("生成ファイルが維持されていない: %v", err)
			}
			if !strings.Contains(f.out.String(), strings.ReplaceAll(i18n.UI(i18n.Resolve(cfg.Language, i18n.OSLang))["console.password_unset"], "{port}", fmt.Sprint(cfg.Admin.Port))) {
				t.Fatalf("パスワード表示=%s", f.out.String())
			}
		})
	}
}

func TestGeneration停止期限と二重停止(t *testing.T) {
	for _, slow := range []bool{false, true} {
		t.Run(map[bool]string{false: "即応", true: "期限"}[slow], func(t *testing.T) {
			f := newFailureFixture(t)
			g, err := f.a.createGeneration(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			<-f.h.started
			// 期限は、停止を呼ぶ直前の時刻からの差で確かめる（実時間の経過に頼らず、負荷で揺れないように）
			var start time.Time
			f.stop = func(ctx context.Context) {
				if g.ctx.Err() != nil {
					t.Error("Stopping 前に世代が取消された")
				}
				deadline, ok := ctx.Deadline()
				if !ok || deadline.Sub(start) < core.DefaultStopTimeout {
					t.Errorf("Stopping の期限が短い: %v", deadline.Sub(start))
				}
				if slow {
					<-ctx.Done()
				}
				if g.ctx.Err() != nil {
					t.Error("Stopping 中に世代が取消された")
				}
			}
			start = time.Now()
			g.stopGeneration()
			elapsed := time.Since(start)
			// 上限は止まらないことを見つけるためだけの値（負荷で揺れないよう余裕を持たせる）
			if slow && (elapsed < core.DefaultStopTimeout || elapsed > 15*time.Second) {
				t.Fatalf("停止時間=%v", elapsed)
			}
			if !slow && elapsed > 10*time.Second {
				t.Fatalf("即応なのに停止が遅い: %v", elapsed)
			}
			before := len(f.r.events)
			g.stopGeneration()
			if len(f.r.events) != before {
				t.Fatal("停止を二度実行した")
			}
			if g.ctx.Err() == nil {
				t.Fatal("世代が未取消")
			}
		})
	}
}

func TestGenerationHTTP強制終了とServe異常(t *testing.T) {
	for _, mode := range []string{"Shutdown期限", "Serve異常", "ServerClosed"} {
		t.Run(mode, func(t *testing.T) {
			f := newFailureFixture(t)
			f.h.shutdown = func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan int, 1)
			go func() { done <- f.a.run(ctx) }()
			<-f.h.started
			// Serve の終了値は開始前に設定する必要があるため、終了合図だけをここで送る。
			if mode == "Shutdown期限" {
				cancel()
			} else {
				if mode == "Serve異常" {
					f.h.serveError = errors.New("Serve 異常")
				}
				close(f.h.done)
			}
			start := time.Now()
			select {
			case code := <-done:
				want := 0
				if mode == "Serve異常" {
					want = 1
				}
				if code != want {
					t.Fatalf("終了コード=%d", code)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("終了待ちが詰まった")
			}
			if time.Since(start) < 900*time.Millisecond {
				t.Fatal("Shutdown の期限を待たなかった")
			}
			if f.a.Apply().State != ApplyApplied || f.a.hash == (sha256.Sum256(nil)) {
				t.Fatal("実行時の終了を作成失敗にした")
			}
			events := strings.Join(f.r.events, ",")
			if !strings.Contains(events, "http.close,generation.cancel") {
				t.Fatalf("強制終了順=%s", events)
			}
			if f.a.current != nil {
				t.Fatal("世代が未回収")
			}
		})
	}
}
