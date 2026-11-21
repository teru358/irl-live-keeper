package obs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/gorilla/websocket"
)

// ---- 偽の obs-websocket の相手 ----

type auxReq struct {
	Type string
	ID   string
	Data json.RawMessage
	Conn int // 何本目の接続か（1 から）
}

// auxReply は要求への応答のしかた。
type auxReply struct {
	Skip  bool          // 返さない
	Delay time.Duration // 遅らせる
	ID    string        // 空でなければ、この ID で返す（ID のずれ）
	Data  string        // 応答の responseData（空なら {}）
}

type auxServer struct {
	srv     *httptest.Server
	handler func(auxReq) auxReply
	// ghost が true なら、Identified の直後に、誰も待っていない応答を1つ送る。goobs の
	// handleOpcodes が受け手のいない応答の送信で止まり、以後の要求が goobs の中で止まる。
	ghost  bool
	wsConn map[int]*websocket.Conn

	mu     sync.Mutex
	conns  int
	reqs   []auxReq
	closed chan int // 接続が閉じられたとき、その接続の番号を流す
	wg     sync.WaitGroup
}

func newAuxServer(t *testing.T) *auxServer {
	t.Helper()
	s := &auxServer{closed: make(chan int, 1024)}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns++
		n := s.conns
		if s.wsConn == nil {
			s.wsConn = map[int]*websocket.Conn{}
		}
		s.wsConn[n] = c
		ghost := s.ghost
		s.mu.Unlock()
		defer func() { c.Close(); s.closed <- n }()

		var wmu sync.Mutex
		write := func(v any) {
			wmu.Lock()
			defer wmu.Unlock()
			_ = c.WriteJSON(v)
		}
		write(map[string]any{"op": 0, "d": map[string]any{
			"obsWebSocketVersion": "5.5.0", "rpcVersion": 1,
			"authentication": map[string]string{"challenge": "", "salt": ""},
		}})
		connDone := make(chan struct{})
		defer close(connDone)
		for {
			var m struct {
				Op int             `json:"op"`
				D  json.RawMessage `json:"d"`
			}
			if err := c.ReadJSON(&m); err != nil {
				return
			}
			switch m.Op {
			case 1:
				write(map[string]any{"op": 2, "d": map[string]any{"negotiatedRpcVersion": 1}})
				if ghost {
					write(map[string]any{"op": 7, "d": map[string]any{
						"requestType": "Ghost", "requestId": "ghost",
						"requestStatus": map[string]any{"result": true, "code": 100},
					}})
				}
			case 6:
				var d struct {
					Type string          `json:"requestType"`
					ID   string          `json:"requestId"`
					Data json.RawMessage `json:"requestData"`
				}
				_ = json.Unmarshal(m.D, &d)
				req := auxReq{Type: d.Type, ID: d.ID, Data: d.Data, Conn: n}
				s.mu.Lock()
				s.reqs = append(s.reqs, req)
				h := s.handler
				s.mu.Unlock()
				var rep auxReply
				if h != nil {
					rep = h(req)
				}
				if rep.Skip {
					continue
				}
				id := d.ID
				if rep.ID != "" {
					id = rep.ID
				}
				data := json.RawMessage("{}")
				if rep.Data != "" {
					data = json.RawMessage(rep.Data)
				}
				resp := map[string]any{"op": 7, "d": map[string]any{
					"requestType": d.Type, "requestId": id,
					"requestStatus": map[string]any{"result": true, "code": 100},
					"responseData":  data,
				}}
				if rep.Delay <= 0 {
					write(resp)
					continue
				}
				s.wg.Add(1)
				go func() {
					defer s.wg.Done()
					select {
					case <-time.After(rep.Delay):
						write(resp)
					case <-connDone:
					}
				}()
			}
		}
	}))
	t.Cleanup(func() { s.srv.Close(); s.wg.Wait() })
	return s
}

// kill は n 本目の接続を、サーバーの側から閉じる（OBS から切られたことにする）。
func (s *auxServer) kill(n int) {
	s.mu.Lock()
	c := s.wsConn[n]
	s.mu.Unlock()
	if c != nil {
		c.Close()
	}
}

func (s *auxServer) setHandler(h func(auxReq) auxReply) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

func (s *auxServer) accepted() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns
}

