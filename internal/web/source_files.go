package web

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
	if rest := strings.TrimPrefix(p, "/api/source-files/"); rest != "" && m == http.MethodDelete {
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

// sourceFileJSON is one row of the list response. Name is the relative path
// ("/" separated). Reason is a code and ReasonText the same in the request language.
type sourceFileJSON struct {
	Name       string   `json:"name"`
	Size       int64    `json:"size"`
	Type       string   `json:"type"`
	Usable     bool     `json:"usable"`
	Reason     string   `json:"reason,omitempty"`
	ReasonText string   `json:"reason_text,omitempty"`
	UsedBy     []string `json:"used_by"`
}

type sourceFileNotice struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func sourceFileList(list sources.FileList, lang i18n.Lang) map[string]any {
	files := make([]sourceFileJSON, 0, len(list.Files))
	for _, f := range list.Files {
		row := sourceFileJSON{Name: f.Name, Size: f.Size, Type: f.Type, Usable: f.Usable, Reason: f.Reason, UsedBy: f.UsedBy}
		if f.Reason != "" {
			row.ReasonText = i18n.Render(lang, i18n.New("source_files.reason_"+f.Reason))
		}
		files = append(files, row)
	}
	notices := []sourceFileNotice{}
	if list.ScanLimited {
		notices = append(notices, sourceFileNotice{Code: "source_files.scan_limit", Message: i18n.Render(lang, i18n.New("source_files.scan_limit", "limit", strconv.Itoa(sources.MaxScanEntries)))})
	}
	return map[string]any{"files": files, "notices": notices}
}

func serveSourceFilesAPI(w http.ResponseWriter, r *http.Request, b Backend, e *Events, lang i18n.Lang) {
	w.Header().Set("Cache-Control", "no-store")
	store, ok := sourceFiles(b)
	if !ok {
		writeError(w, 503, lang, i18n.New("api.stopped"))
		return
	}
	if r.URL.Path == "/api/source-files" {
		switch r.Method {
		case http.MethodGet:
			// The saved sources are read before the folder, so that no store
			// lock is held while the other one is.
			usage, sourceErr := assetUsage(b)
			list, err := store.List(func(n string) []string { return useNames(usage[sources.AssetPrefix+n]) })
			if err != nil {
				writeError(w, 500, lang, i18n.New("source_files.io"))
				return
			}
			body := sourceFileList(list, lang)
			if sourceErr != nil {
				// Unused is not assumed: say that the uses are unknown.
				body["notices"] = append(body["notices"].([]sourceFileNotice), sourceFileNotice{Code: "sources.unreadable", Message: i18n.Render(lang, i18n.New("sources.unreadable"))})
			}
			// The rebuild read every file again, so a replacement with the same
			// size and time shows up in the pages that are open.
			refreshSources(e)
			writeJSON(w, 200, body)
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
				} else if errors.Is(err, sources.ErrNameTooLong) {
					writeError(w, 400, lang, i18n.New("source_files.name_too_long"))
				} else if errors.Is(err, sources.ErrReservedName) {
					writeError(w, 400, lang, i18n.New("source_files.reason_reserved_name"))
				} else {
					writeError(w, 400, lang, i18n.New("source_files.invalid"))
				}
				return
			}
			if err = store.Save(name, data); err != nil {
				if errors.Is(err, os.ErrExist) {
					writeError(w, 409, lang, i18n.New("source_files.duplicate"))
				} else if errors.Is(err, sources.ErrFileTooLarge) || errors.Is(err, sources.ErrTotalTooLarge) || errors.Is(err, sources.ErrTooManyFiles) {
					writeError(w, 413, lang, i18n.New("source_files.limit"))
				} else {
					writeError(w, 400, lang, i18n.New("source_files.invalid"))
				}
				return
			}
			requestLogger(r).Info("source file uploaded", "name", name, "size", len(data))
			refreshSources(e)
			writeJSON(w, 201, map[string]any{"ok": true, "name": name, "size": len(data)})
		}
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/source-files/")
	if r.Method != http.MethodDelete {
		writeError(w, 405, lang, i18n.New("api.method"))
		return
	}
	if name == "" || strings.ContainsRune(name, '\\') {
		writeError(w, 400, lang, i18n.New("source_files.invalid"))
		return
	}
	usage, readErr := assetUsage(b)
	if readErr != nil {
		writeError(w, 409, lang, i18n.New("sources.unreadable"))
		return
	}
	if used := usage[sources.AssetPrefix+name]; len(used) > 0 && r.URL.Query().Get("confirm") != "true" {
		writeJSON(w, 409, map[string]any{"error": i18n.Render(lang, i18n.New("source_files.in_use")), "code": "source_files.in_use", "used_by": useNames(used), "used_titles": useTitles(used)})
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
	refreshSources(e)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// refreshSources renders the sources again and sends what changed.
func refreshSources(e *Events) {
	if e != nil {
		e.updateSources(time.Now())
	}
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
	data, typ, hash, err := store.OpenWithHash(name)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	etag := `"` + hash + `"`
	w.Header().Set("ETag", etag)
	if typ == "image/svg+xml" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	} else {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(data)
}
