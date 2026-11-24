// Package settings は、設定の表記を保ちながら検証済みの変更を作る。
package settings

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teru358/irl-live-keeper/internal/config"
	"gopkg.in/yaml.v3"
)

// Change はドット区切りのキーと [番号] で指定する変更。Value は YAML に符号化できる値。
// 例: admin.port、cameras[0].name。配列全体の置換もできる。
type Change struct {
	Path  string
	Value any
}

var errInvalid = errors.New("設定の変更を検証できません")
var pathPart = regexp.MustCompile(`^([a-z_][a-z_0-9]*)(?:\[([0-9]+)\])?$`)

type step struct {
	key   string
	index int
}

func parsePath(p string) ([]step, error) {
	var out []step
	for _, part := range strings.Split(p, ".") {
		m := pathPart.FindStringSubmatch(part)
		if m == nil {
			return nil, errInvalid
		}
		out = append(out, step{key: m[1], index: -1})
		if m[2] != "" {
			i, e := strconv.Atoi(m[2])
			if e != nil {
				return nil, errInvalid
			}
			out = append(out, step{index: i})
		}
	}
	return out, nil
}
func child(n *yaml.Node, s step, create bool) (*yaml.Node, error) {
	if s.index >= 0 {
		if n.Kind != yaml.SequenceNode || s.index >= len(n.Content) {
			return nil, errInvalid
		}
		return n.Content[s.index], nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, errInvalid
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == s.key {
			return n.Content[i+1], nil
		}
	}
	if !create {
		return nil, errInvalid
	}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s.key}, v)
	return v, nil
}
func encode(n *yaml.Node) ([]byte, error) {
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if e.Encode(n) != nil {
		return nil, errInvalid
	}
	return b.Bytes(), nil
}
func offset(src string, line, col int) int {
	p := 0
	for i := 1; i < line; i++ {
		j := strings.IndexByte(src[p:], '\n')
		if j < 0 {
			return len(src)
		}
		p += j + 1
	}
	for range col - 1 {
		if p >= len(src) {
			break
		}
		_, size := utf8.DecodeRuneInString(src[p:])
		p += size
	}
	return p
}

