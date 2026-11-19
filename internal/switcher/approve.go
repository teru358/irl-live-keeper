package switcher

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// Approve は実行役が Action を送る直前に呼び、その時点の状態でバッチの有効条件を確かめる。
// 確かめる順は、共通の条件（接続と3つの Epoch）、SetScene の Skip、Purpose の有効条件。
// SetScene の Proceed では期待を登録する。未完了の一覧からは消さない（消すのは BatchDone）。
func (m *Machine) Approve(batchID int, action Action, override Override, at time.Time) ApproveResult {
	m.expire(at)
	s := &m.st
	pb, ok := s.Pending[batchID]
	if !ok {
		return Reject{}
	}
	if !m.commonValid(pb) {
		return Reject{}
	}
	set, isSet := action.(SetScene)
	if isSet && s.Role == set.Role {
		return Skip{}
	}
	switch m.purposeValid(pb.Purpose) {
	case verdictProceed:
	case verdictReplace:
		pb.Replaced = true
		s.Pending[batchID] = pb
		return Replace{Batch: m.emit(replacementOf(pb.Purpose), at, 0)}
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
		Role:         set.Role,
		RegisteredAt: at,
		Override:     override,
		TransitionMs: copyInt(s.TransitionMs),
	})
	return Proceed{ReqID: id}
}

// commonValid は Purpose によらない共通の条件。接続していて、バッチを作ったあとに
// 切断も配信状態の変化もなく、シーンを切り替えるバッチなら手動の切替もないこと。
func (m *Machine) commonValid(pb PendingBatch) bool {
	s := &m.st
	if !s.Connected || pb.ConnEpoch != s.ConnEpoch || pb.StreamEpoch != s.StreamEpoch {
		return false
	}
	return !pb.Purpose.switchesScene() || pb.SceneEpoch == s.SceneEpoch
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
	linkOK := s.Link == ingest.LinkOK
	switch p {
	case PurposeAutoFail:
		return ok(s.Streaming && s.Role == RoleLive && bad(s.Link) && !s.Hold && !m.expectingOutsideAuto())
	case PurposeAutoRecover:
		return ok(s.Streaming && s.Role == RoleFail && linkOK && !s.Hold && !m.expectingOutsideAuto())
	}
	return verdictReject
}

// replacementOf は Replace で置き換える先の Purpose。
func replacementOf(p Purpose) Purpose {
	switch p {
	case PurposeGoLive:
		return PurposeGoFail
	case PurposePauseOffLive:
		return PurposePauseOffFail
	}
	return 0
}
