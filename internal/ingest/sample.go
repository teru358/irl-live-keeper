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
	Camera   string        // カメラの id（= path 名）
	At       time.Time     // 取得時刻
	Interval time.Duration // 前回の Sample からの経過時間（この Sample が表す区間の長さ）
	Online   bool          // path に publisher がいるか
	Protocol string        // "rtmp" / "srt" / ""
	Kbps     float64       // 基準からの inboundBytes の差分から求めたビットレート
	SRT      *SRTStats     // SRT のときだけ
	Err      error         // path の API の失敗（このとき Kbps は 0 として扱う）
}

// SampleSet は1回の読み取りで出す、全カメラの Sample の1組。
// Cameras はカメラの設定の順（NewPoller の ids の順）。各 Sample の At は組の At と同じで、
// カメラは Sample.Camera の id でも確かめられる。
type SampleSet struct {
	At      time.Time
	Cameras []Sample
}
