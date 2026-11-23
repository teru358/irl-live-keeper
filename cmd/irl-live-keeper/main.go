// Command irl-live-keeper は、屋外配信の受信状態を監視し、OBS のシーンを自動で切り替える。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/teru358/irl-live-keeper/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	base, err := baseDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "実行ファイルの場所を調べられません:", err)
		os.Exit(1)
	}
	code := app.Run(ctx, app.Options{BaseDir: base, Stdout: os.Stdout, Stderr: os.Stderr})
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
