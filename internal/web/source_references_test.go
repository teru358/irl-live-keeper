package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources"
)

func refBackend(t *testing.T) (*fileBackend, string) {
	t.Helper()
	dir := t.TempDir()
	b := &fileBackend{files: sources.FilesStore{Dir: filepath.Join(dir, "source-files")}, sourceStore: sources.Store{Path: filepath.Join(dir, "sources.yaml")}}
	b.snap = core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1"}, {ID: "cam2"}}}
	return b, dir
}

func saveTestSources(t *testing.T, b *fileBackend, list ...sources.Source) {
	t.Helper()
	if err := b.sourceStore.Transaction(b.Snapshot(), func([]sources.Source, error) ([]sources.Source, bool) { return list, true }); err != nil {
		t.Fatal(err)
	}
}

func testSource(name, title, body string) sources.Source {
	s := sources.Default()
	s.Name, s.Title, s.Body = name, title, body
	return s
}

func apiCall(b Backend, e *Events, method, target string, body any) *httptest.ResponseRecorder {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, target, nil)
	} else {
		data, _ := json.Marshal(body)
		r = httptest.NewRequest(method, target, bytes.NewReader(data))
	}
	r.Header.Set(headerName, "1")
	w := httptest.NewRecorder()
	apiHandler(b, e).ServeHTTP(w, r)
	return w
}

