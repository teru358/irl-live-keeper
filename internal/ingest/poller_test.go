package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const fixtureDir = "testdata/mediamtx-v1.21.4"

// loadFixture は録った実物の応答をそのまま読む。
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// derive は fixture を JSON として読み直し、inboundBytes と source.id だけを差し替えた写しを返す。
// id が空なら source.id は変えない。形は実物のまま保たれる。
func derive(t *testing.T, name string, bytes int64, id string) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(loadFixture(t, name), &m); err != nil {
		t.Fatal(err)
	}
	m["inboundBytes"] = bytes
	if id != "" {
		src, ok := m["source"].(map[string]any)
		if !ok {
			t.Fatalf("%s に source がない", name)
		}
		src["id"] = id
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// reply は path の API の1回分の応答。
type reply struct {
	status int
	body   []byte
	delay  time.Duration
}

func ok(body []byte) reply { return reply{status: 200, body: body} }

// fakeServer は paths/get を順番に、srt/conns/list を固定で返す。
type fakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	paths    []reply
	conns    reply
	pathReqs int
	connReqs int
	auths    []string
}

func newFake(t *testing.T, paths []reply, conns reply) *fakeServer {
	f := &fakeServer{paths: paths, conns: conns}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		user, pass, _ := r.BasicAuth()
		f.auths = append(f.auths, user+":"+pass)
		var rep reply
		switch r.URL.Path {
		case "/v3/paths/get/live":
			if f.pathReqs < len(f.paths) {
				rep = f.paths[f.pathReqs]
			} else {
				rep = reply{status: 500}
			}
			f.pathReqs++
		case "/v3/srt/conns/list":
			rep = f.conns
			f.connReqs++
		default:
			rep = reply{status: 404}
		}
		f.mu.Unlock()
		time.Sleep(rep.delay)
		if rep.status == 0 {
			rep.status = 200
		}
		w.WriteHeader(rep.status)
		w.Write(rep.body)
	}))
	t.Cleanup(f.Close)
	return f
}

var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func at(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }

func poll(f *fakeServer, p *Poller, secs ...int) []Sample {
	var out []Sample
	for _, s := range secs {
		out = append(out, p.Poll(nil2ctx(), at(s)))
	}
	return out
}

func newP(f *fakeServer) *Poller {
	return NewPoller(f.URL, "api", "secret", "live", f.Client())
}

func wantKbps(t *testing.T, got []Sample, want ...float64) {
	t.Helper()
	for i, w := range want {
		if got[i].Kbps != w {
			t.Errorf("t=%d: Kbps=%v, want %v (Err=%v)", i, got[i].Kbps, w, got[i].Err)
		}
	}
}

const (
	idA = "754c24d1-d4b6-4ccb-9959-5fff1f0896ba" // b1 の source.id
	idB = "11111111-2222-3333-4444-555555555555"
)

// P1: 起動後の最初は基準を取るだけ。b1（RTMP）の inboundBytes を 0 と 87,500 に差し替えた写し。
func TestPollerP1(t *testing.T) {
	f := newFake(t, []reply{
		ok(derive(t, "b1_rtmp_paths_get.json", 0, "")),
		ok(derive(t, "b1_rtmp_paths_get.json", 87500, "")),
	}, reply{})
	got := poll(f, newP(f), 0, 1)
	wantKbps(t, got, 0, 700)
	if got[0].Interval != 0 || got[1].Interval != time.Second {
		t.Errorf("Interval = %v, %v", got[0].Interval, got[1].Interval)
	}
	if !got[1].Online || got[1].Protocol != "rtmp" || got[1].Err != nil {
		t.Errorf("sample = %+v", got[1])
	}
}

