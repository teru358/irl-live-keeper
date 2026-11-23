// Package app は、屋外配信の受信状態を監視し、OBS のシーンを自動で切り替える実行処理を提供する。
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
	"runtime"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/redact"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"github.com/teru358/irl-live-keeper/internal/web"
)

const (
	configName = "config.yaml"
	// pollInterval は Poller が MediaMTX の API を読む間隔。
	pollInterval = time.Second
	// restartWait は RestartMedia の送出から切替までの待ち時間。Windows と Linux の OBS で、RESTART から
	// 再生開始まで約 1.02秒だった。切替のトランジションの時間も加わるので、1秒で戻った瞬間に映像が出る。
	restartWait = time.Second
	// shutdownTimeout は管理 API の HTTP サーバーの終了を待つ上限。終わる道全体（Stopping 2秒 + これ + 部品の待ち）を、
	// Windows のコンソールを閉じるときの猶予（数秒）に収めるため短くする。
	shutdownTimeout = time.Second
	// stateDirName は hidden.json などの状態を置くフォルダ（config.yaml のあるフォルダの下）。
	stateDirName = "state"
)

// partsContext は部品用の context。親（シグナルの ctx）の値は引き継ぐが、親の取り消しは伝えない。
// 止めるのは返す cancel だけ。
func partsContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(context.WithoutCancel(parent))
}

// hiddenPath は隠した項目の記録 hidden.json のパス。`--dir` ができたら、その下になる（今は base）。
func hiddenPath(base string) string {
	return filepath.Join(base, stateDirName, "hidden.json")
}

// generate は config.yaml がないときの生成。テストで差し替える。
var generate = config.Generate

// Options は app の起動に必要な基準フォルダと出力先。
type Options struct {
	BaseDir string
	Stdout  io.Writer
	Stderr  io.Writer
}

type appDeps struct {
	initJob       func() error
	stage         func(string)
	listen        func(string, string) (net.Listener, error)
	loadConfig    func(string) (*config.Config, bool, []byte, error)
	openLogger    func(logging.Options) (logHandle, error)
	checkPorts    func(*config.Config) error
	newSupervisor func(*config.Config, string, string, *slog.Logger) supervisorRunner
	newPoller     func(string, string, string, []string, []string) pollRunner
	newOBS        func(obs.Config, *slog.Logger) obsRunner
	newMediaWatch func(obsRunner, *slog.Logger) mediaWatcher
	loadHidden    func(string) ([]switcher.HiddenEntry, string)
	newCore       func(core.Deps) coreRunner
	newHTTP       func(coreRunner, string) servingHTTP
}

type runner interface{ Run(context.Context) }
type pollRunner interface {
	Run(context.Context, time.Duration, chan<- ingest.SampleSet)
}
type logHandle interface {
	Slog() *slog.Logger
	AddSecrets(...string)
	Close() error
}
type supervisorRunner interface {
	runner
	APICredentials() (string, string)
	Status() supervisor.Status
	SlowReaders() []supervisor.SlowReader
}
type obsRunner interface {
	obs.Client
	runner
	Warnings() []string
}
type mediaWatcher interface {
	core.MediaWatcher
	Close()
}
type coreRunner interface {
	runner
	stopper
	web.Backend
}
type servingHTTP interface {
	httpServer
	Serve(net.Listener) error
}

type ApplyState string
type ApplyKind string

const (
	ApplyIdle     ApplyState = "idle"
	ApplyApplying ApplyState = "applying"
	ApplyApplied  ApplyState = "applied"
	ApplyFailed   ApplyState = "failed"
)

type Apply struct {
	State  ApplyState
	Kind   ApplyKind
	Reason string
}

type generation struct {
	ctx      context.Context
	cancel   context.CancelFunc
	started  bool
	core     coreRunner
	logger   logHandle
	server   servingHTTP
	mu       sync.Mutex
	stopped  bool
	exitCode int
	wg       sync.WaitGroup
	serveErr chan error
	app      *App
}

type App struct {
	opts       Options
	initErr    error
	initWarned bool
	deps       appDeps
	mu         sync.Mutex
	apply      Apply
	hash       [32]byte
	current    *generation
}