func (s *auxServer) requests() []auxReq {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]auxReq(nil), s.reqs...)
}

func (s *auxServer) config(t *testing.T) Config {
	t.Helper()
	host, port, err := net.SplitHostPort(s.srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := strconv.Atoi(port)
	return Config{Host: host, Port: p}
}

// waitClosed は接続 n が閉じられるのを待つ。
func (s *auxServer) waitClosed(t *testing.T, what string) int {
	t.Helper()
	select {
	case n := <-s.closed:
		return n
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: サーバー側の接続が閉じられない", what)
		return 0
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("待っても成り立たない: %s", what)
}

// ---- 偽の時計 ----

type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
}
type fakeWaiter struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.waiters = append(c.waiters, fakeWaiter{at: c.now.Add(d), ch: ch})
	return ch
}

// pending は、まだ起きていない待ちの数。
func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	rest := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.at.After(c.now) {
			w.ch <- c.now
		} else {
			rest = append(rest, w)
		}
	}
	c.waiters = rest
}

// ---- 部品 ----

// newTestAux は短い期限（reqTimeout）の Aux を作る。Cleanup で Close する。
func newTestAux(t *testing.T, cfg Config, reqTimeout time.Duration) *Aux {
	t.Helper()
	a := NewAux(cfg, slog.New(slog.DiscardHandler))
	a.reqTimeout = reqTimeout
	a.respTimeout = reqTimeout
	t.Cleanup(a.Close)
	return a
}

func mustJSON(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("requestData が JSON でない: %q: %v", raw, err)
	}
	return m
}

