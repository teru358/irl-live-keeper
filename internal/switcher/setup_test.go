package switcher

import (
	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// 状態を作る小さな部品。all でつなげて setup に渡す。

type setupFn = func(s *state)

// LinkStateT は受信状態の型の短い名前。
type LinkStateT = ingest.LinkState

func all(fns ...setupFn) setupFn {
	return func(s *state) {
		for _, f := range fns {
			f(s)
		}
	}
}

func scn(name string) setupFn         { return func(s *state) { s.Scene = name } }
func link(l ingest.LinkState) setupFn { return func(s *state) { s.Link = l } }
func own(o Owner, ret string) setupFn { return func(s *state) { inStandby(s, o, ret) } }
func autoOff(s *state)                { s.AutoEnabled = false }
func notStreaming(s *state)           { s.Streaming = false }
func startAt(name string) setupFn     { return func(s *state) { s.StartScene = sp(name) } }
func p(id int, pp Purpose, ret string) setupFn {
	return func(s *state) { pend(s, id, pp, ret) }
}
func x(req int, scene string, pp Purpose, ret string) setupFn {
	return func(s *state) { addExp(s, req, scene, pp, ret) }
}
func sentAt(pp Purpose, ms int) setupFn { return func(s *state) { s.LastAutoSent[pp] = at(ms) } }

// disconnected は今の状態のまま切断した状態にする（切断の直前の値を持つ）。
func disconnected(s *state) {
	s.Connected = false
	s.PrevConnScene = s.Scene
	s.PrevConnStreaming = s.Streaming
	s.Expectations = nil
}

// fresh は生成した直後の状態に、設定 CFG だけを入れた状態にする（未接続）。
func fresh(s *state) {
	*s = newMachine().st
	s.Standby = standby
	s.Monitored = []string{camera, camera2}
	s.AutoEnabled = true
	s.ConfigEpoch = 1
}

// applyCfg は設定を差し替え、設定の世代を進める。
func applyCfg(s *state, c ScenesConfigured) {
	s.Standby = c.Standby
	s.Monitored = append([]string(nil), c.Monitored...)
	s.AutoEnabled = c.AutoEnabled
	s.StartScene = nil
	if c.StartScene != nil {
		s.StartScene = sp(*c.StartScene)
	}
	s.ConfigEpoch++
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

func linkTo(l ingest.LinkState) func(s *state) { return func(s *state) { s.Link = l } }
