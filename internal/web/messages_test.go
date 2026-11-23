package web

import (
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"net/http/httptest"
	"testing"
)

func TestErrorLanguages(t *testing.T) {
	for _, tc := range []struct{ query, accept, want string }{
		{"", "", "Not connected to OBS"}, {"?lang=en", "ja", "Not connected to OBS"}, {"?lang=ja", "en", "OBS に未接続"}, {"", "ja-JP", "OBS に未接続"},
	} {
		r := httptest.NewRequest("POST", "/api/standby/on"+tc.query, nil)
		r.SetBasicAuth("admin", testPass)
		r.Header.Set(headerName, "1")
		r.Header.Set("Accept-Language", tc.accept)
		w := httptest.NewRecorder()
		New(&fakeBackend{err: switcher.ErrNotConnected{}}, testPass).ServeHTTP(w, r)
		body := decodeBody(t, w)
		if body["error"] != tc.want || body["code"] != "api.disconnected" {
			t.Errorf("応答: %v", body)
		}
	}
}

func TestInvalidStateLanguages(t *testing.T) {
	for _, tc := range []struct{ lang, want string }{
		{"ja", "受け付けられない: 配信中"}, {"en", "Cannot accept: Streaming"}, {"", "Cannot accept: Streaming"},
	} {
		r := httptest.NewRequest("POST", "/api/standby/on?lang="+tc.lang, nil)
		r.SetBasicAuth("admin", testPass)
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		New(&fakeBackend{err: switcher.ErrInvalidState{Reason: switcher.ReasonStreaming}}, testPass).ServeHTTP(w, r)
		body := decodeBody(t, w)
		if w.Code != 409 || body["error"] != tc.want || body["code"] != "api.invalid" {
			t.Errorf("応答: %v", body)
		}
	}
}
