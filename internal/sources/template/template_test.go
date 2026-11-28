package template

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var defs = map[string]Definition{
	"a":         {Number, ""},
	"b":         {Bool, ""},
	"text":      {String, ""},
	"sec":       {Seconds, ""},
	"now":       {Clock, ""},
	"cam1.kbps": {Number, ""},
	"nota":      {Bool, ""},
}

func run(t *testing.T, s, want string, v map[string]Value) {
	t.Helper()
	p, e := Compile(s, defs)
	if e != nil {
		t.Fatalf("%q: %v", s, e)
	}
	if got := p.Render(v); got != want {
		t.Fatalf("%q: got %q want %q", s, got, want)
	}
}

// 第9章「字句」「文法」「型」: 差し込みと条件分岐の入出力。
func TestLanguage(t *testing.T) {
	v := map[string]Value{
		"a":    {Defined: true, Type: Number, Number: 10.4},
		"b":    boolean(false),
		"sec":  {Defined: true, Type: Seconds, Number: 5025.9},
		"text": {Defined: true, Type: String, String: "<script>&"},
		"now":  {Defined: true, Type: Clock, Time: time.Date(2026, 1, 1, 19, 30, 5, 0, time.Local)}}
	for _, c := range []struct{ s, w string }{
		{"{a}}}", "10}"},
		{"{{a}}", "{a}"},
		{"{if a<11}Y{end}", "Y"},
		{"{if nota}Y{else}N{end}", "N"},
		{"{if (not a) == b}Y{end}", "Y"},
		{"{if false or true and false}N{else}Y{end}", "Y"},
		{"{if (false or true) and true}Y{end}", "Y"},
		{"{if 10<a}Y{end}", "Y"},
		{`{if "x" != text}Y{end}`, "Y"},
		{"{if false}N{elif true}{if b}N{else}Y{end}{else}N{end}", "Y"},
		{"前\n \t{if true}\t\n中\n {else}\n後\n {end}\n末", "前\n中\n末"},
		{"{if true} Y {end}", " Y "},
		{" {if true}\r\nY\n {end}", "Y\n"},
		{"{sec}|{sec:ms}|{sec:s}", "1:23:45|83:45|5025"},
		{"{now}|{now:hms}", "19:30|19:30:05"},
		{"{a}|{a:1}|{a:2}|{a:3}|{a:4}|{a:5}|{a:6}", "10|10.4|10.40|10.400|10.4000|10.40000|10.400000"},
		{"{text}|{b}", "<script>&|false"},
		{"{if sec>5025}Y{end}", "Y"},
		{"{cam1.kbps}{if not cam1.kbps}Y{end}{if not (cam1.kbps != 5)}Y{end}", "YY"},
	} {
		t.Run(c.s, func(t *testing.T) { run(t, c.s, c.w, v) })
	}
}

// 第9章「誤りと文字の扱い」: 保存時の検証。
func TestErrors(t *testing.T) {
	for _, s := range []string{"}",
		"{a",
		"{if {a}}",
		"{if a < 2 < 3}x{end}",
		"{if true}",
		"{end}",
		"{else}",
		"{if true}{else}{else}{end}",
		"{if true}{end x}",
		"{unknown}",
		"{if unknown}x{end}",
		"{text:0}",
		"{a:hms}",
		"{b:s}",
		"{sec:0}",
		"{now:ms}",
		"{if now == now}x{end}",
		"{if 1now}x{end}",
		`{if text == "\n"}x{end}`,
		"{if text == \"a\nb\"}x{end}",
		"{a..b}",
		"{a:7}",
		"{if 1and true}x{end}",
		`{sec:"hms"}`} {
		t.Run(s, func(t *testing.T) {
			if _, e := Compile(s, defs); e == nil {
				t.Fatalf("accepted %q", s)
			}
		})
	}
	_, err := Compile("全角\nあ}", defs)
	e := err.(*Error)
	if e.Line != 2 || e.Column != 2 || e.Message.Code != "template.syntax" {
		t.Fatal(e)
	}
}

