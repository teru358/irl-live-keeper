//go:build linux

// シェルスクリプトの偽の MediaMTX と kill(pid, 0) を使うので、Linux だけで動かす。

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunは戻る前にMediaMTXを止めて回収する(t *testing.T) {
	base := t.TempDir()
	bin := filepath.Join(base, "fake-mediamtx.sh")
	pidFile := filepath.Join(base, "mtx.pid")
	script := fmt.Sprintf("#!/bin/sh\necho $$ > %s.tmp\nmv %s.tmp %s\nexec sleep 30\n", pidFile, pidFile, pidFile)
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	useGenerate(t, bin, nil)

	var stdout, stderr syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runInBackground(ctx, base, &stdout, &stderr)

	var pid int
	waitFor(t, "偽の MediaMTX が PID を書く", func() bool {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		p, err := strconv.Atoi(strings.TrimSpace(string(b)))
		pid = p
		return err == nil
	})
	// テストが失敗しても偽の MediaMTX を残さない
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("終了コード = %d, want 0\nstderr: %s", code, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run が終わらない")
	}
	// run が戻った直後に、子が存在しない。kill(pid, 0) はゾンビにも成功するので、
	// ESRCH なら Wait で回収済みでもある
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("run が戻ったのに MediaMTX（pid %d）が残っている: kill(pid, 0) = %v", pid, err)
	}
}
