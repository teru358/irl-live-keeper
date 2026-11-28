//go:build unix

package sources

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A named pipe must be listed as not regular and must never be opened.
func TestScanDoesNotOpenNamedPipes(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.png"), 0600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	writeFile(t, dir, "ok.png", samplePNG(t))
	s := &FilesStore{Dir: dir}
	done := make(chan map[string]FileInfo, 1)
	go func() { done <- listMap(t, s) }()
	select {
	case m := <-done:
		wantReason(t, m, "pipe.png", ReasonNotRegular)
		wantUsable(t, m, "ok.png")
	case <-time.After(10 * time.Second):
		t.Fatal("scan blocked on a named pipe")
	}
}
