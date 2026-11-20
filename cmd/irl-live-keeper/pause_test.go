package main

import (
	"errors"
	"strings"
	"testing"
)

func TestShouldPause(t *testing.T) {
	cases := []struct {
		name  string
		code  int
		procs int
		err   error
		want  bool
	}{
		{"正常終了は待たない", 0, 1, nil, false},
		{"失敗で自分だけなら待つ", 1, 1, nil, true},
		{"失敗でシェルもいれば待たない", 1, 2, nil, false},
		{"失敗で3つ以上でも待たない", 1, 3, nil, false},
		{"数えられなければ待たない", 1, 0, errors.New("x"), false},
		{"0 個は待たない", 1, 0, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldPause(tc.code, func() (int, error) { return tc.procs, tc.err })
			if got != tc.want {
				t.Errorf("shouldPause = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWaitEnterは案内を出して1行読む(t *testing.T) {
	var out strings.Builder
	waitEnter(strings.NewReader("\n"), &out)
	if !strings.Contains(out.String(), "Enter") {
		t.Errorf("案内がない: %q", out.String())
	}
	// 標準入力が読めなくても終わる
	waitEnter(strings.NewReader(""), &out)
}
