package logging

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestLogger(t *testing.T, secrets ...string) (*Logger, *bytes.Buffer, string, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: at(2026, 11, 21, 12, 0, 0)}
	var out bytes.Buffer
	dir := t.TempDir()
	return openLogger(t, dir, 7, secrets, clk, &out), &out, dir, clk
}

func TestAddSecretsEverywhere(t *testing.T) {
	lg, out, dir, clk := newTestLogger(t, "old-secret")
	ch, unsub := lg.Subscribe(10)
	defer unsub()
	lg.AddSecrets("", "new-secret")
	lg.Slog().Info("x", "a", "old-secret", "b", "new-secret")
	got := <-ch
	file := readFile(t, dir+"/"+fileName(clk.Now()))
	rec := lg.Recent(10, slog.LevelDebug)
	if len(rec) != 1 {
		t.Fatalf("recent = %d", len(rec))
	}
	for name, s := range map[string]string{"stdout": out.String(), "file": file, "sub": got.Text, "recent": rec[0].Text} {
		if strings.Contains(s, "old-secret") || strings.Contains(s, "new-secret") {
			t.Errorf("%s に生の値: %q", name, s)
		}
		if !strings.Contains(s, "a=***") || !strings.Contains(s, "b=***") {
			t.Errorf("%s が *** でない: %q", name, s)
		}
	}
}

func TestAddSecretsConcurrent(t *testing.T) {
	lg, _, _, _ := newTestLogger(t)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			lg.Slog().Info("line", "v", "s0")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			lg.AddSecrets(fmt.Sprintf("s%d", i))
		}
	}()
	wg.Wait()
	lg.Slog().Info("after", "v", "s0", "w", "s299")
	rec := lg.Recent(1, slog.LevelDebug)
	if strings.Contains(rec[0].Text, "s0") || strings.Contains(rec[0].Text, "s299") {
		t.Errorf("生の値: %q", rec[0].Text)
	}
}

func TestSubscribeDelivers(t *testing.T) {
	lg, _, _, clk := newTestLogger(t)
	ch, unsub := lg.Subscribe(4)
	defer unsub()
	lg.Slog().Warn("hello")
	got := <-ch
	if got.Level != slog.LevelWarn || !got.At.Equal(clk.Now()) {
		t.Errorf("got %+v", got)
	}
	if !strings.Contains(got.Text, "msg=hello") || strings.HasSuffix(got.Text, "\n") {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestSubscribeStuckDoesNotBlock(t *testing.T) {
	lg, _, _, _ := newTestLogger(t)
	_, unsubStuck := lg.Subscribe(1) // 読まない
	defer unsubStuck()
	ch, unsub := lg.Subscribe(2000)
	defer unsub()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			lg.Slog().Info("n", "i", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("書き込みが止まった")
	}
	if len(ch) != 1000 {
		t.Errorf("読む受け手に届いたのは %d", len(ch))
	}
}

func TestSubscribeUnsubscribeAndClose(t *testing.T) {
	lg, _, _, _ := newTestLogger(t)
	ch1, unsub1 := lg.Subscribe(2)
	ch2, _ := lg.Subscribe(2)
	unsub1()
	unsub1() // 2回目も安全
	if _, ok := <-ch1; ok {
		t.Error("解除のあともチャネルが開いている")
	}
	lg.Slog().Info("x")
	if len(ch2) != 1 {
		t.Errorf("残った受け手に届かない")
	}
	<-ch2
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-ch2; ok {
		t.Error("Close のあともチャネルが開いている")
	}
	unsub1()
	ch3, unsub3 := lg.Subscribe(1) // Close のあとの登録は、閉じたチャネルを返す
	if _, ok := <-ch3; ok {
		t.Error("Close のあとのチャネルが開いている")
	}
	unsub3()
}

func TestSubscribePanicsOnBadN(t *testing.T) {
	lg, _, _, _ := newTestLogger(t)
	for _, n := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("n=%d で panic しない", n)
				}
			}()
			lg.Subscribe(n)
		}()
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		raw  string
		want slog.Level
	}{
		{"time=2026-01-01T00:00:00.000+09:00 level=DEBUG msg=a\n", slog.LevelDebug},
		{"time=x level=INFO msg=a\n", slog.LevelInfo},
		{"time=x level=WARN msg=a\n", slog.LevelWarn},
		{"time=x level=ERROR msg=a\n", slog.LevelError},
		{"time=x level=WARN+2 msg=a\n", slog.LevelWarn + 2},
		{"time=x level=BOGUS msg=a\n", slog.LevelInfo},
		{"level=\n", slog.LevelInfo},
		{"レベルなし\n", slog.LevelInfo},
	}
	for _, c := range cases {
		if got := parseLevel(c.raw); got != c.want {
			t.Errorf("parseLevel(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestRecentLimit(t *testing.T) {
	lg, _, _, _ := newTestLogger(t)
	for i := 0; i < 2001; i++ {
		lg.Slog().Info("m", "i", i)
	}
	rec := lg.Recent(5000, slog.LevelDebug)
	if len(rec) != 2000 {
		t.Fatalf("len = %d", len(rec))
	}
	if !strings.HasSuffix(rec[0].Text, "i=1") || !strings.HasSuffix(rec[1999].Text, "i=2000") {
		t.Errorf("先頭 %q 末尾 %q", rec[0].Text, rec[1999].Text)
	}
	for i := 2001; i < 3000; i++ { // 詰め直しをまたいでも 2000 行
		lg.Slog().Info("m", "i", i)
	}
	rec = lg.Recent(5000, slog.LevelDebug)
	if len(rec) != 2000 || !strings.HasSuffix(rec[0].Text, "i=1000") || !strings.HasSuffix(rec[1999].Text, "i=2999") {
		t.Errorf("len=%d 先頭 %q", len(rec), rec[0].Text)
	}
}

func TestRecentTable(t *testing.T) {
	lg, _, _, _ := newTestLogger(t)
	l := lg.Slog()
	l.Info("a")
	l.Warn("b")
	l.Error("c")
	l.Info("d")
	l.Warn("e")
	msgs := func(ls []Line) string {
		var s []string
		for _, ln := range ls {
			i := strings.Index(ln.Text, "msg=")
			s = append(s, ln.Text[i+4:])
		}
		return strings.Join(s, ",")
	}
	cases := []struct {
		n    int
		min  slog.Level
		want string
	}{
		{0, slog.LevelDebug, ""},
		{-3, slog.LevelDebug, ""},
		{100, slog.LevelDebug, "a,b,c,d,e"},
		{100000, slog.LevelDebug, "a,b,c,d,e"},
		{2, slog.LevelDebug, "d,e"},
		{100, slog.LevelWarn, "b,c,e"},
		{2, slog.LevelWarn, "c,e"},
		{100, slog.LevelError, "c"},
		{100, slog.LevelError + 1, ""},
	}
	for _, c := range cases {
		got := lg.Recent(c.n, c.min)
		if got == nil {
			t.Errorf("Recent(%d,%v) が nil", c.n, c.min)
		}
		if m := msgs(got); m != c.want {
			t.Errorf("Recent(%d,%v) = %q, want %q", c.n, c.min, m, c.want)
		}
	}
}
