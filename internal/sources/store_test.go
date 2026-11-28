package sources

import (
	"errors"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"strings"
	"testing"
)

func TestValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Source)
		code   string
	}{
		// S2-92 C-2・D-9: 保存の誤りは理由ごとの鍵。名前の誤りは「空」「使えない文字」「長すぎる」に分かれる。
		{"name", func(s *Source) { s.Name = "../bad" }, "sources.name_chars"}, {"name empty", func(s *Source) { s.Name = "" }, "sources.name_empty"},
		{"name Japanese", func(s *Source) { s.Name = "あいう" }, "sources.name_chars"}, {"name space", func(s *Source) { s.Name = "my source" }, "sources.name_chars"},
		{"name 33 characters", func(s *Source) { s.Name = strings.Repeat("a", 33) }, "sources.name_long"},
		{"width", func(s *Source) { s.OBSWidth = 0 }, "sources.size_range"}, {"height", func(s *Source) { s.OBSHeight = 8193 }, "sources.size_range"},
		{"title", func(s *Source) { s.Title = strings.Repeat("x", 1025) }, "sources.title_long"},
		{"body over 64 KiB", func(s *Source) { s.Body = strings.Repeat("x", MaxBody+1) }, "sources.body_too_large"},
		{"body over 64 KiB in bytes", func(s *Source) { s.Body = strings.Repeat("あ", MaxBody/3+1) }, "sources.body_too_large"},
		{"template", func(s *Source) { s.Body = "{unknown}" }, "template.unknown"},
		{"reserved files", func(s *Source) { s.Name = "files" }, "sources.reserved_name"},
		{"reserved frame", func(s *Source) { s.Name = "frame" }, "sources.reserved_name"},
		{"reserved preview", func(s *Source) { s.Name = "preview" }, "sources.reserved_name"},
		{"reserved test-obs", func(s *Source) { s.Name = "test-obs" }, "sources.reserved_name"},
		{"reserved assets", func(s *Source) { s.Name = "assets" }, "sources.reserved_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Default()
			s.Name = "demo"
			tt.change(&s)
			err := s.Validate(core.Snapshot{})
			if err == nil {
				t.Fatal("accepted")
			}
			var ve *ValidationError
			var te *template.Error
			code := ""
			if errors.As(err, &ve) {
				code = ve.Message.Code
			} else if errors.As(err, &te) {
				code = te.Message.Code
			}
			if code != tt.code {
				t.Fatal(code, err)
			}
		})
	}
	// Exactly 64 KiB (65536 bytes) is accepted; one more byte is not (above).
	s := Default()
	s.Name = "ok_-123"
	s.Body = strings.Repeat("x", MaxBody)
	if e := s.Validate(core.Snapshot{}); e != nil {
		t.Fatal(e)
	}
	// Names that merely start with a reserved word are fine.
	s.Name = "files2"
	if e := s.Validate(core.Snapshot{}); e != nil {
		t.Fatal(e)
	}
}

// S2-92: 予約された名前の文には、その名前を入れる（予約は files・assets だけではない）。
func TestReservedNameMessageNamesTheName(t *testing.T) {
	s := Default()
	s.Name = "frame"
	var ve *ValidationError
	if err := s.Validate(core.Snapshot{}); !errors.As(err, &ve) || ve.Message.Values["name"] != "frame" {
		t.Fatal(err)
	}
}
