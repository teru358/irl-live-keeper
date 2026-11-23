package app

import (
	"github.com/teru358/irl-live-keeper/internal/web"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageRestartHTTPと世代Cookie(t *testing.T) {
	a := newApp(Options{}, appDeps{initJob: func() error { return nil }})
	a.running = true
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
