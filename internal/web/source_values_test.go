package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/obs"
)

type listedValuesResponse struct {
	Groups []struct {
		Kind   string                     `json:"kind"`
		Camera *struct{ ID, Name string } `json:"camera"`
		Values []struct {
			Name, Type, Format string
			Text               *string
		} `json:"values"`
	} `json:"groups"`
}

func TestSourceValuesAPI(t *testing.T) {
	b := &sourcesBackend{}
	b.snap.Cameras = []core.CameraStatus{{ID: "cam1", Name: "正面", State: "LOW", Kbps: 600.4}, {ID: "cam2", Name: "後方"}}
	b.snap.OBS = core.OBSStatus{Connected: true, SceneCameras: []string{"cam1"}}
	b.snap.SceneDetails = map[string]obs.Scene{"live": {Name: "Live", Kind: "camera", Cameras: []string{"cam1"}}}
	h := apiHandler(b, nil)
	for _, tc := range []struct{ method, body, kbps, elapsed string }{
		{"GET", "", "600", ""},
		{"POST", `{"simulate":{"line":"OK","stream":"streaming"}}`, "4200", "1:23:45"},
		{"POST", `{"simulate":{"line":"LOW","stream":"streaming"}}`, "600", "1:23:45"},
		{"POST", `{"simulate":{"line":"OK"}}`, "4200", ""},
		{"POST", `{"simulate":{"values":{"cam1.kbps":1234.6}}}`, "1235", ""},
		{"POST", `{}`, "600", ""},
	} {
		r := httptest.NewRequest(tc.method, "/api/source-values", strings.NewReader(tc.body))
		r.Header.Set(headerName, "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Error("values must not be cached")
		}
		var data listedValuesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		kinds := []string{}
		texts := map[string]*string{}
		for _, g := range data.Groups {
			kinds = append(kinds, g.Kind)
			if g.Kind != "camera" && g.Camera != nil {
				t.Error("unexpected camera metadata")
			}
			for _, v := range g.Values {
				texts[v.Name] = v.Text
			}
		}
		if !reflect.DeepEqual(kinds, []string{"camera", "camera", "standby", "obs", "stream", "other"}) {
			t.Fatal(kinds)
		}
		for i, c := range b.snap.Cameras {
			g := data.Groups[i]
			if g.Camera == nil || g.Camera.ID != c.ID || g.Camera.Name != c.Name {
				t.Errorf("camera heading %d=%+v", i, g.Camera)
			}
		}
		if texts["cam1.kbps"] == nil || *texts["cam1.kbps"] != tc.kbps {
			t.Errorf("kbps in %s: %v", tc.body, texts["cam1.kbps"])
		}
		if tc.elapsed == "" {
			if texts["stream.elapsed"] != nil {
				t.Error("elapsed should be null")
			}
		} else if texts["stream.elapsed"] == nil || *texts["stream.elapsed"] != tc.elapsed {
			t.Error("elapsed format")
		}
		if texts["standby.return"] != nil {
			t.Error("undefined return should be null")
		}
		// The wire format must keep empty format and null text, and omit camera except on camera groups.
		var raw map[string][]map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		for _, g := range raw["groups"] {
			var kind string
			_ = json.Unmarshal(g["kind"], &kind)
			if _, ok := g["camera"]; ok != (kind == "camera") {
				t.Errorf("camera field on %s", kind)
			}
			var rows []map[string]json.RawMessage
			_ = json.Unmarshal(g["values"], &rows)
			for _, row := range rows {
				if len(row) != 4 || row["name"] == nil || row["type"] == nil || row["format"] == nil || row["text"] == nil {
					t.Errorf("row=%v", row)
				}
			}
		}
	}

}

func TestSourceValuesErrorsMatchPreview(t *testing.T) {
	h := apiHandler(&sourcesBackend{}, nil)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{`, 400}, {`{} {}`, 400}, {`{"extra":true}`, 400},
		{`{"simulate":{"extra":true}}`, 400}, {`{"simulate":"OK"}`, 400},
		{`{"simulate":{"line":"bad"}}`, 422},
		{`{"simulate":{"mode":"OK"}}`, 400},
		{`{"simulate":{"mode":"custom","values":{"cam1.kbps":1}}}`, 400},
		{`{"simulate":{"stream":"actual","standby":"nope"}}`, 422},
		{`{"simulate":{"values":{"password":"secret"}}}`, 422},
		{`{"simulate":{"line":"OK","values":{"obs.streaming":1}}}`, 422},
		{`{"simulate":null}`, 200},
	} {
		for _, path := range []string{"/api/source-values", "/api/sources/preview"} {
			body := tc.body
			if path == "/api/sources/preview" && strings.HasPrefix(body, `{"simulate":`) {
				body = `{"names":[],` + body[1:]
			}
			r := httptest.NewRequest("POST", path, strings.NewReader(body))
			r.Header.Set(headerName, "1")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Errorf("%s %s: %d %s want %d", path, body, w.Code, w.Body.String(), tc.status)
			}
		}
	}
	for _, tc := range []struct {
		method string
		header bool
		want   int
	}{{"POST", false, 403}, {"PATCH", true, 405}, {"PUT", true, 405}, {"DELETE", true, 405}} {
		r := httptest.NewRequest(tc.method, "/api/source-values", strings.NewReader(`{}`))
		if tc.header {
			r.Header.Set(headerName, "1")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s=%d want %d", tc.method, w.Code, tc.want)
		}
	}
}

func TestSourceValuesAuthentication(t *testing.T) {
	h := New(&sourcesBackend{}, testPass)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, remote := range []string{"127.0.0.1:1234", "192.168.1.2:1234"} {
			for _, auth := range []bool{false, true} {
				r := httptest.NewRequest(method, "http://localhost/api/source-values", strings.NewReader(`{}`))
				r.RemoteAddr = remote
				r.Header.Set(headerName, "1")
				if auth {
					r.SetBasicAuth("admin", testPass)
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				want := 401
				if auth {
					want = 200
				}
				if w.Code != want {
					t.Errorf("%s %s auth=%v: %d %s", method, remote, auth, w.Code, w.Body.String())
				}
			}
		}
	}
}
