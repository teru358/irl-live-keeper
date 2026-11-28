package web

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func idleTicker(time.Duration) (<-chan time.Time, func()) { return make(chan time.Time), func() {} }

func clientCount(e *Events) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.clients)
}

// 接続と切断を繰り返しても、持っている接続が増えない（終わった接続を map に残さない）。
func TestSourceSSEDropsEndedConnections(t *testing.T) {
	e := &Events{clients: map[*eventClient]bool{}, sourceTexts: map[string]string{"demo": "x"}, ticker: idleTicker}
	for i := 0; i < 50; i++ {
		r := httptest.NewRequest("GET", "/source/demo/events", nil)
		ctx, cancel := context.WithCancel(r.Context())
		done := make(chan struct{})
		go func() {
			e.serveSource(&deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}, r.WithContext(ctx), "demo")
			close(done)
		}()
		for clientCount(e) == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
		<-done
	}
	if n := clientCount(e); n != 0 {
		t.Fatalf("終わった接続が %d 件残っている", n)
	}
}

// Close は動いているソースの接続の終わりを待ち、そのあと map は空になる。
func TestEventsCloseWaitsAndEmptiesSourceClients(t *testing.T) {
	e := &Events{clients: map[*eventClient]bool{}, sourceTexts: map[string]string{"demo": "x"}, done: make(chan struct{}), finished: make(chan struct{}), ticker: idleTicker}
	close(e.finished)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.serveSource(&deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}, httptest.NewRequest("GET", "/source/demo/events", nil), "demo")
		}()
	}
	go e.Close()
	e.Close()
	wg.Wait()
	if n := clientCount(e); n != 0 {
		t.Fatalf("Close のあとに %d 件残っている", n)
	}
}
