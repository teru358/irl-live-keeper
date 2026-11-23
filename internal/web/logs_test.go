package web

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/logging"
)

func TestLogsAPI(t *testing.T) {
	logger, err := logging.Open(logging.Options{Dir: t.TempDir(), KeepDays: 1, Secrets: []string{"hidden-secret"}, Stdout: &strings.Builder{}})
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	for i := 0; i < 2005; i++ {
		logger.Slog().Info("row", "n", i)
	}
	logger.Slog().Info("ordinary", "password", "hidden-secret")
	logger.Slog().Warn("warning")
	logger.Slog().Error("failure")
	events := NewEvents(&fakeBackend{}, logger, nil)
	defer events.Close()
	h := NewWithEvents(&fakeBackend{}, testPass, 8787, NewAuth(nil), events)
	call := func(path string, auth bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		if auth {
			r.SetBasicAuth("admin", testPass)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if got := call("/api/logs", false).Code; got != 401 {
		t.Fatalf("認証なし: %d", got)
	}
	cases := []struct {
		q     string
		want  int
		level string
	}{
		{"", 500, ""}, {"?lines=2001", 2000, ""}, {"?lines=5000&level=INFO", 2000, ""},
		{"?lines=0", 400, ""}, {"?lines=nope", 400, ""}, {"?level=DEBUG", 400, ""},
		{"?lines=10&level=INFO", 10, ""}, {"?lines=10&level=WARN", 2, "WARN"}, {"?lines=10&level=ERROR", 1, "ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.q, func(t *testing.T) {
			w := call("/api/logs"+tc.q, true)
			if w.Code != 200 && tc.want != 400 {
				t.Fatalf("status=%d", w.Code)
			}
			if tc.want == 400 {
				if w.Code != 400 {
					t.Fatalf("status=%d", w.Code)
				}
				return
			}
			var lines []map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &lines); err != nil {
				t.Fatal(err)
			}
			if len(lines) != tc.want {
				t.Fatalf("行数=%d want=%d", len(lines), tc.want)
			}
			for _, ln := range lines {
				if tc.level == "ERROR" && ln["level"] != "ERROR" {
					t.Fatalf("level=%v", ln["level"])
				}
				if tc.level == "WARN" && ln["level"] != "WARN" && ln["level"] != "ERROR" {
					t.Fatalf("level=%v", ln["level"])
				}
				if strings.Contains(fmt.Sprint(ln["text"]), "hidden-secret") {
					t.Fatal("秘密が漏れた")
				}
			}
		})
	}
	// 応答は新しい行だけを最大2000件保持する。
	w := call("/api/logs?lines=5000", true)
	var newest []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &newest); err != nil {
		t.Fatal(err)
	}
	if len(newest) != 2000 {
		t.Fatalf("上限=%d", len(newest))
	}
	if _, ok := newest[0]["t"]; !ok {
		t.Fatal("時刻がない")
	}
}
