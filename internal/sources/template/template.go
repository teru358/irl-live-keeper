// Package template はソースのテンプレートを検証し、平文に組み立てる。
// 出力をページに入れる側は textContent などを使うこと。
package template

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teru358/irl-live-keeper/internal/i18n"
)

type Type uint8

const (
	Number Type = iota
	String
	Bool
	Seconds
	Clock
)

// Definition は型と既定の形。数値の空の形は 0 桁になる。
type Definition struct {
	Type   Type
	Format string
}

// Value のゼロ値は未定義。Type はコンパイル時の定義に合わせる。
type Value struct {
	Defined bool
	Type    Type
	Number  float64
	String  string
	Bool    bool
	Time    time.Time
}
type Error struct {
	Line, Column int
	Message      i18n.Msg
}

func (e *Error) Error() string {
	return i18n.Render(i18n.JA, e.Message)
}

type token struct {
	kind, text string
	pos        int
}
type part struct {
	text string
	ts   []token
	pos  int
}
type expr struct {
	op, name    string
	value       Value
	typ         Type
	left, right *expr
}
type node struct {
	text, name, format string
	cond               *expr
	yes, no            []node
}
type Program struct{ nodes []node }

func (p *Program) Render(values map[string]Value) string {
	var b strings.Builder
	render(&b, p.nodes, values)
	return b.String()
}
func render(b *strings.Builder, ns []node, v map[string]Value) {
	for _, n := range ns {
		if n.cond != nil {
			if truth(n.cond.eval(v)) {
				render(b, n.yes, v)
			} else {
				render(b, n.no, v)
			}
		} else if n.name != "" {
			b.WriteString(format(v[n.name], n.format))
		} else {
			b.WriteString(n.text)
		}
	}
}
func truth(v Value) bool {
	if !v.Defined {
		return false
	}
	switch v.Type {
	case Number, Seconds:
		return v.Number != 0
	case String:
		return v.String != ""
	case Bool:
		return v.Bool
	case Clock:
		return true
	}
	return false
}
func boolean(b bool) Value {
	return Value{Defined: true, Type: Bool, Bool: b}
}
func (e *expr) eval(v map[string]Value) Value {
	switch e.op {
	case "value":
		return e.value
	case "name":
		return v[e.name]
	case "not":
		return boolean(!truth(e.left.eval(v)))
	case "and":
		return boolean(truth(e.left.eval(v)) && truth(e.right.eval(v)))
	case "or":
		return boolean(truth(e.left.eval(v)) || truth(e.right.eval(v)))
	}
	return boolean(compare(e.left.eval(v), e.right.eval(v), e.op))
}
func compare(a, b Value, op string) bool {
	if !a.Defined || !b.Defined {
		return false
	}
	at, bt := a.Type, b.Type
	if at == Seconds {
		at = Number
	}
	if bt == Seconds {
		bt = Number
	}
	if at != bt {
		return op == "!="
	}
	eq := false
	switch at {
	case Number:
		switch op {
		case "<":
			return a.Number < b.Number
		case "<=":
			return a.Number <= b.Number
		case ">":
			return a.Number > b.Number
		case ">=":
			return a.Number >= b.Number
		}
		eq = a.Number == b.Number
	case String:
		eq = a.String == b.String
	case Bool:
		eq = a.Bool == b.Bool
	}
	if op == "==" {
		return eq
	}
	if op == "!=" {
		return !eq
	}
	return false
}
func format(v Value, f string) string {
	if !v.Defined {
		return ""
	}
	switch v.Type {
	case Number:
		n, _ := strconv.Atoi(f)
		return strconv.FormatFloat(math.Round(v.Number*math.Pow10(n))/math.Pow10(n), 'f', n, 64)
	case String:
		return v.String
	case Bool:
		return strconv.FormatBool(v.Bool)
	case Clock:
		if f == "hms" {
			return v.Time.In(time.Local).Format("15:04:05")
		}
		return v.Time.In(time.Local).Format("15:04")
	case Seconds:
		s := math.Floor(v.Number)
		if f == "s" {
			return strconv.FormatFloat(s, 'f', 0, 64)
		}
		if f == "ms" {
			return fmt.Sprintf("%.0f:%02.0f", math.Floor(s/60), math.Mod(s, 60))
		}
		return fmt.Sprintf("%.0f:%02.0f:%02.0f", math.Floor(s/3600), math.Mod(math.Floor(s/60), 60), math.Mod(s, 60))
	}
	return ""
}

