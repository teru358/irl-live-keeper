package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCrashLogPathAndRemoveEmpty(t *testing.T) {
	dir := t.TempDir()
	started := time.Date(2026, 11, 23, 9, 8, 7, 0, time.FixedZone("JST", 9*60*60))
	path := CrashLogPath(dir, started)
	if got, want := filepath.Base(path), "crash-20261123-090807.log"; got != want {
		t.Fatalf("CrashLogPath basename = %q, want %q", got, want)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEmptyCrashLog(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("empty crash log remains: %v", err)
	}
	if err := os.WriteFile(path, []byte("panic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEmptyCrashLog(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "panic" {
		t.Fatalf("nonempty crash log = %q, %v", got, err)
	}
}
