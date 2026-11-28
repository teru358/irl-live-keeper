package settings

import (
	"github.com/teru358/irl-live-keeper/internal/config"
	"os"
	"path/filepath"
	"testing"
)

// 置き換えの途中で失敗しても、戻せる版が少なくとも一つ残る。
func TestRestoreBackupKeepsRecoverableVersion(t *testing.T) {
	setup := func(t *testing.T) (path string, bak, cur []byte) {
		t.Helper()
		path = filepath.Join(t.TempDir(), "config.yaml")
		old, err := config.Generate(path)
		if err != nil {
			t.Fatal(err)
		}
		bak = mustConfig(t, old)
		next := *old
		next.Admin.Port++ // 読めるが起動できない版（ポートが違う）を想定
		cur = mustConfig(t, &next)
		if err = os.WriteFile(path+".bak", bak, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, cur, 0600); err != nil {
			t.Fatal(err)
		}
		return path, bak, cur
	}
	failOn := func(target string) func(string, string) error {
		return func(from, to string) error {
			if to == target {
				return os.ErrPermission
			}
			return os.Rename(from, to)
		}
	}
	read := func(path string) string {
		b, _ := os.ReadFile(path)
		return string(b)
	}
	t.Run("config.yaml の置き換えに失敗", func(t *testing.T) {
		path, bak, cur := setup(t)
		if _, _, err := restoreBackup(path, failOn(path)); err == nil {
			t.Fatal("失敗が返らない")
		}
		if read(path+".bak") != string(bak) {
			t.Fatal("bak が書き換わった")
		}
		if read(path) != string(cur) {
			t.Fatal("config.yaml が書き換わった")
		}
	})
	t.Run("bak の入れ替えに失敗", func(t *testing.T) {
		path, bak, _ := setup(t)
		if _, _, err := restoreBackup(path, failOn(path+".bak")); err != nil {
			t.Fatal(err)
		}
		if read(path+".bak") != string(bak) {
			t.Fatal("bak が失われた")
		}
		if _, err := config.Parse([]byte(read(path))); err != nil {
			t.Fatal("戻した config.yaml が読めない", err)
		}
	})
	t.Run("成功", func(t *testing.T) {
		path, _, cur := setup(t)
		if _, _, err := restoreBackup(path, os.Rename); err != nil {
			t.Fatal(err)
		}
		if read(path+".bak") != string(cur) {
			t.Fatal("置き換えた版が bak にない")
		}
	})
}
