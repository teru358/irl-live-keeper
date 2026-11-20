//go:build windows

package main

import (
	"errors"
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
