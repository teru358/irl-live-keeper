package web

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type setupBackend struct {
	fakeBackend
	calls int
}

func (b *setupBackend) SetInitialPassword(string) error { b.calls++; return nil }
func TestInitialPasswordGuards(t *testing.T) {
	for _, tc := range []struct {
		name, path, remote, host, origin, header, auth string
		status                                         int
	}{
		{"page", "/", "127.0.0.1:42", "localhost:8787", "", "", "", 200},
		{"lan", "/", "192.168.1.2:42", "localhost:8787", "", "", "", 403},
		{"host", "/", "127.0.0.1:42", "evil.example:8787", "", "", "", 403},
		{"origin", "/api/setup", "127.0.0.1:42", "localhost:8787", "http://evil.example:8787", "1", "", 403},
		{"csrf", "/api/setup", "127.0.0.1:42", "localhost:8787", "", "", "", 403},
		{"basic", "/", "127.0.0.1:42", "localhost:8787", "", "", "Basic abc", 403},
		{"api", "/api/status", "127.0.0.1:42", "localhost:8787", "", "", "", 403},
		{"save", "/api/setup", "[::1]:42", "[::1]:8787", "http://localhost:8787", "1", "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &setupBackend{}
			h := New(b, "")
			method := "GET"
			if tc.path == "/api/setup" {
				method = "POST"
			}
			r := httptest.NewRequest(method, tc.path, strings.NewReader(`{"password":"password123"}`))
			r.RemoteAddr = tc.remote
			r.Host = tc.host
			r.Header.Set("Origin", tc.origin)
			r.Header.Set(headerName, tc.header)
			if tc.auth != "" {
				r.Header.Set("Authorization", tc.auth)
			}
			r.Header.Set("X-Forwarded-For", "127.0.0.1")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestInitialPasswordConcurrent(t *testing.T) {
	b := &setupBackend{}
	h := New(b, "")
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "http://localhost:8787/api/setup", strings.NewReader(`{"password":"password123"}`))
			r.RemoteAddr = "127.0.0.1:42"
			r.Header.Set(headerName, "1")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for c := range codes {
		counts[c]++
	}
	if counts[200] != 1 || counts[409] != 1 || b.calls != 1 {
		t.Fatal(counts, b.calls)
	}
}
