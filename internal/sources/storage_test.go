package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageValidationAndRecovery(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sources.yaml")
	store := Store{Path: p}
	for _, bad := range []string{"[", "- name: demo\n  extra: true\n", "- name: demo\n- name: demo\n", "- name: demo\n  obs_width: 0\n", "- name: demo\n  template: '{unknown}'\n", "[]\n---\n[]\n"} {
		if e := os.WriteFile(p, []byte(bad), 0600); e != nil {
			t.Fatal(e)
		}
		e := store.Transaction(core.Snapshot{}, func(list []Source, err error) ([]Source, bool) {
			if err == nil || len(list) != 0 {
				t.Fatalf("accepted %q", bad)
			}
			return []Source{}, true
		})
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := os.ReadFile(p)
		if string(raw) != bad {
			t.Fatal("overwrote invalid file")
		}
	}
	if e := os.WriteFile(p, []byte("- name: demo\n"), 0600); e != nil {
		t.Fatal(e)
	}
	e := store.Transaction(core.Snapshot{}, func(list []Source, err error) ([]Source, bool) {
		if err != nil || len(list) != 1 || list[0].OBSWidth != 800 {
			t.Fatalf("reload: %v %v", list, err)
		}
		list[0].CSS = "body {content: '</style>'}"
		return list, true
	})
	if e != nil {
		t.Fatal(e)
	}
	e = store.Transaction(core.Snapshot{}, func(list []Source, err error) ([]Source, bool) {
		if err != nil || list[0].CSS != "body {content: '</style>'}" {
			t.Fatal("round trip failed")
		}
		return nil, false
	})
	if e != nil {
		t.Fatal(e)
	}
}
