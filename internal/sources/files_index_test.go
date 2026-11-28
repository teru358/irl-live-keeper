package sources

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// paddedPNG is a valid PNG followed by zero bytes, size bytes long.
func paddedPNG(t *testing.T, size int) []byte {
	t.Helper()
	b := samplePNG(t)
	if size < len(b) {
		t.Fatal("size too small")
	}
	return append(b, make([]byte, size-len(b))...)
}

func writeFile(t *testing.T, dir, rel string, data []byte) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func listMap(t *testing.T, s *FilesStore) map[string]FileInfo {
	t.Helper()
	l, err := s.List(nil)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]FileInfo{}
	for _, f := range l.Files {
		m[f.Name] = f
	}
	return m
}

func wantUsable(t *testing.T, m map[string]FileInfo, name string) {
	t.Helper()
	f, ok := m[name]
	if !ok || !f.Usable || f.Reason != "" || f.Type == "" {
		t.Fatalf("%q: %+v ok=%v", name, f, ok)
	}
}

func wantReason(t *testing.T, m map[string]FileInfo, name string, r Reason) {
	t.Helper()
	f, ok := m[name]
	if !ok || f.Usable || f.Reason != string(r) || f.Type != "" {
		t.Fatalf("%q: %+v ok=%v, want reason %s", name, f, ok, r)
	}
}

func TestCheckPath(t *testing.T) {
	seg := func(n int) string { return strings.Repeat("a", n-4) + ".png" }
	dirSeg := func(n int) string { return strings.Repeat("d", n) }
	for _, tc := range []struct {
		path string
		want Reason
	}{
		{"x.png", ""},
		{"a/x.png", ""},
		{"a/b/c/x.png", ""},
		{"a/b/c/d/x.png", ReasonTooDeep},
		{"my.logo.v2.png", ""},
		{"a.b/x.png", ""},
		{"a/b/c/" + seg(128), ""},
		{"a/b/c/" + seg(129), ReasonBadName},
		{dirSeg(128) + "/x.png", ""},
		{dirSeg(129) + "/x.png", ReasonBadName},
		{dirSeg(128) + "/" + dirSeg(128) + "/" + dirSeg(128) + "/" + strings.Repeat("e", 121) + ".png", ""},                // 512
		{dirSeg(128) + "/" + dirSeg(128) + "/" + dirSeg(128) + "/" + strings.Repeat("e", 122) + ".png", ReasonPathTooLong}, // 513
		{"x.", ReasonBadName},
		{"d./x.png", ReasonBadName},
		{".hidden.png", ReasonBadName},
		{"..png", ReasonBadName},
		{"a/../x.png", ReasonBadName},
		{"../x.png", ReasonBadName},
		{"/x.png", ReasonBadName},
		{"a//x.png", ReasonBadName},
		{`a\x.png`, ReasonBadName},
		{`C:\x.png`, ReasonBadName},
		{"x.png/", ReasonBadName},
		{"my logo.png", ReasonBadName},
		{"日本.png", ReasonBadName},
		{"CON.png", ReasonReservedName},
		{"con.png", ReasonReservedName},
		{"a/NUL.png", ReasonReservedName},
		{"AUX", ReasonReservedName},
		{"prn.tar.png", ReasonReservedName},
		{"COM1.png", ReasonReservedName},
		{"com9.png", ReasonReservedName},
		{"LPT9/x.png", ReasonReservedName},
		{"COM0.png", ""},
		{"COM10.png", ""},
		{"CONSOLE.png", ""},
		{"x.txt", ReasonBadExtension},
		{"x", ReasonBadExtension},
		{"x.PNG", ""},
		{"", ReasonBadName},
	} {
		if got := CheckPath(tc.path); got != tc.want {
			t.Errorf("CheckPath(%.40q len=%d) = %q, want %q", tc.path, len(tc.path), got, tc.want)
		}
	}
}

