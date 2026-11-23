// Command irl-live-keeper は、屋外配信の受信状態を監視し、OBS のシーンを自動で切り替える。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/teru358/irl-live-keeper/internal/app"
	"github.com/teru358/irl-live-keeper/internal/i18n"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "service" {
		os.Exit(runService(os.Args[2:], os.Stdout, os.Stderr))
	}
	dataDir := flag.String("dir", "", "folder for config.yaml, logs and data (default: the executable's folder)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	base, err := baseDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.Render(i18n.OSLang(), i18n.New("console.executable", "error", err.Error())))
		os.Exit(1)
	}
	exeDir := base
	if *dataDir != "" {
		base, err = filepath.Abs(*dataDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.Render(i18n.OSLang(), i18n.New("console.executable", "error", err.Error())))
			os.Exit(1)
		}
	}
	st, statErr := os.Stdout.Stat()
	terminal := statErr == nil && st.Mode()&os.ModeCharDevice != 0
	code := app.Run(ctx, app.Options{BaseDir: base, ExeDir: exeDir, Stdout: os.Stdout, Stderr: os.Stderr,
		StdoutIsTerminal: func() bool { return terminal }, InvocationID: os.Getenv("INVOCATION_ID")})
	if shouldPause(code, consoleProcessCount) {
		waitEnterLanguage(os.Stdin, os.Stderr, i18n.OSLang())
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
