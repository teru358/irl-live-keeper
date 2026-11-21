package switcher

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 第5節の4つ（表の6章 HB-INV-01〜04）を、switcher の範囲で確かめる性質のテスト。
// 乱数で入力の列（CamerasChanged、ItemEnabledChanged、ItemRequestDone、SceneChanged、SceneMapChanged、StudioModeChanged、
// 配信の開始と停止、Command、OBSConnected / OBSDisconnected、承認）を作る。判定は Machine の V / A / Health を使わず、
// このファイルの小さな模型（偽の OBS の事実と、Machine に渡した Event から立てた観測）でする。
// 偽の OBS との結合とファイルは M-12。種は固定（run ごとに種 = 通し番号）で、落ちたら種と列を出す。

type pkey = scenemap.ItemKey

type preq struct {
	key     pkey
	value   bool
	batchID int
	apply   bool // 偽の OBS が値を適用する（結果が Timeout のときは適用されたか不明）
	result  ItemResult
	at      int // 結果を返す時刻 ms
}

type pbatchRun struct {
	b       Batch
	next    int
	failed  bool
	waiting int // 結果待ちの項目の要求
}

type pevent struct {
	at    int
	ev    Event
	human bool // 人の操作として偽の OBS が出したイベント（本体の要求の適用ではない）
}

// pscene は模型が持つ、本体が Proceed した SetScene の期待の台帳（Machine の Expectations は読まない）。
type pscene struct {
	to        string
	reqID     int
	responded bool
	respAt    int
}

type pfake struct {
	t    *testing.T
	seed int64
	rng  *rand.Rand
	m    *Machine
	now  int // ms

	// 偽の OBS の事実
	items     map[pkey]bool
	scene     string
	streaming bool
	studio    bool
	connected bool
	base      *scenemap.Map
	cstates   map[string]scenemap.CameraState

	// 設定（Machine に渡した値）
	autoOn   bool
	hide     map[string]bool // カメラごとの on_bad=hide
	studioHd string

	// Machine に渡した Event から立てた観測
	seen      map[pkey]bool
	seenScene string
	seenState map[string]scenemap.CameraState
	seenStrm  bool
	seenStud  bool

	lastItemEvAt int
	queue        []*pbatchRun
	reqs         map[uint64]*preq
	events       []pevent
	humanHid     map[pkey]bool // 人が隠した項目（人の false のイベントを渡した）
	scenePend    []pscene      // 独立の期待の台帳
	trueUntil    map[pkey]int  // 本体の true の期待が残る時刻（応答 + 2秒。応答前は大きな値）。同じ値の人の操作と見分けられない間（M-9）
	curHuman     bool
	evAt         int
	markHuman    bool
	mode         int // 回復の区間の型
	log          []string

	// 判定用
	needRecord   map[pkey]bool   // 本体が false を送って、まだ true と決着していない項目（記録がなければ失敗）
	strictHidden map[pkey]bool   // 本体が false を送って、Timeout でない項目
	interval     map[string]bool // 人が戻した、同じ不調の区間
	stopped      bool
}

func pkeyName(k pkey) string { return fmt.Sprintf("%s#%d", letter(k.ContainerUUID), k.ItemID) }

const (
	gUUID = "uuid-group"
	oUUID = "uuid-opening"
)

// pMap は無作為に選んだ構造の地図（入れ子、共有の末端、ひし形、同カメラ複数入力）。
func pMap(r *rand.Rand) *scenemap.Map {
	m := hbMap()
	m.Inputs[inSub+"2"] = scenemap.Input{Name: "sub-src2", CameraID: camSub}
	it := func(id int, src string) scenemap.Item { return scenemapItem(id, src, true) }
	put := func(u string, items ...scenemap.Item) {
		c := m.Scenes[u]
		c.Items = items
		m.Scenes[u] = c
	}
	m.Groups[gUUID] = scenemapContainer("G", it(1, inSub))
	switch r.Intn(4) {
	case 0: // 既定
	case 1: // 共有の末端: A と D と B が同じ入力 is
		put(camera, it(1, inMain), it(2, inSub))
		put(camera2, it(1, inMain), it(2, inSub))
		put(deskScene, it(1, inSub), it(2, inSub+"2"))
	case 2: // ひし形: A に is の直接の項目と、グループ経由の道
		put(camera, it(1, inMain), it(2, inSub), scenemapGroupItem(3, gUUID, true))
		put(deskScene, scenemapGroupItem(1, gUUID, true))
		put(oUUID, scenemapSceneItem(1, camera), scenemapSceneItem(2, deskScene))
	default: // 入れ子: O が A と D を含む。D は s だけ
		put(oUUID, scenemapSceneItem(1, camera), scenemapSceneItem(2, deskScene))
		put(deskScene, it(1, inSub))
	}
	return m
}

