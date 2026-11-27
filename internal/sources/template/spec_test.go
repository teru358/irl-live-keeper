package template

import (
	"fmt"
	"testing"
	"time"
)

// 第9章「字句」「文法」: 入力と出力で各規則を区別する。
func TestSpecLexicalAndGrammar(t *testing.T) {
	cases := []struct {
		rule  string
		input string
		want  string
	}{
		{rule: "字句: 左から読む", input: "{{{a}}}", want: "{10}"},
		{rule: "字句: 二重の閉じ括弧", input: "}}", want: "}"},
		{rule: "字句: タグの前後の空白とタブ", input: "{ \ta\t }", want: "10"},
		{rule: "字句: 引用符の復号", input: `{if text == "a\"b\\c{}"}Y{end}`, want: "Y"},
		{rule: "字句: 予約語は名前全体で一致", input: "{if nota}Y{end}", want: "Y"},
		{rule: "字句: 数値の負号と小数", input: "{if -1.5 < 0}Y{end}", want: "Y"},
		{rule: "文法: 比較はnotより先 900", input: "{if not camera.kbps < 800}Y{else}N{end}", want: "Y"},
		{rule: "文法: notはandより先", input: "{if not false and false}Y{else}N{end}", want: "N"},
		{rule: "文法: andはorより先", input: "{if true or false and false}Y{end}", want: "Y"},
		{rule: "文法: 括弧でnotの範囲を限定", input: "{if (not a) == false}Y{end}", want: "Y"},
		{rule: "文法: 括弧でorを先にする", input: "{if (true or false) and false}Y{else}N{end}", want: "N"},
		{rule: "文法: notの再帰", input: "{if not not true}Y{end}", want: "Y"},
		{rule: "文法: 最初の真のelifだけ選ぶ", input: "{if false}A{elif true}B{elif true}C{else}D{end}", want: "B"},
		{rule: "文法: 全部偽ならelse", input: "{if false}A{elif false}B{else}C{end}", want: "C"},
		{rule: "文法: else省略", input: "{if false}A{end}B", want: "B"},
		{rule: "文法: 入れ子", input: "{if true}{if false}A{else}B{end}C{end}", want: "BC"},
		{rule: "制御行: 同じ行に複数タグなら保持", input: " {if true}{end} \n", want: "  \n"},
		{rule: "制御行: 差し込みだけの行は保持", input: " {a} \n", want: " 10 \n"},
	}
	values := map[string]Value{
		"a":           {Defined: true, Type: Number, Number: 10},
		"text":        {Defined: true, Type: String, String: "a\"b\\c{}"},
		"nota":        boolean(true),
		"camera.kbps": {Defined: true, Type: Number, Number: 900},
	}
	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) { run(t, c.input, c.want, values) })
	}
	t.Run("文法: 比較はnotより先 700", func(t *testing.T) {
		values["camera.kbps"] = Value{Defined: true, Type: Number, Number: 700}
		run(t, "{if not camera.kbps < 800}Y{else}N{end}", "N", values)
	})
}

