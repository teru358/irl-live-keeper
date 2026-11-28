package web

import (
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/sources"
)

func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal(what, "が、読み終わらない本文の保存に待たされている")
	}
}

// 読み終わらない本文の保存要求があっても、ほかの保存や一覧は待たない（ソース）。
func TestSourcesSlowBodyDoesNotHoldStore(t *testing.T) {
	b := &sourcesBackend{store: sources.Store{Path: filepath.Join(t.TempDir(), "sources.yaml")}}
	pr, pw := io.Pipe()
	r := httptest.NewRequest("POST", "/api/sources", pr)
	r.Header.Set(headerName, "1")
	slow := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		serveSources(w, r, b, "en")
		slow <- w
	}()
	time.Sleep(50 * time.Millisecond) // 本文を読み始めさせる
	within(t, "一覧", func() {
		w := httptest.NewRecorder()
		serveSources(w, httptest.NewRequest("GET", "/api/sources", nil), b, "en")
		if w.Code != 200 {
			t.Error(w.Code, w.Body.String())
		}
	})
	within(t, "別の保存", func() {
		body := `{"name":"other","body":"x"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/sources", strings.NewReader(body))
		r.Header.Set(headerName, "1")
		serveSources(w, r, b, "en")
		if w.Code != 201 {
			t.Error(w.Code, w.Body.String())
		}
	})
	_ = pw.CloseWithError(io.ErrUnexpectedEOF)
	select {
	case w := <-slow:
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("遅い要求が終わらない")
	}
}

// 全体の hash は使われないので返さない（ページごとの page_hashes だけ）。
func TestGetSettingsHasNoWholeHash(t *testing.T) {
	b := settingsFixture(t)
	w := settingsCall(b, "GET", "/api/settings", "", "127.0.0.1:42", true)
	if strings.Contains(w.Body.String(), `"hash"`) || !strings.Contains(w.Body.String(), `"page_hashes"`) {
		t.Fatal(w.Body.String())
	}
}

// 読み終わらない本文の保存要求があっても、設定の別の要求は App.mu 側（SettingsTransaction）を待たない。
func TestSettingsSlowBodyDoesNotHoldTransaction(t *testing.T) {
	b := settingsFixture(t)
	pr, pw := io.Pipe()
	r := httptest.NewRequest("PUT", "/api/settings", pr)
	r.RemoteAddr = "127.0.0.1:4"
	r.Header.Set(headerName, "1")
	slow := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		serveSettings(w, r, b, "en")
		slow <- w
	}()
	time.Sleep(50 * time.Millisecond)
	within(t, "ほかの設定の要求", func() {
		w := settingsCall(b, "GET", "/api/settings", "", "127.0.0.1:42", true)
		if w.Code != 200 {
			t.Error(w.Code, w.Body.String())
		}
	})
	_ = pw.CloseWithError(io.ErrUnexpectedEOF)
	select {
	case w := <-slow:
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("遅い要求が終わらない")
	}
}
