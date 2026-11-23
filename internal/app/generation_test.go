package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/web"
)

type generationRecord struct {
	sync.Mutex
	events []string
}

func (r *generationRecord) add(s string) { r.Lock(); defer r.Unlock(); r.events = append(r.events, s) }

type generationPart struct {
	name    string
	r       *generationRecord
	started chan context.Context
}

func (p *generationPart) Run(ctx context.Context) {
	p.started <- ctx
	<-ctx.Done()
	p.r.add(p.name + ".return")
}

type generationSup struct{ *generationPart }

func (*generationSup) APICredentials() (string, string) { return "api", "secret-api" }
func (*generationSup) Status() supervisor.Status {
	return supervisor.Status{State: "restarting", LastError: "再試行中"}
}
func (*generationSup) SlowReaders() []supervisor.SlowReader { return nil }

type generationPoll struct{ *generationPart }

func (p *generationPoll) Run(c context.Context, _ time.Duration, _ chan<- ingest.SampleSet) {
	p.generationPart.Run(c)
}

type generationOBS struct {
	obs.Client
	*generationPart
}

func (*generationOBS) Warnings() []string { return []string{"OBS 未接続"} }

type generationCore struct {
	web.Backend
	*generationPart
	stopping func(context.Context)
}

func (c *generationCore) Stopping(ctx context.Context) { c.stopping(ctx) }

type generationWatch struct {
	core.MediaWatcher
	r *generationRecord
}

func (w *generationWatch) Close() { w.r.add("watch.return") }

type generationLog struct {
	log     *slog.Logger
	r       *generationRecord
	secrets []string
}

func (l *generationLog) Slog() *slog.Logger     { return l.log }
func (l *generationLog) AddSecrets(s ...string) { l.secrets = append(l.secrets, s...) }
func (l *generationLog) Close() error           { l.r.add("log.closed"); return nil }

type generationHTTP struct {
	done    chan struct{}
	started chan struct{}
	r       *generationRecord
}

func (h *generationHTTP) Serve(net.Listener) error {
	close(h.started)
	<-h.done
	h.r.add("http.return")
	return http.ErrServerClosed
}
func (h *generationHTTP) Shutdown(context.Context) error { close(h.done); return nil }
func (h *generationHTTP) Close() error                   { return nil }

