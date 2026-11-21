package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

const testPass = "s3cret-Pass_1"

// fakeBackend は core の代わり。Command の呼び出しを記録し、決めたエラーを返す。
type fakeBackend struct {
	mu       sync.Mutex
	snap     core.Snapshot
	err      error
	calls    []switcher.CommandKind
	deadline time.Duration // 最後の Command の ctx の期限までの残り（期限がなければ 0）
}

func (f *fakeBackend) Snapshot() core.Snapshot { return f.snap }

func (f *fakeBackend) Command(ctx context.Context, k switcher.CommandKind) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, k)
	f.deadline = 0
	if d, ok := ctx.Deadline(); ok {
		f.deadline = time.Until(d)
	}
	return f.err
}

func (f *fakeBackend) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type reqOpt struct {
	user, pass string
	noAuth     bool
	header     *string // nil なら X-IRL-Live-Keeper を付けない
}

func strp(s string) *string { return &s }

func do(t *testing.T, h http.Handler, method, path string, o reqOpt) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	if !o.noAuth {
		u := o.user
		if u == "" {
			u = "admin"
		}
		p := o.pass
		if p == "" {
			p = testPass
		}
		r.SetBasicAuth(u, p)
	}
	if o.header != nil {
		r.Header.Set("X-IRL-Live-Keeper", *o.header)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

var ok1 = strp("1")

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("本文が JSON でない: %v: %q", err, w.Body.String())
	}
	return m
}

var commandPaths = []struct {
	path string
	kind switcher.CommandKind
}{
	{"/api/stream/start", switcher.CmdStreamStart},
	{"/api/stream/stop", switcher.CmdStreamStop},
	{"/api/standby/on", switcher.CmdStandbyOn},
	{"/api/standby/off", switcher.CmdStandbyOff},
	{"/api/cameras/show-all", switcher.CmdShowAllCameras},
}

func TestCommandAccepted(t *testing.T) {
	for _, c := range commandPaths {
		t.Run(c.path, func(t *testing.T) {
			b := &fakeBackend{}
			w := do(t, New(b, testPass), http.MethodPost, c.path, reqOpt{header: ok1})
			if w.Code != http.StatusOK {
				t.Fatalf("code = %d, body %q", w.Code, w.Body.String())
			}
			if m := decodeBody(t, w); m["ok"] != true || len(m) != 1 {
				t.Fatalf("body = %v", m)
			}
			if len(b.calls) != 1 || b.calls[0] != c.kind {
				t.Fatalf("calls = %v, want [%v]", b.calls, c.kind)
			}
			if b.deadline <= 0 || b.deadline > commandTimeout {
				t.Fatalf("ctx の期限までの残り = %v, want (0, %v]", b.deadline, commandTimeout)
			}
		})
	}
}

func TestCommandTimeoutIs5s(t *testing.T) {
	if commandTimeout != 5*time.Second {
		t.Fatalf("commandTimeout = %v, want 5s", commandTimeout)
	}
}

func TestCommandErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		code     int
		errorMsg string // 本文の error に含まれるべき文字列
	}{
		{"invalid state", switcher.ErrInvalidState{Reason: "配信中ではない"}, http.StatusConflict, "配信中ではない"},
		{"invalid state wrapped", fmt.Errorf("x: %w", switcher.ErrInvalidState{Reason: "期待が残っている"}), http.StatusConflict, "期待が残っている"},
		{"not connected", switcher.ErrNotConnected{}, http.StatusServiceUnavailable, "OBS に未接続"},
		{"not connected wrapped", fmt.Errorf("x: %w", switcher.ErrNotConnected{}), http.StatusServiceUnavailable, "OBS に未接続"},
		{"stopped", core.ErrStopped, http.StatusServiceUnavailable, ""},
		{"deadline", context.DeadlineExceeded, http.StatusServiceUnavailable, ""},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &fakeBackend{err: c.err}
			w := do(t, New(b, testPass), http.MethodPost, "/api/standby/on", reqOpt{header: ok1})
			if w.Code != c.code {
				t.Fatalf("code = %d, want %d", w.Code, c.code)
			}
			m := decodeBody(t, w)
			msg, _ := m["error"].(string)
			if msg == "" || len(m) != 1 {
				t.Fatalf("body = %v", m)
			}
			if !strings.Contains(msg, c.errorMsg) {
				t.Fatalf("error = %q, want to contain %q", msg, c.errorMsg)
			}
		})
	}
}

