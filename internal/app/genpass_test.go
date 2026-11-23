package app

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const passwordLabel = "管理 API のパスワード"

// config.yaml を生成したあと、起動の表示より前に失敗して終わるときは、生成したパスワードを標準出力に出す。
// ログファイルには書かない。
func TestRunは生成後にポートの確認で失敗してもパスワードを表示する(t *testing.T) {
	base := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	busy := l.Addr().(*net.TCPAddr).Port
	cfgOf := useGenerate(t, filepath.Join(base, "none"), map[string]int{"rtmp_port": busy})

	var stdout, stderr syncBuffer
	if code := run(context.Background(), base, &stdout, &stderr); code != 1 {
		t.Fatalf("終了コード = %d, want 1\nstderr: %s", code, stderr.String())
	}
	pass := cfgOf().Admin.Password
	if pass == "" {
		t.Fatal("生成した設定を受け取れていない")
	}
	if !strings.Contains(stdout.String(), passwordLabel+"（config.yaml を生成しました）: "+pass) {
		t.Errorf("標準出力に生成したパスワードがない:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "srt://") {
		t.Error("起動を止めたのに起動の表示が出ている")
	}
	if got := readLogs(t, filepath.Join(base, "logs")); strings.Contains(got, pass) {
		t.Errorf("ログファイルにパスワードが書かれた:\n%s", got)
	}

	// 2回目は config.yaml がある（生成していない）ので、パスワードを出さない
	var stdout2, stderr2 syncBuffer
	if code := run(context.Background(), base, &stdout2, &stderr2); code != 1 {
		t.Fatalf("2回目の終了コード = %d, want 1", code)
	}
	if strings.Contains(stdout2.String(), passwordLabel) || strings.Contains(stdout2.String(), pass) {
		t.Errorf("生成していないのにパスワードを出した:\n%s", stdout2.String())
	}
}

// ログを開けずに終わるときも、生成したパスワードを標準出力に出す。
func TestRunは生成後にログを開けなくてもパスワードを表示する(t *testing.T) {
	base := t.TempDir()
	// ログのディレクトリの場所にファイルを置いて、開けなくする
	if err := os.WriteFile(filepath.Join(base, "logs"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgOf := useGenerate(t, filepath.Join(base, "none"), nil)

	var stdout, stderr syncBuffer
	if code := run(context.Background(), base, &stdout, &stderr); code != 1 {
		t.Fatalf("終了コード = %d, want 1\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "ログを開けません") {
		t.Fatalf("ログを開けない失敗になっていない:\n%s", stderr.String())
	}
	if pass := cfgOf().Admin.Password; pass == "" || !strings.Contains(stdout.String(), passwordLabel+"（config.yaml を生成しました）: "+pass) {
		t.Errorf("標準出力に生成したパスワードがない:\n%s", stdout.String())
	}
}
