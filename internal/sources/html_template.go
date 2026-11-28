package sources

import (
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"sort"
	"strings"
)

type htmlBranch struct{ depth, pos int }

func templateEnd(s string, i int) int {
	if i+1 < len(s) && (s[i:i+2] == "{{" || s[i:i+2] == "}}") {
		return i + 2
	}
	quoted := false
	for j := i + 1; j < len(s); j++ {
		if quoted && s[j] == '\\' {
			j++
			continue
		}
		if s[j] == '"' {
			quoted = !quoted
		}
		if !quoted && s[j] == '}' {
			return j + 1
		}
	}
	return len(s)
}
func templateKind(s string) string {
	k, _ := template.TagToken(s)
	return k
}

// hasTemplateEnd recognizes closing tags with the template lexer, including
// whitespace around the keyword.
func hasTemplateEnd(s string) bool {
	for i := 0; i < len(s); {
		if s[i] != '{' {
			i++
			continue
		}
		end := templateEnd(s, i)
		if templateKind(s[i:end]) == "end" {
			return true
		}
		i = end
	}
	return false
}
func escapeBraces(s string) string {
	s = strings.ReplaceAll(s, "{", "{{")
	return strings.ReplaceAll(s, "}", "}}")
}

// styleValueContext marks a template tag inside a style attribute value. It is
// not an attribute name, so attribute rules do not apply, and it is not body
// text, so the restricted-text-parent rule (table/tr direct text) does not apply.
const styleValueContext = "\x00style-value"

func (p *htmlScanner) tag(i int, attr string) (int, error) {
	end := templateEnd(p.s, i)
	text := p.s[i:end]
	if j := strings.IndexByte(text, 0); j >= 0 {
		return end, p.fail(i+j, "html_nul")
	}
	kind := templateKind(text)
	if attr != "" && attr != "style" && attr != styleValueContext && !has(htmlRules.TemplateAttributes, attr) && !pattern(htmlRules.TemplateAttributePatterns, attr) {
		return end, p.fail(i, "html_template_position")
	}
	if attr == "style" {
		return end, p.fail(i, "html_template_position")
	}
	control := kind == "if" || kind == "elif" || kind == "else" || kind == "end"
	if attr == "" && !control && len(p.stack) > 0 && has(htmlRules.Nesting.RestrictedTextParents, p.stack[len(p.stack)-1].name) {
		return end, p.fail(i, "html_nesting")
	}
	snippet := text
	if kind == "if" {
		snippet += "{end}"
	}
	if kind == "elif" {
		snippet = strings.Replace(text, "elif", "if  ", 1) + "{end}"
	}
	if text != "{{" && text != "}}" && (kind != "else" && kind != "end" || len(p.branches) == 0) {
		if _, err := template.Compile(snippet, p.defs); err != nil {
			if te, ok := err.(*template.Error); ok {
				line, col := 1, 1
				offset := len(text)
				for j, r := range text {
					if line == te.Line && col == te.Column {
						offset = j
						break
					}
					if r == '\n' {
						line++
						col = 1
					} else {
						col++
					}
				}
				return end, p.fail(i+offset, strings.TrimPrefix(te.Message.Code, "template."))
			}
			return end, err
		}
	}
	if kind == "if" {
		p.branches = append(p.branches, htmlBranch{len(p.stack), i})
	} else if kind == "elif" || kind == "else" || kind == "end" {
		if len(p.branches) > 0 {
			b := p.branches[len(p.branches)-1]
			if b.depth != len(p.stack) {
				return end, p.fail(i, "html_branch")
			}
			if kind == "end" {
				p.branches = p.branches[:len(p.branches)-1]
			}
		}
	} else if attr == "" && len(p.stack) > 0 && has(htmlRules.Nesting.RestrictedTextParents, p.stack[len(p.stack)-1].name) {
		return end, p.fail(i, "html_nesting")
	}
	return end, nil
}
func (p *htmlScanner) styleValue(s string, base int) (string, error) {
	raw := s
	// Decode literal CSS only; template syntax keeps its original spelling.
	var decodedText strings.Builder
	var positions []int
	for i := 0; i < len(s); {
		end := i
		if s[i] == '{' || s[i] == '}' {
			end = templateEnd(s, i)
			decodedText.WriteString(s[i:end])
			for j := i; j < end; j++ {
				positions = append(positions, base+j)
			}
		} else {
			for end < len(s) && s[end] != '{' && s[end] != '}' {
				end++
			}
			value, offsets, err := decodeHTML(s[i:end], base+i)
			if err != nil {
				return "", p.fail(err.pos, err.code)
			}
			decodedText.WriteString(value)
			positions = append(positions, offsets[:len(offsets)-1]...)
		}
		i = end
	}
	positions = append(positions, base+len(s))
	decoded := decodedText.String()

	s = decoded
	var b strings.Builder
	quote := byte(0)
	comment, url := false, false
	groupEnd := 0
	for i := 0; i < len(s); {
		if (s[i] != '{' && s[i] != '}') || raw[positions[i]-base] == '&' {
			c := s[i]
			if comment {
				if strings.HasPrefix(s[i:], "*/") {
					comment = false
					b.WriteString("*/")
					i += 2
					continue
				}
			} else if quote != 0 {
				if c == quote {
					quote = 0
				}
			} else if strings.HasPrefix(s[i:], "/*") {
				comment = true
				b.WriteString("/*")
				i += 2
				continue
			} else if c == '"' || c == '\'' {
				quote = c
			} else if i+4 <= len(s) && strings.EqualFold(s[i:i+4], "url(") {
				url = true
			} else if c == ')' {
				url = false
			}
			b.WriteByte(c)
			i++
			continue
		}
		a := i
		end := templateEnd(s, i)
		tag := s[i:end]
		bad := quote != 0 || comment || url || tag == "{{" || tag == "}}" || i > 0 && (ident(s[i-1]) || s[i-1] == '@' || s[i-1] == '#')
		if end > groupEnd {
			groupEnd = end
			for groupEnd < len(s) && s[groupEnd] == '{' {
				groupEnd = templateEnd(s, groupEnd)
			}
		}
		next := groupEnd
		if i > 0 && next < len(s) && (s[i-1] == '/' && s[next] == '*' || s[i-1] == '*' && s[next] == '/') {
			bad = true
		}
		j := end
		for j < len(s) && letter(s[j]) {
			j++
		}
		if j == end && j < len(s) && s[j] == '%' {
			j++
		}
		if j < len(s) && (ident(s[j]) || s[j] == '(') {
			bad = true
		}
		if bad {
			return "", p.fail(positions[a], "html_template_position")
		}
		kind, name := template.TagToken(tag)
		d, ok := p.defs[name]
		if kind != "name" || !ok || d.Type != template.Number {
			return "", p.fail(positions[a], "html_style_type")
		}
		b.WriteString(strings.Repeat(" ", end-i))
		i = end
	}
	return b.String(), nil
}

