package sources

import "sort"

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
func allowedAssetRune(c rune) bool {
	rs := htmlRules.Asset.AllowedRanges
	i := sort.Search(len(rs), func(i int) bool { return rs[i][1] >= c })
	return i < len(rs) && rs[i][0] <= c
}