// P2: 失敗の回は基準を変えず、次は2秒分を2秒で割る。
func TestPollerP2(t *testing.T) {
	f := newFake(t, []reply{
		ok(derive(t, "b1_rtmp_paths_get.json", 0, "")),
		{status: 500, body: loadFixture(t, "a2_paths_get_unknown.json")},
		ok(derive(t, "b1_rtmp_paths_get.json", 175000, "")),
	}, reply{})
	got := poll(f, newP(f), 0, 1, 2)
	wantKbps(t, got, 0, 0, 700)
	if got[1].Err == nil || got[1].Online || got[1].Protocol != "" {
		t.Errorf("失敗の回: %+v", got[1])
	}
	if got[2].Interval != time.Second {
		t.Errorf("Interval = %v", got[2].Interval)
	}
}

// P3: 識別子が変わった回は 0 で、そこを新しい基準にする。
func TestPollerP3(t *testing.T) {
	f := newFake(t, []reply{
		ok(derive(t, "b1_rtmp_paths_get.json", 500000, idA)),
		ok(derive(t, "b1_rtmp_paths_get.json", 600000, idB)),
		ok(derive(t, "b1_rtmp_paths_get.json", 687500, idB)),
	}, reply{})
	wantKbps(t, poll(f, newP(f), 0, 1, 2), 0, 0, 700)
}

// P4: inboundBytes が減ったら 0 で再基準。
func TestPollerP4(t *testing.T) {
	f := newFake(t, []reply{
		ok(derive(t, "b1_rtmp_paths_get.json", 500000, "")),
		ok(derive(t, "b1_rtmp_paths_get.json", 20000, "")),
		ok(derive(t, "b1_rtmp_paths_get.json", 107500, "")),
	}, reply{})
	wantKbps(t, poll(f, newP(f), 0, 1, 2), 0, 0, 700)
}

// 識別子なし（offline の a）から識別子あり（b1）への変化は再基準。
func TestPollerNullToPresentRebases(t *testing.T) {
	f := newFake(t, []reply{
		ok(loadFixture(t, "a_paths_get_offline.json")),
		ok(derive(t, "b1_rtmp_paths_get.json", 187791, "")),
		ok(derive(t, "b1_rtmp_paths_get.json", 187791+87500, "")),
	}, reply{})
	got := poll(f, newP(f), 0, 1, 2)
	wantKbps(t, got, 0, 0, 700)
	if got[0].Online || !got[1].Online {
		t.Errorf("Online: %v %v", got[0].Online, got[1].Online)
	}
}

// 実物の b1 → b2 は差分から求めた値になる。
func TestPollerRecordedRTMP(t *testing.T) {
	f := newFake(t, []reply{
		ok(loadFixture(t, "b1_rtmp_paths_get.json")),
		ok(loadFixture(t, "b2_rtmp_paths_get_1s_later.json")),
	}, reply{})
	got := poll(f, newP(f), 0, 1)
	var b1, b2 struct {
		InboundBytes int64 `json:"inboundBytes"`
	}
	json.Unmarshal(loadFixture(t, "b1_rtmp_paths_get.json"), &b1)
	json.Unmarshal(loadFixture(t, "b2_rtmp_paths_get_1s_later.json"), &b2)
	want := float64(b2.InboundBytes-b1.InboundBytes) * 8 / 1000
	if got[1].Kbps != want || want <= 0 {
		t.Errorf("Kbps=%v want %v", got[1].Kbps, want)
	}
}

// offline の応答は Online false、Kbps 0。
func TestPollerOffline(t *testing.T) {
	f := newFake(t, []reply{
		ok(loadFixture(t, "a_paths_get_offline.json")),
		ok(loadFixture(t, "a_paths_get_offline.json")),
	}, reply{})
	got := poll(f, newP(f), 0, 1)
	for i, s := range got {
		if s.Online || s.Kbps != 0 || s.Err != nil || s.Protocol != "" || s.SRT != nil {
			t.Errorf("sample %d = %+v", i, s)
		}
	}
}

