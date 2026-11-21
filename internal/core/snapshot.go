package core

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// Snapshot は status の JSON（9章）になる読み取り専用の状態。
// キーは null のときも省かない。
// カメラの spec 第9節の sources、hidden、user_shown、unusable と obs.studio_mode は、隠す・戻す（0b）で足す。
type Snapshot struct {
	Cameras   []CameraStatus `json:"cameras"` // 設定の順
	OBS       OBSStatus      `json:"obs"`
	MediaMTX  string         `json:"mediamtx"`
	URLs      []CameraURLs   `json:"urls"` // カメラごと。設定の順
	LastError *LastError     `json:"last_error"`
}

// CameraStatus はカメラ1台の設定と、受信状態と最後の Sample の値。
type CameraStatus struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	OnBad    string     `json:"on_bad"` // standby / hide
	State    string     `json:"state"`  // OK / LOW / DOWN
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
	Connected  bool   `json:"connected"`
	Streaming  bool   `json:"streaming"`
	Scene      string `json:"scene"`       // 地図の名前（なければ UUID）
	SceneClass string `json:"scene_class"` // monitored / standby / other / unknown
	// SceneCameras は今のシーンに映るカメラの id（辞書順。空でも [] にする）。
	SceneCameras []string `json:"scene_cameras"`
	// SceneHealth は今のシーンの健康（OK / LOW / DOWN）。監視対象のシーンでなければ null。
	SceneHealth *string       `json:"scene_health"`
	Standby     StandbyStatus `json:"standby"`
	AutoEnabled bool          `json:"auto_enabled"`
	Warnings    []string      `json:"warnings"` // 空でも [] にする
}

// StandbyStatus は待機の持ち主と戻り先（シーンの名前）。戻り先がなければ ReturnScene は null。
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

// CameraURLs はカメラ1台の status に出す固定の URL。止めたプロトコルの送信用は空文字。
type CameraURLs struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PublishSRT  string `json:"publish_srt"`
	PublishRTMP string `json:"publish_rtmp"`
	OBSRead     string `json:"obs_read"`
}

// publish は今の状態から Snapshot を作って公開する。New と、中核ループが入力を1つ処理するたびに呼ぶ。
func (c *Core) publish() {
	ms := c.m.Snapshot()
	c.logStateChanges(ms)
	s := Snapshot{
		Cameras: make([]CameraStatus, len(c.cams)),
		OBS: OBSStatus{
			Connected:    ms.Connected,
			Streaming:    ms.Streaming,
			Scene:        sceneLabel(ms.Scene, ms.SceneName),
			SceneClass:   ms.Class.String(),
			SceneCameras: append([]string{}, ms.VisibleCameras...),
			Standby:      StandbyStatus{Owner: ms.Standby.Owner.String()},
			AutoEnabled:  ms.AutoEnabled,
			Warnings:     []string{},
		},
		URLs: make([]CameraURLs, len(c.cams)),
	}
	for i, cam := range c.cams {
		cs := CameraStatus{
			ID: cam.cfg.ID, Name: cam.cfg.Name, OnBad: string(cam.cfg.OnBad),
			State: cam.judge.State().String(), Kbps: cam.sample.Kbps, Online: cam.sample.Online, Protocol: cam.sample.Protocol,
		}
		if st := cam.sample.SRT; st != nil {
			cs.SRT = &SRTStatus{RTTMs: st.RTTMs, LossRate: st.LossRate, RecvMbps: st.RecvMbps}
		}
		s.Cameras[i] = cs
		s.URLs[i] = cam.cfg.URLs
		s.URLs[i].ID, s.URLs[i].Name = cam.cfg.ID, cam.cfg.Name
	}
	if ms.HasHealth {
		h := string(ms.Health)
		s.OBS.SceneHealth = &h
	}
	if r := ms.Standby.ReturnScene; r != "" {
		name := sceneLabel(r, ms.Standby.ReturnSceneName)
		s.OBS.Standby.ReturnScene = &name
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

// sceneLabel は status とログに出すシーンの名前。地図に名前がなければ UUID。
func sceneLabel(uuid, name string) string {
	if name != "" {
		return name
	}
	return uuid
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

// logStateChanges は前回の状態と比べ、変わったものを1行ずつログに書く。最初の Snapshot では書かない。
// 人の切替は switcher の SceneEpoch（人の切替とみなした回数）が増えたことで知る。
func (c *Core) logStateChanges(ms switcher.Snapshot) {
	prev := c.prevSw
	c.prevSw = &ms
	if prev == nil {
		return
	}
	if ms.Streaming != prev.Streaming {
		c.log.Info("stream state", "streaming", ms.Streaming)
	}
	if ms.SceneEpoch != prev.SceneEpoch {
		c.log.Info("scene changed by user", "from", sceneLabel(prev.Scene, prev.SceneName), "to", sceneLabel(ms.Scene, ms.SceneName))
	}
	if ms.Standby != prev.Standby {
		c.log.Info("standby changed", "owner", ms.Standby.Owner.String(), "return_scene", sceneLabel(ms.Standby.ReturnScene, ms.Standby.ReturnSceneName))
	}
}