func newApp(opts Options, deps appDeps) *App {
	if deps.initJob == nil {
		deps.initJob = supervisor.InitJob
	}
	if deps.stage == nil {
		deps.stage = func(string) {}
	}
	if deps.listen == nil {
		deps.listen = net.Listen
	}
	if deps.loadConfig == nil {
		deps.loadConfig = loadConfigBytes
	}
	if deps.openLogger == nil {
		deps.openLogger = func(o logging.Options) (logHandle, error) { return logging.Open(o) }
	}
	if deps.checkPorts == nil {
		deps.checkPorts = supervisor.CheckPorts
	}
	if deps.newSupervisor == nil {
		deps.newSupervisor = func(c *config.Config, b, w string, l *slog.Logger) supervisorRunner {
			return supervisor.New(c, b, w, l)
		}
	}
	if deps.newPoller == nil {
		deps.newPoller = func(u, a, p string, ids, paths []string) pollRunner {
			return ingest.NewPoller(u, a, p, ids, paths, nil)
		}
	}
	if deps.newOBS == nil {
		deps.newOBS = func(c obs.Config, l *slog.Logger) obsRunner { return obs.New(c, l) }
	}
	if deps.newMediaWatch == nil {
		deps.newMediaWatch = func(o obsRunner, l *slog.Logger) mediaWatcher {
			return obs.NewMediaWatch(o.(*obs.WS).Aux().MediaAux(), l, nil)
		}
	}
	if deps.loadHidden == nil {
		deps.loadHidden = core.LoadHidden
	}
	if deps.newCore == nil {
		deps.newCore = func(d core.Deps) coreRunner { return core.New(d) }
	}
	if deps.newHTTP == nil {
		deps.newHTTP = func(c coreRunner, p string) servingHTTP {
			return &http.Server{Handler: web.New(c, p), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
		}
	}
	return &App{opts: opts, initErr: deps.initJob(), deps: deps, apply: Apply{State: ApplyIdle}}
}

// Apply は現在の初回反映状態の値を返す。
func (a *App) Apply() Apply { a.mu.Lock(); defer a.mu.Unlock(); return a.apply }

func (a *App) setApply(v Apply) { a.mu.Lock(); a.apply = v; a.mu.Unlock() }

// Run はコンソール用 app を起動し、終了コードを返す。
func Run(ctx context.Context, opts Options) int {
	return newApp(opts, appDeps{}).run(ctx)
}

func (a *App) run(ctx context.Context) int {
	return a.runApp(ctx)
}

// run は旧テスト用の非公開ラッパー。実行本体は Run に集約する。
func run(ctx context.Context, base string, stdout, stderr io.Writer) int {
	return Run(ctx, Options{BaseDir: base, Stdout: stdout, Stderr: stderr})
}

func (a *App) runApp(ctx context.Context) int {
	g, err := a.createGeneration(ctx)
	if err != nil {
		fmt.Fprintln(a.opts.Stderr, err)
		return 1
	}
	code := 0
	select {
	case <-ctx.Done():
		g.logger.Slog().Info("止める操作を受けました。終了します")
	case err := <-g.serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			g.logger.Slog().Error("管理 API が止まりました", "error", err)
			fmt.Fprintf(a.opts.Stderr, "管理 API が止まりました: %v\n", err)
			code = 1
		}
	}
	g.exitCode = code
	g.stopGeneration()
	return code
}

// generationError は表示する理由と、内部で使う失敗段を分けて保持する。
type generationError struct {
	stage string
	err   error
}

func (e *generationError) Error() string { return e.err.Error() }
func (e *generationError) Unwrap() error { return e.err }

