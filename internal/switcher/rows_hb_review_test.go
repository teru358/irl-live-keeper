package switcher

import (
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// レビューで足した確認。表の I(R)（3.2 の右端の列）、空の地図での記録の保持、U の再送、予約のあとの記録の消失、停止中の Auto。

func (h *hb) wantRestartActions(b Batch, want ...Action) {
	h.t.Helper()
	if len(b.Actions) != len(want) {
		h.t.Fatalf("Actions: got %+v, want %+v", b.Actions, want)
	}
	for i := range want {
		if rm, ok := want[i].(RestartMedia); ok {
			g, ok := b.Actions[i].(RestartMedia)
			if !ok || len(g.Inputs) != len(rm.Inputs) || g.Wait != rm.Wait {
				h.t.Fatalf("Actions[%d]: got %+v, want %+v", i, b.Actions[i], want[i])
			}
			for j := range rm.Inputs {
				if g.Inputs[j] != rm.Inputs[j] {
					h.t.Fatalf("Actions[%d] の入力: got %v, want %v", i, g.Inputs, rm.Inputs)
				}
			}
			continue
		}
		if b.Actions[i] != want[i] {
			h.t.Fatalf("Actions[%d]: got %+v, want %+v", i, b.Actions[i], want[i])
		}
	}
}

func TestHBIR再起動の対象は有効な動きで決める(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-OF-01/I(A)", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.standbyOnManual(t)
			bs, _ := h.evErr(Command{At: at(2000), Kind: CmdStandbyOff})
			h.out(bs, "M(A)")
			// I(A) は standby のカメラだけ（hide の s は含めない）。送出の記録がなくても同じ。
			h.wantRestartActions(bs[0], RestartMedia{Inputs: []string{inMain}, Wait: W}, SetScene{SceneUUID: camera})
		}},
		// /0c: Studio off で A は健康 OK でも、表示中の悪い s が隠れるまで戻らない（逆向きの待ち）。Hc だけを出し、
		// x の false を確かめたあとに R(A)（HB-SM-05 の置き換え。FB-CC-04）。I(A) は standby の m だけ。
		{"HB-SM-05/I(A)/0c", func(t *testing.T) {
			h := newHB(t, hbStudio())
			bs := h.sIs(1000, DOWN)
			h.proceed2scene(bs[0], 1100)
			h.scene(1500, standby)
			bs = h.ev(StudioModeChanged{At: at(7000), Enabled: false})
			h.out(bs, "Hc(s)[x,y]")
			rs := h.sendFalseOut(bs[0], 7100, kx, "R(A)")
			h.wantRestartActions(rs[0], RestartMedia{Inputs: []string{inMain}, Wait: W}, SetScene{SceneUUID: camera})
		}},
		{"HB-SQ-02/t9 R(D)", func(t *testing.T) {
			h := newHB(t, hbAt(deskScene))
			bs := h.sIs(1000, DOWN)
			h.proceed2scene(bs[0], 1100)
			h.out(h.scene(1300, standby), "Hc(s)[x,y]")
			hc := h.lastBatch
			h.sendFalse(hc, 1400, kx)
			h.sendFalse(hc, 1400, ky)
			h.bdone(1600, hc.BatchID, BatchCompleted, nil)
			sc := h.sIs(8000, OK)
			h.m.Approve(sc[0].BatchID, restartOf(inSub), OverrideNone{}, at(8100))
			h.sendTrue(sc[0], 8200, kx)
			rs := h.sendTrueOut(sc[0], 8300, ky, "R(D)")
			h.wantRestartActions(rs[0], SetScene{SceneUUID: deskScene})
		}},
		{"I(R)/Studio on standby は含める", func(t *testing.T) {
			h := newHB(t, hbStudio())
			h.m.st.Scene = standby // /0c: 今の program（配信に実表示）の入力は除外するので、戻り先とは別のシーンにいる状態で確かめる
			got := h.m.restartInputs(camera)
			if len(got) != 2 || got[0] != inMain || got[1] != inSub {
				t.Fatalf("on/standby の I(A): %v", got)
			}
			for _, mode := range []string{StudioHideIgnore, StudioHideHide} {
				h := newHB(t, hbStudio(), hbStudioHide(mode))
				h.m.st.Scene = standby
				if got := h.m.restartInputs(camera); len(got) != 1 || got[0] != inMain {
					t.Fatalf("on/%s の I(A): %v", mode, got)
				}
			}
			h2 := newHB(t)
			h2.m.st.Scene = standby
			if got := h2.m.restartInputs(camera); len(got) != 1 || got[0] != inMain {
				t.Fatalf("off の I(A): %v", got)
			}
		}},
	})
}

