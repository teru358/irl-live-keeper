package sources

import (
	_ "embed"
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
)

// HTMLRules is the shared, versioned HTML policy served to browser clients.
//
//go:embed html_rules.json
var HTMLRules []byte

type htmlPolicy struct {
	TemplateAttributes, TemplateAttributePatterns                                          []string
	Elements, GlobalAttributes, AttributeNamePatterns, VoidElements, AttributeFreeElements []string
	ElementAttributes                                                                      map[string][]string
	Asset                                                                                  struct {
		Prefix, SegmentPattern, RenderedQueryPattern string
		MaxSegments, MaxPathLength                   int
		Extensions                                   []string
	}
	CSS struct {
		RejectFunctions, StripVendorPrefixes, RejectAtRules []string
		FragmentPattern                                     string
	}
	Nesting struct {
		Parents, Children                                                                 map[string][]string
		RestrictedTextParents, PForbiddenDescendants, LiAncestorBoundary, HeadingElements []string
		MaxDepth                                                                          int
	}
}

var htmlRules = func() htmlPolicy {
	var p htmlPolicy
	if e := json.Unmarshal(HTMLRules, &p); e != nil {
		panic(e)
	}
	return p
}()

func has(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
func pattern(xs []string, s string) bool {
	for _, x := range xs {
		if regexp.MustCompile(x).MatchString(s) {
			return true
		}
	}
	return false
}
func htmlError(s string, pos int, code string) *template.Error {
	if pos > len(s) {
		pos = len(s)
	}
	pre := s[:pos]
	line := strings.Count(pre, "\n") + 1
	column := utf8.RuneCountInString(pre[strings.LastIndex(pre, "\n")+1:]) + 1
	return &template.Error{Line: line, Column: column, Message: i18n.New("template."+code, "line", strconv.Itoa(line), "column", strconv.Itoa(column))}
}
func space(c byte) bool    { return strings.ContainsRune(" \t\r\n\f", rune(c)) }
func letter(c byte) bool   { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func ident(c byte) bool    { return letter(c) || c >= '0' && c <= '9' || c == '_' || c == '-' }
func attrChar(c byte) bool { return ident(c) || c == '.' || c == ':' }

// CheckHTMLURL validates a decoded URL without resolving or normalizing it.
func CheckHTMLURL(value, mode string, css bool) bool {
	low := strings.ToLower(value)
	if strings.HasPrefix(low, "data:image/") || css && strings.HasPrefix(low, "data:font/") {
		return strings.Contains(value, ",")
	}
	if css && regexp.MustCompile(htmlRules.CSS.FragmentPattern).MatchString(value) {
		return true
	}
	a := htmlRules.Asset
	if !strings.HasPrefix(value, a.Prefix) {
		return false
	}
	path := value
	if i := strings.IndexByte(path, '?'); i >= 0 {
		if mode != "rendered" || !regexp.MustCompile(a.RenderedQueryPattern).MatchString(path[i+1:]) {
			return false
		}
		path = path[:i]
	}
	if len(path) > a.MaxPathLength || strings.Contains(path, "..") || strings.Contains(path, "//") || strings.ContainsAny(path, "?#%") {
		return false
	}
	rel := strings.TrimPrefix(path, a.Prefix)
	parts := strings.Split(rel, "/")
	if len(parts) > a.MaxSegments || CheckPath(rel) != "" {
		return false
	}
	for _, p := range parts {
		if !regexp.MustCompile(a.SegmentPattern).MatchString(p) {
			return false
		}
	}
	i := strings.LastIndexByte(rel, '.')
	return i >= 0 && has(a.Extensions, strings.ToLower(rel[i+1:]))
}

type htmlIssue struct {
	pos  int
	code string
}

func cssCheck(s, mode string, refs map[string]bool) *htmlIssue {
	fail := func(p int, c string) *htmlIssue { return &htmlIssue{p, c} }
	for i := 0; i < len(s); {
		a := i
		if s[i] == '\\' {
			return fail(i, "css_escape")
		}
		if strings.HasPrefix(s[i:], "/*") {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return fail(i, "css_syntax")
			}
			end += i + 4
			if j := strings.IndexByte(s[i:end], '\\'); j >= 0 {
				return fail(i+j, "css_escape")
			}
			i = end
			continue
		}
		if s[i] == '"' || s[i] == '\'' {
			q := s[i]
			i++
			for i < len(s) && s[i] != q {
				if s[i] == '\\' {
					return fail(i, "css_escape")
				}
				if strings.ContainsRune("\n\r\f", rune(s[i])) {
					return fail(a, "css_syntax")
				}
				i++
			}
			if i == len(s) {
				return fail(a, "css_syntax")
			}
			i++
			continue
		}
		at := s[i] == '@'
		if at {
			i++
		}
		start := i
		for i < len(s) && ident(s[i]) {
			i++
		}
		if start == i {
			if i < len(s) && s[i] == '\\' {
				return fail(i, "css_escape")
			}
			i++
			continue
		}
		name := strings.ToLower(s[start:i])
		if at && has(htmlRules.CSS.RejectAtRules, name) {
			return fail(a, "css_import")
		}
		if i >= len(s) || s[i] != '(' {
			continue
		}
		base := name
		for _, p := range htmlRules.CSS.StripVendorPrefixes {
			base = strings.TrimPrefix(base, p)
		}
		if has(htmlRules.CSS.RejectFunctions, base) {
			return fail(start, "css_function")
		}
		if name != "url" {
			continue
		}
		i++
		for i < len(s) && space(s[i]) {
			i++
		}
		vpos := i
		var value string
		if i < len(s) && (s[i] == '"' || s[i] == '\'') {
			q := s[i]
			i++
			vstart := i
			for i < len(s) && s[i] != q {
				if s[i] == '\\' {
					return fail(i, "css_escape")
				}
				if strings.ContainsRune("\n\r\f", rune(s[i])) {
					return fail(vpos, "css_syntax")
				}
				i++
			}
			if i == len(s) {
				return fail(vpos, "css_syntax")
			}
			value = s[vstart:i]
			i++
			for i < len(s) {
				if space(s[i]) {
					i++
					continue
				}
				if strings.HasPrefix(s[i:], "/*") {
					end := strings.Index(s[i+2:], "*/")
					if end < 0 {
						return fail(i, "css_syntax")
					}
					if j := strings.IndexByte(s[i:i+end+4], '\\'); j >= 0 {
						return fail(i+j, "css_escape")
					}
					i += end + 4
					continue
				}
				break
			}
			if i == len(s) || s[i] != ')' {
				return fail(vpos, "css_syntax")
			}
		} else {
			st := i
			for i < len(s) && s[i] != ')' {
				if s[i] == '\\' {
					return fail(i, "css_escape")
				}
				i++
			}
			if i == len(s) {
				return fail(a, "css_syntax")
			}
			raw := s[st:i]
			value = strings.Trim(raw, " \t\r\n\f")
			vpos = st + len(raw) - len(strings.TrimLeft(raw, " \t\r\n\f"))
		}
		if !CheckHTMLURL(value, mode, true) {
			return fail(vpos, "html_url")
		}
		if strings.HasPrefix(value, htmlRules.Asset.Prefix) && refs != nil {
			refs[value] = true
		}
		i++
	}
	return nil
}

// CheckHTMLCSS checks decoded CSS. Positions are byte offsets into this input.
func CheckHTMLCSS(s, mode string) error {
	if e := cssCheck(s, mode, nil); e != nil {
		return htmlError(s, e.pos, e.code)
	}
	return nil
}
func decodeHTML(s string, base int) (string, []int, *htmlIssue) {
	var b strings.Builder
	var positions []int
	for i := 0; i < len(s); {
		a := i
		value := s[i : i+1]
		i++
		if s[a] == '&' {
			end := strings.IndexByte(s[a:], ';')
			if end < 0 {
				return "", nil, &htmlIssue{base + a, "html_lex"}
			}
			entity := s[a : a+end+1]
			value = ""
			named := map[string]string{"&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": "\"", "&apos;": "'", "&nbsp;": "\u00a0"}
			if v, ok := named[entity]; ok {
				value = v
			} else if strings.HasPrefix(entity, "&#") {
				digits := entity[2 : len(entity)-1]
				radix := 10
				if strings.HasPrefix(digits, "x") || strings.HasPrefix(digits, "X") {
					radix = 16
					digits = digits[1:]
				}
				n, e := strconv.ParseUint(digits, radix, 32)
				if e != nil || n == 0 || n > 0x10ffff || n >= 0xd800 && n <= 0xdfff || n >= 0x80 && n <= 0x9f {
					return "", nil, &htmlIssue{base + a, "html_lex"}
				}
				value = string(rune(n))
			} else {
				return "", nil, &htmlIssue{base + a, "html_lex"}
			}
			i = a + end + 1
		}
		b.WriteString(value)
		for range []byte(value) {
			positions = append(positions, base+a)
		}
	}
	positions = append(positions, base+len(s))
	return b.String(), positions, nil
}

type htmlOpen struct {
	name string
	pos  int
}
type htmlScanner struct {
	defs     map[string]template.Definition
	branches []htmlBranch
	literal  [][2]int
	s, mode  string
	stack    []htmlOpen
	refs     map[string]bool
}

func (p *htmlScanner) fail(pos int, code string) error { return htmlError(p.s, pos, code) }
func (p *htmlScanner) css(s string, base int, decode bool) error {
	positions := make([]int, len(s)+1)
	for i := range positions {
		positions[i] = base + i
	}
	if decode {
		var e *htmlIssue
		s, positions, e = decodeHTML(s, base)
		if e != nil {
			return p.fail(e.pos, e.code)
		}
	}
	if e := cssCheck(s, p.mode, p.refs); e != nil {
		return p.fail(positions[e.pos], e.code)
	}
	return nil
}
func (p *htmlScanner) run() error {
	s := p.s
	for i := 0; i < len(s); {
		if s[i] == 0 {
			return p.fail(i, "html_nul")
		}
		if strings.HasPrefix(s[i:], "<!--") {
			a := i
			end := strings.Index(s[i+4:], "-->")
			if end < 0 {
				if j := strings.IndexByte(s[i:], 0); j >= 0 {
					return p.fail(i+j, "html_nul")
				}
				return p.fail(a, "html_comment")
			}
			end += i + 4
			if j := strings.Index(s[i+4:end], "--!>"); j >= 0 {
				return p.fail(i+4+j, "html_comment")
			}
			if end == i+3 {
				return p.fail(a, "html_comment")
			}
			if j := strings.IndexByte(s[i:end], 0); j >= 0 {
				return p.fail(i+j, "html_nul")
			}
			p.literal = append(p.literal, [2]int{i, end + 3})
			i = end + 3
			continue
		}
		if p.mode == "save" && (s[i] == '{' || s[i] == '}') {
			end, e := p.tag(i, "")
			if e != nil {
				return e
			}
			i = end
			continue
		}
		if s[i] != '<' {
			if len(p.stack) > 0 && has(htmlRules.Nesting.RestrictedTextParents, p.stack[len(p.stack)-1].name) && !space(s[i]) {
				if s[i] == '&' {
					end := strings.IndexByte(s[i:], ';')
					if end >= 0 {
						v := html.UnescapeString(s[i : i+end+1])
						if strings.HasPrefix(s[i:], "&#") && len(v) == 1 && space(v[0]) {
							i += end + 1
							continue
						}
					}
				}
				return p.fail(i, "html_nesting")
			}
			if s[i] == '&' {
				if regexp.MustCompile("^&#(0+|[xX]0+);").MatchString(s[i:]) {
					return p.fail(i, "html_nul")
				}
			}
			i++
			continue
		}
		a := i
		i++
		closing := false
		if i < len(s) && s[i] == '/' {
			closing = true
			i++
		}
		if i >= len(s) || !letter(s[i]) {
			if i < len(s) && s[i] == '{' {
				return p.fail(i, "html_template_position")
			}
			return p.fail(a, "html_lex")
		}
		st := i
		for i < len(s) && (letter(s[i]) || s[i] >= '0' && s[i] <= '9' || s[i] == '-') {
			i++
		}
		if i < len(s) && s[i] == '{' {
			return p.fail(i, "html_template_position")
		}
		name := strings.ToLower(s[st:i])
		if !has(htmlRules.Elements, name) {
			return p.fail(a, "html_element")
		}
		attrs := map[string]bool{}
		self := false
		for {
			before := i
			for i < len(s) && space(s[i]) {
				i++
			}
			if i == len(s) {
				return p.fail(a, "html_lex")
			}
			if s[i] == '>' {
				i++
				break
			}
			if s[i] == '/' && i+1 < len(s) && s[i+1] == '>' && !closing {
				self = true
				i += 2
				break
			}
			if s[i] == '{' {
				return p.fail(i, "html_template_position")
			}
			if closing || before == i || !attrChar(s[i]) {
				return p.fail(i, "html_lex")
			}
			ap := i
			for i < len(s) && attrChar(s[i]) {
				i++
			}
			if i < len(s) && s[i] == '{' {
				return p.fail(i, "html_template_position")
			}
			attr := strings.ToLower(s[ap:i])
			if has(htmlRules.AttributeFreeElements, name) || !has(htmlRules.GlobalAttributes, attr) && !has(htmlRules.ElementAttributes[name], attr) && !pattern(htmlRules.AttributeNamePatterns, attr) {
				return p.fail(ap, "html_attribute")
			}
			if attrs[attr] {
				return p.fail(ap, "html_duplicate")
			}
			attrs[attr] = true
			for i < len(s) && space(s[i]) {
				i++
			}
			if i == len(s) || s[i] != '=' {
				return p.fail(i, "html_quote")
			}
			i++
			for i < len(s) && space(s[i]) {
				i++
			}
			if i < len(s) && s[i] == '{' {
				return p.fail(i, "html_template_position")
			}
			if i == len(s) || s[i] != '"' {
				return p.fail(i, "html_quote")
			}
			i++
			vp := i
			attrBranch := len(p.branches)
			for i < len(s) && s[i] != '"' {
				if p.mode == "save" && (s[i] == '{' || s[i] == '}') {
					if attr == "style" {
						i = templateEnd(s, i)
						continue
					}
					kind := templateKind(s[i:templateEnd(s, i)])
					if (kind == "end" || kind == "else" || kind == "elif") && len(p.branches) == attrBranch && attrBranch > 0 {
						return p.fail(p.branches[attrBranch-1].pos, "html_branch")
					}
					end, e := p.tag(i, attr)
					if e != nil {
						return e
					}
					i = end
					continue
				}
				if s[i] == 0 {
					return p.fail(i, "html_nul")
				}
				i++
			}
			if i == len(s) {
				return p.fail(a, "html_lex")
			}
			if len(p.branches) > attrBranch {
				return p.fail(p.branches[attrBranch].pos, "html_branch")
			}
			value := s[vp:i]
			if j := strings.IndexByte(value, 0); j >= 0 {
				return p.fail(vp+j, "html_nul")
			}
			if attr != "src" && attr != "style" {
				if loc := regexp.MustCompile("&#(0+|[xX]0+);").FindStringIndex(value); loc != nil {
					return p.fail(vp+loc[0], "html_nul")
				}
			}
			i++
			if attr == "src" {
				v, positions, e := decodeHTML(value, vp)
				if e != nil {
					return p.fail(e.pos, e.code)
				}
				if !CheckHTMLURL(v, p.mode, false) {
					return p.fail(positions[0], "html_url")
				}
				if strings.HasPrefix(v, htmlRules.Asset.Prefix) {
					p.refs[v] = true
				}
			} else if attr == "style" {
				if p.mode == "save" {
					if err := p.styleCSS(value, vp); err != nil {
						return err
					}
				} else if err := p.css(value, vp, true); err != nil {
					return err
				}
			}
		}
		if closing {
			if len(p.branches) > 0 && len(p.stack) <= p.branches[len(p.branches)-1].depth {
				return p.fail(a, "html_branch")
			}
			if len(p.stack) == 0 || p.stack[len(p.stack)-1].name != name || has(htmlRules.VoidElements, name) {
				return p.fail(a, "html_nesting")
			}
			p.stack = p.stack[:len(p.stack)-1]
			continue
		}
		parent := ""
		if len(p.stack) > 0 {
			parent = p.stack[len(p.stack)-1].name
		}
		n := htmlRules.Nesting
		if len(p.stack) >= n.MaxDepth || self && !has(htmlRules.VoidElements, name) {
			return p.fail(a, "html_nesting")
		}
		if xs := n.Parents[name]; len(xs) > 0 && !has(xs, parent) {
			return p.fail(a, "html_nesting")
		}
		if xs := n.Children[parent]; len(xs) > 0 && !has(xs, name) {
			return p.fail(a, "html_nesting")
		}
		for j := len(p.stack) - 1; j >= 0; j-- {
			ancestor := p.stack[j].name
			if ancestor == "p" && has(n.PForbiddenDescendants, name) || has(n.HeadingElements, ancestor) && has(n.HeadingElements, name) {
				return p.fail(a, "html_nesting")
			}
			if name == "li" {
				if has(n.LiAncestorBoundary, ancestor) {
					break
				}
				if ancestor == "li" {
					return p.fail(a, "html_nesting")
				}
			}
		}
		if name == "style" {
			re := regexp.MustCompile("(?i)</style[\t\n\r\f />]")
			loc := re.FindStringIndex(s[i:])
			if loc == nil {
				if j := strings.IndexByte(s[i:], 0); j >= 0 {
					return p.fail(i+j, "html_nul")
				}
				return p.fail(a, "html_style_unclosed")
			}
			end := i + loc[0]
			if j := strings.IndexByte(s[i:end], 0); j >= 0 {
				return p.fail(i+j, "html_nul")
			}
			p.literal = append(p.literal, [2]int{i, end})
			if e := p.css(s[i:end], i, false); e != nil {
				return e
			}
			p.stack = append(p.stack, htmlOpen{name, a})
			i = end
			continue
		}
		if !has(htmlRules.VoidElements, name) {
			p.stack = append(p.stack, htmlOpen{name, a})
		}
	}
	if len(p.stack) > 0 {
		return p.fail(p.stack[len(p.stack)-1].pos, "html_nesting")
	}
	return nil
}

// ValidateHTML checks HTML, CSS and URL syntax. CompileHTML adds template
// grammar and type validation for saved bodies.
func ValidateHTML(s, mode string) error {
	p := htmlScanner{s: s, mode: mode, refs: map[string]bool{}}
	return p.run()
}