func scenemapSceneItem(id int, src string) scenemap.Item {
	return scenemap.Item{ID: id, SourceUUID: src, SourceKind: scenemap.SceneSource, Enabled: true}
}

func newPfake(t *testing.T, seed int64) *pfake {
	f := &pfake{t: t, seed: seed, rng: rand.New(rand.NewSource(seed))}
	f.m = New(Config{RestartWait: W, Endpoint: Endpoint{Host: hbHost, Port: hbPort}})
	f.base = pMap(f.rng)
	if _, ok := f.base.Scenes[oUUID]; !ok {
		f.base.Scenes[oUUID] = scenemap.Container{Name: "Opening"}
	}
	f.items = map[pkey]bool{}
	for u, c := range f.base.Scenes {
		for _, i := range c.Items {
			f.items[pkey{ContainerUUID: u, ItemID: i.ID}] = i.Enabled
		}
	}
	for u, c := range f.base.Groups {
		for _, i := range c.Items {
			f.items[pkey{ContainerUUID: u, ItemID: i.ID}] = i.Enabled
		}
	}
	f.scene = camera
	f.streaming = true
	f.connected = true
	f.autoOn = true
	f.hide = map[string]bool{camMain: false, camSub: true}
	f.studioHd = StudioHideStandby
	f.cstates = map[string]scenemap.CameraState{camMain: OK, camSub: OK}
	f.reqs = map[uint64]*preq{}
	f.trueUntil = map[pkey]int{}
	f.humanHid = map[pkey]bool{}
	f.needRecord = map[pkey]bool{}
	f.strictHidden = map[pkey]bool{}
	f.interval = map[string]bool{}
	f.seen = map[pkey]bool{}
	f.seenState = copyStates(f.cstates)
	f.handle(f.cfgEv())
	f.handle(CamerasChanged{At: f.at(), States: copyStates(f.cstates)})
	f.handle(f.connEv())
	return f
}

func copyStates(s map[string]scenemap.CameraState) map[string]scenemap.CameraState {
	c := map[string]scenemap.CameraState{}
	for k, v := range s {
		c[k] = v
	}
	return c
}

func (f *pfake) at() time.Time { return at(f.now) }

func (f *pfake) cfgEv() Event {
	on := func(c string) scenemap.Motion {
		if f.hide[c] {
			return scenemap.MotionHide
		}
		return scenemap.MotionStandby
	}
	return ScenesConfigured{At: f.at(), Standby: "Standby", AutoEnabled: f.autoOn, StudioModeHide: f.studioHd,
		Cameras: []CameraConfig{{ID: camMain, OnBad: on(camMain)}, {ID: camSub, OnBad: on(camSub)}}}
}

// truthMap は偽の OBS の今の値で作った地図。
func (f *pfake) truthMap() *scenemap.Map {
	m := cloneTestMap(f.base)
	for k, v := range f.items {
		setItem(m, k.ContainerUUID, k.ItemID, v)
	}
	return m
}

func (f *pfake) connEv() Event {
	f.seen = map[pkey]bool{}
	for k, v := range f.items {
		f.seen[k] = v
	}
	f.seenScene, f.seenStrm, f.seenStud = f.scene, f.streaming, f.studio
	f.seenState = copyStates(f.cstates)
	// 読み直した地図で true と確かめられた項目は、もう本体が隠したままではない。
	for k := range f.needRecord {
		if f.seen[k] {
			delete(f.needRecord, k)
			delete(f.strictHidden, k)
		}
	}
	return OBSConnected{At: f.at(), Streaming: f.streaming, Scene: nameOf(f.scene), SceneUUID: f.scene, TransitionMs: ip(300),
		Map: f.truthMap(), StudioMode: f.studio}
}

func (f *pfake) logf(format string, a ...any) {
	f.log = append(f.log, fmt.Sprintf("[%6dms] ", f.now)+fmt.Sprintf(format, a...))
}

func (f *pfake) fail(format string, a ...any) {
	f.t.Helper()
	var tail []string
	for _, l := range f.log {
		if !strings.Contains(l, "switcher.Tick") {
			tail = append(tail, l)
		}
	}
	n := 40
	if v, err := strconv.Atoi(os.Getenv("HB_TAIL")); err == nil {
		n = v
	}
	if len(tail) > n {
		tail = tail[len(tail)-n:]
	}
	f.t.Fatalf("種 %d: %s\n--- 直近の列 ---\n%s", f.seed, fmt.Sprintf(format, a...), strings.Join(tail, "\n"))
}

