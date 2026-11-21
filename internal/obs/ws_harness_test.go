package obs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/andreykaipov/goobs/api/requests/scenes"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// fakeOBS は偽の obs-websocket の相手が返す、OBS の状態（接続の読み取りの応答の元）。
type fakeOBS struct {
	hookMu    sync.Mutex // hook の呼び出しを1本にする（hook の持つ変数を守る）
	mu        sync.Mutex
	scene     string
	sceneUUID string
	transMs   float64
	version   string
	// hook は、既定の応答より先に呼ばれる。ok が true ならその応答を使う。
	hook func(auxReq) (auxReply, bool)
	// counts は要求の種類ごとの回数。
	counts map[string]int

	// 地図の読み取りの元。scenes が nil なら、今のシーン1つだけ（項目なし）。
	collection string
	scenes     []fakeScene
	groups     map[string]fakeScene // 鍵はグループの UUID
	inputs     []fakeInput
}

type fakeItem struct {
	ID         int
	SourceUUID string
	SourceName string
	Kind       string // "input" | "scene" | "group"
	Enabled    bool
}

type fakeScene struct {
	UUID, Name string
	Items      []fakeItem
}

type fakeInput struct {
	UUID, Name, Kind string
	Settings         map[string]any
}

// rtspInput は URL がカメラに当たる ffmpeg_source。
func rtspInput(uuid, name, url string) fakeInput {
	return fakeInput{UUID: uuid, Name: name, Kind: "ffmpeg_source",
		Settings: map[string]any{"input": url, "is_local_file": false}}
}

// update は世界の状態を、相手が応答するときと競合しないように書き換える。
func (f *fakeOBS) update(fn func(*fakeOBS)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeOBS) sceneItemsJSON(items []fakeItem) string {
	var out []map[string]any
	for i, it := range items {
		m := map[string]any{
			"sceneItemId": it.ID, "sceneItemIndex": i, "sourceUuid": it.SourceUUID, "sourceName": it.SourceName,
			"sceneItemEnabled": it.Enabled, "isGroup": it.Kind == "group",
		}
		switch it.Kind {
		case "input":
			m["sourceType"], m["inputKind"] = "OBS_SOURCE_TYPE_INPUT", "ffmpeg_source"
		default:
			m["sourceType"], m["inputKind"] = "OBS_SOURCE_TYPE_SCENE", nil
		}
		out = append(out, m)
	}
	return jsonString(map[string]any{"sceneItems": out})
}

func newFakeOBS() *fakeOBS {
	return &fakeOBS{scene: "Live", sceneUUID: "uuid-live", transMs: 300, version: "5.5.0", counts: map[string]int{}}
}

func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (f *fakeOBS) setScene(name, uuid string) {
	f.mu.Lock()
	f.scene, f.sceneUUID = name, uuid
	f.mu.Unlock()
}

func (f *fakeOBS) setTransMs(ms float64) {
	f.mu.Lock()
	f.transMs = ms
	f.mu.Unlock()
}

func (f *fakeOBS) setHook(h func(auxReq) (auxReply, bool)) {
	f.mu.Lock()
	f.hook = h
	f.mu.Unlock()
}

func (f *fakeOBS) count(typ string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[typ]
}

func (f *fakeOBS) handle(r auxReq) auxReply {
	f.mu.Lock()
	f.counts[r.Type]++
	hook := f.hook
	scene, uuid, ms, version := f.scene, f.sceneUUID, f.transMs, f.version
	f.mu.Unlock()
	if hook != nil {
		f.hookMu.Lock()
		rep, ok := hook(r)
		f.hookMu.Unlock()
		if ok {
			return rep
		}
	}
	switch r.Type {
	case "GetVersion":
		return auxReply{Data: jsonString(map[string]any{"obsWebSocketVersion": version, "rpcVersion": 1})}
	case "GetStreamStatus":
		return auxReply{Data: jsonString(map[string]any{"outputActive": false, "outputReconnecting": false})}
	case "GetCurrentProgramScene":
		return auxReply{Data: jsonString(map[string]any{
			"sceneName": scene, "sceneUuid": uuid, "currentProgramSceneName": scene, "currentProgramSceneUuid": uuid,
		})}
	case "GetCurrentSceneTransition":
		return auxReply{Data: jsonString(map[string]any{"transitionName": "Fade", "transitionFixed": false, "transitionDuration": ms})}
	case "GetSceneList":
		f.mu.Lock()
		defer f.mu.Unlock()
		list := f.scenes
		if list == nil {
			list = []fakeScene{{UUID: uuid, Name: scene}}
		}
		var out []map[string]any
		for i, sc := range list {
			out = append(out, map[string]any{"sceneName": sc.Name, "sceneUuid": sc.UUID, "sceneIndex": i})
		}
		return auxReply{Data: jsonString(map[string]any{"scenes": out})}
	case "GetSceneCollectionList":
		f.mu.Lock()
		defer f.mu.Unlock()
		return auxReply{Data: jsonString(map[string]any{"currentSceneCollectionName": f.collection, "sceneCollections": []string{f.collection}})}
	case "GetSceneItemList", "GetGroupSceneItemList":
		var q struct {
			SceneUUID string `json:"sceneUuid"`
		}
		_ = json.Unmarshal(r.Data, &q)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Type == "GetGroupSceneItemList" {
			if g, ok := f.groups[q.SceneUUID]; ok {
				return auxReply{Data: f.sceneItemsJSON(g.Items)}
			}
			return auxReply{Code: 600}
		}
		for _, sc := range f.scenes {
			if sc.UUID == q.SceneUUID {
				return auxReply{Data: f.sceneItemsJSON(sc.Items)}
			}
		}
		if f.scenes == nil && q.SceneUUID == uuid {
			return auxReply{Data: f.sceneItemsJSON(nil)}
		}
		return auxReply{Code: 600}
	case "GetInputList":
		var q struct {
			InputKind string `json:"inputKind"`
		}
		_ = json.Unmarshal(r.Data, &q)
		f.mu.Lock()
		defer f.mu.Unlock()
		var out []map[string]any
		for _, in := range f.inputs {
			if q.InputKind == "" || in.Kind == q.InputKind {
				out = append(out, map[string]any{"inputUuid": in.UUID, "inputName": in.Name, "inputKind": in.Kind})
			}
		}
		return auxReply{Data: jsonString(map[string]any{"inputs": out})}
	case "GetInputSettings":
		var q struct {
			InputUUID string `json:"inputUuid"`
		}
		_ = json.Unmarshal(r.Data, &q)
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, in := range f.inputs {
			if in.UUID == q.InputUUID {
				return auxReply{Data: jsonString(map[string]any{"inputKind": in.Kind, "inputSettings": in.Settings})}
			}
		}
		return auxReply{Code: 600}
	}
	return auxReply{}
}

