// Package redact は、ログの行に含まれる秘密を *** に置き換える。
// 標準出力、ログファイル、MediaMTX の出力のどれにも同じ規則を使う。
package redact

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// mask は秘密の代わりに書く文字列。
const mask = "***"

// Masker は秘密のパターンを持ち、行の中の秘密を置き換える。nil の Masker は何も置き換えない。
type Masker struct {
	patterns []string
}

// New は秘密から Masker を作る。空の秘密は無視する。
// slog の TextHandler は " や \ などを含む値を strconv.Quote の形で書くので、その形のパターンも加える。
func New(secrets ...string) *Masker {
	var pats []string
	for _, s := range secrets {
		if s == "" {
			continue
		}
		pats = append(pats, s)
		if q := strconv.Quote(s); q[1:len(q)-1] != s {
			pats = append(pats, q[1:len(q)-1])
		}
	}
	return &Masker{patterns: pats}
}

// span は行の中の区間 [from, to)。
type span struct{ from, to int }

// Mask は line の中で、いずれかのパターンに一致する箇所をすべて *** に置き換えた行を返す。
// 一致は重なるものも含めてすべて集め、重なる区間と隣り合う区間を1つにまとめてから置き換える。
// そのため、途中で重なる2つの秘密の、どちらの文字も残らない。
// Masker が nil のとき、または秘密が1つもないときは line をそのまま返す。
func (m *Masker) Mask(line string) string {
	if m == nil || len(m.patterns) == 0 {
		return line
	}

	var spans []span
	for _, p := range m.patterns {
		// 1バイトずつずらして探し、重なる一致も漏らさない
		for from := 0; from < len(line); {
			i := strings.Index(line[from:], p)
			if i < 0 {
				break
			}
			start := from + i
			spans = append(spans, span{start, start + len(p)})
			from = start + 1
		}
	}
	if len(spans) == 0 {
		return line
	}

	slices.SortFunc(spans, func(a, b span) int { return cmp.Compare(a.from, b.from) })
	merged := spans[:1]
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.from <= last.to { // 重なる、または隣り合う
			last.to = max(last.to, s.to)
		} else {
			merged = append(merged, s)
		}
	}

	var b strings.Builder
	prev := 0
	for _, s := range merged {
		b.WriteString(line[prev:s.from])
		b.WriteString(mask)
		prev = s.to
	}
	b.WriteString(line[prev:])
	return b.String()
}
