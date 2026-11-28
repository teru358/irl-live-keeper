package sources

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"gopkg.in/yaml.v3"
)

// MaxBody is the largest body in bytes, including any data: URLs in it.
const MaxBody = 64 << 10

var NamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// ReservedNames are URL path segments under /source/ that a source cannot take.
var ReservedNames = map[string]bool{"files": true, "assets": true}

// Source is one saved source. Body is the HTML fragment for the inside of <body>
// (<style>, HTML and {...} template tags together).
type Source struct {
	// Broken is derived from the current snapshot and never persisted or exposed as source JSON.
	Broken    *template.Error `json:"-" yaml:"-"`
	Name      string          `json:"name" yaml:"name"`
	Title     string          `json:"title" yaml:"title"`
	Body      string          `json:"body" yaml:"body"`
	OBSWidth  int             `json:"obs_width" yaml:"obs_width"`
	OBSHeight int             `json:"obs_height" yaml:"obs_height"`
}

func Default() Source { return Source{OBSWidth: 800, OBSHeight: 200} }
func (s Source) Validate(snapshot core.Snapshot) error {
	invalid := func() error { return &ValidationError{Message: i18n.New("sources.invalid")} }
	if !NamePattern.MatchString(s.Name) || len(s.Title) > 1024 || s.OBSWidth < 1 || s.OBSWidth > 8192 || s.OBSHeight < 1 || s.OBSHeight > 8192 {
		return invalid()
	}
	if ReservedNames[s.Name] {
		return &ValidationError{Message: i18n.New("sources.reserved_name")}
	}
	if len(s.Body) > MaxBody {
		return &ValidationError{Message: i18n.New("sources.body_too_large")}
	}
	_, err := CompileHTML(s.Body, Definitions(snapshot))
	return err
}

type ValidationError struct{ Message i18n.Msg }

func (e *ValidationError) Error() string { return i18n.Render(i18n.JA, e.Message) }

// Store serializes reload and replacement. Every transaction reads the disk again.
type Store struct {
	mu     sync.Mutex
	Path   string
	logger *slog.Logger
	broken map[string]brokenState
}

type brokenState struct {
	body, code   string
	line, column int
}

// SetLogger selects the current generation logger without resetting warning history.
func (s *Store) SetLogger(logger *slog.Logger) {
	s.mu.Lock()
	s.logger = logger
	s.mu.Unlock()
}

// brokenChanges runs before the transaction callback, which may modify the list.
func (s *Store) brokenChanges(list []Source) []struct{ name, code string } {
	next := make(map[string]brokenState)
	changed := []struct{ name, code string }{}
	for _, src := range list {
		if src.Broken == nil {
			continue
		}
		e := src.Broken
		state := brokenState{src.Body, e.Message.Code, e.Line, e.Column}
		next[src.Name] = state
		if previous, ok := s.broken[src.Name]; !ok || previous != state {
			changed = append(changed, struct{ name, code string }{src.Name, e.Message.Code})
		}
	}
	s.broken = next
	return changed
}

func (s *Store) Transaction(snapshot core.Snapshot, f func([]Source, error) ([]Source, bool)) error {
	var changed []struct{ name, code string }
	var logger *slog.Logger
	err := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		list, err := s.read(snapshot)
		changed = s.brokenChanges(list)
		logger = s.logger
		return s.replace(list, err, f)
	}()
	// Logging can wait on subscribers or a file, so release the store lock first.
	if logger == nil {
		logger = slog.Default()
	}
	for _, change := range changed {
		logger.Warn("source template needs repair", "name", change.name, "code", change.code)
	}
	return err
}

func (s *Store) replace(list []Source, err error, f func([]Source, error) ([]Source, bool)) error {
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
		if seen[v.Name] {
			return fail()
		}
		if err := v.Validate(snapshot); err != nil {
			var te *template.Error
			if !errors.As(err, &te) {
				return fail()
			}
			v.Broken = te
		}
		seen[v.Name] = true
		list = append(list, v)
	}
	return list, nil
}
