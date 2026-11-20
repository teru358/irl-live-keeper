package redact

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// fragments は s の2バイトの断片を並べる。
func fragments(s string) []string {
	var out []string
	for i := 0; i+2 <= len(s); i++ {
		out = append(out, s[i:i+2])
	}
	return out
}

// refMask は、一致した区間をバイトごとに数えて作る基準の実装。merge の結果と比べるために使う。
func refMask(line string, pats []string) string {
	covered := make([]bool, len(line))
	for _, p := range pats {
		for j := 0; j+len(p) <= len(line); j++ {
			if line[j:j+len(p)] == p {
				for k := j; k < j+len(p); k++ {
					covered[k] = true
				}
			}
		}
	}
	var b strings.Builder
	for i := 0; i < len(line); {
		if !covered[i] {
			b.WriteByte(line[i])
			i++
			continue
		}
		b.WriteString(mask)
		for i < len(line) && covered[i] {
			i++
		}
	}
	return b.String()
}

func TestMask(t *testing.T) {
	cases := []struct {
		name    string
		secrets []string
		line    string
		want    string
	}{
		{"秘密がなければ行はそのまま", nil, "abc", "abc"},
		{"空の秘密は無視する（空の文字列はどこにでも一致してしまう）", []string{""}, "abc", "abc"},
		{"空の秘密と本物の秘密が混ざっても、本物だけ置き換える", []string{"", "S"}, "xSy", "x***y"},
		{"1つの秘密", []string{"SECRET123"}, "pass=SECRET123 port=1935", "pass=*** port=1935"},
		{"行に秘密がなければそのまま", []string{"SECRET123"}, "port=1935", "port=1935"},
		{"URL の中の秘密は、その部分だけ", []string{"SECRET123"},
			"srt://host:8890?streamid=publish:live:irl:SECRET123", "srt://host:8890?streamid=publish:live:irl:***"},
		{"秘密が行の先頭と末尾にある", []string{"ab"}, "abXab", "***X***"},
		{"部分文字列: 短い秘密を先に渡しても長い方が全部 ***", []string{"abc", "abcdef"}, "xabcdefy", "x***y"},
		{"部分文字列: 長い秘密を先に渡しても同じ", []string{"abcdef", "abc"}, "xabcdefy", "x***y"},
		{"途中で重なる2つの秘密（xab と bcd）", []string{"xab", "bcd"}, "xabcd", "***"},
		{"重なる2つの秘密を逆の順で渡しても同じ", []string{"bcd", "xab"}, "xabcd", "***"},
		{"重なりが連鎖する3つの秘密", []string{"ab", "bc", "cd"}, "abcd", "***"},
		{"隣り合う2つの秘密は1つの *** になる", []string{"ab", "cd"}, "abcd", "***"},
		{"離れた2つの秘密はそれぞれ ***", []string{"ab", "cd"}, "abXcd", "***X***"},
		{"同じ秘密が1行に2回", []string{"SECRET"}, "SECRET-SECRET", "***-***"},
		{"秘密が自分自身と重なる（aa は aaa で2回）", []string{"aa"}, "xaaay", "x***y"},
		{"秘密が自分自身と続けて重なる（aa は aaaa で3回）", []string{"aa"}, "aaaa", "***"},
		{"多バイト文字の秘密", []string{"秘密ab"}, "x秘密aby", "x***y"},
		{"引用される形の秘密（生の形）", []string{`p"w\d`}, `p"w\d`, "***"},
		{"引用される形の秘密（slog が引用してエスケープした形）", []string{`p"w\d`}, `pass="p\"w\\d"`, `pass="***"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := New(tc.secrets...).Mask(tc.line)
			if got != tc.want {
				t.Fatalf("Mask(%q) = %q, want %q", tc.line, got, tc.want)
			}
			// 置き換えたあとに、どの秘密の2文字以上の断片も残らない（期待の行にも無いもの）
			for _, s := range tc.secrets {
				for _, f := range fragments(s) {
					if strings.Contains(got, f) && !strings.Contains(tc.want, f) {
						t.Errorf("秘密 %q の断片 %q が残った: %q", s, f, got)
					}
				}
			}
		})
	}
}

func TestMaskNilAndNoSecrets(t *testing.T) {
	var nilMasker *Masker
	if got := nilMasker.Mask("pass=abc"); got != "pass=abc" {
		t.Errorf("nil の Masker: Mask = %q, want そのまま", got)
	}
	if got := New().Mask("pass=abc"); got != "pass=abc" {
		t.Errorf("秘密なし: Mask = %q, want そのまま", got)
	}
}

// 乱数で作った行と秘密で、区間の併合が基準の実装と一致することを確かめる。
// 文字は少数に絞り、重なりと隣接が多く起きるようにする。" と \ は引用形を作るために入れる。
func TestMaskMatchesReferenceOnRandomInput(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	const alphabet = "abc\"\\"
	randStr := func(minLen, maxLen int) string {
		n := minLen + rng.IntN(maxLen-minLen+1)
		b := make([]byte, n)
		for i := range b {
			b[i] = alphabet[rng.IntN(len(alphabet))]
		}
		return string(b)
	}
	for range 3000 {
		var secrets []string
		for range rng.IntN(4) {
			secrets = append(secrets, randStr(0, 4)) // 空の秘密も混ぜる
		}
		m := New(secrets...)
		line := randStr(0, 24)
		got := m.Mask(line)
		want := refMask(line, m.patterns)
		if got != want {
			t.Fatalf("秘密 %q、行 %q: Mask = %q, want %q", secrets, line, got, want)
		}
	}
}
