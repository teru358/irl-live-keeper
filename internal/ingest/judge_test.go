package ingest

import (
	"errors"
	"testing"
	"time"
)

var defaultTh = Thresholds{
	FailKbps:    800,
	RecoverKbps: 1500,
	LowSec:      5 * time.Second,
	RecoverSec:  10 * time.Second,
	DownSec:     2 * time.Second,
}

// step は Sample の列の1要素。want はその Sample の後の状態。
type step struct {
	s    Sample
	want LinkState
}

func zero(iv time.Duration) Sample { return Sample{Interval: iv, Online: false} }
func kbps(k float64, iv time.Duration) Sample {
	return Sample{Interval: iv, Online: true, Protocol: "rtmp", Kbps: k}
}

const sec = time.Second

func run(t *testing.T, th Thresholds, start LinkState, steps []step) {
	t.Helper()
	j := NewJudge(th)
	j.state = start
	for i, st := range steps {
		if got := j.Update(st.s); got != st.want {
			t.Fatalf("sample %d: got %v, want %v", i+1, got, st.want)
		}
		if j.State() != st.want {
			t.Fatalf("sample %d: State() = %v, want %v", i+1, j.State(), st.want)
		}
	}
}

// repeat は同じ Sample を n 回、期待も同じで並べる。
func repeat(n int, s Sample, want LinkState) []step {
	var out []step
	for i := 0; i < n; i++ {
		out = append(out, step{s, want})
	}
	return out
}

func TestJudgeInitialDown(t *testing.T) {
	if got := NewJudge(defaultTh).State(); got != LinkDown {
		t.Fatalf("got %v, want DOWN", got)
	}
}

func TestLinkStateString(t *testing.T) {
	for s, w := range map[LinkState]string{LinkOK: "OK", LinkLow: "LOW", LinkDown: "DOWN"} {
		if s.String() != w {
			t.Errorf("%d: got %q, want %q", int(s), s.String(), w)
		}
	}
}

func TestJudgeRow1(t *testing.T) {
	run(t, defaultTh, LinkOK, []step{{zero(sec), LinkOK}, {zero(sec), LinkDown}})
}

func TestJudgeRow2(t *testing.T) {
	steps := repeat(4, kbps(600, sec), LinkOK)
	steps = append(steps, step{kbps(600, sec), LinkLow})
	run(t, defaultTh, LinkOK, steps)
}

func TestJudgeRow3(t *testing.T) {
	steps := repeat(4, kbps(600, sec), LinkOK)
	steps = append(steps, step{kbps(1000, sec), LinkOK})        // t=5
	steps = append(steps, repeat(4, kbps(600, sec), LinkOK)...) // t=6..9
	steps = append(steps, step{kbps(600, sec), LinkLow})        // t=10
	run(t, defaultTh, LinkOK, steps)
}

func TestJudgeRow4(t *testing.T) {
	steps := []step{{kbps(2000, sec), LinkLow}}
	steps = append(steps, repeat(8, kbps(2000, sec), LinkLow)...) // t=2..9
	steps = append(steps, step{kbps(2000, sec), LinkOK})          // t=10
	run(t, defaultTh, LinkDown, steps)
}

func TestJudgeRow5(t *testing.T) {
	steps := repeat(6, kbps(2000, sec), LinkLow)
	steps = append(steps, step{kbps(1200, sec), LinkLow})         // t=7
	steps = append(steps, repeat(9, kbps(2000, sec), LinkLow)...) // t=8..16
	steps = append(steps, step{kbps(2000, sec), LinkOK})          // t=17
	run(t, defaultTh, LinkLow, steps)
}

func TestJudgeRow6(t *testing.T) {
	iv := 1500 * time.Millisecond
	run(t, defaultTh, LinkOK, []step{{zero(iv), LinkOK}, {zero(iv), LinkDown}})
}

func TestJudgeRow7(t *testing.T) {
	steps := []step{{Sample{Interval: sec, Err: errors.New("api"), Kbps: 0}, LinkOK}}
	steps = append(steps, repeat(15, kbps(2000, sec), LinkOK)...)
	run(t, defaultTh, LinkOK, steps)
}

