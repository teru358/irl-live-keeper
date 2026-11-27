// Package app は、屋外配信の受信状態を監視し、OBS のシーンを自動で切り替える実行処理を提供する。
package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/redact"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"github.com/teru358/irl-live-keeper/internal/sources"
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
	BaseDir          string
	ExeDir           string
	Stdout           io.Writer
	Stderr           io.Writer
	StdoutIsTerminal func() bool
	InvocationID     string
	CrashDir         string
	CrashPath        string
	StartedAt        time.Time
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
	loadHidden    func(string) ([]switcher.HiddenEntry, i18n.Msg)
	newCore       func(core.Deps) coreRunner
	newHTTP       func(coreRunner, string, i18n.Lang) servingHTTP
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
	UpdateScenes([]string)
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
	ApplyRestart  ApplyKind  = "restart"
	ApplyRevert   ApplyKind  = "revert"
	ApplyIdle     ApplyState = "idle"
	ApplyApplying ApplyState = "applying"
	ApplyApplied  ApplyState = "applied"
	ApplyFailed   ApplyState = "failed"
	ApplyReverted ApplyState = "reverted"
)

type Apply struct {
	State  ApplyState
	Kind   ApplyKind
	Reason string
}

type generation struct {
	obs      obsRunner
	events   *web.Events
	lang     i18n.Lang
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
	sourcesStore           *sources.Store
	pendingSettings        *savedSettings
	liveConfig             *config.Config
	savedSession           string
	settingsReady          chan struct{}
	updateOBSInput         bool
	obsInputURLs           map[string]string   // camera id -> OBS URL。プロセス寿命の書き換え印
	obsInputOldURLs        map[string]string   // camera id -> 保存前の OBS URL
	obsInputUUIDs          map[string][]string // 保存前の地図で対応した入力 UUID。カメラ ID 変更用
	obsInputWarning        msgBox              // core の中核ループが読むので App.mu ではなく専用の lock で守る
	windowLang             i18n.Lang
	windowClose, windowLog string
	windowGenerated        bool
	windowPort             int
	generationID           uint64
	auth                   *web.Auth
	adminPassword          string
	windowPasswordUnset    bool
	adminPort              int
	opts                   Options
	initErr                error
	initWarned             bool
	deps                   appDeps
	mu                     sync.Mutex
	apply                  Apply
	hash                   [32]byte
	current                *generation
	restarts               chan struct{}
	running                bool
	secrets                []string
	bannerShown            bool
	consoleLang            i18n.Lang
}

func newApp(opts Options, deps appDeps) *App {
	if opts.ExeDir == "" {
		opts.ExeDir = opts.BaseDir
	}
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
		deps.loadHidden = core.LoadHiddenMessage
	}
	if deps.newCore == nil {
		deps.newCore = func(d core.Deps) coreRunner { return core.New(d) }
	}
	auth := web.NewAuth(nil)
	var app *App
	if deps.newHTTP == nil {
		deps.newHTTP = func(c coreRunner, p string, lang i18n.Lang) servingHTTP {
			return &http.Server{Handler: web.NewWithAuth(c, p, app.adminPort, auth, lang), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
		}
	}
	app = &App{auth: auth, opts: opts, initErr: deps.initJob(), deps: deps, apply: Apply{State: ApplyIdle}, restarts: make(chan struct{}, 1), settingsReady: make(chan struct{}, 1)}
	return app
}

// Apply は現在の反映状態の値を返す。
func (a *App) Apply() Apply { a.mu.Lock(); defer a.mu.Unlock(); return a.apply }

func (a *App) setApply(v Apply) { a.mu.Lock(); a.apply = v; a.mu.Unlock() }

func (a *App) rememberSecrets(secrets ...string) {
	seen := make(map[string]struct{}, len(a.secrets)+len(secrets))
	for _, secret := range a.secrets {
		if secret != "" {
			seen[secret] = struct{}{}
		}
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if _, ok := seen[secret]; ok {
			continue
		}
		a.secrets = append(a.secrets, secret)
		seen[secret] = struct{}{}
	}
}

// ErrApplying は反映中の操作を受け付けられないことを表す。
var ErrApplying = errors.New("settings are being applied")

