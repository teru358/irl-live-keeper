package web

import (
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKeepSettingsSession(t *testing.T) {
	for _, keep := range []string{"saved", ""} {
		a := NewAuth(nil)
		a.sessions["saved"] = time.Now().Add(time.Hour)
		a.sessions["other"] = time.Now().Add(time.Hour)
		a.sessionAuth["saved"] = "password"
		a.sessionAuth["other"] = "password"
		a.KeepSession(keep)
		if _, ok := a.session("other"); ok {
			t.Fatal("ほかのセッションが残った")
		}
		_, ok := a.session("saved")
		if ok != (keep != "") {
			t.Fatal("保存した要求のセッション")
		}
		if len(a.sessionAuth) != len(a.sessions) {
			t.Fatal("認証の記憶が残った")
		}
	}
}

func TestSettingsStartsAfterResponseAndUnlock(t *testing.T) {
	b := settingsFixture(t)
	w := httptest.NewRecorder()
	started := false
	b.store.StartApply = func() {
		if w.Code != 202 || !w.Flushed {
			t.Fatal("202 の送出前に反映")
		}
		if !b.mu.TryLock() {
			t.Fatal("transaction の lock が残る")
		}
		b.mu.Unlock()
		started = true
	}
	body := `{"hash":"` + settings.Hash(b.store.Raw) + `","values":{"log":{"keep_days":3}}}`
	r := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	serveSettings(w, r, b, i18n.JA)
	if !started {
		t.Fatal("反映を開始していない")
	}
}
