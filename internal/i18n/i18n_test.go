package i18n

import (
	"reflect"
	"sort"
	"testing"
)

func TestRender(t *testing.T) {
	for _, tc := range []struct {
		lang Lang
		msg  Msg
		want string
	}{
		{JA, New("obs.media", "name", "IRL"), "IRL が映っていない可能性があります"},
		{EN, New("obs.media", "name", "IRL"), "IRL may not be visible."},
		{"fr", New("obs.media", "name", "{name}"), "{name} may not be visible."},
		{JA, New("unknown"), "unknown"},
	} {
		if got := Render(tc.lang, tc.msg); got != tc.want {
			t.Errorf("Render = %q, want %q", got, tc.want)
		}
	}
}
func TestSelect(t *testing.T) {
	for _, tc := range []struct {
		lang, accept string
		want         Lang
	}{
		{"ja", "en", JA}, {"en", "ja", EN}, {"xx", "ja-JP", JA}, {"", "en-GB", EN},
		{"", "fr", EN}, {"", "", EN}, {"", "fr,ja-JP,en", JA},
		{"", "en-US,en;q=0.9", EN}, {"", "ja;q=0.2,en;q=0.9", EN},
		{"", "ja;q=0,en;q=0", EN}, {"", "ja;q=bad,en", EN}, {"xx", "", EN},
		{"", "ja;q=0.8,en;q=0.8", JA}, {"", "fr;q=1,ja;q=0.5", JA},
	} {
		if got := Select(tc.lang, tc.accept); got != tc.want {
			t.Errorf("Select(%q,%q)=%q, want %q", tc.lang, tc.accept, got, tc.want)
		}
	}
}
func TestCatalogs(t *testing.T) {
	seen := map[string]bool{}
	for _, code := range Codes {
		if seen[code] {
			t.Errorf("code 重複: %s", code)
		}
		seen[code] = true
		var names [][]string
		for _, lang := range []Lang{JA, EN} {
			s, ok := catalogs[lang][code]
			if !ok {
				t.Errorf("%s に %s がない", lang, code)
			}
			var ns []string
			for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
				ns = append(ns, m[1])
			}
			sort.Strings(ns)
			names = append(names, ns)
		}
		if !reflect.DeepEqual(names[0], names[1]) {
			t.Errorf("差し込み不一致: %s", code)
		}
	}
	for lang, c := range catalogs {
		for code := range c {
			if !seen[code] {
				t.Errorf("一覧にない code: %s/%s", lang, code)
			}
		}
	}
}

func TestEnglishFallback(t *testing.T) {
	catalogs[EN]["test.fallback"] = "Hello {name}"
	defer delete(catalogs[EN], "test.fallback")
	if got := Render(JA, New("test.fallback", "name", "IRL")); got != "Hello IRL" {
		t.Fatal(got)
	}
}

func TestLegacyRoundTrip(t *testing.T) {
	for _, code := range Codes {
		if code == "external.detail" {
			continue
		}
		m := New(code)
		for _, name := range placeholder.FindAllStringSubmatch(catalogs[JA][code], -1) {
			m.Values[name[1]] = "test"
		}
		text := Render(JA, m)
		got := FromJapanese(text)
		if got.Code != code || Render(JA, got) != text {
			t.Errorf("互換変換: %s → %+v", code, got)
		}
	}
}

func TestNestedReason(t *testing.T) {
	m := New("hidden.load")
	m.Values["error"] = New("hidden.version", "version", "99")
	if got := Render(EN, m); got != "Cannot read hidden.json (Unknown version 99). Using an empty record. The old file was saved as hidden.json.broken." {
		t.Fatal(got)
	}
	if got := Render(JA, m); got != "hidden.json を読めなかったため（知らない版 99）、空として扱います。元のファイルは hidden.json.broken に残しました" {
		t.Fatal(got)
	}
}
