package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigBytesハッシュは読み込んだ版を使う(t *testing.T) {
	base := t.TempDir()
	cfg, generated, raw, err := loadConfigBytes(base)
	if err != nil || !generated || cfg == nil {
		t.Fatalf("初回設定の読込: generated=%v err=%v", generated, err)
	}
	got := sha256.Sum256(raw)
	wantBytes, err := os.ReadFile(filepath.Join(base, configName))
	if err != nil {
		t.Fatal(err)
	}
	if want := sha256.Sum256(wantBytes); got != want {
		t.Fatalf("設定ハッシュが保存した初回設定と異なる: %x != %x", got, want)
	}
}

func TestApply設定読込失敗の遷移(t *testing.T) {
	var stages []string
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, configName), []byte("not: [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newApp(Options{BaseDir: base, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}, appDeps{
		initJob: func() error { return nil }, stage: func(s string) { stages = append(stages, s) },
	})
	if got := a.Apply(); got.State != ApplyIdle {
		t.Fatalf("初期状態 = %+v", got)
	}
	if code := a.run(context.Background()); code != 1 {
		t.Fatalf("終了コード = %d", code)
	}
	got := a.Apply()
	if got.State != ApplyFailed || got.Kind != "" || got.Reason == "" {
		t.Fatalf("失敗状態 = %+v", got)
	}
	if !reflect.DeepEqual(stages, []string{"config.load", "apply.failed"}) {
		t.Fatalf("段 = %v", stages)
	}
}

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
	if n := strings.Count(got, "cannot use child process job object"); n != 1 {
		t.Fatalf("InitJob 警告の回数 = %d, want 1: %s", n, got)
	}
	if !strings.Contains(got, "初期化失敗") {
		t.Fatalf("警告に失敗理由がない: %s", got)
	}
}