func TestPollerFailures(t *testing.T) {
	cases := map[string]reply{
		"404":       {status: 404, body: loadFixture(t, "a2_paths_get_unknown.json")},
		"500":       {status: 500},
		"timeout":   {delay: 800 * time.Millisecond, body: loadFixture(t, "a_paths_get_offline.json")},
		"malformed": ok([]byte(`{"online": tru`)),
	}
	for name, rep := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t, []reply{rep}, reply{})
			s := newP(f).Poll(nil2ctx(), at(0))
			if s.Err == nil || s.Kbps != 0 || s.Online || s.Protocol != "" {
				t.Errorf("sample = %+v", s)
			}
		})
	}
}

// 失敗の Err に応答の本文を含めない（RTMP の conns API はパスワードを返す）。
func TestPollerErrorHasNoBody(t *testing.T) {
	f := newFake(t, []reply{{status: 500, body: []byte("SECRET-BODY")}}, reply{})
	s := newP(f).Poll(nil2ctx(), at(0))
	if s.Err == nil || contains(s.Err.Error(), "SECRET-BODY") {
		t.Errorf("Err = %v", s.Err)
	}
	f2 := newFake(t, []reply{ok([]byte(`SECRET-BODY`))}, reply{})
	s = newP(f2).Poll(nil2ctx(), at(0))
	if s.Err == nil || contains(s.Err.Error(), "SECRET-BODY") {
		t.Errorf("Err = %v", s.Err)
	}
}

// SRT: e3 の統計を取り出す。e1 → e2 の1秒後で Kbps も出る。
func TestPollerSRT(t *testing.T) {
	f := newFake(t, []reply{
		ok(loadFixture(t, "e1_srt_paths_get.json")),
		ok(loadFixture(t, "e2_srt_paths_get_1s_later.json")),
	}, ok(loadFixture(t, "e3_srt_conns_list.json")))
	got := poll(f, newP(f), 0, 1)
	s := got[1]
	if s.Protocol != "srt" || !s.Online || s.Err != nil || s.Kbps <= 0 {
		t.Fatalf("sample = %+v", s)
	}
	if s.SRT == nil {
		t.Fatal("SRT is nil")
	}
	want := SRTStats{RTTMs: 0.0484145357443751, LossRate: 0, RecvMbps: 0.609234195018386}
	// LossRate は e3 の値をそのまま読んで確かめる
	var e3 struct {
		Items []struct {
			Rate float64 `json:"packetsReceivedLossRate"`
		} `json:"items"`
	}
	json.Unmarshal(loadFixture(t, "e3_srt_conns_list.json"), &e3)
	want.LossRate = e3.Items[0].Rate
	if *s.SRT != want {
		t.Errorf("SRT = %+v, want %+v", *s.SRT, want)
	}
}

// 接続一覧は source.id が一致するものを優先し、path と state で絞る。
func TestPollerSRTPicksPublisher(t *testing.T) {
	var m map[string]any
	json.Unmarshal(loadFixture(t, "e3_srt_conns_list.json"), &m)
	items := m["items"].([]any)
	mk := func(id, state, path string, rtt float64) any {
		c := map[string]any{}
		for k, v := range items[0].(map[string]any) {
			c[k] = v
		}
		c["id"], c["state"], c["path"], c["msRTT"] = id, state, path, rtt
		return c
	}
	e1id := "8ee482a9-2c79-4405-8007-88cf25da4df2"
	m["items"] = []any{
		mk("x1", "read", "live", 1),
		mk("x2", "publish", "other", 2),
		mk("x3", "publish", "live", 3),
		mk(e1id, "publish", "live", 4),
	}
	body, _ := json.Marshal(m)
	f := newFake(t, []reply{ok(loadFixture(t, "e1_srt_paths_get.json"))}, ok(body))
	s := newP(f).Poll(nil2ctx(), at(0))
	if s.SRT == nil || s.SRT.RTTMs != 4 {
		t.Errorf("SRT = %+v", s.SRT)
	}
	// id が一致するものがなければ publish かつ path 一致の先頭
	m["items"] = []any{mk("x1", "read", "live", 1), mk("x3", "publish", "live", 3)}
	body, _ = json.Marshal(m)
	f = newFake(t, []reply{ok(loadFixture(t, "e1_srt_paths_get.json"))}, ok(body))
	s = newP(f).Poll(nil2ctx(), at(0))
	if s.SRT == nil || s.SRT.RTTMs != 3 {
		t.Errorf("SRT = %+v", s.SRT)
	}
}

