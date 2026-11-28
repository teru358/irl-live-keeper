package main

import (
	"bytes"
	"io"
	"testing"
)

func TestCLIArguments(t *testing.T) {
	for _, args := range [][]string{{"--cli", "--dir", "C:\\data"}, {"--dir", "C:\\data", "--cli"}} {
		a, err := parseArgs(args)
		if err != nil || !a.cli || a.dir != "C:\\data" {
			t.Fatalf("%+v: %v", a, err)
		}
	}
	a, err := parseArgs(nil)
	if err != nil || a.cli {
		t.Fatalf("default: %+v %v", a, err)
	}
	if _, err := parseArgs([]string{"--unknown"}); err == nil {
		t.Fatal("unknown flag")
	}
}
func TestGUIOutput(t *testing.T) {
	console := &bytes.Buffer{}
	if outputForMode(true, nil) != io.Discard {
		t.Fatal("GUI output")
	}
	if outputForMode(false, console) != console {
		t.Fatal("console output")
	}
}

func TestRestoreBackupArguments(t *testing.T) {
	for _, args := range [][]string{{"--cli", "restore-backup", "--dir", "/tmp/keeper"}, {"--dir", "/tmp/keeper", "--cli", "restore-backup"}} {
		got, err := parseArgs(args)
		if err != nil || !got.restore || !got.cli || got.dir != "/tmp/keeper" {
			t.Fatal("restore command arguments rejected")
		}
	}
}
