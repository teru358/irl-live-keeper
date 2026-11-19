//go:build windows

package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const lockThread = false

var (
	jobMu sync.Mutex
	// jobHandle は閉じない。本体自身が Job に入っているので、閉じると本体も終了する。
	// CreateJobObject にセキュリティ属性を渡さないので、子には継承されない。
	jobHandle windows.Handle
	// selfInJob が true なら、子は生成時に Job を継承する。
	selfInJob bool
)

// InitJob は KILL_ON_JOB_CLOSE の Job Object を作り、本体自身をその Job に入れる。
// 起動時に1回だけ呼ぶ。Job は作れたが本体を入れられなかったとき（入れ子にできない Job の中など）は、
// エラーを返したうえで Job を残し、以後は子を Start した直後に Job へ割り当てる方式に切り替える。
func InitJob() error {
	jobMu.Lock()
	defer jobMu.Unlock()
	if jobHandle != 0 {
		return nil
	}
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("Job Object を作れません: %w", err)
	}
	// breakaway は許可しない（BREAKAWAY_OK / SILENT_BREAKAWAY_OK を立てない）。
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return fmt.Errorf("Job Object を設定できません: %w", err)
	}
	jobHandle = h
	if err := windows.AssignProcessToJobObject(h, windows.CurrentProcess()); err != nil {
		return fmt.Errorf("本体を Job に入れられません（子の起動直後に割り当てる方式に切り替えます）: %w", err)
	}
	selfInJob = true
	return nil
}

func prepare(cmd *exec.Cmd) {}

// afterStart は、本体を Job に入れられなかったときだけ、起動した子を Job に割り当てる。
func afterStart(cmd *exec.Cmd) {
	jobMu.Lock()
	h, self := jobHandle, selfInJob
	jobMu.Unlock()
	if h == 0 || self || cmd.Process == nil {
		return
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(ph)
	_ = windows.AssignProcessToJobObject(h, ph)
}

// interrupt は Windows には SIGTERM がないので、強制終了する。
func interrupt(p *os.Process) error { return p.Kill() }
