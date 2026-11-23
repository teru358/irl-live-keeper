package switcher

import (
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestPurposeMessages(t *testing.T) {
	for p := PurposeAutoFail; p <= PurposeRestoreAll; p++ {
		m := purposeMessage(i18n.New("switch.unconfirmed", "scene", "camera"), p)
		machine := &Machine{}
		machine.recordError(time.Time{}, m)
		en := i18n.Render(i18n.EN, machine.st.LastError.Localized)
		if strings.Contains(en, p.String()) || strings.ContainsFunc(en, func(r rune) bool { return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) }) {
			t.Errorf("目的の英語: %s", en)
		}
	}
}
