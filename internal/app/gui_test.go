package app

import (
	"context"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/web"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWindowOnceURL(t *testing.T) {
	a := web.NewAuth(nil)
	token, err := a.NewOnce()
	if err != nil {
		t.Fatal(err)
	}
	u := OnceURL(9123, token)
	want := "http://127.0.0.1:9123/auth/once?t=" + token
	if u != want {
		t.Fatal(u)
	}
	h := web.NewWithAuth(nil, "secret", 9123, a)
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodGet, u, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if i == 0 && (w.Code != 303 || len(w.Result().Cookies()) == 0) {
			t.Fatal(w.Code)
		}
		if i == 1 && (w.Header().Get("Location") != "/?login" || len(w.Result().Cookies()) != 0) {
			t.Fatal("reused")
		}
	}
}

// 窓の各終了通知は同じ cancel を渡し、世代の停止を待つ。
func TestWindowShutdown(t *testing.T) {
	for _, event := range []string{"exit", "close", "logoff"} {
		t.Run(event, func(t *testing.T) {
			f := newFailureFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan int, 1)
			go func() { done <- f.a.RunWindow(ctx) }()
			select {
			case <-f.h.started:
			case <-time.After(3 * time.Second):
				t.Fatal("start timeout")
			}
			cancel()
			select {
			case code := <-done:
				if code != 0 {
					t.Fatal(code)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("stop timeout")
			}
			stages := strings.Join(f.stages(), ",")
			for _, v := range []string{"core.stopping", "http.shutdown", "generation.cancel", "parts.wait", "logger.close"} {
				if !strings.Contains(stages, v) {
					t.Fatal(stages)
				}
			}
		})
	}
}
func TestWindowStartupFailureRetry(t *testing.T) {
	f := newFailureFixture(t)
	load := f.a.deps.loadConfig
	var attempts atomic.Int32
	f.a.deps.loadConfig = func(b string) (*config.Config, bool, []byte, error) {
		if attempts.Add(1) == 1 {
			return nil, false, nil, errors.New("broken config")
		}
		return load(b)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- f.a.RunWindow(ctx) }()
	deadline := time.After(3 * time.Second)
	for {
		err := f.a.Restart()
		if err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("retry unavailable")
		default:
			runtime.Gosched()
		}
	}
	select {
	case <-f.h.started:
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not start")
	}
	cancel()
	<-done
}

func TestWindowStartupOrder(t *testing.T) {
	var order []string
	order = append(order, "instance", "crash")
	a := newApp(Options{Stdout: io.Discard, Stderr: io.Discard}, appDeps{initJob: func() error { order = append(order, "job"); return nil }, loadConfig: func(string) (*config.Config, bool, []byte, error) {
		order = append(order, "generation")
		return nil, false, nil, errors.New("stop")
	}})
	order = append(order, "window")
	a.runApp(context.Background())
	if strings.Join(order, ",") != "instance,crash,job,window,generation" {
		t.Fatal(order)
	}
}
