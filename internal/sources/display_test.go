package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"strings"
	"testing"
	"time"
)

func TestRenderFailure(t *testing.T) {
	text, err := RenderSimulation(Source{Body: "{obs.password}"}, core.Snapshot{}, time.Now(), nil, Simulation{}, nil)
	if err == nil || text != "" {
		t.Fatal(text, err)
	}
}
func TestBaseStylesheet(t *testing.T) {
	if strings.Contains(BaseStylesheet, "opacity") {
		t.Fatal(BaseStylesheet)
	}
	for _, want := range []string{"html,body{margin:0;background:transparent;}"} {
		if !strings.Contains(BaseStylesheet, want) {
			t.Fatal(want, BaseStylesheet)
		}
	}
}