// 第9章「型」: 全型の真偽、未定義、差し込みの既定と明示の形。
func TestSpecTruth(t *testing.T) {
	cases := []struct {
		rule  string
		value Value
		want  string
	}{
		{"数値0", Value{Defined: true, Type: Number}, "N"},
		{"負の数値", Value{Defined: true, Type: Number, Number: -1}, "Y"},
		{"空文字", Value{Defined: true, Type: String}, "N"},
		{"文字列", Value{Defined: true, Type: String, String: "false"}, "Y"},
		{"偽", boolean(false), "N"},
		{"真", boolean(true), "Y"},
		{"秒0", Value{Defined: true, Type: Seconds}, "N"},
		{"小数秒", Value{Defined: true, Type: Seconds, Number: 0.4}, "Y"},
		{"時刻", Value{Defined: true, Type: Clock}, "Y"},
		{"未定義", Value{}, "N"},
	}
	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) {
			p, err := Compile("{if x}Y{else}N{end}", map[string]Definition{"x": {Type: c.value.Type}})
			if err != nil {
				t.Fatal(err)
			}
			if got := p.Render(map[string]Value{"x": c.value}); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

// 第9章「比較」: 型の組み合わせ×全演算子。期待値は演算せず表に固定する。
func TestSpecComparisonTable(t *testing.T) {
	number := Value{Defined: true, Type: Number, Number: 2}
	seconds := Value{Defined: true, Type: Seconds, Number: 3}
	text := Value{Defined: true, Type: String, String: "x"}
	b := boolean(true)
	missing := Value{}
	cases := []struct {
		rule        string
		left, right Value
		// 順に == != < <= > >=
		want string
	}{
		{"数値と秒", number, seconds, "FTTTFF"},
		{"秒と数値", seconds, number, "FTFFTT"},
		{"数値と文字列", number, text, "FTFFFF"},
		{"文字列と数値", text, number, "FTFFFF"},
		{"数値と真偽", number, b, "FTFFFF"},
		{"真偽と数値", b, number, "FTFFFF"},
		{"秒と文字列", seconds, text, "FTFFFF"},
		{"文字列と秒", text, seconds, "FTFFFF"},
		{"秒と真偽", seconds, b, "FTFFFF"},
		{"真偽と秒", b, seconds, "FTFFFF"},
		{"文字列と真偽", text, b, "FTFFFF"},
		{"真偽と文字列", b, text, "FTFFFF"},
		{"等しい文字列の大小も偽", text, text, "TFFFFF"},
		{"等しい真偽の大小も偽", b, b, "TFFFFF"},
		{"等しい数値", number, number, "TFFTFT"},
		{"等しい秒", seconds, seconds, "TFFTFT"},
		{"異なる文字列", text, Value{Defined: true, Type: String, String: "y"}, "FTFFFF"},
		{"異なる真偽", b, boolean(false), "FTFFFF"},
		{"未定義どうし", missing, missing, "FFFFFF"},
	}
	for _, value := range []Value{number, seconds, text, b} {
		cases = append(cases,
			struct {
				rule        string
				left, right Value
				want        string
			}{fmt.Sprintf("未定義が左/%d", value.Type), missing, value, "FFFFFF"},
			struct {
				rule        string
				left, right Value
				want        string
			}{fmt.Sprintf("未定義が右/%d", value.Type), value, missing, "FFFFFF"},
		)
	}
	for _, c := range cases {
		for i, op := range []string{"==", "!=", "<", "<=", ">", ">="} {
			t.Run(c.rule+op, func(t *testing.T) {
				p, err := Compile("{if x "+op+" y}T{else}F{end}", map[string]Definition{"x": {Type: c.left.Type}, "y": {Type: c.right.Type}})
				if err != nil {
					t.Fatal(err)
				}
				if got := p.Render(map[string]Value{"x": c.left, "y": c.right}); got != string(c.want[i]) {
					t.Fatalf("got %s want %c", got, c.want[i])
				}
			})
		}
	}
}

// 第9章「比較」: 時刻を片側だけに含む場合も保存時に拒否する。
func TestSpecClockComparisonErrors(t *testing.T) {
	for _, op := range []string{"==", "!=", "<", "<=", ">", ">="} {
		for _, operands := range [][2]string{{"now", "now"}, {"now", "1"}, {"1", "now"}, {"now", "text"}, {"text", "now"}, {"now", "b"}, {"b", "now"}, {"now", "sec"}, {"sec", "now"}} {
			input := "{if " + operands[0] + " " + op + " " + operands[1] + "}Y{end}"
			t.Run(input, func(t *testing.T) {
				_, err := Compile(input, defs)
				e, ok := err.(*Error)
				if !ok || e.Message.Code != "template.comparison" {
					t.Fatalf("want comparison error, got %v", err)
				}
			})
		}
	}
}

// 第9章「型」: 秒は比較では小数を保ち、全形式で切り捨てる。
func TestSpecFormatsAndFraction(t *testing.T) {
	values := map[string]Value{
		"a":   {Defined: true, Type: Number, Number: -1.25},
		"sec": {Defined: true, Type: Seconds, Number: 10.4},
		"now": {Defined: true, Type: Clock, Time: time.Date(2026, 1, 1, 0, 5, 9, 0, time.Local)},
	}
	cases := []struct{ rule, input, want string }{
		{"負の数も四捨五入", "{a:1}", "-1.3"},
		{"秒の既定", "{sec}", "0:00:10"},
		{"秒hms", "{sec:hms}", "0:00:10"},
		{"秒ms", "{sec:ms}", "0:10"},
		{"秒s", "{sec:s}", "10"},
		{"秒の小数比較", "{if sec>10}Y{end}", "Y"},
		{"時刻の既定", "{now}", "00:05"},
		{"時刻hm", "{now:hm}", "00:05"},
		{"時刻hms", "{now:hms}", "00:05:09"},
		{"未定義の差し込み", "[{text}]", "[]"},
		{"未定義のnot", "{if not text}Y{end}", "Y"},
		{"未定義比較のnot", "{if not (text != 5)}Y{end}", "Y"},
	}
	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) { run(t, c.input, c.want, values) })
	}
}

