package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"strings"
	"testing"
	"time"
)

func TestRenderFailure(t *testing.T) {
	text, err := Render(Source{Body: "{obs.password}"}, core.Snapshot{}, time.Now())
	if err == nil || text != "" {
		t.Fatal(text, err)
	}
}
func TestBaseStylesheet(t *testing.T) {
	for _, want := range []string{"html,body{margin:0;background:transparent;}", ".irl-source{", ".irl-empty{opacity:0;}"} {
		if !strings.Contains(BaseStylesheet, want) {
			t.Fatal(want, BaseStylesheet)
		}
	}
}
