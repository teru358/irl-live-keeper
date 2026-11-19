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
		return m.reject(pb)
	}
	set, isSet := action.(SetScene)
	if isSet && s.Role == set.Role {
		return Skip{}
	}
	switch m.purposeValid(pb.Purpose, pb.StartID) {
	case verdictProceed:
	case verdictReplace:
		pb.Replaced = true
		s.Pending[batchID] = pb
		return Replace{Batch: m.emit(replacementOf(pb.Purpose), at, 0)}
	default:
		return m.reject(pb)
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

// reject は却下する。StartIntro / StartStream なら、自分と同じ番号の配信開始の待ちも消す。
// 記録は、続いて届く BatchDone{Rejected} で行う。
func (m *Machine) reject(pb PendingBatch) ApproveResult {
	if w := m.st.StartWait; w != nil && pb.StartID != 0 && w.ID == pb.StartID {
		m.st.StartWait = nil
	}
	return Reject{}
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
func (m *Machine) purposeValid(p Purpose, startID int) verdict {
	s := &m.st
	ok := func(c bool) verdict {
		if c {
			return verdictProceed
		}
		return verdictReject
	}
	linkOK := s.Link == ingest.LinkOK
	ownWait := func(id int) bool { return s.StartWait != nil && s.StartWait.ID == id }
	switch p {
	case PurposeAutoFail:
		return ok(s.Streaming && s.Role == RoleLive && bad(s.Link) && !s.Hold && !m.expectingOutsideAuto())
	case PurposeAutoRecover:
		return ok(s.Streaming && s.Role == RoleFail && linkOK && !s.Hold && !m.expectingOutsideAuto())
	case PurposeGoLive, PurposePauseOffLive:
		from := RoleIntro
		if p == PurposePauseOffLive {
			from = RolePause
		}
		switch {
		case !s.Streaming || s.Role != from:
			return verdictReject
		case !linkOK:
			// 受信状態だけが悪くなった。Fail に戻す操作に決め直す。
			return verdictReplace
		}
		return verdictProceed
	case PurposeGoFail:
		return ok(s.Streaming && s.Role == RoleIntro)
	case PurposePauseOffFail, PurposePauseOffIntro:
		return ok(s.Streaming && s.Role == RolePause)
	case PurposePauseOn:
		return ok(s.Streaming && (s.Role == RoleIntro || s.Role == RoleLive || s.Role == RoleFail))
	case PurposeStartIntro:
		return ok(!s.Streaming && ownWait(startID))
	case PurposeStartStream:
		return ok(!s.Streaming && s.Role == RoleIntro && ownWait(startID))
	case PurposeStopStream:
		return ok(s.Streaming)
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