// ErrNotRunning は実行役が操作を受け付けないことを表す。
var ErrNotRunning = errors.New("application is not running")

// Restart は実行役へ再起動を依頼し、世代の停止を待たず戻る。
func (a *App) Restart() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return ErrNotRunning
	}
	if a.apply.State == ApplyApplying {
		return ErrApplying
	}
	a.apply = Apply{State: ApplyApplying, Kind: ApplyRestart}
	a.restarts <- struct{}{}
	return nil
}

// Run はコンソール用 app を起動し、終了コードを返す。
func Run(ctx context.Context, opts Options) int {
	return Prepare(opts)(ctx)
}

func (a *App) run(ctx context.Context) int {
	return a.runApp(ctx)
}

// run は旧テスト用の非公開ラッパー。実行本体は Run に集約する。
func run(ctx context.Context, base string, stdout, stderr io.Writer) int {
	return Run(ctx, Options{BaseDir: base, Stdout: stdout, Stderr: stderr})
}

func (a *App) runApp(ctx context.Context) int {
	a.mu.Lock()
	a.running = true
	a.apply = Apply{State: ApplyApplying, Kind: a.apply.Kind}
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.running = false; a.mu.Unlock() }()
	g, err := a.createGeneration(ctx)
	if err != nil {
		if a.Apply().Kind == ApplyRestart {
			a.windowLogMessage("restart failed", "error", err.Error())
		}
		fmt.Fprintln(a.opts.Stderr, a.failureMessage(err))
		return 1
	}
	for {
		code := 0
		select {
		case <-ctx.Done():
			g.logger.Slog().Info("Shutdown requested")
		case <-a.settingsReady:
			a.mu.Lock()
			pending := a.pendingSettings
			a.mu.Unlock()
			if pending != nil && pending.UpdateOBSInput {
				a.captureOBSInputUUIDs(g, pending)
			}
			if pending.Summary.Apply != settings.ApplyRestart {
				a.applyLive(g, pending)
				continue
			}
			// 自分の受け口へは待たずに送る（もう再起動が積まれていれば、その再起動が新しい設定を読む）
			select {
			case a.restarts <- struct{}{}:
			default:
			}
			continue
		case <-a.restarts:
			g.logger.Slog().Info("restart requested")
			previous := g
			g.stopGenerationKeepingLog()
			if ctx.Err() != nil {
				_ = previous.logger.Close()
				return 0
			}
			g, err = a.createGeneration(ctx)
			if err != nil {
				previous.logger.Slog().Error("restart failed", "error", redact.New(a.secrets...).Mask(err.Error()))
				_ = previous.logger.Close()
				fmt.Fprintln(a.opts.Stderr, a.failureMessage(err))
				return 1
			}
			_ = previous.logger.Close()
			continue
		case err := <-g.serveErr:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				g.logger.Slog().Error("admin API stopped", "error", err)
				fmt.Fprintln(a.opts.Stderr, i18n.Render(g.lang, i18n.New("console.serve", "error", err.Error())))
				code = 1
			}
		}
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		g.exitCode = code
		g.stopGeneration()
		return code
	}
}

