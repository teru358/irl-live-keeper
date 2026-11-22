package switcher

import (
	"strings"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 0c（シーンごとの逃がし先）の行と列のための部品。表 docs/superpowers/plans/2026-11-22-switcher-table-0c.md の記号:
// A=camera（m と s）、D=deskScene（s だけ）、S=standby、T=talk（カメラなし）、B=camera2（m だけ）。
// 既定は 0b の fixture（newHB）に、設定の逃がし先を足したもの。

// hbFB は設定の scene_fallbacks（名前のまま）を足す。
func hbFB(rows ...FallbackRow) hbOpt {
	return func(h *hb) { h.cfg.SceneFallbacks = rows }
}

func fbRow(from, to string) FallbackRow {
	return FallbackRow{Scene: nameOf(from), Fallback: nameOf(to)}
}

// 表の既定の設定 A→D→S（D の行はなく、D の先は共通待機）。
var fbAD = hbFB(fbRow(camera, deskScene))

// sceneC は表の C（別の監視シーン。m が映る）。地図に足して使う。
const sceneC = "uuid-c"

func init() { hbSceneNames[sceneC] = "C" }

func addSceneC(m *scenemap.Map) {
	m.Scenes[sceneC] = scenemapContainer("C", scenemapItem(1, inMain, true))
}

// addSceneCs は C を s だけが映るシーンにする（C が悪い間も m の A/B が戻れる fixture）。
func addSceneCs(m *scenemap.Map) {
	m.Scenes[sceneC] = scenemapContainer("C", scenemapItem(1, inSub, true))
}

// hbNoStandby は地図から共通待機 S を消す（FB-NM-06）。
func hbNoStandby() hbOpt {
	return hbMapEdit(func(m *scenemap.Map) { delete(m.Scenes, standby) })
}

// pdesc は自動切替のバッチを、行き先と履歴計画つきの表の書き方にする。F(D;+A,B)、R(A)。ほかは desc と同じ。
func pdesc(b Batch) string {
	switch b.Purpose {
	case PurposeAutoFail:
		to := b.Actions[len(b.Actions)-1].(SetScene).SceneUUID
		return "F(" + letter(to) + ";+" + letters(b.Plan.Plan.Append) + ")"
	case PurposeAutoRecover:
		return "R(" + letter(b.Plan.Plan.Target) + ")"
	}
	return desc(b)
}

func pdescs(bs []Batch) string {
	var out []string
	for _, b := range bs {
		out = append(out, pdesc(b))
	}
	return strings.Join(out, " ")
}

func letters(us []string) string {
	var out []string
	for _, u := range us {
		out = append(out, letter(u))
	}
	return strings.Join(out, ",")
}

// outP は出力の列が want（pdesc の書き方）であることを確かめる。
func (h *hb) outP(bs []Batch, want string) []Batch {
	h.t.Helper()
	if got := pdescs(bs); got != want {
		h.t.Fatalf("出力: got %q, want %q", got, want)
	}
	return bs
}

// wantStack は auto のスタック（下→上）が want（"A,D"。空は none で空）であることを確かめる。Snapshot の写しと内部の両方。
func (h *hb) wantStack(want string) {
	h.t.Helper()
	if got := letters(h.m.st.Stack); got != want {
		h.t.Fatalf("スタック: got [%s], want [%s]", got, want)
	}
	snap := h.m.Snapshot()
	if got := letters(snap.Standby.Stack); got != want {
		h.t.Fatalf("Snapshot のスタック: got [%s], want [%s]", got, want)
	}
	switch {
	case want == "":
		if snap.Standby.Owner == OwnerAuto {
			h.t.Fatalf("スタックが空なのに auto")
		}
	default:
		if snap.Standby.Owner != OwnerAuto || snap.Standby.ReturnScene != h.m.st.Stack[0] {
			h.t.Fatalf("auto の戻り先は最下段: %+v", snap.Standby)
		}
	}
}

func (h *hb) wantOwner(o Owner, ret string) {
	h.t.Helper()
	st := h.m.Snapshot().Standby
	if st.Owner != o || st.ReturnScene != ret {
		h.t.Fatalf("持ち主: got %v %q, want %v %q", st.Owner, st.ReturnScene, o, ret)
	}
}

func (h *hb) wantScene(u string) {
	h.t.Helper()
	if got := h.m.st.Scene; got != u {
		h.t.Fatalf("現在のシーン: got %s, want %s", letter(got), letter(u))
	}
}

// settle は「scene 決着」: Restart があれば承認し、SetScene の承認 Proceed → SRD OK → BD Completed → SC 一致。
// 返すのは、BD と SC で出た列（合わせて）。
func (h *hb) settle(b Batch, ms int) []Batch {
	h.t.Helper()
	var to string
	for _, a := range b.Actions {
		switch a := a.(type) {
		case RestartMedia:
			if r := h.m.Approve(b.BatchID, a, OverrideNone{}, at(ms)); !isProceed(r) {
				h.t.Fatalf("Approve(Restart) = %#v", r)
			}
		case SetScene:
			to = a.SceneUUID
			r := h.m.Approve(b.BatchID, a, OverrideNone{}, at(ms))
			p, ok := r.(Proceed)
			if !ok {
				h.t.Fatalf("Approve(SetScene %s) = %#v", letter(to), r)
			}
			h.ev(SceneRequestDone{At: at(ms), ReqID: p.ReqID, Result: RequestOK})
		}
	}
	var out []Batch
	out = append(out, h.bdone(ms, b.BatchID, BatchCompleted, nil)...)
	out = append(out, h.scene(ms+1, to)...)
	return out
}

// hideAll は「hide 決着」: 各項目の false の承認・OK・イベントのあと BD。返すのは全部で出た列。
func (h *hb) hideAll(b Batch, ms int) []Batch {
	h.t.Helper()
	var out []Batch
	for _, it := range b.Items {
		k := scenemap.ItemKey{ContainerUUID: it.SceneUUID, ItemID: it.ItemID}
		p := h.proceed(b, ms, k, false)
		out = append(out, h.itemDone(ms, p.ItemReqID, ItemOK)...)
		out = append(out, h.itemEv(ms, k, false)...)
	}
	return append(out, h.bdone(ms+1, b.BatchID, BatchCompleted, nil)...)
}

// showAll は「show 決着」: 固定の Restart を承認し、各項目の true の承認・OK・イベントのあと BD。返すのは全部で出た列。
func (h *hb) showAll(b Batch, ms int) []Batch {
	h.t.Helper()
	var out []Batch
	for _, a := range b.Actions {
		switch a := a.(type) {
		case RestartMedia:
			if r := h.m.Approve(b.BatchID, a, OverrideNone{}, at(ms)); !isProceed(r) {
				if r != (Skip{}) {
					h.t.Fatalf("Approve(Restart) = %#v", r)
				}
			}
		case SetItemEnabled:
			k := scenemap.ItemKey{ContainerUUID: a.SceneUUID, ItemID: a.ItemID}
			p := h.proceed(b, ms+1, k, true)
			out = append(out, h.itemDone(ms+1, p.ItemReqID, ItemOK)...)
			out = append(out, h.itemEv(ms+1, k, true)...)
		}
	}
	return append(out, h.bdone(ms+2, b.BatchID, BatchCompleted, nil)...)
}

// toSAD は A から m/s が同時に DOWN になり、S に着いて x/y を隠した状態にする（FB-SQ-03）。S、auto/[A,D]、x/y H。時刻は ms から。
func (h *hb) toSAD(ms int) {
	h.t.Helper()
	bs := h.cams(ms, DOWN, DOWN)
	h.outP(bs, "F(S;+A,D) Hc(s)[x,y]")
	h.settle(bs[0], ms+100)
	h.hideAll(bs[1], ms+200)
	h.wantScene(standby)
	h.wantStack("A,D")
}

// setAuto は履歴を直接作る（経路が目的でない行）。現在のシーンも変える。
func (h *hb) setAuto(cur string, stack ...string) {
	h.t.Helper()
	h.m.st.Scene = cur
	h.m.setAuto(stack)
}

// setStates はカメラの状態を直接書く（判断を走らせず、前提の状態だけ作る。次の Tick などで判断が走る）。
func (h *hb) setStates(m, s scenemap.CameraState) {
	h.m.st.CameraStates = map[string]scenemap.CameraState{camMain: m, camSub: s}
}

// expectAuto は、SetScene の承認 Proceed を通して期待を登録する（SRD・BD・SC は流さない）。
func (h *hb) approveSet(b Batch, ms int) Proceed {
	h.t.Helper()
	to := b.Actions[len(b.Actions)-1].(SetScene).SceneUUID
	r := h.m.Approve(b.BatchID, SetScene{SceneUUID: to}, OverrideNone{}, at(ms))
	p, ok := r.(Proceed)
	if !ok {
		h.t.Fatalf("Approve(SetScene %s) = %#v", letter(to), r)
	}
	return p
}

// completeBatch は SRD OK と BD Completed を流す（SC は流さない）。
func (h *hb) completeBatch(b Batch, req int, ms int) []Batch {
	h.t.Helper()
	h.ev(SceneRequestDone{At: at(ms), ReqID: req, Result: RequestOK})
	return h.bdone(ms, b.BatchID, BatchCompleted, nil)
}

// failBatch は SRD Error と BD Failed を流す。
func (h *hb) failBatch(b Batch, req int, ms int) []Batch {
	h.t.Helper()
	h.ev(SceneRequestDone{At: at(ms), ReqID: req, Result: RequestError})
	return h.bdone(ms, b.BatchID, BatchFailed, errTest)
}

func (h *hb) wantNoExpectations() {
	h.t.Helper()
	if len(h.m.st.Expectations) != 0 {
		h.t.Fatalf("期待が残っている: %+v", h.m.st.Expectations)
	}
}