// handle は Event を Machine に渡し、出た列を実行待ちに入れる。観測もここで更新する。
func (f *pfake) handle(ev Event) {
	f.t.Helper()
	f.observe(ev)
	f.logf("-> %T %s", ev, shortEv(ev))
	bs, err := f.m.Handle(ev)
	if err != nil {
		f.logf("   cmdErr %v", err)
	}
	for _, b := range bs {
		f.logf("   out %s #%d", desc(b), b.BatchID)
		f.queue = append(f.queue, &pbatchRun{b: b})
	}
	f.check()
}

func shortEv(ev Event) string {
	switch e := ev.(type) {
	case ItemEnabledChanged:
		return fmt.Sprintf("%s=%v", pkeyName(pkey{ContainerUUID: e.SceneUUID, ItemID: e.ItemID}), e.Enabled)
	case CamerasChanged:
		return fmt.Sprintf("m=%s s=%s", e.States[camMain], e.States[camSub])
	case ItemRequestDone:
		return fmt.Sprintf("r%d %v", e.ReqID, e.Result)
	case SceneChanged:
		return letter(e.SceneUUID)
	case Command:
		return e.Kind.String()
	case StreamStateChanged:
		return fmt.Sprint(e.Streaming)
	case StudioModeChanged:
		return fmt.Sprint(e.Enabled)
	case BatchDone:
		return fmt.Sprintf("#%d %d", e.BatchID, e.Result)
	}
	return ""
}

// observe は Machine に渡す Event から、独立の観測（見えている値、camera の状態、配信）を更新する。
func (f *pfake) observe(ev Event) {
	switch e := ev.(type) {
	case ItemEnabledChanged:
		k := pkey{ContainerUUID: e.SceneUUID, ItemID: e.ItemID}
		f.seen[k] = e.Enabled
		if f.curHuman {
			if e.Enabled {
				delete(f.humanHid, k)
			} else {
				f.humanHid[k] = true
			}
		} else {
			delete(f.humanHid, k)
		}
		if e.Enabled {
			if f.curHuman && f.needRecord[k] && f.evAt > f.trueUntil[k] {
				// 本体が隠した項目を、人が戻した（模型が発生元を持つ）。本体の true の期待が残る間は、同じ値の人の操作と
				// 見分けられない（M-9 の既知制約）ので除く。それ以外で、同じ不調の区間なら印を立てる。
				c := f.cameraOfKey(k)
				if c != "" && f.hideBad(c) {
					f.interval[c] = true
				}
			}
			delete(f.needRecord, k)
			delete(f.strictHidden, k)
		}
	case CamerasChanged:
		for c, v := range e.States {
			if f.seenState[c] != v {
				f.seenState[c] = v
				if v == OK {
					delete(f.interval, c)
				}
			}
		}
	case StreamStateChanged:
		f.seenStrm = e.Streaming
		if !e.Streaming {
			f.interval = map[string]bool{}
			f.scenePend = nil
		}
	case StudioModeChanged:
		f.seenStud = e.Enabled
	case SceneChanged:
		f.seenScene = e.SceneUUID
		hit := -1
		for i, p := range f.scenePend {
			if p.to == e.SceneUUID {
				hit = i
				break
			}
		}
		if hit < 0 {
			f.scenePend = nil // 人の切替は、期待を全部消す
		} else {
			var kept []pscene
			for _, p := range f.scenePend[hit+1:] {
				if p.to != e.SceneUUID {
					kept = append(kept, p)
				}
			}
			f.scenePend = kept
		}
	case SceneRequestDone:
		for i := range f.scenePend {
			if f.scenePend[i].reqID == e.ReqID {
				f.scenePend[i].responded = true
				f.scenePend[i].respAt = f.evAt
			}
		}
	case OBSDisconnected:
		f.scenePend = nil
	}
}

func (f *pfake) cameraOfKey(k pkey) string {
	var found string
	var c scenemap.Container
	var ok bool
	if c, ok = f.base.Scenes[k.ContainerUUID]; !ok {
		c, ok = f.base.Groups[k.ContainerUUID]
	}
	if !ok {
		return ""
	}
	for _, it := range c.Items {
		if it.ID == k.ItemID && it.SourceKind == scenemap.InputSource {
			found = f.base.Inputs[it.SourceUUID].CameraID
		}
	}
	return found
}

// hideBad はカメラ c が「有効な hide で不良、配信中、自動有効」の区間にいるか（観測と設定だけから求める）。
func (f *pfake) hideBad(c string) bool {
	if !f.hide[c] || !f.autoOn || !f.seenStrm || f.seenState[c] == OK {
		return false
	}
	return !f.seenStud || f.studioHd == StudioHideHide
}