func TestScanClassifiesFiles(t *testing.T) {
	dir := t.TempDir()
	png := samplePNG(t)
	writeFile(t, dir, "root.png", png)
	writeFile(t, dir, "a/b/c/deep.png", png)
	writeFile(t, dir, "a/b/c/d/toodeep.png", png)
	writeFile(t, dir, "my.logo.v2.png", png)
	writeFile(t, dir, "bad name.png", png)
	writeFile(t, dir, "my dir/x.png", png)
	writeFile(t, dir, "CON.png", png)
	writeFile(t, dir, "notes.txt", []byte("hi"))
	writeFile(t, dir, "big.png", paddedPNG(t, MaxFileSize+1))
	writeFile(t, dir, "exact.png", paddedPNG(t, MaxFileSize))
	writeFile(t, dir, "broken.png", []byte("\x89PNG\r\n\x1a\nnot-an-image"))
	writeFile(t, dir, "empty.png", nil)
	writeFile(t, dir, "mismatch.jpg", png)
	s := &FilesStore{Dir: dir}
	m := listMap(t, s)
	for _, n := range []string{"root.png", "a/b/c/deep.png", "my.logo.v2.png", "exact.png"} {
		wantUsable(t, m, n)
	}
	wantReason(t, m, "a/b/c/d", ReasonTooDeep)
	if _, ok := m["a/b/c/d/toodeep.png"]; ok {
		t.Fatal("file below the third folder was listed")
	}
	wantReason(t, m, "bad name.png", ReasonBadName)
	wantReason(t, m, "my dir/x.png", ReasonBadName)
	wantReason(t, m, "CON.png", ReasonReservedName)
	wantReason(t, m, "notes.txt", ReasonBadExtension)
	wantReason(t, m, "big.png", ReasonTooLarge)
	wantReason(t, m, "broken.png", ReasonNotImage)
	wantReason(t, m, "empty.png", ReasonNotImage)
	wantReason(t, m, "mismatch.jpg", ReasonExtMismatch)
	if m["big.png"].Size != MaxFileSize+1 {
		t.Fatalf("size %d", m["big.png"].Size)
	}
	for n, f := range m {
		if !f.Usable {
			if _, _, err := s.Open(n); err == nil {
				t.Errorf("unusable file %q served", n)
			}
		}
	}
	data, typ, err := s.Open("a/b/c/deep.png")
	if err != nil || typ != "image/png" || !bytes.Equal(data, png) {
		t.Fatalf("%v %s", err, typ)
	}
}

func TestListIsSortedByPathBytes(t *testing.T) {
	dir := t.TempDir()
	png := samplePNG(t)
	for _, n := range []string{"b.png", "a/b.png", "a.png", "a-1.png", "A.png", "a_.png"} {
		writeFile(t, dir, n, png)
	}
	l, err := (&FilesStore{Dir: dir}).List(nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range l.Files {
		got = append(got, f.Name)
	}
	want := []string{"A.png", "a-1.png", "a.png", "a/b.png", "a_.png", "b.png"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSymlinksAreRejectedAndNotFollowed(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	png := samplePNG(t)
	writeFile(t, outside, "secret.png", png)
	writeFile(t, outside, "sub/inner.png", png)
	writeFile(t, dir, "real.png", png)
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(dir, "link.png")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "sub"), filepath.Join(dir, "linkdir")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "real.png"), filepath.Join(dir, "inside.png")); err != nil {
		t.Fatal(err)
	}
	s := &FilesStore{Dir: dir}
	m := listMap(t, s)
	wantUsable(t, m, "real.png")
	wantReason(t, m, "link.png", ReasonSymlink)
	wantReason(t, m, "linkdir", ReasonSymlink)
	wantReason(t, m, "inside.png", ReasonSymlink)
	if _, ok := m["linkdir/inner.png"]; ok {
		t.Fatal("followed a directory link")
	}
	for _, n := range []string{"link.png", "linkdir/inner.png", "inside.png"} {
		if _, _, err := s.Open(n); err == nil {
			t.Fatalf("served %q", n)
		}
	}
}