func TestSourceFileUsageUsesTheParsedReferences(t *testing.T) {
	b, dir := refBackend(t)
	root := filepath.Join(dir, "source-files")
	for _, n := range []string{"a.png", "a.png2.png", "xa.png", "d/e.png", "unused.png"} {
		writeTestFile(t, root, n, pngBytes(t))
	}
	saveTestSources(t, b,
		testSource("overlay", "回線の表示", "{if cam1.online}<img src=\"&#47;source&#47;files/a.png\">{else}<span></span>{end}"),
		testSource("other", "", "<img src=\"/source/files/a.png2.png\"><img src=\"/source/files/xa.png\"><style>.x{background:url(/source/files/d/e.png)}</style><!-- <img src=\"/source/files/unused.png\"> -->"),
	)
	w := apiCall(b, nil, "GET", "/api/source-files", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var got struct {
		Files []struct {
			Name   string   `json:"name"`
			UsedBy []string `json:"used_by"`
		} `json:"files"`
		Notices []map[string]string `json:"notices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	used := map[string][]string{}
	for _, f := range got.Files {
		used[f.Name] = f.UsedBy
	}
	want := map[string][]string{"a.png": {"overlay"}, "a.png2.png": {"other"}, "xa.png": {"other"}, "d/e.png": {"other"}, "unused.png": {}}
	if !reflect.DeepEqual(used, want) || len(got.Notices) != 0 {
		t.Fatalf("%v want %v (%v)", used, want, got.Notices)
	}
}

func TestDeletingAnAssetInUseNeedsConfirmationAndNestedFilesCanBeDeleted(t *testing.T) {
	b, dir := refBackend(t)
	root := filepath.Join(dir, "source-files")
	writeTestFile(t, root, "a.png", pngBytes(t))
	writeTestFile(t, root, "d/e/n.png", pngBytes(t))
	writeTestFile(t, root, "free.png", pngBytes(t))
	saveTestSources(t, b,
		testSource("overlay", "回線の表示", "<img src=\"/source/files/a.png\">"),
		testSource("nested", "", "{if cam1.online}<img src=\"/source/files/d/e/n.png\">{end}"),
	)
	w := apiCall(b, nil, "DELETE", "/api/source-files/a.png?lang=ja", nil)
	var body struct {
		Code      string   `json:"code"`
		Error     string   `json:"error"`
		UsedBy    []string `json:"used_by"`
		UsedTitle []string `json:"used_titles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 409 || body.Code != "source_files.in_use" || !reflect.DeepEqual(body.UsedBy, []string{"overlay"}) || !reflect.DeepEqual(body.UsedTitle, []string{"回線の表示"}) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "a.png")); err != nil {
		t.Fatal("deleted without confirmation")
	}
	// A nested file is checked the same way, and is not deleted by a plain request.
	w = apiCall(b, nil, "DELETE", "/api/source-files/d/e/n.png", nil)
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 409 || !reflect.DeepEqual(body.UsedBy, []string{"nested"}) || !reflect.DeepEqual(body.UsedTitle, []string{"nested"}) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := apiCall(b, nil, "DELETE", "/api/source-files/d/e/n.png?confirm=true", nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "d", "e", "n.png")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if w := apiCall(b, nil, "DELETE", "/api/source-files/a.png?confirm=true", nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := getPublic(New(b, testPass), "/source/files/a.png"); w.Code != 404 {
		t.Fatal(w.Code)
	}
	// An asset nobody uses needs no confirmation; one that is not there is 404.
	if w := apiCall(b, nil, "DELETE", "/api/source-files/free.png", nil); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := apiCall(b, nil, "DELETE", "/api/source-files/free.png", nil); w.Code != 404 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	// Deleting leaves the sources saved: they now point at a missing asset.
	if w := apiCall(b, nil, "GET", "/api/sources", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "a.png") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestUnreadableSourcesAreSaidNotTakenAsUnused(t *testing.T) {
	b, dir := refBackend(t)
	root := filepath.Join(dir, "source-files")
	writeTestFile(t, root, "a.png", pngBytes(t))
	if err := os.WriteFile(b.sourceStore.Path, []byte("not: [valid"), 0600); err != nil {
		t.Fatal(err)
	}
	w := apiCall(b, nil, "GET", "/api/source-files?lang=en", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"code":"sources.unreadable"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, target := range []string{"/api/source-files/a.png?lang=en", "/api/source-files/a.png?confirm=true"} {
		w = apiCall(b, nil, "DELETE", target, nil)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "sources.unreadable") && !strings.Contains(w.Body.String(), "Cannot read") {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "a.png")); err != nil {
		t.Fatal("deleted while the uses are unknown")
	}
}

func TestAssetUsageIncludesASourceThatNeedsRepair(t *testing.T) {
	b, dir := refBackend(t)
	writeTestFile(t, filepath.Join(dir, "source-files"), "a.png", pngBytes(t))
	// cam9 does not exist, so this body needs repair; it still uses a.png.
	saveTestSources(t, b, testSource("old", "古い", "{cam9.kbps}<img src=\"/source/files/a.png\">"))
	var list struct {
		Files []struct {
			UsedBy []string `json:"used_by"`
		} `json:"files"`
	}
	if err := json.Unmarshal(apiCall(b, nil, "GET", "/api/source-files", nil).Body.Bytes(), &list); err != nil || len(list.Files) != 1 || !reflect.DeepEqual(list.Files[0].UsedBy, []string{"old"}) {
		t.Fatalf("%v %+v", err, list)
	}
}

func TestMissingAssetCanBeSavedAndPreviewWarns(t *testing.T) {
	b, dir := refBackend(t)
	writeTestFile(t, filepath.Join(dir, "source-files"), "here.png", pngBytes(t))
	src := testSource("pic", "", "<img src=\"/source/files/here.png\"><img src=\"/source/files/d/gone.png\">")
	w := apiCall(b, nil, "POST", "/api/sources", src)
	if w.Code != 201 {
		t.Fatalf("a body with a missing asset was not saved: %d %s", w.Code, w.Body.String())
	}
	preview := func(body string, lang string) map[string]any {
		t.Helper()
		src.Body = body
		w := apiCall(b, nil, "POST", "/api/sources/preview?lang="+lang, map[string]any{"source": src, "simulate": map[string]any{}})
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := preview(src.Body, "en")
	warnings, _ := out["warnings"].([]any)
	if len(warnings) != 1 {
		t.Fatalf("%v", out)
	}
	warn := warnings[0].(map[string]any)
	if warn["code"] != "template.asset_missing" || !strings.Contains(warn["message"].(string), "d/gone.png") || strings.Contains(warn["message"].(string), "here.png") || !reflect.DeepEqual(warn["assets"], []any{"d/gone.png"}) {
		t.Fatalf("%v", warn)
	}
	text := out["text"].(string)
	if !strings.Contains(text, "/source/files/here.png?v=") || strings.Contains(text, "gone.png?v=") {
		t.Fatal(text)
	}
	ja := preview(src.Body, "ja")["warnings"].([]any)[0].(map[string]any)["message"].(string)
	if !strings.Contains(ja, "素材が見つかりません") {
		t.Fatal(ja)
	}
	if out := preview("<img src=\"/source/files/here.png\">", "en"); len(out["warnings"].([]any)) != 0 {
		t.Fatalf("%v", out["warnings"])
	}
	// A body that does not compile has the usual template error and no warnings.
	src.Body = "{nope}"
	w = apiCall(b, nil, "POST", "/api/sources/preview", map[string]any{"source": src, "simulate": map[string]any{}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "template_error") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSourcesListReportsCameraReferences(t *testing.T) {
	b, _ := refBackend(t)
	saveTestSources(t, b,
		testSource("both", "二つ", "{cam1.kbps}{if cam2.online}x{end}"),
		testSource("styled", "", "<style>.a{width:{cam1.kbps}}</style><!-- {cam2.state} -->text"),
		testSource("attr", "", "<span style=\"width:{cam2.kbps}px\"></span>"),
	)
	w := apiCall(b, nil, "GET", "/api/sources", nil)
	var got struct {
		Sources []struct {
			Name    string   `json:"name"`
			Title   string   `json:"title"`
			Cameras []string `json:"cameras"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 {
		t.Fatal(err, w.Code, w.Body.String())
	}
	cams := map[string][]string{}
	for _, s := range got.Sources {
		cams[s.Name] = s.Cameras
	}
	want := map[string][]string{"both": {"cam1", "cam2"}, "styled": {}, "attr": {"cam2"}}
	if !reflect.DeepEqual(cams, want) {
		t.Fatalf("%v want %v", cams, want)
	}
	// The single-source GET is the saved source only (it is what the editor saves back).
	if w := apiCall(b, nil, "GET", "/api/sources/both", nil); strings.Contains(w.Body.String(), "cameras") {
		t.Fatal(w.Body.String())
	}
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

func drain(c *eventClient) []string {
	var out []string
	for len(c.queue) > 0 {
		if s, ok := (<-c.queue).value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func TestChangedAssetIsPushedEvenWithoutNow(t *testing.T) {
	b, dir := refBackend(t)
	root := filepath.Join(dir, "source-files")
	first := append(pngBytes(t), 0, 0, 0, 0)
	writeTestFile(t, root, "a.png", first)
	saveTestSources(t, b, testSource("pic", "", "<img src=\"/source/files/a.png\">"))
	if err := b.files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	c := &eventClient{topics: map[string]bool{"source:pic": true}, queue: make(chan eventMessage, 20)}
	e := &Events{backend: b, clients: map[*eventClient]bool{c: true}}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e.updateSources(t0)
	if got := drain(c); !reflect.DeepEqual(got, []string{"<img src=\"/source/files/a.png?v=" + hashOf(first) + "\">"}) {
		t.Fatalf("%q", got)
	}

	// The same name with other contents and another size: found by the stat.
	second := append(pngBytes(t), 1, 2, 3, 4, 5, 6, 7, 8)
	writeTestFile(t, root, "a.png", second)
	e.updateSources(t0.Add(time.Second))
	if got := drain(c); len(got) != 0 {
		t.Fatalf("looked before the interval: %q", got)
	}
	e.updateSources(t0.Add(2 * time.Second))
	if got := drain(c); !reflect.DeepEqual(got, []string{"<img src=\"/source/files/a.png?v=" + hashOf(second) + "\">"}) {
		t.Fatalf("%q", got)
	}

	// The same size and the same modification time cannot be seen by a stat ...
	info, err := os.Stat(filepath.Join(root, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	third := append(pngBytes(t), 8, 7, 6, 5, 4, 3, 2, 1)
	writeTestFile(t, root, "a.png", third)
	if err := os.Chtimes(filepath.Join(root, "a.png"), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	e.updateSources(t0.Add(4 * time.Second))
	if got := drain(c); len(got) != 0 {
		t.Fatalf("a stat cannot see this: %q", got)
	}
	// ... but opening the asset list rebuilds the index, and the pages that are open get it.
	e2 := e
	if w := apiCall(b, e2, "GET", "/api/source-files", nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := drain(c); !reflect.DeepEqual(got, []string{"<img src=\"/source/files/a.png?v=" + hashOf(third) + "\">"}) {
		t.Fatalf("%q", got)
	}

	// The asset goes away: the version goes, the body stays.
	if w := apiCall(b, e, "DELETE", "/api/source-files/a.png?confirm=true", nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := drain(c); !reflect.DeepEqual(got, []string{"<img src=\"/source/files/a.png\">"}) {
		t.Fatalf("%q", got)
	}
}

func TestAssetsAreNotLookedAtWhileNobodyWatches(t *testing.T) {
	b, dir := refBackend(t)
	root := filepath.Join(dir, "source-files")
	first := append(pngBytes(t), 0)
	writeTestFile(t, root, "a.png", first)
	saveTestSources(t, b, testSource("pic", "", "<img src=\"/source/files/a.png\">"))
	if err := b.files.Rebuild(); err != nil {
		t.Fatal(err)
	}
	e := &Events{backend: b, clients: map[*eventClient]bool{}}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e.updateSources(t0)
	writeTestFile(t, root, "a.png", append(pngBytes(t), 1, 1))
	e.updateSources(t0.Add(10 * time.Second))
	if got := b.files.Versions([]string{"/source/files/a.png"})["/source/files/a.png"]; got != hashOf(first) {
		t.Fatalf("looked with nobody connected: %s", got)
	}
	// A connected admin sources screen makes it look.
	c := &eventClient{topics: map[string]bool{"sources": true}, queue: make(chan eventMessage, 20)}
	e.clients[c] = true
	e.updateSources(t0.Add(11 * time.Second))
	if got := b.files.Versions([]string{"/source/files/a.png"})["/source/files/a.png"]; got == hashOf(first) {
		t.Fatal("not looked at with a screen connected")
	}
}
