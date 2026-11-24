package app

import (
	"github.com/teru358/irl-live-keeper/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitialPasswordPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configName)
	if _, err := config.Generate(path); err != nil {
		t.Fatal(err)
	}
	a := &App{opts: Options{BaseDir: dir}}
	b := &pageBackend{app: a}
	before, _ := os.ReadFile(path)
	if err := b.SetInitialPassword("password123"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	cfg, err := config.Load(path)
	if err != nil || cfg.Admin.Password != "password123" || a.adminPassword != "password123" {
		t.Fatal("password not updated", err)
	}
	for _, line := range strings.Split(string(before), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") && !strings.Contains(string(after), line) {
			t.Fatal("comment lost")
		}
	}
	if err := b.SetInitialPassword("different123"); err == nil {
		t.Fatal("overwrite accepted")
	}
}
