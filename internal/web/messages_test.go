package web

import (
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"net/http/httptest"
	"testing"
)

// S2-92 C-12: 接続の誤りは次の操作を言う。
const (
	notConnectedEN = "Not connected to OBS. Start OBS, then check the address, port and password in Settings > OBS."
	notConnectedJA = "OBS につながっていません。OBS を起動し、「設定」の「OBS」のアドレス・ポート・パスワードを確認してください。"
)

func TestErrorLanguages(t *testing.T) {
	for _, tc := range []struct{ query, accept, want string }{
		{"", "", notConnectedEN}, {"?lang=en", "ja", notConnectedEN}, {"?lang=ja", "en", notConnectedJA}, {"", "ja-JP", notConnectedJA},
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
		{"ja", "受け付けられません: 配信中"}, {"en", "Cannot accept: Streaming"}, {"", "Cannot accept: Streaming"},
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
