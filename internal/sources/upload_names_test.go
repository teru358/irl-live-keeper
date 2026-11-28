package sources

import (
	"mime"
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

func TestUploadExtendedFilenameUsesStandardParser(t *testing.T) {
	for _, header := range []string{
		`form-data; name="file"; filename="a.png"; filename="a.png"`,
		`form-data; name="file"; filename="a.png"; filename*=UTF-8''%E5%A4%95.png`,
		`form-data; name="file"; filename="a.png"; filename*=bad''%ZZ`,
		`form-data; name="file"; filename*0*=UTF-8''%E5%A4; filename*1*=%95.png`,
		`form-data; name="file"; filename="a.png"; filename="b.png"`,
	} {
		t.Run(header, func(t *testing.T) {
			_, params, parseErr := mime.ParseMediaType(header)
			want, nameErr := SafeFileName(params["filename"])
			r := httptest.NewRequest("POST", "/", strings.NewReader("--b\r\nContent-Disposition: "+header+"\r\n\r\nx\r\n--b--\r\n"))
			r.Header.Set("Content-Type", "multipart/form-data; boundary=b")
			got, _, err := ReadUpload(httptest.NewRecorder(), r)
			if parseErr != nil || nameErr != nil {
				if err == nil {
					t.Fatal("accepted invalid parsed filename")
				}
			} else if err != nil || got != want {
				t.Fatalf("got %q %v want %q", got, err, want)
			}
		})
	}
}
