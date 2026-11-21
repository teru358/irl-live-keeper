package switcher

import (
	"slices"
	"sort"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 0b: 本体が隠した項目の記録、項目の期待と送信台帳、Hide / Show / RestoreAll の判断と承認。
// 表は docs/superpowers/plans/2026-11-21-switcher-table-0b.md。

const (
	// itemQWindow は応答から項目の期待を保つ時間（H-03。境界は > で、ちょうどは保つ）。
	itemQWindow = 2 * time.Second
	// pSweep は決着した P を、地図 true のとき掃除するまでの時間（M-2。ちょうどは保つ）。
	pSweep = 10 * time.Second
	// itemRetry は Hide / Show 共通の再送の間隔。
	itemRetry = 5 * time.Second
	// itemFailLimit は続けて失敗してよい回数。3以上は使えない。
	itemFailLimit = 3
)

// ---- 記録の基本操作 ----

func (m *Machine) touch() { m.st.HiddenRevision++ }

func entryZone(e HiddenEntry) zone { return zone{e.Host, e.Port, e.Collection} }

func (m *Machine) entryIdx(k scenemap.ItemKey) int {
	return slices.IndexFunc(m.st.Hidden, func(e HiddenEntry) bool { return e.Key() == k })
}

func (m *Machine) sortHidden() {
	sort.SliceStable(m.st.Hidden, func(i, j int) bool {
		a, b := m.st.Hidden[i], m.st.Hidden[j]
		if a.ContainerUUID != b.ContainerUUID {
			return a.ContainerUUID < b.ContainerUUID
		}
		return a.ItemID < b.ItemID
	})
}

func (m *Machine) removeEntry(i int) {
	s := &m.st
	k := s.Hidden[i].Key()
	s.Hidden = slices.Delete(s.Hidden, i, i+1)
	delete(s.Unsent, k)
	delete(s.MAConfirm, k)
	m.touch()
}

// removeEntryKey は鍵の記録があれば外す。
func (m *Machine) removeEntryKey(k scenemap.ItemKey) {
	if i := m.entryIdx(k); i >= 0 {
		m.removeEntry(i)
	}
}

// findItem は地図の項目を返す。
func (m *Machine) findItem(k scenemap.ItemKey) (scenemap.Item, bool) {
	mp := m.st.Map
	if mp == nil {
		return scenemap.Item{}, false
	}
	c, ok := mp.Scenes[k.ContainerUUID]
	if !ok {
		c, ok = mp.Groups[k.ContainerUUID]
	}
	if !ok {
		return scenemap.Item{}, false
	}
	for _, it := range c.Items {
		if it.ID == k.ItemID {
			return it, true
		}
	}
	return scenemap.Item{}, false
}

// setMapItem は地図の項目の Enabled を書き換える。項目がなければ false。
func (m *Machine) setMapItem(k scenemap.ItemKey, v bool) bool {
	mp := m.st.Map
	if mp == nil {
		return false
	}
	containers := mp.Scenes
	c, ok := containers[k.ContainerUUID]
	if !ok {
		containers = mp.Groups
		c, ok = containers[k.ContainerUUID]
	}
	if !ok {
		return false
	}
	i := slices.IndexFunc(c.Items, func(it scenemap.Item) bool { return it.ID == k.ItemID })
	if i < 0 {
		return false
	}
	items := slices.Clone(c.Items)
	items[i].Enabled = v
	c.Items = items
	containers[k.ContainerUUID] = c
	return true
}

func (m *Machine) qIdx(k scenemap.ItemKey, v bool) int {
	return slices.IndexFunc(m.st.ItemExpectations, func(q ItemExpectation) bool { return q.Key == k && q.Value == v })
}

func (m *Machine) hasQ(k scenemap.ItemKey) bool {
	return slices.ContainsFunc(m.st.ItemExpectations, func(q ItemExpectation) bool { return q.Key == k })
}

func (m *Machine) qByReq(req uint64) int {
	return slices.IndexFunc(m.st.ItemExpectations, func(q ItemExpectation) bool { return q.ReqID == req })
}

func (m *Machine) dropQKey(k scenemap.ItemKey) {
	m.st.ItemExpectations = slices.DeleteFunc(m.st.ItemExpectations, func(q ItemExpectation) bool { return q.Key == k })
}

// ---- 有効な動きと「戻してよいか」 ----

func (m *Machine) cameraCfg(id string) (CameraConfig, bool) {
	for _, c := range m.st.Cameras {
		if c.ID == id {
			return c, true
		}
	}
	return CameraConfig{}, false
}

// motionOf はカメラの有効な動き。on_bad=hide は Studio Mode のとき studio_mode_hide に従う。
func (m *Machine) motionOf(c CameraConfig) scenemap.Motion {
	if c.OnBad != scenemap.MotionHide {
		return scenemap.MotionStandby
	}
	s := &m.st
	if !s.StudioMode {
		return scenemap.MotionHide
	}
	switch s.StudioModeHide {
	case StudioHideIgnore:
		return scenemap.MotionIgnore
	case StudioHideHide:
		return scenemap.MotionHide
	}
	return scenemap.MotionStandby
}

func (m *Machine) unusable(c string) bool { return m.st.ItemFailures[c] >= itemFailLimit }

// canHide はカメラ c の項目を新しく隠してよいか（有効な hide で、不良、自動有効、配信中、人が戻していない、使える）。
func (m *Machine) canHide(c string) bool {
	s := &m.st
	cc, ok := m.cameraCfg(c)
	if !ok || !s.Connected || s.Map == nil || !s.Streaming || !s.AutoEnabled || s.StoppingReceived {
		return false
	}
	return m.motionOf(cc) == scenemap.MotionHide && s.CameraStates[c] != scenemap.StateOK && !s.UserShown[c] && !m.unusable(c)
}

// canRestore は本体が隠したカメラ c の項目を戻してよいか。OK、配信停止、自動無効、on_bad=standby（Studio Mode の standby を含む）、
// 設定から消えたカメラ。Studio Mode の ignore だけでは戻さない（H-07）。
func (m *Machine) canRestore(c string) bool {
	s := &m.st
	cc, ok := m.cameraCfg(c)
	switch {
	case !ok, !s.Streaming, !s.AutoEnabled:
		return true
	case m.motionOf(cc) == scenemap.MotionStandby:
		return true
	}
	return s.CameraStates[c] == scenemap.StateOK
}

// entryMismatch は記録の項目が、記録時の入力 UUID・カメラと違うものになっているか（URL の変更など）。
// 食い違いは戻す理由になる。健康への照合の述語とは別（HB-MP-02、HB-AP-16）。
func (m *Machine) entryMismatch(e HiddenEntry) bool {
	it, ok := m.findItem(e.Key())
	if !ok {
		return false
	}
	calc := m.calcMap()
	in, has := calc.Inputs[it.SourceUUID]
	return it.SourceKind != scenemap.InputSource || it.SourceUUID != e.SourceUUID || !has || in.CameraID != e.CameraID
}

// entryRestorable は記録 e の項目を戻してよいか: カメラの戻してよい条件、または記録との食い違い。
func (m *Machine) entryRestorable(e HiddenEntry) bool {
	return m.canRestore(e.CameraID) || m.entryMismatch(e)
}

// ---- 映る V(R) と実際に表示 A(R) ----

// validEntry は項目 it の記録で、健康と表示に反映してよいもの（接続先は Hidden が今の Z だけを持つ。
// 入力 UUID と、その入力の今のカメラが記録と一致するとき）。
func (m *Machine) validEntry(k scenemap.ItemKey, it scenemap.Item, calc *scenemap.Map) *HiddenEntry {
	i := m.entryIdx(k)
	if i < 0 {
		return nil
	}
	e := &m.st.Hidden[i]
	if it.SourceKind != scenemap.InputSource || it.SourceUUID != e.SourceUUID {
		return nil
	}
	if in, ok := calc.Inputs[it.SourceUUID]; !ok || in.CameraID != e.CameraID {
		return nil
	}
	return e
}

// cameraSet はシーン uuid に映る（actual=false）か実際に表示（actual=true）のカメラの id。
// extra は仮に非表示とする項目（actual のとき）。
func (m *Machine) cameraSet(uuid string, actual bool, extra map[scenemap.ItemKey]bool) []string {
	s := &m.st
	if s.Map == nil {
		return nil
	}
	calc := m.calcMap()
	ids := scenemap.CamerasInWith(calc, uuid, func(k scenemap.ItemKey, it scenemap.Item) bool {
		if actual && extra[k] {
			return false
		}
		if e := m.validEntry(k, it, &calc); e != nil {
			if !actual {
				return true
			}
			switch e.State {
			case HiddenH:
				return false
			case HiddenP:
				if _, unsent := s.Unsent[k]; unsent {
					return it.Enabled
				}
				return false
			case HiddenU:
				// 未決着は表示と数える（spec の明示例外）。ただし新しい false を送って応答前か OK の間は、非表示と数える。
				if qi := m.qIdx(k, false); qi >= 0 {
					if q := s.ItemExpectations[qi]; !q.Responded || q.Result == ItemOK {
						return false
					}
				}
				return true
			}
		}
		return it.Enabled
	})
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func (m *Machine) actualCameras(uuid string, extra map[scenemap.ItemKey]bool) []string {
	return m.cameraSet(uuid, true, extra)
}

// protectedScenes は「空にしない」の対象のシーン: 今のシーンと、Proceed 済みで期待が残る切替先。
func (m *Machine) protectedScenes() []string {
	s := &m.st
	var out []string
	add := func(u string) {
		if u != "" && !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	add(s.Scene)
	for _, x := range s.Expectations {
		add(x.Scene)
	}
	return out
}

// wouldEmpty は extra を非表示にしたとき、配信中の保護対象の監視対象シーンの A(R) が空になるかを返す。
func (m *Machine) wouldEmpty(extra map[scenemap.ItemKey]bool) bool {
	s := &m.st
	if !s.Streaming {
		return false
	}
	for _, r := range m.protectedScenes() {
		if m.classOf(r) == ClassMonitored && len(m.actualCameras(r, extra)) == 0 {
			return true
		}
	}
	return false
}

// ---- カメラの項目 L(c) ----

// sortedContainers は地図のシーンとグループの UUID を辞書順で返す。
func (m *Machine) sortedContainers() []string {
	mp := m.st.Map
	if mp == nil {
		return nil
	}
	var ids []string
	for u := range mp.Scenes {
		ids = append(ids, u)
	}
	for u := range mp.Groups {
		ids = append(ids, u)
	}
	sort.Strings(ids)
	return slices.Compact(ids)
}

func (m *Machine) containerOf(u string) scenemap.Container {
	if c, ok := m.st.Map.Scenes[u]; ok {
		return c
	}
	return m.st.Map.Groups[u]
}

// itemsOfCamera はカメラ c の入力の項目（設定のカメラだけ）を、ContainerUUID 辞書順→ItemID 昇順で返す。
func (m *Machine) itemsOfCamera(c string) []itemRef {
	if m.st.Map == nil {
		return nil
	}
	calc := m.calcMap()
	var out []itemRef
	for _, u := range m.sortedContainers() {
		items := slices.Clone(m.containerOf(u).Items)
		sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		for _, it := range items {
			if it.SourceKind != scenemap.InputSource {
				continue
			}
			if in, ok := calc.Inputs[it.SourceUUID]; ok && in.CameraID == c {
				out = append(out, itemRef{Key: scenemap.ItemKey{ContainerUUID: u, ItemID: it.ID}, Item: it})
			}
		}
	}
	return out
}

type itemRef struct {
	Key  scenemap.ItemKey
	Item scenemap.Item
}

// pendingTarget は未完了の項目のバッチが、まだ決着していない対象としてこの項目を持っているかを返す。
func (m *Machine) pendingTarget(k scenemap.ItemKey) bool {
	for _, pb := range m.st.Pending {
		for _, pi := range pb.Items {
			if pi.Key == k {
				return true
			}
		}
	}
	return false
}

// itemPending はカメラ c の Hide / Show が未完了かを返す。
func (m *Machine) itemPending(c string) bool {
	for _, pb := range m.st.Pending {
		if (pb.Purpose == PurposeHideCamera || pb.Purpose == PurposeShowCamera) && pb.CameraID == c {
			return true
		}
	}
	return false
}

// retryOK は Hide / Show 共通の再送の間隔（前回の送出から5秒以上）を満たすか。
func (m *Machine) retryOK(c string, at time.Time) bool {
	last, ok := m.st.LastItemSent[c]
	return !ok || at.Sub(last) >= itemRetry
}

// ---- 隠した項目の記録の照合 ----

// resplit は今の接続先と地図のコレクションに合う記録を Hidden に、合わない記録を Retained に分ける。
func (m *Machine) resplit() {
	s := &m.st
	if s.Map == nil {
		return
	}
	s.Z = zone{m.ep.Host, m.ep.Port, s.Map.Collection}
	var hid, ret []HiddenEntry
	moved := false
	for _, e := range s.Hidden {
		if entryZone(e) == s.Z {
			hid = append(hid, e)
		} else {
			ret = append(ret, e)
			moved = true
		}
	}
	for _, e := range s.Retained {
		if entryZone(e) == s.Z {
			hid = append(hid, e)
			moved = true
		} else {
			ret = append(ret, e)
		}
	}
	if !moved {
		return
	}
	s.Hidden, s.Retained = hid, ret
	for k := range s.Unsent {
		if m.entryIdx(k) < 0 {
			delete(s.Unsent, k)
		}
	}
	m.sortHidden()
	m.touch()
}

func mapIsEmpty(mp *scenemap.Map) bool {
	return mp == nil || (len(mp.Scenes) == 0 && len(mp.Groups) == 0)
}

// reconcile は今の地図で Hidden の記録を照合する（OBSConnected、SceneMapChanged）。空の地図では何もしない。
func (m *Machine) reconcile(at time.Time) {
	s := &m.st
	if mapIsEmpty(s.Map) {
		return
	}
	for i := 0; i < len(s.Hidden); {
		e := &s.Hidden[i]
		k := e.Key()
		it, ok := m.findItem(k)
		if !ok {
			m.dropQKey(k)
			m.removeEntry(i)
			continue
		}
		if s.MAConfirm[k] && it.Enabled {
			m.removeEntry(i)
			continue
		}
		if it.SourceUUID != e.SourceUUID {
			i++
			continue
		}
		_, unsent := s.Unsent[k]
		switch e.State {
		case HiddenP:
			switch {
			case !it.Enabled && !unsent:
				e.State = HiddenH
				m.touch()
			case it.Enabled && !unsent && e.SettledAt != nil && at.Sub(*e.SettledAt) > pSweep && !m.hasQ(k):
				m.removeEntry(i)
				continue
			}
		case HiddenU:
			if !it.Enabled {
				e.State = HiddenH
				m.touch()
			}
		case HiddenH:
			if it.Enabled {
				m.removeEntry(i)
				continue
			}
		}
		i++
	}
}

// ---- 期限（E） ----

// expireItems は項目の期待の期限切れを処理し、決着した P の掃除をする。期限ちょうどは保つ。
func (m *Machine) expireItems(at time.Time) {
	s := &m.st
	var kept []ItemExpectation
	for _, q := range s.ItemExpectations {
		if !q.Responded || !at.After(q.Deadline) {
			kept = append(kept, q)
			continue
		}
		m.finishQ(q)
	}
	s.ItemExpectations = kept
	// 決着した P の掃除（10秒を超えたら。U は対象外）。
	if s.Map != nil {
		for i := 0; i < len(s.Hidden); {
			e := &s.Hidden[i]
			if e.State == HiddenP && e.SettledAt != nil && at.Sub(*e.SettledAt) > pSweep && !m.hasQ(e.Key()) {
				if it, ok := m.findItem(e.Key()); ok && it.Enabled {
					m.removeEntry(i)
					continue
				}
			}
			i++
		}
	}
}

// finishQ は期限切れの項目の期待 q の終わりの処理: 無イベントの補正（Hide / Show の OK だけ）と、地図 true の false 期待の整理。
func (m *Machine) finishQ(q ItemExpectation) {
	s := &m.st
	if s.Map == nil {
		return
	}
	k := q.Key
	hasEntry := m.entryIdx(k) >= 0
	compensate := q.Result == ItemOK && !q.SawEvent && !q.Contra && (q.Purpose == PurposeHideCamera || q.Purpose == PurposeShowCamera)
	if compensate {
		// 応答が OK で、期限まで期待どおりのイベントが来なかった: すでにその値だったとみなし、期待どおりのイベントと同じ扱いにする。
		if m.setMapItem(k, q.Value) {
			m.applyMatchedEvent(k, q.Value)
		}
		return
	}
	if !q.Value && hasEntry {
		if i := m.entryIdx(k); i >= 0 {
			e := s.Hidden[i]
			if it, ok := m.findItem(k); ok && it.Enabled && (e.State == HiddenH || e.State == HiddenP) {
				m.removeEntry(i)
			}
		}
	}
}

// applyMatchedEvent は期待どおりの値 v のイベント（または無イベントの補正）が来たときの記録の変化。
func (m *Machine) applyMatchedEvent(k scenemap.ItemKey, v bool) {
	i := m.entryIdx(k)
	if i < 0 {
		return
	}
	e := &m.st.Hidden[i]
	if v {
		m.removeEntry(i)
		return
	}
	if e.State != HiddenH {
		if _, unsent := m.st.Unsent[k]; !unsent {
			e.State = HiddenH
			m.touch()
		}
	}
}

// ---- 項目 Event ----

func (m *Machine) onItemEnabledEvent(e ItemEnabledChanged) []Batch {
	s := &m.st
	if !s.Connected || s.Map == nil {
		return nil
	}
	k := scenemap.ItemKey{ContainerUUID: e.SceneUUID, ItemID: e.ItemID}
	changed := false
	// Q の照合。同じ項目の中だけ。一致しても消さず SawEvent を立てる。反対の値は Contra。
	matched := false
	for i := range s.ItemExpectations {
		q := &s.ItemExpectations[i]
		if q.Key != k {
			continue
		}
		if q.Value == e.Enabled {
			if !matched && !q.SawEvent {
				q.SawEvent = true
				matched = true
				changed = true
			}
		} else if !q.Contra {
			q.Contra = true
		}
	}
	// 記録の更新。
	if _, unsent := s.Unsent[k]; unsent {
		// 予約しただけで、まだ false を送っていない。人の操作でも記録は触らない（承認で Skip か送るかを決める）。
	} else if ei := m.entryIdx(k); ei >= 0 {
		ent := s.Hidden[ei]
		changed = true
		if !e.Enabled {
			switch ent.State {
			case HiddenU:
				s.Hidden[ei].State = HiddenH
				m.touch()
			case HiddenP:
				if _, unsent := s.Unsent[k]; !unsent {
					s.Hidden[ei].State = HiddenH
					m.touch()
				}
			}
		} else if matched {
			m.removeEntry(ei)
		} else {
			// 期待にない true。人が戻したか、遅れた応答。
			falseQ := m.qIdx(k, false) >= 0
			if !m.canRestore(ent.CameraID) {
				s.UserShown[ent.CameraID] = true
			}
			if !falseQ && ent.State != HiddenU {
				m.removeEntry(ei)
			}
		}
	}
	// 地図の更新。
	if it, ok := m.findItem(k); ok && it.Enabled != e.Enabled {
		m.setMapItem(k, e.Enabled)
		changed = true
	}
	if !changed {
		return nil
	}
	m.refine()
	return m.jb(e.At, false)
}

// ---- 結果 ----

func (m *Machine) onItemRequestDone(e ItemRequestDone) []Batch {
	s := &m.st
	rec, ok := s.ItemReceipts[e.ReqID]
	if !ok || rec.Result != 0 {
		return nil
	}
	rec.Result = e.Result
	rec.ResultAt = e.At
	s.ItemReceipts[e.ReqID] = rec
	if pb, pending := s.Pending[rec.BatchID]; pending {
		if e.Result == ItemError || e.Result == ItemTimeout {
			pb.Failures++
			s.Pending[rec.BatchID] = pb
		}
	} else {
		delete(s.ItemReceipts, e.ReqID)
	}
	respond := func() {
		if qi := m.qByReq(e.ReqID); qi >= 0 {
			q := &s.ItemExpectations[qi]
			q.Responded = true
			q.RespondedAt = e.At
			q.Result = e.Result
			q.Deadline = e.At.Add(itemQWindow)
		}
	}
	dropQ := func() {
		if qi := m.qByReq(e.ReqID); qi >= 0 {
			s.ItemExpectations = slices.Delete(s.ItemExpectations, qi, qi+1)
		}
	}
	k := rec.Key
	ei := m.entryIdx(k)
	if !rec.Value {
		switch e.Result {
		case ItemOK:
			respond()
			if ei >= 0 && s.Hidden[ei].State == HiddenP && s.Hidden[ei].SettledAt == nil {
				t := e.At
				s.Hidden[ei].SettledAt = &t
				m.touch()
			}
		case ItemTimeout:
			respond()
			if ei >= 0 && s.Hidden[ei].State == HiddenP {
				t := e.At
				s.Hidden[ei].State = HiddenU
				s.Hidden[ei].SettledAt = &t
				m.touch()
			}
		case ItemError:
			dropQ()
			if ei >= 0 && s.Hidden[ei].State == HiddenP {
				m.removeEntry(ei)
			}
		case ItemNotFound:
			dropQ()
			if ei >= 0 {
				m.removeEntry(ei)
			}
		}
	} else {
		switch e.Result {
		case ItemOK:
			respond()
			if rec.Scope == RestoreTrackedItems && ei >= 0 {
				m.removeEntry(ei)
			}
		case ItemTimeout:
			respond()
		case ItemError:
			dropQ()
			delete(s.MAConfirm, k)
		case ItemNotFound:
			dropQ()
			if ei >= 0 {
				m.removeEntry(ei)
			}
		}
	}
	m.refine()
	return m.jb(e.At, false)
}

// ---- バッチの発行 ----

// emitItem は項目のバッチを1本作る。
func (m *Machine) emitItem(p Purpose, camera string, items []pendItem, actions []Action, scope RestoreScope, fromCmd bool) Batch {
	s := &m.st
	id := s.NextBatchID
	s.NextBatchID++
	s.Pending[id] = PendingBatch{
		BatchID: id, Purpose: p,
		SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch,
		CameraID: camera, Scope: scope, FromCommand: fromCmd, Items: items,
	}
	bi := make([]BatchItem, len(items))
	for i, it := range items {
		bi[i] = BatchItem{SceneUUID: it.Key.ContainerUUID, ItemID: it.Key.ItemID, SourceUUID: it.Source}
	}
	return Batch{
		BatchID: id, Purpose: p,
		SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch,
		Actions: actions, CameraID: camera, Items: bi, RestoreScope: scope, FromCommand: fromCmd,
	}
}

func itemActions(items []pendItem) []Action {
	acts := make([]Action, len(items))
	for i, it := range items {
		acts[i] = SetItemEnabled{SceneUUID: it.Key.ContainerUUID, ItemID: it.Key.ItemID, Enabled: it.Value}
	}
	return acts
}

// cameraOrder は Hide / Show の走査順: 設定の順、設定外の id（一覧にだけある）は辞書順。
func (m *Machine) cameraOrder() []string {
	ids := m.cameraIDs()
	var extra []string
	for _, e := range m.st.Hidden {
		if !slices.Contains(ids, e.CameraID) && !slices.Contains(extra, e.CameraID) {
			extra = append(extra, e.CameraID)
		}
	}
	sort.Strings(extra)
	return append(ids, extra...)
}

// genShows は ShowCamera の候補を出す。
func (m *Machine) genShows(at time.Time) []Batch {
	s := &m.st
	if !s.Connected || s.Map == nil || s.StoppingReceived {
		return nil
	}
	var out []Batch
	for _, c := range m.cameraOrder() {
		if m.itemPending(c) || !m.retryOK(c, at) {
			continue
		}
		var items []pendItem
		for _, e := range s.Hidden {
			if e.CameraID != c || !m.entryRestorable(e) {
				continue
			}
			it, ok := m.findItem(e.Key())
			if !ok || it.Enabled || m.hasQ(e.Key()) || m.pendingTarget(e.Key()) {
				continue
			}
			items = append(items, pendItem{Key: e.Key(), Source: e.SourceUUID, Value: true})
		}
		if len(items) == 0 {
			continue
		}
		var acts []Action
		if in := m.restartTargets(c); len(in) > 0 {
			acts = append(acts, RestartMedia{Inputs: in, Wait: m.cfg.RestartWait})
		}
		acts = append(acts, itemActions(items)...)
		s.LastItemSent[c] = at
		out = append(out, m.emitItem(PurposeShowCamera, c, items, acts, RestoreNone, false))
	}
	return out
}

// restartTargets は Show の Restart の対象: c が OK のとき、c の入力のうち地図で表示の項目が1つもない入力（UUID の辞書順）。
func (m *Machine) restartTargets(c string) []string {
	s := &m.st
	if _, ok := m.cameraCfg(c); !ok || s.CameraStates[c] != scenemap.StateOK {
		return nil
	}
	calc := m.calcMap()
	shown := map[string]bool{}
	for _, u := range m.sortedContainers() {
		for _, it := range m.containerOf(u).Items {
			if it.Enabled && it.SourceKind == scenemap.InputSource {
				shown[it.SourceUUID] = true
			}
		}
	}
	var out []string
	for uuid, in := range calc.Inputs {
		if in.CameraID == c && !shown[uuid] {
			out = append(out, uuid)
		}
	}
	sort.Strings(out)
	return out
}

// genHides は HideCamera の候補を出す。同じ Handle の中で先に候補にした項目は、あとの候補の「空にしない」検査に重ねる。
func (m *Machine) genHides(at time.Time) []Batch {
	s := &m.st
	if !s.Connected || s.Map == nil || s.StoppingReceived || m.hideSuppressed() {
		return nil
	}
	var out []Batch
	extra := map[scenemap.ItemKey]bool{}
	for _, c := range m.cameraIDs() {
		if !m.canHide(c) || m.itemPending(c) || !m.retryOK(c, at) {
			continue
		}
		var items []pendItem
		for _, r := range m.itemsOfCamera(c) {
			if !r.Item.Enabled || m.hasQ(r.Key) || m.pendingTarget(r.Key) {
				continue
			}
			items = append(items, pendItem{Key: r.Key, Source: r.Item.SourceUUID, Value: false})
		}
		if len(items) == 0 {
			continue
		}
		trial := maps_clone(extra)
		for _, it := range items {
			trial[it.Key] = true
		}
		if m.wouldEmpty(trial) {
			continue
		}
		extra = trial
		s.LastItemSent[c] = at
		out = append(out, m.emitItem(PurposeHideCamera, c, items, itemActions(items), RestoreNone, false))
	}
	return out
}

func maps_clone(m map[scenemap.ItemKey]bool) map[scenemap.ItemKey]bool {
	c := make(map[scenemap.ItemKey]bool, len(m)+4)
	for k, v := range m {
		c[k] = v
	}
	return c
}

// hideSuppressed は StartScene が未完了か、その期待が残る間（Hide を出さない）。
func (m *Machine) hideSuppressed() bool {
	for _, pb := range m.st.Pending {
		if pb.Purpose == PurposeStartScene {
			return true
		}
	}
	for _, x := range m.st.Expectations {
		if x.Purpose == PurposeStartScene {
			return true
		}
	}
	return false
}

// waitingHidden は戻り先 R が待っている隠した項目があるか: R に映る OK で使えるカメラのうち、一覧に地図で false の項目があるもの。
func (m *Machine) waitingHidden(r string) bool {
	s := &m.st
	for _, c := range m.cameraSet(r, false, nil) {
		if s.CameraStates[c] != scenemap.StateOK || m.unusable(c) {
			continue
		}
		for _, e := range s.Hidden {
			if e.CameraID != c {
				continue
			}
			if it, ok := m.findItem(e.Key()); ok && !it.Enabled {
				return true
			}
		}
	}
	return false
}

// ---- Stopping、すべて表示に戻す ----

func (m *Machine) onStopping(e Stopping) []Batch {
	s := &m.st
	if s.StoppingReceived {
		return nil
	}
	s.StoppingReceived = true
	if !s.Connected || s.RestoreIssued {
		return nil
	}
	s.RestoreIssued = true
	if len(s.Hidden) == 0 || mapIsEmpty(s.Map) {
		// 一覧が空、または空の地図（古い OBS など）では、何も戻さず記録を残す（次の起動で戻す）。
		return nil
	}
	items := make([]pendItem, 0, len(s.Hidden))
	for _, h := range s.Hidden {
		items = append(items, pendItem{Key: h.Key(), Source: h.SourceUUID, Value: true})
	}
	return []Batch{m.emitItem(PurposeRestoreAll, "", items, itemActions(items), RestoreTrackedItems, false)}
}

// onShowAll は手動の「すべて表示に戻す」。非表示のカメラの項目と、現 Z の記録と、false の期待の項目を対象にする。
func (m *Machine) onShowAll(e Command) ([]Batch, error) {
	s := &m.st
	if !s.Connected {
		return nil, ErrNotConnected{}
	}
	if slices.ContainsFunc(m.pendingList(), func(pb PendingBatch) bool {
		return pb.Purpose == PurposeRestoreAll && pb.Scope == RestoreAllCameraItems
	}) {
		return nil, ErrInvalidState{Reason: ReasonSamePending}
	}
	s.MAFloor = s.NextBatchID
	keys := map[scenemap.ItemKey]string{}
	if s.Map != nil {
		calc := m.calcMap()
		for _, u := range m.sortedContainers() {
			for _, it := range m.containerOf(u).Items {
				if it.SourceKind != scenemap.InputSource || it.Enabled {
					continue
				}
				if in, ok := calc.Inputs[it.SourceUUID]; ok && in.CameraID != "" {
					keys[scenemap.ItemKey{ContainerUUID: u, ItemID: it.ID}] = it.SourceUUID
				}
			}
		}
	}
	for _, h := range s.Hidden {
		if _, ok := m.findItem(h.Key()); ok {
			keys[h.Key()] = h.SourceUUID
		}
	}
	for _, q := range s.ItemExpectations {
		if !q.Value {
			if it, ok := m.findItem(q.Key); ok {
				if _, have := keys[q.Key]; !have {
					keys[q.Key] = it.SourceUUID
				}
			}
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	var ks []scenemap.ItemKey
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].ContainerUUID != ks[j].ContainerUUID {
			return ks[i].ContainerUUID < ks[j].ContainerUUID
		}
		return ks[i].ItemID < ks[j].ItemID
	})
	items := make([]pendItem, len(ks))
	for i, k := range ks {
		items[i] = pendItem{Key: k, Source: keys[k], Value: true}
	}
	return []Batch{m.emitItem(PurposeRestoreAll, "", items, itemActions(items), RestoreAllCameraItems, true)}, nil
}

func (m *Machine) pendingList() []PendingBatch {
	out := make([]PendingBatch, 0, len(m.st.Pending))
	for _, pb := range m.st.Pending {
		out = append(out, pb)
	}
	return out
}

// ---- HiddenRestored、HiddenWriteFailed ----

func (m *Machine) onHiddenRestored(e HiddenRestored) {
	s := &m.st
	if s.HiddenRestoredDone || s.Connected || s.StoppingReceived {
		return
	}
	s.HiddenRestoredDone = true
	for _, h := range e.Entries {
		s.Retained = append(s.Retained, h.clone())
	}
}

func (m *Machine) onHiddenWriteFailed(e HiddenWriteFailed) {
	s := &m.st
	pb, ok := s.Pending[e.BatchID]
	if !ok || !pb.Reserved || pb.WriteFailed {
		return
	}
	pb.WriteFailed = true
	m.rollbackBatch(&pb, true)
	s.Pending[e.BatchID] = pb
	m.recordError(e.At, "隠した項目を記録できないため、隠すのを止めました")
}

// rollbackBatch は、バッチが作った未送信の予約を予約前へ戻す。all なら送った分（保存に失敗した最初の false）の期待と台帳も戻す。
func (m *Machine) rollbackBatch(pb *PendingBatch, all bool) {
	s := &m.st
	for i := range pb.Items {
		pi := &pb.Items[i]
		if pi.Sent && all {
			if qi := m.qByReq(pi.ReqID); qi >= 0 {
				s.ItemExpectations = slices.Delete(s.ItemExpectations, qi, qi+1)
			}
			delete(s.ItemReceipts, pi.ReqID)
			pi.Sent = false
			pi.ReqID = 0
			if pb.Sent > 0 {
				pb.Sent--
			}
			if pi.Created {
				s.Unsent[pi.Key] = pb.BatchID
			}
		}
		if pi.Created {
			if bid, unsent := s.Unsent[pi.Key]; unsent && bid == pb.BatchID {
				if ei := m.entryIdx(pi.Key); ei >= 0 && s.Hidden[ei].State == HiddenP {
					m.removeEntry(ei)
				}
				delete(s.Unsent, pi.Key)
			}
		}
	}
}

// cloneHidden は記録の複製（外から変えても内部は変わらない）。空は nil。
func cloneHidden(es []HiddenEntry) []HiddenEntry {
	if len(es) == 0 {
		return nil
	}
	out := make([]HiddenEntry, len(es))
	for i, e := range es {
		out[i] = e.clone()
	}
	return out
}
