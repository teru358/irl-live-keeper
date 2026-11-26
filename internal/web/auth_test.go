package web

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func authRequest(h http.Handler, method, path, body string, c *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set(headerName, "1")
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestSessionログイン期限延長ログアウト(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := NewAuth(func() time.Time { return now })
	h := NewWithAuth(&fakeBackend{}, testPass, 9999, a)
	w := authRequest(h, "POST", "/api/login", `{"password":"`+testPass+`"}`, nil)
	if w.Code != 200 {
		t.Fatal(w)
	}
	c := w.Result().Cookies()[0]
	if c.Name != "irl_session_9999" || !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteStrictMode || !c.Expires.Equal(now.Add(sessionLife)) {
		t.Fatal(c)
	}
	now = now.Add(6 * 24 * time.Hour)
	w = authRequest(h, "GET", "/api/status", "", c)
	if w.Code != 200 || !w.Result().Cookies()[0].Expires.Equal(now.Add(sessionLife)) {
		t.Fatal(w)
	}
	now = now.Add(6 * 24 * time.Hour)
	if w = authRequest(h, "GET", "/api/status", "", c); w.Code != 200 {
		t.Fatal(w)
	}
	if w = authRequest(h, "POST", "/api/logout", "", c); w.Code != 200 || w.Result().Cookies()[len(w.Result().Cookies())-1].MaxAge != -1 {
		t.Fatal(w)
	}
	if w = authRequest(h, "GET", "/api/status", "", c); w.Code != 401 || w.Header().Get("WWW-Authenticate") != "" {
		t.Fatal(w)
	}
	w = authRequest(h, "POST", "/api/login", `{"password":"`+testPass+`"}`, nil)
	c = w.Result().Cookies()[0]
	now = now.Add(sessionLife)
	if w = authRequest(h, "GET", "/api/status", "", c); w.Code != 401 {
		t.Fatal(w)
	}
}
func TestOnce期限一回ヘッダーログ(t *testing.T) {
	now := time.Now()
	a := NewAuth(func() time.Time { return now })
	var log bytes.Buffer
	h := NewWithLogger(&fakeBackend{}, testPass, 8787, a, nil, slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
	token, err := a.NewOnce()
	if err != nil || len(token) != 32 {
		t.Fatal(token, err)
	}
	for i := 0; i < 2; i++ {
		w := authRequest(h, "GET", "/auth/once?t="+token, "", nil)
		if w.Code != 303 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal(w)
		}
		if (len(w.Result().Cookies()) > 0) != (i == 0) {
			t.Fatal(w)
		}
		if i == 1 && w.Header().Get("Location") != "/?login" {
			t.Fatal(w)
		}
	}
	if strings.Contains(log.String(), token) || strings.Contains(log.String(), "?t=") {
		t.Fatal("ログにクエリがある")
	}
	token, _ = a.NewOnce()
	now = now.Add(time.Minute)
	w := authRequest(h, "GET", "/auth/once?t="+token, "", nil)
	if len(w.Result().Cookies()) != 0 {
		t.Fatal(w)
	}
	w = authRequest(h, "GET", w.Header().Get("Location"), "", nil)
	if !strings.Contains(w.Body.String(), `type="password"`) || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal(w)
	}
}
func TestLogin照合前と誤り(t *testing.T) {
	h := New(&fakeBackend{}, testPass)
	for _, tc := range []struct {
		method, body, header string
		code                 int
	}{{"GET", "", "", 405}, {"POST", `{"password":"wrong"}`, "", 403}, {"POST", `{"password":"wrong"}`, "1", 401}, {"POST", "{", "1", 400}, {"POST", `{} {}`, "1", 400}} {
		r := httptest.NewRequest(tc.method, "/api/login?lang=ja", strings.NewReader(tc.body))
		r.Header.Set(headerName, tc.header)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code || decodeBody(t, w)["code"] == nil {
			t.Fatal(w)
		}
	}
	w := authRequest(h, "GET", "/assets/app/api.js", "", nil)
	b, _ := io.ReadAll(w.Result().Body)
	if w.Code != 200 || !bytes.Contains(b, []byte("fetch")) {
		t.Fatal(w)
	}
}

// 既定の slog ではなく、注入した世代の logger へ3種類の操作が届くこと。
func TestInjectedOperationLogger(t *testing.T) {
	var out bytes.Buffer
	h := NewWithLogger(&sceneBackend{}, testPass, 8787, NewAuth(nil), nil, slog.New(slog.NewTextHandler(&out, nil)))
	w := authRequest(h, "POST", "/api/login", `{"password":"`+testPass+`"}`, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	w = authRequest(h, "POST", "/api/obs/scene", `{"scene_uuid":"u"}`, cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = authRequest(h, "POST", "/api/logout", "", cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, want := range []string{"admin login", "admin logout", "scene selected from admin page", "scene=待機", "scene_uuid=u"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out.String())
		}
	}
}

func TestSourceKindPeerAddress(t *testing.T) {
	for _, tc := range []struct{ peer, want string }{
		{"127.0.0.1:1234", "local"}, {"[::1]:1234", "local"},
		{"192.168.1.2:1234", "lan"}, {"100.63.255.255:1234", "lan"},
		{"100.64.0.0:1234", "tailscale"}, {"100.127.255.255:1234", "tailscale"},
		{"100.128.0.0:1234", "lan"}, {"[::ffff:100.64.1.2]:1234", "tailscale"},
		{"invalid", "lan"},
	} {
		t.Run(tc.peer, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Forwarded-For", "100.64.1.1")
			r.Header.Set("X-Real-IP", "127.0.0.1")
			if got := sourceKind(r); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
