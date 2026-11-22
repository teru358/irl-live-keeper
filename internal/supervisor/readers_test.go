package supervisor

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestTracker() (*readerTracker, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)}
	tr := newReaderTracker()
	tr.now = clk.Now
	return tr, clk
}

func slowPaths(tr *readerTracker) []string {
	var out []string
	for _, r := range tr.Slow() {
		out = append(out, r.Path)
	}
	return out
}

// feed は行を流し、同じ時刻で reader is too slow を n 回出す。
func slowN(tr *readerTracker, who string, n int) {
	for i := 0; i < n; i++ {
		tr.Observe(who + " reader is too slow, discarding 81 frames")
	}
}

func TestReaderTrackerLineForms(t *testing.T) {
	tests := []struct {
		name  string
		lines []string // 最後の1行までを流したあとの
		slow  []string
	}{
		{"RTSP の session", []string{
			"[RTSP] [session 2610e2a2] is reading from path 'live/main', with TCP, 2 tracks (H264, MPEG-4 Audio)",
		}, []string{"live/main"}},
		{"RTMP の conn", []string{
			"[RTMP] [conn 192.168.1.11:41388] is reading from path 'live/sub', 2 tracks (H264, MPEG-4 Audio)",
		}, []string{"live/sub"}},
		{"SRT の conn", []string{
			"[SRT] [conn 127.0.0.1:5555] is reading from path 'live/cam', 1 track (H264)",
		}, []string{"live/cam"}},
		{"1段の path", []string{
			"[RTSP] [session abc] is reading from path 'main', with TCP, 1 track (H264)",
		}, []string{"main"}},
		{"覚えていない id は数えない", nil, nil},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			tr, _ := newTestTracker()
			who := ""
			for _, l := range c.lines {
				tr.Observe(l)
				who = l[:strings.Index(l, " is reading")]
			}
			if who == "" {
				who = "[RTSP] [session unknown]"
			}
			slowN(tr, who, 3)
			if got := slowPaths(tr); !reflect.DeepEqual(got, c.slow) {
				t.Errorf("slow = %v, want %v", got, c.slow)
			}
		})
	}
}

func TestReaderTrackerDestroyedForgets(t *testing.T) {
	for _, end := range []string{
		"[RTSP] [session s1] destroyed: terminated",
		"[RTSP] [session s1] destroyed",
	} {
		tr, _ := newTestTracker()
		tr.Observe("[RTSP] [session s1] is reading from path 'live/main', with TCP, 1 track (H264)")
		tr.Observe(end)
		slowN(tr, "[RTSP] [session s1]", 5)
		if got := slowPaths(tr); got != nil {
			t.Errorf("%q のあとに数えた: %v", end, got)
		}
	}
	tr, _ := newTestTracker()
	tr.Observe("[RTMP] [conn 1.2.3.4:5] is reading from path 'live/main', 1 track (H264)")
	tr.Observe("[RTMP] [conn 1.2.3.4:5] closed: EOF")
	slowN(tr, "[RTMP] [conn 1.2.3.4:5]", 5)
	if got := slowPaths(tr); got != nil {
		t.Errorf("closed のあとに数えた: %v", got)
	}
}

func TestReaderTrackerIDReuse(t *testing.T) {
	tr, _ := newTestTracker()
	tr.Observe("[RTMP] [conn 1.2.3.4:5] is reading from path 'live/a', 1 track (H264)")
	tr.Observe("[RTMP] [conn 1.2.3.4:5] closed: EOF")
	tr.Observe("[RTMP] [conn 1.2.3.4:5] is reading from path 'live/b', 1 track (H264)")
	slowN(tr, "[RTMP] [conn 1.2.3.4:5]", 3)
	if got := slowPaths(tr); !reflect.DeepEqual(got, []string{"live/b"}) {
		t.Errorf("slow = %v, want [live/b]", got)
	}
}

func TestReaderTrackerPathsSeparate(t *testing.T) {
	tr, _ := newTestTracker()
	tr.Observe("[RTSP] [session a] is reading from path 'live/main', with TCP, 1 track (H264)")
	tr.Observe("[RTSP] [session b] is reading from path 'live/sub', with TCP, 1 track (H264)")
	slowN(tr, "[RTSP] [session a]", 2)
	slowN(tr, "[RTSP] [session b]", 3)
	if got := slowPaths(tr); !reflect.DeepEqual(got, []string{"live/sub"}) {
		t.Errorf("slow = %v, want [live/sub]", got)
	}
}

