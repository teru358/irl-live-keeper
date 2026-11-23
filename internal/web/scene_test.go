package web

import (
	"context"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"net/http/httptest"
	"strings"
	"testing"
)

type sceneBackend struct {
	fakeBackend
	sceneErr error
}

func (b *sceneBackend) Scenes() ([]obs.Scene, error) {
	return []obs.Scene{{UUID: "u", Name: "待機"}}, b.sceneErr
}
func (b *sceneBackend) SelectScene(context.Context, string) error { return b.sceneErr }
func TestSceneAPI(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		header string
		want   int
	}{
		{"成功", nil, "1", 200}, {"未接続", obs.ErrAuxUnavailable, "1", 503}, {"未知", core.ErrSceneMissing, "1", 404}, {"失敗", errors.New("失敗"), "1", 502}, {"CSRF", nil, "", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &sceneBackend{sceneErr: tc.err}
			r := httptest.NewRequest("POST", "/api/obs/scene", strings.NewReader(`{"scene_uuid":"u"}`))
			r.Header.Set(headerName, tc.header)
			w := httptest.NewRecorder()
			apiHandler(b, nil).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestSceneAuthenticationAndList(t *testing.T) {
	b := &sceneBackend{}
	for _, path := range []string{"/api/obs/scene", "/api/obs/scenes"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		New(b, testPass).ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/api/obs/scenes", nil)
	r.SetBasicAuth("admin", testPass)
	w := httptest.NewRecorder()
	New(b, testPass).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"uuid":"u"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	page, _ := assets.ReadFile("assets/page.html")
	if !strings.Contains(string(page), `id="scenes"`) {
		t.Fatal("一覧がない")
	}
}
