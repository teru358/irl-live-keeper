package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// fb は検証を通る基準の設定に scene_fallbacks を載せる。
func fb(rows ...SceneFallback) *Config {
	c := valid()
	c.OBS.SceneFallbacks = rows
	return c
}

// fieldErr は Validate の ByField の結果を返す（通れば空）。
func fieldErr(t *testing.T, c *Config) map[string]string {
	t.Helper()
	err := c.Validate()
	if err == nil {
		return map[string]string{}
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("ValidationError ではない: %v", err)
	}
	return ve.ByField()
}

func mustContain(t *testing.T, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("%q を含まない: %s", w, s)
		}
	}
}

func TestFBCFG01_省略と空リストは同値で有効(t *testing.T) {
	cfg, err := parse([]byte(strings.Replace(defaultTemplate, `"*"`, `"abcdefgh12"`, -1)))
	if err != nil {
		t.Fatalf("ひな形を読めない: %v", err)
	}
	if len(cfg.OBS.SceneFallbacks) != 0 {
		t.Fatalf("省略で空でない: %+v", cfg.OBS.SceneFallbacks)
	}
	// キーを消した（省略）場合も空
	omit := regexp.MustCompile(`(?m)^  scene_fallbacks: \[\].*\n`).ReplaceAllString(strings.Replace(defaultTemplate, `"*"`, `"abcdefgh12"`, -1), "")
	if strings.Contains(omit, "scene_fallbacks: []") {
		t.Fatal("テストの前提: キーが消えていない")
	}
	cfg2, err := parse([]byte(omit))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg2.OBS.SceneFallbacks) != 0 {
		t.Fatalf("省略で空でない")
	}
	if e := fieldErr(t, fb()); len(e) != 0 {
		t.Fatalf("空が不正: %v", e)
	}
	if e := fieldErr(t, fb([]SceneFallback{}...)); len(e) != 0 {
		t.Fatalf("空が不正: %v", e)
	}
}

func TestFBCFG01_ひな形に例のコメントがあり読み直せる(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := Generate(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	mustContain(t, string(b), "scene_fallbacks: []", "# ", "fallback:")
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.OBS.SceneFallbacks) != 0 {
		t.Fatal("ひな形が空リストでない")
	}
}

func TestFBCFG_読み込み(t *testing.T) {
	cfg := withFallbacksYAML(t, "  scene_fallbacks:\n    - scene: シーン1\n      fallback: シーン2\n    - {scene: シーン2, fallback: Standby}\n")
	want := []SceneFallback{{Scene: "シーン1", Fallback: "シーン2"}, {Scene: "シーン2", Fallback: "Standby"}}
	if !reflect.DeepEqual(cfg.OBS.SceneFallbacks, want) {
		t.Fatalf("読み込み = %+v", cfg.OBS.SceneFallbacks)
	}
}

func TestFBCFG02_重複したsceneは不正(t *testing.T) {
	e := fieldErr(t, fb(SceneFallback{"A", "D"}, SceneFallback{"B", "D"}, SceneFallback{"A", "T"}))
	r, ok := e["obs.scene_fallbacks[2].scene"]
	if !ok {
		t.Fatalf("重複の行の scene が箇所でない: %v", e)
	}
	mustContain(t, r, "A", "1 行目", "1行にまとめ")
	if len(e) != 1 {
		t.Fatalf("余計な違反: %v", e)
	}
}

func TestFBCFG03_自己参照は長さ1の輪(t *testing.T) {
	e := fieldErr(t, fb(SceneFallback{"A", "A"}))
	r, ok := e["obs.scene_fallbacks[0].fallback"]
	if !ok {
		t.Fatalf("fallback が箇所でない: %v", e)
	}
	mustContain(t, r, "A→A", "輪", "別のシーン")
	if len(e) != 1 {
		t.Fatalf("余計な違反: %v", e)
	}
}

