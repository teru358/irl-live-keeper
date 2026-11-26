package web

import (
	"errors"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
)

func isSourceFilesAPI(p string) bool {
	return p == "/api/source-files" || strings.HasPrefix(p, "/api/source-files/")
}
func sourceFilesMethod(p, m string) string {
	if p == "/api/source-files" {
		if m == http.MethodGet || m == http.MethodPost {
			return m
		}
		return ""
	}
	if strings.TrimPrefix(p, "/api/source-files/") != "" && !strings.Contains(strings.TrimPrefix(p, "/api/source-files/"), "/") && m == http.MethodDelete {
		return m
	}
	if m == http.MethodGet {
		return m
	}
	return ""
}

func sourceFiles(b Backend) (*sources.FilesStore, bool) {
	v, ok := b.(interface{ SourceFilesStore() *sources.FilesStore })
	if !ok || v.SourceFilesStore() == nil {
		return nil, false
	}
	return v.SourceFilesStore(), true
}
func fileUsers(b Backend, name string) ([]string, error) {
	v, ok := b.(interface{ SourcesStore() *sources.Store })
	if !ok {
		return []string{}, nil
	}
	used := []string{}
	var readErr error
	_ = v.SourcesStore().Transaction(b.Snapshot(), func(list []sources.Source, err error) ([]sources.Source, bool) {
		if err != nil {
			readErr = err
			return nil, false
		}
		needle := "/source/files/" + name
		for _, src := range list {
			if strings.Contains(src.CSS, needle) {
				used = append(used, src.Name)
			}
		}
		return nil, false
	})
	return used, readErr
}
func serveSourceFilesAPI(w http.ResponseWriter, r *http.Request, b Backend, lang i18n.Lang) {
	w.Header().Set("Cache-Control", "no-store")
	store, ok := sourceFiles(b)
	if !ok {
		writeError(w, 503, lang, i18n.New("api.stopped"))
		return
	}
	if r.URL.Path == "/api/source-files" {
		switch r.Method {
		case http.MethodGet:
			var sourceErr error
			list, err := store.List(func(n string) []string {
				refs, e := fileUsers(b, n)
				if e != nil {
					sourceErr = e
				}
				return refs
			})
			if sourceErr != nil {
				writeError(w, 409, lang, i18n.New("sources.unreadable"))
				return
			}
			if err != nil {
				writeError(w, 500, lang, i18n.New("source_files.io"))
				return
			}
			writeJSON(w, 200, map[string]any{"files": list})
		case http.MethodPost:
			if sourceKind(r) != "local" {
				lan, ok := b.(interface{ AdminLANEnabled() bool })
				if !ok || !lan.AdminLANEnabled() {
					writeError(w, 403, lang, i18n.New("settings.local_only"))
					return
				}
			}
			name, data, err := sources.ReadUpload(w, r)
			if err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) || errors.Is(err, sources.ErrFileTooLarge) {
					writeError(w, 413, lang, i18n.New("source_files.limit"))
				} else {
					writeError(w, 400, lang, i18n.New("source_files.invalid"))
				}
				return
			}
			if err = store.Save(name, data); err != nil {
				if errors.Is(err, os.ErrExist) {
					writeError(w, 409, lang, i18n.New("source_files.duplicate"))
				} else if errors.Is(err, sources.ErrFileTooLarge) || errors.Is(err, sources.ErrTotalTooLarge) {
					writeError(w, 413, lang, i18n.New("source_files.limit"))
				} else {
					writeError(w, 400, lang, i18n.New("source_files.invalid"))
				}
				return
			}
			requestLogger(r).Info("source file uploaded", "name", name, "size", len(data))
			writeJSON(w, 201, map[string]any{"ok": true, "name": name, "size": len(data)})
		}
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/source-files/")
	if path.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		writeError(w, 400, lang, i18n.New("source_files.invalid"))
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, 405, lang, i18n.New("api.method"))
		return
	}
	used, readErr := fileUsers(b, name)
	if readErr != nil {
		writeError(w, 409, lang, i18n.New("sources.unreadable"))
		return
	}
	if len(used) > 0 && r.URL.Query().Get("confirm") != "true" {
		writeJSON(w, 409, map[string]any{"error": i18n.Render(lang, i18n.New("source_files.in_use")), "code": "source_files.in_use", "used_by": used})
		return
	}
	if err := store.Delete(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, lang, i18n.New("api.not_found"))
		} else {
			writeError(w, 500, lang, i18n.New("source_files.io"))
		}
		return
	}
	requestLogger(r).Info("source file deleted", "name", name)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func serveSourceFile(w http.ResponseWriter, r *http.Request, b Backend) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	store, ok := sourceFiles(b)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/source/files/")
	data, typ, err := store.Open(name)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if typ == "image/svg+xml" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	} else {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:")
	}
	_, _ = w.Write(data)
}
