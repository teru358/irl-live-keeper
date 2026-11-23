//go:build windows

package winproc

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestOpenUsesBreakawayFlag(t *testing.T) {
	if got, want := openCreationFlags(), uint32(windows.CREATE_BREAKAWAY_FROM_JOB); got != want {
		t.Fatalf("open creation flags = %#x, want %#x", got, want)
	}
}
