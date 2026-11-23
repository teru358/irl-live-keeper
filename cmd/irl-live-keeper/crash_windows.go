//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/teru358/irl-live-keeper/internal/logging"
)

func prepareCrashOutput(base string, started time.Time) (dir, path string, cleanup func(), err error) {
	dir = filepath.Join(base, "logs")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	path = logging.CrashLogPath(dir, started)
	if err = logging.PruneCrashLogs(dir, path, 0, false, started); err != nil {
		return
	}
	var file *os.File
	file, err = os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	if err = debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		file.Close()
		return
	}
	cleanup = func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Fprintln(os.Stderr, closeErr)
		}
		if removeErr := logging.RemoveEmptyCrashLog(path); removeErr != nil {
			fmt.Fprintln(os.Stderr, removeErr)
		}
	}
	return
}
