//go:build windows

package main

import (
	"errors"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// consoleProcessCount は、今のコンソールにつながっているプロセスの数を返す。
func consoleProcessCount() (int, error) {
	var list [8]uint32
	r, _, err := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&list[0])), uintptr(len(list)))
	if r == 0 {
		if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
			err = errors.New("GetConsoleProcessList が 0 を返した")
		}
		return 0, err
	}
	return int(r), nil
}

// installConsoleStop は AllocConsole 後にハンドラを登録する。
// CLOSE のハンドラをすぐ返すと Windows が終了処理中の本体を強制終了する。
func installConsoleStop(gui bool, stop func()) (func(), error) {
	if gui {
		return func() {}, nil
	}
	done := make(chan struct{})
	callback := syscall.NewCallback(func(ctrl uintptr) uintptr {
		switch ctrl {
		case 0, 1, 2:
			stop()
			select {
			case <-done:
			case <-time.After(4 * time.Second):
			}
			return 1
		}
		return 0
	})
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")
	r, _, err := proc.Call(callback, 1)
	if r == 0 {
		return nil, err
	}
	return func() { close(done); proc.Call(callback, 0) }, nil
}