func TestFBCFG04_多段の輪(t *testing.T) {
	e := fieldErr(t, fb(SceneFallback{"A", "D"}, SceneFallback{"D", "B"}, SceneFallback{"B", "A"}))
	if len(e) != 1 {
		t.Fatalf("輪は1回だけ報告する: %v", e)
	}
	r, ok := e["obs.scene_fallbacks[0].fallback"]
	if !ok {
		t.Fatalf("該当項目が違う: %v", e)
	}
	mustContain(t, r, "A→D→B→A", "輪", "切")
	// 輪に入る尾（尾の行は輪の違反でない）
	e = fieldErr(t, fb(SceneFallback{"X", "A"}, SceneFallback{"A", "D"}, SceneFallback{"D", "A"}))
	if len(e) != 1 {
		t.Fatalf("尾は報告しない: %v", e)
	}
	if _, ok := e["obs.scene_fallbacks[1].fallback"]; !ok {
		t.Fatalf("輪の先頭行が箇所でない: %v", e)
	}
}

func TestFBCFG05_sceneに待機シーンは不正(t *testing.T) {
	e := fieldErr(t, fb(SceneFallback{"Standby", "A"}))
	r, ok := e["obs.scene_fallbacks[0].scene"]
	if !ok {
		t.Fatalf("scene が箇所でない: %v", e)
	}
	mustContain(t, r, "Standby", "待機シーン", "普通のシーン")
	if len(e) != 1 {
		t.Fatalf("余計な違反: %v", e)
	}
}

func TestFBCFG06_fallbackに待機シーンは有効(t *testing.T) {
	if e := fieldErr(t, fb(SceneFallback{"A", "Standby"})); len(e) != 0 {
		t.Fatalf("共通待機の明示が不正: %v", e)
	}
}

func TestFBCFG07_空の名前は項目ごとに不正で空白だけとは区別(t *testing.T) {
	e := fieldErr(t, fb(SceneFallback{"", "D"}, SceneFallback{"A", ""}, SceneFallback{"", ""}))
	for _, f := range []string{
		"obs.scene_fallbacks[0].scene", "obs.scene_fallbacks[1].fallback",
		"obs.scene_fallbacks[2].scene", "obs.scene_fallbacks[2].fallback",
	} {
		if r, ok := e[f]; !ok {
			t.Errorf("%s がない: %v", f, e)
		} else {
			mustContain(t, r, "空")
		}
	}
	if len(e) != 4 {
		t.Fatalf("違反の数: %v", e)
	}
	// 空白だけは空でない（OBS の実名かもしれない）
	if e := fieldErr(t, fb(SceneFallback{" ", "　"})); len(e) != 0 {
		t.Fatalf("空白だけが不正: %v", e)
	}
}

func TestFBCFG08_前後の空白はtrimしない(t *testing.T) {
	c := fb(SceneFallback{" A ", " D "})
	if e := fieldErr(t, c); len(e) != 0 {
		t.Fatalf("空白つきが不正: %v", e)
	}
	if c.OBS.SceneFallbacks[0] != (SceneFallback{" A ", " D "}) {
		t.Fatalf("Validate が値を変えた: %+v", c.OBS.SceneFallbacks)
	}
	// 読み込みでも維持
	cfg := withFallbacksYAML(t, "  scene_fallbacks:\n    - scene: \" A \"\n      fallback: \" D \"\n")
	if cfg.OBS.SceneFallbacks[0] != (SceneFallback{" A ", " D "}) {
		t.Fatalf("読み込みで trim された: %+v", cfg.OBS.SceneFallbacks)
	}
	// " A " と "A" は別の名前（重複でも輪でもない）
	if e := fieldErr(t, fb(SceneFallback{"A", " A "}, SceneFallback{" A ", "B"})); len(e) != 0 {
		t.Fatalf("空白違いが同名扱い: %v", e)
	}
}

func TestFBCFG09_行数の上限はない(t *testing.T) {
	var rows []SceneFallback
	for i := 0; i < 40; i++ {
		rows = append(rows, SceneFallback{fmt.Sprintf("S%d", i), fmt.Sprintf("S%d", i+1)})
	}
	if e := fieldErr(t, fb(rows...)); len(e) != 0 {
		t.Fatalf("40行が不正: %v", e)
	}
}

func TestFBCFG10_on_stream_startと同名でも追加制約なし(t *testing.T) {
	c := fb(SceneFallback{"Live", "Live2"})
	c.OnStreamStart.Enabled = true
	c.OnStreamStart.Scene = "Live"
	if e := fieldErr(t, c); len(e) != 0 {
		t.Fatalf("同名が不正: %v", e)
	}
	c = fb(SceneFallback{"A", "Live"})
	c.OnStreamStart.Enabled = true
	c.OnStreamStart.Scene = "Live"
	if e := fieldErr(t, c); len(e) != 0 {
		t.Fatalf("同名が不正: %v", e)
	}
}

