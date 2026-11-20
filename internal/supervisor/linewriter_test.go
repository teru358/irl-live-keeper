package supervisor

import (
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

var logLineRe = regexp.MustCompile(`level=(\w+) msg="?(.*?)"?$`)

// parseOut は TextHandler の出力から、(水準, msg) の列を取り出す。時刻と component は見ない。
func parseOut(t *testing.T, out string) [][2]string {
	t.Helper()
	var got [][2]string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l == "" {
			continue
		}
		m := logLineRe.FindStringSubmatch(l)
		if m == nil {
			t.Fatalf("解釈できない行: %q", l)
		}
		got = append(got, [2]string{m[1], strings.ReplaceAll(m[2], `\"`, `"`)})
	}
	return got
}

func newTestLW(secrets ...string) (*lineWriter, *syncBuf, *time.Time) {
	buf := &syncBuf{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
	lw := newLineWriter(log, secrets...)
	now := time.Date(2026, 11, 20, 17, 0, 0, 0, time.UTC)
	lw.now = func() time.Time { return now }
	return lw, buf, &now
}

func TestLineWriterLevels(t *testing.T) {
	tests := []struct {
		line, level string
		lastErr     string
	}{
		{"2026/11/20 17:05:10 ERR [RTMP] failed", "ERROR", "2026/11/20 17:05:10 ERR [RTMP] failed"},
		{"2026/11/20 17:05:10 WAR [RTMP] slow", "WARN", ""},
		{"2026/11/20 17:05:10 INF [RTMP] ok", "INFO", ""},
		{"2026/11/20 17:05:10 DEB [RTMP] detail", "INFO", ""},
		{"2026/11/20 17:05:10 INF [API] ERR in the middle", "INFO", ""},
		{"2026/11/20 17:05:10 INF listener WAR word", "INFO", ""},
		{"ERR no date prefix", "ERROR", "ERR no date prefix"},
		{"something without level", "INFO", ""},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			lw, buf, _ := newTestLW()
			lw.Write([]byte(tt.line + "\n"))
			got := parseOut(t, buf.String())
			want := [][2]string{{tt.level, tt.line}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
			if lw.lastErrorLine() != tt.lastErr {
				t.Errorf("lastErr = %q, want %q", lw.lastErrorLine(), tt.lastErr)
			}
		})
	}
}

const (
	opened1  = "2026/11/20 17:05:10 INF [RTSP] [conn 127.0.0.1:59805] opened"
	closed1  = "2026/11/20 17:05:10 INF [RTSP] [conn 127.0.0.1:59805] closed: no stream is available on path 'live'"
	opened2  = "2026/11/20 17:05:11 INF [RTSP] [conn 127.0.0.1:59282] opened"
	created  = "2026/11/20 17:05:11 INF [RTSP] [session f8764e6f] created by 127.0.0.1:59282"
	reading  = "2026/11/20 17:05:11 INF [RTSP] [session f8764e6f] is reading from path 'live', 2 tracks"
	closed2  = "2026/11/20 17:05:12 INF [RTSP] [conn 127.0.0.1:59282] closed: terminated"
	openedV6 = "2026/11/20 17:05:10 INF [RTSP] [conn [::1]:59805] opened"
	closedV6 = "2026/11/20 17:05:10 INF [RTSP] [conn [::1]:59805] closed: no stream is available on path 'live'"
	openedR  = "2026/11/20 17:05:10 INF [RTSP] [conn 192.168.1.5:59805] opened"
	closedR  = "2026/11/20 17:05:10 INF [RTSP] [conn 192.168.1.5:59805] closed: no stream is available on path 'live'"
)

type step struct {
	advance time.Duration
	line    string
}

func TestLineWriterHidesOBSReread(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
		flush bool
		want  []string
	}{
		{"読み直しは両方書かない", []step{{0, opened1}, {0, closed1}}, true, nil},
		{"次の行が別なら opened を先に書く", []step{{0, opened1}, {0, "2026/11/20 17:05:10 INF [RTSP] [conn 127.0.0.1:59805] closed: terminated"}}, true,
			[]string{opened1, "2026/11/20 17:05:10 INF [RTSP] [conn 127.0.0.1:59805] closed: terminated"}},
		{"session の行で opened を書き出す", []step{{0, opened2}, {0, created}, {0, reading}}, true, []string{opened2, created, reading}},
		{"別の接続の行は持ったまま", []step{{0, opened1}, {0, opened2}, {0, closed1}, {0, created}, {0, reading}, {0, closed2}}, true,
			[]string{opened2, created, reading, closed2}},
		{"2秒たったら次の行のときに書く", []step{{0, opened1}, {3 * time.Second, "2026/11/20 17:05:13 INF other"}}, false,
			[]string{opened1, "2026/11/20 17:05:13 INF other"}},
		{"2秒たったあとの closed は書く", []step{{0, opened1}, {3 * time.Second, closed1}}, true, []string{opened1, closed1}},
		{"終了時に持っている行を書く", []step{{0, opened1}}, true, []string{opened1}},
		{"::1 は隠さない", []step{{0, openedV6}, {0, closedV6}}, true, []string{openedV6, closedV6}},
		{"loopback 以外は隠さない", []step{{0, openedR}, {0, closedR}}, true, []string{openedR, closedR}},
		{"opened だけで他の行がなければ持つだけ", []step{{0, opened1}, {time.Second, "2026/11/20 17:05:11 INF other"}}, false, []string{"2026/11/20 17:05:11 INF other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lw, buf, now := newTestLW()
			for _, s := range tt.steps {
				*now = now.Add(s.advance)
				lw.Write([]byte(s.line + "\n"))
			}
			if tt.flush {
				lw.flush()
			}
			var got []string
			for _, g := range parseOut(t, buf.String()) {
				got = append(got, g[1])
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got\n%v\nwant\n%v", got, tt.want)
			}
		})
	}
}

// 持っている opened の行にも秘密の置き換えは効く。
func TestLineWriterRedactsHeldLine(t *testing.T) {
	lw, buf, _ := newTestLW("secretsecret")
	lw.Write([]byte("2026/11/20 17:05:10 INF [RTSP] [conn 127.0.0.1:1] opened\n"))
	lw.Write([]byte("2026/11/20 17:05:10 INF [RTSP] [session a] created by 127.0.0.1:1 secretsecret\n"))
	out := buf.String()
	if strings.Contains(out, "secretsecret") || !strings.Contains(out, "***") {
		t.Errorf("置き換えが効いていない:\n%s", out)
	}
}