func TestJudgeRow8(t *testing.T) {
	run(t, defaultTh, LinkLow, []step{{zero(sec), LinkLow}, {zero(sec), LinkDown}})
}

func TestJudgeRow9(t *testing.T) {
	steps := []step{
		{zero(sec), LinkOK},       // t=1
		{kbps(600, sec), LinkOK},  // t=2 zero の累積が 0 に戻る
		{kbps(600, sec), LinkOK},  // t=3
		{kbps(600, sec), LinkOK},  // t=4
		{kbps(600, sec), LinkLow}, // t=5 below は t=1 から続く
	}
	run(t, defaultTh, LinkOK, steps)
}

func TestJudgeRow10(t *testing.T) {
	th := defaultTh
	th.DownSec, th.LowSec = 2*sec, 2*sec
	run(t, th, LinkOK, []step{{zero(sec), LinkOK}, {zero(sec), LinkDown}})
}

func TestJudgeRow11(t *testing.T) {
	th := defaultTh
	th.RecoverSec = sec
	run(t, th, LinkDown, []step{{kbps(2000, sec), LinkOK}})
}

func TestJudgeBoundaries(t *testing.T) {
	// fail_kbps ちょうどは below ではない: 5 秒続けても OK のまま。
	run(t, defaultTh, LinkOK, repeat(6, kbps(800, sec), LinkOK))
	// 800 未満なら LOW になる（上の確認が意味を持つことの対照）。
	run(t, defaultTh, LinkOK, append(repeat(4, kbps(799.9, sec), LinkOK), step{kbps(799.9, sec), LinkLow}))
	// recover_kbps ちょうどは good: 10 秒で OK。
	run(t, defaultTh, LinkLow, append(repeat(9, kbps(1500, sec), LinkLow), step{kbps(1500, sec), LinkOK}))
	// 1499.9 は good ではない。
	run(t, defaultTh, LinkLow, repeat(12, kbps(1499.9, sec), LinkLow))
}

func TestJudgeErrCountsAsZero(t *testing.T) {
	e := Sample{Interval: sec, Online: true, Kbps: 5000, Err: errors.New("x")}
	run(t, defaultTh, LinkOK, []step{{e, LinkOK}, {e, LinkDown}})
}

func TestJudgeOfflineCountsAsZero(t *testing.T) {
	o := Sample{Interval: sec, Online: false, Kbps: 5000}
	run(t, defaultTh, LinkOK, []step{{o, LinkOK}, {o, LinkDown}})
}

func TestJudgeFirstSampleIntervalZero(t *testing.T) {
	// Interval 0 の最初の Sample は累積に何も足さない。
	j := NewJudge(defaultTh)
	if got := j.Update(Sample{Interval: 0, Online: false}); got != LinkDown {
		t.Fatalf("DOWN の最初の zero で %v", got)
	}
	j2 := NewJudge(defaultTh)
	j2.state = LinkOK
	if got := j2.Update(Sample{Interval: 0, Online: false}); got != LinkOK {
		t.Fatalf("got %v, want OK", got)
	}
	if j2.zero != 0 || j2.below != 0 || j2.good != 0 {
		t.Fatalf("累積が 0 でない: %v %v %v", j2.zero, j2.below, j2.good)
	}
	// DOWN で Interval 0 の非 zero の最初の Sample は規則4により LOW になるが、good は 0 のまま。
	j3 := NewJudge(defaultTh)
	if got := j3.Update(Sample{Interval: 0, Online: true, Kbps: 2000}); got != LinkLow {
		t.Fatalf("got %v, want LOW", got)
	}
	if j3.good != 0 {
		t.Fatalf("good = %v, want 0", j3.good)
	}
}

func TestJudgeZeroIsNeverGood(t *testing.T) {
	// zero の Sample は Kbps が高くても good に数えない。good の累積は 0 に戻る。
	g := kbps(2000, sec)
	e := Sample{Interval: sec, Online: true, Kbps: 5000, Err: errors.New("x")}
	steps := append(repeat(9, g, LinkLow), step{e, LinkLow})
	steps = append(steps, repeat(9, g, LinkLow)...)
	steps = append(steps, step{g, LinkOK})
	run(t, defaultTh, LinkLow, steps)
}