func TestUnauthorized(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		o      reqOpt
	}{
		{"status no auth", http.MethodGet, "/api/status", reqOpt{noAuth: true}},
		{"status wrong pass", http.MethodGet, "/api/status", reqOpt{pass: "wrong"}},
		{"status wrong user", http.MethodGet, "/api/status", reqOpt{user: "root"}},
		{"status pass prefix", http.MethodGet, "/api/status", reqOpt{pass: testPass[:len(testPass)-1]}},
		{"status pass longer", http.MethodGet, "/api/status", reqOpt{pass: testPass + "x"}},
		{"post no auth with header", http.MethodPost, "/api/standby/on", reqOpt{noAuth: true, header: ok1}},
		{"post wrong pass with header", http.MethodPost, "/api/stream/stop", reqOpt{pass: "wrong", header: ok1}},
		{"post no auth no header", http.MethodPost, "/api/stream/stop", reqOpt{noAuth: true}},
		{"wrong method no auth", http.MethodGet, "/api/stream/stop", reqOpt{noAuth: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &fakeBackend{snap: sampleSnapshot()}
			w := do(t, New(b, testPass), c.method, c.path, c.o)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("code = %d, want 401", w.Code)
			}
			if got := w.Header().Get("WWW-Authenticate"); got != `Basic realm="irl-live-keeper"` {
				t.Fatalf("WWW-Authenticate = %q", got)
			}
			m := decodeBody(t, w)
			if _, ok := m["error"].(string); !ok || len(m) != 1 {
				t.Fatalf("body = %v", m)
			}
			if body := w.Body.String(); strings.Contains(body, "rtsp://") || strings.Contains(body, "Camera") {
				t.Fatalf("401 の本文に状態が出ている: %q", body)
			}
			if n := b.callCount(); n != 0 {
				t.Fatalf("Command が %d 回呼ばれた", n)
			}
		})
	}
}

func TestEmptyPasswordNeverAuthenticates(t *testing.T) {
	// config の検証で空のパスワードは弾くが、web 単体でも空の照合で通さない
	b := &fakeBackend{}
	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	r.SetBasicAuth("admin", "")
	w := httptest.NewRecorder()
	New(b, "").ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
}

func TestForbiddenWithoutHeader(t *testing.T) {
	cases := []struct {
		name   string
		header *string
	}{
		{"missing", nil},
		{"empty", strp("")},
		{"zero", strp("0")},
		{"true", strp("true")},
		{"one with space", strp(" 1")},
		{"eleven", strp("11")},
	}
	for _, c := range commandPaths {
		for _, h := range cases {
			t.Run(c.path+"/"+h.name, func(t *testing.T) {
				b := &fakeBackend{}
				w := do(t, New(b, testPass), http.MethodPost, c.path, reqOpt{header: h.header})
				if w.Code != http.StatusForbidden {
					t.Fatalf("code = %d, want 403", w.Code)
				}
				m := decodeBody(t, w)
				if _, ok := m["error"].(string); !ok || len(m) != 1 {
					t.Fatalf("body = %v", m)
				}
				if n := b.callCount(); n != 0 {
					t.Fatalf("Command が %d 回呼ばれた", n)
				}
			})
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	cases := []struct {
		method, path, allow string
	}{
		{http.MethodPost, "/api/status", "GET"},
		{http.MethodPut, "/api/status", "GET"},
		{http.MethodGet, "/api/stream/start", "POST"},
		{http.MethodGet, "/api/standby/off", "POST"},
		{http.MethodDelete, "/api/standby/on", "POST"},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			b := &fakeBackend{}
			w := do(t, New(b, testPass), c.method, c.path, reqOpt{header: ok1})
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("code = %d, want 405", w.Code)
			}
			if got := w.Header().Get("Allow"); got != c.allow {
				t.Fatalf("Allow = %q, want %q", got, c.allow)
			}
			decodeBody(t, w)
			if n := b.callCount(); n != 0 {
				t.Fatalf("Command が %d 回呼ばれた", n)
			}
		})
	}
}

