package app

import (
	"os"
	"testing"
)

// このパッケージのテストは、コンソールの日本語の文を期待しているものが多い。config の language は
// 既定の auto で、OS の言語（Linux と macOS は LC_ALL などの環境変数）で決まるので、回すマシンの言語に
// 左右されないよう、日本語に固定する。auto の決め方そのものは i18n のテストで表にして確かめる。
func TestMain(m *testing.M) {
	os.Setenv("LC_ALL", "ja_JP.UTF-8")
	os.Exit(m.Run())
}
