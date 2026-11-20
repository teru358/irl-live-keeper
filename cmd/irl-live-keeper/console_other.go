//go:build !windows

package main

import "errors"

// consoleProcessCount は Windows 以外では使わない。常に待たないよう、エラーを返す。
func consoleProcessCount() (int, error) {
	return 0, errors.New("Windows 以外ではコンソールのプロセスを数えない")
}
