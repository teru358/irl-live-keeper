package sources

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitBarrier(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("operation deadlocked")
	}
}

func TestFileUpdatesBarrier(t *testing.T) {
	ops := []string{"Refresh", "Rebuild", "OpenWithHash", "Save", "Delete"}
	for _, first := range ops {
		for _, second := range ops {
			t.Run(first+"/"+second, func(t *testing.T) {
				s := &FilesStore{Dir: t.TempDir()}
				data := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
				if err := s.Save("a.svg", data); err != nil {
					t.Fatal(err)
				}
				run := func(op string) {
					switch op {
					case "Refresh":
						s.Refresh([]string{AssetPrefix + "a.svg"})
					case "Rebuild":
						_ = s.Rebuild()
					case "OpenWithHash":
						_, _, _, _ = s.OpenWithHash("a.svg")
					case "Save":
						_ = s.Save("b.svg", data)
					case "Delete":
						_ = s.Delete("a.svg")
					}
				}
				entered, release, done1, started, done2 := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
				blocked := false
				filesOperationHook = func(op string) {
					if op == first && !blocked {
						blocked = true
						close(entered)
						<-release
					}
				}
				go func() { run(first); close(done1) }()
				waitBarrier(t, entered)
				// Change the disk while the first owner is paused; none of the
				// queued operations may publish the remembered usable contents.
				if err := os.WriteFile(filepath.Join(s.Dir, "a.svg"), []byte("corrupt"), 0600); err != nil {
					t.Fatal(err)
				}

				go func() { close(started); run(second); close(done2) }()
				waitBarrier(t, started)
				// The first operation owns filesMu. A competing update must wait here.
				select {
				case <-done2:
					t.Error("competing operation escaped serialization")
				default:
				}
				close(release)
				waitBarrier(t, done1)
				waitBarrier(t, done2)
				filesOperationHook = nil
				if _, _, _, err := s.OpenWithHash("a.svg"); err == nil {
					t.Fatal("old usable contents returned after invalidation")
				}

				if first == "Delete" || second == "Delete" {
					if _, _, _, err := s.OpenWithHash("a.svg"); err == nil {
						t.Fatal("deleted result became usable again")
					}
				}
				if first == "Save" || second == "Save" {
					if _, _, _, err := s.OpenWithHash("b.svg"); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestPartialReadDirFailsClosed(t *testing.T) {
	s := &FilesStore{Dir: t.TempDir()}
	if err := os.Mkdir(filepath.Join(s.Dir, "親"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "親", "夕.svg"), []byte(`<svg/>`), 0600); err != nil {
		t.Fatal(err)
	}
	original := readIndexDir
	defer func() { readIndexDir = original }()
	for _, phase := range []string{"rebuild", "refresh"} {
		t.Run(phase, func(t *testing.T) {
			readIndexDir = original
			if err := s.Rebuild(); err != nil {
				t.Fatal(err)
			}
			readIndexDir = func(f *os.File, n int) ([]fs.DirEntry, error) {
				des, err := original(f, n)
				if filepath.Base(f.Name()) == "親" {
					return des, errors.New("partial directory read")
				}
				return des, err
			}
			if phase == "rebuild" {
				if err := s.Rebuild(); err != nil {
					t.Fatal(err)
				}
			} else {
				s.Refresh([]string{AssetPrefix + "親/夕.svg"})
			}
			if _, _, _, err := s.OpenWithHash("親/夕.svg"); err == nil {
				t.Fatal("served child of unconfirmed parent")
			}
		})
	}
}

// Pause after ReadDir returned the parent's snapshot, while filesMu is held.
func TestRefreshParentReadRebuildBarrier(t *testing.T) {
	for _, first := range []string{"Refresh", "Rebuild"} {
		t.Run(first, func(t *testing.T) {
			s := &FilesStore{Dir: t.TempDir()}
			if err := s.Save("A.svg", []byte("<svg/>")); err != nil {
				t.Fatal(err)
			}
			original := readIndexDir
			entered, release, done1, done2 := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			blocked := false
			readIndexDir = func(f *os.File, n int) ([]fs.DirEntry, error) {
				des, err := original(f, n)
				if !blocked {
					blocked = true
					close(entered)
					<-release
				}
				return des, err
			}
			defer func() { readIndexDir = original }()
			run := func(op string) {
				if op == "Refresh" {
					s.Refresh([]string{AssetPrefix + "A.svg"})
				} else {
					_ = s.Rebuild()
				}
			}
			go func() { run(first); close(done1) }()
			waitBarrier(t, entered)
			if err := os.WriteFile(filepath.Join(s.Dir, "a.svg"), []byte("<svg/>"), 0600); err != nil {
				t.Fatal(err)
			}
			second := "Rebuild"
			if first == "Rebuild" {
				second = "Refresh"
			}
			started := make(chan struct{})
			go func() { close(started); run(second); close(done2) }()
			waitBarrier(t, started)
			select {
			case <-done2:
				t.Error("update escaped filesMu")
			default:
			}
			close(release)
			waitBarrier(t, done1)
			waitBarrier(t, done2)
			for _, name := range []string{"A.svg", "a.svg"} {
				if _, _, _, err := s.OpenWithHash(name); err == nil {
					t.Fatalf("served collision %s", name)
				}
			}
			if len(s.Available()) != 0 {
				t.Fatal("collision advertised")
			}
		})
	}
}
