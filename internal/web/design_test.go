package web

import (
	"strings"
	"testing"
)

func TestDesignAssets(t *testing.T) {
	page, _ := assets.ReadFile("assets/page.html")
	app, _ := assets.ReadFile("assets/app/main.js")
	css, _ := assets.ReadFile("assets/style.css")
	for _, marker := range []string{`id="tabs"`, `id="summary"`, `id="sheet"`, `id="theme-switch"`} {
		if !strings.Contains(string(page), marker) {
			t.Fatal(marker)
		}
	}
	for _, marker := range []string{`<dl class="facts">`, `id="obs"`, `id="standby"`, `id="mediamtx"`, `t('warning')`, `.warn`, `class="stream-zone"`, `rate-value`} {
		if !strings.Contains(string(page)+string(app), marker) && !strings.Contains(string(css), marker) {
			t.Fatalf("試作に合わせた表示要素がない: %s", marker)
		}
	}
	ops := string(app)
	manual, warning, showAll, stream := strings.Index(ops, `t('manual_standby')`), strings.Index(ops, `t('warning')`), strings.Index(ops, `t('show_all')`), strings.Index(ops, `class="stream-zone"`)

	if manual < 0 || warning < manual || showAll < warning || stream < showAll {
		t.Fatal("操作は手動の待機、注意、全ソース表示、配信の順に並べる")
	}
	// 書体は同梱しない（各機器の書体を使う。2026-11-23 のユーザーの判断）。外部の書体や CDN も読まない
	for _, s := range []string{"@font-face", "fonts.googleapis", "fonts.gstatic", "http://", "https://"} {
		if strings.Contains(string(css), s) || strings.Contains(string(page), s) {
			t.Fatalf("書体の同梱か外部の読み込みがある: %s", s)
		}
	}
	if !strings.Contains(string(css), "tabular-nums") {
		t.Fatal("数字の桁をそろえる設定（tabular-nums）がない")
	}
}
