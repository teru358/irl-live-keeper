package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
)

// S2-92 C-2・D-9: ソースの保存の誤りは、理由ごとの鍵と文で返る（「ソースの値の範囲が正しくありません。」の1つにしない）。
func TestSourceSaveErrorsHaveReasons(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	b.snap.Cameras = []core.CameraStatus{{ID: "cam1"}}
	h := apiHandler(b, nil)
	post := func(lang, body string, want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/sources?lang="+lang, strings.NewReader(body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
		return decodeBody(t, w)
	}
	for _, tc := range []struct{ body, code, ja, en string }{
		{`{"name":"","title":"x","body":"a"}`, "sources.name_empty", "URL 用の名前を入れてください。", "Enter a URL name."},
		{`{"name":"日本語","body":"a"}`, "sources.name_chars", "URL 用の名前に使えるのは、英数字と - と _ だけです。", "The URL name can only use letters, digits, - and _."},
		{`{"name":"my source","body":"a"}`, "sources.name_chars", "", ""},
		{`{"name":"` + strings.Repeat("a", 33) + `","body":"a"}`, "sources.name_long", "", ""},
		{`{"name":"demo","obs_width":0,"obs_height":200,"body":"a"}`, "sources.size_range", "", ""},
		{`{"name":"demo","title":"` + strings.Repeat("x", 1025) + `","body":"a"}`, "sources.title_long", "", ""},
		{`{"name":"files","body":"a"}`, "sources.reserved_name", "「files」は予約されているため、URL 用の名前に使えません。", "\"files\" is reserved and cannot be used as a URL name."},
	} {
		for lang, want := range map[string]string{"ja": tc.ja, "en": tc.en} {
			got := post(lang, tc.body, 422)
			if got["code"] != tc.code {
				t.Fatalf("%s: %v", tc.body, got)
			}
			if want != "" && got["error"] != want {
				t.Fatalf("%s/%s: %v", tc.code, lang, got["error"])
			}
			if got["error"] == "" || got["error"] == tc.code {
				t.Fatalf("%s: 文がない %v", tc.code, got)
			}
		}
	}
	post("ja", `{"name":"demo","body":"{cam1.kbps}"}`, 201)
	if got := post("ja", `{"name":"demo","body":"a"}`, 409); got["code"] != "sources.duplicate" || !strings.Contains(got["error"].(string), "すでにあります") {
		t.Fatal(got)
	}
}

func uiTables(t *testing.T) (ja, en map[string]string) {
	t.Helper()
	return i18n.UI(i18n.JA), i18n.UI(i18n.EN)
}

// S2-92: 文の表の鍵は日本語と英語でそろう（管理ページの区分 ui も）。
func TestUITablesHaveTheSameKeys(t *testing.T) {
	ja, en := uiTables(t)
	for key := range ja {
		if en[key] == "" {
			t.Errorf("英語の表にない: %s", key)
		}
	}
	for key := range en {
		if ja[key] == "" {
			t.Errorf("日本語の表にない: %s", key)
		}
	}
	for key, text := range ja {
		if got, want := strings.Count(text, "{"), strings.Count(en[key], "{"); got != want {
			t.Errorf("差し込みの数が違う: %s", key)
		}
	}
}

func readAll(t *testing.T, dir string, pattern string, skipTests bool) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, f := range files {
		if skipTests && strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
		all.WriteByte('\n')
	}
	return all.String()
}

