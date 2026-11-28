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
	if err := a.issue(issue, "irl_session_8787", "password"); err != nil {
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
	app := pageModules(t)
	for _, id := range []string{"menu", "links", "obs", "standby", "mediamtx", "actions", "language-switch"} {
		if !strings.Contains(page.Body.String()+string(app), `id="`+id+`"`) {
			t.Fatal(id)
		}
	}
	removed := []string{"log-panel", "soon"}
	for _, id := range removed {
		if strings.Contains(string(app), `id="`+id+`"`) {
			t.Errorf("removed element remains: %s", id)
		}
	}
	for _, page := range []string{"history", "logs"} {
		if strings.Contains(string(app), `data-page="`+page+`"`) {
			t.Errorf("removed menu remains: %s", page)
		}
	}
	if !strings.Contains(call("/", "GET", false).Body.String(), `"login":true`) {
		t.Fatal("未認証の画面")
	}
	for _, path := range []string{"/assets/app/main.js", "/assets/style.css", "/assets/vendor/qrcode-generator-2.0.4.js", "/assets/favicon-32.png", "/favicon.ico"} {
		w := call(path, "GET", false)
		wantType := "text/css"
		if strings.HasSuffix(path, "js") {
			wantType = "javascript"
		}
		if strings.HasSuffix(path, ".png") || path == "/favicon.ico" {
			wantType = "image/png"
		}
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), wantType) {
			t.Fatal(path, w.Header())
		}
	}
	pageHTML, err := assets.ReadFile("assets/page.html")
	if err != nil || !strings.Contains(string(pageHTML), `<link rel="icon" type="image/png" href="/assets/favicon-32.png">`) || !strings.Contains(string(pageHTML), `<link rel="apple-touch-icon" href="/assets/apple-touch-icon.png">`) {
		t.Fatal("favicon links missing from page head")
	}
	setupHandler := NewWithAuth(&fakeBackend{}, "", 8787, NewAuth(nil))
	for _, path := range []string{"/assets/favicon-32.png", "/favicon.ico", "/assets/app/main.js", "/assets/app/shell.js", "/assets/vendor/lit-html-3.3.3/lit-html.js", "/assets/vendor/lit-html-3.3.3/directives/live.js"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Host = "localhost:8787"
		r.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		setupHandler.ServeHTTP(w, r)
		wantType := "image/png"
		if strings.HasSuffix(path, ".js") {
			wantType = "javascript"
		}
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), wantType) {
			t.Fatalf("password未設定時の %s: %d %v", path, w.Code, w.Header())
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
	for _, file := range []string{"assets/page.html", "assets/app/settings.js", "assets/app/main.js", "assets/app/shell.js", "assets/app/phone.js", "assets/app/settings-state.js", "assets/app/sources.js"} {
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

func TestSettingsPageControls(t *testing.T) {
	w := httptest.NewRecorder()
	servePage(w, &displayFake{}, false)
	app := pageModules(t)
	html := w.Body.String() + string(app)
	for _, want := range []string{`id="settings-panel"`, `id=${page+'-groups'}`, `id=${page+'-fields'}`, `id=${page+'-local'}`, `role="switch"`, `id="settings-confirm"`, `id="cameras-confirm"`, `id=${page+'-update'}`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}
	js := pageModules(t)
	ja, en := i18n.UI(i18n.JA), i18n.UI(i18n.EN)
	// All dynamically selected labels and apply outcomes must exist in both catalogs.
	for key, value := range ja {
		if strings.HasPrefix(key, "setting_") || strings.HasPrefix(key, "settings_") {
			if value == "" || en[key] == "" {
				t.Errorf("missing translation %s", key)
			}
		}
	}
	for _, key := range []string{"language", "admin", "mediamtx", "stream", "cameras", "obs", "on_stream_start", "switch", "log", "gui"} {
		if ja["setting_"+key] == "" || en["setting_"+key] == "" {
			t.Error(key)
		}
	}
	for _, want := range []string{"settings/preview", "settingsMode", "?readonly=${localOnly}", "settings_stream_warning", "settings_pending_urls", "result.admin_url"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("missing behavior %s", want)
		}
	}
}

func TestSettingsFailureAndRefreshPaths(t *testing.T) {
	js := pageModules(t)
	for _, want := range []string{"settings_not_saved", "s.errors=e.fields||{}", "fieldGroup", "s.group=fieldGroup", "focusSettingError", "settings_reloaded", "settings_conflicts", "JSON.stringify([status.generation,status.apply])", "reloadAfterApply", "rebaseDraft", "s.baseline=JSON.stringify(fresh)"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("missing error/refresh path %s", want)
		}
	}
}

func pageModules(t *testing.T) []byte {
	t.Helper()
	var result []byte
	for _, name := range []string{"main", "shell", "settings", "settings-state", "settings-flow", "connection", "phone", "sources"} {
		b, err := assets.ReadFile("assets/app/" + name + ".js")
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, b...)
	}
	return result
}

func TestSourcesPageControls(t *testing.T) {
	app := string(pageModules(t))
	for _, want := range []string{`id="sources-panel"`, `data-page="sources"`, `id="source-body"`, `allow-same-origin`, `/api/sources/frame`, `updateRendered`, `src_discard`, `sources/test-obs/alive`} {
		if !strings.Contains(app, want) {
			t.Fatal(want)
		}
	}
}
