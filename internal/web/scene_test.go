package web

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"net/http/httptest"
	"reflect"
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
		{"成功", nil, "1", 200}, {"未接続", obs.ErrAuxUnavailable, "1", 503}, {"未知", core.ErrSceneMissing, "1", 404}, {"失敗", obs.ErrAuxRejected, "1", 502}, {"不確定", errors.New("切断"), "1", 503}, {"期限", obs.ErrAuxTimeout, "1", 503}, {"ID", obs.ErrAuxMismatch, "1", 503}, {"未送信", obs.ErrAuxNotSent, "1", 503}, {"CSRF", nil, "", 403},
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

type classifiedSceneBackend struct {
	sceneBackend
	list []obs.Scene
}

func (b *classifiedSceneBackend) Scenes() ([]obs.Scene, error) { return b.list, nil }

func TestSceneListClassificationResponse(t *testing.T) {
	for _, tc := range []struct {
		name, kind       string
		cameras, parents []string
	}{
		{"共通の待機", "standby", []string{}, []string{"Camera"}},
		{"個別のカメラあり", "camera", []string{"main"}, []string{"Camera"}},
		{"個別のカメラなし", "standby", []string{}, []string{"Camera"}},
		{"ただのカメラなし", "none", []string{}, []string{}},
		{"個別の設定なし", "camera", []string{"main"}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &classifiedSceneBackend{list: []obs.Scene{{UUID: "u", Name: tc.name, Kind: tc.kind, Cameras: tc.cameras, FallbackOf: tc.parents}}}
			w := httptest.NewRecorder()
			apiHandler(b, nil).ServeHTTP(w, httptest.NewRequest("GET", "/api/obs/scenes", nil))
			var got []obs.Scene
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 || !reflect.DeepEqual(got, b.list) {
				t.Fatal(w.Code, w.Body.String(), err)
			}
		})
	}
}

type inputListBackend struct {
	fakeBackend
	list []obs.Input
	err  error
}

func (b *inputListBackend) Inputs() ([]obs.Input, error) { return b.list, b.err }

func TestInputListAPI(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    *inputListBackend
		want int
	}{
		{"成功", &inputListBackend{list: []obs.Input{{UUID: "in-1", Name: "カメラ", Kind: "ffmpeg_source"}}}, 200},
		{"未接続", &inputListBackend{err: obs.ErrAuxUnavailable}, 503},
		{"期限", &inputListBackend{err: obs.ErrAuxTimeout}, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			apiHandler(tc.b, nil).ServeHTTP(w, httptest.NewRequest("GET", "/api/obs/inputs", nil))
			if w.Code != tc.want {
				t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
			}
			if tc.want == 200 && w.Body.String() != `[{"uuid":"in-1","name":"カメラ","kind":"ffmpeg_source"}]`+"\n" {
				t.Fatalf("body = %s", w.Body.String())
			}
		})
	}
}