// 第9章「字句」「文法」「誤りと文字の扱い」: 保存時に拒否する入力。
func TestSpecSaveErrors(t *testing.T) {
	cases := []struct{ rule, input, code string }{
		{"字句: 対のない閉じ括弧", "}", "syntax"},
		{"字句: タグ内の開き括弧", "{if {a}}", "syntax"},
		{"字句: 閉じていないタグ", "{a", "syntax"},
		{"字句: 許されないエスケープ", `{if "\t"}Y{end}`, "syntax"},
		{"字句: 文字列内のLF", "{if \"a\nb\"}Y{end}", "syntax"},
		{"字句: 文字列内のCR", "{if \"a\rb\"}Y{end}", "syntax"},
		{"字句: 小数点の後に数字が必要", "{if 1.}Y{end}", "syntax"},
		{"字句: 小数点の前に数字が必要", "{if .5}Y{end}", "syntax"},
		{"字句: 数値と予約語の区切り", "{if 1and true}Y{end}", "syntax"},
		{"字句: 名前の各区間に先頭文字が必要", "{a.1}", "syntax"},
		{"文法: 空のタグ", "{}", "syntax"},
		{"文法: 予約語は差し込めない", "{true}", "syntax"},
		{"文法: 比較を連結しない", "{if a<2<3}Y{end}", "syntax"},
		{"文法: 比較の右辺にnotを直接書かない", "{if a == not b}Y{end}", "syntax"},
		{"文法: 括弧の閉じ忘れ", "{if (a}Y{end}", "syntax"},
		{"文法: endが必要", "{if true}Y", "syntax"},
		{"文法: 単独のelif", "{elif true}", "syntax"},
		{"文法: elseの後にelifは置けない", "{if true}{else}{elif true}{end}", "syntax"},
		{"文法: elseに引数はない", "{if true}{else true}{end}", "syntax"},
		{"保存時: 実行されない分岐も名前を検証", "{if false}{unknown}{end}", "unknown"},
		{"保存時: 数値の形の上限", "{a:7}", "format"},
		{"保存時: 数値の形は1桁", "{a:00}", "format"},
		{"保存時: 文字列に形を付けない", "{text:hms}", "format"},
		{"保存時: 真偽に形を付けない", "{b:0}", "format"},
		{"保存時: 秒に数値の形を付けない", "{sec:0}", "format"},
		{"保存時: 時刻にmsは付けない", "{now:ms}", "format"},
	}
	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) {
			_, err := Compile(c.input, defs)
			e, ok := err.(*Error)
			if !ok || e.Message.Code != "template."+c.code {
				t.Fatalf("input %q: want %s, got %v", c.input, c.code, err)
			}
		})
	}
}

// 第9章「誤りと文字の扱い」: 行・列は1始まりのコードポイント数。
func TestSpecErrorPositions(t *testing.T) {
	cases := []struct {
		input        string
		line, column int
		code         string
	}{
		{"全角🙂}", 1, 4, "syntax"},
		{"前\n全角🙂}", 2, 4, "syntax"},
		{"前\r\n全角{unknown}", 2, 4, "unknown"},
		{"全角{if true}\n全角{text:0}{end}", 2, 4, "format"},
		{"全角\n {if true}\n全角}\n{end}", 3, 3, "syntax"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			_, err := Compile(c.input, defs)
			e, ok := err.(*Error)
			if !ok {
				t.Fatalf("want Error, got %v", err)
			}
			if e.Line != c.line || e.Column != c.column || e.Message.Code != "template."+c.code {
				t.Fatalf("got %d:%d %s, want %d:%d %s", e.Line, e.Column, e.Message.Code, c.line, c.column, c.code)
			}
		})
	}
}

// 第9章「型」: 数値の各形式0〜6と四捨五入。
func TestSpecNumberFormats(t *testing.T) {
	cases := []struct{ input, want string }{
		{"{a:0}", "1"},
		{"{a:1}", "1.2"},
		{"{a:2}", "1.23"},
		{"{a:3}", "1.235"},
		{"{a:4}", "1.2346"},
		{"{a:5}", "1.23457"},
		{"{a:6}", "1.234568"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			run(t, c.input, c.want, map[string]Value{"a": {Defined: true, Type: Number, Number: 1.2345678}})
		})
	}
}

// 9章「字句」: 形の名前は差し込みのタグの `:` のすぐ後ろに書く。空白をはさむと保存のときの誤り
func TestFormatNameMustFollowColon(t *testing.T) {
	for _, src := range []string{"{a: 0}", "{a:\t1}", "{sec: hms}"} {
		if _, err := Compile(src, defs); err == nil {
			t.Errorf("%q: 誤りにならない", src)
		}
	}
	for _, src := range []string{"{a:0}", "{ a:1 }", "{sec:hms}"} {
		if _, err := Compile(src, defs); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
