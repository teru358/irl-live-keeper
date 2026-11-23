//go:build !windows

package winproc

import (
	"os/exec"
	"runtime"
)

func OpenURL(url string) error     { return openWith(url) }
func OpenFolder(path string) error { return openWith(path) }

func openWith(target string) error {
	program := "xdg-open"
	if runtime.GOOS == "darwin" {
		program = "open"
	}
	return exec.Command(program, target).Start()
}
