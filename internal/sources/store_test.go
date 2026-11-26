package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"strings"
	"testing"
)

func TestValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Source)
	}{
		{"name", func(s *Source) { s.Name = "../bad" }}, {"width", func(s *Source) { s.OBSWidth = 0 }}, {"height", func(s *Source) { s.OBSHeight = 8193 }},
		{"css", func(s *Source) { s.CSS = strings.Repeat("x", MaxCSS+1) }}, {"template", func(s *Source) { s.Template = "{unknown}" }},
		{"font", func(s *Source) { s.Style.Font = strings.Repeat("x", 257) }}, {"size", func(s *Source) { s.Style.FontSize = 513 }},
		{"color", func(s *Source) { s.Style.Color = "red; bad" }}, {"weight", func(s *Source) { s.Style.FontWeight = 101 }},
		{"outline", func(s *Source) { s.Style.OutlineWidth = 33 }}, {"blur", func(s *Source) { s.Style.ShadowBlur = 129 }},
		{"shadow", func(s *Source) { s.Style.ShadowX = -513 }}, {"opacity", func(s *Source) { s.Style.BackgroundOpacity = 1.1 }},
		{"radius", func(s *Source) { s.Style.BorderRadius = -1 }}, {"padding", func(s *Source) { s.Style.Padding = 4097 }},
		{"align", func(s *Source) { s.Style.Align = "justify" }}, {"transition", func(s *Source) { s.Style.Transition = "spin" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Default()
			s.Name = "demo"
			tt.change(&s)
			if s.Validate(core.Snapshot{}) == nil {
				t.Fatal("accepted")
			}
		})
	}
	s := Default()
	s.Name = "ok_-123"
	s.CSS = strings.Repeat("x", MaxCSS)
	s.Style.Color = "#abcdef80"
	if e := s.Validate(core.Snapshot{}); e != nil {
		t.Fatal(e)
	}
}
