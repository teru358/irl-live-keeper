package switcher

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// Approve は実行役が Action を送る直前に呼び、その時点の状態でバッチの有効条件を確かめる。
// 確かめる順は、期限切れの期待の削除、共通の条件（接続と4つの Epoch）、SetScene の Skip、
// Purpose の有効条件。SetScene の Proceed では期待を登録する。
// 未完了の一覧からは消さない（消すのは BatchDone）。
func (m *Machine) Approve(batchID int, action Action, override Override, at time.Time) ApproveResult {
	m.expire(at)
	s := &m.st
	pb, ok := s.Pending[batchID]
	if !ok {
		return Reject{}
	}
	if !m.commonValid(pb) {
		if pb.Purpose == PurposeStartScene && pb.SceneEpoch != s.SceneEpoch {
			// 人が先にシーンを選んでいた。ほかの条件が同時に崩れていても、この却下は記録しない。
			pb.ManualRejected = true
			s.Pending[batchID] = pb
		}
		return Reject{}
	}
	set, isSet := action.(SetScene)
	if isSet && pb.Purpose != PurposeStandbyOn && s.Scene == set.Scene {
		return Skip{}
	}
	switch m.purposeValid(pb.Purpose) {
	case verdictProceed:
	case verdictReplace:
		// 受信状態だけが悪くなった。その場で持ち主を auto にし、回復したら AutoRecover で戻す。
		s.StandbyOwner = OwnerAuto
		pb.Replaced = true
		s.Pending[batchID] = pb
		return Replace{Batch: nil}
	default:
		return Reject{}
	}
	if !isSet {
		return Proceed{ReqID: 0}
	}
	if override == nil {
		override = OverrideNone{}
	}
	id := s.NextReqID
	s.NextReqID++
	s.Expectations = append(s.Expectations, Expectation{
		ReqID:        id,
		Scene:        set.Scene,
		Purpose:      pb.Purpose,
		ReturnScene:  pb.ReturnScene,
		ConfigEpoch:  s.ConfigEpoch,
		RegisteredAt: at,
		Override:     override,
		TransitionMs: copyInt(s.TransitionMs),
	})
	return Proceed{ReqID: id}
}

// commonValid は Purpose によらない共通の条件。接続していて、バッチを作ったあとに
// 切断も配信状態の変化もなく、シーンを切り替えるバッチなら手動の切替も設定の変更もないこと。
func (m *Machine) commonValid(pb PendingBatch) bool {
	s := &m.st
	if !s.Connected || pb.ConnEpoch != s.ConnEpoch || pb.StreamEpoch != s.StreamEpoch {
		return false
	}
	return !pb.Purpose.switchesScene() || (pb.SceneEpoch == s.SceneEpoch && pb.ConfigEpoch == s.ConfigEpoch)
}

type verdict int

const (
	verdictReject verdict = iota
	verdictProceed
	verdictReplace
)

// purposeValid は Purpose ごとの有効条件を今の状態で確かめる。
func (m *Machine) purposeValid(p Purpose) verdict {
	s := &m.st
	ok := func(c bool) verdict {
		if c {
			return verdictProceed
		}
		return verdictReject
	}
	inStandby := s.Scene == s.Standby
	switch p {
	case PurposeAutoFail, PurposeAutoRecover:
		need, _ := m.autoNeed()
		return ok(need == p)
	case PurposeStandbyOn:
		return ok(s.Streaming && s.StandbyOwner == OwnerNone && !inStandby)
	case PurposeStandbyOffMonitored:
		if !s.Streaming || s.StandbyOwner != OwnerManual || !inStandby {
			return verdictReject
		}
		if s.AutoEnabled && s.Link != ingest.LinkOK {
			return verdictReplace
		}
		return verdictProceed
	case PurposeStandbyOffOther:
		return ok(s.Streaming && s.StandbyOwner == OwnerManual && inStandby)
	case PurposeStartScene:
		return ok(s.Streaming)
	case PurposeStartStream:
		return ok(!s.Streaming)
	case PurposeStopStream:
		return ok(s.Streaming)
	}
	return verdictReject
}
