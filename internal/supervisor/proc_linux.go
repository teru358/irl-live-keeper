//go:build linux

package supervisor

import (
	"os"
	"os/exec"
	"syscall"
)

// Go の Pdeathsig は、子を fork した OS スレッドが終わった時点で発火する
// （go.dev/issue/27505）。そのため spawn は起動から Wait までを、
// LockOSThread した専用の goroutine 1本で行う。
const lockThread = true

// prepare は本体が死んだときに子へ SIGKILL が届くようにする。
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}

func afterStart(cmd *exec.Cmd) {}

// interrupt は子に終了を求める。
func interrupt(p *os.Process) error { return p.Signal(syscall.SIGTERM) }

// InitJob は Windows 専用の処理なので、Linux では何もしない。
func InitJob() error { return nil }
