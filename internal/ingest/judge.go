package ingest

import "time"

// LinkState は受信状態。
type LinkState int

const (
	// LinkOK は十分なビットレートで受信できている状態。
	LinkOK LinkState = iota
	// LinkLow はビットレートが足りない、または途切れた後で回復待ちの状態。
	LinkLow
	// LinkDown は途切れている状態。起動直後はこの状態。
	LinkDown
)

func (s LinkState) String() string {
	switch s {
	case LinkOK:
		return "OK"
	case LinkLow:
		return "LOW"
	case LinkDown:
		return "DOWN"
	}
	return "UNKNOWN"
}

// Thresholds は Judge の閾値。
type Thresholds struct {
	FailKbps    float64       // これ未満は below
	RecoverKbps float64       // これ以上は good
	LowSec      time.Duration // below がこの時間続くと OK から LOW
	RecoverSec  time.Duration // good がこの時間続くと OK に復帰
	DownSec     time.Duration // zero がこの時間続くと DOWN
}

// Judge は Sample の列から受信状態を判定する。時計は読まず、Sample の
// Interval だけを使う純粋な処理である。
type Judge struct {
	th    Thresholds
	state LinkState
	zero  time.Duration // zero の Sample が連続している間の Interval の合計
	below time.Duration // below の同上
	good  time.Duration // good の同上
}

// NewJudge は初期状態 DOWN の Judge を返す。
func NewJudge(th Thresholds) *Judge {
	return &Judge{th: th, state: LinkDown}
}

// State は現在の受信状態を返す。
func (j *Judge) State() LinkState { return j.state }

// Update は Sample を取り込み、更新後の受信状態を返す。
func (j *Judge) Update(s Sample) LinkState {
	isZero := !s.Online || s.Err != nil || s.Kbps == 0
	isBelow := isZero || s.Kbps < j.th.FailKbps
	isGood := !isZero && s.Kbps >= j.th.RecoverKbps

	j.zero = accumulate(j.zero, isZero, s.Interval)
	j.below = accumulate(j.below, isBelow, s.Interval)
	j.good = accumulate(j.good, isGood, s.Interval)

	// 上から評価し、最初に当てはまったものだけを適用する。
	switch {
	case (j.state == LinkLow || j.state == LinkDown) && j.good >= j.th.RecoverSec:
		j.state = LinkOK
	case (j.state == LinkOK || j.state == LinkLow) && j.zero >= j.th.DownSec:
		j.state = LinkDown
	case j.state == LinkOK && j.below >= j.th.LowSec:
		j.state = LinkLow
	case j.state == LinkDown && !isZero:
		j.state = LinkLow
	}
	return j.state
}

// accumulate は条件を満たす間だけ Interval を足し、満たさなければ 0 に戻す。
func accumulate(acc time.Duration, met bool, d time.Duration) time.Duration {
	if !met {
		return 0
	}
	return acc + d
}

// SetThresholds は状態を保って閾値を替え、連続時間をすべて捨てる。
func (j *Judge) SetThresholds(th Thresholds) {
	j.th = th
	j.zero, j.below, j.good = 0, 0, 0
}