func TestFBCFG11_OBSにない名前でもconfigとして有効(t *testing.T) {
	if e := fieldErr(t, fb(SceneFallback{"存在しないシーン", "これも無い"})); len(e) != 0 {
		t.Fatalf("不正: %v", e)
	}
}

func TestFBCFG12_待機名の変更で旧名が残れば警告(t *testing.T) {
	prev := fb()
	prev.OBS.StandbyScene = "S"
	next := fb(SceneFallback{"S", "A"}, SceneFallback{"B", "S"}, SceneFallback{"C", "D"})
	next.OBS.StandbyScene = "S2"
	if err := next.Validate(); err != nil {
		t.Fatalf("保存が成功しない: %v", err)
	}
	w := StandbyRenameWarnings(prev, next)
	if len(w) != 2 {
		t.Fatalf("警告の数 = %d: %v", len(w), w)
	}
	mustContain(t, w[0], "1 行目", "scene", "S", "普通のシーンとして残り、新しい待機シーンにはなりません")
	mustContain(t, w[1], "2 行目", "fallback", "S", "普通のシーンとして残り、新しい待機シーンにはなりません")
	// 元も先も旧名の行は項目ごとに2件
	next2 := fb(SceneFallback{"S", "S"})
	next2.OBS.StandbyScene = "S2"
	if w := StandbyRenameWarnings(prev, next2); len(w) != 2 {
		t.Fatalf("両項目で2件: %v", w)
	}
	// 旧名が残らなければ警告なし
	next3 := fb(SceneFallback{"C", "D"})
	next3.OBS.StandbyScene = "S2"
	if w := StandbyRenameWarnings(prev, next3); len(w) != 0 {
		t.Fatalf("警告が出た: %v", w)
	}
}

func TestFBCFG13_新しい待機名がsceneなら検証エラーで警告だけでは成功しない(t *testing.T) {
	next := fb(SceneFallback{"S2", "A"})
	next.OBS.StandbyScene = "S2"
	e := fieldErr(t, next)
	if _, ok := e["obs.scene_fallbacks[0].scene"]; !ok {
		t.Fatalf("その行の scene が箇所でない: %v", e)
	}
}

func TestFBCFG14_待機名の変更がなければ過去名警告なし(t *testing.T) {
	prev := fb(SceneFallback{"A", "Standby"})
	next := fb(SceneFallback{"A", "Standby"}, SceneFallback{"Old", "B"})
	if w := StandbyRenameWarnings(prev, next); len(w) != 0 {
		t.Fatalf("警告が出た: %v", w)
	}
	// 前の設定がない（初回・起動）なら出さない
	if w := StandbyRenameWarnings(nil, next); len(w) != 0 {
		t.Fatalf("nil で警告が出た: %v", w)
	}
	if w := StandbyRenameWarnings(prev, nil); len(w) != 0 {
		t.Fatalf("nil で警告が出た: %v", w)
	}
}

func TestFBCFG15_起動時の警告に過去名警告を含めない(t *testing.T) {
	c := fb(SceneFallback{"S", "A"})
	c.Admin.LAN = true
	for _, w := range c.Warnings() {
		if strings.Contains(w, "新しい待機シーンにはなりません") || strings.Contains(w, "scene_fallbacks") {
			t.Fatalf("Warnings に scene_fallbacks の警告が混ざった: %s", w)
		}
	}
	c.Admin.LAN = false
	if w := c.Warnings(); len(w) != 0 {
		t.Fatalf("Warnings = %v", w)
	}
}

// withFallbacksYAML はひな形の scene_fallbacks の行を、与えた YAML の塊に置き換えて読む。
func withFallbacksYAML(t *testing.T, block string) *Config {
	t.Helper()
	src := strings.Replace(defaultTemplate, `"*"`, `"abcdefgh12"`, -1)
	src = regexp.MustCompile(`(?m)^  scene_fallbacks: \[\].*\n`).ReplaceAllLiteralString(src, block)
	cfg, err := parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
