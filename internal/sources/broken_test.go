package sources

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/core"
)

func TestReadBrokenTemplateTable(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		count     int
		bad       bool
	}{
		{"valid", "- name: good\n  template: hello\n", 1, false},
		{"template and valid", "- name: broken\n  template: '{link.kbps}'\n- name: good\n  template: hello\n", 2, false},
		{"syntax", "- name: broken\n  template: '{if}'\n", 1, false},
		{"format", "- name: broken\n  template: '{obs.scene:0}'\n", 1, false},
		{"yaml", "[", 0, true},
		{"unknown field", "- name: broken\n  template: '{link.kbps}'\n  extra: true\n", 0, true},
		{"duplicate", "- name: broken\n  template: '{link.kbps}'\n- name: broken\n", 0, true},
		{"style", "- name: broken\n  template: '{link.kbps}'\n  style: {font_size: 513}\n", 0, true},
		{"name", "- name: '../bad'\n  template: '{link.kbps}'\n", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}
			if err := os.WriteFile(s.Path, []byte(tc.raw), 0600); err != nil {
				t.Fatal(err)
			}
			err := s.Transaction(core.Snapshot{}, func(list []Source, err error) ([]Source, bool) {
				if (err != nil) != tc.bad || len(list) != tc.count {
					t.Fatalf("count=%d err=%v", len(list), err)
				}
				return nil, false
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReadAfterCameraIDChange(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}
	raw := "- name: camera\n  template: '{cam2.kbps}'\n- name: good\n  template: hello\n"
	if err := os.WriteFile(s.Path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cam2", "cam3", "", "cam2"} {
		snapshot := core.Snapshot{}
		if id != "" {
			snapshot.Cameras = []core.CameraStatus{{ID: id}}
		}
		if err := s.Transaction(snapshot, func(list []Source, err error) ([]Source, bool) {
			if err != nil || len(list) != 2 {
				t.Fatalf("id=%s list=%v err=%v", id, list, err)
			}
			if (list[0].Broken == nil) != (id == "cam2") || list[1].Broken != nil {
				t.Fatal("incorrect read-derived broken marker", id, list)
			}
			if (list[0].Validate(snapshot) == nil) != (id == "cam2") {
				t.Fatal("wrong camera validation")
			}
			return nil, false
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBrokenLogOnlyOnChange(t *testing.T) {
	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, nil)))
	defer slog.SetDefault(previous)
	s := Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}
	read := func(raw string) {
		t.Helper()
		if err := os.WriteFile(s.Path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if err := s.Transaction(core.Snapshot{}, func([]Source, error) ([]Source, bool) { return nil, false }); err != nil {
			t.Fatal(err)
		}
	}
	broken := "- name: bad\n  template: 'PRIVATE {link.kbps}'\n- name: good\n  template: hello\n"
	read(broken)
	first := log.String()
	if strings.Count(first, "source template needs repair") != 1 || !strings.Contains(first, "name=bad code=template.unknown") {
		t.Fatal(first)
	}
	read(broken)
	read(strings.Replace(broken, "hello", "world", 1))
	if log.String() != first {
		t.Fatal("repeated unchanged warning", log.String())
	}
	read(strings.Replace(broken, "PRIVATE {link.kbps}", "PRIVATE {if}", 1))
	if strings.Count(log.String(), "source template needs repair") != 2 {
		t.Fatal(log.String())
	}
	read("- name: bad\n  template: repaired\n")
	read(broken)
	if strings.Count(log.String(), "source template needs repair") != 3 {
		t.Fatal("not logged after recovery", log.String())
	}
	if strings.Contains(log.String(), "PRIVATE") || strings.Contains(log.String(), "link.kbps") {
		t.Fatal("template leaked", log.String())
	}
}

type brokenLogWriter func([]byte) (int, error)

func (f brokenLogWriter) Write(p []byte) (int, error) { return f(p) }

func TestBrokenLoggingUsesGenerationLoggerOutsideStoreLock(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}
	var first, second bytes.Buffer
	writer := brokenLogWriter(func(p []byte) (int, error) {
		if !s.mu.TryLock() {
			t.Error("store lock held while logging")
		} else {
			s.mu.Unlock()
		}
		return first.Write(p)
	})
	s.SetLogger(slog.New(slog.NewTextHandler(writer, nil)))
	read := func(text string) {
		t.Helper()
		if err := os.WriteFile(s.Path, []byte("- name: bad\n  template: '"+text+"'\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := s.Transaction(core.Snapshot{}, func([]Source, error) ([]Source, bool) { return nil, false }); err != nil {
			t.Fatal(err)
		}
	}
	read("{link.kbps}")
	if strings.Count(first.String(), "source template needs repair") != 1 {
		t.Fatal(first.String())
	}
	s.SetLogger(slog.New(slog.NewTextHandler(&second, nil)))
	read("{link.kbps}")
	if second.Len() != 0 {
		t.Fatal("logger change reset warning history", second.String())
	}
	read("new {link.kbps}")
	if strings.Count(second.String(), "source template needs repair") != 1 {
		t.Fatal("template change did not log", second.String())
	}
}
