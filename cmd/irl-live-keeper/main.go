// Command irl-live-keeper は、屋外配信の受信状態を監視し、OBS のシーンを自動で切り替える。
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/teru358/irl-live-keeper/internal/app"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/settings"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	started := time.Now()
	if len(os.Args) > 1 && os.Args[1] == "service" {
		return runService(os.Args[2:], os.Stdout, os.Stderr)
	}
	args, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	mode, err := prepareMode(args.cli)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			stop()
		case <-ctx.Done():
		}
	}()
	// 終了処理中の2回目以降は、登録済みの通知先で受けて無視する。
	finishConsole, err := installConsoleStop(mode.gui, stop)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer finishConsole()

	base, err := baseDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.Render(i18n.OSLang(), i18n.New("console.executable", "error", err.Error())))
		return 1
	}
	exeDir := base
	if args.dir != "" {
		base, err = filepath.Abs(args.dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.Render(i18n.OSLang(), i18n.New("console.executable", "error", err.Error())))
			return 1
		}
	}
	release, hash, duplicate, err := acquireInstance(base, mode.gui)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer release()
	if duplicate {
		return 0
	}
	if args.restore {
		_, reset, err := settings.RestoreBackup(filepath.Join(base, "config.yaml"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if reset {
			fmt.Fprintln(os.Stdout, i18n.UI(i18n.OSLang())["credentials_reset"])
		}
		return 0
	}
	crashDir, crashPath, cleanupCrash, crashErr := prepareCrashOutput(base, started)
	if crashErr != nil {
		fmt.Fprintln(os.Stderr, i18n.Render(i18n.OSLang(), i18n.New("console.executable", "error", crashErr.Error())))
		return 1
	}
	defer cleanupCrash()
	st, statErr := os.Stdout.Stat()
	terminal := statErr == nil && st.Mode()&os.ModeCharDevice != 0
	out := outputForMode(mode.gui, os.Stdout)
	errOut := outputForMode(mode.gui, os.Stderr)
	runner := app.PrepareWindow(app.Options{BaseDir: base, ExeDir: exeDir, Stdout: out, Stderr: errOut,
		StdoutIsTerminal: func() bool { return terminal }, InvocationID: os.Getenv("INVOCATION_ID"), CrashDir: crashDir, CrashPath: crashPath, StartedAt: started})
	code := runMode(ctx, stop, mode.gui, hash, runner, cleanupCrash)
	if mode.pause && shouldPause(code, consoleProcessCount) {
		waitEnterLanguage(os.Stdin, os.Stderr, i18n.OSLang())
	}
	return code
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

func outputForMode(gui bool, console io.Writer) io.Writer {
	if gui {
		return io.Discard
	}
	return console
}
