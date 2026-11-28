package web

import (
	"bytes"
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
	if p == "/api/sources/frame" {
		return m == "GET"
	}
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
	if r.URL.Path == "/api/sources/frame" {
		writeSourceFrame(w, "preview", false)
		return
	}
	provider, ok := b.(interface{ SourcesStore() *sources.Store })
	if !ok {
		writeError(w, 503, lang, i18n.New("api.stopped"))
		return
	}
	collection := r.URL.Path == "/api/sources"
	name := strings.TrimPrefix(r.URL.Path, "/api/sources/")
	saved, deleted := false, false
	// The body is read before the store lock is taken, and the reply is written
	// after it is released: a slow sender or reader must not hold up other saves
	// or the source list. Inside the transaction we only decide.
	var body []byte
	var bodyErr error
	if r.Method == "PUT" || r.Method == "POST" {
		body, bodyErr = io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	}
	var res reply
	err := provider.SourcesStore().Transaction(b.Snapshot(), func(list []sources.Source, loadErr error) ([]sources.Source, bool) {
		if loadErr != nil {
			if r.Method == "GET" {
				res = reply{200, map[string]any{"sources": []sources.Source{}, "broken": map[string]any{}, "names": sourceNames(b), "default": sources.Default(), "error": i18n.Render(lang, i18n.New("sources.unreadable")), "code": "sources.unreadable"}}
			} else {
				res = errorReply(409, lang, i18n.New("sources.unreadable"))
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
				defs := sources.Definitions(b.Snapshot())
				rows := make([]sourceRow, 0, len(list))
				for _, src := range list {
					rows = append(rows, sourceRow{src, sources.ReferencesOf(src.Body, defs).Cameras})
				}
				res = reply{200, map[string]any{"sources": rows, "broken": broken, "names": sourceNames(b), "default": sources.Default()}}
			} else if index >= 0 {
				res = reply{200, list[index]}
			} else {
				res = errorReply(404, lang, i18n.New("api.not_found"))
			}
			return nil, false
		}
		if !collection && index < 0 {
			res = errorReply(404, lang, i18n.New("api.not_found"))
			return nil, false
		}
		if r.Method == "DELETE" {
			deleted = true
			return append(list[:index], list[index+1:]...), true
		}
		v := sources.Default()
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		if bodyErr != nil || d.Decode(&v) != nil || d.Decode(new(any)) != io.EOF {
			res = errorReply(400, lang, i18n.New("sources.invalid"))
			return nil, false
		}
		if e := v.Validate(b.Snapshot()); e != nil {
			var te *template.Error
			if errors.As(e, &te) {
				res = reply{422, map[string]any{"error": i18n.Render(lang, te.Message), "code": te.Message.Code, "line": te.Line, "column": te.Column, "fields": map[string]string{"body": i18n.Render(lang, te.Message)}}}
			} else if ve := (*sources.ValidationError)(nil); errors.As(e, &ve) {
				res = errorReply(422, lang, ve.Message)
			} else {
				res = errorReply(422, lang, i18n.New("sources.invalid"))
			}
			return nil, false
		}
		if !collection && v.Name != name {
			res = errorReply(422, lang, i18n.New("sources.invalid"))
			return nil, false
		}
		if collection {
			for _, old := range list {
				if old.Name == v.Name {
					res = errorReply(409, lang, i18n.New("sources.duplicate"))
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
	if res.status != 0 {
		res.write(w)
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

// sourceRow is a source in the list, with the camera ids its body uses in any
// branch (cam<n>.* names outside <style> text). The editor does not send
// cameras back; it is not part of a saved source.
type sourceRow struct {
	sources.Source
	Cameras []string `json:"cameras"`
}

func sourceNames(b Backend) []string {
	names := []string{}
	for name := range sources.Definitions(b.Snapshot()) {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
