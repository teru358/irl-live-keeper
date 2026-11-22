package supervisor

import (
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

// 「OBS が読み切れていない」の判定の定数。
const (
	// slowWindow の間に slowThreshold 回以上 reader is too slow が出たら「追いつかない」。
	slowWindow    = 30 * time.Second
	slowThreshold = 3
	// slowRelease は、最後の reader is too slow からこれだけたったら「追いつかない」を解く。
	slowRelease = 60 * time.Second
)

var (
	// 読み手が path を読み始めた行（RTSP の session、RTMP・SRT の conn）。
	readingRe = regexp.MustCompile(`^\[(\w+)\] \[(session|conn) ([^\]]+)\] is reading from path '([^']*)'`)
	// 読み手が終わった行（RTSP の session は destroyed、conn は closed）。
	readerEndRe = regexp.MustCompile(`^\[(\w+)\] \[(session|conn) ([^\]]+)\] (?:destroyed|closed)\b`)
	// 読み手が追いつかず、MediaMTX がフレームを捨てた行。
	readerSlowRe = regexp.MustCompile(`^\[(\w+)\] \[(session|conn) ([^\]]+)\] reader is too slow, discarding (\d+) frames?\b`)
)

// SlowReader は、読み手（OBS）が追いつかない path。
type SlowReader struct {
	Path  string
	Count int // 直近の窓（30秒）の reader is too slow の回数
}

// pathSlow は path ごとの reader is too slow の記録。
type pathSlow struct {
	events []time.Time // 窓の中の時刻（古い順）
	last   time.Time
	slow   bool
}

// readerTracker は MediaMTX の行から、path ごとの「読み手が追いつかない」を数える。
// 読み手の id → path は「is reading from path」で覚え、destroyed / closed で忘れる。
type readerTracker struct {
	now func() time.Time // テストで差し替える

	mu      sync.Mutex
	readers map[string]string // 「プロトコル kind id」→ path
	paths   map[string]*pathSlow
}

func newReaderTracker() *readerTracker {
	return &readerTracker{now: time.Now, readers: map[string]string{}, paths: map[string]*pathSlow{}}
}

// Reset は覚えたものを全部忘れる（MediaMTX を起動し直したとき）。
func (t *readerTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.readers = map[string]string{}
	t.paths = map[string]*pathSlow{}
}

// Observe は MediaMTX の行（水準の語を除いた本文）を1行読む。
func (t *readerTracker) Observe(rest string) {
	if t == nil {
		return
	}
	if m := readingRe.FindStringSubmatch(rest); m != nil {
		t.mu.Lock()
		t.readers[m[1]+" "+m[2]+" "+m[3]] = m[4]
		t.mu.Unlock()
		return
	}
	if m := readerEndRe.FindStringSubmatch(rest); m != nil {
		t.mu.Lock()
		delete(t.readers, m[1]+" "+m[2]+" "+m[3])
		t.mu.Unlock()
		return
	}
	if m := readerSlowRe.FindStringSubmatch(rest); m != nil {
		if n, err := strconv.Atoi(m[4]); err != nil || n < 1 {
			return
		}
		now := t.now()
		t.mu.Lock()
		defer t.mu.Unlock()
		path, ok := t.readers[m[1]+" "+m[2]+" "+m[3]]
		if !ok {
			return // 覚えていない読み手は数えない
		}
		p := t.paths[path]
		if p == nil {
			p = &pathSlow{}
			t.paths[path] = p
		}
		p.prune(now)
		p.events = append(p.events, now)
		p.last = now
		if len(p.events) >= slowThreshold {
			p.slow = true
		}
	}
}

// prune は窓から出た時刻を捨てる（窓は now-slowWindow より新しいもの）。
func (p *pathSlow) prune(now time.Time) {
	i := 0
	for i < len(p.events) && now.Sub(p.events[i]) >= slowWindow {
		i++
	}
	p.events = p.events[i:]
}

// Slow は「追いつかない」path の一覧を path の順で返す。
func (t *readerTracker) Slow() []SlowReader {
	if t == nil {
		return nil
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []SlowReader
	for path, p := range t.paths {
		p.prune(now)
		if p.slow && now.Sub(p.last) >= slowRelease {
			p.slow = false
			p.events = nil
		}
		if p.slow {
			out = append(out, SlowReader{Path: path, Count: len(p.events)})
		} else if len(p.events) == 0 {
			delete(t.paths, path)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