func TestAuxSendsUUIDArguments(t *testing.T) {
	s := newAuxServer(t)
	a := newTestAux(t, s.config(t), 2*time.Second)
	a.SetMainConnected(true)

	if _, err := a.GetSceneList(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetSceneItemList("scene-uuid"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetGroupSceneItemList("group-uuid"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetInputList("ffmpeg_source"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetInputSettings("input-uuid"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetSceneCollectionList(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetInputSettings("input-uuid", map[string]any{"input": "rtsp://x"}, true); err != nil {
		t.Fatal(err)
	}

	reqs := s.requests()
	if len(reqs) != 7 {
		t.Fatalf("要求が %d 本届いた: %+v", len(reqs), reqs)
	}
	want := []struct {
		typ  string
		args map[string]any
	}{
		{"GetSceneList", nil},
		{"GetSceneItemList", map[string]any{"sceneUuid": "scene-uuid"}},
		{"GetGroupSceneItemList", map[string]any{"sceneUuid": "group-uuid"}},
		{"GetInputList", map[string]any{"inputKind": "ffmpeg_source"}},
		{"GetInputSettings", map[string]any{"inputUuid": "input-uuid"}},
		{"GetSceneCollectionList", nil},
		{"SetInputSettings", map[string]any{"inputUuid": "input-uuid", "overlay": true}},
	}
	for i, w := range want {
		r := reqs[i]
		if r.Type != w.typ {
			t.Errorf("%d 本目: %s のはずが %s", i, w.typ, r.Type)
		}
		got := map[string]any{}
		if len(r.Data) > 0 {
			got = mustJSON(t, r.Data)
		}
		for k, v := range w.args {
			if got[k] != v {
				t.Errorf("%s: %s = %v のはずが %v（%s）", w.typ, k, v, got[k], r.Data)
			}
		}
		for _, name := range []string{"sceneName", "inputName"} {
			if _, ok := got[name]; ok {
				t.Errorf("%s: 名前の引数 %s が送られている（UUID だけで送る）", w.typ, name)
			}
		}
		if w.typ == "SetInputSettings" {
			is, _ := got["inputSettings"].(map[string]any)
			if is["input"] != "rtsp://x" {
				t.Errorf("SetInputSettings: inputSettings が届かない: %s", r.Data)
			}
		}
	}
}

func TestAuxOpensOnlyWhileMainConnected(t *testing.T) {
	s := newAuxServer(t)
	a := newTestAux(t, s.config(t), 2*time.Second)

	if _, err := a.GetSceneList(); !errors.Is(err, ErrAuxUnavailable) {
		t.Fatalf("本来の接続がないとき: ErrAuxUnavailable のはずが %v", err)
	}
	if n := s.accepted(); n != 0 {
		t.Fatalf("本来の接続がないのに補助の接続が開かれた: %d 本", n)
	}

	a.SetMainConnected(true)
	waitFor(t, "つながったら要求なしでも開く", func() bool { return s.accepted() == 1 })
	if _, err := a.GetSceneList(); err != nil {
		t.Fatal(err)
	}

	a.SetMainConnected(false)
	if n := s.waitClosed(t, "本来の接続が切れたとき"); n != 1 {
		t.Fatalf("閉じられた接続が %d 本目", n)
	}
	start := time.Now()
	if _, err := a.GetSceneList(); !errors.Is(err, ErrAuxUnavailable) {
		t.Fatalf("切れたあと: ErrAuxUnavailable のはずが %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("開いていないときの要求は待たずに失敗するべき")
	}

	a.SetMainConnected(true)
	waitFor(t, "また開く", func() bool { return s.accepted() == 2 })
	if _, err := a.GetSceneList(); err != nil {
		t.Fatal(err)
	}
}

func TestAuxTimeoutIncludesMutexWait(t *testing.T) {
	s := newAuxServer(t)
	s.setHandler(func(auxReq) auxReply { return auxReply{Skip: true} })
	const limit = 300 * time.Millisecond
	a := newTestAux(t, s.config(t), limit)
	// goobs の応答待ちは期限よりずっと長くして、期限で戻るのが Aux の timer だと確かめる。
	a.respTimeout = 1500 * time.Millisecond
	a.SetMainConnected(true)
	waitFor(t, "開く", func() bool { return s.accepted() == 1 })

	type res struct {
		err error
		el  time.Duration
	}
	run := func(ch chan<- res) {
		start := time.Now()
		_, err := a.GetSceneList()
		ch <- res{err, time.Since(start)}
	}
	r1, r2 := make(chan res, 1), make(chan res, 1)
	go run(r1)
	waitFor(t, "1本目が相手に届く", func() bool { return len(s.requests()) == 1 })
	go run(r2)
	time.Sleep(50 * time.Millisecond)
	if n := len(s.requests()); n != 1 {
		t.Fatalf("2本目は Aux の中で待つはずだが、相手に %d 本届いた", n)
	}
	for i, ch := range []chan res{r1, r2} {
		select {
		case r := <-ch:
			if !errors.Is(r.err, ErrAuxTimeout) {
				t.Errorf("%d 本目: ErrAuxTimeout のはずが %v", i+1, r.err)
			}
			if r.el < limit-60*time.Millisecond || r.el > limit+150*time.Millisecond {
				t.Errorf("%d 本目: 期限 %v のあたりで戻るはずが %v", i+1, limit, r.el)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%d 本目が戻らない", i+1)
		}
	}
	s.waitClosed(t, "期限切れのあと")
	if n := len(s.requests()); n != 1 {
		t.Fatalf("2本目が goobs に入って相手に届いた: %d 本", n)
	}
}

// 接続を開く待ちで期限が来る場合（偽の時計で期限を起こす）。
func TestAuxDeadlineWhileOpening(t *testing.T) {
	s := newAuxServer(t)
	clk := newFakeClock()
	a := newTestAux(t, s.config(t), 2*time.Second)
	a.clock = clk
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	a.dialFn = func(ctx context.Context) (*goobs.Client, *connRecorder, error) {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, nil, errors.New("開けない")
	}
	a.SetMainConnected(true)
	<-started

	errc := make(chan error, 1)
	go func() { _, err := a.GetSceneList(); errc <- err }()
	waitFor(t, "要求の期限の待ちが置かれる", func() bool { return clk.pending() == 1 })
	select {
	case err := <-errc:
		t.Fatalf("期限の前に戻った: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	clk.Advance(2 * time.Second)
	select {
	case err := <-errc:
		if !errors.Is(err, ErrAuxTimeout) {
			t.Fatalf("ErrAuxTimeout のはずが %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("期限が来ても、開く待ちの要求が戻らない")
	}
	close(release)
}

// goobs の中で止まった要求（受け手のいない応答で handleOpcodes が止まる）があっても、
// 要求は期限で戻り、Close は上限内に戻る。
func TestAuxCloseReturnsWhenGoobsIsStuck(t *testing.T) {
	s := newAuxServer(t)
	s.ghost = true
	a := newTestAux(t, s.config(t), 300*time.Millisecond)
	a.respTimeout = 300 * time.Millisecond
	a.SetMainConnected(true)
	waitFor(t, "開く", func() bool { return s.accepted() == 1 })

	errs := make(chan error, 2)
	for range 2 {
		go func() { _, err := a.GetSceneList(); errs <- err }()
	}
	for range 2 {
		select {
		case err := <-errs:
			if !errors.Is(err, ErrAuxTimeout) {
				t.Errorf("ErrAuxTimeout のはずが %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("要求が期限で戻らない")
		}
	}
	closed := make(chan struct{})
	go func() { a.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(a.respTimeout + 3*time.Second):
		t.Fatal("goobs の中で止まった goroutine があると Close が戻らない")
	}
	s.waitClosed(t, "Close のあと")
}

// 要求を出さなくても、本来の接続がつながっている間は、試す間隔の規則で自分で開き直す。
func TestAuxRetriesWithoutRequests(t *testing.T) {
	s := newAuxServer(t)
	clk := newFakeClock()
	a := newTestAux(t, s.config(t), 2*time.Second)
	a.clock = clk
	var mu sync.Mutex
	attempts := 0
	fail := true
	real := a.dialFn
	a.dialFn = func(ctx context.Context) (*goobs.Client, *connRecorder, error) {
		mu.Lock()
		attempts++
		f := fail
		mu.Unlock()
		if f {
			return nil, nil, errors.New("開けない")
		}
		return real(ctx)
	}
	count := func() int { mu.Lock(); defer mu.Unlock(); return attempts }

	a.SetMainConnected(true)
	waitFor(t, "最初の試み", func() bool { return count() == 1 })
	for i, sec := range []int{5, 10, 20, 30, 30} {
		d := time.Duration(sec) * time.Second
		waitFor(t, "待ちが置かれる", func() bool { return clk.pending() == 1 })
		before := count()
		clk.Advance(d - time.Millisecond)
		time.Sleep(30 * time.Millisecond)
		if count() != before {
			t.Fatalf("%d 回目の間隔 %ds の途中で試した", i+1, sec)
		}
		clk.Advance(time.Millisecond)
		waitFor(t, "間隔のあとに試す", func() bool { return count() == before+1 })
	}

	// 成功したら、そのあと補助の接続だけが切られても、また試す。
	mu.Lock()
	fail = false
	mu.Unlock()
	waitFor(t, "待ちが置かれる", func() bool { return clk.pending() == 1 })
	clk.Advance(30 * time.Second)
	waitFor(t, "開く", func() bool { return s.accepted() == 1 })
	before := count()
	s.kill(1)
	waitFor(t, "補助の接続だけが切られたらまた開く", func() bool { return s.accepted() == 2 })
	if count() != before+1 {
		t.Fatalf("試した回数が %d → %d", before, count())
	}

	// 本来の接続が切れたら、試さない。
	mu.Lock()
	fail = true
	mu.Unlock()
	a.SetMainConnected(false)
	s.waitClosed(t, "1本目")
	s.waitClosed(t, "2本目")
	before = count()
	for range 3 {
		clk.Advance(60 * time.Second)
		time.Sleep(30 * time.Millisecond)
	}
	if count() != before {
		t.Fatalf("本来の接続が切れているのに試した: %d → %d", before, count())
	}
}

func TestAuxRebuildsClientAfterTimeout(t *testing.T) {
	s := newAuxServer(t)
	const limit = 300 * time.Millisecond
	// 1本目は期限のあとに遅れて応答する。作り直したあとの2本目に届いてはならない。
	s.setHandler(func(r auxReq) auxReply {
		if r.Conn == 1 {
			return auxReply{Delay: limit + 200*time.Millisecond}
		}
		return auxReply{}
	})
	a := newTestAux(t, s.config(t), limit)
	a.SetMainConnected(true)
	waitFor(t, "開く", func() bool { return s.accepted() == 1 })

	if _, err := a.GetSceneList(); !errors.Is(err, ErrAuxTimeout) {
		t.Fatalf("ErrAuxTimeout のはずが %v", err)
	}
	if n := s.waitClosed(t, "期限切れのあと"); n != 1 {
		t.Fatalf("閉じられたのは %d 本目", n)
	}
	if _, err := a.GetSceneList(); err != nil {
		t.Fatalf("作り直したあとの要求が失敗: %v", err)
	}
	if n := s.accepted(); n != 2 {
		t.Fatalf("新しい接続が受けられていない: %d 本", n)
	}
	time.Sleep(limit + 300*time.Millisecond) // 遅れた応答が届く（閉じた接続へ）時間
	if _, err := a.GetSceneList(); err != nil {
		t.Fatalf("遅れた応答の影響を受けた: %v", err)
	}
}

func TestAuxRebuildsClientAfterMismatchedID(t *testing.T) {
	s := newAuxServer(t)
	s.setHandler(func(r auxReq) auxReply {
		if r.Conn == 1 {
			return auxReply{ID: "someone-else"}
		}
		return auxReply{}
	})
	a := newTestAux(t, s.config(t), 2*time.Second)
	a.SetMainConnected(true)
	waitFor(t, "開く", func() bool { return s.accepted() == 1 })

	if _, err := a.GetSceneList(); !errors.Is(err, ErrAuxMismatch) {
		t.Fatalf("ErrAuxMismatch のはずが %v", err)
	}
	if n := s.waitClosed(t, "ID のずれのあと"); n != 1 {
		t.Fatalf("閉じられたのは %d 本目", n)
	}
	if _, err := a.GetSceneList(); err != nil {
		t.Fatalf("作り直したあとの要求が失敗: %v", err)
	}
	if n := s.accepted(); n != 2 {
		t.Fatalf("新しい接続が受けられていない: %d 本", n)
	}
}

func TestAuxRetryIntervalSequence(t *testing.T) {
	s := newAuxServer(t)
	clk := newFakeClock()
	a := newTestAux(t, s.config(t), 2*time.Second)
	a.clock = clk
	var mu sync.Mutex
	attempts := 0
	fail := true
	real := a.dialFn
	a.dialFn = func(ctx context.Context) (*goobs.Client, *connRecorder, error) {
		mu.Lock()
		attempts++
		f := fail
		mu.Unlock()
		if f {
			return nil, nil, errors.New("開けない")
		}
		return real(ctx)
	}
	count := func() int { mu.Lock(); defer mu.Unlock(); return attempts }
	setFail := func(v bool) { mu.Lock(); fail = v; mu.Unlock() }

	a.SetMainConnected(true)
	waitFor(t, "最初の試み", func() bool { return count() == 1 })
	waitFor(t, "最初の失敗が記録される", func() bool {
		_, err := a.GetSceneList()
		return errors.Is(err, ErrAuxUnavailable) && count() == 1
	})

	for i, sec := range []int{5, 10, 20, 30, 30} {
		d := time.Duration(sec) * time.Second
		before := count()
		clk.Advance(d - time.Millisecond)
		if _, err := a.GetSceneList(); !errors.Is(err, ErrAuxUnavailable) {
			t.Fatalf("%d 回目の間隔 %ds の途中: ErrAuxUnavailable のはずが %v", i+1, sec, err)
		}
		if count() != before {
			t.Fatalf("%d 回目の間隔 %ds の途中で試した", i+1, sec)
		}
		clk.Advance(time.Millisecond)
		if _, err := a.GetSceneList(); !errors.Is(err, ErrAuxUnavailable) {
			t.Fatalf("%d 回目の間隔 %ds のあと: ErrAuxUnavailable のはずが %v", i+1, sec, err)
		}
		if count() != before+1 {
			t.Fatalf("%d 回目の間隔 %ds のあとに試していない（%d → %d）", i+1, sec, before, count())
		}
	}

	// 成功したら5秒に戻る。
	setFail(false)
	clk.Advance(30 * time.Second)
	if _, err := a.GetSceneList(); err != nil {
		t.Fatalf("成功するはずが %v", err)
	}
	setFail(true)
	a.SetMainConnected(false)
	s.waitClosed(t, "切り離し")
	before := count()
	a.SetMainConnected(true) // 成功のあとなので待たずに試す（そして失敗する）
	waitFor(t, "成功のあとはすぐ試す", func() bool { return count() == before+1 })
	waitFor(t, "失敗が記録される", func() bool {
		_, err := a.GetSceneList()
		return errors.Is(err, ErrAuxUnavailable) && count() == before+1
	})
	clk.Advance(5*time.Second - time.Millisecond)
	_, _ = a.GetSceneList()
	if count() != before+1 {
		t.Fatal("成功のあとの最初の失敗は5秒あけるはず（5秒の手前で試した）")
	}
	clk.Advance(time.Millisecond)
	_, _ = a.GetSceneList()
	if count() != before+2 {
		t.Fatalf("5秒のあとに試していない（%d）", count()-before)
	}
}

func TestAuxCloseLeavesNothing(t *testing.T) {
	s := newAuxServer(t)
	s.setHandler(func(auxReq) auxReply { return auxReply{Skip: true} })
	// 基準は相手のサーバーを立てたあと
	runtime.GC()
	base := runtime.NumGoroutine()

	a := NewAux(s.config(t), slog.New(slog.DiscardHandler))
	a.reqTimeout = 5 * time.Second
	a.respTimeout = 500 * time.Millisecond
	a.SetMainConnected(true)
	waitFor(t, "開く", func() bool { return s.accepted() == 1 })

	// 応答待ちの要求を2本置いたまま Close する
	errs := make(chan error, 2)
	for range 2 {
		go func() { _, err := a.GetSceneList(); errs <- err }()
	}
	waitFor(t, "1本目が届く", func() bool { return len(s.requests()) == 1 })
	a.Close()
	for range 2 {
		select {
		case err := <-errs:
			if !errors.Is(err, ErrAuxClosed) {
				t.Errorf("Close で待っていた要求: ErrAuxClosed のはずが %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Close のあとも要求が戻らない")
		}
	}
	s.waitClosed(t, "Close のあと")
	if _, err := a.GetSceneList(); !errors.Is(err, ErrAuxClosed) {
		t.Fatalf("Close のあとの要求: ErrAuxClosed のはずが %v", err)
	}
	a.SetMainConnected(true) // Close のあとは何も起きない
	a.Close()                // 2回目も安全

	waitFor(t, "goroutine が基準まで戻る", func() bool { return runtime.NumGoroutine() <= base })
	if n := s.accepted(); n != 1 {
		t.Fatalf("Close のあとに接続が増えた: %d", n)
	}
}

func TestAuxCloseDuringOpenLeavesNothing(t *testing.T) {
	s := newSilentServer(t) // Hello を送らない
	runtime.GC()
	base := runtime.NumGoroutine()

	a := NewAux(s.config(t), slog.New(slog.DiscardHandler))
	a.reqTimeout = 2 * time.Second
	a.respTimeout = 300 * time.Millisecond
	a.SetMainConnected(true)
	waitFor(t, "接続が受けられる", func() bool { return s.accepted() == 1 })

	errc := make(chan error, 1)
	go func() { _, err := a.GetSceneList(); errc <- err }()
	time.Sleep(50 * time.Millisecond)
	a.Close()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("開けないのに成功した")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close のあとも要求が戻らない")
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("開きかけの接続が閉じられていない")
	}
	// goobs.New は認証の前に失敗すると、中の goroutine（handleOpcodes）を1本残す（goobs の制約。
	// コアの spec 13章）。Client が返らないので外から止められない。それ以外は残らない。
	waitFor(t, "goroutine が基準 + goobs の1本まで戻る", func() bool { return runtime.NumGoroutine() <= base+1 })
}

func TestAuxConcurrentUseThenClose(t *testing.T) {
	s := newAuxServer(t)
	a := NewAux(s.config(t), slog.New(slog.DiscardHandler))
	a.reqTimeout = time.Second
	a.respTimeout = time.Second

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = a.GetSceneList()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			a.SetMainConnected(i%2 == 0)
			time.Sleep(5 * time.Millisecond)
		}
	}()
	time.Sleep(600 * time.Millisecond)
	a.Close()
	close(stop)
	wg.Wait()
	// 開きかけを取り消すたびに、goobs が内部の goroutine を1本残す（既知の制約）ので、
	// goroutine の総数は見ない。代わりに、相手が受けた接続がすべて閉じられたことを確かめる。
	n := s.accepted()
	for i := 0; i < n; i++ {
		s.waitClosed(t, "受けた接続 "+strconv.Itoa(i+1)+" 本目")
	}
}
