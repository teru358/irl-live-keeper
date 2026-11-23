//go:build windows

package main

import (
	"os"
	"testing"
	"time"
)

func TestCrashOutputCleanup(t *testing.T) {
	_, path, cleanup, err := prepareCrashOutput(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("空の crash log が残った: %v", err)
	}
}
