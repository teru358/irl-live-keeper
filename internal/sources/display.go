package sources

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"strings"
	"time"
)

// Render is the single template assembly path for public and administrative views.
func Render(s Source, snapshot core.Snapshot, now time.Time, stats ...map[string]template.Value) (string, error) {
	p, err := template.Compile(s.Template, Definitions(snapshot))
	if err != nil {
		return "", err
	}
	return p.Render(Values(snapshot, now, stats...)), nil
}

func cssString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch c {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(c)
		case '\n', '\r', '\f', 0:
			fmt.Fprintf(&b, "\\%x ", c)
		case '<':
			b.WriteString("\\3c ")
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Stylesheet places custom CSS last and never embeds it in HTML.
func Stylesheet(s Source) string {
	st := s.Style
	var b strings.Builder
	b.WriteString("html,body{margin:0;background:transparent;}\n.irl-text{white-space:pre-wrap;}\n.irl-source{opacity:1;")
	if st.Font != "" {
		fmt.Fprintf(&b, "font-family:%s;", cssString(st.Font))
	}
	if st.FontSize != 0 {
		fmt.Fprintf(&b, "font-size:%gpx;", st.FontSize)
	}
	if st.FontWeight != 0 {
		fmt.Fprintf(&b, "font-weight:%d;", st.FontWeight)
	}
	if st.Color != "" {
		fmt.Fprintf(&b, "color:%s;", st.Color)
	}
	if st.Align != "" {
		fmt.Fprintf(&b, "text-align:%s;", st.Align)
	}
	fmt.Fprintf(&b, "padding:%gpx;border-radius:%gpx;", st.Padding, st.BorderRadius)
	if st.OutlineColor != "" {
		fmt.Fprintf(&b, "-webkit-text-stroke:%gpx %s;paint-order:stroke fill;", st.OutlineWidth, st.OutlineColor)
	}
	if st.ShadowColor != "" {
		fmt.Fprintf(&b, "text-shadow:%gpx %gpx %gpx %s;", st.ShadowX, st.ShadowY, st.ShadowBlur, st.ShadowColor)
	}
	if st.BackgroundColor != "" {
		var r, g, bl, a uint64
		fmt.Sscanf(st.BackgroundColor[:7], "#%02x%02x%02x", &r, &g, &bl)
		alpha := st.BackgroundOpacity
		if len(st.BackgroundColor) == 9 {
			fmt.Sscanf(st.BackgroundColor[7:], "%02x", &a)
			alpha *= float64(a) / 255
		}
		fmt.Fprintf(&b, "background-color:rgba(%d,%d,%d,%g);", r, g, bl, alpha)
	}
	if st.Transition == "fade" {
		b.WriteString("transition:opacity .3s;")
	}
	b.WriteString("}\n.irl-empty{opacity:0;}\n")
	b.WriteString(s.CSS)
	return b.String()
}