func TestReaderTrackerWindow(t *testing.T) {
	const who = "[RTSP] [session a]"
	setup := func() (*readerTracker, *fakeClock) {
		tr, clk := newTestTracker()
		tr.Observe(who + " is reading from path 'live/main', with TCP, 1 track (H264)")
		return tr, clk
	}
	t.Run("2回では出ない", func(t *testing.T) {
		tr, _ := setup()
		slowN(tr, who, 2)
		if got := tr.Slow(); got != nil {
			t.Errorf("slow = %v", got)
		}
	})
	t.Run("3回で出る、回数を持つ", func(t *testing.T) {
		tr, _ := setup()
		slowN(tr, who, 3)
		if got := tr.Slow(); !reflect.DeepEqual(got, []SlowReader{{Path: "live/main", Count: 3}}) {
			t.Errorf("slow = %v", got)
		}
	})
	t.Run("窓の外の回は数えない（30秒ちょうどは外）", func(t *testing.T) {
		tr, clk := setup()
		slowN(tr, who, 2)
		clk.Add(30 * time.Second) // ちょうど窓の端。前の2回は外
		slowN(tr, who, 1)
		if got := tr.Slow(); got != nil {
			t.Errorf("slow = %v", got)
		}
	})
	t.Run("窓の内（29秒）なら数える", func(t *testing.T) {
		tr, clk := setup()
		slowN(tr, who, 2)
		clk.Add(29 * time.Second)
		slowN(tr, who, 1)
		if got := tr.Slow(); len(got) != 1 {
			t.Errorf("slow = %v", got)
		}
	})
	t.Run("最後から60秒で解ける（59秒はまだ、60秒で解ける）", func(t *testing.T) {
		tr, clk := setup()
		slowN(tr, who, 3)
		clk.Add(59 * time.Second)
		if got := tr.Slow(); len(got) != 1 {
			t.Errorf("59秒: slow = %v", got)
		}
		clk.Add(1 * time.Second)
		if got := tr.Slow(); got != nil {
			t.Errorf("60秒: slow = %v", got)
		}
	})
	t.Run("続けて出ている間は解けない", func(t *testing.T) {
		tr, clk := setup()
		slowN(tr, who, 3)
		for i := 0; i < 5; i++ {
			clk.Add(40 * time.Second)
			slowN(tr, who, 1)
			if got := tr.Slow(); len(got) != 1 {
				t.Fatalf("%d 回目: slow = %v", i, got)
			}
		}
	})
	t.Run("解けたあとは、また3回で出る", func(t *testing.T) {
		tr, clk := setup()
		slowN(tr, who, 3)
		clk.Add(61 * time.Second)
		if got := tr.Slow(); got != nil {
			t.Fatalf("slow = %v", got)
		}
		slowN(tr, who, 2)
		if got := tr.Slow(); got != nil {
			t.Errorf("2回で出た: %v", got)
		}
		slowN(tr, who, 1)
		if got := tr.Slow(); len(got) != 1 {
			t.Errorf("3回で出ない: %v", got)
		}
	})
}

func TestReaderTrackerResetForgetsAll(t *testing.T) {
	tr, _ := newTestTracker()
	tr.Observe("[RTSP] [session a] is reading from path 'live/main', with TCP, 1 track (H264)")
	slowN(tr, "[RTSP] [session a]", 3)
	tr.Reset()
	if got := tr.Slow(); got != nil {
		t.Errorf("Reset のあと slow = %v", got)
	}
	// 読み手も忘れている（起動し直したあとの古い id は数えない）
	slowN(tr, "[RTSP] [session a]", 3)
	if got := tr.Slow(); got != nil {
		t.Errorf("Reset のあと、覚えていない id を数えた: %v", got)
	}
}

func TestReaderTrackerNilSafe(t *testing.T) {
	var tr *readerTracker
	tr.Observe("[RTSP] [session a] reader is too slow, discarding 1 frame")
	if tr.Slow() != nil {
		t.Error("nil の Slow")
	}
}