func TestOpenRejectsFileSwappedForSymlinkAfterIndexing(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	png := samplePNG(t)
	writeFile(t, dir, "a/x.png", png)
	writeFile(t, outside, "o.png", png)
	s := &FilesStore{Dir: dir}
	listMap(t, s)
	if err := os.Remove(filepath.Join(dir, "a", "x.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "o.png"), filepath.Join(dir, "a", "x.png")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if _, _, err := s.Open("a/x.png"); err == nil {
		t.Fatal("served a symlink")
	}
	// A directory swapped for a link is refused as well.
	writeFile(t, dir, "d/y.png", png)
	listMap(t, s)
	if err := os.RemoveAll(filepath.Join(dir, "d")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "d")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	writeFile(t, outside, "y.png", png)
	if _, _, err := s.Open("d/y.png"); err == nil {
		t.Fatal("served through a directory link")
	}
}

func TestCountLimitUsesUsableFilesInPathOrder(t *testing.T) {
	dir := t.TempDir()
	png := samplePNG(t)
	for i := 0; i < MaxFilesCount; i++ {
		writeFile(t, dir, fmt.Sprintf("f%04d.png", i), png)
	}
	// Unusable files are not counted, wherever they sort.
	writeFile(t, dir, "a-broken.png", []byte("nope"))
	writeFile(t, dir, "z-broken.png", []byte("nope"))
	s := &FilesStore{Dir: dir}
	m := listMap(t, s)
	usable := 0
	for _, f := range m {
		if f.Usable {
			usable++
		}
	}
	if usable != MaxFilesCount {
		t.Fatalf("usable %d", usable)
	}
	wantReason(t, m, "a-broken.png", ReasonNotImage)
	// One more usable file sorts last and is over the limit; a later small
	// one stays over the limit too.
	writeFile(t, dir, "g0.png", png)
	writeFile(t, dir, "g1.png", png)
	m = listMap(t, s)
	wantUsable(t, m, "f0999.png")
	wantReason(t, m, "g0.png", ReasonOverLimit)
	wantReason(t, m, "g1.png", ReasonOverLimit)
	if _, _, err := s.Open("g0.png"); err == nil {
		t.Fatal("served an over-limit file")
	}
	// A file that sorts first pushes the last usable one over the limit.
	writeFile(t, dir, "0first.png", png)
	m = listMap(t, s)
	wantUsable(t, m, "0first.png")
	wantReason(t, m, "f0999.png", ReasonOverLimit)
	wantUsable(t, m, "f0998.png")
}

func TestTotalSizeLimitUsesPathOrder(t *testing.T) {
	dir := t.TempDir()
	big := paddedPNG(t, MaxFileSize)
	n := MaxFilesSize / MaxFileSize
	for i := 0; i < n; i++ {
		writeFile(t, dir, fmt.Sprintf("m%02d.png", i), big)
	}
	small := samplePNG(t)
	writeFile(t, dir, "n.png", small)
	// Too large files and broken files do not count.
	writeFile(t, dir, "a.png", paddedPNG(t, MaxFileSize+1))
	s := &FilesStore{Dir: dir}
	m := listMap(t, s)
	wantUsable(t, m, fmt.Sprintf("m%02d.png", n-1))
	wantReason(t, m, "a.png", ReasonTooLarge)
	wantReason(t, m, "n.png", ReasonOverLimit)
}

func TestScanStopsAtEntryLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < MaxScanEntries; i++ {
		writeFile(t, dir, fmt.Sprintf("n%05d.txt", i), nil)
	}
	s := &FilesStore{Dir: dir}
	l, err := s.List(nil)
	if err != nil || l.ScanLimited || len(l.Files) != MaxScanEntries {
		t.Fatalf("%v limited=%v n=%d", err, l.ScanLimited, len(l.Files))
	}
	writeFile(t, dir, "zzz.png", samplePNG(t))
	l, err = s.List(nil)
	if err != nil || !l.ScanLimited || len(l.Files) != MaxScanEntries {
		t.Fatalf("%v limited=%v n=%d", err, l.ScanLimited, len(l.Files))
	}
}

func TestScanEntryLimitCountsFoldersAcrossLevels(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 6000; i++ {
		writeFile(t, dir, fmt.Sprintf("a/n%05d.txt", i), nil)
	}
	for i := 0; i < 6000; i++ {
		writeFile(t, dir, fmt.Sprintf("b/n%05d.txt", i), nil)
	}
	l, err := (&FilesStore{Dir: dir}).List(nil)
	if err != nil || !l.ScanLimited || len(l.Files) != MaxScanEntries-2 {
		t.Fatalf("%v limited=%v n=%d", err, l.ScanLimited, len(l.Files))
	}
}

func TestIndexIsRebuiltOnListSaveAndDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "source-files")
	png := samplePNG(t)
	s := &FilesStore{Dir: dir}
	if _, _, err := s.Open("x.png"); err == nil {
		t.Fatal("opened from a missing folder")
	}
	writeFile(t, dir, "x.png", png)
	// The index was built when the folder did not exist, so it does not know x.png.
	if _, _, err := s.Open("x.png"); err == nil {
		t.Fatal("served a file the index does not know")
	}
	listMap(t, s)
	if _, _, err := s.Open("x.png"); err != nil {
		t.Fatalf("after list: %v", err)
	}
	if err := s.Save("y.png", png); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open("y.png"); err != nil {
		t.Fatalf("after save: %v", err)
	}
	if err := s.Delete("y.png"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open("y.png"); err == nil {
		t.Fatal("served a deleted file")
	}
	if err := s.Delete("y.png"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second delete: %v", err)
	}
	// Startup builds the index once.
	s2 := &FilesStore{Dir: dir}
	if err := s2.EnsureIndex(); err != nil {
		t.Fatal(err)
	}
	if err := s2.EnsureIndex(); err != nil || s2.ScanCount() != 1 {
		t.Fatalf("scans %d %v", s2.ScanCount(), err)
	}
}

func TestOpenDoesNotScanTheFolder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a/x.png", samplePNG(t))
	s := &FilesStore{Dir: dir}
	if err := s.EnsureIndex(); err != nil {
		t.Fatal(err)
	}
	before := s.ScanCount()
	for i := 0; i < 20; i++ {
		if _, _, err := s.Open("a/x.png"); err != nil {
			t.Fatal(err)
		}
		_, _, _ = s.Open("missing.png")
		_, _, _ = s.Open("a")
		_, _, _ = s.Open("../x.png")
	}
	if got := s.ScanCount(); got != before {
		t.Fatalf("scans %d -> %d", before, got)
	}
}

