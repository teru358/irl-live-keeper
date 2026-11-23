package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestApp準備でInitJobを一度だけ呼ぶ(t *testing.T) {
	calls := 0
	wantErr := errors.New("初期化失敗")
	a := newApp(Options{BaseDir: t.TempDir(), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, appDeps{
		initJob: func() error {
			calls++
			return wantErr
		},
	})
	if calls != 1 {
		t.Fatalf("InitJob 呼び出し回数 = %d, want 1", calls)
	}
	if a.initErr != wantErr {
		t.Fatalf("InitJob の失敗が保存されていない: %v", a.initErr)
	}
	// 不正な設定でも App 準備は先に済み、世代作成の前に再実行しない。
	if code := a.run(context.Background()); code != 1 {
		t.Fatalf("不正な設定の終了コード = %d, want 1", code)
	}
	if calls != 1 {
		t.Fatalf("run 後の InitJob 呼び出し回数 = %d, want 1", calls)
	}
}

func TestInitJob失敗はloggerへ一度だけ警告する(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, nil))
	logInitJobWarning(log, errors.New("初期化失敗"))
	got := out.String()
	if n := strings.Count(got, "子プロセスをまとめて終了させる仕組みを使えません"); n != 1 {
		t.Fatalf("InitJob 警告の回数 = %d, want 1: %s", n, got)
	}
	if !strings.Contains(got, "初期化失敗") {
		t.Fatalf("警告に失敗理由がない: %s", got)
	}
}
