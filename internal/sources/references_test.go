package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
)

func refDefs() map[string]template.Definition {
	return Definitions(core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1"}, {ID: "cam2"}, {ID: "cam3"}}})
}

func TestReferencesDecodeCharactersAndMatchExactPaths(t *testing.T) {
	body := `<img src="&#47;source&#x2f;files/a.png"><img src="/source/files/a.png2.png"><img src="/source/files/xa.png"><img src="/source/files/d/b.png">`
	got := ReferencesOf(body, refDefs()).Assets
	want := []string{"/source/files/a.png", "/source/files/a.png2.png", "/source/files/d/b.png", "/source/files/xa.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
	// The exact path is what a usage check compares: a.png is not a.png2.png or xa.png.
	set := map[string]bool{}
	for _, p := range got {
		set[p] = true
	}
	if !set["/source/files/a.png"] || set["/source/files/a"] || set["/source/files/a.png2"] {
		t.Fatal(set)
	}
}

func TestReferencesReadEveryBranchAndStyle(t *testing.T) {
	body := "{if cam1.online}<img src=\"/source/files/yes.png\">{elif cam2.online}<img src=\"/source/files/maybe.png\">{else}<img src=\"/source/files/no.png\">{end}" +
		"<style>.a{background:url(\"/source/files/block.png\")}.b{background:url(/source/files/plain.png)}</style>" +
		"<span style=\"background:url(&quot;/source/files/attr.png&quot;)\"></span>" +
		"<!-- <img src=\"/source/files/comment.png\"> -->"
	got := ReferencesOf(body, refDefs()).Assets
	want := []string{"/source/files/attr.png", "/source/files/block.png", "/source/files/maybe.png", "/source/files/no.png", "/source/files/plain.png", "/source/files/yes.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
}

func TestReferencesListCameraIDsWithoutStyleText(t *testing.T) {
	body := "{cam1.kbps}{if cam2.online and obs.streaming}x{end}" +
		"<style>.a{width:{cam3.kbps}}</style><!-- {cam3.state} -->" +
		"<span style=\"width:{cam1.kbps}px\"></span>"
	got := ReferencesOf(body, refDefs()).Cameras
	if want := []string{"cam1", "cam2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
	if got := ReferencesOf("{if cam3.state == \"OK\"}x{else}{cam1.srt.rtt}{end}", refDefs()).Cameras; !reflect.DeepEqual(got, []string{"cam1", "cam3"}) {
		t.Fatalf("%v", got)
	}
	if got := ReferencesOf("plain", refDefs()); len(got.Cameras) != 0 || len(got.Assets) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestReferencesOfABodyThatNoLongerCompiles(t *testing.T) {
	// cam9 was removed, so the body is broken; what it uses is still reported.
	body := `{cam9.kbps}<img src="/source/files/kept.png"><span style="width:{cam9.kbps}px"></span>{if cam1.online}<img src="/source/files/other.png">{end}`
	if _, err := CompileHTML(body, refDefs()); err == nil {
		t.Fatal("the body compiles")
	}
	got := ReferencesOf(body, refDefs())
	if !reflect.DeepEqual(got.Assets, []string{"/source/files/kept.png", "/source/files/other.png"}) || !reflect.DeepEqual(got.Cameras, []string{"cam1", "cam9"}) {
		t.Fatalf("%+v", got)
	}
}

func TestMissingAssetsAreWarnings(t *testing.T) {
	p, err := CompileHTML(`<img src="/source/files/a.png"><img src="/source/files/gone.png">`, refDefs())
	if err != nil {
		t.Fatal(err)
	}
	available := map[string]bool{"/source/files/a.png": true}
	if got := p.MissingAssets(available); !reflect.DeepEqual(got, []string{"/source/files/gone.png"}) {
		t.Fatal(got)
	}
	if got := p.Warnings(available); !reflect.DeepEqual(got, []string{"template.asset_missing"}) {
		t.Fatal(got)
	}
	if len(p.MissingAssets(nil)) != 0 {
		t.Fatal("no index must not report")
	}
}

func versionOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

func TestRenderAddsVersionsThatTheRenderedCheckAccepts(t *testing.T) {
	body := `<img src="/source/files/a.png">` +
		`<img src="&#47;source&#47;files&#47;d/b.png">` +
		`<img src="/source/files/missing.png">` +
		`<style>.x{background:url("/source/files/a.png")}.y{background:url( /source/files/d/b.png )}.z{background:url('/source/files/missing.png')}</style>` +
		`<span style="background:url(&quot;/source/files/a.png&quot;)"></span>` +
		`<span style="background:url(/source/files/d/b.png)">{cam1.kbps}</span>`
	versions := func(paths []string) map[string]string {
		out := map[string]string{}
		for _, p := range paths {
			switch p {
			case "/source/files/a.png":
				out[p] = "0123456789ab"
			case "/source/files/d/b.png":
				out[p] = "ba9876543210"
			}
		}
		return out
	}
	src := Source{Body: body}
	out, err := RenderSimulation(src, core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1"}}}, time.Now(), nil, Simulation{}, versions)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<img src="/source/files/a.png?v=0123456789ab">`,
		`<img src="&#47;source&#47;files&#47;d/b.png?v=ba9876543210">`,
		`<img src="/source/files/missing.png">`,
		`url("/source/files/a.png?v=0123456789ab")`,
		`url( /source/files/d/b.png?v=ba9876543210 )`,
		`url('/source/files/missing.png')`,
		`url(&quot;/source/files/a.png?v=0123456789ab&quot;)`,
		`url(/source/files/d/b.png?v=ba9876543210)`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q missing in %s", want, out)
		}
	}
	if strings.Count(out, "?v=") != 6 {
		t.Fatalf("%d versions: %s", strings.Count(out, "?v="), out)
	}
	// The generated form passes the rendered check, and only that form.
	if err := ValidateHTML(out, "rendered"); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileHTML(strings.Replace(body, `src="/source/files/a.png"`, `src="/source/files/a.png?v=0123456789ab"`, 1), refDefs()); err == nil {
		t.Fatal("a saved body must not carry a query")
	}
	// Without a lookup nothing is added.
	plain, err := RenderSimulation(src, core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1"}}}, time.Now(), nil, Simulation{}, nil)
	if err != nil || strings.Contains(plain, "?v=") {
		t.Fatal(err, plain)
	}
}

func TestFilesVersionsFollowContentAndRefreshSeesStatChanges(t *testing.T) {
	dir := t.TempDir()
	first := samplePNG(t)
	writeFile(t, dir, "a.png", first)
	writeFile(t, dir, "d/b.png", first)
	writeFile(t, dir, "bad.png", []byte("not an image"))
	s := &FilesStore{Dir: dir}
	paths := []string{"/source/files/a.png", "/source/files/d/b.png", "/source/files/bad.png", "/source/files/none.png"}
	if s.Versions(paths) != nil {
		t.Fatal("versions before the index exists")
	}
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	v := s.Versions(paths)
	if len(v) != 2 || v["/source/files/a.png"] != versionOf(first) || v["/source/files/d/b.png"] != versionOf(first) {
		t.Fatal(v)
	}
	if s.Refresh(paths) {
		t.Fatal("nothing changed")
	}
	// Same name, different size: the stat sees it.
	second := paddedPNG(t, len(first)+50)
	writeFile(t, dir, "a.png", second)
	if !s.Refresh(paths) {
		t.Fatal("a replaced file was not seen")
	}
	if got := s.Versions(paths)["/source/files/a.png"]; got != versionOf(second) {
		t.Fatal(got)
	}
	// The same size and the same modification time cannot be seen by a stat.
	info, err := os.Stat(filepath.Join(dir, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	other := append([]byte(nil), second...)
	other[len(other)-1] = 1
	writeFile(t, dir, "a.png", other)
	if err := os.Chtimes(filepath.Join(dir, "a.png"), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if s.Refresh(paths) || s.Versions(paths)["/source/files/a.png"] != versionOf(second) {
		t.Fatal("a stat cannot know this")
	}
	// A rebuild reads the contents again and finds it.
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if got := s.Versions(paths)["/source/files/a.png"]; got != versionOf(other) {
		t.Fatal(got)
	}
	// A file that appears, and one that goes away.
	writeFile(t, dir, "none.png", first)
	if !s.Refresh(paths) || s.Versions(paths)["/source/files/none.png"] != versionOf(first) {
		t.Fatal("a new file was not seen")
	}
	if err := os.Remove(filepath.Join(dir, "d", "b.png")); err != nil {
		t.Fatal(err)
	}
	if !s.Refresh(paths) {
		t.Fatal("a removed file was not seen")
	}
	if _, ok := s.Versions(paths)["/source/files/d/b.png"]; ok {
		t.Fatal("a removed file keeps a version")
	}
	if av := s.Available(); !av["/source/files/a.png"] || av["/source/files/d/b.png"] || av["/source/files/bad.png"] {
		t.Fatal(av)
	}
}

func TestDeleteRemovesNestedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "x.png", samplePNG(t))
	writeFile(t, dir, "d/e/y.png", samplePNG(t))
	s := &FilesStore{Dir: dir}
	for _, n := range []string{"d", "d/e", "../x.png", "d/../x.png", `a\x.png`, "", "/x.png", "d//e/y.png", "a/b/c/d/e.png"} {
		if err := s.Delete(n); err == nil {
			t.Errorf("deleted %q", n)
		}
	}
	if err := s.Delete("d/e/y.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "d", "e", "y.png")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "d", "e")); err != nil {
		t.Fatal("the folder must stay", err)
	}
	if err := s.Delete("x.png"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("x.png"); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRefreshRunsAlongsideListAndSave(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.png", samplePNG(t))
	s := &FilesStore{Dir: dir}
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	paths := []string{"/source/files/a.png", "/source/files/b.png"}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			// A different size each time, so that Refresh replaces the row.
			_ = os.WriteFile(filepath.Join(dir, "a.png"), paddedPNG(t, len(samplePNG(t))+i), 0600)
			s.Refresh(paths)
			s.Versions(paths)
		}
	}()
	for i := 0; i < 20; i++ {
		if _, err := s.List(nil); err != nil {
			t.Fatal(err)
		}
		_ = s.Save("b.png", samplePNG(t))
		_ = s.Delete("b.png")
	}
	<-done
}