func (a *App) failureMessage(err error) string {
	message := a.Apply().Reason
	if message == "" {
		message = i18n.Render(a.consoleLang, i18n.Reason(err))
	}
	if _, statErr := os.Stat(filepath.Join(a.opts.BaseDir, configName+".bak")); statErr == nil {
		lang := a.consoleLang
		if lang != i18n.JA && lang != i18n.EN {
			lang = i18n.OSLang()
		}
		message += "\n" + i18n.Render(lang, i18n.New("console.rollback"))
	}
	return message
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
	kind := a.Apply().Kind
	a.setApply(Apply{State: ApplyApplying, Kind: kind})
	stage := a.deps.stage
	stage("config.load")
	loadMasker := redact.New(a.secrets...)
	// 設定。なければ生成する
	cfg, generated, raw, err := a.deps.loadConfig(base)
	if err != nil {
		message := i18n.New("console.config_load")
		var validation *config.ValidationError
		if errors.As(err, &validation) {
			lines := make([]string, len(validation.Fields))
			for i, field := range validation.Fields {
				lines[i] = field.Field + ": " + loadMasker.Mask(i18n.Render(i18n.EN, field.Message))
			}
			message = i18n.New("console.config_invalid", "details", strings.Join(lines, "\n  - "))
		}
		// 窓と管理ページの理由は本体の言語で（ログ用の英語は Error()）
		a.setApply(Apply{State: ApplyFailed, Kind: kind, Reason: loadMasker.Mask(i18n.Render(a.consoleLang, message))})
		stage("apply.failed")
		return nil, &generationError{"config.load", i18n.Error{Message: message, Cause: err}}
	}
	lang := i18n.Resolve(cfg.Language, i18n.OSLang)
	a.mu.Lock()
	a.windowLang = lang
	a.windowClose = cfg.GUI.CloseAction
	a.windowLog = resolvePath(base, cfg.Log.Dir)
	a.windowGenerated = a.windowGenerated || generated
	a.windowPort = cfg.Admin.Port
	a.windowPasswordUnset = cfg.Admin.Password == ""
	a.mu.Unlock()
	a.consoleLang = lang
	if cfg.Admin.Password == "" {
		fmt.Fprintln(stdout, strings.ReplaceAll(i18n.UI(lang)["console.password_unset"], "{port}", fmt.Sprint(cfg.Admin.Port)))
	}
	candidate := sha256.Sum256(raw)
	a.rememberSecrets(cfg.Admin.Password, cfg.Stream.PublishPass, cfg.Stream.ReadPass, cfg.OBS.Password)
	masker := redact.New(a.secrets...)
	// 生成したパスワードを知る手段が config.yaml を開くことだけにならないよう、起動の表示より前に
	// 失敗して終わるときは、終わる前に標準出力へ出す（ログには書かない）。
	showGeneratedPassword := func() {
		if cfg.Admin.Password != "" && shouldShowGeneratedPassword(generated, a.sensitiveOutput()) {
			fmt.Fprint(stdout, adminPasswordLine(cfg.Admin.Password, lang))
		}
	}
	logDir := resolvePath(base, cfg.Log.Dir)
	binPath := configuredMediaMTXBinary(base, a.opts.ExeDir, cfg.MediaMTX.Binary, runtime.GOOS)

	// ログ。パスワードは標準出力にもファイルにも書かない
	stage("logger.open")
	logger, err := a.deps.openLogger(logging.Options{
		Dir:      logDir,
		KeepDays: cfg.Log.KeepDays,
		Stdout:   stdout,
		Secrets:  append([]string(nil), a.secrets...),
	})
	if err != nil {
		showGeneratedPassword()
		message := i18n.New("console.logger_open", "error", masker.Mask(err.Error()))
		a.setApply(Apply{State: ApplyFailed, Kind: kind, Reason: i18n.Render(lang, message)})
		stage("apply.failed")
		return nil, &generationError{"logger.open", i18n.Error{Message: message, Cause: err}}
	}
	log := logger.Slog()
	if a.opts.CrashDir != "" {
		if err := logging.PruneCrashLogs(a.opts.CrashDir, a.opts.CrashPath, cfg.Log.KeepDays, true, a.opts.StartedAt); err != nil {
			log.Warn("cannot remove old crash logs", "err", err)
		}
	}
	for _, w := range cfg.WarningMessages() {
		log.Warn(i18n.Render(i18n.EN, w))
	}

	if !a.initWarned {
		logInitJobWarning(log, initErr)
		a.initWarned = true
	}

	stage("ports.check")
	if err := a.deps.checkPorts(cfg); err != nil {
		log.Error("port precheck failed", "error", err)
		showGeneratedPassword()
		_ = logger.Close()
		stage("logger.close")
		// 画面とコンソールの文は、エラーが持つ文（Msg）を本体の言語で組み立てる（Error() はログ用の英語）
		reason := masker.Mask(i18n.Render(lang, i18n.Reason(err)))
		a.setApply(Apply{State: ApplyFailed, Kind: kind, Reason: reason})
		stage("apply.failed")
		return nil, &generationError{"ports.check", i18n.Error{Message: i18n.Text(masker.Mask(i18n.Render(i18n.EN, i18n.Reason(err)))), Cause: err}}
	}

	// 送信用 URL の host。起動時に1回だけ決める
	if !a.bannerShown {
		stage("banner.write")
		host, auto, found := choosePublishHost(cfg.Stream.PublishHost, systemInterfaces)
		if auto && !found {
			log.Warn("LAN IPv4 address not found; using 127.0.0.1 for publish URL host")
		}
		// 管理 API の URL。lan が true のときは、送信用 URL の自動の決め方と同じで LAN のアドレスを調べる
		lanHost, _, lanFound := choosePublishHost("", systemInterfaces)
		adminURL, adminNote := adminBannerURL(cfg.Admin.Port, cfg.Admin.LAN, lanHost, lanFound, lang)
		cameraURLs := buildCameraURLs(cfg, host)
		if a.sensitiveOutput() {
			cameraURLs = maskCameraURLs(cameraURLs)
			adminURL = maskURLPassword(adminURL)
		}
		info := bannerInfo{
			Cameras:  cameraURLs,
			AdminURL: adminURL, AdminNote: adminNote, AutoHost: auto, HostFound: found,
			Lang: lang,
		}
		if generated {
			if !a.sensitiveOutput() {
				info.AdminPassword = cfg.Admin.Password
			}
		}
		info.SensitiveOutput = a.sensitiveOutput()
		fmt.Fprint(stdout, startupBanner(info))
		a.bannerShown = true
	}
	host, _, _ := choosePublishHost(cfg.Stream.PublishHost, systemInterfaces)
	cameraURLs := buildCameraURLs(cfg, host)

	// 部品を動かす
	// 部品の context はシグナルの ctx から独立させる。シグナルは終わる手順（shutdown）の開始の合図にだけ使い、
	// Stopping と HTTP の停止のあとで、shutdown が cancel を呼ぶ。
	stage("process.configure")
	a.configureProcess(cfg)
	stage("parts.create")
	g := &generation{logger: logger, app: a, lang: lang}
	runCtx, cancel := partsContext(ctx)
	g.ctx, g.cancel = runCtx, cancel
	wg := &g.wg

	sup := a.deps.newSupervisor(cfg, binPath, base, log)
	apiUser, apiPass := sup.APICredentials()
	a.rememberSecrets(apiPass)
	logger.AddSecrets(apiPass)
	masker.Add(apiPass)
	poller := a.deps.newPoller(fmt.Sprintf("http://127.0.0.1:%d", cfg.MediaMTX.APIPort), apiUser, apiPass, cameraIDs(cfg), cameraPaths(cfg))
	samples := make(chan ingest.SampleSet) // Poller → core。全カメラの Sample の1組
	obsClient := a.deps.newOBS(obsConfig(cfg), log)
	connectedUpdates := make(chan switcher.OBSConnected, 1)
	if hook, ok := obsClient.(interface {
		SetConnectedHook(func(switcher.OBSConnected))
	}); ok {
		hook.SetConnectedHook(func(e switcher.OBSConnected) {
			select {
			case connectedUpdates <- e:
			default:
				select {
				case <-connectedUpdates:
				default:
				}
				select {
				case connectedUpdates <- e:
				default:
				}
			}
		})
	}

	// 隠した項目の記録。core を作る前に読む（読めなければ警告を出して空として扱い、元のファイルは .broken に残す）
	stage("hidden.load")
	hidden, hiddenWarn := a.deps.loadHidden(hiddenPath(base))
	if hiddenWarn.Code != "" {
		log.Warn(i18n.Render(i18n.EN, hiddenWarn))
	}
	if len(hidden) > 0 {
		// 落ちたあとの起動で記録を引き継いだことを、あとで追えるようにする（M-13 の Windows の確認）
		log.Info("Loaded hidden item records; they will be checked after connecting to OBS", "items", len(hidden))
	}

	deps := coreDeps(cfg, cameraURLs)
	deps.HiddenPath, deps.HiddenRestored, deps.HiddenLoadWarning = hiddenPath(base), hidden, i18n.Render(lang, hiddenWarn)
	deps.HiddenLoadMessage = hiddenWarn
	deps.Machine = switcher.New(switcher.Config{
		RestartWait: restartWait,
		Endpoint:    switcher.Endpoint{Host: cfg.OBS.Host, Port: cfg.OBS.Port},
	})
	deps.OBS = obsClient
	deps.Samples = samples
	deps.MediaMTX = sup.Status
	deps.SlowReaders = sup.SlowReaders
	if w, ok := obsClient.(interface{ WarningMessages() []i18n.Msg }); ok {
		deps.WarningMessages = func() []i18n.Msg {
			out := w.WarningMessages()
			if m := sup.Status().LastMessage; m.Code == "mediamtx.missing" {
				out = append(out, m)
			}
			// 中核ループから呼ばれるので App.mu を取らない（App が mu を持ったまま core を待つと互いに待ち合う）
			pending := a.obsInputWarning.Load()
			if pending.Code != "" {
				out = append(out, pending)
			}
			return out
		}
	}
	mediaWatch := a.deps.newMediaWatch(obsClient, log)
	deps.MediaWatch = mediaWatch
	deps.Log = log
	stage("core.new")
	c := a.deps.newCore(deps)
	g.core = c
	g.obs = obsClient
	stage("receivers.register")
	var eventLogger interface {
		Subscribe(int) (<-chan logging.Line, func())
	}
	if l, ok := logger.(interface {
		Subscribe(int) (<-chan logging.Line, func())
	}); ok {
		eventLogger = l
	}
	a.mu.Lock()
	a.generationID++
	id := a.generationID
	a.mu.Unlock()
	if a.sourcesStore == nil {
		a.sourcesStore = &sources.Store{Path: filepath.Join(a.opts.BaseDir, "sources.yaml")}
	}
	a.sourcesStore.SetLogger(log)
	backend := &pageBackend{sourceStore: a.sourcesStore, statistics: statisticsReader(obsClient), Backend: c, senderConfig: cfg, senderDefault: host, app: a, id: id, language: cfg.Language, closeAction: cfg.GUI.CloseAction, lan: cfg.Admin.LAN}
	g.events = web.NewEvents(backend, eventLogger, nil, lang)

	stage("parts.start")
	g.started = true
	wg.Go(func() { sup.Run(runCtx) })
	wg.Go(func() { poller.Run(runCtx, pollInterval, samples) })
	wg.Go(func() { obsClient.Run(runCtx) })
	wg.Go(func() {
		for {
			select {
			case <-runCtx.Done():
				return
			case e := <-connectedUpdates:
				a.updateOBSInputs(obsClient, e)
			}
		}
	})
	wg.Go(func() { c.Run(runCtx) })
	wg.Go(func() { <-runCtx.Done(); mediaWatch.Close() }) // 見張りの goroutine を残さない

	// 管理 API。待ち受けに失敗したときも、部品を止めてから終了する
	srv := a.deps.newHTTP(c, cfg.Admin.Password, lang)
	if h, ok := srv.(*http.Server); ok {
		h.Handler = web.NewWithLogger(backend, cfg.Admin.Password, a.adminPort, a.auth, g.events, log, lang)
	}
	g.serveErr = make(chan error, 1)
	addr := listenAddr(cfg.Admin.Port, cfg.Admin.LAN)
	stage("http.listen")
	ln, err := a.deps.listen("tcp", addr)
	if err != nil {
		log.Error("cannot listen on admin.port", "listen", addr, "error", err)
		g.exitCode = 1 // 止める処理の「終了しました」のログを、実際の終了コードに合わせる
		g.stopGeneration()

		message := i18n.New("console.listen", "port", fmt.Sprint(cfg.Admin.Port), "address", addr, "error", masker.Mask(err.Error()))
		a.setApply(Apply{State: ApplyFailed, Kind: kind, Reason: i18n.Render(lang, message)})
		stage("apply.failed")
		return nil, &generationError{"http.listen", i18n.Error{Message: message, Cause: err}}
	}
	g.server = srv
	stage("http.serve")
	wg.Go(func() { g.serveErr <- srv.Serve(ln) })
	a.mu.Lock()
	a.hash = candidate
	a.liveConfig = cfg
	a.current = g
	a.mu.Unlock()
	stage("config.commit")
	if kind != "" {
		log.Info("restart complete", "kind", kind)
	}
	state := ApplyApplied
	if kind == ApplyRevert {
		state = ApplyReverted
	}
	a.setApply(Apply{State: state, Kind: kind})

	return g, nil
}