func TestNotFound(t *testing.T) {
	for _, p := range []string{"/", "/api", "/api/", "/api/stream", "/api/standby/toggle", "/api/status/x", "/api/stream/start/"} {
		t.Run(p, func(t *testing.T) {
			b := &fakeBackend{}
			// 未知のパスは認証より先に 404
			w := do(t, New(b, testPass), http.MethodPost, p, reqOpt{noAuth: true, header: ok1})
			if w.Code != http.StatusNotFound {
				t.Fatalf("code = %d, want 404", w.Code)
			}
			decodeBody(t, w)
			if n := b.callCount(); n != 0 {
				t.Fatalf("Command が %d 回呼ばれた", n)
			}
		})
	}
}

func TestStatusGETDoesNotNeedHeader(t *testing.T) {
	b := &fakeBackend{snap: sampleSnapshot()}
	w := do(t, New(b, testPass), http.MethodGet, "/api/status", reqOpt{})
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
}

func sampleSnapshot() core.Snapshot {
	rs, health := "Camera", "OK"
	return core.Snapshot{
		Cameras: []core.CameraStatus{
			{ID: "main", Name: "メイン", OnBad: "standby", State: "OK", Kbps: 4200.5, Online: true, Protocol: "srt",
				SRT: &core.SRTStatus{RTTMs: 85.2, LossRate: 0.01, RecvMbps: 4.2}},
			{ID: "sub", Name: "サブ", OnBad: "hide", State: "DOWN"},
		},
		OBS: core.OBSStatus{Connected: true, Streaming: true, Scene: "Standby", SceneClass: "standby",
			SceneCameras: []string{"main"}, SceneHealth: &health,
			Standby:     core.StandbyStatus{Owner: "auto", ReturnScene: &rs},
			AutoEnabled: true, Warnings: []string{"シーン X がない"}},
		MediaMTX: "running",
		URLs: []core.CameraURLs{
			{ID: "main", Name: "メイン", PublishSRT: "srt://a", PublishRTMP: "rtmp://b", OBSRead: "rtsp://obs:p@c"},
			{ID: "sub", Name: "サブ", PublishSRT: "srt://a2", PublishRTMP: "rtmp://b2", OBSRead: "rtsp://obs:p@c2"},
		},
		LastError: &core.LastError{At: time.Date(2026, 11, 20, 13, 0, 0, 0, time.UTC), Source: "switcher", Message: "失敗"},
	}
}

// keys は JSON のオブジェクトのキーを並べる。
func keys(t *testing.T, v any) []string {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("オブジェクトでない: %#v", v)
	}
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return sorted(ks)
}

