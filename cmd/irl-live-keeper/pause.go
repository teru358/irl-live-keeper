package main

import (
	"bufio"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"io"
)

// shouldPause は、エラー終了の前に Enter を待つかどうか。
// 終了コードが 0 以外で、かつコンソールにつながっているのが自分だけのときに true。
// エクスプローラーからのダブルクリックでは、自分のためにコンソールが作られ、終了と同時に閉じて
// 理由が読めない。ターミナルから起動したときはシェルもつながっているので待たない。
// 数えられないときは待たない。
func shouldPause(code int, consoleProcs func() (int, error)) bool {
	if code == 0 {
		return false
	}
	n, err := consoleProcs()
	if err != nil {
		return false
	}
	return n == 1
}

// waitEnter は案内を out に出し、in から1行読む。読めなくても返る。
func waitEnter(in io.Reader, out io.Writer) {
	waitEnterLanguage(in, out, i18n.JA)
}

func waitEnterLanguage(in io.Reader, out io.Writer, lang i18n.Lang) {
	fmt.Fprintln(out, i18n.Render(lang, i18n.New("console.pause")))
	_, _ = bufio.NewReader(in).ReadString('\n')
}
