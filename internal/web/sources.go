package web

import (
	"encoding/json"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"io"
	"net/http"
	"sort"
	"strings"
)

func isSourcesPath(p string) bool {
	return p == "/api/source-values" || p == "/api/sources/test-obs/alive" || p == "/api/sources" || strings.HasPrefix(p, "/api/sources/") && sources.NamePattern.MatchString(strings.TrimPrefix(p, "/api/sources/"))
}
func sourcesMethod(p, m string) bool {
	if p == "/api/sources/preview" || p == "/api/sources/test-obs/alive" {
		return m == "POST"
	}
	if p == "/api/sources/test-obs" {
		return m == "PUT" || m == "DELETE"
	}
	if p == "/api/sources" || p == "/api/source-values" {
		return m == "GET" || m == "POST"
	}
	return m == "GET" || m == "PUT" || m == "DELETE"
}
func serveSources(w http.ResponseWriter, r *http.Request, b Backend, lang i18n.Lang) {
	w.Header().Set("Cache-Control", "no-store")
	provider, ok := b.(interface{ SourcesStore() *sources.Store })
	if !ok {
		writeError(w, 503, lang, i18n.New("api.stopped"))
		return
	}
	collection := r.URL.Path == "/api/sources"
	name := strings.TrimPrefix(r.URL.Path, "/api/sources/")
	saved, deleted := false, false
	err := provider.SourcesStore().Transaction(b.Snapshot(), func(list []sources.Source, loadErr error) ([]sources.Source, bool) {
		if loadErr != nil {
			if r.Method == "GET" {
				writeJSON(w, 200, map[string]any{"sources": []sources.Source{}, "broken": map[string]any{}, "names": sourceNames(b), "default": sources.Default(), "error": i18n.Render(lang, i18n.New("sources.unreadable")), "code": "sources.unreadable"})
			} else {
				writeError(w, 409, lang, i18n.New("sources.unreadable"))
			}
			return nil, false
		}
		index := -1
		for i, v := range list {
			if v.Name == name {
				index = i
			}
		}
		if r.Method == "GET" {
			if collection {
				broken := map[string]any{}
				for _, src := range list {
					if e := src.Broken; e != nil {
						broken[src.Name] = map[string]any{"code": e.Message.Code, "error": i18n.Render(lang, e.Message), "line": e.Line, "column": e.Column}
					}
				}
				writeJSON(w, 200, map[string]any{"sources": list, "broken": broken, "names": sourceNames(b), "default": sources.Default()})
			} else if index >= 0 {
				writeJSON(w, 200, list[index])
			} else {
				writeError(w, 404, lang, i18n.New("api.not_found"))
			}
			return nil, false
		}
		if !collection && index < 0 {
			writeError(w, 404, lang, i18n.New("api.not_found"))
			return nil, false
		}
		if r.Method == "DELETE" {
			deleted = true
			return append(list[:index], list[index+1:]...), true
		}
		v := sources.Default()
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
		d.DisallowUnknownFields()
		if d.Decode(&v) != nil || d.Decode(new(any)) != io.EOF {
			writeError(w, 400, lang, i18n.New("sources.invalid"))
			return nil, false
		}
		if e := v.Validate(b.Snapshot()); e != nil {
			var te *template.Error
			if errors.As(e, &te) {
				writeJSON(w, 422, map[string]any{"error": i18n.Render(lang, te.Message), "code": te.Message.Code, "line": te.Line, "column": te.Column, "fields": map[string]string{"template": i18n.Render(lang, te.Message)}})
			} else {
				writeError(w, 422, lang, i18n.New("sources.invalid"))
			}
			return nil, false
		}
		if !collection && v.Name != name {
			writeError(w, 422, lang, i18n.New("sources.invalid"))
			return nil, false
		}
		if collection {
			for _, old := range list {
				if old.Name == v.Name {
					writeError(w, 409, lang, i18n.New("sources.duplicate"))
					return nil, false
				}
			}
			list = append(list, v)
		} else {
			list[index] = v
		}
		name = v.Name
		saved = true
		return list, true
	})
	if err != nil {
		writeError(w, 500, lang, i18n.New("sources.write_failed"))
		return
	}
	if saved || deleted {
		event := "source saved"
		if deleted {
			event = "source deleted"
		}
		requestLogger(r).Info(event, "name", name)
		status := 200
		if collection {
			status = 201
		}
		writeJSON(w, status, map[string]bool{"ok": true})
	}
}

func sourceNames(b Backend) []string {
	names := []string{}
	for name := range sources.Definitions(b.Snapshot()) {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