type parser struct {
	source string
	defs   map[string]Definition
	parts  []part
	pi     int
	ts     []token
	ti     int
}

func (p *parser) fail(pos int, code string) *Error {
	prefix := p.source[:pos]
	line := strings.Count(prefix, "\n") + 1
	last := strings.LastIndex(prefix, "\n")
	col := utf8.RuneCountInString(prefix[last+1:]) + 1
	return &Error{line, col, i18n.New("template."+code, "line", strconv.Itoa(line), "column", strconv.Itoa(col))}
}

// Compile は保存時に字句・文法・型を検証する。渡された表は保持しない。
func Compile(source string, defs map[string]Definition) (*Program, error) {
	p := &parser{source: source, defs: defs}
	if e := p.lex(); e != nil {
		return nil, e
	}
	ns, e := p.sequence()
	if e != nil {
		return nil, e
	}
	if p.pi != len(p.parts) {
		return nil, p.fail(p.parts[p.pi].pos, "syntax")
	}
	return &Program{ns}, nil
}
func alpha(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_'
}
func digit(c byte) bool {
	return c >= '0' && c <= '9'
}
func (p *parser) lex() error {
	s := p.source
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			p.parts = append(p.parts, part{text: b.String()})
			b.Reset()
		}
	}
	for i := 0; i < len(s); {
		if i+1 < len(s) && (s[i:i+2] == "{{" || s[i:i+2] == "}}") {
			b.WriteByte(s[i])
			i += 2
			continue
		}
		if s[i] == '}' {
			return p.fail(i, "syntax")
		}
		if s[i] != '{' {
			b.WriteByte(s[i])
			i++
			continue
		}
		flush()
		start := i
		i++
		var ts []token
		for {
			if i >= len(s) {
				return p.fail(i, "syntax")
			}
			c := s[i]
			if c == ' ' || c == '\t' {
				i++
				continue
			}
			if c == '}' {
				i++
				break
			}
			a := i
			k := ""
			val := ""
			switch {
			case alpha(c):
				i++
				for i < len(s) && (alpha(s[i]) || digit(s[i]) || s[i] == '.') {
					i++
				}
				val = s[a:i]
				k = "name"
				for _, seg := range strings.Split(val, ".") {
					if seg == "" || !alpha(seg[0]) {
						return p.fail(a, "syntax")
					}
				}
				if strings.Contains(" if elif else end and or not true false ", " "+val+" ") {
					k = val
				}
			case digit(c) || c == '-':
				i++
				if c == '-' && (i >= len(s) || !digit(s[i])) {
					return p.fail(a, "syntax")
				}
				for i < len(s) && digit(s[i]) {
					i++
				}
				if i < len(s) && s[i] == '.' {
					i++
					j := i
					for i < len(s) && digit(s[i]) {
						i++
					}
					if i == j {
						return p.fail(a, "syntax")
					}
				}
				k = "number"
				val = s[a:i]
			case c == '"':
				i++
				var q strings.Builder
				closed := false
				for i < len(s) {
					c = s[i]
					if c == '"' {
						i++
						closed = true
						break
					}
					if c == '\n' || c == '\r' {
						return p.fail(i, "syntax")
					}
					if c == '\\' {
						i++
						if i >= len(s) || (s[i] != '\\' && s[i] != '"') {
							return p.fail(i, "syntax")
						}
						c = s[i]
					}
					q.WriteByte(c)
					i++
				}
				if !closed {
					return p.fail(a, "syntax")
				}
				k = "string"
				val = q.String()
			default:
				i++
				k = string(c)
				if i < len(s) && s[i] == '=' && strings.Contains("<>=!", k) {
					i++
					k += "="
				}
				if !strings.Contains(" < <= > >= == != ( ) : ", " "+k+" ") {
					return p.fail(a, "syntax")
				}
				val = k
			}
			if len(ts) > 0 && ts[len(ts)-1].pos+len(ts[len(ts)-1].text) == a && wordToken(k) && wordToken(ts[len(ts)-1].kind) {
				return p.fail(a, "syntax")
			}
			ts = append(ts, token{k, val, a})
		}
		if len(ts) == 0 {
			return p.fail(start, "syntax")
		}
		p.parts = append(p.parts, part{ts: ts, pos: start})
		// 制御タグだけの行を元の位置情報を保ったまま除く。
		k := ts[0].kind
		if k == "if" || k == "elif" || k == "else" || k == "end" {
			ls := strings.LastIndex(s[:start], "\n") + 1
			end := i
			for end < len(s) && (s[end] == ' ' || s[end] == '\t' || s[end] == '\r') {
				end++
			}
			if strings.Trim(s[ls:start], " \t") == "" && (end == len(s) || s[end] == '\n') {
				if len(p.parts) >= 2 {
					prev := &p.parts[len(p.parts)-2]
					if prev.ts == nil {
						prev.text = strings.TrimSuffix(prev.text, s[ls:start])
					}
				}
				i = end
				if i < len(s) {
					i++
				}
			}
		}
	}
	flush()
	return nil
}
func (p *parser) peek() string {
	if p.ti >= len(p.ts) {
		return ""
	}
	return p.ts[p.ti].kind
}
func (p *parser) pos() int {
	if p.ti < len(p.ts) {
		return p.ts[p.ti].pos
	}
	return p.ts[len(p.ts)-1].pos + len(p.ts[len(p.ts)-1].text)
}
func (p *parser) expression(level int) (*expr, error) {
	if level == 3 {
		return p.operand()
	}
	if level == 2 && p.peek() == "not" {
		p.ti++
		x, e := p.expression(2)
		return &expr{op: "not", left: x, typ: Bool}, e
	}
	x, e := p.expression(level + 1)
	if e != nil {
		return nil, e
	}
	for {
		op := p.peek()
		valid := level == 0 && op == "or" || level == 1 && op == "and" || level == 2 && strings.Contains(" < <= > >= == != ", " "+op+" ") && op != ""
		if !valid {
			break
		}
		pos := p.pos()
		p.ti++
		y, err := p.expression(level + 1)
		if err != nil {
			return nil, err
		}
		if level == 2 && (x.typ == Clock || y.typ == Clock) {
			return nil, p.fail(pos, "comparison")
		}
		x = &expr{op: op, left: x, right: y, typ: Bool}
		if level == 2 {
			break
		}
	}
	return x, nil
}
func (p *parser) operand() (*expr, error) {
	pos := p.pos()
	k := p.peek()
	if k == "" {
		return nil, p.fail(pos, "syntax")
	}
	t := p.ts[p.ti]
	p.ti++
	e := &expr{op: "value"}
	switch k {
	case "(":
		x, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, p.fail(p.pos(), "syntax")
		}
		p.ti++
		return x, nil
	case "name":
		d, ok := p.defs[t.text]
		if !ok {
			return nil, p.fail(pos, "unknown")
		}
		e.op = "name"
		e.name = t.text
		e.typ = d.Type
		return e, nil
	case "number":
		n, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, p.fail(pos, "syntax")
		}
		e.value = Value{Defined: true, Type: Number, Number: n}
	case "string":
		e.value = Value{Defined: true, Type: String, String: t.text}
	case "true", "false":
		e.value = boolean(k == "true")
	default:
		return nil, p.fail(pos, "syntax")
	}
	e.typ = e.value.Type
	return e, nil
}
func validFormat(t Type, f string) bool {
	switch t {
	case Number:
		return len(f) == 1 && f[0] >= '0' && f[0] <= '6'
	case Seconds:
		return f == "hms" || f == "ms" || f == "s"
	case Clock:
		return f == "hm" || f == "hms"
	case String, Bool:
		return f == ""
	}
	return false
}
func (p *parser) sequence() ([]node, error) {
	var ns []node
	for p.pi < len(p.parts) {
		a := p.parts[p.pi]
		if a.ts == nil {
			ns = append(ns, node{text: a.text})
			p.pi++
			continue
		}
		k := a.ts[0].kind
		if k == "elif" || k == "else" || k == "end" {
			break
		}
		p.ts = a.ts
		p.ti = 0
		if k == "if" {
			n, e := p.block()
			if e != nil {
				return nil, e
			}
			ns = append(ns, n)
			continue
		}
		if k != "name" {
			return nil, p.fail(a.pos, "syntax")
		}
		t := a.ts[0]
		d, ok := p.defs[t.text]
		if !ok {
			return nil, p.fail(t.pos, "unknown")
		}
		f := d.Format
		if f == "" {
			switch d.Type {
			case Number:
				f = "0"
			case Seconds:
				f = "hms"
			case Clock:
				f = "hm"
			}
		}
		if len(a.ts) > 1 {
			if len(a.ts) != 3 || a.ts[1].kind != ":" {
				return nil, p.fail(a.pos, "syntax")
			}
			if a.ts[2].kind != "name" && a.ts[2].kind != "number" {
				return nil, p.fail(a.ts[2].pos, "syntax")
			}
			// 形の名前は `:` のすぐ後ろ（9章「字句」。`{a: 0}` は誤り）
			if a.ts[2].pos != a.ts[1].pos+1 {
				return nil, p.fail(a.ts[2].pos, "syntax")
			}
			f = a.ts[2].text
			if f == "" {
				return nil, p.fail(a.ts[2].pos, "format")
			}
		}
		if !validFormat(d.Type, f) {
			return nil, p.fail(t.pos, "format")
		}
		ns = append(ns, node{name: t.text, format: f})
		p.pi++
	}
	return ns, nil
}
func (p *parser) block() (node, error) {
	p.ti = 1
	x, e := p.expression(0)
	if e != nil {
		return node{}, e
	}
	if p.ti != len(p.ts) {
		return node{}, p.fail(p.pos(), "syntax")
	}
	n := node{cond: x}
	p.pi++
	n.yes, e = p.sequence()
	if e != nil {
		return n, e
	}
	if p.pi == len(p.parts) {
		return n, p.fail(len(p.source), "syntax")
	}
	a := p.parts[p.pi]
	switch a.ts[0].kind {
	case "elif":
		p.ts = a.ts
		p.ti = 0
		child, err := p.block()
		n.no = []node{child}
		return n, err
	case "else":
		if len(a.ts) != 1 {
			return n, p.fail(a.pos, "syntax")
		}
		p.pi++
		n.no, e = p.sequence()
		if e != nil {
			return n, e
		}
		if p.pi == len(p.parts) {
			return n, p.fail(len(p.source), "syntax")
		}
		a = p.parts[p.pi]
	}
	if a.ts[0].kind != "end" || len(a.ts) != 1 {
		return n, p.fail(a.pos, "syntax")
	}
	p.pi++
	return n, nil
}