// createGeneration は途中の世代を公開せず、全段の成功後に反映する。
func (a *App) createGeneration(ctx context.Context) (*generation, error) {
	base, stdout, initErr := a.opts.BaseDir, a.opts.Stdout, a.initErr
	a.setApply(Apply{State: ApplyApplying})
	stage := a.deps.stage
	stage("config.load")
	// 設定。なければ生成する
	cfg, generated, raw, err := a.deps.loadConfig(base)
	if err != nil {
		a.setApply(Apply{State: ApplyFailed, Reason: err.Error()})
		stage("apply.failed")
		return nil, &generationError{"config.load", err}
	}
	candidate := sha256.Sum256(raw)
	masker := redact.New(cfg.Admin.Password, cfg.Stream.PublishPass, cfg.Stream.ReadPass, cfg.OBS.Password)
	// 生成したパスワードを知る手段が config.yaml を開くことだけにならないよう、起動の表示より前に
	// 失敗して終わるときは、終わる前に標準出力へ出す（ログには書かない）。
	showGeneratedPassword := func() {
		if generated {
			fmt.Fprint(stdout, adminPasswordLine(cfg.Admin.Password))
		}
	}
	logDir := resolvePath(base, cfg.Log.Dir)
	binPath := mediaMTXBinary(base, cfg.MediaMTX.Binary, runtime.GOOS)

	// ログ。パスワードは標準出力にもファイルにも書かない
	stage("logger.open")
	logger, err := a.deps.openLogger(logging.Options{
		Dir:      logDir,
		KeepDays: cfg.Log.KeepDays,
		Stdout:   stdout,
		Secrets:  []string{cfg.Admin.Password, cfg.Stream.PublishPass, cfg.Stream.ReadPass, cfg.OBS.Password},
	})
	if err != nil {
		showGeneratedPassword()
		a.setApply(Apply{State: ApplyFailed, Reason: masker.Mask(err.Error())})
		stage("apply.failed")
		return nil, &generationError{"logger.open", fmt.Errorf("ログを開けません: %s", masker.Mask(err.Error()))}
	}
	log := logger.Slog()
	for _, w := range cfg.Warnings() {
		log.Warn(w)
	}

	if !a.initWarned {
		logInitJobWarning(log, initErr)
		a.initWarned = true
	}

	stage("ports.check")
	if err := a.deps.checkPorts(cfg); err != nil {
		log.Error("ポートの事前確認に失敗しました", "error", err)
		showGeneratedPassword()
		_ = logger.Close()
		stage("logger.close")
		a.setApply(Apply{State: ApplyFailed, Reason: masker.Mask(err.Error())})
		stage("apply.failed")
		return nil, &generationError{"ports.check", errors.New(masker.Mask(err.Error()))}
	}

	// 送信用 URL の host。起動時に1回だけ決める
	stage("banner.write")
	host, auto, found := choosePublishHost(cfg.Stream.PublishHost, systemInterfaces)
	if auto && !found {
		log.Warn("LAN の IPv4 アドレスが見つからないため、送信用 URL の host を 127.0.0.1 にしました。stream.publish_host で固定できます")
	}
	// 管理 API の URL。lan が true のときは、送信用 URL の自動の決め方と同じで LAN のアドレスを調べる
	lanHost, _, lanFound := choosePublishHost("", systemInterfaces)
	adminURL, adminNote := adminBannerURL(cfg.Admin.Port, cfg.Admin.LAN, lanHost, lanFound)
	cameraURLs := buildCameraURLs(cfg, host)
	info := bannerInfo{
		Cameras:  cameraURLs,
		AdminURL: adminURL, AdminNote: adminNote, AutoHost: auto, HostFound: found,
	}
	if generated {
		info.AdminPassword = cfg.Admin.Password
	}
	fmt.Fprint(stdout, startupBanner(info))

	// 部品を動かす
	// 部品の context はシグナルの ctx から独立させる。シグナルは終わる手順（shutdown）の開始の合図にだけ使い、
	// Stopping と HTTP の停止のあとで、shutdown が cancel を呼ぶ。
	stage("process.configure")
	stage("parts.create")
	g := &generation{logger: logger, app: a}
	runCtx, cancel := partsContext(ctx)
	g.ctx, g.cancel = runCtx, cancel
	wg := &g.wg

	sup := a.deps.newSupervisor(cfg, binPath, base, log)
	apiUser, apiPass := sup.APICredentials()
	logger.AddSecrets(apiPass)
	masker.Add(apiPass)
	poller := a.deps.newPoller(fmt.Sprintf("http://127.0.0.1:%d", cfg.MediaMTX.APIPort), apiUser, apiPass, cameraIDs(cfg), cameraPaths(cfg))
	samples := make(chan ingest.SampleSet) // Poller → core。全カメラの Sample の1組
	obsClient := a.deps.newOBS(obsConfig(cfg), log)

	// 隠した項目の記録。core を作る前に読む（読めなければ警告を出して空として扱い、元のファイルは .broken に残す）
	stage("hidden.load")
	hidden, hiddenWarn := a.deps.loadHidden(hiddenPath(base))
	if hiddenWarn != "" {
		log.Warn(hiddenWarn)
	}
	if len(hidden) > 0 {
		// 落ちたあとの起動で記録を引き継いだことを、あとで追えるようにする（M-13 の Windows の確認）
		log.Info("隠した項目の記録を読んだ（OBS につないだら照合する）", "items", len(hidden))
	}

	deps := coreDeps(cfg, cameraURLs)
	deps.HiddenPath, deps.HiddenRestored, deps.HiddenLoadWarning = hiddenPath(base), hidden, hiddenWarn
	deps.Machine = switcher.New(switcher.Config{
		RestartWait: restartWait,
		Endpoint:    switcher.Endpoint{Host: cfg.OBS.Host, Port: cfg.OBS.Port},
	})
	deps.OBS = obsClient
	deps.Samples = samples
	deps.MediaMTX = sup.Status
	deps.SlowReaders = sup.SlowReaders
	deps.Warnings = obsClient.Warnings
	mediaWatch := a.deps.newMediaWatch(obsClient, log)
	deps.MediaWatch = mediaWatch
	deps.Log = log
	stage("core.new")
	c := a.deps.newCore(deps)
	g.core = c
	stage("receivers.register")

	stage("parts.start")
	g.started = true
	wg.Go(func() { sup.Run(runCtx) })
	wg.Go(func() { poller.Run(runCtx, pollInterval, samples) })
	wg.Go(func() { obsClient.Run(runCtx) })
	wg.Go(func() { c.Run(runCtx) })
	wg.Go(func() { <-runCtx.Done(); mediaWatch.Close() }) // 見張りの goroutine を残さない

	// 管理 API。待ち受けに失敗したときも、部品を止めてから終了する
	srv := a.deps.newHTTP(c, cfg.Admin.Password)
	g.serveErr = make(chan error, 1)
	addr := listenAddr(cfg.Admin.Port, cfg.Admin.LAN)
	stage("http.listen")
	ln, err := a.deps.listen("tcp", addr)
	if err != nil {
		log.Error("管理 API のポート（admin.port）で待ち受けできません", "listen", addr, "error", err)
		g.exitCode = 1 // 止める処理の「終了しました」のログを、実際の終了コードに合わせる
		g.stopGeneration()

		a.setApply(Apply{State: ApplyFailed, Reason: masker.Mask(err.Error())})
		stage("apply.failed")
		return nil, &generationError{"http.listen", fmt.Errorf("管理 API のポート（admin.port）%d で待ち受けできません（%s）: %s", cfg.Admin.Port, addr, masker.Mask(err.Error()))}
	}
	g.server = srv
	stage("http.serve")
	wg.Go(func() { g.serveErr <- srv.Serve(ln) })
	a.mu.Lock()
	a.hash = candidate
	a.current = g
	a.mu.Unlock()
	stage("config.commit")
	a.setApply(Apply{State: ApplyApplied})

	return g, nil
}