// Rewrite は元のバイト列から変更後の内容を作る。エラーに設定値を含めない。
func Rewrite(src []byte, changes []Change) ([]byte, error) {
	return rewrite(src, changes, config.Parse)
}
func rewrite(src []byte, changes []Change, parse func([]byte) (*config.Config, error)) ([]byte, error) {
	if _, e := config.Parse(src); e != nil {
		return nil, errInvalid
	}
	var expected yaml.Node
	if yaml.Unmarshal(src, &expected) != nil {
		return nil, errInvalid
	}
	for _, c := range changes {
		p, e := parsePath(c.Path)
		if e != nil {
			return nil, e
		}
		n := expected.Content[0]
		for _, s := range p {
			n, e = child(n, s, true)
			if e != nil {
				return nil, e
			}
		}
		var v yaml.Node
		if v.Encode(c.Value) != nil {
			return nil, errInvalid
		}
		normalizeStrings(&v)
		omitEmptyFields(&v)
		v.Anchor = n.Anchor
		*n = v
	}
	canonical, e := encode(&expected)
	if e != nil {
		return nil, e
	}
	want, e := config.Parse(canonical)
	if e != nil {
		return nil, errInvalid
	}
	out := string(src)
	for _, c := range changes {
		var doc yaml.Node
		if yaml.Unmarshal([]byte(out), &doc) != nil {
			return nil, errInvalid
		}
		root := doc.Content[0]
		p, _ := parsePath(c.Path)
		n := root
		simple := true
		var target *yaml.Node
		for _, s := range p {
			if !ordinary(n) {
				simple = false
			}
			next, er := child(n, s, false)
			if er != nil {
				simple = false
				break
			}
			n = next
			target = n
		}
		var v yaml.Node
		if v.Encode(c.Value) != nil {
			return nil, errInvalid
		}
		normalizeStrings(&v)
		omitEmptyFields(&v)
		if simple && target != nil {
			if changed, ok := patch(out, target, &v); ok {
				out = changed
				continue
			}
		}
		// 通常の対応づけにない項目は、既存の末尾へ追加する。
		parent := root
		found := true
		for _, s := range p[:len(p)-1] {
			var er error
			parent, er = child(parent, s, false)
			if er != nil {
				found = false
				break
			}
		}
		leaf := p[len(p)-1]
		if found && parent != root && leaf.index < 0 && parent.Kind == yaml.MappingNode && plainTree(parent) {
			if _, er := child(parent, leaf, false); er != nil {
				pair := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: leaf.key}, &v}}
				b, er := encode(pair)
				if er != nil {
					return nil, er
				}
				indent := 2
				if len(parent.Content) > 0 {
					indent = parent.Content[0].Column - 1
				}
				if parent == root {
					indent = 0
				}
				lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
				for i := range lines {
					lines[i] = strings.Repeat(" ", indent) + lines[i]
				}
				pos := offset(out, lastLine(parent)+1, 1)
				addition := strings.Join(lines, "\n") + "\n"
				if pos > 0 && out[pos-1] != '\n' {
					addition = "\n" + addition
				}
				out = out[:pos] + addition + out[pos:]
				continue
			}
		}
		// 特殊な表記や追加は、その最上位区分だけを書き直す。
		section, e := child(root, p[0], true)
		if e != nil {
			return nil, e
		}
		n = section
		for _, s := range p[1:] {
			n, e = child(n, s, true)
			if e != nil {
				return nil, e
			}
		}
		v.Anchor = n.Anchor
		*n = v
		idx := 0
		for root.Content[idx].Value != p[0].key {
			idx += 2
		}
		pair := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: root.Content[idx : idx+2]}
		b, e := encode(pair)
		if e != nil {
			return nil, e
		}
		start, end := len(out), len(out)
		if root.Content[idx].Line > 0 {
			start = offset(out, root.Content[idx].Line, 1)
			if idx+2 < len(root.Content) && root.Content[idx+2].Line > 0 {
				end = offset(out, root.Content[idx+2].Line, 1)
				// 次の区分の直前の空行とコメントはそのまま残す。
				for end > start {
					prev := strings.LastIndex(out[:end-1], "\n") + 1
					line := strings.TrimSpace(out[prev:end])
					if line != "" && !strings.HasPrefix(line, "#") {
						break
					}
					end = prev
				}
			}
		}
		if start == len(out) && start > 0 && !strings.HasSuffix(out, "\n") {
			out += "\n"
			start++
			end++
		}
		out = out[:start] + string(b) + out[end:]
	}
	got, e := parse([]byte(out))
	if e != nil || !reflect.DeepEqual(want, got) {
		return nil, errInvalid
	}
	return []byte(out), nil
}
func ordinary(n *yaml.Node) bool {
	return n.Anchor == "" && n.Kind != yaml.AliasNode && (n.Style == 0 || n.Style == yaml.DoubleQuotedStyle)
}

