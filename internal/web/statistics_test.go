package web

import (
	"github.com/teru358/irl-live-keeper/internal/sources"
	"testing"
	"time"
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

// v0.26: 編集の画面の「値の一覧」を開いている間（最後の /api/source-values から5秒）は、
// 統計を使うソースがなくても読む。
func TestStatisticsDemandValueList(t *testing.T) {
	e := &Events{clients: map[*eventClient]bool{}}
	start := time.Now()
	e.wantStatistics(start)
	if !e.needsStatistics(nil) {
		t.Fatal("値の一覧を開いた直後に読まない")
	}
	old := &Events{clients: map[*eventClient]bool{}}
	old.wantStatistics(start.Add(-statisticsValuesHold - time.Second))
	if old.needsStatistics(nil) {
		t.Fatal("値の一覧を閉じて5秒たっても読む")
	}
	e.wantStatistics(start.Add(-time.Hour)) // 古い時刻で短くしない
	if !e.needsStatistics(nil) {
		t.Fatal("古い要求で期限が縮んだ")
	}
}
