package config

import (
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"strings"
	"testing"
)

func TestValidationMessages(t *testing.T) {
	c := &Config{}
	e := c.Validate().(*ValidationError)
	for _, f := range e.Fields {
		if got := i18n.Render(i18n.JA, f.Message); got != f.Reason {
			t.Errorf("日本語が不一致: %s", f.Field)
		}
		en := i18n.Render(i18n.EN, f.Message)
		if en == "" || en == f.Reason || strings.Contains(en, "{") {
			t.Errorf("英語が不正: %s: %s", f.Field, en)
		}
	}
	if len(e.RenderFields(i18n.EN)) == 0 {
		t.Fatal("fields が空")
	}
	c.Admin.LAN = true
	if i18n.Render(i18n.JA, c.WarningMessages()[0]) != c.Warnings()[0] {
		t.Fatal("警告が不一致")
	}
}
