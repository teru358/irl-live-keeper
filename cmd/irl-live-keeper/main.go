// Command irl-live-keeper は、屋外配信の受信状態を監視し、OBS のシーンを自動で切り替える。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"github.com/teru358/irl-live-keeper/internal/obs"
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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	base, err := baseDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "実行ファイルの場所を調べられません:", err)
		os.Exit(1)
	}
	code := run(ctx, base, os.Stdout, os.Stderr)
	if shouldPause(code, consoleProcessCount) {
		waitEnter(os.Stdin, os.Stderr)
	}
	os.Exit(code)
}

// baseDir は実行ファイルのあるディレクトリ（シンボリックリンクは辿る）。
func baseDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// run は起動から終了までを行い、終了コードを返す。base は config.yaml などの基準のディレクトリ。
func run(ctx context.Context, base string, stdout, stderr io.Writer) int {
	// 設定。なければ生成する
	cfg, generated, err := loadConfig(base)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
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
	logger, err := logging.Open(logging.Options{
		Dir:      logDir,
		KeepDays: cfg.Log.KeepDays,
		Stdout:   stdout,
		Secrets:  []string{cfg.Admin.Password, cfg.Stream.PublishPass, cfg.Stream.ReadPass, cfg.OBS.Password},
	})
	if err != nil {
		fmt.Fprintln(stderr, "ログを開けません:", err)
		showGeneratedPassword()
		return 1
	}
	defer logger.Close()
	log := logger.Slog()
	for _, w := range cfg.Warnings() {
		log.Warn(w)
	}

	if err := supervisor.InitJob(); err != nil {
		log.Warn("子プロセスをまとめて終了させる仕組みを使えません", "error", err)
	}

	if err := supervisor.CheckPorts(cfg); err != nil {
		log.Error("ポートの事前確認に失敗しました", "error", err)
		fmt.Fprintln(stderr, err)
		showGeneratedPassword()
		return 1
	}

	// 送信用 URL の host。起動時に1回だけ決める
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
	runCtx, cancel := partsContext(ctx)
	defer cancel()
	var wg sync.WaitGroup

	sup := supervisor.New(cfg, binPath, base, log)
	apiUser, apiPass := sup.APICredentials()
	poller := ingest.NewPoller(fmt.Sprintf("http://127.0.0.1:%d", cfg.MediaMTX.APIPort), apiUser, apiPass, cameraIDs(cfg), nil)
	samples := make(chan ingest.SampleSet) // Poller → core。全カメラの Sample の1組
	obsClient := obs.New(obsConfig(cfg), log)

	// 隠した項目の記録。core を作る前に読む（読めなければ警告を出して空として扱い、元のファイルは .broken に残す）
	hidden, hiddenWarn := core.LoadHidden(hiddenPath(base))
	if hiddenWarn != "" {
		log.Warn(hiddenWarn)
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
	deps.Warnings = obsClient.Warnings
	mediaWatch := obs.NewMediaWatch(obsClient.Aux().MediaAux(), log, nil)
	deps.MediaWatch = mediaWatch
	deps.Log = log
	c := core.New(deps)

	wg.Go(func() { sup.Run(runCtx) })
	wg.Go(func() { poller.Run(runCtx, pollInterval, samples) })
	wg.Go(func() { obsClient.Run(runCtx) })
	wg.Go(func() { c.Run(runCtx) })
	wg.Go(func() { <-runCtx.Done(); mediaWatch.Close() }) // 見張りの goroutine を残さない

	// 管理 API。待ち受けに失敗したときも、部品を止めてから終了する
	srv := &http.Server{
		Handler:           web.New(c, cfg.Admin.Password),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := make(chan error, 1)
	addr := listenAddr(cfg.Admin.Port, cfg.Admin.LAN)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		shutdown(c, nil, cancel, &wg)
		log.Error("管理 API のポート（admin.port）で待ち受けできません", "listen", addr, "error", err)
		fmt.Fprintf(stderr, "管理 API のポート（admin.port）%d で待ち受けできません（%s）: %v\n", cfg.Admin.Port, addr, err)
		return 1
	}
	go func() { serveErr <- srv.Serve(ln) }()

	code := 0
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		log.Error("管理 API が止まりました", "error", err)
		fmt.Fprintf(stderr, "管理 API が止まりました: %v\n", err)
		code = 1
	}

	// core に Stopping を渡して隠した項目を戻し（最大2秒）、HTTP サーバーを止め、ctx を取り消し、MediaMTX が止まるのを待つ
	shutdown(c, srv, cancel, &wg)
	return code
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
	path := filepath.Join(base, configName)
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		cfg, err = generate(path)
		return cfg, err == nil, err
	}
	cfg, err = config.Load(path)
	return cfg, false, err
}
