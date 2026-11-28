package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/sources"
)

func getPublic(h http.Handler, p string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
	return w
}

func writeTestFile(t *testing.T, dir, rel string, data []byte) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFilesListShowsUnusableFilesWithReasons(t *testing.T) {
	dir := t.TempDir()
	b := &fileBackend{files: sources.FilesStore{Dir: dir}}
	writeTestFile(t, dir, "ok.png", pngBytes(t))
	writeTestFile(t, dir, "a/b/c/deep.png", pngBytes(t))
	writeTestFile(t, dir, "bad name.png", pngBytes(t))
	writeTestFile(t, dir, "broken.png", []byte("nope"))
	w := httptest.NewRecorder()
	apiHandler(b, nil).ServeHTTP(w, httptest.NewRequest("GET", "/api/source-files?lang=en", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var got struct {
		Files []struct {
			Name       string   `json:"name"`
			Size       int64    `json:"size"`
			Type       string   `json:"type"`
			Usable     bool     `json:"usable"`
			Reason     string   `json:"reason"`
			ReasonText string   `json:"reason_text"`
			UsedBy     []string `json:"used_by"`
		} `json:"files"`
		Notices []map[string]string `json:"notices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Notices) != 0 || len(got.Files) != 4 {
		t.Fatalf("%+v", got)
	}
	by := map[string]int{}
	for i, f := range got.Files {
		by[f.Name] = i
	}
	deep := got.Files[by["a/b/c/deep.png"]]
	if !deep.Usable || deep.Type != "image/png" || deep.Size == 0 || deep.Reason != "" || deep.ReasonText != "" || deep.UsedBy == nil {
		t.Fatalf("%+v", deep)
	}
	for name, reason := range map[string]string{"bad name.png": "bad_name", "broken.png": "not_image"} {
		f := got.Files[by[name]]
		if f.Usable || f.Reason != reason || f.ReasonText == "" || strings.Contains(f.ReasonText, "source_files.") || f.Type != "" {
			t.Fatalf("%s: %+v", name, f)
		}
	}
	w = httptest.NewRecorder()
	apiHandler(b, nil).ServeHTTP(w, httptest.NewRequest("GET", "/api/source-files?lang=ja", nil))
	if !strings.Contains(w.Body.String(), "画像として読めません") {
		t.Fatal(w.Body.String())
	}
}

func TestSourceFilesListReportsScanLimitNotice(t *testing.T) {
	dir := t.TempDir()
	b := &fileBackend{files: sources.FilesStore{Dir: dir}}
	for i := 0; i <= sources.MaxScanEntries; i++ {
		writeTestFile(t, dir, fmt.Sprintf("n%05d.txt", i), nil)
	}
	w := httptest.NewRecorder()
	apiHandler(b, nil).ServeHTTP(w, httptest.NewRequest("GET", "/api/source-files?lang=en", nil))
	var got struct {
		Files   []map[string]any    `json:"files"`
		Notices []map[string]string `json:"notices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 {
		t.Fatal(w.Code, err)
	}
	if len(got.Files) != sources.MaxScanEntries || len(got.Notices) != 1 || got.Notices[0]["code"] != "source_files.scan_limit" || !strings.Contains(got.Notices[0]["message"], "10000") {
		t.Fatalf("%d %+v", len(got.Files), got.Notices)
	}
}

func TestNestedSourceFileIsServedByExactNameWithoutScanning(t *testing.T) {
	dir := t.TempDir()
	b := &fileBackend{files: sources.FilesStore{Dir: dir}}
	writeTestFile(t, dir, "Logos/a/b/Mark.png", pngBytes(t))
	writeTestFile(t, dir, "broken.png", []byte("nope"))
	h := New(b, testPass)
	if w := getPublic(h, "/source/files/Logos/a/b/Mark.png"); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("%d %#v", w.Code, w.Header())
	}
	scans := b.files.ScanCount()
	for i := 0; i < 10; i++ {
		getPublic(h, "/source/files/Logos/a/b/Mark.png")
		getPublic(h, "/source/files/missing.png")
		getPublic(h, "/source/files/broken.png")
	}
	if got := b.files.ScanCount(); got != scans {
		t.Fatalf("public requests scanned the folder: %d -> %d", scans, got)
	}
	for _, p := range []string{"/source/files/logos/a/b/mark.png", "/source/files/LOGOS/A/B/MARK.PNG", "/source/files/Logos/a/b/Mark.PNG", "/source/files/broken.png", "/source/files/Logos", "/source/files/Logos/a/b/", "/source/files/"} {
		if w := getPublic(h, p); w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
}

func TestSourceFilePublicRequestRevalidatesAReplacedFile(t *testing.T) {
	dir := t.TempDir()
	b := &fileBackend{files: sources.FilesStore{Dir: dir}}
	writeTestFile(t, dir, "x.png", pngBytes(t))
	h := New(b, testPass)
	if w := getPublic(h, "/source/files/x.png"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	writeTestFile(t, dir, "x.png", []byte("garbage"))
	if w := getPublic(h, "/source/files/x.png"); w.Code != 404 {
		t.Fatalf("served replaced file: %d", w.Code)
	}
}

func TestUploadReindexesAndServes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "source-files")
	b := &fileBackend{files: sources.FilesStore{Dir: dir}}
	h := New(b, testPass)
	if w := getPublic(h, "/source/files/photo.png"); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := uploadRequest(t, h, b, "127.0.0.1:1", true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := getPublic(h, "/source/files/photo.png"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("DELETE", "/api/source-files/photo.png", nil)
	r.Header.Set(headerName, "1")
	w := httptest.NewRecorder()
	apiHandler(b, nil).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := getPublic(h, "/source/files/photo.png"); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
