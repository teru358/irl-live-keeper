package web

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/sources"
)

type fileBackend struct {
	fakeBackend
	files       sources.FilesStore
	sourceStore sources.Store
	lan         bool
}

func (b *fileBackend) SourceFilesStore() *sources.FilesStore { return &b.files }
func (b *fileBackend) AdminLANEnabled() bool                 { return b.lan }
func (b *fileBackend) SourcesStore() *sources.Store          { return &b.sourceStore }
func pngBytes(t *testing.T) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	im.Set(0, 0, color.White)
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func uploadRequest(t *testing.T, h http.Handler, b *fileBackend, remote string, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "photo.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(pngBytes(t))
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/source-files", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set(headerName, "1")
	r.RemoteAddr = remote
	if auth {
		r.SetBasicAuth("admin", testPass)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestSourceFilesLANRequiresAdminLAN(t *testing.T) {
	b := &fileBackend{files: sources.FilesStore{Dir: filepath.Join(t.TempDir(), "source-files")}}
	h := New(b, testPass)
	w := uploadRequest(t, h, b, "192.168.1.12:1234", true)
	if w.Code != 403 {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	b.lan = true
	w = uploadRequest(t, h, b, "192.168.1.12:1234", true)
	if w.Code != 201 {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}
func TestSourceFileIsPublicAndHasSafeType(t *testing.T) {
	dir := t.TempDir()
	b := &fileBackend{files: sources.FilesStore{Dir: dir}}
	if err := b.files.Save("photo.png", pngBytes(t)); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/source/files/photo.png", nil)
	r.RemoteAddr = "192.168.1.12:1234"
	w := httptest.NewRecorder()
	New(b, testPass).ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("%d %#v", w.Code, w.Header())
	}
	if w.Header().Get("Cache-Control") != "no-cache" || !regexp.MustCompile(`^"[0-9a-f]{64}"$`).MatchString(w.Header().Get("ETag")) {
		t.Fatalf("cache headers: %#v", w.Header())
	}
	if _, err := os.Stat(filepath.Join(dir, "photo.png")); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFileETagConditionalAndReplacedContents(t *testing.T) {
	dir := t.TempDir()
	b := &fileBackend{files: sources.FilesStore{Dir: dir}}
	first := pngBytes(t)
	if err := b.files.Save("photo.png", first); err != nil {
		t.Fatal(err)
	}
	h := New(b, testPass)
	initial := getPublic(h, "/source/files/photo.png")
	if initial.Code != 200 {
		t.Fatal(initial.Code)
	}
	etag := initial.Header().Get("ETag")
	r := httptest.NewRequest("GET", "/source/files/photo.png", nil)
	r.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 || w.Header().Get("ETag") != etag {
		t.Fatalf("conditional response: %d %q %#v", w.Code, w.Body.String(), w.Header())
	}

	second := pngVariant(t, color.RGBA{R: 255, A: 255})
	if bytes.Equal(first, second) {
		t.Fatal("test PNGs are identical")
	}
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), second, 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(dir, "photo.png"), old, old); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("GET", "/source/files/photo.png", nil)
	r.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("ETag") == etag || !bytes.Equal(w.Body.Bytes(), second) {
		t.Fatalf("replacement response: %d ETag=%q body matches=%v", w.Code, w.Header().Get("ETag"), bytes.Equal(w.Body.Bytes(), second))
	}
	if w.Header().Get("ETag") != fmt.Sprintf("%q", fmt.Sprintf("%x", sha256.Sum256(second))) {
		t.Fatalf("ETag is not content SHA-256: %q", w.Header().Get("ETag"))
	}
}

func TestSVGSourceFileHasSandboxHeaders(t *testing.T) {
	b := &fileBackend{files: sources.FilesStore{Dir: t.TempDir()}}
	data := []byte("<?xml version=\"1.0\"?><svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")
	if err := b.files.Save("icon.svg", data); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/source/files/icon.svg", nil)
	w := httptest.NewRecorder()
	New(b, testPass).ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/svg+xml" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Security-Policy") != "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox" {
		t.Fatalf("%d %#v", w.Code, w.Header())
	}
}

func pngVariant(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	im.SetRGBA(0, 0, c)
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
