package web

import (
	"encoding/json"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

type displayFake struct {
	fakeBackend
	applying bool
}

func (b *displayFake) DisplayStatus() map[string]any {
	return map[string]any{"apply": map[string]any{"state": "applied"}, "generation": 2, "gui": map[string]any{"close_action": "confirm"}, "admin": map[string]any{"lan": false}}
}
func (b *displayFake) RestartHTTP() (int, error) {
	if b.applying {
		return 409, nil
	}
	b.applying = true
	return 202, nil
}
func TestPageStatusRestartSession(t *testing.T) {
	b := &displayFake{}
	a := NewAuth(nil)
	issue := httptest.NewRecorder()
	if err := a.issue(issue, "irl_session_8787"); err != nil {
		t.Fatal(err)
	}
	cookie := issue.Result().Cookies()[0]
	call := func(path, method string, login bool) *httptest.ResponseRecorder {
		t.Helper()
		h := NewWithAuth(b, testPass, 8787, a)
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set(headerName, "1")
		if login {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	page := call("/", "GET", true)
	for _, id := range []string{"menu", "links", "obs", "standby", "mediamtx", "actions"} {
		if !strings.Contains(page.Body.String(), `id="`+id+`"`) {
			t.Fatal(id)
		}
	}
	if !strings.Contains(call("/", "GET", false).Body.String(), `"login":true`) {
		t.Fatal("未認証の画面")
	}
	for _, path := range []string{"/assets/login.js", "/assets/style.css"} {
		w := call(path, "GET", false)
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), map[bool]string{true: "javascript", false: "text/css"}[strings.HasSuffix(path, "js")]) {
			t.Fatal(path, w.Header())
		}
	}
	var status map[string]any
	if err := json.Unmarshal(call("/api/status", "GET", true).Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"apply", "generation", "gui", "admin", "cameras", "obs", "last_error"} {
		if _, ok := status[key]; !ok {
			t.Fatal(key)
		}
	}
	if w := call("/api/restart", "POST", true); w.Code != 202 {
		t.Fatal(w.Code)
	}
	if w := call("/api/restart", "POST", true); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := call("/api/status", "GET", true); w.Code != 200 {
		t.Fatal("世代を作り直した後の Cookie", w.Code)
	}
}
func TestUIKeys(t *testing.T) {
	ja, en := i18n.UI(i18n.JA), i18n.UI(i18n.EN)
	if len(ja) == 0 || len(ja) != len(en) {
		t.Fatal("文の鍵数")
	}
	for k := range ja {
		if en[k] == "" {
			t.Fatal(k)
		}
	}
	for _, file := range []string{"assets/page.html", "assets/login.js"} {
		b, err := assets.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range regexp.MustCompile(`data-ui="([a-z_]+)"|\bt\('([a-z_]+)'\)`).FindAllStringSubmatch(string(b), -1) {
			key := m[1] + m[2]
			if ja[key] == "" || en[key] == "" {
				t.Fatal(file, key)
			}
		}
	}
}