// cameras は項目の値 enabled のもとで、シーン u に映るカメラの集合（入れ子をたどる、循環はたどらない）。
func (f *pfake) cameras(u string, enabled func(pkey) bool) map[string]bool {
	out := map[string]bool{}
	var walk func(u string, kind scenemap.SourceKind, depth int)
	walk = func(u string, kind scenemap.SourceKind, depth int) {
		if depth > 6 {
			return
		}
		var c scenemap.Container
		var ok bool
		if kind == scenemap.GroupSource {
			c, ok = f.base.Groups[u]
		} else {
			c, ok = f.base.Scenes[u]
		}
		if !ok {
			return
		}
		for _, it := range c.Items {
			if !enabled(pkey{ContainerUUID: u, ItemID: it.ID}) {
				continue
			}
			switch it.SourceKind {
			case scenemap.InputSource:
				if id := f.base.Inputs[it.SourceUUID].CameraID; id != "" {
					out[id] = true
				}
			default:
				walk(it.SourceUUID, it.SourceKind, depth+1)
			}
		}
	}
	walk(u, scenemap.SceneSource, 0)
	return out
}

// ---- 不変条件 ----

func (f *pfake) check() {
	f.t.Helper()
	s := &f.m.st
	// 記録の構造
	seen := map[pkey]bool{}
	for _, e := range s.Hidden {
		k := e.Key()
		if seen[k] {
			f.fail("記録の鍵が重複 %s", pkeyName(k))
		}
		seen[k] = true
		if e.State < HiddenP || e.State > HiddenU {
			f.fail("記録の状態が不正 %v", e.State)
		}
	}
	for k := range s.Unsent {
		i := f.m.entryIdx(k)
		if i < 0 || s.Hidden[i].State != HiddenP {
			f.fail("未送信の予約 %s に P の記録がない", pkeyName(k))
		}
	}
	// HB-INV-01: 本体が false を送った項目は、true と決着するまで、正しい Z・入力 UUID の記録にある（Snapshot の写しで見る）。
	for k := range f.needRecord {
		f.wantRecorded(k, "記録がない（本体が false を送った）")
	}
}

// protectedTargets は今のシーンと、模型の台帳にある Proceed 済みの切替先（応答から 300ms + 3秒を過ぎたものは除く）。
func (f *pfake) protectedTargets() []string {
	targets := []string{f.seenScene}
	var kept []pscene
	for _, p := range f.scenePend {
		if p.responded && f.now > p.respAt+3300 {
			continue
		}
		kept = append(kept, p)
		targets = append(targets, p.to)
	}
	f.scenePend = kept
	return targets
}

// onProceedScene は SetScene の Proceed の時点（適用の直後）の判定: 配信中の監視対象の切替先を空にしない。
func (f *pfake) onProceedScene(to string, reqID int) {
	f.t.Helper()
	f.scenePend = append(f.scenePend, pscene{to: to, reqID: reqID})
	if !f.seenStrm || to == standby {
		return
	}
	plain := func(x pkey) bool { return f.seen[x] || f.needRecord[x] }
	// 承認までに届いた人の非表示とのすれ違いで0になるものは、本体の操作の違反に数えない（既知の制約）。
	hidden := func(x pkey) bool { return (f.seen[x] || f.humanHid[x]) && !f.strictHidden[x] }
	if len(f.cameras(to, plain)) > 0 && len(f.cameras(to, hidden)) == 0 {
		f.fail("HB-INV-02: SetScene(%s) の適用で実際の表示が空のシーンへ切り替わる", letter(to))
	}
}

// onProceedFalse は false の Proceed の直前の判定（INV-01 の保存の時点、INV-02、INV-03）。
func (f *pfake) onProceedFalse(b Batch, k pkey, p Proceed) {
	f.t.Helper()
	// INV-01: 実送信の前に、保存する一覧の写し（Snapshot）にこの項目の正しい記録がある（送信前の保存障壁）。
	f.wantRecorded(k, "Proceed(false) の時点で記録がない")
	// INV-03: 人が戻した区間では、新しい false を送らない。
	if c := f.cameraOfKey(k); c != "" && f.interval[c] {
		f.fail("HB-INV-03: 人が戻した区間（%s）で新しい false を送った %s", c, pkeyName(k))
	}
	// INV-02: 配信中の今のシーンと、Proceed 済みの切替先の監視対象シーンを空にしない。
	if !f.seenStrm {
		return
	}
	hidden := func(x pkey) bool {
		if x == k {
			return false
		}
		if f.strictHidden[x] {
			return false
		}
		return f.seen[x]
	}
	plain := func(x pkey) bool { return f.seen[x] || f.needRecord[x] }
	targets := f.protectedTargets()
	for _, r := range targets {
		if r == standby {
			continue
		}
		if len(f.cameras(r, plain)) == 0 {
			continue // 人が空にした、または監視対象でない
		}
		if len(f.cameras(r, hidden)) == 0 {
			f.fail("HB-INV-02: false(%s) で %s の実際の表示が空になる", pkeyName(k), letter(r))
		}
	}
}

// ---- 実行役の模型 ----