// 第9章「比較」: 同値の全型と未定義を全演算子で確認。
func TestComparisonMatrix(t *testing.T) {
	vals := []Value{{Defined: true, Type: Number, Number: 2},
		{Defined: true, Type: Seconds, Number: 2},
		{Defined: true, Type: String, String: "x"}, boolean(true), {}}
	for i, a := range vals {
		for j, b := range vals {
			for _, op := range []string{"==", "!=", "<", "<=", ">", ">="} {
				t.Run(fmt.Sprintf("%d/%d/%s", i, j, op), func(t *testing.T) {
					d := map[string]Definition{"x": {a.Type, ""}, "y": {b.Type, ""}}
					p, e := Compile("{if x "+op+" y}T{else}F{end}", d)
					if e != nil {
						t.Fatal(e)
					}
					want := false
					if a.Defined && b.Defined {
						same := i == j || i < 2 && j < 2
						switch op {
						case "==":
							want = same
						case "!=":
							want = !same
						case "<=", ">=":
							want = i < 2 && j < 2
						}
					}
					w := "F"
					if want {
						w = "T"
					}
					if got := p.Render(map[string]Value{"x": a, "y": b}); got != w {
						t.Fatalf("got %s want %s", got, w)
					}
				})
			}
		}
	}
}

// 第9章「型」: 定義ごとの数値の既定形式。
func TestDefaults(t *testing.T) {
	p, e := Compile("{n}|{s}|{c}", map[string]Definition{"n": {Number, "2"}, "s": {Seconds, ""}, "c": {Clock, ""}})
	if e != nil {
		t.Fatal(e)
	}
	got := p.Render(map[string]Value{"n": {Defined: true, Type: Number, Number: 1.235}, "s": {Defined: true, Type: Seconds, Number: 312.9}})
	if got != "1.24|0:05:12|" {
		t.Fatal(got)
	}
}
func FuzzCompile(f *testing.F) {
	for _, s := range []string{"",
		"{{a}}",
		"{a}}}",
		"{if a<2}{a}{else}全角{end}",
		`{if text == "\\"}x{end}`, "\xff}"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, e := Compile(s, defs)
		if e == nil {
			p.Render(nil)
		}
	})
}

// 第9章「文法」: 制御タグだけの行の空白と改行を除く。
func TestControlLines(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for _, pad := range []string{"", " ", "\t "} {
			t.Run(fmt.Sprintf("%q/%q", nl, pad), func(t *testing.T) {
				s := "前" + nl + pad + "{if false}" + pad + nl + "不可" + nl + pad + "{elif true}" + pad + nl + "可" + nl + pad + "{else}" + pad + nl + "不可" + nl + pad + "{end}" + pad + nl + "後"
				run(t, s, "前"+nl+"可"+nl+"後", nil)
			})
		}
	}
}

// 第9章「比較」: 数値の大小と文字列・真偽の大小禁止。
func TestNumericOperators(t *testing.T) {
	for _, c := range []struct{ expr, w string }{{"1<2", "Y"},
		{"1<=2", "Y"},
		{"2>1", "Y"},
		{"2>=1", "Y"},
		{"1==2", ""},
		{"1!=2", "Y"},
		{`"a"<"b"`, ""},
		{"false > true", ""},
		{"not not true", "Y"}} {
		run(t,
			"{if "+c.expr+"}Y{end}", c.w, nil)
	}
}

// 64 KiB の入力。制御タグのたびに本文の頭から行の頭を探し直さず、長さに比例する時間で読む。
func largeBodies() map[string]string {
	unit := "{if cam1.kbps > 0}a{end}"
	lines := "{if cam1.kbps > 0}\na\n{end}\n"
	return map[string]string{
		"one line":   strings.Repeat(unit, 65536/len(unit)),
		"with lines": strings.Repeat(unit+"\n", 65536/(len(unit)+1)),
		"control":    strings.Repeat(lines, 65536/len(lines)),
	}
}

func TestCompileLargeBodyIsFast(t *testing.T) {
	for name, s := range largeBodies() {
		t.Run(name, func(t *testing.T) {
			best := time.Hour
			for n := 0; n < 5; n++ {
				start := time.Now()
				if _, e := Compile(s, defs); e != nil {
					t.Fatal(e)
				}
				if d := time.Since(start); d < best {
					best = d
				}
			}
			t.Logf("%s: %d bytes, best of 5 %v", name, len(s), best)
			if best > 20*time.Millisecond {
				t.Fatalf("Compile of %d bytes took %v; want 20ms or less", len(s), best)
			}
		})
	}
}

func BenchmarkCompile64KiB(b *testing.B) {
	for name, s := range largeBodies() {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(s)))
			for i := 0; i < b.N; i++ {
				if _, e := Compile(s, defs); e != nil {
					b.Fatal(e)
				}
			}
		})
	}
}