// HTMLProgram retains validated template syntax and literal HTML.
type HTMLProgram struct {
	p            *template.Program
	refs         []string
	styleLiteral bool
}

func CompileHTML(s string, defs map[string]template.Definition) (*HTMLProgram, error) {
	p := htmlScanner{s: s, mode: "save", defs: defs, refs: map[string]bool{}}
	if e := p.run(); e != nil {
		return nil, e
	}
	// Literal CSS and comments are opaque to the template parser.
	var b strings.Builder
	cursor := 0
	for _, r := range p.literal {
		b.WriteString(s[cursor:r[0]])
		b.WriteString(escapeBraces(s[r[0]:r[1]]))
		cursor = r[1]
	}
	b.WriteString(s[cursor:])
	masked := []byte(s)
	for _, r := range p.literal {
		for i := r[0]; i < r[1]; i++ {
			if masked[i] == '{' || masked[i] == '}' {
				masked[i] = ' '
			}
		}
	}
	if _, e := template.Compile(string(masked), defs); e != nil {
		return nil, e
	}
	program, e := template.Compile(b.String(), defs)
	if e != nil {
		return nil, e
	}
	refs := make([]string, 0, len(p.refs))
	for r := range p.refs {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	literal := false
	for _, r := range p.literal {
		text := s[r[0]:r[1]]
		if strings.HasPrefix(text, "<!--") {
			continue
		}
		for name := range defs {
			if strings.Contains(text, "{"+name+"}") || strings.Contains(text, "{"+name+":") {
				literal = true
			}
		}
	}
	return &HTMLProgram{program, refs, literal}, nil
}
func (p *HTMLProgram) Render(v map[string]template.Value) string { return p.p.RenderHTML(v) }

// AssetReferences returns saved asset paths from every branch.
func (p *HTMLProgram) AssetReferences() []string { return append([]string(nil), p.refs...) }

// Warnings reports literal template-like CSS and missing assets. A nil asset
// set means that no index is available, so existence is not checked.
func (p *HTMLProgram) Warnings(available map[string]bool) []string {
	result := []string{}
	if p.styleLiteral {
		result = append(result, "template.style_literal")
	}
	if available != nil {
		for _, r := range p.refs {
			if !available[r] {
				result = append(result, "template.asset_missing")
				break
			}
		}
	}
	return result
}

// styleCSS checks both undefined and numeric substitutions, retaining an offset
// map through substitution removal and subsequent character-reference decoding.
func (p *htmlScanner) styleCSS(raw string, base int) error {
	if _, err := p.styleValue(raw, base); err != nil {
		return err
	}
	for _, replacement := range []string{"", "0"} {
		var b strings.Builder
		var original []int
		for i := 0; i < len(raw); {
			if raw[i] == '{' {
				end := templateEnd(raw, i)
				b.WriteString(replacement)
				for range []byte(replacement) {
					original = append(original, base+i)
				}
				i = end
				continue
			}
			b.WriteByte(raw[i])
			original = append(original, base+i)
			i++
		}
		original = append(original, base+len(raw))
		decoded, positions, e := decodeHTML(b.String(), 0)
		if e != nil {
			return p.fail(original[e.pos], e.code)
		}
		if e := cssCheck(decoded, p.mode, p.refs); e != nil {
			return p.fail(original[positions[e.pos]], e.code)
		}
	}
	return nil
}
