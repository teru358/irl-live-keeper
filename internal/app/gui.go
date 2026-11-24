package app

import (
	"context"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"net/url"
	"os"
	"path/filepath"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/settings"
)

// PrepareWindow は Job の初期化を窓の作成より前に行う。
func PrepareWindow(opts Options) *App      { return newApp(opts, appDeps{}) }
func (a *App) Run(ctx context.Context) int { return a.runApp(ctx) }
func (a *App) RunWindow(ctx context.Context) int {
	for {
		code := a.runApp(ctx)
		if ctx.Err() != nil {
			return code
		}
		a.mu.Lock()
		a.running = true
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			return 0
		case <-a.restarts:
		}
	}
}

type WindowState struct {
	PasswordUnset bool
	Snapshot      *core.Snapshot
	Apply         Apply
	Language      i18n.Lang
	CloseAction   string
	LogDir        string
	Generated     bool
	Restore       bool
}

func (a *App) WindowState() WindowState {
	a.mu.Lock()
	s := WindowState{PasswordUnset: a.windowPasswordUnset, Apply: a.apply, Language: a.windowLang, CloseAction: a.windowClose, LogDir: a.windowLog, Generated: a.windowGenerated}
	if s.Language == "" {
		s.Language = i18n.OSLang()
	}
	if s.LogDir == "" {
		s.LogDir = filepath.Join(a.opts.BaseDir, "logs")
	}
	if a.current != nil {
		v := a.current.core.Snapshot()
		s.Snapshot = &v
	}
	a.mu.Unlock()
	if s.Apply.State == ApplyFailed {
		bak, e1 := os.ReadFile(filepath.Join(a.opts.BaseDir, configName+".bak"))
		raw, e2 := os.ReadFile(filepath.Join(a.opts.BaseDir, configName))
		if e1 == nil && e2 == nil && settings.Hash(bak) != settings.Hash(raw) {
			if _, e1 = config.Parse(bak); e1 == nil {
				s.Restore = true
			}
		}
	}
	return s
}
func OnceURL(port int, token string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/auth/once?t=%s", port, url.QueryEscape(token))
}
func (a *App) WindowURL() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current == nil {
		return "", ErrNotRunning
	}
	t, err := a.auth.NewOnce()
	if err != nil {
		return "", err
	}
	return OnceURL(a.windowPort, t), nil
}

// SecondInstanceBlocked は既存の本体のログに重複起動を残す。
func (a *App) SecondInstanceBlocked() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current != nil {
		a.current.logger.Slog().Info("second instance blocked")
	}
}

// Retry は窓の再試行を記録してから、通常の再起動手順へ渡す。
func (a *App) Retry() error {
	a.windowLogMessage("retry requested")
	return a.Restart()
}

// RestoreBackup は、失敗した config.yaml を bak（前に世代を作れた版）と入れ替えて、起動し直す（窓の「元に戻して再起動」）。
func (a *App) RestoreBackup() error {
	a.windowLogMessage("restore backup requested")
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.apply.State != ApplyFailed || (a.apply.Kind != "" && a.apply.Kind != ApplyRestart) {
		return ErrNotRunning
	}
	path := filepath.Join(a.opts.BaseDir, configName)
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		return err
	}
	if _, err = config.Parse(bak); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err == nil && settings.Hash(raw) == settings.Hash(bak) {
		return os.ErrInvalid
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err = settings.WriteFile(path+".bak", raw); err != nil {
			return err
		}
	}
	if err = settings.WriteFile(path, bak); err != nil {
		return err
	}
	a.apply = Apply{State: ApplyApplying, Kind: ApplyRevert}
	// lock を持ったまま送るので待たない（失敗の状態では再起動は積まれていない。積まれていれば、それが元に戻した版を読む）
	select {
	case a.restarts <- struct{}{}:
	default:
	}
	return nil
}

func (a *App) windowLogMessage(message string, args ...any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current != nil {
		a.current.logger.Slog().Info(message, args...)
	} else if a.deps.openLogger != nil {
		dir := a.windowLog
		if dir == "" {
			dir = filepath.Join(a.opts.BaseDir, "logs")
		}
		if l, err := a.deps.openLogger(logging.Options{Dir: dir, KeepDays: 7, Stdout: a.opts.Stdout, Secrets: a.secrets}); err == nil {
			if message == "restart failed" {
				l.Slog().Error(message, args...)
			} else {
				l.Slog().Info(message, args...)
			}
			_ = l.Close()
		}
	}
}
