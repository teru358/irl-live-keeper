package logging

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPruneCrashLogsTable(t *testing.T) {
	today := at(2026, 11, 20, 12, 0, 0)
	cases := []struct {
		name      string
		stamp     string
		empty     bool
		keepKnown bool
		wantGone  bool
	}{
		{"empty old known", "20261105-120000", true, true, true},
		{"nonempty old known", "20261105-120000", false, true, true},
		{"empty boundary known", "20261106-120000", true, true, true},
		{"nonempty boundary known", "20261106-120000", false, true, false},
		{"empty new known", "20261107-120000", true, true, true},
		{"nonempty new known", "20261107-120000", false, true, false},
		{"empty old unknown", "20261105-120000", true, false, true},
		{"nonempty old unknown", "20261105-120000", false, false, false},
		{"empty boundary unknown", "20261106-120000", true, false, true},
		{"nonempty boundary unknown", "20261106-120000", false, false, false},
		{"empty new unknown", "20261107-120000", true, false, true},
		{"nonempty new unknown", "20261107-120000", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			name := "crash-" + tc.stamp + ".log"
			content := []byte("nonempty")
			if tc.empty {
				content = nil
			}
			if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := PruneCrashLogs(dir, "", 14, tc.keepKnown, today); err != nil {
				t.Fatalf("PruneCrashLogs: %v", err)
			}
			_, err := os.Stat(filepath.Join(dir, name))
			gone := os.IsNotExist(err)
			if gone != tc.wantGone {
				t.Errorf("gone = %v, want %v (err=%v)", gone, tc.wantGone, err)
			}
		})
	}
}

func TestPruneCrashLogsPreservesCurrentInvalidAndNonRegular(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"crash-20261105-120000.log",
		"crash-2026.log", "crash-20261340-000000.log", "crash-20261105-250000.log",
		"crash-2026115-120000.log", "xcrash-20261105-120000.log",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "crash-20261104-120000.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "crash-20261103-120000.log")
	if err := os.Symlink(filepath.Join(dir, names[0]), link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if err := PruneCrashLogs(dir, names[0], 14, true, at(2026, 11, 20, 12, 0, 0)); err != nil {
		t.Fatalf("PruneCrashLogs: %v", err)
	}
	for _, name := range append(names, "crash-20261104-120000.log", "crash-20261103-120000.log") {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was touched: %v", name, err)
		}
	}
}

func TestPruneCrashLogsRemovesEmptyCurrentException(t *testing.T) {
	dir := t.TempDir()
	name := "crash-20261105-120000.log"
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PruneCrashLogs(dir, name, 14, true, at(2026, 11, 20, 12, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatalf("current crash log was removed: %v", err)
	}
}

func TestPruneCrashLogsJoinsDeleteErrorsAndContinues(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("この環境ではディレクトリ権限で削除を拒否できない")
	}
	dir := t.TempDir()
	names := []string{"crash-20261105-120000.log", "crash-20261104-120000.log"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	err := PruneCrashLogs(dir, "", 14, true, at(2026, 11, 20, 12, 0, 0))
	if err == nil {
		t.Fatal("削除エラーが返らなかった")
	}
	for _, name := range names {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("結合されたエラーに %q がない: %v", name, err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
			t.Errorf("削除に失敗したファイルがなくなった: %s: %v", name, statErr)
		}
	}
}
