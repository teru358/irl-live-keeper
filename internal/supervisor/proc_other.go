//go:build !linux && !windows

package supervisor

import (
	"os"
	"os/exec"
	"syscall"
)

// macOS などには Pdeathsig に相当する仕組みがないため、正常終了時の停止だけとする（既知の制約）。
const lockThread = false

func prepare(cmd *exec.Cmd) {}

func afterStart(cmd *exec.Cmd) {}

func interrupt(p *os.Process) error { return p.Signal(syscall.SIGTERM) }

// InitJob は Windows 専用の処理なので、ここでは何もしない。
func InitJob() error { return nil }