// S2-92: 使わない鍵を残さない。鍵は管理ページの JS か、本体の Go のどちらかの文字列に出てくる。
// 画面が鍵を組み立てるもの（'setting_'+key など）は、前の部分で数える。
func TestUIKeysAreUsed(t *testing.T) {
	ja, _ := uiTables(t)
	source := readAll(t, "assets/app", "*.js", false)
	for _, dir := range []string{"../gui", "../i18n", "../app", "../web"} {
		source += readAll(t, dir, "*.go", true)
	}
	dynamic := []string{"setting_", "settings_group_", "settings_default_", "settings_regenerate_", "confirm_regenerate_", "mediamtx_", "sender_", "kind_", "theme_", "language_", "src_value_", "src_values_", "src_template_", "src_url_", "src_line_", "src_standby_", "src_stream_", "window_"}
	// 画面が 'settings_'+kind、'settings_'+state、'src_'+欄の名前 などで作る鍵と、本体の小さな窓が状態の値から作る鍵（down）。
	built := map[string]bool{"settings_added": true, "settings_removed": true, "settings_failed": true, "settings_reverted": true, "mediamtx_down": true, "settings_added_camera": true, "settings_removed_camera": true, "settings_save": true, "settings_reload": true, "settings_restart": true, "down": true,
		"src_title": true, "src_name": true, "src_obs_width": true, "src_obs_height": true, "src_editonly": true, "src_previewonly": true, "src_split": true, "src_horizontal": true, "src_vertical": true, "src_line": true, "src_standby": true, "src_stream": true}
	var unused []string
	for key := range ja {
		if built[key] || strings.Contains(source, "'"+key+"'") || strings.Contains(source, "\""+key+"\"") {
			continue
		}
		used := false
		for _, prefix := range dynamic {
			if strings.HasPrefix(key, prefix) {
				used = true
			}
		}
		if !used {
			unused = append(unused, key)
		}
	}
	sort.Strings(unused)
	if len(unused) > 0 {
		t.Fatalf("使っていない鍵: %v", unused)
	}
	// 項目の説明（_help）には、その項目の名前の鍵がある（名前を直して説明を忘れた鍵を残さない）。
	for key := range ja {
		if strings.HasPrefix(key, "setting_") && strings.HasSuffix(key, "_help") {
			if ja[strings.TrimSuffix(key, "_help")] == "" {
				t.Errorf("名前のない説明: %s", key)
			}
		}
	}
}

var (
	// 「切り替え」は名詞のときだけ「切替」にする（切り替える・切り替わる・切り替えて は動詞なので残す）。
	switchNoun = regexp.MustCompile(`切り替え(?:の|を|が|は|に|先|ボタン|中|$|。|、|\)|）|」)`)
	// 「ホスト名」は入力する中身の言い方なので残す。欄の名前としての「ホスト」は「アドレス」。
	hostWord = regexp.MustCompile(`ホスト(?:名)?`)
	appWord  = regexp.MustCompile(`(?:^|[^信])アプリ(?:[^名]|$)`)
)

// S2-92 C-6（spec 14章 v0.37）: 言葉をそろえる。本体・確認・捨てる・切替・アドレス・OBS の読み取り。
func TestWordingIsUnified(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.JA} {
		check := func(where, key, text string) {
			t.Helper()
			for _, bad := range []string{"確かめ", "破棄", "住所"} {
				if strings.Contains(text, bad) {
					t.Errorf("%s %s: 「%s」を使わない: %s", where, key, bad, text)
				}
			}
			// 本体を「アプリ」と呼ばない（「配信アプリ」「アプリ名」は別のもの）。
			if appWord.MatchString(text) {
				t.Errorf("%s %s: 「本体」にそろえる: %s", where, key, text)
			}
			if switchNoun.MatchString(text) {
				t.Errorf("%s %s: 「切替」にそろえる: %s", where, key, text)
			}
			for _, m := range hostWord.FindAllString(text, -1) {
				if m == "ホスト" {
					t.Errorf("%s %s: 「アドレス」にそろえる: %s", where, key, text)
				}
			}
		}
		for key, text := range i18n.UI(lang) {
			check("ui", key, text)
		}
		for _, code := range i18n.Codes {
			check("messages", code, i18n.Render(lang, i18n.New(code)))
		}
	}
	// 英語は本体を "the app" と呼ぶ（D-5）。"keeper" や "the application" にしない。
	for key, text := range i18n.UI(i18n.EN) {
		if strings.Contains(text, "application") || strings.Contains(text, "keeper") || strings.Contains(text, "＊") {
			t.Errorf("en %s: %s", key, text)
		}
	}
	ja, _ := uiTables(t)
	for key, want := range map[string]string{
		"settings_restart_note": "＊ 保存すると本体が再起動します",
		"settings_group_stream": "OBS の読み取り",
		"setting_stream":        "OBS の読み取り",
		"settings_restart":      "本体を再起動する",
		"sender_endpoint":       "アドレス",
		"setting_host":          "アドレス",
		"setting_publish_host":  "送信用アドレス",
		"setting_obs_read_host": "OBS 用アドレス",
		"sender_address":        "送り先のアドレス",
		"settings_discard":      "保存していない変更を捨てますか？",
		"src_discard":           "保存していない変更を捨てますか？",
	} {
		if ja[key] != want {
			t.Errorf("%s: %q（%q のはず）", key, ja[key], want)
		}
	}
}