// wsHarness は偽の相手と WS をつなぎ、流れてきたイベントを集める。
type wsHarness struct {
	t      *testing.T
	s      *auxServer
	f      *fakeOBS
	w      *WS
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	evs []switcher.Event
}

// newWSHarness は WS を作って Run する。tune は Run の前に WS を調整する。
func newWSHarness(t *testing.T, tune func(*WS)) *wsHarness {
	t.Helper()
	s := newAuxServer(t)
	f := newFakeOBS()
	s.setHandler(f.handle)
	return startWSHarness(t, s, f, tune)
}

func startWSHarness(t *testing.T, s *auxServer, f *fakeOBS, tune func(*WS)) *wsHarness {
	t.Helper()
	s.mu.Lock()
	s.version = f.version
	s.mu.Unlock()
	w := New(s.config(t), slog.New(slog.DiscardHandler))
	w.timeout = 300 * time.Millisecond
	if tune != nil {
		tune(w)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &wsHarness{t: t, s: s, f: f, w: w, cancel: cancel, done: make(chan struct{})}
	go func() { w.Run(ctx); close(h.done) }()
	go func() {
		for {
			select {
			case e := <-w.Events():
				h.mu.Lock()
				h.evs = append(h.evs, e)
				h.mu.Unlock()
			case <-h.done:
				return
			}
		}
	}()
	t.Cleanup(h.stop)
	return h
}

// stop は Run を止めて、戻るのを待つ。何度呼んでもよい。
func (h *wsHarness) stop() {
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(10 * time.Second):
		h.t.Error("ctx を取り消しても Run が戻らない")
	}
}

func (h *wsHarness) events() []switcher.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]switcher.Event(nil), h.evs...)
}

// await は、集めたイベントが cond を満たすまで待つ。
func (h *wsHarness) await(what string, cond func([]switcher.Event) bool) []switcher.Event {
	h.t.Helper()
	for i := 0; i < 500; i++ {
		evs := h.events()
		if cond(evs) {
			return evs
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("待っても成り立たない: %s（集めたイベント: %s）", what, describe(h.events()))
	return nil
}

func describe(evs []switcher.Event) string {
	s := ""
	for _, e := range evs {
		s += fmt.Sprintf("%T%+v ", e, e)
	}
	return s
}

// countOf は種類 T のイベントの数。
func countOf[T switcher.Event](evs []switcher.Event) int {
	n := 0
	for _, e := range evs {
		if _, ok := e.(T); ok {
			n++
		}
	}
	return n
}

// lastOf は種類 T の最後のイベント。
func lastOf[T switcher.Event](evs []switcher.Event) (T, bool) {
	var zero T
	for i := len(evs) - 1; i >= 0; i-- {
		if e, ok := evs[i].(T); ok {
			return e, true
		}
	}
	return zero, false
}

// awaitConnected は OBSConnected が n 回流れるまで待つ。
func (h *wsHarness) awaitConnected(n int) []switcher.Event {
	h.t.Helper()
	return h.await(fmt.Sprintf("OBSConnected が %d 回", n), func(evs []switcher.Event) bool {
		return countOf[switcher.OBSConnected](evs) >= n
	})
}

func (h *wsHarness) awaitDisconnected(n int) []switcher.Event {
	h.t.Helper()
	return h.await(fmt.Sprintf("OBSDisconnected が %d 回", n), func(evs []switcher.Event) bool {
		return countOf[switcher.OBSDisconnected](evs) >= n
	})
}

// requestsOf は、相手に届いた種類 typ の要求。
func (s *auxServer) requestsOf(typ string) []auxReq {
	var out []auxReq
	for _, r := range s.requests() {
		if r.Type == typ {
			out = append(out, r)
		}
	}
	return out
}

func itoa(n int) string { return fmt.Sprint(n) }

func newSetScene(name string) *scenes.SetCurrentProgramSceneParams {
	return scenes.NewSetCurrentProgramSceneParams().WithSceneName(name)
}
