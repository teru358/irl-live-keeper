package obs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// silentServer は WebSocket へのアップグレードは受け付けるが、Hello を送らない偽の OBS。
// 受けた接続ごとに、サーバー側の読み取りがエラーで戻ったとき（接続が閉じられたとき）に done へ流す。
type silentServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	n    int
	done chan struct{}
}

func newSilentServer(t *testing.T) *silentServer {
	t.Helper()
	s := &silentServer{done: make(chan struct{}, 1024)}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		s.mu.Lock()
		s.n++
		s.mu.Unlock()
		defer c.Close()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				s.done <- struct{}{}
				return
			}
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *silentServer) accepted() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func (s *silentServer) config(t *testing.T) Config {
	t.Helper()
	host, port, err := net.SplitHostPort(s.srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := strconv.Atoi(port)
	return Config{Host: host, Port: p}
}

func TestSessionClosesConnectionWhenHandshakeFails(t *testing.T) {
	s := newSilentServer(t)
	w := New(s.config(t), slog.New(slog.DiscardHandler))
	w.timeout = 200 * time.Millisecond

	if w.session(context.Background()) {
		t.Fatal("Hello が来ないのに接続できたことになっている")
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("応答待ちが切れたあとも、サーバー側の接続が閉じられていない")
	}
}

func TestSessionClosesConnectionWhenContextEnds(t *testing.T) {
	s := newSilentServer(t)
	w := New(s.config(t), slog.New(slog.DiscardHandler))
	// goobs.New は ctx では中断できず、応答待ちが切れるまで戻らない。
	// 接続はそれより前に、ctx が終わった時点で閉じられる。
	w.timeout = 2 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	ret := make(chan bool, 1)
	go func() { ret <- w.session(ctx) }()
	for i := 0; i < 200 && s.accepted() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if s.accepted() == 0 {
		t.Fatal("サーバーが接続を受けていない")
	}
	cancel()
	select {
	case <-s.done:
	case <-time.After(1 * time.Second):
		t.Fatal("ctx が終わっても、応答待ちが切れる前にサーバー側の接続が閉じられない")
	}
	select {
	case <-ret:
	case <-time.After(5 * time.Second):
		t.Fatal("応答待ちが切れても session が戻らない")
	}
}

func TestRunClosesEveryConnectionAfterRetries(t *testing.T) {
	s := newSilentServer(t)
	w := New(s.config(t), slog.New(slog.DiscardHandler))
	w.timeout = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	ret := make(chan struct{})
	go func() { w.Run(ctx); close(ret) }()
	// 1回目の失敗 + 1 秒の待ち + 2回目の失敗
	for i := 0; i < 300 && s.accepted() < 2; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	n := s.accepted()
	if n < 2 {
		t.Fatalf("再試行されていない: 接続は %d 回", n)
	}
	time.Sleep(300 * time.Millisecond) // 2回目の応答待ちが切れるまで
	cancel()
	select {
	case <-ret:
	case <-time.After(3 * time.Second):
		t.Fatal("ctx を取り消しても Run が戻らない")
	}
	n = s.accepted()
	for i := 0; i < n; i++ {
		select {
		case <-s.done:
		case <-time.After(3 * time.Second):
			t.Fatalf("接続 %d 本のうち %d 本しか閉じられていない", n, i)
		}
	}
}

// newPipe は net.Pipe の両端を返す。テストの後で両端を閉じる。
func newPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	return a, b
}

// requirePeerEOF は peer の Read が期限内に io.EOF で戻ることを確かめる。
// 閉じられていなければ Read は戻らず、期限切れで失敗する。
func requirePeerEOF(t *testing.T, peer net.Conn, what string) {
	t.Helper()
	ret := make(chan error, 1)
	go func() {
		_, err := peer.Read(make([]byte, 1))
		ret <- err
	}()
	select {
	case err := <-ret:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("%s: 相手の Read が io.EOF ではなく %v で戻った", what, err)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s: 接続が閉じられていない（相手の Read が戻らない）", what)
	}
}

func TestConnRecorderClosesConnAddedAfterClose(t *testing.T) {
	var rec connRecorder
	rec.close()
	c, peer := newPipe(t)
	rec.add(c)
	requirePeerEOF(t, peer, "close のあとに add した接続")
}

func TestConnRecorderAddAfterCloseReturnsErrClosed(t *testing.T) {
	var rec connRecorder
	rec.close()
	c, peer := newPipe(t)
	err := rec.add(c)
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("close のあとの add は net.ErrClosed を包んだエラーを返すべきだが、%v を返した", err)
	}
	requirePeerEOF(t, peer, "エラーを返した add の接続")
}

func TestConnRecorderClosesConnAddedBeforeClose(t *testing.T) {
	var rec connRecorder
	c, peer := newPipe(t)
	rec.add(c)
	rec.close()
	requirePeerEOF(t, peer, "close の前に add した接続")
}

func TestConnRecorderCloseTwiceDoesNotPanic(t *testing.T) {
	var rec connRecorder
	c, peer := newPipe(t)
	rec.add(c)
	rec.close()
	rec.close() // 2回目は何もしない（パニックしない）
	requirePeerEOF(t, peer, "close を2回呼んだあとの接続")
}

// TestConnRecorderAddAndCloseConcurrently は add と close を並行に呼ぶ。
// どちらが先でも、記録された接続はすべて閉じられていなければならない。
func TestConnRecorderAddAndCloseConcurrently(t *testing.T) {
	var rec connRecorder
	const n = 50
	conns := make([]net.Conn, n)
	peers := make([]net.Conn, n)
	for i := range n {
		conns[i], peers[i] = newPipe(t)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec.add(conns[i])
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		rec.close()
	}()
	close(start)
	wg.Wait()

	for i, p := range peers {
		requirePeerEOF(t, p, "並行に add した接続 "+strconv.Itoa(i))
	}
}
