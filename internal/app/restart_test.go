package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"gopkg.in/yaml.v3"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRestart受付と失敗(t *testing.T) {
	f := newFailureFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	f.stop = func(context.Context) { close(entered); <-release }
	done := make(chan int, 1)
	go func() { done <- f.a.run(ctx) }()
	<-f.h.started
	// 要求の終了まで Shutdown を待たせ、Restart 自体が待たないことを確かめる。
	requestDone := make(chan struct{})
	f.h.shutdown = func(context.Context) error { <-requestDone; close(f.h.done); return nil }
	waitApplyState(t, f.a, ApplyApplied)
	if err := f.a.Restart(); err != nil {
		t.Fatal(err)
	}
	<-entered
	if v := f.a.Apply(); v.State != ApplyApplying || v.Kind != ApplyRestart {
		t.Fatal(v)
	}
	if !errors.Is(f.a.Restart(), ErrApplying) {
		t.Fatal("重複が通った")
	}
	f.a.deps.loadConfig = loadConfigBytes
	// 次の読込を失敗させる。
	if err := removeRestartConfig(f.a.opts.BaseDir); err != nil {
		t.Fatal(err)
	}
	close(requestDone)
	close(release)
	select {
	case code := <-done:
		if code != 1 {
			t.Fatal(code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("終了が詰まった")
	}
	if v := f.a.Apply(); v.State != ApplyFailed || v.Kind != ApplyRestart || v.Reason == "" {
		t.Fatal(v)
	}
	if f.stderr.Len() == 0 || f.a.current != nil {
		t.Fatal("失敗の回収または表示がない")
	}
}

func removeRestartConfig(base string) error {
	return os.WriteFile(filepath.Join(base, configName), []byte("not: [valid"), 0600)
}

func waitApplyState(t *testing.T, a *App, state ApplyState) Apply {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		v := a.Apply()
		if v.State == state {
			return v
		}
		select {
		case <-deadline.C:
			t.Fatalf("Apply が %s にならなかった: %+v", state, v)
		case <-tick.C:
		}
	}
}

func TestApp秘密は再起動を重ねても集合のまま(t *testing.T) {
	a := newApp(Options{}, appDeps{initJob: func() error { return nil }})
	for i := 0; i < 3; i++ {
		a.rememberSecrets("admin-secret", "obs-secret", "api-secret", "", "obs-secret")
	}
	if want := []string{"admin-secret", "obs-secret", "api-secret"}; !reflect.DeepEqual(a.secrets, want) {
		t.Fatalf("秘密集合=%v, want %v", a.secrets, want)
	}
}

func TestRestart読込失敗で前世代の秘密を伏せる(t *testing.T) {
	f := newFailureFixture(t)
	loads := 0
	f.a.deps.loadConfig = func(base string) (*config.Config, bool, []byte, error) {
		loads++
		cfg, generated, raw, err := loadConfigBytes(base)
		if err != nil && loads > 1 {
			return nil, generated, raw, fmt.Errorf("YAML decode failed near %s: %w", "old-obs-secret", err)
		}
		return cfg, generated, raw, err
	}
	path := filepath.Join(f.a.opts.BaseDir, configName)
	initial, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var initialNode yaml.Node
	if err := yaml.Unmarshal(initial, &initialNode); err != nil {
		t.Fatal(err)
	}
	secret := "old-obs-secret"
	root := initialNode.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "obs" {
			m := root.Content[i+1]
			for j := 0; j < len(m.Content); j += 2 {
				if m.Content[j].Value == "password" {
					m.Content[j+1].Value = secret
					m.Content[j+1].Tag = "!!str"
				}
			}
		}
	}
	initial, err = yaml.Marshal(&initialNode)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, _, err := loadConfigBytes(f.a.opts.BaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OBS.Password != secret {
		t.Fatalf("初回 OBS パスワード=%q", cfg.OBS.Password)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- f.a.run(ctx) }()
	<-f.h.started
	waitApplyState(t, f.a, ApplyApplied)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		t.Fatal(err)
	}
	root2 := node.Content[0]
	for i := 0; i < len(root2.Content); i += 2 {
		if root2.Content[i].Value != "admin" {
			continue
		}
		admin := root2.Content[i+1]
		for j := 0; j < len(admin.Content); j += 2 {
			if admin.Content[j].Value == "port" {
				admin.Content[j+1].Value = secret
				admin.Content[j+1].Tag = "!!str"
				admin.Content[j+1].Style = yaml.DoubleQuotedStyle
			}
		}
	}
	bad, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.a.Restart(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("終了コード=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("再起動の失敗で終了しなかった")
	}
	if strings.Contains(f.a.Apply().Reason, secret) || strings.Contains(f.stderr.String(), secret) {
		t.Fatalf("前世代の秘密が漏れた: Apply=%q stderr=%q", f.a.Apply().Reason, f.stderr.String())
	}
}

func TestRestart終了後はErrNotRunning(t *testing.T) {
	f := newFailureFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- f.a.run(ctx) }()
	<-f.h.started
	waitApplyState(t, f.a, ApplyApplied)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("終了が詰まった")
	}
	if err := f.a.Restart(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("終了後の Restart = %v, want ErrNotRunning", err)
	}
}

