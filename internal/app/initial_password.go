package app

import (
	"crypto/sha256"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"os"
	"path/filepath"
)

func (b *pageBackend) SetInitialPassword(password string) error {
	a := b.app
	a.mu.Lock()
	defer a.mu.Unlock()
	path := filepath.Join(a.opts.BaseDir, configName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("password setup failed")
	}
	cfg, err := config.Parse(raw)
	if err != nil || cfg.Admin.Password != "" || a.adminPassword != "" {
		return errors.New("password setup unavailable")
	}
	out, err := settings.Rewrite(raw, []settings.Change{{Path: "admin.password", Value: password}})
	if err != nil {
		return err
	}
	if err = settings.WriteFile(path, out); err != nil {
		return errors.New("password setup failed")
	}
	a.hash = sha256.Sum256(out)
	a.adminPassword = password
	a.windowPasswordUnset = false
	return nil
}
