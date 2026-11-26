package sources

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func samplePNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.White)
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestSafeFileName(t *testing.T) {
	for in, want := range map[string]string{"logo.png": "logo.png", "my logo.PNG": "my_logo.png", "x!!.jpg": "x__.jpg"} {
		got, err := SafeFileName(in)
		if err != nil || got != want {
			t.Fatalf("%q => %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"../x.png", "x/../y.png", `..\\evil.png`, "x.exe", ""} {
		if _, err := SafeFileName(in); err == nil {
			t.Fatalf("accepted %q", in)
		}
	}
}
func TestDetectImageRejectsMismatchSVGAndCorrupt(t *testing.T) {
	png := samplePNG(t)
	if typ, err := DetectImage("x.png", png); err != nil || typ != "image/png" {
		t.Fatalf("%s %v", typ, err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{{"x.jpg", png}, {"x.svg", []byte("<html><svg></svg></html>")}, {"x.svg", []byte("<?xml version='1.0'?><root/>")}, {"x.png", []byte("\x89PNG\r\n\x1a\nnot-an-image")}, {"x.avif", []byte("BMnot-an-avif")}, {"x.bmp", []byte("<svg></svg>")}} {
		if _, err := DetectImage(tc.name, tc.data); err == nil {
			t.Fatalf("accepted %s", tc.name)
		}
	}
}

func TestDetectImageAcceptsAVIFBMPAndSVG(t *testing.T) {
	avif := []byte{0, 0, 0, 16, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f', 0, 0, 0, 0}
	bmp := make([]byte, 26)
	copy(bmp, "BM")
	bmp[2] = 26
	bmp[10] = 26
	bmp[14] = 12 // BITMAPCOREHEADER
	bmp[18], bmp[20] = 1, 1
	svg := []byte(" \n<?xml version=\"1.0\"?>\n<!-- safe -->\n<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"mark.avif", avif, "image/avif"}, {"mark.bmp", bmp, "image/bmp"}, {"mark.svg", svg, "image/svg+xml"},
	} {
		got, err := DetectImage(tc.name, tc.data)
		if err != nil || got != tc.want {
			t.Errorf("%s: type=%q err=%v", tc.name, got, err)
		}
	}
	for _, name := range []string{"art.avif", "art.bmp", "art.svg"} {
		if _, err := SafeFileName(name); err != nil {
			t.Errorf("SafeFileName(%q): %v", name, err)
		}
	}
}
func TestFilesStoreLimitsAndDuplicate(t *testing.T) {
	s := FilesStore{Dir: t.TempDir()}
	png := samplePNG(t)
	if err := s.Save("one.png", bytes.Repeat([]byte{'x'}, MaxFileSize+1)); err == nil {
		t.Fatal("accepted oversized file")
	}
	if err := s.Save("one.png", png); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("one.png", png); err == nil || !strings.Contains(err.Error(), "exist") {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestFilesStoreTotalLimit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.png"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(filepath.Join(dir, "existing.png"), MaxFilesSize); err != nil {
		t.Fatal(err)
	}
	s := FilesStore{Dir: dir}
	if err := s.Save("new.png", samplePNG(t)); err != ErrTotalTooLarge {
		t.Fatalf("got %v", err)
	}
}
