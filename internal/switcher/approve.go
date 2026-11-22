package switcher

import (
	"slices"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
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
	if pb.Purpose.isItem() {
		return m.approveItem(&pb, action, at)
	}
	if s.StoppingReceived && pb.Purpose.IsAuto() {
		// 停止の受信後は Auto の後続を承認で却下する。Command 由来と StartScene は従来の条件で承認する（HB-AP-26）。
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
	if isSet && pb.Purpose != PurposeStandbyOn && s.Scene == set.SceneUUID {
		return Skip{}
	}
	target := ""
	if isSet {
		target = set.SceneUUID
	}
	if m.purposeValid(pb, isSet, target) != verdictProceed {
		return Reject{}
	}
	if rm, ok := action.(RestartMedia); ok {
		// 実行する入力は、固定した入力から今の program に実表示されているものを外した部分集合（C-03）。空なら送らず、待ちもしない。
		shown := m.actualInputs(s.Scene)
		var sub []string
		for _, in := range rm.Inputs {
			if !shown[in] {
				sub = append(sub, in)
			}
		}
		if len(sub) == 0 {
			return Skip{}
		}
		return Proceed{RestartInputs: sub}
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
		Scene:        set.SceneUUID,
		Purpose:      pb.Purpose,
		ReturnScene:  pb.ReturnScene,
		Plan:         pb.Plan.clone(),
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
)

// purposeValid は Purpose ごとの有効条件を今の状態（地図・健康・待機 UUID）で確かめる。
// SetScene の Action では、バッチを作ったときに固定した切替先 target が、今の待機・戻り先と一致することも確かめる
// （地図の変更は ConfigEpoch を進めないので、承認で古い切替先を止める。W-02）。
func (m *Machine) purposeValid(pb PendingBatch, isSet bool, target string) verdict {
	p := pb.Purpose
	s := &m.st
	ok := func(c bool) verdict {
		if c {
			return verdictProceed
		}
		return verdictReject
	}
	inStandby := s.StandbyUUID != "" && s.Scene == s.StandbyUUID
	switch p {
	case PurposeAutoFail, PurposeAutoRecover:
		// 承認では最新の状態から候補と方向・履歴計画を計算し直し、バッチの行き先・計画と違えば却下する（別の行き先に書き換えない）。
		d, has := m.decide()
		return ok(has && d.Purpose == p && samePlan(d.Plan, pb.Plan) && (!isSet || target == d.Target))
	case PurposeStandbyOn:
		// 受付条件を再検査する。戻り先（auto なら最下段、none なら今のシーン）が変わっていたら却下する（manual 化しない）。
		return ok(s.Streaming && s.StandbyUUID != "" && (s.StandbyOwner == OwnerNone || s.StandbyOwner == OwnerAuto) && !inStandby &&
			(!isSet || target == s.StandbyUUID) && m.onReturnTarget() == pb.ReturnScene)
	case PurposeStandbyOffMonitored, PurposeStandbyOffOther:
		// 最新の状態から OFF の計画を計算し直し、バッチと違えば却下する（auto 化へ置き換えない。押し直しで選び直す）。
		if !s.Streaming || s.StandbyOwner != OwnerManual || !inStandby {
			return verdictReject
		}
		op, reason := m.offPlan()
		return ok(reason == "" && !op.Immediate && op.Purpose == p && samePlan(op.Plan, pb.Plan) && (!isSet || target == op.Target))
	case PurposeStartScene:
		return ok(s.Streaming)
	case PurposeStartStream:
		return ok(!s.Streaming)
	case PurposeStopStream:
		return ok(s.Streaming)
	}
	return verdictReject
}

// ---- 項目のバッチの承認（0b） ----

// itemCommonValid は項目のバッチの共通の条件。Hide / Show は接続・ConnEpoch・StreamEpoch と、停止の受信前。
// 停止用と手動の RestoreAll は接続と ConnEpoch だけ。
func (m *Machine) itemCommonValid(pb *PendingBatch) bool {
	s := &m.st
	if !s.Connected || s.Map == nil || pb.ConnEpoch != s.ConnEpoch || pb.WriteFailed {
		return false
	}
	if pb.Purpose == PurposeRestoreAll {
		return true
	}
	return pb.StreamEpoch == s.StreamEpoch && !s.StoppingReceived
}

func (m *Machine) approveItem(pb *PendingBatch, action Action, at time.Time) ApproveResult {
	s := &m.st
	if !m.itemCommonValid(pb) {
		return Reject{}
	}
	switch a := action.(type) {
	case RestartMedia:
		if pb.Purpose != PurposeShowCamera {
			return Reject{}
		}
		// 共通条件のあとにカメラの状態だけを見る。c が OK で、入力のどれかが今も c のものなら送る。
		if s.CameraStates[pb.CameraID] != scenemap.StateOK {
			return Skip{}
		}
		// 固定した入力のすべてが今もそのカメラの入力のときだけ送る。一つでも違えば Skip。
		calc := m.calcMap()
		for _, in := range a.Inputs {
			if x, ok := calc.Inputs[in]; !ok || x.CameraID != pb.CameraID {
				return Skip{}
			}
		}
		return Proceed{}
	case SetItemEnabled:
		return m.approveSetItem(pb, a, at)
	}
	return Reject{}
}

func (m *Machine) approveSetItem(pb *PendingBatch, a SetItemEnabled, at time.Time) ApproveResult {
	s := &m.st
	k := scenemap.ItemKey{ContainerUUID: a.SceneUUID, ItemID: a.ItemID}
	idx := slices.IndexFunc(pb.Items, func(pi pendItem) bool { return pi.Key == k && pi.Value == a.Enabled })
	if idx < 0 {
		return Reject{}
	}
	pi := &pb.Items[idx]
	defer func() { s.Pending[pb.BatchID] = *pb }()
	it, inMap := m.findItem(k)
	ei := m.entryIdx(k)
	if !a.Enabled {
		return m.approveHide(pb, pi, it, inMap, at)
	}
	switch pb.Purpose {
	case PurposeShowCamera:
		if ei < 0 || !m.entryRestorable(s.Hidden[ei]) {
			return Skip{}
		}
		if !inMap {
			if !mapIsEmpty(s.Map) {
				m.removeEntry(ei)
			}
			return Skip{}
		}
		if it.Enabled {
			if m.qIdx(k, false) >= 0 {
				return m.proceedItem(pb, pi, at)
			}
			if s.Hidden[ei].State != HiddenU {
				m.removeEntry(ei)
			}
			return Skip{}
		}
		return m.proceedItem(pb, pi, at)
	default: // RestoreAll（停止用と手動）
		if !inMap {
			if ei >= 0 && !mapIsEmpty(s.Map) {
				m.removeEntry(ei)
			}
			return Skip{}
		}
		if ei < 0 {
			// 一覧外。カメラの項目（設定のカメラの入力）であることを再確認する。
			calc := m.calcMap()
			if in, ok := calc.Inputs[it.SourceUUID]; !ok || in.CameraID == "" || it.SourceKind != scenemap.InputSource {
				return Skip{}
			}
			if pb.Scope != RestoreAllCameraItems && it.Enabled {
				return Skip{} // 停止用は、記録のない表示の項目に送らない。手動は固定した対象に、地図 true でも送る（HB-AP-22）
			}
			return m.proceedItem(pb, pi, at)
		}
		if it.Enabled {
			st := s.Hidden[ei].State
			if st == HiddenP || st == HiddenU || m.qIdx(k, false) >= 0 {
				return m.proceedItem(pb, pi, at)
			}
			m.removeEntry(ei)
			return Skip{}
		}
		return m.proceedItem(pb, pi, at)
	}
}

// approveHide は Hide の false の承認。目的別の条件（有効な hide、StartScene、MA より前の Hide、空にしない）のあと、項目の(1)〜(3)。
func (m *Machine) approveHide(pb *PendingBatch, pi *pendItem, it scenemap.Item, inMap bool, at time.Time) ApproveResult {
	s := &m.st
	if pb.Purpose != PurposeHideCamera || pb.BatchID < s.MAFloor || !m.canHide(pb.CameraID) || m.hideSuppressed() {
		return Reject{}
	}
	switch {
	case !inMap:
		return Skip{}
	case it.SourceUUID != pi.Source || m.calcMapInputCamera(it.SourceUUID) != pb.CameraID:
		return Skip{}
	case !it.Enabled || m.hasQ(pi.Key):
		return Skip{}
	}
	if m.wouldEmpty(map[scenemap.ItemKey]bool{pi.Key: true}) {
		return Reject{}
	}
	return m.proceedItem(pb, pi, at)
}

func (m *Machine) calcMapInputCamera(input string) string {
	calc := m.calcMap()
	return calc.Inputs[input].CameraID
}

// proceedItem は項目の要求を送ってよいとき: 最初の false では全対象を予約し、Q と台帳を登録して ReqID を返す。
func (m *Machine) proceedItem(pb *PendingBatch, pi *pendItem, at time.Time) ApproveResult {
	s := &m.st
	persist := false
	if !pi.Value && pb.Reserved && m.entryIdx(pi.Key) < 0 {
		// 予約のあとに照合で記録が消えた（項目が消えて戻った、など）。記録なしでは false を送らない: 作り直して保存する。
		s.Hidden = append(s.Hidden, HiddenEntry{
			CameraID: pb.CameraID, Host: s.Z.Host, Port: s.Z.Port, Collection: s.Z.Collection,
			ContainerUUID: pi.Key.ContainerUUID, ItemID: pi.Key.ItemID, SourceUUID: pi.Source,
			State: HiddenP, WrittenAt: at,
		})
		pi.Created = true
		m.sortHidden()
		m.touch()
		persist = true
	}
	if !pi.Value && !pb.Reserved {
		pb.Reserved = true
		persist = true
		for i := range pb.Items {
			it := &pb.Items[i]
			if m.entryIdx(it.Key) >= 0 {
				continue
			}
			s.Hidden = append(s.Hidden, HiddenEntry{
				CameraID: pb.CameraID, Host: s.Z.Host, Port: s.Z.Port, Collection: s.Z.Collection,
				ContainerUUID: it.Key.ContainerUUID, ItemID: it.Key.ItemID, SourceUUID: it.Source,
				State: HiddenP, WrittenAt: at,
			})
			it.Created = true
			s.Unsent[it.Key] = pb.BatchID
		}
		m.sortHidden()
		m.touch()
	}
	id := s.NextItemReq
	s.NextItemReq++
	pi.Sent = true
	pi.ReqID = id
	pb.Sent++
	if !pi.Value && s.Unsent[pi.Key] == pb.BatchID {
		delete(s.Unsent, pi.Key) // 送った（予約の最初の false）。true の Proceed は他のバッチの予約に触れない。
	}
	s.ItemExpectations = append(s.ItemExpectations, ItemExpectation{
		Key: pi.Key, Value: pi.Value, ReqID: id, BatchID: pb.BatchID, Purpose: pb.Purpose, Scope: pb.Scope,
	})
	s.ItemReceipts[id] = ItemReceipt{ReqID: id, Key: pi.Key, Value: pi.Value, BatchID: pb.BatchID,
		CameraID: pb.CameraID, Z: s.Z, Source: pi.Source, Purpose: pb.Purpose, Scope: pb.Scope}
	if pb.Scope == RestoreAllCameraItems && pi.Value {
		s.MAConfirm[pi.Key] = true
	}
	return Proceed{ItemReqID: id, MustPersist: persist}
}
