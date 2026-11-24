package core

import (
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// Snapshot は status の JSON（9章）になる読み取り専用の状態。
// キーは null のときも省かない。
type Snapshot struct {
	SceneDetails map[string]obs.Scene `json:"-"`
	Cameras      []CameraStatus       `json:"cameras"` // 設定の順
	OBS          OBSStatus            `json:"obs"`
	MediaMTX     string               `json:"mediamtx"`
	URLs         []CameraURLs         `json:"urls"` // カメラごと。設定の順
	LastError    *LastError           `json:"last_error"`
}

// CameraStatus はカメラ1台の設定と、受信状態と最後の Sample の値。
type CameraStatus struct {
	FailKbps    float64    `json:"fail_kbps"`
	RecoverKbps float64    `json:"recover_kbps"`
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	OnBad       string     `json:"on_bad"` // standby / hide
	State       string     `json:"state"`  // OK / LOW / DOWN
	Kbps        float64    `json:"kbps"`
	Online      bool       `json:"online"`
	Protocol    string     `json:"protocol"`
	SRT         *SRTStatus `json:"srt"` // SRT のときだけ
	// Sources はカメラのメディアソースの名前（地図から。なければ []）。
	Sources []string `json:"sources"`
	// Hidden は本体が隠している項目の数（今の接続先とシーンコレクションの記録。隠しかけと未決着を含む）。
	Hidden int `json:"hidden"`
	// UserShown は人が戻した印（印のある間は、本体は隠さない）。Unusable は隠す・戻す操作に続けて3回失敗したこと。
	UserShown bool `json:"user_shown"`
	Unusable  bool `json:"unusable"`
}

// SRTStatus は SRT 接続の表示用の統計（MediaMTX の /v3/srt/conns/list の値のまま）。
// LossRate は % （packetsReceivedLossRate。gosrt の PktRecvLossRate「受け取ったデータに対する再送されたデータの割合」）。
// 0.76 は 0.76%。RecvMbps は SRT の段階で受け取った速さで、MediaMTX が読めないデータ（暗号化の違いなど）も数える。
type SRTStatus struct {
	RTTMs    float64 `json:"rtt_ms"`
	LossRate float64 `json:"loss_rate"`
	RecvMbps float64 `json:"recv_mbps"`
}

// OBSStatus は OBS の接続と、switcher が追っている状態。
type OBSStatus struct {
	Connected  bool   `json:"connected"`
	Streaming  bool   `json:"streaming"`
	SceneUUID  string `json:"scene_uuid"`
	Scene      string `json:"scene"`       // 地図の名前（なければ UUID）
	SceneClass string `json:"scene_class"` // monitored / standby / other / unknown
	// SceneCameras は今のシーンに映るカメラの id（辞書順。空でも [] にする）。
	SceneCameras []string `json:"scene_cameras"`
	// SceneHealth は今のシーンの健康（OK / LOW / DOWN）。監視対象のシーンでなければ null。
	SceneHealth *string `json:"scene_health"`
	// StudioMode は OBS の Studio Mode がオンか。
	StudioMode  bool          `json:"studio_mode"`
	Standby     StandbyStatus `json:"standby"`
	AutoEnabled bool          `json:"auto_enabled"`
	Warnings    []i18n.Msg    `json:"warnings"` // 空でも [] にする
}

// StandbyStatus は待機の持ち主と戻り先（シーンの名前）。戻り先がなければ ReturnScene は null。
//
// Stack は auto のスタック（下→上、今の地図での名前。0c）。auto 以外は空。ReturnScene は manual の単独の戻り先、
// auto ならスタックの最下段。
type StandbyStatus struct {
	Owner       string   `json:"owner"` // none / auto / manual
	ReturnScene *string  `json:"return_scene"`
	Stack       []string `json:"stack"` // 空でも [] にする
}

// LastError は最後に記録したエラー。Source は switcher か mediamtx。
type LastError struct {
	At      time.Time `json:"at"`
	Source  string    `json:"source"`
	Message i18n.Msg  `json:"message"`
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
	c.onHiddenChanged(ms)
	s := Snapshot{
		Cameras: make([]CameraStatus, len(c.cams)),
		OBS: OBSStatus{
			Connected:    ms.Connected,
			Streaming:    ms.Streaming,
			SceneUUID:    ms.Scene,
			Scene:        c.sceneLabel(ms.Scene, ms.SceneName),
			SceneClass:   ms.Class.String(),
			SceneCameras: append([]string{}, ms.VisibleCameras...),
			StudioMode:   ms.Connected && ms.StudioMode,
			Standby:      StandbyStatus{Owner: ms.Standby.Owner.String(), Stack: []string{}},
			AutoEnabled:  ms.AutoEnabled,
			Warnings:     []i18n.Msg{},
		},
		URLs: make([]CameraURLs, len(c.cams)),
	}
	s.SceneDetails = sceneDetails(c.mapCopy, c.standbyName, c.fallbackRows)
	for i, cam := range c.cams {
		cs := CameraStatus{
			FailKbps: cam.cfg.Thresholds.FailKbps, RecoverKbps: cam.cfg.Thresholds.RecoverKbps,
			ID: cam.cfg.ID, Name: cam.cfg.Name, OnBad: string(cam.cfg.OnBad),
			State: cam.judge.State().String(), Kbps: cam.sample.Kbps, Online: cam.sample.Online, Protocol: cam.sample.Protocol,
			Sources: append([]string{}, c.sourceNames[cam.cfg.ID]...),
		}
		for _, h := range ms.HiddenEntries {
			if h.CameraID == cam.cfg.ID {
				cs.Hidden++
			}
		}
		for _, ci := range ms.Cameras {
			if ci.ID == cam.cfg.ID {
				cs.UserShown, cs.Unusable = ci.UserShown, ci.Unusable
			}
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
		name := c.sceneLabel(r, ms.Standby.ReturnSceneName)
		s.OBS.Standby.ReturnScene = &name
	}
	for i, u := range ms.Standby.Stack {
		name := ""
		if i < len(ms.Standby.StackNames) {
			name = ms.Standby.StackNames[i]
		}
		s.OBS.Standby.Stack = append(s.OBS.Standby.Stack, c.sceneLabel(u, name))
	}
	for _, w := range ms.FallbackWarnings {
		switch {
		case w.Index < 0:
		case w.Field == "scene":
			s.OBS.Warnings = append(s.OBS.Warnings, i18n.New("fallback.scene", "row", strconv.Itoa(w.Index+1), "name", w.Name))
		default:
			s.OBS.Warnings = append(s.OBS.Warnings, i18n.New("fallback.target", "row", strconv.Itoa(w.Index+1), "name", w.Name))
		}
	}
	if c.warningMessages != nil {
		s.OBS.Warnings = append(s.OBS.Warnings, c.warningMessages()...)
	}
	if c.media != nil {
		s.OBS.Warnings = append(s.OBS.Warnings, c.media.w.WarningMessages()...)
	}
	s.OBS.Warnings = append(s.OBS.Warnings, c.slowReaderWarnings()...)
	if w := c.warnStudioMode(ms); w.Code != "" {
		s.OBS.Warnings = append(s.OBS.Warnings, w)
	}
	if n := len(ms.RetainedEntries); ms.Connected && n > 0 {
		s.OBS.Warnings = append(s.OBS.Warnings, i18n.New("hidden.retained", "count", strconv.Itoa(n)))
	}
	if c.loadMessage.Code != "" {
		s.OBS.Warnings = append(s.OBS.Warnings, c.loadMessage)
	}
	if c.store != nil && c.store.failing.Load() {
		s.OBS.Warnings = append(s.OBS.Warnings, i18n.New("hidden.write"))
	}
	if c.mediamtx != nil {
		st := c.mediamtx()
		s.MediaMTX = st.State
		c.noteMediaMTXError(st.LastError, st.LastMessage)
		if c.lastErr != nil && c.lastErr.Source == SourceMediaMTX && st.LastMessage.Code != "" {
			c.lastErr.Message = st.LastMessage
		}
	}
	c.noteSwitcherError(ms.LastError)
	if c.lastErr != nil {
		e := *c.lastErr
		s.LastError = &e
	}
	c.snap.Store(&s)
	c.notifySnapshot()
}

// last_error は switcher と MediaMTX の2か所から来る。どちらも消えずに上書きされるだけなので、
// 前回から変わった方を新しい記録とみなし、時刻の新しい方を出す。switcher の記録は自分の時刻を持ち、
// MediaMTX の記録は変わったことに気づいた時刻を付ける。時刻が同じなら、後から気づいた方を出す。

func (c *Core) noteSwitcherError(e *switcher.ErrorInfo) {
	if e == nil {
		return
	}
	v := *e
	if p := c.prevSwErr; p != nil && p.At.Equal(v.At) && reflect.DeepEqual(p.Localized, v.Localized) {
		return
	}
	c.prevSwErr = &v
	c.offerError(LastError{At: v.At, Source: SourceSwitcher, Message: v.Localized})
}

// noteMediaMTXError は supervisor の LastError を見る。空になっても前の記録は消さない。
func (c *Core) noteMediaMTXError(msg string, message i18n.Msg) {
	if msg == c.prevMtxErr {
		return
	}
	c.prevMtxErr = msg
	if msg != "" {
		c.offerError(LastError{At: c.now(), Source: SourceMediaMTX, Message: message})
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
		msg := "scene changed by user" // OBS や IRL Link での人の切替
		if ms.AdminSceneEpoch != prev.AdminSceneEpoch {
			msg = "scene changed from admin page" // 管理ページの意図（S2-58）による人の切替
		}
		c.log.Info(msg, "from", c.sceneLabel(prev.Scene, prev.SceneName), "to", c.sceneLabel(ms.Scene, ms.SceneName))
	}
	if ms.Standby.Owner != prev.Standby.Owner || ms.Standby.ReturnScene != prev.Standby.ReturnScene ||
		ms.Standby.ReturnSceneName != prev.Standby.ReturnSceneName || !slices.Equal(ms.Standby.Stack, prev.Standby.Stack) {
		c.log.Info("standby changed", "owner", ms.Standby.Owner.String(), "return_scene", c.sceneLabel(ms.Standby.ReturnScene, ms.Standby.ReturnSceneName))
	}
}

// slowReaderWarnings は、読み手（OBS）が追いつかないカメラの警告をカメラごとに1行返す。
// 切り替わりはログに出す。警告だけで、健康や切替には関わらない。
func (c *Core) slowReaderWarnings() []i18n.Msg {
	if c.slowRead == nil {
		return nil
	}
	slow := map[string]supervisor.SlowReader{}
	for _, r := range c.slowRead() {
		slow[r.Path] = r
	}
	now := c.now()
	var out []i18n.Msg
	for _, cam := range c.cams {
		r, ok := slow[cam.cfg.Path]
		if cam.cfg.Path == "" {
			ok = false
		}
		// 止まった（DOWN の）カメラには出さない。DOWN のあいだに残った記録でも、つなぎ直したあとに出さない
		// （T-15 の2日目: カメラが止まっても約60秒残った）。つなぎ直したあとの新しい記録では、また出す。
		down := cam.judge.State() == ingest.LinkDown
		if down {
			cam.downAt = now
			ok = false
		} else if ok && !r.Last.After(cam.downAt) {
			ok = false
		}
		if ok {
			out = append(out, i18n.New("obs.slow_reader", "name", cam.cfg.Name))
		}
		if ok != cam.slow {
			cam.slow = ok
			if ok {
				c.log.Info("obs reader too slow", "camera", cam.cfg.ID, "count", r.Count)
			} else {
				reason := "caught_up"
				if down {
					reason = "camera_down"
				}
				c.log.Info("obs reader caught up", "camera", cam.cfg.ID, "count", 0, "reason", reason)
			}
		}
	}
	return out
}

// Render は共有の Snapshot を変えず、表示文だけを要求の言語にする。
func (s Snapshot) Render(lang i18n.Lang) any {
	type obsStatus struct {
		OBSStatus
		Warnings []string `json:"warnings"`
	}
	type lastError struct {
		*LastError
		Message string `json:"message"`
	}
	var last *lastError
	if s.LastError != nil {
		last = &lastError{s.LastError, i18n.Render(lang, s.LastError.Message)}
	}
	return struct {
		Snapshot
		OBS       obsStatus  `json:"obs"`
		LastError *lastError `json:"last_error"`
	}{s, obsStatus{s.OBS, i18n.Strings(lang, s.OBS.Warnings)}, last}
}

// sceneDetails は公開用の種類と逃がし先の親を地図から作る。
func sceneDetails(m *scenemap.Map, standby string, rows []switcher.FallbackRow) map[string]obs.Scene {
	out := make(map[string]obs.Scene)
	if m != nil {
		for uuid, sc := range m.Scenes {
			cameras := scenemap.CamerasIn(*m, uuid, nil, true)
			ids := make([]string, 0, len(cameras))
			for _, id := range cameras {
				ids = append(ids, id)
			}
			slices.Sort(ids)
			out[uuid] = obs.Scene{UUID: uuid, Name: sc.Name, Kind: "none", Cameras: ids, FallbackOf: []string{}}
		}
		for uuid, sc := range out {
			target := sc.Name == standby
			for _, row := range rows {
				if strings.TrimSpace(row.Fallback) == sc.Name {
					target = true
				}
			}
			if len(sc.Cameras) > 0 {
				sc.Kind = "camera"
			} else if target {
				sc.Kind = "standby"
			}
			for _, parent := range out {
				explicit := ""
				for _, row := range rows {
					if strings.TrimSpace(row.Scene) == parent.Name {
						explicit = strings.TrimSpace(row.Fallback)
						break
					}
				}
				if explicit == sc.Name || (explicit == "" && sc.Name == standby && parent.Name != standby && len(parent.Cameras) > 0) {
					sc.FallbackOf = append(sc.FallbackOf, parent.Name)
				}
			}
			slices.Sort(sc.FallbackOf)
			out[uuid] = sc
		}
	}
	return out
}
