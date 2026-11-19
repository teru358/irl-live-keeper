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
	LastError *string    `json:"last_error"`
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
	Connected  bool       `json:"connected"`
	Streaming  bool       `json:"streaming"`
	Scene      string     `json:"scene"`
	Role       string     `json:"role"`
	ManualHold ManualHold `json:"manual_hold"`
	Warnings   []string   `json:"warnings"` // 空でも [] にする
}

// ManualHold は手動保持の状態。Since は保持中だけ値を持つ。
type ManualHold struct {
	Active bool       `json:"active"`
	Since  *time.Time `json:"since"`
}

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
			Connected: ms.Connected,
			Streaming: ms.Streaming,
			Scene:     ms.Scene,
			Role:      ms.Role.String(),
			ManualHold: ManualHold{
				Active: ms.Hold.Active,
			},
			Warnings: []string{},
		},
		URLs: c.urls,
	}
	if st := c.sample.SRT; st != nil {
		s.Link.SRT = &SRTStatus{RTTMs: st.RTTMs, LossRate: st.LossRate, RecvMbps: st.RecvMbps}
	}
	if ms.Hold.Active {
		since := ms.Hold.Since
		s.OBS.ManualHold.Since = &since
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
// 前回から変わった方を新しい記録とみなし、最後に変わったものを出す。

func (c *Core) noteSwitcherError(e *switcher.ErrorInfo) {
	if e == nil {
		return
	}
	if p := c.prevSwErr; p != nil && p.At.Equal(e.At) && p.Msg == e.Msg {
		return
	}
	v := *e
	c.prevSwErr = &v
	msg := e.Msg
	c.lastErr = &msg
}

func (c *Core) noteMediaMTXError(msg string) {
	if msg == c.prevMtxErr {
		return
	}
	c.prevMtxErr = msg
	if msg != "" {
		m := msg
		c.lastErr = &m
	}
}
