//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/gui"
	"github.com/teru358/irl-live-keeper/internal/single"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

var kernel = windows.NewLazySystemDLL("kernel32.dll")
var user = windows.NewLazySystemDLL("user32.dll")

func prepareMode(cli bool) (executionMode, error) {
	h, _, _ := kernel.NewProc("GetConsoleWindow").Call()
	mode := executionMode{gui: h == 0 && !cli, pause: h != 0 && !cli}
	if cli {
		if h == 0 {
			r, _, err := kernel.NewProc("AllocConsole").Call()
			if r == 0 {
				return mode, err
			}
		}
		r, _, err := kernel.NewProc("SetConsoleOutputCP").Call(65001)
		if r == 0 {
			return mode, err
		}
		var errOpen error
		os.Stdin, errOpen = os.OpenFile("CONIN$", os.O_RDONLY, 0)
		if errOpen != nil {
			return mode, errOpen
		}
		os.Stdout, errOpen = os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if errOpen != nil {
			return mode, errOpen
		}
		os.Stderr = os.Stdout
	}
	return mode, nil
}
func acquireInstance(base string, graphical bool) (func(), string, bool, error) {
	hash, err := single.DirectoryHash(base)
	if err != nil {
		return func() {}, "", false, err
	}
	name, _ := windows.UTF16PtrFromString(single.MutexName(hash))
	h, err := windows.CreateMutex(nil, false, name)
	duplicate := errors.Is(err, windows.ERROR_ALREADY_EXISTS)
	if err != nil && !duplicate {
		return func() {}, hash, false, err
	}
	cleanup := func() { windows.CloseHandle(h) }
	if duplicate {
		class, _ := windows.UTF16PtrFromString(single.ClassName(hash))
		wnd, _, _ := user.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), 0)
		if wnd != 0 && graphical {
			user.NewProc("ShowWindow").Call(wnd, 9)
			user.NewProc("SetForegroundWindow").Call(wnd)
		} else if !graphical {
			fmt.Fprintln(os.Stdout, "irl-live-keeper: already running for this directory")
		}
	}
	return cleanup, hash, duplicate, nil
}
func runMode(ctx context.Context, stop context.CancelFunc, graphical bool, hash string, run func(context.Context) int) int {
	if !graphical {
		return run(ctx)
	}
	return gui.Run(ctx, stop, single.ClassName(hash), run)
}
