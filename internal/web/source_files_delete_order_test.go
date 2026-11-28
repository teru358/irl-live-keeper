package web

import (
	"os"
	"path/filepath"
	"testing"
)

// S2-89: a name that is not in the index is 404 before the in-use check, even when a body refers to it.
func TestDeleteUnknownNameIs404BeforeInUse(t *testing.T) {
	b, dir := refBackend(t)
	root := filepath.Join(dir, "source-files")
	writeTestFile(t, root, "Sky.png", pngBytes(t))
	saveTestSources(t, b,
		testSource("overlay", "", "<img src=\"/source/files/sky.png\"><img src=\"/source/files/gone.png\"><img src=\"/source/files/Sky.png\">"),
	)
	for _, name := range []string{"sky.png", "SKY.PNG", "gone.png"} {
		for _, query := range []string{"", "?confirm=true"} {
			if w := apiCall(b, nil, "DELETE", "/api/source-files/"+name+query, nil); w.Code != 404 {
				t.Fatalf("%s%s: %d %s", name, query, w.Code, w.Body.String())
			}
		}
	}
	// In the index and in use: still 409, and the file stays.
	if w := apiCall(b, nil, "DELETE", "/api/source-files/Sky.png", nil); w.Code != 409 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "Sky.png")); err != nil {
		t.Fatal(err)
	}
}