func (f *pfake) allKeys() []pkey {
	var ks []pkey
	for k := range f.items {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].ContainerUUID != ks[j].ContainerUUID {
			return ks[i].ContainerUUID < ks[j].ContainerUUID
		}
		return ks[i].ItemID < ks[j].ItemID
	})
	return ks
}

func (f *pfake) schedule(afterMs int, ev Event) {
	at := f.now + afterMs
	// OBS のイベントは変化の順に届く。項目のイベントは、前に予約した項目のイベントより後にする。
	if _, ok := ev.(ItemEnabledChanged); ok {
		if at <= f.lastItemEvAt {
			at = f.lastItemEvAt + 1
		}
		f.lastItemEvAt = at
	}
	f.logf("   予約 @%dms %T %s", at, ev, shortEv(ev))
	f.events = append(f.events, pevent{at: at, ev: ev, human: f.markHuman})
}

func (f *pfake) scheduleH(afterMs int, ev Event) {
	f.markHuman = true
	f.schedule(afterMs, ev)
	f.markHuman = false
}

// deliver は期限の来たイベントを時刻の順に渡す。切断中のイベントは捨てる（OBSConnected が最新の値を運ぶ）。
func (f *pfake) deliver() {
	for {
		sort.SliceStable(f.events, func(i, j int) bool { return f.events[i].at < f.events[j].at })
		if len(f.events) == 0 || f.events[0].at > f.now {
			return
		}
		e := f.events[0]
		f.events = f.events[1:]
		if _, isConn := e.ev.(OBSConnected); !isConn && !f.connected {
			if _, isReq := e.ev.(ItemRequestDone); !isReq {
				if _, isBD := e.ev.(BatchDone); !isBD {
					continue
				}
			}
		}
		f.curHuman = e.human
		f.handleAt(e.ev, e.at)
		f.curHuman = false
	}
}

// handleAt は Event の時刻を at に直して渡す。
func (f *pfake) handleAt(ev Event, atMs int) {
	f.t.Helper()
	f.evAt = atMs
	switch e := ev.(type) {
	case ItemEnabledChanged:
		e.At = at(atMs)
		ev = e
	case ItemRequestDone:
		e.At = at(atMs)
		ev = e
	case SceneChanged:
		e.At = at(atMs)
		ev = e
	case SceneRequestDone:
		e.At = at(atMs)
		ev = e
	case BatchDone:
		e.At = at(atMs)
		ev = e
	case StudioModeChanged:
		e.At = at(atMs)
		ev = e
	case StreamStateChanged:
		e.At = at(atMs)
		ev = e
	}
	if e, ok := ev.(ItemRequestDone); ok {
		if r, ok := f.reqs[e.ReqID]; ok {
			delete(f.reqs, e.ReqID)
			if r.value {
				f.trueUntil[r.key] = atMs + 2000
				if e.Result == ItemError || e.Result == ItemNotFound {
					f.trueUntil[r.key] = atMs
				}
			}
			if e.Result == ItemTimeout {
				delete(f.strictHidden, r.key)
			}
			if e.Result == ItemError && !r.value {
				delete(f.needRecord, r.key)
				delete(f.strictHidden, r.key)
			}
			if e.Result == ItemNotFound {
				delete(f.needRecord, r.key)
			}
			if run := f.runOf(r.batchID); run != nil && (e.Result == ItemError || e.Result == ItemTimeout) {
				run.failed = true
			}
		}
	}
	f.handle(ev)
}

func (f *pfake) runOf(id int) *pbatchRun {
	for _, r := range f.queue {
		if r.b.BatchID == id {
			return r
		}
	}
	return nil
}

// step は実行役の1歩: 実行待ちのバッチの次の Action を承認する。
func (f *pfake) execStep(_ bool) {
	f.t.Helper()
	if len(f.queue) == 0 {
		return
	}
	i := f.rng.Intn(len(f.queue))
	run := f.queue[i]
	b := run.b
	finish := func(res BatchResult) {
		var err error
		if run.failed {
			err = errTest
		}
		f.queue = append(f.queue[:i], f.queue[i+1:]...)
		f.handleAt(BatchDone{BatchID: b.BatchID, Result: res, Err: err}, f.now)
	}
	if run.next >= len(b.Actions) {
		finish(BatchCompleted)
		return
	}
	a := b.Actions[run.next]
	r := f.m.Approve(b.BatchID, a, OverrideNone{}, at(f.now))
	f.logf("approve #%d %T -> %T", b.BatchID, a, r)
	switch r := r.(type) {
	case Reject, Replace:
		finish(BatchRejected)
	case Skip:
		run.next++
	case Proceed:
		run.next++
		switch a := a.(type) {
		case SetScene:
			if f.connected {
				f.scene = a.SceneUUID
				f.onProceedScene(a.SceneUUID, r.ReqID)
				f.adversary()
				f.schedule(50+f.rng.Intn(1500), SceneRequestDone{ReqID: r.ReqID, Result: RequestOK})
				f.schedule(100+f.rng.Intn(2500), SceneChanged{Scene: nameOf(a.SceneUUID), SceneUUID: a.SceneUUID})
			}
		case SetItemEnabled:
			f.onItemProceed(run, a, r)
		}
	}
}

