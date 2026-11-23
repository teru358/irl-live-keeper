package core

import (
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestStatusMessageSourcesEnglish(t *testing.T) {
	hidden := i18n.New("hidden.load", "error", "read failed")
	h := newHarness(t, func(_ *switcher.Config, d *Deps) { d.HiddenLoadMessage = hidden })
	messages := append([]i18n.Msg{}, h.core.Snapshot().OBS.Warnings...)
	sw := i18n.New("switch.start_missing", "scene", "camera")
	h.core.noteSwitcherError(&switcher.ErrorInfo{At: time.Now(), Localized: sw})
	messages = append(messages, h.core.lastErr.Message)
	st := supervisor.Status{LastError: "mediamtx が終了しました", LastMessage: i18n.New("mediamtx.exit")}
	h.core.noteMediaMTXError(st.LastError, st.LastMessage)
	messages = append(messages, h.core.lastErr.Message)
	if len(messages) != 3 {
		t.Fatalf("文の数: %d", len(messages))
	}
	for _, m := range messages {
		en := i18n.Render(i18n.EN, m)
		if en == "" || strings.ContainsFunc(en, func(r rune) bool { return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) }) {
			t.Errorf("英語に日本語: %s", en)
		}
	}
}