// patch は通常のスカラーやシーケンスの値部分だけを元の文字列で置き換える。
func patch(src string, n, v *yaml.Node) (string, bool) {
	if !ordinary(n) || !ordinary(v) {
		return "", false
	}
	if n.Kind == yaml.SequenceNode && v.Kind == yaml.SequenceNode && len(n.Content) > 0 {
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode || !ordinary(c) || strings.Contains(c.Value, "\n") {
				return "", false
			}
		}
		var b strings.Builder
		indent := strings.Repeat(" ", n.Column-1)
		if len(v.Content) == 0 {
			b.WriteString(indent + "[]\n")
		}
		for _, c := range v.Content {
			if c.Kind != yaml.ScalarNode {
				return "", false
			}
			b.WriteString(indent + "- " + scalarFor(c, false) + "\n")
		}
		start := offset(src, n.Line, 1)
		last := n.Content[len(n.Content)-1]
		end := offset(src, last.Line+1, 1)
		return src[:start] + b.String() + src[end:], true
	}
	if n.Kind != yaml.ScalarNode || v.Kind != yaml.ScalarNode || strings.Contains(n.Value, "\n") {
		return "", false
	}
	start := offset(src, n.Line, n.Column)
	end := strings.IndexByte(src[start:], '\n')
	if end < 0 {
		end = len(src)
	} else {
		end += start
	}
	line := src[start:end]
	r := []rune(line)
	stop := len(r)
	comment := -1
	quoted := len(r) > 0 && r[0] == '"'
	escaped := false
	for i, ch := range r {
		if quoted {
			if i > 0 && ch == '"' && !escaped {
				stop = i + 1
				quoted = false
			}
			if ch == '\\' && !escaped {
				escaped = true
			} else {
				escaped = false
			}
			continue
		}
		if ch == '#' && (i == 0 || r[i-1] == ' ' || r[i-1] == '\t') {
			comment = i
			break
		}
	}
	if quoted {
		return "", false
	}
	if n.Style != yaml.DoubleQuotedStyle {
		stop = len(r)
		if comment >= 0 {
			stop = comment
		}
		for stop > 0 && (r[stop-1] == ' ' || r[stop-1] == '\t' || r[stop-1] == '\r') {
			stop--
		}
	}
	value := scalarFor(v, n.Style == yaml.DoubleQuotedStyle)
	prefix := ""
	if n.Tag == "!!null" {
		prefix = " "
	}
	tail := string(r[stop:])
	if comment >= 0 {
		spaces := comment - len([]rune(prefix+value))
		if spaces < 1 {
			spaces = 1
		}
		tail = strings.Repeat(" ", spaces) + string(r[comment:])
	}
	return src[:start] + prefix + value + tail + src[end:], true
}
func scalarFor(n *yaml.Node, forceQuoted bool) string {
	if n.Tag == "!!str" {
		if forceQuoted {
			return strconv.Quote(n.Value)
		}
		if legacyPlainKeyword(n.Value) {
			return strconv.Quote(n.Value)
		}
		candidate := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: n.Value}
		b, err := encode(candidate)
		if err == nil {
			plain := strings.TrimSpace(string(b))
			var decoded yaml.Node
			if !strings.ContainsAny(plain, "\r\n") && yaml.Unmarshal(b, &decoded) == nil && len(decoded.Content) == 1 && decoded.Content[0].Style == 0 && decoded.Content[0].Tag == "!!str" && decoded.Content[0].Value == n.Value {
				return plain
			}
		}
		return strconv.Quote(n.Value)
	}
	return n.Value
}

func legacyPlainKeyword(value string) bool {
	switch strings.ToLower(value) {
	case "yes", "no", "true", "false", "on", "off", "null", "~":
		return true
	}
	return false
}

func normalizeStrings(n *yaml.Node) {
	for _, child := range n.Content {
		normalizeStrings(child)
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
		if strings.HasPrefix(scalarFor(n, false), `"`) {
			n.Style = yaml.DoubleQuotedStyle
		} else {
			n.Style = 0
		}
	}
}

// Hash はファイル内容の SHA-256（小文字の16進表記）。
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// WriteFile は同じディレクトリに fsync 済みの一時ファイルを作り、置換する。
// Windows でも os.Rename の置換を利用する。失敗時は元ファイルを削除しない。
func WriteFile(path string, data []byte) error {
	return writeFile(path, data, os.Rename)
}
func writeFile(path string, data []byte, rename func(string, string) error) error {
	if _, e := config.Parse(data); e != nil {
		return errInvalid
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if e != nil {
		return errors.New("設定の一時ファイルを作成できません")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e == nil {
		e = rename(f.Name(), path)
	}
	if e != nil {
		return errors.New("設定ファイルを置換できません")
	}
	return nil
}

func plainTree(n *yaml.Node) bool {
	if !ordinary(n) || strings.Contains(n.Value, "\n") {
		return false
	}
	for _, c := range n.Content {
		if !plainTree(c) {
			return false
		}
	}
	return true
}
func lastLine(n *yaml.Node) int {
	line := n.Line
	for _, c := range n.Content {
		if v := lastLine(c); v > line {
			line = v
		}
	}
	return line
}

// omitEmptyFields は構造体由来の nil フィールドを YAML に出さない。
func omitEmptyFields(n *yaml.Node) {
	for _, child := range n.Content {
		omitEmptyFields(child)
	}
	if n.Kind != yaml.MappingNode {
		return
	}
	content := n.Content[:0]
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i+1].Tag == "!!null" {
			continue
		}
		content = append(content, n.Content[i], n.Content[i+1])
	}
	n.Content = content
}
