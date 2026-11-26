package sources

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"gopkg.in/yaml.v3"
)

const MaxCSS = 64 << 10
const MaxText = 64 << 10

var NamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)

type Style struct {
	Font              string  `json:"font" yaml:"font"`
	FontSize          float64 `json:"font_size" yaml:"font_size"`
	Color             string  `json:"color" yaml:"color"`
	FontWeight        int     `json:"font_weight" yaml:"font_weight"`
	OutlineColor      string  `json:"outline_color" yaml:"outline_color"`
	OutlineWidth      float64 `json:"outline_width" yaml:"outline_width"`
	ShadowColor       string  `json:"shadow_color" yaml:"shadow_color"`
	ShadowBlur        float64 `json:"shadow_blur" yaml:"shadow_blur"`
	ShadowX           float64 `json:"shadow_x" yaml:"shadow_x"`
	ShadowY           float64 `json:"shadow_y" yaml:"shadow_y"`
	BackgroundColor   string  `json:"background_color" yaml:"background_color"`
	BackgroundOpacity float64 `json:"background_opacity" yaml:"background_opacity"`
	BorderRadius      float64 `json:"border_radius" yaml:"border_radius"`
	Padding           float64 `json:"padding" yaml:"padding"`
	Align             string  `json:"align" yaml:"align"`
	Transition        string  `json:"transition" yaml:"transition"`
}
type Source struct {
	Name      string `json:"name" yaml:"name"`
	Title     string `json:"title" yaml:"title"`
	Template  string `json:"template" yaml:"template"`
	Style     Style  `json:"style" yaml:"style"`
	CSS       string `json:"css" yaml:"css"`
	OBSWidth  int    `json:"obs_width" yaml:"obs_width"`
	OBSHeight int    `json:"obs_height" yaml:"obs_height"`
}

func Default() Source { return Source{OBSWidth: 800, OBSHeight: 200} }
func (s Source) Validate(snapshot core.Snapshot) error {
	invalid := func() error { return &ValidationError{Message: i18n.New("sources.invalid")} }
	if !NamePattern.MatchString(s.Name) || len(s.Title) > 1024 || len(s.Template) > MaxText || len(s.CSS) > MaxCSS || s.OBSWidth < 1 || s.OBSWidth > 8192 || s.OBSHeight < 1 || s.OBSHeight > 8192 {
		return invalid()
	}
	st := s.Style
	if len(st.Font) > 256 || st.FontWeight != 0 && (st.FontWeight < 100 || st.FontWeight > 900 || st.FontWeight%100 != 0) {
		return invalid()
	}
	for _, c := range []string{st.Color, st.OutlineColor, st.ShadowColor, st.BackgroundColor} {
		if c != "" && !colorPattern.MatchString(c) {
			return invalid()
		}
	}
	for _, v := range []struct{ n, lo, hi float64 }{{st.FontSize, 0, 512}, {st.OutlineWidth, 0, 32}, {st.ShadowBlur, 0, 128}, {st.ShadowX, -512, 512}, {st.ShadowY, -512, 512}, {st.BackgroundOpacity, 0, 1}, {st.BorderRadius, 0, 4096}, {st.Padding, 0, 4096}} {
		if math.IsNaN(v.n) || math.IsInf(v.n, 0) || v.n < v.lo || v.n > v.hi {
			return invalid()
		}
	}
	if st.Align != "" && st.Align != "left" && st.Align != "center" && st.Align != "right" {
		return invalid()
	}
	if st.Transition != "" && st.Transition != "none" && st.Transition != "fade" {
		return invalid()
	}
	_, err := template.Compile(s.Template, Definitions(snapshot))
	return err
}

type ValidationError struct{ Message i18n.Msg }

func (e *ValidationError) Error() string { return i18n.Render(i18n.JA, e.Message) }

// Store serializes reload and replacement. Every transaction reads the disk again.
type Store struct {
	mu   sync.Mutex
	Path string
}

func (s *Store) Transaction(snapshot core.Snapshot, f func([]Source, error) ([]Source, bool)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read(snapshot)
	next, write := f(list, err)
	if !write || err != nil {
		return nil
	}
	data, err := yaml.Marshal(next)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".sources-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = os.Rename(tmp.Name(), s.Path)
	}
	return err
}
func (s *Store) read(snapshot core.Snapshot) ([]Source, error) {
	fail := func() ([]Source, error) { return []Source{}, &ValidationError{Message: i18n.New("sources.unreadable")} }
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []Source{}, nil
	}
	if err != nil {
		return fail()
	}
	var nodes []yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(raw))
	d.KnownFields(true)
	if d.Decode(&nodes) != nil || d.Decode(new(any)) != io.EOF {
		return fail()
	}
	list := []Source{}
	seen := map[string]bool{}
	for _, n := range nodes {
		v := Default()
		b, e := yaml.Marshal(n)
		if e != nil {
			return fail()
		}
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if dec.Decode(&v) != nil {
			return fail()
		}
		if v.Validate(snapshot) != nil || seen[v.Name] {
			return fail()
		}
		seen[v.Name] = true
		list = append(list, v)
	}
	return list, nil
}
