package switcher

import (
	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 状態を作る小さな部品。all でつなげて setup に渡す。

type setupFn = func(s *state)

// LinkStateT は受信状態（カメラの状態）の型の短い名前。
type LinkStateT = scenemap.CameraState

func all(fns ...setupFn) setupFn {
	return func(s *state) {
		for _, f := range fns {
			f(s)
		}
	}
}

func scn(uuid string) setupFn { return func(s *state) { s.Scene = uuid } }

// link は単独のカメラ main の状態を設定する。
func link(l scenemap.CameraState) setupFn { return func(s *state) { s.CameraStates[camMain] = l } }
func own(o Owner, ret string) setupFn     { return func(s *state) { inStandby(s, o, ret) } }
func autoOff(s *state)                    { s.AutoEnabled = false }
func notStreaming(s *state)               { s.Streaming = false }
func startAt(uuid string) setupFn {
	return func(s *state) {
		s.StartScene = sp(nameOf(uuid))
		resolveNames(s)
	}
}
func p(id int, pp Purpose, ret string) setupFn {
	return func(s *state) { pend(s, id, pp, ret) }
}
func x(req int, scene string, pp Purpose, ret string) setupFn {
	return func(s *state) { addExp(s, req, scene, pp, ret) }
}
func sentAt(pp Purpose, ms int) setupFn { return func(s *state) { s.LastAutoSent[pp] = at(ms) } }

// states は複数のカメラの状態を設定する。
func states(m, sub, d scenemap.CameraState) setupFn {
	return func(s *state) {
		s.CameraStates = map[string]scenemap.CameraState{camMain: m, camSub: sub, camDesk: d}
	}
}

// useMap は地図を差し替える（直接作る状態用。名前も引き直す）。
func useMap(m *scenemap.Map) setupFn {
	return func(s *state) {
		s.Map = cloneTestMap(m)
		resolveNames(s)
	}
}

// disconnected は今の状態のまま切断した状態にする（切断の直前の値を持ち、地図と解決した UUID を消す）。
func disconnected(s *state) {
	s.Connected = false
	s.PrevConnScene = s.Scene
	s.PrevConnStreaming = s.Streaming
	if s.Map != nil {
		s.PrevConnCollection = s.Map.Collection
	}
	s.Map = nil
	s.StandbyUUID = ""
	s.StartSceneUUID = ""
	s.Expectations = nil
}

// fresh は生成した直後の状態に、設定 CFG だけを入れた状態にする（未接続、地図なし、main は DOWN）。
func fresh(s *state) {
	*s = newMachine().st
	s.Standby = "Standby"
	s.Cameras = []CameraConfig{cam(camMain)}
	s.CameraStates = map[string]scenemap.CameraState{camMain: DOWN}
	s.AutoEnabled = true
	s.ConfigEpoch = 1
}

// applyCfg は設定を差し替え、設定の世代を進める。残るカメラの状態は保ち、増えたものは DOWN、減ったものは除く。
func applyCfg(s *state, c ScenesConfigured) {
	s.Standby = c.Standby
	s.Cameras = append([]CameraConfig(nil), c.Cameras...)
	st := map[string]scenemap.CameraState{}
	for _, cc := range c.Cameras {
		if v, ok := s.CameraStates[cc.ID]; ok {
			st[cc.ID] = v
		} else {
			st[cc.ID] = DOWN
		}
	}
	s.CameraStates = st
	s.AutoEnabled = c.AutoEnabled
	s.StartScene = nil
	if c.StartScene != nil {
		s.StartScene = sp(*c.StartScene)
	}
	s.ConfigEpoch++
	resolveNames(s)
}

// cfgChange は CFG に mods を当てた設定への差し替えを、期待の状態に当てはめる関数を返す。
// clear なら持ち主と戻り先も消す。
func cfgChange(clear bool, mods ...cfgMod) func(s *state) {
	return func(s *state) {
		applyCfg(s, cfgOf(0, mods...))
		if clear {
			clearOwner(s)
		}
	}
}

func linkTo(l scenemap.CameraState) func(s *state) {
	return func(s *state) { s.CameraStates[camMain] = l }
}

// mapChange は地図を visible のシーンにだけ main の項目を置いた地図に差し替えた変化（ConfigEpoch は進めない）。
// clear なら持ち主と戻り先も消す。
func mapChange(clear bool, visible ...string) func(s *state) {
	return func(s *state) {
		s.Map = testMap(visible...)
		resolveNames(s)
		if clear {
			clearOwner(s)
		}
	}
}