// adversary は SetScene を Proceed した直後に、カメラを不調にして Hide を誘い、その承認を先に進める（切替の期待が残る間の
// 空の検査の窓を広げる）。
func (f *pfake) adversary() {
	f.t.Helper()
	if f.rng.Intn(3) != 0 {
		return
	}
	f.cstates[camSub] = DOWN
	f.handle(CamerasChanged{At: f.at(), States: copyStates(f.cstates)})
	for i := 0; i < 3 && len(f.queue) > 0; i++ {
		last := len(f.queue) - 1
		f.queue[0], f.queue[last] = f.queue[last], f.queue[0]
		f.execStep(false)
	}
}

func (f *pfake) onItemProceed(run *pbatchRun, a SetItemEnabled, p Proceed) {
	f.t.Helper()
	k := pkey{ContainerUUID: a.SceneUUID, ItemID: a.ItemID}
	if !a.Enabled {
		f.onProceedFalse(run.b, k, p)
		if p.MustPersist && f.rng.Intn(50) == 0 {
			f.logf("   保存に失敗")
			f.handleAt(HiddenWriteFailed{BatchID: run.b.BatchID}, f.now)
			// 実行役には Reject を返す（バッチを捨てる）。
			for i, q := range f.queue {
				if q == run {
					f.queue = append(f.queue[:i], f.queue[i+1:]...)
					break
				}
			}
			f.handleAt(BatchDone{BatchID: run.b.BatchID, Result: BatchRejected}, f.now)
			return
		}
		f.needRecord[k] = true
		f.strictHidden[k] = true
	} else {
		f.trueUntil[k] = 1 << 60
	}
	res := ItemOK
	apply := true
	switch n := f.rng.Intn(100); {
	case n < 8:
		res = ItemTimeout
		apply = f.rng.Intn(2) == 0
	case n < 14:
		res = ItemError
		apply = false
	}
	delay := 50 + f.rng.Intn(900)
	rq := &preq{key: k, value: a.Enabled, batchID: run.b.BatchID, apply: apply, result: res}
	f.reqs[p.ItemReqID] = rq
	if apply && f.connected {
		f.items[k] = a.Enabled
		if f.items[k] != f.seen[k] {
			f.schedule(delay/2+f.rng.Intn(300), ItemEnabledChanged{SceneUUID: k.ContainerUUID, ItemID: k.ItemID, Enabled: a.Enabled})
		}
	} else if apply {
		f.items[k] = a.Enabled
	}
	f.schedule(delay, ItemRequestDone{ReqID: p.ItemReqID, Result: res})
}

// ---- 入力の列 ----

func (f *pfake) randomStep(allowCmd bool) {
	f.t.Helper()
	f.now += f.rng.Intn(1500)
	f.deliver()
	n := f.rng.Intn(100)
	switch {
	case n < 28:
		f.execStep(false)
	case n < 50:
		f.now += 500 + f.rng.Intn(1500)
		f.deliver()
		f.handle(Tick{At: f.at()})
	case n < 60:
		for _, c := range []string{camMain, camSub} {
			if f.rng.Intn(3) == 0 {
				f.cstates[c] = []scenemap.CameraState{OK, LOW, DOWN}[f.rng.Intn(3)]
			}
		}
		f.handle(CamerasChanged{At: f.at(), States: copyStates(f.cstates)})
	case n < 68:
		ks := f.allKeys()
		k := ks[f.rng.Intn(len(ks))]
		f.items[k] = !f.items[k]
		if f.connected {
			f.scheduleH(f.rng.Intn(300), ItemEnabledChanged{SceneUUID: k.ContainerUUID, ItemID: k.ItemID, Enabled: f.items[k]})
		}
	case n < 72:
		scs := []string{camera, camera2, deskScene, standby, talk, oUUID}
		to := scs[f.rng.Intn(len(scs))]
		if f.connected && to != f.scene {
			f.scene = to
			f.scheduleH(f.rng.Intn(300), SceneChanged{Scene: nameOf(to), SceneUUID: to})
		}
	case n < 75:
		f.streaming = !f.streaming
		if f.connected {
			f.schedule(f.rng.Intn(300), StreamStateChanged{Streaming: f.streaming})
		}
	case n < 79:
		f.studio = !f.studio
		if f.connected {
			f.schedule(f.rng.Intn(300), StudioModeChanged{Enabled: f.studio})
		}
	case n < 82:
		if f.connected {
			f.connected = false
			f.handle(OBSDisconnected{At: f.at()})
			f.events = nil // 切断で届かなかったイベントは捨てる（結果は実行役が Timeout にする）
			for id := range f.reqs {
				f.schedule(100, ItemRequestDone{ReqID: id, Result: ItemTimeout})
				delete(f.strictHidden, f.reqs[id].key)
			}
		} else {
			f.connected = true
			f.handle(f.connEv())
			f.interval = map[string]bool{}
		}
	case n < 85 && allowCmd:
		kinds := []CommandKind{CmdStreamStart, CmdStreamStop, CmdStandbyOn, CmdStandbyOff, CmdShowAllCameras}
		f.handle(Command{At: f.at(), Kind: kinds[f.rng.Intn(len(kinds))]})
	case n < 87:
		if f.connected {
			// core は今の地図にある項目の値を引き継いで Machine に渡す（観測した値の地図）。
			m := cloneTestMap(f.base)
			for k, v := range f.seen {
				setItem(m, k.ContainerUUID, k.ItemID, v)
			}
			f.handle(SceneMapChanged{At: f.at(), Map: m})
		}
	case n < 90:
		f.autoOn = f.rng.Intn(4) != 0
		f.hide[camSub] = f.rng.Intn(5) != 0
		f.hide[camMain] = f.rng.Intn(4) == 0 // hide が2台の列（H-09）も出す
		f.studioHd = []string{StudioHideStandby, StudioHideIgnore, StudioHideHide}[f.rng.Intn(3)]
		f.handle(f.cfgEv())
	default:
		f.deliver()
	}
}

