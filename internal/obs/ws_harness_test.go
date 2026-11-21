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
		return auxReply{Data: jsonString(map[string]any{"scenes": []map[string]any{{"sceneName": scene, "sceneUuid": uuid, "sceneIndex": 0}}})}
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
