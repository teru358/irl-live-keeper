package web

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 本物の HTTP サーバーで、WriteTimeout を過ぎても SSE が切れないこと（SSE の応答だけ書き込みの期限を外す）。
// ソケットを開くので、サンドボックスでは落ちる。
func TestEventsSurviveWriteTimeout(t *testing.T) {
	e := NewEvents(eventBackend{}, nil, func() any { return map[string]string{"state": "idle"} })
	defer e.Close()
	h := NewWithEvents(eventBackend{}, "secret-password", 8787, NewAuth(nil), e)
	srv := httptest.NewUnstartedServer(h)
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/events?topics=status", nil)
	req.SetBasicAuth("admin", "secret-password")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	// status は1秒ごとに来る。WriteTimeout（200ms）の何倍もの間、続けて受けられること
	got := make(chan int, 1)
	go func() {
		n := 0
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "event: status") {
				n++
				if n == 3 {
					break
				}
			}
		}
		got <- n
	}()
	select {
	case n := <-got:
		if n < 3 {
			t.Fatalf("SSE が途中で切れた（status を %d 件だけ受けた）", n)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("status を3件受けられない")
	}
}