func TestHB空の地図で記録を消さない(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-OC-03/Stopping", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.ev(OBSDisconnected{At: at(500)})
			h.ev(OBSConnected{At: at(600), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300),
				Map: &scenemap.Map{Collection: collectionX}})
			bs := h.ev(Stopping{At: at(1000)})
			for _, b := range bs {
				for _, a := range b.Actions {
					if si, ok := a.(SetItemEnabled); ok {
						h.ap(b, 1100, scenemapKey(si.SceneUUID, si.ItemID), si.Enabled)
					}
				}
				h.bdone(1200, b.BatchID, BatchCompleted, nil)
			}
			h.wantRetainedOrHidden("x:H")
		}},
	})
}

func TestHBUの再送とA(t *testing.T) {
	// 未決着 U に新しい false を送った間は、その項目を非表示と数える（応答が OK か未応答の間）。
	h := newHB(t, hbNoY())
	b, req := h.pX(1000)
	h.itemDone(1200, req, ItemTimeout)
	h.bdone(1300, b.BatchID, BatchCompleted, errTest)
	h.wantEntries("x:U")
	h.wantA(camera, "cam1", "cam2") // U は表示と数える
	h.tick(6000)                    // 5秒たった: 再送
	hc := h.lastBatch
	if hc.Purpose != PurposeHideCamera {
		t.Fatalf("再送がない: %s", desc(hc))
	}
	h.proceed(hc, 6100, kx, false)
	h.wantEntries("x:U")
	h.wantA(camera, "cam1") // 送った false の応答前は非表示と数える
}

func TestHB予約のあとに記録が消えても記録なしでは送らない(t *testing.T) {
	h := newHB(t)
	b := h.hide(1000)
	h.proceed(b, 1100, kx, false)
	// y の予約（未送信 P）が、照合で消える（項目が消えて戻る）。
	m := editMap(h.mp, func(m *scenemap.Map) {
		c := m.Scenes[deskScene]
		c.Items = nil
		m.Scenes[deskScene] = c
	})
	h.smc(1200, m)
	h.wantEntries("x:P")
	m2 := cloneTestMap(h.mp) // 項目が戻った
	h.smc(1300, m2)
	r := h.ap(b, 1400, ky, false)
	if p, ok := r.(Proceed); ok {
		if h.entry(ky) == nil || !p.MustPersist {
			t.Fatalf("記録なしで送る / 保存の印がない: entry=%v %+v", h.entry(ky), p)
		}
	}
}

func TestHB停止中のAuto後続はRejectする(t *testing.T) {
	h := newHB(t)
	bs := h.sIs(1000, LOW) // m は OK。F は出ない
	_ = bs
	bs = h.cams(1100, LOW, LOW)
	var f Batch
	for _, b := range bs {
		if b.Purpose == PurposeAutoFail {
			f = b
		}
	}
	if f.BatchID == 0 {
		t.Fatalf("F がない: %s", descs(bs))
	}
	h.ev(Stopping{At: at(1200)})
	if r := h.m.Approve(f.BatchID, SetScene{SceneUUID: standby}, OverrideNone{}, at(1300)); r != (Reject{}) {
		t.Fatalf("停止中の Auto: %#v", r)
	}
}
