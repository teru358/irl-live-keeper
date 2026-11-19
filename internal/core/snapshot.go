package core

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// Snapshot は status の JSON（9章）になる読み取り専用の状態。
// キーは null のときも省かない。
type Snapshot struct {
	Link      LinkStatus `json:"link"`
	OBS       OBSStatus  `json:"obs"`
	MediaMTX  string     `json:"mediamtx"`
	URLs      URLs       `json:"urls"`
	LastError *LastError `json:"last_error"`
}

// LinkStatus は受信状態と最後の Sample の値。
type LinkStatus struct {
	State    string     `json:"state"` // OK / LOW / DOWN
	Kbps     float64    `json:"kbps"`
	Online   bool       `json:"online"`
	Protocol string     `json:"protocol"`
	SRT      *SRTStatus `json:"srt"` // SRT のときだけ
}

// SRTStatus は SRT 接続の表示用の統計。
type SRTStatus struct {
	RTTMs    float64 `json:"rtt_ms"`
	LossRate float64 `json:"loss_rate"`
	RecvMbps float64 `json:"recv_mbps"`
}

// OBSStatus は OBS の接続と、switcher が追っている状態。
type OBSStatus struct {
	Connected   bool          `json:"connected"`
	Streaming   bool          `json:"streaming"`
	Scene       string        `json:"scene"`
	SceneClass  string        `json:"scene_class"` // monitored / standby / other / unknown
	Standby     StandbyStatus `json:"standby"`
	AutoEnabled bool          `json:"auto_enabled"`
	Warnings    []string      `json:"warnings"` // 空でも [] にする
}

// StandbyStatus は待機の持ち主と戻り先。戻り先がなければ ReturnScene は null。
type StandbyStatus struct {
	Owner       string  `json:"owner"` // none / auto / manual
	ReturnScene *string `json:"return_scene"`
}

// LastError は最後に記録したエラー。Source は switcher か mediamtx。
type LastError struct {
	At      time.Time `json:"at"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
}

// last_error の出どころ。
const (
	SourceSwitcher = "switcher"
	SourceMediaMTX = "mediamtx"
)

// URLs は status に出す固定の URL。
type URLs struct {
	PublishSRT  string `json:"publish_srt"`
	PublishRTMP string `json:"publish_rtmp"`
	OBSRead     string `json:"obs_read"`
}

// publish は今の状態から Snapshot を作って公開する。New と、中核ループが入力を1つ処理するたびに呼ぶ。
func (c *Core) publish() {
	ms := c.m.Snapshot()
	s := Snapshot{
		Link: LinkStatus{
			State:    c.judge.State().String(),
			Kbps:     c.sample.Kbps,
			Online:   c.sample.Online,
			Protocol: c.sample.Protocol,
		},
		OBS: OBSStatus{
			Connected:   ms.Connected,
			Streaming:   ms.Streaming,
			Scene:       ms.Scene,
			SceneClass:  ms.Class.String(),
			Standby:     StandbyStatus{Owner: ms.Standby.Owner.String()},
			AutoEnabled: ms.AutoEnabled,
			Warnings:    []string{},
		},
		URLs: c.urls,
	}
	if r := ms.Standby.ReturnScene; r != "" {
		s.OBS.Standby.ReturnScene = &r
	}
	if st := c.sample.SRT; st != nil {
		s.Link.SRT = &SRTStatus{RTTMs: st.RTTMs, LossRate: st.LossRate, RecvMbps: st.RecvMbps}
	}
	if c.warnings != nil {
		if w := c.warnings(); len(w) > 0 {
			s.OBS.Warnings = append([]string(nil), w...)
		}
	}
	if c.mediamtx != nil {
		st := c.mediamtx()
		s.MediaMTX = st.State
		c.noteMediaMTXError(st.LastError)
	}
	c.noteSwitcherError(ms.LastError)
	if c.lastErr != nil {
		e := *c.lastErr
		s.LastError = &e
	}
	c.snap.Store(&s)
}

// last_error は switcher と MediaMTX の2か所から来る。どちらも消えずに上書きされるだけなので、
// 前回から変わった方を新しい記録とみなし、時刻の新しい方を出す。switcher の記録は自分の時刻を持ち、
// MediaMTX の記録は変わったことに気づいた時刻を付ける。時刻が同じなら、後から気づいた方を出す。

func (c *Core) noteSwitcherError(e *switcher.ErrorInfo) {
	if e == nil {
		return
	}
	if p := c.prevSwErr; p != nil && p.At.Equal(e.At) && p.Msg == e.Msg {
		return
	}
	v := *e
	c.prevSwErr = &v
	c.offerError(LastError{At: e.At, Source: SourceSwitcher, Message: e.Msg})
}

// noteMediaMTXError は supervisor の LastError を見る。空になっても前の記録は消さない。
func (c *Core) noteMediaMTXError(msg string) {
	if msg == c.prevMtxErr {
		return
	}
	c.prevMtxErr = msg
	if msg != "" {
		c.offerError(LastError{At: c.now(), Source: SourceMediaMTX, Message: msg})
	}
}

func (c *Core) offerError(e LastError) {
	if c.lastErr == nil || !e.At.Before(c.lastErr.At) {
		c.lastErr = &e
	}
}