// S2-92 C-7: ボタンは動詞の形（「保存」「削除」「編集」「コピー」「条件」の名詞だけをやめる）。カメラを足すボタンは「追加」のまま。
func TestButtonsAreVerbs(t *testing.T) {
	ja, _ := uiTables(t)
	for key, want := range map[string]string{
		"settings_save": "保存する", "src_save": "保存する", "settings_remove": "削除する", "src_delete": "削除する",
		"src_edit": "編集する", "sender_copy": "コピーする", "src_condition": "条件を差し込む", "settings_add": "追加",
		"confirm_yes": "はい", "confirm_no": "いいえ",
	} {
		if ja[key] != want {
			t.Errorf("%s: %q（%q のはず）", key, ja[key], want)
		}
	}
}

// S2-92 C-12・D-19: 日本語の表にも英語の生の文を置かない。どうすればよいかを言う誤りには次の操作がある。
func TestErrorsSayWhatToDo(t *testing.T) {
	for _, code := range []string{"api.deadline", "api.canceled", "api.disconnected", "api.stopped", "api.internal", "sources.write_failed", "source_files.io"} {
		for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
			text := i18n.Render(lang, i18n.New(code))
			if strings.Contains(text, "context ") || text == code {
				t.Errorf("%s/%s: %s", lang, code, text)
			}
		}
	}
	if got := i18n.Render(i18n.EN, i18n.New("api.unauthorized")); got != "Please sign in." {
		t.Fatal(got)
	}
	if got := i18n.Render(i18n.EN, i18n.New("api.deadline")); got != "Timed out. Try again." {
		t.Fatal(got)
	}
	if got := i18n.Render(i18n.EN, i18n.New("api.stopped")); got != "The app has stopped. Restart it." {
		t.Fatal(got)
	}
	for code, want := range map[string]string{"api.deadline": "もう一度試してください", "api.disconnected": "確認してください", "failed": "もう一度試してください"} {
		if code == "failed" {
			if ja, _ := uiTables(t); !strings.Contains(ja["failed"], want) {
				t.Errorf("failed: %s", ja["failed"])
			}
			continue
		}
		if got := i18n.Render(i18n.JA, i18n.New(code)); !strings.Contains(got, want) {
			t.Errorf("%s: %s", code, got)
		}
	}
}

// S2-92 C-10: 知らせと誤りの文は、です・ます調で「。」で終わる（差し込みで終わる文と、ほかの文に入る断片は除く）。
func TestMessagesAreSentences(t *testing.T) {
	for _, code := range i18n.Codes {
		fragment := strings.HasPrefix(code, "purpose.") || strings.HasPrefix(code, "api.reason_") || strings.HasPrefix(code, "hidden.version") || strings.HasPrefix(code, "hidden.entry") ||
			strings.HasPrefix(code, "banner.") || strings.HasPrefix(code, "console.") || strings.HasPrefix(code, "service.") || strings.HasPrefix(code, "source_files.reason_") || code == "external.detail"
		if fragment {
			continue
		}
		text := i18n.Render(i18n.JA, i18n.New(code))
		if strings.HasSuffix(text, "}") || strings.HasSuffix(text, "。") || strings.HasSuffix(text, "？") {
			continue
		}
		// 差し込みの値で終わる文（「…できません: {error}」）は、値の後に「。」を付けない（上の "}" で終わる文）。
		t.Errorf("%s: 「。」で終わらない: %s", code, text)
	}
}
