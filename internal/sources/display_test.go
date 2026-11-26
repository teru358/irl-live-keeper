package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"strings"
	"testing"
	"time"
)

func TestCSSStrings(t *testing.T) {
	for _, tt := range []struct{ in, want string }{{`"`, `"\""`}, {`\`, `"\\"`}, {"\n", `"\a "`}, {"</style>", `"\3c /style>"`}} {
		if got := cssString(tt.in); got != tt.want {
			t.Errorf("%q: %q", tt.in, got)
		}
	}
}
func TestRenderFailure(t *testing.T) {
	text, err := Render(Source{Template: "{obs.password}"}, core.Snapshot{}, time.Now())
	if err == nil || text != "" {
		t.Fatal(text, err)
	}
	s := Default()
	s.Style.BackgroundColor = "#ff8040"
	s.Style.BackgroundOpacity = .5
	if !strings.Contains(Stylesheet(s), "rgba(255,128,64,0.5)") {
		t.Fatal(Stylesheet(s))
	}
}