func TestGeneration生成停止とHidden(t *testing.T) {
	for _, mode := range []string{"正常", "不在", "破損"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			cfg, _, raw, err := loadConfigBytes(base)
			if err != nil {
				t.Fatal(err)
			}
			if mode != "不在" {
				if err := os.MkdirAll(filepath.Dir(hiddenPath(base)), 0700); err != nil {
					t.Fatal(err)
				}
				data := `{"version":1,"entries":[{"camera":"main","host":"localhost","port":4455,"collection":"配信","container":"scene-id","item":1,"source":"source-id","state":"hidden"}]}`
				if mode == "破損" {
					data = "{"
				}
				if err := os.WriteFile(hiddenPath(base), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r := &generationRecord{}
			part := func(n string) *generationPart { return &generationPart{n, r, make(chan context.Context, 1)} }
			sup, poll, ob, cr := part("sup"), part("poll"), part("obs"), part("core")
			parent, cancel := context.WithCancel(context.WithValue(context.Background(), "世代の値", "維持"))
			defer cancel()
			stopEntered, stopRelease := make(chan struct{}), make(chan struct{})
			logOut := &strings.Builder{}
			logger := &generationLog{log: slog.New(slog.NewTextHandler(logOut, nil)), r: r}
			h := &generationHTTP{make(chan struct{}), make(chan struct{}), r}
			var gotDeps core.Deps
			var a *App
			initCalls := 0
			committed := make(chan struct{})
			a = newApp(Options{base, io.Discard, io.Discard}, appDeps{
				initJob: func() error { initCalls++; return errors.New("Job 失敗") },
				loadConfig: func(string) (*config.Config, bool, []byte, error) {
					if a.Apply().State != ApplyApplying {
						t.Error("読込時に applying ではない")
					}
					return cfg, false, raw, nil
				},
				stage: func(s string) {
					r.add(s)
					if s == "config.commit" {
						close(committed)
					}
				},
				openLogger: func(logging.Options) (logHandle, error) { return logger, nil }, checkPorts: func(*config.Config) error { return nil },
				newSupervisor: func(*config.Config, string, string, *slog.Logger) supervisorRunner { return &generationSup{sup} },
				newPoller: func(_, u, p string, _, _ []string) pollRunner {
					if u != "api" || p != "secret-api" {
						t.Error("API 資格情報が異なる")
					}
					return &generationPoll{poll}
				},
				newOBS:        func(obs.Config, *slog.Logger) obsRunner { return &generationOBS{generationPart: ob} },
				newMediaWatch: func(obsRunner, *slog.Logger) mediaWatcher { return &generationWatch{r: r} },
				newCore: func(d core.Deps) coreRunner {
					gotDeps = d
					return &generationCore{generationPart: cr, stopping: func(context.Context) { close(stopEntered); <-stopRelease }}
				},
				listen: func(string, string) (net.Listener, error) { return nil, nil }, newHTTP: func(coreRunner, string, i18n.Lang) servingHTTP { return h },
			})
			result := make(chan int, 1)
			go func() { result <- a.run(parent) }()
			<-committed
			<-h.started
			contexts := []context.Context{<-sup.started, <-poll.started, <-ob.started, <-cr.started}
			// Apply の公開は commit 通知の直後なので、停止の障壁を通ってから確かめる。
			cancel()
			<-stopEntered
			for _, c := range contexts {
				if c.Err() != nil || c.Value("世代の値") != "維持" {
					t.Errorf("親の取消しが部品に伝わった: %v", c.Err())
				}
			}
			if a.Apply().State != ApplyApplied {
				t.Errorf("反映状態: %+v", a.Apply())
			}
			close(stopRelease)
			if code := <-result; code != 0 {
				t.Fatalf("終了コード=%d", code)
			}
			if a.hash != sha256.Sum256(raw) || a.current != nil {
				t.Error("ハッシュまたは世代回収が不正")
			}
			if initCalls != 1 || strings.Count(logOut.String(), "子プロセスをまとめて終了させる仕組みを使えません") != 1 {
				t.Error("InitJob または警告の回数が不正")
			}
			if !reflect.DeepEqual(logger.secrets, []string{"secret-api"}) {
				t.Errorf("追加の秘密=%v", logger.secrets)
			}
			if gotDeps.MediaMTX().State != "restarting" || len(gotDeps.WarningMessages()) == 0 {
				t.Error("劣化状態が配線されていない")
			}
			if gotDeps.HiddenPath != hiddenPath(base) {
				t.Error("hidden のパスが異なる")
			}
			if mode == "正常" {
				if len(gotDeps.HiddenRestored) != 1 || gotDeps.HiddenRestored[0].ContainerUUID != "scene-id" || gotDeps.HiddenLoadWarning != "" {
					t.Errorf("復元記録=%+v 警告=%s", gotDeps.HiddenRestored, gotDeps.HiddenLoadWarning)
				}
			} else if len(gotDeps.HiddenRestored) != 0 {
				t.Error("空の記録ではない")
			}
			if mode == "破損" {
				if gotDeps.HiddenLoadWarning == "" {
					t.Error("破損の警告がない")
				}
				if _, err := os.Stat(hiddenPath(base) + ".broken"); err != nil {
					t.Fatal(err)
				}
			}
			for _, c := range contexts {
				if c.Err() == nil {
					t.Error("世代 ctx が取り消されていない")
				}
			}
			want := []string{"config.load", "logger.open", "ports.check", "banner.write", "process.configure", "parts.create", "hidden.load", "core.new", "receivers.register", "parts.start", "http.listen", "http.serve", "config.commit", "core.stopping", "sse.close", "http.shutdown", "generation.cancel", "parts.wait", "history.finish", "logger.close"}
			var stages []string
			positions := map[string]int{}
			for i, s := range r.events {
				positions[s] = i
				if !strings.HasSuffix(s, ".return") && s != "log.closed" {
					stages = append(stages, s)
				}
			}
			if !reflect.DeepEqual(stages, want) {
				t.Fatalf("段=%v", stages)
			}
			for _, n := range []string{"sup", "poll", "obs", "core", "watch", "http"} {
				if positions[n+".return"] >= positions["parts.wait"] {
					t.Errorf("%s の回収が遅い", n)
				}
			}
			if positions["log.closed"] >= positions["logger.close"] {
				t.Error("Close より前に通知した")
			}
		})
	}
}

func (*generationOBS) WarningMessages() []i18n.Msg { return []i18n.Msg{i18n.New("api.disconnected")} }
