//go:build windows

package winproc

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func OpenURL(url string) error {
	exe, args := URLCommand(url)
	return startBreakaway(exe, args...)
}

func OpenFolder(path string) error {
	exe, args := FolderCommand(path)
	return startBreakaway(exe, args...)
}

func startBreakaway(exe string, args ...string) error {
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: openCreationFlags(),
		CmdLine:       WindowsCommandLine(args...),
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func openCreationFlags() uint32 { return windows.CREATE_BREAKAWAY_FROM_JOB }