// ---- 回復の区間（HB-INV-04 の到達性） ----

// recover は入力の列の末尾に、安定した回復の区間を足す: 接続し、全カメラ OK、要求は成功し、イベントは遅れず、Tick を続ける。
// 上限は 5秒×3 + 2秒 + 全対象のバッチ処理時間 + Tick 間隔1秒。ここでは余裕をみて 60秒回す。
func (f *pfake) recoverPhase() {
	f.t.Helper()
	// 飛んでいる Event と結果を、遅れて全部届ける（回復の区間ではイベントは欠落しない）。
	if f.connected {
		for _, e := range f.events {
			if e.at > f.now {
				f.now = e.at
			}
		}
		f.deliver()
	} else {
		// 切断中の要求は、実行役が Timeout にして返す。値は適用されていたことにして、つなぎ直す（地図に載る）。
		f.events = nil
		left := f.reqs
		f.reqs = map[uint64]*preq{}
		for _, r := range left {
			f.items[r.key] = r.value
		}
		f.connected = true
		f.handle(f.connEv())
		ids := make([]uint64, 0, len(left))
		for id := range left {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			delete(f.strictHidden, left[id].key)
			f.handleAt(ItemRequestDone{ReqID: id, Result: ItemTimeout}, f.now)
		}
	}
	// 回復の型: 0 = 全カメラ OK。1 = 配信の停止、2 = auto=false、3 = on_bad=standby（どれもカメラは不良のまま）。
	switch f.mode {
	case 0:
		f.cstates = map[string]scenemap.CameraState{camMain: OK, camSub: OK}
	default:
		f.cstates = map[string]scenemap.CameraState{camMain: DOWN, camSub: DOWN}
	}
	f.handle(CamerasChanged{At: f.at(), States: copyStates(f.cstates)})
	switch f.mode {
	case 1:
		f.streaming = false
		f.handleAt(StreamStateChanged{Streaming: false}, f.now)
	case 2:
		f.autoOn = false
		f.handle(f.cfgEv())
	case 3:
		f.hide[camMain], f.hide[camSub] = false, false
		f.handle(f.cfgEv())
	}
	// 残っている要求は成功として返す。
	for _, r := range f.reqs {
		f.items[r.key] = r.value
	}
	f.reqs = map[uint64]*preq{}
	end := f.now + 60000
	for f.now < end {
		f.now += 1000
		// 実行待ちを全部、成功で流す。
		for guard := 0; len(f.queue) > 0 && guard < 200; guard++ {
			run := f.queue[0]
			b := run.b
			if run.next >= len(b.Actions) {
				f.queue = f.queue[1:]
				f.handleAt(BatchDone{BatchID: b.BatchID, Result: BatchCompleted}, f.now)
				continue
			}
			a := b.Actions[run.next]
			r := f.m.Approve(b.BatchID, a, OverrideNone{}, at(f.now))
			switch r := r.(type) {
			case Reject, Replace:
				f.queue = f.queue[1:]
				f.handleAt(BatchDone{BatchID: b.BatchID, Result: BatchRejected}, f.now)
			case Skip:
				run.next++
			case Proceed:
				run.next++
				switch a := a.(type) {
				case SetScene:
					f.scene = a.SceneUUID
					f.onProceedScene(a.SceneUUID, r.ReqID)
					f.handleAt(SceneRequestDone{ReqID: r.ReqID, Result: RequestOK}, f.now)
					f.handleAt(SceneChanged{Scene: nameOf(a.SceneUUID), SceneUUID: a.SceneUUID}, f.now)
				case SetItemEnabled:
					k := pkey{ContainerUUID: a.SceneUUID, ItemID: a.ItemID}
					if !a.Enabled {
						f.onProceedFalse(b, k, r)
						f.needRecord[k] = true
						f.strictHidden[k] = true
					} else {
						f.trueUntil[k] = 1 << 60
					}
					f.items[k] = a.Enabled
					f.handleAt(ItemRequestDone{ReqID: r.ItemReqID, Result: ItemOK}, f.now)
					f.handleAt(ItemEnabledChanged{SceneUUID: k.ContainerUUID, ItemID: k.ItemID, Enabled: a.Enabled}, f.now)
				}
			}
		}
		f.handle(Tick{At: f.at()})
	}
	// 到達: 本体が隠した項目はすべて表示に戻り、記録は残らない（Timeout で未決着の U は、実際に表示なら残ってよい）。
	for k := range f.needRecord {
		if !f.items[k] {
			mv, _ := f.mapValue(k)
			f.fail("HB-INV-04: %s が回復の区間のあとも非表示（Machine の地図 %v、記録 %v、Hidden=%v Q=%v Pending=%d）",
				pkeyName(k), mv, f.m.entryIdx(k) >= 0, hbEntryString(f.m.st.Hidden), f.m.st.ItemExpectations, len(f.m.st.Pending))
		}
	}
	for _, e := range f.m.st.Hidden {
		if v, ok := f.items[e.Key()]; ok && !v {
			mv, _ := f.mapValue(e.Key())
			_, uns := f.m.st.Unsent[e.Key()]
			f.fail("HB-INV-04: 記録 %s:%s が残り、項目は非表示のまま（Machine の地図 %v、未送信 %v、Q=%v Pending=%d 接続 %v）",
				pkeyName(e.Key()), e.State, mv, uns, f.m.st.ItemExpectations, len(f.m.st.Pending), f.m.st.Connected)
		}
		if e.State != HiddenU && f.items[e.Key()] {
			// 表示に戻ったのに H / P の記録が残る: 読み直し・期待で外れるはず
			f.fail("HB-INV-04: 項目は表示なのに記録 %s:%s が残る", pkeyName(e.Key()), e.State)
		}
	}
}

