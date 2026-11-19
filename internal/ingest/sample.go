package ingest

import "time"

// SRTStats は SRT 接続の表示用の統計。判定には使わない。
type SRTStats struct {
	RTTMs    float64 // msRTT
	LossRate float64 // packetsReceivedLossRate
	RecvMbps float64 // mbpsReceiveRate
}

// Sample は Poller が1回の読み取りで出力する値。
type Sample struct {
	At       time.Time     // 取得時刻
	Interval time.Duration // 前回の Sample からの経過時間（この Sample が表す区間の長さ）
	Online   bool          // path に publisher がいるか
	Protocol string        // "rtmp" / "srt" / ""
	Kbps     float64       // 基準からの inboundBytes の差分から求めたビットレート
	SRT      *SRTStats     // SRT のときだけ
	Err      error         // path の API の失敗（このとき Kbps は 0 として扱う）
}
