//go:build windows

package supervisor

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsProcessFlags(t *testing.T) {
	if got, want := mediaCreationFlags(), uint32(windows.CREATE_NEW_PROCESS_GROUP|windows.CREATE_NO_WINDOW); got != want {
		t.Fatalf("MediaMTX creation flags = %#x, want %#x", got, want)
	}
	if got, want := jobLimitFlags(), uint32(windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE|windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK); got != want {
		t.Fatalf("Job limit flags = %#x, want %#x", got, want)
	}
}
