package sources

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

// 5 MiB を少し超える取り込みは、サイズ超過（ErrFileTooLarge）として返る。
func TestUploadSlightlyOverLimitIsFileTooLarge(t *testing.T) {
	body := "--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\n\r\n" + strings.Repeat("x", MaxFileSize+1) + "\r\n--b--\r\n"
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=b")
	if _, _, err := ReadUpload(httptest.NewRecorder(), r); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v", err)
	}
}
