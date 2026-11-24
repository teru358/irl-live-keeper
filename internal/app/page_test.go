package app

import (
	"context"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/web"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageRestartHTTPと世代Cookie(t *testing.T) {
	a := newApp(Options{}, appDeps{initJob: func() error { return nil }})
	a.running = true
	a.adminPassword = "password"
	b := &pageBackend{app: a, language: "auto", id: 1}
	handler := web.NewWithAuth(b, "password", 8787, a.auth)
	login := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"password":"password"}`))
	login.Header.Set("X-IRL-Live-Keeper", "1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, login)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	cookie := w.Result().Cookies()[0]
	for _, want := range []int{202, 409} {
		req := httptest.NewRequest("POST", "/api/restart", nil)
		req.Header.Set("X-IRL-Live-Keeper", "1")
		req.AddCookie(cookie)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != want {
			t.Fatalf("%d != %d", out.Code, want)
		}
	}
	// 再起動後の世代にも同じ認証の記憶を渡す。
	next := web.NewWithAuth(&pageBackend{app: a, id: 2}, "password", 8787, a.auth)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	out := httptest.NewRecorder()
	next.ServeHTTP(out, req)
	if out.Code != 200 || !strings.Contains(out.Body.String(), `"login":false`) {
		t.Fatal("再起動後の Cookie が無効")
	}
}

func TestSettingsPasswordSessions(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprint(restart), func(t *testing.T) {
			cfg := testConfig(t)
			a := newApp(Options{BaseDir: t.TempDir()}, appDeps{initJob: func() error { return nil }})
			a.adminPassword = "old-password"
			b := &pageBackend{app: a}
			h := web.NewWithAuth(b, "old-password", 8787, a.auth)
			login := func() *http.Cookie {
				r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"password":"old-password"}`))
				r.Header.Set("X-IRL-Live-Keeper", "1")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 200 {
					t.Fatal(w.Code)
				}
				return w.Result().Cookies()[0]
			}
			saved, other := login(), login()
			cfg.Admin.Password = "new-password"
			a.savedSession = saved.Value
			if restart {
				a.configureProcess(cfg)
			} else {
				a.applyLive(&generation{ctx: context.Background(), core: &settingsCore{}, obs: &settingsOBS{}, logger: &settingsLog{}}, &savedSettings{Next: cfg})
			}
			for _, c := range []*http.Cookie{saved, other} {
				r := httptest.NewRequest("GET", "/", nil)
				r.AddCookie(c)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				want := `"login":false`
				if c == other {
					want = `"login":true`
				}
				if !strings.Contains(w.Body.String(), want) {
					t.Fatal("セッションの保持/失効が違う")
				}
			}
			r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"password":"new-password"}`))
			r.Header.Set("X-IRL-Live-Keeper", "1")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatal("新しいパスワードで照合できない", w.Code)
			}
		})
	}
}