func TestOpenRevalidatesAReplacedFile(t *testing.T) {
	dir := t.TempDir()
	png := samplePNG(t)
	writeFile(t, dir, "x.png", png)
	s := &FilesStore{Dir: dir}
	listMap(t, s)
	scans := s.ScanCount()
	// A different size.
	bigger := paddedPNG(t, len(png)+100)
	writeFile(t, dir, "x.png", bigger)
	data, typ, err := s.Open("x.png")
	if err != nil || typ != "image/png" || !bytes.Equal(data, bigger) {
		t.Fatalf("%v %s %d", err, typ, len(data))
	}
	// A new modification time only.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "x.png"), later, later); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open("x.png"); err != nil {
		t.Fatal(err)
	}
	// The file becomes something else with the same name.
	writeFile(t, dir, "x.png", []byte("not an image any more"))
	if _, _, err := s.Open("x.png"); err == nil {
		t.Fatal("served broken contents")
	}
	if s.ScanCount() != scans {
		t.Fatal("a whole scan was made")
	}
	m := listMap(t, s)
	wantReason(t, m, "x.png", ReasonNotImage)
	// And back again.
	writeFile(t, dir, "x.png", png)
	listMap(t, s)
	if _, _, err := s.Open("x.png"); err != nil {
		t.Fatal(err)
	}
	// A file grown past the limit after indexing is read no further than
	// the limit and refused.
	writeFile(t, dir, "x.png", paddedPNG(t, MaxFileSize+1000))
	if _, _, err := s.Open("x.png"); err == nil {
		t.Fatal("served an oversize file")
	}
	// A file replaced by a directory is refused.
	if err := os.Remove(filepath.Join(dir, "x.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "x.png"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open("x.png"); err == nil {
		t.Fatal("served a directory")
	}
}

func TestOpenRequiresTheExactIndexName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Photo.png", samplePNG(t))
	writeFile(t, dir, "a/b.png", samplePNG(t))
	s := &FilesStore{Dir: dir}
	listMap(t, s)
	for _, n := range []string{"photo.png", "PHOTO.PNG", "A/b.png", "a/B.png", "a//b.png", "./a/b.png", "a/../a/b.png", "/a/b.png", `a\b.png`, "a/b.png/", ""} {
		if _, _, err := s.Open(n); err == nil {
			t.Errorf("served %q", n)
		}
	}
	for _, n := range []string{"Photo.png", "a/b.png"} {
		if _, _, err := s.Open(n); err != nil {
			t.Errorf("%q: %v", n, err)
		}
	}
}

func TestSaveWritesOnlyDirectlyAndChecksLimits(t *testing.T) {
	dir := t.TempDir()
	png := samplePNG(t)
	s := &FilesStore{Dir: dir}
	for _, n := range []string{"a/x.png", "../x.png", "CON.png", "nul.png", `a\x.png`, "x.txt", "x.", ""} {
		if err := s.Save(n, png); err == nil {
			t.Errorf("saved %q", n)
		}
	}
	if err := s.Save(strings.Repeat("a", 125)+".png", png); err == nil {
		t.Error("saved a 129-character name")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("%d entries", len(entries))
	}
	for i := 0; i < MaxFilesCount; i++ {
		writeFile(t, dir, fmt.Sprintf("f%04d.png", i), png)
	}
	if err := s.Save("one-more.png", png); err != ErrTooManyFiles {
		t.Fatalf("got %v", err)
	}
	// Unusable files do not count toward the limits.
	if err := os.Remove(filepath.Join(dir, "f0000.png")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "broken.png", []byte("x"))
	if err := s.Save("one-more.png", png); err != nil {
		t.Fatal(err)
	}
}

func TestSaveRefusesToFollowALinkAtTheName(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "new.png"), filepath.Join(dir, "x.png")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	s := &FilesStore{Dir: dir}
	if err := s.Save("x.png", samplePNG(t)); err == nil {
		t.Fatal("saved through a link")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.png")); err == nil {
		t.Fatal("wrote outside the folder")
	}
}

func TestDeleteKeepsWorkingAtTheRootOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "x.png", samplePNG(t))
	writeFile(t, dir, "d/y.png", samplePNG(t))
	s := &FilesStore{Dir: dir}
	for _, n := range []string{"d/y.png", "d", "../x.png", `a\x.png`, ""} {
		if err := s.Delete(n); err == nil {
			t.Errorf("deleted %q", n)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "d", "y.png")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("x.png"); err != nil {
		t.Fatal(err)
	}
}