func TestRestart再読込と秘密と表示(t *testing.T) {
	f := newFailureFixture(t)
	f.a.deps.loadConfig = loadConfigBytes
	f.a.deps.openLogger = func(o logging.Options) (logHandle, error) { return logging.Open(o) }
	initCalls := 0
	f.a.deps.initJob = func() error { initCalls++; return nil }
	// InitJob は newApp 時に実行済みであり、再起動では呼ばない。
	got := make(chan obs.Config, 2)
	originalOBS := f.a.deps.newOBS
	f.a.deps.newOBS = func(c obs.Config, l *slog.Logger) obsRunner {
		got <- c
		l.Info("旧秘密 old-obs-secret secret-api")
		return originalOBS(c, l)
	}
	path := filepath.Join(f.a.opts.BaseDir, configName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		t.Fatal(err)
	}
	setOBS := func(value string) {
		root := node.Content[0]
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value == "obs" {
				m := root.Content[i+1]
				for j := 0; j < len(m.Content); j += 2 {
					if m.Content[j].Value == "password" {
						m.Content[j+1].Value = value
					}
				}
			}
		}
		data, e := yaml.Marshal(&node)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	setOBS("old-obs-secret")
	second := &failureHTTP{generationHTTP: generationHTTP{make(chan struct{}), make(chan struct{}), f.r}, serveError: http.ErrServerClosed}
	n := 0
	f.a.deps.newHTTP = func(coreRunner, string) servingHTTP {
		n++
		if n == 1 {
			return f.h
		}
		return second
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- f.a.run(ctx) }()
	<-f.h.started
	first := <-got
	if first.Password != "old-obs-secret" {
		t.Fatal("初回の設定")
	}
	for _, p := range f.parts {
		<-p.started
	}
	waitApplyState(t, f.a, ApplyApplied)
	setOBS("new-obs-secret")
	if err := f.a.Restart(); err != nil {
		t.Fatal(err)
	}
	<-second.started
	if c := <-got; c.Password != "new-obs-secret" {
		t.Fatal("再読込されていない")
	}
	cancel()
	if code := <-done; code != 0 {
		t.Fatal(code)
	}
	if v := f.a.Apply(); v.State != ApplyApplied || v.Kind != ApplyRestart {
		t.Fatal(v)
	}
	if initCalls != 0 {
		t.Fatal("InitJob を再実行した")
	}
	if strings.Count(f.out.String(), "旧秘密 *** ***") != 2 {
		t.Fatal("旧秘密が伏字にならない")
	}
	if strings.Count(strings.Join(f.r.events, ","), "banner.write") != 1 {
		t.Fatal("表示が重複した")
	}
}

func TestRestart停止中のシグナル(t *testing.T) {
	f := newFailureFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	f.stop = func(context.Context) { close(entered); <-release }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- f.a.run(ctx) }()
	<-f.h.started
	waitApplyState(t, f.a, ApplyApplied)
	if err := f.a.Restart(); err != nil {
		t.Fatal(err)
	}
	<-entered
	cancel()
	close(release)
	if code := <-done; code != 0 {
		t.Fatal(code)
	}
	if f.a.current != nil {
		t.Fatal("世代が残った")
	}
	if strings.Count(strings.Join(f.r.events, ","), "config.load") != 1 {
		t.Fatal("終了時に再作成した")
	}
}

func TestRestart作成中のシグナル(t *testing.T) {
	f := newFailureFixture(t)
	original := f.a.deps.loadConfig
	entered, release := make(chan struct{}), make(chan struct{})
	calls := 0
	f.a.deps.loadConfig = func(base string) (*config.Config, bool, []byte, error) {
		calls++
		if calls == 2 {
			close(entered)
			<-release
		}
		return original(base)
	}
	second := &failureHTTP{generationHTTP: generationHTTP{make(chan struct{}), make(chan struct{}), f.r}, serveError: http.ErrServerClosed}
	n := 0
	f.a.deps.newHTTP = func(coreRunner, string) servingHTTP {
		n++
		if n == 1 {
			return f.h
		}
		return second
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- f.a.run(ctx) }()
	<-f.h.started
	for _, p := range f.parts {
		<-p.started
	}
	waitApplyState(t, f.a, ApplyApplied)
	if err := f.a.Restart(); err != nil {
		t.Fatal(err)
	}
	<-entered
	cancel()
	close(release)
	select {
	case code := <-done:
		if code != 0 {
			t.Fatal(code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("回収が詰まった")
	}
	if f.a.current != nil || n != 2 {
		t.Fatal("作成した世代が未回収")
	}
	for _, p := range f.parts {
		if c := <-p.started; c.Err() == nil {
			t.Fatal("部品が未停止")
		}
	}
}
