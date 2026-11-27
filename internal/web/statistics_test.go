package web

import (
	"github.com/teru358/irl-live-keeper/internal/sources"
	"testing"
)

func TestStatisticsDemand(t *testing.T) {
	c := &eventClient{topics: map[string]bool{"source:a": true}}
	e := &Events{clients: map[*eventClient]bool{c: true}}
	for _, tc := range []struct {
		name, text          string
		admin, active, want bool
	}{
		{"a", "{stream.kbps}", false, true, true},
		{"b", "{stream.kbps}", false, true, false},
		{"a", "{camera.kbps}", true, true, false},
		{"b", "{obs.fps}", true, true, true},
		{"a", "{stream.elapsed}", false, false, false},
	} {
		c.topics = map[string]bool{"source:a": true, "sources": tc.admin}
		e.clients[c] = tc.active
		if got := e.needsStatistics([]sources.Source{{Name: tc.name, Template: tc.text}}); got != tc.want {
			t.Errorf("%+v: %v", tc, got)
		}
	}
	if e.needsStatistics(nil) {
		t.Fatal("no sources")
	}
}
