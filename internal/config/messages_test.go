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

func TestCameraValueIDMessage(t *testing.T) {
	c := valid()
	c.Cameras[0].ID = "kbps"
	err := c.Validate()
	e, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	for _, field := range e.Fields {
		if field.Field != "cameras[0].id" {
			continue
		}
		ja, en := i18n.Render(i18n.JA, field.Message), i18n.Render(i18n.EN, field.Message)
		if !strings.Contains(ja, "kbps") || !strings.Contains(en, "kbps") || !strings.Contains(ja, "camera.*") || !strings.Contains(en, "camera.*") || ja == en || strings.Contains(en, "{a}") {
			t.Fatalf("invalid localized reason: ja=%q en=%q", ja, en)
		}
		return
	}
	t.Fatal("missing camera ID reason")
}
