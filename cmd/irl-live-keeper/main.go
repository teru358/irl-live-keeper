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
	// shutdownTimeout は管理 API の HTTP サーバーの終了を待つ上限。
	shutdownTimeout = 5 * time.Second
)

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
	urls := buildURLs(cfg, host)
	// 管理 API の URL。lan が true のときは、送信用 URL の自動の決め方と同じで LAN のアドレスを調べる
	lanHost, _, lanFound := choosePublishHost("", systemInterfaces)
	adminURL, adminNote := adminBannerURL(cfg.Admin.Port, cfg.Admin.LAN, lanHost, lanFound)
	info := bannerInfo{
		PublishSRT: urls.PublishSRT, PublishRTMP: urls.PublishRTMP, OBSRead: urls.OBSRead,
		AdminURL: adminURL, AdminNote: adminNote, AutoHost: auto, HostFound: found,
	}
	if generated {
		info.AdminPassword = cfg.Admin.Password
	}
	fmt.Fprint(stdout, startupBanner(info))

	// 部品を動かす
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup

	sup := supervisor.New(cfg, binPath, base, log)
	apiUser, apiPass := sup.APICredentials()
	poller := ingest.NewPoller(fmt.Sprintf("http://127.0.0.1:%d", cfg.MediaMTX.APIPort), apiUser, apiPass, cameraIDs(cfg), nil)
	sets := make(chan ingest.SampleSet)
	samples := make(chan ingest.Sample) // つなぎ: core には先頭のカメラの Sample だけを渡す（M-8 で1組にする）
	obsClient := obs.New(obsConfig(cfg), log)

	deps := coreDeps(cfg, urls)
	deps.Machine = switcher.New(switcher.Config{RestartWait: restartWait})
	deps.Judge = ingest.NewJudge(judgeThresholds(cfg))
	deps.OBS = obsClient
	deps.Samples = samples
	deps.MediaMTX = sup.Status
	deps.Warnings = obsClient.Warnings
	deps.Log = log
	c := core.New(deps)

	wg.Go(func() { sup.Run(runCtx) })
	wg.Go(func() { poller.Run(runCtx, pollInterval, sets) })
	wg.Go(func() { forwardFirstCamera(runCtx, sets, samples) })
	wg.Go(func() { obsClient.Run(runCtx) })
	wg.Go(func() { c.Run(runCtx) })

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
		cancel()
		wg.Wait()
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

	// HTTP サーバーを止め、ctx を取り消し、MediaMTX が止まるのを待つ
	shutCtx, shutCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	if err := srv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_ = srv.Close()
	}
	shutCancel()
	cancel()
	wg.Wait()
	return code
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
