package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func bruteRequest(h http.Handler, method, path, pass, header, addr string, c *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(`{"password":"`+pass+`"}`))
	r.RemoteAddr = addr
	r.Header.Set(headerName, header)
	if pass != "" && path != "/api/login" {
		r.SetBasicAuth(username, pass)
	}
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestBruteforce順と期限(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewAuth(func() time.Time { return now })
	h := NewWithAuth(&fakeBackend{}, testPass, 8787, a)
	ip := "[::1]:100"
	req := func(method, path, pass, header string, c *http.Cookie) *httptest.ResponseRecorder {
		return bruteRequest(h, method, path, pass, header, ip, c)
	}
	session := req("POST", "/api/login", testPass, "1", nil).Result().Cookies()[0]
	// ヘッダーのない要求と資格情報のない要求は失敗数を増やさない。
	for i := 0; i < 10; i++ {
		if w := req("POST", "/api/login", "bad", "", nil); w.Code != 403 {
			t.Fatal(w)
		}
		if w := req("GET", "/api/status", "", "", nil); w.Code != 401 {
			t.Fatal(w)
		}
	}
	if len(a.attempts) != 0 {
		t.Fatal(a.attempts)
	}
	for i := 0; i < 5; i++ {
		if w := req("GET", "/api/status", "bad", "", nil); w.Code != 401 {
			t.Fatal(w)
		}
	}
	for _, tc := range []struct {
		method, path, pass, header string
		cookie                     *http.Cookie
		status                     int
		code                       string
	}{
		{"POST", "/api/unknown", "bad", "", nil, 404, "api.not_found"},
		{"POST", "/api/status", testPass, "", nil, 429, "api.too_many_attempts"},
		{"POST", "/api/status", "", "", nil, 401, "api.unauthorized"},
		{"POST", "/api/status", "", "", session, 405, "api.method"},
		{"POST", "/api/stream/start", "", "", session, 403, "api.forbidden"},
		{"GET", "/api/login", "bad", "", nil, 405, "api.method"},
		{"POST", "/api/login", "bad", "", nil, 403, "api.forbidden"},
		{"POST", "/api/login", testPass, "1", nil, 429, "api.too_many_attempts"},
	} {
		w := req(tc.method, tc.path, tc.pass, tc.header, tc.cookie)
		if w.Code != tc.status || decodeBody(t, w)["code"] != tc.code {
			t.Fatalf("%+v: %s", tc, w.Body.String())
		}
		if tc.status == 429 && w.Header().Get("Retry-After") != "30" {
			t.Fatal(w.Header())
		}
	}
	if w := req("GET", "/api/status", "bad", "", session); w.Code != 200 {
		t.Fatal(w)
	}
	token, _ := a.NewOnce()
	if w := req("GET", "/auth/once?t="+token, "", "", nil); w.Code != 303 || len(w.Result().Cookies()) == 0 {
		t.Fatal(w)
	}
	now = now.Add(30*time.Second - time.Nanosecond)
	if w := req("POST", "/api/login", testPass, "1", nil); w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Fatal(w)
	}
	now = now.Add(time.Nanosecond)
	if w := req("POST", "/api/login", "bad", "1", nil); w.Code != 401 {
		t.Fatal(w)
	}
	if a.attempts["::1"].failures != 1 {
		t.Fatal(a.attempts)
	}
	if w := req("POST", "/api/login", testPass, "1", nil); w.Code != 200 {
		t.Fatal(w)
	}
	if len(a.attempts) != 0 {
		t.Fatal(a.attempts)
	}
	// ポート違いは同一IP、転送ヘッダーは信頼しない。
	for i := 0; i < 5; i++ {
		bruteRequest(h, "POST", "/api/login", "bad", "1", fmt.Sprintf("[::1]:%d", i), nil)
	}
	r := httptest.NewRequest("GET", "/api/status", nil)
	r.RemoteAddr = "[::1]:999"
	r.SetBasicAuth(username, testPass)
	r.Header.Set("X-Forwarded-For", "192.0.2.1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 429 {
		t.Fatal(w)
	}
	if w := bruteRequest(h, "GET", "/api/status", testPass, "", "192.0.2.1:1", nil); w.Code != 200 {
		t.Fatal(w)
	}
}

func TestBruteforce同時要求(t *testing.T) {
	a := NewAuth(nil)
	h := NewWithAuth(&fakeBackend{}, testPass, 8787, a)
	start := make(chan struct{})
	results := make(chan int, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- bruteRequest(h, "POST", "/api/login", "bad", "1", "192.0.2.1:1", nil).Code
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for code := range results {
		counts[code]++
	}
	if counts[401] != 5 || counts[429] != 15 {
		t.Fatal(counts)
	}
	// 待ち中は照合関数を呼ばない。
	called := false
	_, wait := a.checkPassword(httptest.NewRequest("GET", "/", nil), nil)
	_ = wait
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:2"
	_, wait = a.checkPassword(r, func() bool { called = true; return true })
	if called || wait <= 0 {
		t.Fatal(called, wait)
	}
}

func TestBruteforce記録上限(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewAuth(func() time.Time { return now })
	for i := 0; i < maxPasswordIPs+20; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = fmt.Sprintf("192.0.%d.%d:1", i/256, i%256)
		a.checkPassword(r, func() bool { return false })
	}
	if len(a.attempts) != maxPasswordIPs {
		t.Fatal(len(a.attempts))
	}
	if _, ok := a.attempts["192.0.0.0"]; ok {
		t.Fatal("古い記録が残った")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.0.20:2"
	for i := 0; i < 4; i++ {
		a.checkPassword(r, func() bool { return false })
	}
	now = now.Add(passwordWait)
	a.checkPassword(r, nil)
	if _, ok := a.attempts["192.0.0.20"]; ok {
		t.Fatal("待ちの終わった記録が残った")
	}
}