func TestLineWriterFeedsReaders(t *testing.T) {
	lw, buf, _ := newTestLW("SECRET")
	tr, _ := newTestTracker()
	lw.readers = tr
	_, _ = lw.Write([]byte("2026/11/22 23:24:50 INF [RTSP] [session 2610e2a2] is reading from path 'live/main', with TCP, 2 tracks (H264, MPEG-4 Audio)\n"))
	for i := 0; i < 3; i++ {
		_, _ = lw.Write([]byte("2026/11/22 23:24:53 WAR [RTSP] [session 2610e2a2] reader is too slow, discarding 81 frames\n"))
	}
	if got := slowPaths(tr); !reflect.DeepEqual(got, []string{"live/main"}) {
		t.Errorf("slow = %v", got)
	}
	// MediaMTX の WAR の行は今までどおりそのまま流す
	if n := strings.Count(buf.String(), "level=WARN"); n != 3 {
		t.Errorf("WARN の行 = %d, want 3\n%s", n, buf.String())
	}
}

// 偽の MediaMTX が実物と同じ3行を出し、Supervisor を通して path が「追いつかない」になり、
// 起動し直しで忘れる。
func TestSupervisorSlowReadersEndToEnd(t *testing.T) {
	s, buf := newSup(t, "readers")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sleep = func(ctx context.Context, d time.Duration) error { <-ctx.Done(); return ctx.Err() }
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := s.SlowReaders(); len(got) == 1 && got[0].Path == "live/main" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("SlowReaders = %v\n%s", s.SlowReaders(), buf.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(buf.String(), "reader is too slow, discarding 81 frames") {
		t.Errorf("WAR の行が流れていない:\n%s", buf.String())
	}
	cancel()
	<-done
	if got := s.SlowReaders(); got != nil {
		t.Errorf("終了のあと SlowReaders = %v", got)
	}
}

func TestReaderTrackerCapsReaders(t *testing.T) {
	tr, _ := newTestTracker()
	for i := 0; i < maxReaders*3; i++ {
		tr.Observe("[RTMP] [conn 10.0.0." + strconv.Itoa(i) + ":1] is reading from path 'live/x', 1 track (H264)")
	}
	tr.mu.Lock()
	n, o := len(tr.readers), len(tr.order)
	tr.mu.Unlock()
	if n != maxReaders || o != maxReaders {
		t.Errorf("readers = %d, order = %d, want %d", n, o, maxReaders)
	}
	// 古いものから忘れ、新しいものは残る
	slowN(tr, "[RTMP] [conn 10.0.0.0:1]", 3)
	if got := tr.Slow(); got != nil {
		t.Errorf("忘れたはずの古い読み手を数えた: %v", got)
	}
	slowN(tr, "[RTMP] [conn 10.0.2.255:1]", 0)
	last := "[RTMP] [conn 10.0.0." + strconv.Itoa(maxReaders*3-1) + ":1]"
	slowN(tr, last, 3)
	if got := tr.Slow(); len(got) != 1 {
		t.Errorf("新しい読み手を数えない: %v", got)
	}
}

func TestReaderTrackerSameKeyReplaces(t *testing.T) {
	tr, _ := newTestTracker()
	const who = "[RTSP] [session a]"
	tr.Observe(who + " is reading from path 'live/a', with TCP, 1 track (H264)")
	tr.Observe(who + " is reading from path 'live/b', with TCP, 1 track (H264)") // 終わりの行なしで使い回し
	slowN(tr, who, 3)
	if got := slowPaths(tr); !reflect.DeepEqual(got, []string{"live/b"}) {
		t.Errorf("slow = %v, want [live/b]", got)
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.order) != 1 {
		t.Errorf("order = %v", tr.order)
	}
}

// パスワードが path の一部と同じでも、追跡は伏せる前の行を読むので path が合う。ログには出ない。
func TestLineWriterReadersSeeUnmaskedPath(t *testing.T) {
	lw, buf, _ := newTestLW("main")
	tr, _ := newTestTracker()
	lw.readers = tr
	_, _ = lw.Write([]byte("INF [RTSP] [session s1] is reading from path 'live/main', with TCP, 1 track (H264)\n"))
	for i := 0; i < 3; i++ {
		_, _ = lw.Write([]byte("WAR [RTSP] [session s1] reader is too slow, discarding 81 frames\n"))
	}
	if got := slowPaths(tr); !reflect.DeepEqual(got, []string{"live/main"}) {
		t.Errorf("slow = %v", got)
	}
	if strings.Contains(buf.String(), "live/main") || !strings.Contains(buf.String(), "live/***") {
		t.Errorf("ログのパスワードが伏せられていない:\n%s", buf.String())
	}
}
