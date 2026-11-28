package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"time"
)

// Render is the single template assembly path for public and administrative views.
func Render(s Source, snapshot core.Snapshot, now time.Time, stats ...map[string]template.Value) (string, error) {
	if s.Broken != nil {
		return "", nil
	}
	p, err := template.Compile(s.Body, Definitions(snapshot))
	if err != nil {
		return "", err
	}
	return p.Render(Values(snapshot, now, stats...)), nil
}

// BaseStylesheet is the page skeleton CSS until the body carries its own styles.
const BaseStylesheet = "html,body{margin:0;background:transparent;}\n.irl-text{white-space:pre-wrap;}\n.irl-source{opacity:1;}\n.irl-empty{opacity:0;}\n"
