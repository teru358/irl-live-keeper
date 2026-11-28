package switcher

import (
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// codex sol のレビュー（M-11 の検収）で足した行。

// sol 1: 遅れた結果は、送った時の Z の記録だけに当てる（HB-IR-12〜15 と HB-SQ-09 の組み合わせ）。
func TestHB遅れた結果は送った時のZの記録だけに当てる(t *testing.T) {
	for _, res := range []ItemResult{ItemNotFound, ItemTimeout, ItemError, ItemOK} {
		t.Run(res.String(), func(t *testing.T) {
			h := newHB(t, hbNoY())
			b, req := h.pX(1000) // X で x の false を送信（P）
			h.ev(OBSDisconnected{At: at(1300)})
			h.bdone(1400, b.BatchID, BatchRejected, nil)
			y := cloneTestMap(h.mp)
			y.Collection = "collection-y"
			bs := h.ev(OBSConnected{At: at(7000), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300), Map: y})
			h.out(bs, "Hc(s)[x]") // Y で同じ鍵 x を Hide
			p := h.proceed(bs[0], 7100, kx, false)
			h.itemDone(7150, p.ItemReqID, ItemOK)
			h.itemEv(7160, kx, false)
			h.wantEntries("x:H")
			h.wantRetained("x:P")
			h.itemDone(7200, req, res) // X の遅れた結果
			h.wantEntries("x:H")       // Y の記録は触らない
			if e := h.entry(kx); e == nil || e.Collection != "collection-y" || e.State != HiddenH {
				t.Fatalf("Y の記録が変わった: %+v", e)
			}
			want := map[ItemResult]string{ItemNotFound: "", ItemError: "", ItemTimeout: "x:U", ItemOK: "x:P"}[res]
			h.wantRetained(want) // X の記録だけが結果を受ける
		})
	}
}

// sol 2: MA の表示確認待ちの P は、10秒の掃除でも消さない（表 4.8）。
func TestHBMAの確認待ちのPは掃除しない(t *testing.T) {
	h := newHB(t, hbNoY())
	_, req := h.pX(1000)
	h.ev(OBSDisconnected{At: at(1300)})
	h.itemDone(1400, req, ItemOK) // 遅れた OK で P の決着時刻が付く
	h.connect(2000, true, camera, false)
	bs, err := h.evErr(Command{At: at(3000), Kind: CmdShowAllCameras})
	if err != nil {
		t.Fatal(err)
	}
	h.out(bs, "MA[x]")
	p := h.proceed(bs[0], 3100, kx, true)
	h.itemDone(3200, p.ItemReqID, ItemOK) // イベント未着
	h.bdone(3300, bs[0].BatchID, BatchCompleted, nil)
	h.tick(11401) // 決着から10秒超
	h.wantEntries("x:P")
	h.tick(20000)
	h.wantEntries("x:P")
}

// sol 3: Show の Restart は、固定した入力のすべてが今もそのカメラの入力のときだけ送る（HB-AP-13 の追加）。
func TestHBShowのRestartは固定入力の全部が一致するときだけ(t *testing.T) {
	const in2 = "in-sub2"
	opt := hbMapEdit(func(m *scenemap.Map) {
		m.Inputs[in2] = scenemap.Input{Name: "cam2-src2", CameraID: camSub}
		c := m.Scenes[camera2]
		c.Items = append(c.Items, scenemapItem(2, in2, true))
		m.Scenes[camera2] = c
	})
	h := newHB(t, opt, hbNoY())
	h.yOff()
	setItem(h.mp, kx.ContainerUUID, kx.ItemID, false)
	setItem(h.mp, kz.ContainerUUID, kz.ItemID, false)
	h.itemEv(5, kx, false)
	h.itemEv(5, kz, false)
	h.m.st.Hidden = append(h.m.st.Hidden, h.entryOf(kx, HiddenH))
	e := h.entryOf(kz, HiddenH)
	e.SourceUUID = in2
	h.m.st.Hidden = append(h.m.st.Hidden, e)
	h.m.sortHidden()
	bs := h.tick(1000)
	h.out(bs, "Sc(s)[R(is+in-sub2),x,z]")
	// in-sub2 だけが m の入力に変わる
	m := editMap(h.mp, func(m *scenemap.Map) { m.Inputs[in2] = scenemap.Input{Name: "cam2-src2", CameraID: camMain} })
	h.smc(1050, m)
	if r := h.m.Approve(bs[0].BatchID, bs[0].Actions[0], OverrideNone{}, at(1100)); r != (Skip{}) {
		t.Fatalf("一部だけ別カメラの Restart: %#v, want Skip", r)
	}
}

// sol 4: MA の固定対象は、承認の時点で今もカメラの項目なら（地図で true でも）true を送る（HB-AP-22、HB-SQ-10A の 2.4）。
func TestHBMAの固定対象は地図trueでも送る(t *testing.T) {
	h := newHB(t)
	b := h.hide(1000)
	h.proceed(b, 1000, kx, false)
	bs, _ := h.evErr(Command{At: at(2000), Kind: CmdShowAllCameras})
	h.out(bs, "MA[x,y]")
	h.want(h.ap(b, 2100, ky, false), ky, b, 2100, false, Reject{})
	h.bdone(2200, b.BatchID, BatchRejected, nil)
	h.proceed(bs[0], 2400, ky, true) // y は記録がなく地図 true。カメラの項目なので送る
}