func sorted(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

func eqKeys(t *testing.T, name string, v any, want ...string) {
	t.Helper()
	got := keys(t, v)
	want = sorted(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s のキー = %v, want %v", name, got, want)
	}
}

func TestStatusShapeFull(t *testing.T) {
	b := &fakeBackend{snap: sampleSnapshot()}
	w := do(t, New(b, testPass), http.MethodGet, "/api/status", reqOpt{})
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	m := decodeBody(t, w)
	eqKeys(t, "top", m, "cameras", "obs", "mediamtx", "urls", "last_error")
	cams := m["cameras"].([]any)
	if len(cams) != 2 {
		t.Fatalf("cameras = %v", cams)
	}
	cam := cams[0].(map[string]any)
	eqKeys(t, "cameras[0]", cam, "id", "name", "on_bad", "state", "kbps", "online", "protocol", "srt")
	eqKeys(t, "cameras[0].srt", cam["srt"], "rtt_ms", "loss_rate", "recv_mbps")
	o := m["obs"].(map[string]any)
	eqKeys(t, "obs", o, "connected", "streaming", "scene", "scene_class", "scene_cameras", "scene_health", "standby", "auto_enabled", "warnings")
	eqKeys(t, "obs.standby", o["standby"], "owner", "return_scene")
	urls := m["urls"].([]any)
	if len(urls) != 2 {
		t.Fatalf("urls = %v", urls)
	}
	eqKeys(t, "urls[0]", urls[0], "id", "name", "publish_srt", "publish_rtmp", "obs_read")
	eqKeys(t, "last_error", m["last_error"], "at", "source", "message")

	if cam["id"] != "main" || cam["name"] != "メイン" || cam["on_bad"] != "standby" ||
		cam["state"] != "OK" || cam["kbps"] != 4200.5 || cam["online"] != true || cam["protocol"] != "srt" {
		t.Fatalf("cameras[0] = %v", cam)
	}
	if sc, _ := o["scene_cameras"].([]any); len(sc) != 1 || sc[0] != "main" || o["scene_health"] != "OK" {
		t.Fatalf("scene_cameras/scene_health = %v / %v", o["scene_cameras"], o["scene_health"])
	}
	if o["scene_class"] != "standby" || o["standby"].(map[string]any)["owner"] != "auto" ||
		o["standby"].(map[string]any)["return_scene"] != "Camera" {
		t.Fatalf("obs = %v", o)
	}
	if ws, ok := o["warnings"].([]any); !ok || len(ws) != 1 || ws[0] != "シーン X がない" {
		t.Fatalf("warnings = %#v", o["warnings"])
	}
	if m["mediamtx"] != "running" {
		t.Fatalf("mediamtx = %v", m["mediamtx"])
	}
	le := m["last_error"].(map[string]any)
	if le["at"] != "2026-11-20T13:00:00Z" || le["source"] != "switcher" || le["message"] != "失敗" {
		t.Fatalf("last_error = %v", le)
	}
}

func TestStatusShapeNulls(t *testing.T) {
	b := &fakeBackend{snap: core.Snapshot{
		Cameras: []core.CameraStatus{{ID: "main", Name: "メイン", OnBad: "standby", State: "DOWN"}},
		OBS: core.OBSStatus{SceneClass: "unknown", SceneCameras: []string{}, Standby: core.StandbyStatus{Owner: "none"},
			Warnings: []string{}},
		URLs: []core.CameraURLs{},
	}}
	w := do(t, New(b, testPass), http.MethodGet, "/api/status", reqOpt{})
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	m := decodeBody(t, w)
	eqKeys(t, "top", m, "cameras", "obs", "mediamtx", "urls", "last_error")
	if v, ok := m["last_error"]; !ok || v != nil {
		t.Fatalf("last_error = %#v, want null", v)
	}
	cam := m["cameras"].([]any)[0].(map[string]any)
	if v, ok := cam["srt"]; !ok || v != nil {
		t.Fatalf("srt = %#v, want null", v)
	}
	o := m["obs"].(map[string]any)
	if v, ok := o["scene_health"]; !ok || v != nil {
		t.Fatalf("scene_health = %#v, want null", v)
	}
	if sc, ok := o["scene_cameras"].([]any); !ok || len(sc) != 0 {
		t.Fatalf("scene_cameras = %#v, want []", o["scene_cameras"])
	}
	if v, ok := o["standby"].(map[string]any)["return_scene"]; !ok || v != nil {
		t.Fatalf("return_scene = %#v, want null", v)
	}
	if ws, ok := o["warnings"].([]any); !ok || len(ws) != 0 {
		t.Fatalf("warnings = %#v, want []", o["warnings"])
	}
}

func TestStatusNilWarningsBecomesEmptyArray(t *testing.T) {
	// core は [] にするが、web でも null を出さない
	b := &fakeBackend{snap: core.Snapshot{OBS: core.OBSStatus{Warnings: nil}}}
	w := do(t, New(b, testPass), http.MethodGet, "/api/status", reqOpt{})
	if !strings.Contains(w.Body.String(), `"warnings":[]`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

// すべて表示に戻す: OBS に未接続なら 503、終了中なら 409（理由つき）。独自ヘッダーがなければ 403。
func TestShowAllErrors(t *testing.T) {
	const path = "/api/cameras/show-all"
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"未接続", switcher.ErrNotConnected{}, http.StatusServiceUnavailable},
		{"終了中", switcher.ErrInvalidState{Reason: switcher.ReasonStopping}, http.StatusConflict},
		{"同じ操作が実行待ち", switcher.ErrInvalidState{Reason: switcher.ReasonSamePending}, http.StatusConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &fakeBackend{err: c.err}
			w := do(t, New(b, testPass), http.MethodPost, path, reqOpt{header: ok1})
			if w.Code != c.want {
				t.Fatalf("code = %d, want %d: %q", w.Code, c.want, w.Body.String())
			}
			if body := decodeBody(t, w); body["error"] == "" {
				t.Errorf("error が空: %#v", body)
			}
		})
	}
	b := &fakeBackend{}
	if w := do(t, New(b, testPass), http.MethodPost, path, reqOpt{}); w.Code != http.StatusForbidden || b.callCount() != 0 {
		t.Errorf("ヘッダーなし: code = %d, calls = %d", w.Code, b.callCount())
	}
	if w := do(t, New(b, testPass), http.MethodGet, path, reqOpt{header: ok1}); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: code = %d", w.Code)
	}
}
