package web

import (
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
)

// S2-91 C-1・D-1: パスワードが違うログインは「パスワードが違います」と言い、ログインが切れたときの文（api.unauthorized）とは別にする。
func TestLoginWrongPasswordHasItsOwnMessage(t *testing.T) {
	h := NewWithAuth(&fakeBackend{}, testPass, 8787, NewAuth(time.Now))
	for i, tc := range []struct{ lang, want string }{{"ja", "パスワードが違います。"}, {"en", "Wrong password."}} {
		w := bruteRequest(h, "POST", "/api/login?lang="+tc.lang, "bad", "1", "[::1]:"+strconv.Itoa(100+i), nil)
		body := decodeBody(t, w)
		if w.Code != 401 || body["code"] != "api.wrong_password" || body["error"] != tc.want {
			t.Fatalf("%s: %d %v", tc.lang, w.Code, body)
		}
	}
	// ログインが切れた要求（資格情報なし）は、今までどおり api.unauthorized。
	w := bruteRequest(h, "POST", "/api/status?lang=ja", "", "1", "[::1]:900", nil)
	if body := decodeBody(t, w); w.Code != 401 || body["code"] != "api.unauthorized" || body["error"] != "ログインが必要です。" {
		t.Fatalf("%d %v", w.Code, body)
	}
}

func TestTooManyAttemptsMessage(t *testing.T) {
	if got := i18n.Render(i18n.JA, i18n.New("api.too_many_attempts")); got != "パスワードの誤りが続きました。しばらく待ってから、もう一度試してください。" {
		t.Fatal(got)
	}
}

// S2-91 C-3・D-20: MediaMTX の状態の文。本体が返す3つの値（supervisor の State）に、日本語と英語の文がある。
func TestMediaMTXStatesHaveText(t *testing.T) {
	ja, en := i18n.UI(i18n.JA), i18n.UI(i18n.EN)
	for _, state := range []string{supervisor.StateRunning, supervisor.StateDown, supervisor.StateStarting} {
		if ja["mediamtx_"+state] == "" || en["mediamtx_"+state] == "" {
			t.Fatal("mediamtx_" + state)
		}
	}
	for key, want := range map[string][2]string{"mediamtx_down": {"止まっている", "Stopped"}, "mediamtx_failed": {"起動に失敗した", "Failed to start"}} {
		if ja[key] != want[0] || en[key] != want[1] {
			t.Fatal(key, ja[key], en[key])
		}
	}
	main, _ := assets.ReadFile("assets/app/main.js")
	// 知らない値は英語の生の値を出さず「—」にする。
	if regexp.MustCompile(`\|\|data\.mediamtx`).Match(main) || !strings.Contains(string(main), "'—'") {
		t.Fatal("状態の欄が生の値を出すか、知らない値の「—」がない")
	}
}

// S2-91 B-1: ダークのとき、プレビューの iframe が白く塗られない（枠の文書は変えず、iframe の要素に color-scheme を置く）。
func TestPreviewFrameKeepsColorSchemeOfFrameDocument(t *testing.T) {
	css, _ := os.ReadFile("assets/style.css")
	if !regexp.MustCompile(`\.source-preview iframe\s*\{[^}]*color-scheme:\s*light`).Match(css) {
		t.Fatal("iframe の color-scheme がない")
	}
	w := httptest.NewRecorder()
	writeSourceFrame(w, "x", true)
	if strings.Contains(w.Body.String(), "color-scheme") {
		t.Fatal("OBS のページの文書には color-scheme を足さない（既定の文字色が変わるため）")
	}
}