// sol 7（指揮者の変異）: 続けて3回失敗して使えないカメラの隠した項目は、戻り先が待たない。
func TestHB使えないカメラの隠した項目はAutoRecoverを止めない(t *testing.T) {
	setup := func(t *testing.T, f int) *hb {
		h := newHB(t)
		bs := h.cams(1000, LOW, OK)
		h.out(bs, "F(A)")
		h.proceed2scene(bs[0], 1100)
		h.scene(1500, standby)
		h.yOff()
		h.seedH(kx)
		if f > 0 {
			h.m.st.ItemFailures[camSub] = f
		}
		return h
	}
	h := setup(t, 0)
	bs := h.cams(2000, OK, OK)
	for _, b := range bs {
		if b.Purpose == PurposeAutoRecover {
			t.Fatalf("待っている項目があるのに R が出た: %s", descs(bs))
		}
	}
	h = setup(t, 3)
	bs = h.cams(2000, OK, OK)
	found := false
	for _, b := range bs {
		if b.Purpose == PurposeAutoRecover {
			found = true
		}
	}
	if !found {
		t.Fatalf("使えないカメラの項目は待たないのに R が出ない: %s", descs(bs))
	}
}

// sol 6: 0a では「hide も standby と同じ」だった入力が、0b でどう変わるか（withNoop に置き換えた行の元の入力）。
func TestMC元のhide入力の0bでの変化(t *testing.T) {
	t.Run("SQ-14e/SCF-13 の元: cam1 を hide にする設定変更。戻り先 A への R は Restart を付けない", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		// 0a ではこの設定変更は何も出さない。0b では cam1 が不良の hide なので Hc(cam1) が出る。
		bs, err := q.m.Handle(cfg(1000, withCameras(CameraConfig{ID: camMain, OnBad: scenemap.MotionHide})))
		if err != nil || len(bs) != 1 || bs[0].Purpose != PurposeHideCamera {
			t.Fatalf("Hc(cam1) がない: %v %v", descs2(bs), err)
		}
		q.owner(OwnerAuto, camera)
		bs, err = q.m.Handle(lc(2000, OK))
		var r *Batch
		for i := range bs {
			if bs[i].Purpose == PurposeAutoRecover {
				r = &bs[i]
			}
		}
		if err != nil || r == nil {
			t.Fatalf("R がない: %v %v", descs2(bs), err)
		}
		if len(r.Actions) != 1 {
			t.Fatalf("0a では [Restart, SetScene]。0b は hide の入力を含めず SetScene だけ: %+v", r.Actions)
		}
	})
	t.Run("MC-SQ-01/hide の元: s を hide にして s が LOW。F ではなく Hc(s)", func(t *testing.T) {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN},
			withCameras(cam(camMain), CameraConfig{ID: camSub, OnBad: scenemap.MotionHide}, cam(camDesk)))
		bs, err := q.m.Handle(cc(1000, OK, LOW, DOWN))
		if err != nil || len(bs) != 1 || bs[0].Purpose != PurposeHideCamera || bs[0].CameraID != camSub {
			t.Fatalf("0a は AutoFail(cam2)。0b は Hc(cam2) だけ: %v %v", descs2(bs), err)
		}
	})
}

func descs2(bs []Batch) string {
	var out string
	for _, b := range bs {
		out += b.Purpose.String() + " "
	}
	return out
}

// terra: 旧い Z の予約（Unsent）と MA の確認待ちは、別の Z の同じ鍵の記録に及ばない。
func TestHB旧いZの予約と確認待ちは別のZの記録に及ばない(t *testing.T) {
	yMap := func(h *hb) *scenemap.Map {
		y := cloneTestMap(h.mp)
		y.Collection = "collection-y"
		return y
	}
	yEntry := func(h *hb, k scenemap.ItemKey, st HiddenState) HiddenEntry {
		e := h.entryOf(k, st)
		e.Collection = "collection-y"
		return e
	}
	connY := func(h *hb, y *scenemap.Map) {
		h.ev(OBSConnected{At: at(7000), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300), Map: y})
	}
	for _, how := range []string{"Reject", "HiddenWriteFailed"} {
		t.Run("予約の巻き戻し/"+how, func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false) // y は X の未送信 P
			h.ev(OBSDisconnected{At: at(1300)})
			h.m.st.Retained = append(h.m.st.Retained, yEntry(h, ky, HiddenP)) // Y の同じ鍵 y の P
			connY(h, yMap(h))
			if how == "HiddenWriteFailed" {
				h.ev(HiddenWriteFailed{At: at(7100), BatchID: b.BatchID})
			}
			h.bdone(7200, b.BatchID, BatchRejected, nil)
			if e := h.entry(ky); e == nil || e.Collection != "collection-y" {
				t.Fatalf("Y の y の P が消えた: %q / %q", h.entries(), h.retained())
			}
		})
	}
	t.Run("MA の確認待ち", func(t *testing.T) {
		h := newHB(t, hbNoY())
		h.seedH(kx)
		bs, _ := h.evErr(Command{At: at(1000), Kind: CmdShowAllCameras})
		h.out(bs, "MA[x]")
		h.proceed(bs[0], 1100, kx, true) // MAConfirm[x]（X）
		h.ev(OBSDisconnected{At: at(1300)})
		h.m.st.Retained = append(h.m.st.Retained, yEntry(h, kx, HiddenU)) // Y の x の U（地図 true）
		y := yMap(h)
		setItem(y, kx.ContainerUUID, kx.ItemID, true)
		connY(h, y)
		if e := h.entry(kx); e == nil || e.State != HiddenU || e.Collection != "collection-y" {
			t.Fatalf("X の確認待ちが Y の U を消した: %q / %q", h.entries(), h.retained())
		}
	})
}