func wordToken(k string) bool {
	return k == "name" || k == "number" || strings.Contains(" if elif else end and or not true false ", " "+k+" ")
}

// References reports whether name occurs as an identifier in a template tag.
func References(text, name string) bool {
	p := &parser{source: text}
	err := p.lex()
	if err != nil {
		return false
	}
	for _, p := range p.parts {
		for _, t := range p.ts {
			if t.kind == "name" && t.text == name {
				return true
			}
		}
	}
	return false
}

// RenderHTML escapes substitutions while preserving literal template text.
func (p *Program) RenderHTML(values map[string]Value) string {
	var b strings.Builder
	var walk func([]node)
	walk = func(ns []node) {
		for _, n := range ns {
			if n.cond != nil {
				if truth(n.cond.eval(values)) {
					walk(n.yes)
				} else {
					walk(n.no)
				}
			} else if n.name != "" {
				v := values[n.name]
				if v.Type == Number && (math.IsNaN(v.Number) || math.IsInf(v.Number, 0)) {
					v.Defined = false
				}
				b.WriteString(html.EscapeString(strings.ReplaceAll(format(v, n.format), "\x00", "")))
			} else {
				b.WriteString(n.text)
			}
		}
	}
	walk(p.nodes)
	return b.String()
}

// TagToken returns the first lexical token of a template tag.
func TagToken(s string) (kind, name string) {
	p := parser{source: s}
	_ = p.lex()
	if len(p.parts) > 0 && len(p.parts[0].ts) > 0 {
		t := p.parts[0].ts[0]
		return t.kind, t.text
	}
	return "", ""
}
