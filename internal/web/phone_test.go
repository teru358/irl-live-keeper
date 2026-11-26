package web

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/i18n"
)

func TestPhoneLink(t *testing.T) {
	b := settingsFixture(t)
	b.store.Current.Admin.LAN = true
	a := NewAuth(nil)
	h := NewWithAuth(b, testPass, 9876, a)
	call := func(remote string, header bool, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/phone-link", strings.NewReader(body))
		r.RemoteAddr = remote
		r.SetBasicAuth("admin", testPass)
		if header {
			r.Header.Set(headerName, "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("127.0.0.1:42", true, `{"address":"192.168.1.2"}`)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var result struct{ URL string }
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(result.URL)
	if err != nil || u.Host != "192.168.1.2:9876" || u.Path != "/auth/once" {
		t.Fatal(result.URL, err)
	}
	token := u.Query().Get("t")
	for index, want := range []int{303, 303} {
		r := httptest.NewRequest("GET", "/auth/once?t="+token, nil)
		r.RemoteAddr = "192.168.1.8:42"
		response := httptest.NewRecorder()
		h.ServeHTTP(response, r)
		if response.Code != want || (index == 0 && response.Header().Get("Location") != "/") || (index == 1 && response.Header().Get("Location") != "/?login") {
			t.Fatalf("合言葉の使用 %d: %d %s", index, response.Code, response.Body.String())
		}
	}
	for _, test := range []struct {
		remote string
		header bool
		body   string
		code   int
	}{
		{"192.168.1.8:42", true, `{"address":"192.168.1.2"}`, 403},
		{"127.0.0.1:42", false, `{"address":"192.168.1.2"}`, 403},
		{"127.0.0.1:42", true, `{"address":"host/path"}`, 400},
		{"127.0.0.1:42", true, `{"address":"host:123"}`, 400},
	} {
		if got := call(test.remote, test.header, test.body); got.Code != test.code {
			t.Fatalf("%+v: %d", test, got.Code)
		}
	}
	b.store.Current.Admin.LAN = false
	if got := call("127.0.0.1:42", true, `{"address":"192.168.1.2"}`); got.Code != 409 {
		t.Fatal(got.Code)
	}
}

func TestDesignV019Elements(t *testing.T) {
	js := pageModules(t)
	html := js
	for _, part := range []string{"camera:['cameras','switch','on_stream_start']", "admin:['admin','mediamtx']", "other:['gui','log','language']", "settings-card", "settings_restart_note", "placeholder=String(s.values.switch[key])", "'_help'", "phone-link", "60000"} {
		if !strings.Contains(string(js), part) {
			t.Fatal(part)
		}
	}
	for _, id := range []string{"phone-dialog", "phone-address", "phone-qr"} {
		if !strings.Contains(string(html), `id="`+id+`"`) {
			t.Fatal(id)
		}
	}
	for _, part := range []string{"qrcode(0,'M')", "svgNode(s.svg)", "createSvgTag"} {
		if !strings.Contains(string(js), part) {
			t.Fatal(part)
		}
	}
	for key := range i18n.UI(i18n.JA) {
		if strings.HasPrefix(key, "setting_") || strings.HasPrefix(key, "phone_") || strings.HasPrefix(key, "settings_group_") {
			if i18n.UI(i18n.EN)[key] == "" {
				t.Fatal(key)
			}
			if strings.HasPrefix(key, "setting_") && !strings.HasPrefix(key, "setting_option_") && !strings.HasSuffix(key, "_help") {
				for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
					if i18n.UI(lang)[key+"_help"] == "" {
						t.Fatal(key, lang)
					}
				}
			}
		}
	}
}