func TestHB性質_第5節の4つ(t *testing.T) {
	runs := 3000
	if testing.Short() {
		runs = 150
	}
	if v := os.Getenv("HB_RUNS"); v != "" { // 種の数を増やして確かめる
		n, _ := strconv.Atoi(v)
		runs = n
	}
	first := int64(1)
	if v := os.Getenv("HB_SEED"); v != "" { // 落ちた種だけを再現する
		n, _ := strconv.ParseInt(v, 10, 64)
		first, runs = n, int(n)
	}
	for seed := first; seed <= int64(runs); seed++ {
		f := newPfake(t, seed)
		f.mode = int(seed % 4)
		steps := 60 + f.rng.Intn(120)
		for i := 0; i < steps; i++ {
			f.randomStep(true)
		}
		f.recoverPhase()
	}
}

// wantRecorded は Snapshot の一覧の写しに、k の記録が正しい接続先・コレクション・入力 UUID であることを確かめる。
func (f *pfake) wantRecorded(k pkey, what string) {
	f.t.Helper()
	for _, e := range f.m.Snapshot().HiddenEntries {
		if e.Key() != k {
			continue
		}
		if e.Host != hbHost || e.Port != hbPort || e.Collection != collectionX {
			f.fail("HB-INV-01: %s の記録の接続先・コレクションが違う: %s:%d %q", pkeyName(k), e.Host, e.Port, e.Collection)
		}
		if want := f.baseSource(k); e.SourceUUID != want {
			f.fail("HB-INV-01: %s の記録の入力 UUID が違う: %q, want %q", pkeyName(k), e.SourceUUID, want)
		}
		return
	}
	f.fail("HB-INV-01: %s に%s", pkeyName(k), what)
}

func (f *pfake) baseSource(k pkey) string {
	c, ok := f.base.Scenes[k.ContainerUUID]
	if !ok {
		c = f.base.Groups[k.ContainerUUID]
	}
	for _, it := range c.Items {
		if it.ID == k.ItemID {
			return it.SourceUUID
		}
	}
	return ""
}

func (f *pfake) mapValue(k pkey) (bool, bool) {
	it, ok := f.m.findItem(k)
	return it.Enabled, ok
}