func (a *App) sensitiveOutput() bool {
	return a.opts.InvocationID != "" || (a.opts.StdoutIsTerminal != nil && !a.opts.StdoutIsTerminal())
}

func shouldShowGeneratedPassword(generated, sensitiveOutput bool) bool {
	return generated && !sensitiveOutput
}

func (g *generation) stopGeneration()           { g.stopGenerationLog(true) }
func (g *generation) stopGenerationKeepingLog() { g.stopGenerationLog(false) }
func (g *generation) stopGenerationLog(closeLog bool) {
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
	if g.events != nil {
		g.events.Close()
	}
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
	if closeLog {
		g.logger.Slog().Info("shutdown complete", "code", g.exitCode)
	} else {
		g.logger.Slog().Info("generation stopped", "reason", "restart")
	}
	if closeLog {
		_ = g.logger.Close()
		stage("logger.close")
	}
	a.mu.Lock()
	a.current = nil
	a.mu.Unlock()
}

func logInitJobWarning(log *slog.Logger, err error) {
	if err != nil {
		log.Warn("cannot use child process job object", "error", err)
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
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, false, nil, fmt.Errorf("設定フォルダを作成できません: %w", err)
	}
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

// pageBackend は本体の操作と世代固有の表示情報を管理画面に渡す。
type pageBackend struct {
	web.Backend
	statistics            sources.StatisticsReader
	sourceStore           *sources.Store
	senderConfig          *config.Config
	senderDefault         string
	app                   *App
	id                    uint64
	language, closeAction string
	lan                   bool
}

func (b *pageBackend) DisplayStatus() map[string]any {
	b.app.mu.Lock()
	defer b.app.mu.Unlock()
	v := b.app.apply
	closeAction := b.closeAction
	if b.app.liveConfig != nil {
		closeAction = b.app.liveConfig.GUI.CloseAction
	}
	return map[string]any{"apply": map[string]any{"state": v.State, "kind": v.Kind, "reason": v.Reason}, "generation": b.id, "language": b.language, "gui": map[string]any{"close_action": closeAction}, "admin": map[string]any{"lan": b.lan}}
}
func (b *pageBackend) PageLanguage() string                  { return b.language }
func (b *pageBackend) SourceFilesStore() *sources.FilesStore { return b.app.SourceFilesStore() }
func (b *pageBackend) AdminLANEnabled() bool                 { return b.lan }
func (b *pageBackend) RestartHTTP() (int, error) {
	err := b.app.Restart()
	if errors.Is(err, ErrApplying) {
		return 409, err
	}
	if err != nil {
		return 503, err
	}
	return 202, nil
}

func (b *pageBackend) SubscribeSnapshots() <-chan struct{} {
	if c, ok := b.Backend.(interface{ SubscribeSnapshots() <-chan struct{} }); ok {
		return c.SubscribeSnapshots()
	}
	return nil
}
func (b *pageBackend) SubscribeEvents(n int) *core.EventSub {
	if c, ok := b.Backend.(interface{ SubscribeEvents(int) *core.EventSub }); ok {
		return c.SubscribeEvents(n)
	}
	return &core.EventSub{}
}

func (b *pageBackend) Scenes() ([]obs.Scene, error) {
	return b.Backend.(interface{ Scenes() ([]obs.Scene, error) }).Scenes()
}
func (b *pageBackend) Inputs() ([]obs.Input, error) {
	return b.Backend.(interface{ Inputs() ([]obs.Input, error) }).Inputs()
}
func (b *pageBackend) SelectScene(ctx context.Context, uuid string) error {
	return b.Backend.(interface {
		SelectScene(context.Context, string) error
	}).SelectScene(ctx, uuid)
}

// Prepare は Job を初期化し、世代を開始する関数を返す。GUI はこの間に窓を作る。
func Prepare(opts Options) func(context.Context) int {
	a := newApp(opts, appDeps{})
	return a.run
}
