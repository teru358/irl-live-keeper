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
		{"name", func(s *Source) { s.Name = "../bad" }, "sources.invalid"}, {"width", func(s *Source) { s.OBSWidth = 0 }, "sources.invalid"}, {"height", func(s *Source) { s.OBSHeight = 8193 }, "sources.invalid"},
		{"title", func(s *Source) { s.Title = strings.Repeat("x", 1025) }, "sources.invalid"},
		{"body over 64 KiB", func(s *Source) { s.Body = strings.Repeat("x", MaxBody+1) }, "sources.body_too_large"},
		{"body over 64 KiB in bytes", func(s *Source) { s.Body = strings.Repeat("あ", MaxBody/3+1) }, "sources.body_too_large"},
		{"template", func(s *Source) { s.Body = "{unknown}" }, "template.unknown"},
		{"reserved files", func(s *Source) { s.Name = "files" }, "sources.reserved_name"},
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