func (g *generation) stopGeneration() {
	a, srv, wg := g.app, g.server, &g.wg
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return
	}
	g.stopped = true
	g.mu.Unlock()
	stage := a.deps.stage
	if g.started {
		stage("core.stopping")
	}
	if g.started {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), core.DefaultStopTimeout)
		g.core.Stopping(stopCtx)
		stopCancel()
	}
	stage("sse.close")
	if srv != nil {
		stage("http.shutdown")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		if err := srv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = srv.Close()
		}
		shutCancel()
	}
	stage("generation.cancel")
	g.cancel()
	wg.Wait()
	stage("parts.wait")
	stage("history.finish")
	g.logger.Slog().Info("終了しました", "code", g.exitCode)
	_ = g.logger.Close()
	stage("logger.close")
	a.mu.Lock()
	a.current = nil
	a.mu.Unlock()
}

func logInitJobWarning(log *slog.Logger, err error) {
	if err != nil {
		log.Warn("子プロセスをまとめて終了させる仕組みを使えません", "error", err)
	}
}

// stopper は終わる前に隠した項目を戻す core（core.Core.Stopping）。
type stopper interface{ Stopping(ctx context.Context) }

// httpServer は shutdown が止める管理 API のサーバー（*http.Server）。
type httpServer interface {
	Shutdown(ctx context.Context) error
	Close() error
}

// shutdown は終わる道（Ctrl+C、SIGTERM、Windows のコンソールを閉じる）の止める順:
// core の Stopping（最大 core.DefaultStopTimeout＝2秒。Windows のコンソールを閉じるときは数秒しか待たない）→
// HTTP サーバー → ctx の取り消し → 部品が止まるのを待つ（MediaMTX を含む）。srv は nil でもよい。
func shutdown(c stopper, srv httpServer, cancel func(), wg *sync.WaitGroup) {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), core.DefaultStopTimeout)
	c.Stopping(stopCtx)
	stopCancel()
	if srv != nil {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		if err := srv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = srv.Close()
		}
		shutCancel()
	}
	cancel()
	wg.Wait()
}

// loadConfig は base の config.yaml を読む。なければ生成し、generated を true にする。
// log.dir と mediamtx.binary の解決は呼び出し側で行う。
func loadConfig(base string) (cfg *config.Config, generated bool, err error) {
	cfg, generated, _, err = loadConfigBytes(base)
	return
}

func loadConfigBytes(base string) (cfg *config.Config, generated bool, raw []byte, err error) {
	path := filepath.Join(base, configName)
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		cfg, err = generate(path)
		if err == nil {
			raw, err = os.ReadFile(path)
			if err == nil {
				cfg, err = config.Parse(raw)
			}
		}
		return cfg, err == nil, raw, err
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		return nil, false, nil, fmt.Errorf("設定ファイルを読めません: %w", err)
	}
	cfg, err = config.Parse(raw)
	return cfg, false, raw, err
}
