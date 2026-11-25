package web

import (
	"encoding/json"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"net/http"
	"strings"
	"testing"
)

type senderBackend struct{ fakeBackend }

func (*senderBackend) Sender() SenderSettings {
	return SenderSettings{Default: "host", Addresses: []SenderAddress{{Kind: "publish_host", Address: "host", Cameras: []SenderCamera{{ID: "main", SRT: &SenderProtocol{URL: "srt://host", Password: "secret"}}}}}}
}
func TestSenderAPI(t *testing.T) {
	h := NewWithAuth(&senderBackend{}, testPass, 9999, NewAuth(nil))
	if w := authRequest(h, "GET", "/api/sender", "", nil); w.Code != 401 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w)
	}
	w := authRequest(h, "POST", "/api/login", `{"password":"`+testPass+`"}`, nil)
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Value != "" {
			cookie = c
		}
	}
	w = authRequest(h, "GET", "/api/sender", "", cookie)
	var got SenderSettings
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Addresses[0].Cameras[0].SRT.Password != "secret" {
		t.Fatal(w)
	}
}
func TestSenderElementsAndCatalog(t *testing.T) {
	html, _ := assets.ReadFile("assets/page.html")
	js, _ := assets.ReadFile("assets/login.js")
	for _, key := range []string{"sender-address", "sender-cameras"} {
		if !strings.Contains(string(html), key) {
			t.Fatal(key)
		}
	}
	for _, key := range []string{"sender-qr-template", "sender-qr-toggle", "sender-qr-image"} {
		if strings.Contains(string(html), key) {
			t.Fatal(key)
		}
	}
	if strings.Contains(string(js), "sender_qr_") || strings.Contains(string(js), "sender-qr-") {
		t.Fatal("送信の設定に QR の処理が残っています")
	}
	for _, key := range []string{"data.protocol", "sender_copy", "sender_show"} {
		if key == "data.protocol" {
			key = "dataset.protocol"
		}
		if !strings.Contains(string(js), key) {
			t.Fatal(key)
		}
	}
	for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
		for _, key := range []string{"sender_qr_show", "sender_qr_hide", "sender_qr_password", "sender_qr_alt"} {
			if i18n.UI(lang)[key] != "" {
				t.Fatal(lang, "不要な文言", key)
			}
		}
		for _, key := range []string{"sender_hint", "sender_address", "sender_protocol", "sender_url", "sender_endpoint", "sender_key", "sender_streamid", "sender_user", "sender_password", "sender_obs", "sender_copy", "sender_copy_all", "sender_copied", "sender_manual", "sender_show", "sender_hide", "sender_rtmp_hint", "sender_lan", "sender_tailscale", "sender_publish_host"} {
			if i18n.UI(lang)[key] == "" {
				t.Fatal(lang, key)
			}
		}
	}
}
