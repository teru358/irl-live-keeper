package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/sources"
)

func TestSourceFilePathMatrix(t *testing.T) {
	for _, method := range []string{"GET", "DELETE"} {
		for _, name := range []string{"%FF.png", "../a.svg", "%2e%2e/a.svg", "親%5Ca.svg", "親%252Fa.svg", "親/%E5%A4%95.svg", "親%2F%E5%A4%95.svg", "不存在%2F夕.svg"} {
			t.Run(method+"/"+name, func(t *testing.T) {
				dir := t.TempDir()
				_ = os.Mkdir(filepath.Join(dir, "親"), 0700)
				data := []byte(`<svg/>`)
				if err := os.WriteFile(filepath.Join(dir, "親", "夕.svg"), data, 0600); err != nil {
					t.Fatal(err)
				}
				b := &fileBackend{files: sources.FilesStore{Dir: dir}}
				if err := b.files.Rebuild(); err != nil {
					t.Fatal(err)
				}
				prefix := "/source/files/"
				if method == "DELETE" {
					prefix = "/api/source-files/"
				}
				r := httptest.NewRequest(method, prefix+name, nil)
				r.RemoteAddr = "127.0.0.1:1234"
				r.SetBasicAuth("admin", testPass)
				r.Header.Set(headerName, "1")
				w := httptest.NewRecorder()
				New(b, testPass).ServeHTTP(w, r)
				valid := name == "親/%E5%A4%95.svg" || name == "親%2F%E5%A4%95.svg"
				if valid {
					if w.Code != 200 {
						t.Fatalf("%d %s", w.Code, w.Body.String())
					}
					if method == "GET" && !bytes.Equal(w.Body.Bytes(), data) {
						t.Fatal("wrong body")
					}
				} else {
					if w.Code < 400 || w.Code >= 500 {
						t.Fatalf("accepted or redirected: %d", w.Code)
					}
					if _, err := os.Stat(filepath.Join(dir, "親", "夕.svg")); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestSourceFileMultipartMatrix(t *testing.T) {
	part := func(disposition, body string) string {
		return "--b\r\nContent-Disposition: " + disposition + "\r\n\r\n" + body + "\r\n"
	}
	file := func(name, body string) string { return part(`form-data; name="file"; filename="`+name+`"`, body) }
	image := `<svg/>`
	good := file("夕.svg", image)
	end := "--b--\r\n"
	limit := sources.MaxFileSize + (1 << 20)
	sized := func(n int) string { return good + end + strings.Repeat("x", n-len(good+end)) }
	type row struct {
		name, body string
		status     int
		code       string
	}
	rows := []row{
		{"fullwidth slash", file("a／b.svg", image) + end, 400, "source_files.invalid"},
		{"fullwidth backslash", file("a＼b.svg", image) + end, 400, "source_files.invalid"},
		{"invalid UTF8", file("a\xff.svg", image) + end, 400, "source_files.invalid"},
		{"ordinary then file", part(`form-data; name="file"`, "value") + good + end, 201, ""},
		{"empty filename", part(`form-data; name="file"; filename=""`, image) + end, 400, "source_files.invalid"},
		{"absent", part(`form-data; name="other"; filename="a.svg"`, image) + end, 400, "source_files.invalid"},
		{"case sensitive", part(`form-data; name="File"; filename="a.svg"`, image) + end, 400, "source_files.invalid"},
		{"first wins", good + file("second.svg", "bad") + end, 201, ""},
		{"other ignored", part(`form-data; name="other"; filename="a.svg"`, "bad") + good + end, 201, ""},
		{"bad first", file("../bad.svg", image) + good + end, 400, "source_files.invalid"},
		{"truncated", good + "--b\r\nbroken", 400, "source_files.invalid"},
		{"bad tail", good + "--b\r\nBad header\r\n\r\nx\r\n" + end, 400, "source_files.invalid"},
		{"5MiB", file("夕.svg", image+strings.Repeat(" ", sources.MaxFileSize-len(image))) + end, 201, ""},
		{"5MiB+1", file("夕.svg", image+strings.Repeat(" ", sources.MaxFileSize+1-len(image))) + end, 413, "source_files.limit"},
		{"6MiB", sized(limit), 201, ""}, {"6MiB+1", sized(limit + 1), 413, "source_files.limit"},
		{"extra overflow", good + part(`form-data; name="extra"; filename="extra.svg"`, strings.Repeat("x", limit)) + end, 413, "source_files.limit"},
		{"type", file("夕.svg", "bad") + end, 400, "source_files.invalid"},
	}
	for _, n := range []int{1000, 1001} {
		rows = append(rows, row{fmt.Sprint("parts", n), good + strings.Repeat(part(`form-data; name="x"`, ""), n-1) + end, map[bool]int{true: 400, false: 201}[n > 1000], map[bool]string{true: "source_files.invalid", false: ""}[n > 1000]})
	}
	for _, n := range []int{10000, 10001} {
		extra := "--b\r\nContent-Disposition: form-data; name=\"other\"; filename=\"other.svg\"\r\n" + strings.Repeat("X-Test: v\r\n", n-2) + "\r\nx\r\n"
		rows = append(rows, row{fmt.Sprint("headers", n), good + extra + end, map[bool]int{true: 400, false: 201}[n > 10000], map[bool]string{true: "source_files.invalid", false: ""}[n > 10000]})
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			b := &fileBackend{files: sources.FilesStore{Dir: t.TempDir()}}
			r := httptest.NewRequest("POST", "/api/source-files", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "multipart/form-data; boundary=b")
			r.Header.Set(headerName, "1")
			r.SetBasicAuth("admin", testPass)
			r.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()
			New(b, testPass).ServeHTTP(w, r)
			if w.Code != tc.status || tc.code != "" && !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			entries, err := os.ReadDir(b.files.Dir)
			if err != nil {
				t.Fatal(err)
			}
			if tc.status == 201 {
				if len(entries) != 1 || entries[0].Name() != "夕.svg" {
					t.Fatalf("saved %v", entries)
				}
			} else if len(entries) != 0 {
				t.Fatal("saved failed upload")
			}
		})
	}
}

func TestSourceFileUploadResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		count  int
		status int
		code   string
	}{
		{"duplicate", 1, 409, "source_files.duplicate"},
		{"count limit", sources.MaxFilesCount, 413, "source_files.limit"},
		{"Japanese", 0, 201, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &fileBackend{files: sources.FilesStore{Dir: t.TempDir()}}
			for i := 0; i < tc.count; i++ {
				name := fmt.Sprintf("image%d.svg", i)
				if tc.count == 1 {
					name = "夕.svg"
				}
				if err := os.WriteFile(filepath.Join(b.files.Dir, name), []byte(`<svg/>`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			body := "--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"夕.svg\"\r\n\r\n<svg/>\r\n--b--\r\n"
			r := httptest.NewRequest("POST", "/api/source-files", strings.NewReader(body))
			r.Header.Set("Content-Type", "multipart/form-data; boundary=b")
			r.Header.Set(headerName, "1")
			r.SetBasicAuth("admin", testPass)
			r.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()
			h := New(b, testPass)
			h.ServeHTTP(w, r)
			var response struct {
				Code string
				Name string
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || response.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if tc.count == 0 {
				r = httptest.NewRequest("GET", "/api/source-files", nil)
				r.SetBasicAuth("admin", testPass)
				r.RemoteAddr = "127.0.0.1:1234"
				w = httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 200 || !strings.Contains(w.Body.String(), "夕.svg") {
					t.Fatalf("list %d %s", w.Code, w.Body.String())
				}
			}
		})
	}
}
