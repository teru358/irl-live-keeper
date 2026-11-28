package sources

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadOriginalFilename(t *testing.T) {
	for _, tc := range []struct {
		header, want string
		bad          bool
	}{
		{`name="file"; filename="夕焼け.PNG"`, "夕焼け.png", false},
		{`name="file"; filename="a/b.png"`, "", true},
		{`name="file"; filename="a\\b.png"`, "", true},
		{`name="file"; filename="a／b.png"`, "", true},
		{`name="file"; filename=".夕.png"`, "_夕.png", false},
		{`name="file"; filename="が.png"`, "か_.png", false},
		{`name="file"; filename*=UTF-8''%E5%A4%95.png`, "夕.png", false},
	} {
		t.Run(tc.header, func(t *testing.T) {
			body := "--b\r\nContent-Disposition: form-data; " + tc.header + "\r\n\r\nx\r\n--b--\r\n"
			r := httptest.NewRequest("POST", "/", strings.NewReader(body))
			r.Header.Set("Content-Type", "multipart/form-data; boundary=b")
			name, _, err := ReadUpload(httptest.NewRecorder(), r)
			if (err != nil) != tc.bad || name != tc.want {
				t.Fatalf("name=%q err=%v", name, err)
			}
		})
	}
}
func TestUploadRejectsTruncatedTail(t *testing.T) {
	body := "--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\n\r\nx\r\n--b\r\nbroken"
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=b")
	if _, _, err := ReadUpload(httptest.NewRecorder(), r); err == nil {
		t.Fatal("accepted truncated tail")
	}
}