// 接続一覧だけの失敗は SRT nil で、Kbps と Err に影響しない。
func TestPollerSRTConnsFailureOnly(t *testing.T) {
	for name, conns := range map[string]reply{
		"500":       {status: 500},
		"malformed": ok([]byte("{")),
		"timeout":   {delay: 800 * time.Millisecond, body: loadFixture(t, "e3_srt_conns_list.json")},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t, []reply{
				ok(loadFixture(t, "e1_srt_paths_get.json")),
				ok(loadFixture(t, "e2_srt_paths_get_1s_later.json")),
			}, conns)
			got := poll(f, newP(f), 0, 1)
			if got[1].SRT != nil || got[1].Err != nil || got[1].Kbps <= 0 || got[1].Protocol != "srt" {
				t.Errorf("sample = %+v", got[1])
			}
		})
	}
}

// RTMP のときは接続一覧を読まない。
func TestPollerRTMPSkipsConns(t *testing.T) {
	f := newFake(t, []reply{ok(loadFixture(t, "b1_rtmp_paths_get.json"))}, reply{})
	newP(f).Poll(nil2ctx(), at(0))
	if f.connReqs != 0 {
		t.Errorf("conns requests = %d", f.connReqs)
	}
}

func TestPollerBasicAuth(t *testing.T) {
	f := newFake(t, []reply{ok(loadFixture(t, "e1_srt_paths_get.json"))}, ok(loadFixture(t, "e3_srt_conns_list.json")))
	newP(f).Poll(nil2ctx(), at(0))
	if len(f.auths) != 2 {
		t.Fatalf("requests = %d", len(f.auths))
	}
	for _, a := range f.auths {
		if a != "api:secret" {
			t.Errorf("auth = %q", a)
		}
	}
}

// 失敗の Interval は前回の Sample からのまま。
func TestPollerIntervalAcrossFailure(t *testing.T) {
	f := newFake(t, []reply{
		ok(loadFixture(t, "a_paths_get_offline.json")),
		{status: 500},
		ok(loadFixture(t, "a_paths_get_offline.json")),
	}, reply{})
	got := poll(f, newP(f), 0, 1, 3)
	if got[0].Interval != 0 || got[1].Interval != time.Second || got[2].Interval != 2*time.Second {
		t.Errorf("Intervals = %v %v %v", got[0].Interval, got[1].Interval, got[2].Interval)
	}
	if !got[2].At.Equal(at(3)) {
		t.Errorf("At = %v", got[2].At)
	}
}

func TestPollerRun(t *testing.T) {
	f := newFake(t, []reply{
		ok(loadFixture(t, "a_paths_get_offline.json")),
		ok(loadFixture(t, "a_paths_get_offline.json")),
	}, reply{})
	ctx, cancel := ctxCancel()
	out := make(chan Sample) // 受け手がいない状態でも止まれること
	done := make(chan struct{})
	go func() { newP(f).Run(ctx, 10*time.Millisecond, out); close(done) }()
	s := <-out
	if s.Interval != 0 {
		t.Errorf("first Interval = %v", s.Interval)
	}
	time.Sleep(50 * time.Millisecond) // 次の送信で詰まらせる
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run が止まらない")
	}
}

func nil2ctx() context.Context { return context.Background() }

func ctxCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
