package switcher

import (
	"testing"
)

// 表 0c 5（非同期の列）FB-SQ-01〜07・09・11・13・14。時刻は表の t（秒）を ms にしたもの（t=1 → 1000）。
// scene 決着・hide 決着・show 決着は fb_helpers_test.go の settle・hideAll・showAll に展開し、応答・イベントを省略しない。

func TestFBSQ01(t *testing.T) {
	// spec 例1〜4: メイン悪化、サブ悪化、順に回復。
	h := newHB(t, fbAD)
	h.wantStack("")
	f := h.cams(1000, DOWN, OK)
	h.outP(f, "F(D;+A)")
	if len(f[0].Actions) != 1 { // 再起動なし
		t.Fatalf("F に再起動: %v", f[0].Actions)
	}
	h.wantStack("")
	h.outP(h.settle(f[0], 1100), "")
	h.wantScene(deskScene)
	h.wantStack("A")
	// t=2: s DOWN → F(S;+D)。D を空にする Hide は出さない。
	f = h.cams(2000, DOWN, DOWN)
	h.outP(f, "F(S;+D)")
	// t=2.1: S に着いたら Hc(s)（ここで隠せる）。
	out := h.settle(f[0], 2100)
	h.outP(out, "Hc(s)[x,y]")
	h.wantStack("A,D")
	// t=2.2: hide 決着。x/y は H、false。復帰なし。
	h.outP(h.hideAll(out[0], 2200), "")
	if h.entry(kx) == nil || h.entry(ky) == nil {
		t.Fatalf("x/y の記録がない")
	}
	h.wantStack("A,D")
	// t=8: s OK → Sc(s)。D は健康 OK だが表示0。R なし。
	sc := h.cams(8000, DOWN, OK)
	if len(sc) != 1 || sc[0].Purpose != PurposeShowCamera {
		t.Fatalf("Sc(s) でない: %s", pdescs(sc))
	}
	r := h.showAll(sc[0], 8100)
	h.outP(r, "R(D)")
	h.wantStack("A,D") // K は [A,D] のまま
	// t=9.3: R(D) の SetScene 承認、scene 決着。D、auto/[A]。is は hide なので復帰 R で再起動しない。
	if len(r[0].Actions) != 1 {
		t.Fatalf("R(D) に再起動: %v", r[0].Actions)
	}
	h.outP(h.settle(r[0], 9300), "")
	h.wantScene(deskScene)
	h.wantStack("A")
	// t=10: m OK → R(A)。I=[im]（is は含めない）。
	a := h.cams(10000, OK, OK)
	h.outP(a, "R(A)")
	h.wantRestartActions(a[0], restartOf(inMain), SetScene{SceneUUID: camera})
	h.outP(h.settle(a[0], 10100), "")
	h.wantScene(camera)
	h.wantStack("")
	h.wantOwner(OwnerNone, "")
}

func TestFBSQ02(t *testing.T) {
	// spec 例5: 待機中に同時回復。
	h := newHB(t, fbAD)
	h.toSAD(1000)
	sc := h.cams(8000, OK, OK)
	if len(sc) != 1 || sc[0].Purpose != PurposeShowCamera {
		t.Fatalf("Sc(s) でない: %s", pdescs(sc))
	}
	h.wantStack("A,D")
	r := h.showAll(sc[0], 8100)
	h.outP(r, "R(A)") // 下の A へ直接。D への R なし
	h.outP(h.settle(r[0], 8700), "")
	h.wantScene(camera)
	h.wantStack("")
}

func TestFBSQ03(t *testing.T) {
	// spec 例6: 同時悪化で途中を飛ばす。
	h := newHB(t, fbAD)
	bs := h.cams(1000, DOWN, DOWN)
	h.outP(bs, "F(S;+A,D) Hc(s)[x,y]") // D 不良で飛ばす。A には m が残るので x は隠せる
	p := h.approveSet(bs[0], 1100)     // 期待登録、K=[]
	h.wantStack("")
	h.outP(h.hideAll(bs[1], 1200), "") // 保護対象 S は待機、空検査を維持
	h.wantStack("")
	h.completeBatch(bs[0], p.ReqID, 1300)
	h.outP(h.scene(1400, standby), "")
	h.wantStack("A,D") // 実在しない D の到着を作らない
}

func TestFBSQ04(t *testing.T) {
	// spec 例7: メインだけ回復、逆向きの待ち。
	h := newHB(t, fbAD)
	h.setStates(DOWN, DOWN)
	h.setAuto(standby, camera, deskScene)
	hc := h.cams(1000, OK, DOWN)
	h.outP(hc, "Hc(s)[x,y]") // A 健康 OK、逆待あり、R なし
	// 応答だけでは解けず、x の false のイベントで待ちが解消して R。
	r := h.sendFalseOut(hc[0], 1100, kx, "R(A)")
	h.wantStack("A,D") // K は到着まで保持
	h.outP(h.settle(r[0], 1500), "")
	h.wantScene(camera)
	h.wantStack("")
	h.wantOwner(OwnerNone, "")
}

func TestFBSQ04ReleasePoint(t *testing.T) {
	// 逆向きの待ちは、false が確かめられる（項目のイベントが届く）まで解けない。Proceed・応答だけでは R なし。
	h := newHB(t, fbAD)
	h.setStates(OK, DOWN)
	h.setAuto(standby, camera, deskScene)
	hc := h.tick(1000)
	h.outP(hc, "Hc(s)[x,y]")
	p := h.proceed(hc[0], 1100, kx, false)
	h.outP(h.tick(1150), "")
	h.outP(h.itemDone(1200, p.ItemReqID, ItemOK), "")
	h.outP(h.tick(1250), "")
	// SetScene 直前の検査でも待つ（生成の前に承認が来た場合）。
	h.outP(h.itemEv(1300, kx, false), "R(A)")
}

func TestFBSQ04Columns(t *testing.T) {
	t.Run("a: u(s)=true なら Hc なし、逆待なしで R(A)", func(t *testing.T) {
		h := newHB(t, fbAD)
		h.setStates(OK, DOWN)
		h.setAuto(standby, camera, deskScene)
		h.m.st.UserShown[camSub] = true
		h.outP(h.tick(1000), "R(A)")
	})
	t.Run("b: f(s)>=3 なら Hc なし、警告。他の条件が成立すれば R", func(t *testing.T) {
		h := newHB(t, fbAD)
		h.setStates(OK, DOWN)
		h.setAuto(standby, camera, deskScene)
		h.m.st.ItemFailures[camSub] = 3
		h.outP(h.tick(1000), "R(A)")
		if !h.m.Snapshot().Cameras[1].Unusable {
			t.Fatalf("使えない（警告）が status にない")
		}
	})
	t.Run("c: 現 D が不良で x の Hide が今を空にする共有経路なら、逆待から除外せず F(S) を先に", func(t *testing.T) {
		h := newHB(t, fbAD)
		h.setStates(OK, DOWN)
		h.setAuto(deskScene, camera) // D（s だけ）にいて s が悪い。A は逆待（x が A で表示中）
		out := h.tick(1000)
		h.outP(out, "F(S;+D)")
	})
}

func TestFBSQ05(t *testing.T) {
	// サブだけ悪化（spec「隠すとの関係」）。
	h := newHB(t, fbAD)
	hc := h.cams(1000, OK, DOWN)
	h.outP(hc, "Hc(s)[x,y]")
	h.wantStack("")
	h.outP(h.hideAll(hc[0], 1100), "")
	h.wantScene(camera)
	sc := h.cams(7000, OK, OK)
	if len(sc) != 1 || sc[0].Purpose != PurposeShowCamera {
		t.Fatalf("Sc(s) でない: %s", pdescs(sc))
	}
	h.outP(h.showAll(sc[0], 7100), "")
	h.wantScene(camera)
	h.wantStack("")
	h.wantOwner(OwnerNone, "")
}

func TestFBSQ06(t *testing.T) {
	// 履歴と新しい列の重なり。C は m が映る別の監視シーン。
	t.Run("cam1", func(t *testing.T) {
		h := newHB(t, hbMapEdit(addSceneC), hbAt(sceneC))
		h.m.st.CameraStates[camMain] = DOWN
		h.setAuto(sceneC, camera, camera2) // 前提: auto/[A,B]、現在 C 不良、A/B 不可、D 可（s OK）。判断はまだ走らせない
		ce := h.m.st.ConfigEpoch
		c := h.cfg
		c.At = at(1000)
		c.SceneFallbacks = []FallbackRow{{Scene: "C", Fallback: "Camera2"}, fbRow(camera2, deskScene)}
		out := h.ev(c)
		h.outP(out, "F(D;+C)") // 設定 C→B→D→S: B を積み直さず飛ばして D へ
		if h.m.st.ConfigEpoch != ce+1 {
			t.Fatalf("ConfigEpoch %d→%d", ce, h.m.st.ConfigEpoch)
		}
		h.wantStack("A,B")
		p := h.approveSet(out[0], 1100) // 最新も A/B 不可 D 可: Proceed、期待固定（B を追加しない）
		h.completeBatch(out[0], p.ReqID, 1200)
		h.wantStack("A,B")
		h.outP(h.scene(1300, deskScene), "")
		h.wantStack("A,B,C")
	})
	t.Run("下から最初の可候補 B", func(t *testing.T) {
		// C が s だけの fixture（C は悪く、m の B は可、A は s の逆待で不可）。
		h := newHB(t, hbMapEdit(addSceneCs), hbAt(sceneC), hbFB(FallbackRow{Scene: "C", Fallback: "Camera2"}))
		h.setStates(OK, DOWN)
		h.setAuto(sceneC, camera, camera2)
		out := h.tick(1000)
		h.outP(out[:1], "R(B)")
		h.settle(out[0], 1100) // 着いた B で、悪い s の Hide（x,y と C の s の項目）が出る
		h.wantScene(camera2)
		h.wantStack("A")
	})
	t.Run("別列: t=1 で B が可なら F でなく R(B;[A])", func(t *testing.T) {
		h := newHB(t, hbMapEdit(addSceneCs), hbAt(sceneC))
		h.setStates(OK, DOWN)
		h.setAuto(sceneC, camera, camera2)
		c := h.cfg
		c.At = at(1000)
		c.SceneFallbacks = []FallbackRow{{Scene: "C", Fallback: "Camera2"}, fbRow(camera2, deskScene)}
		out := h.ev(c)
		h.outP(out[:1], "R(B)")
	})
}

func TestFBSQ07(t *testing.T) {
	// 削除・期待・遅着（同じ ConfigEpoch）。B（m が映る）が不良で、B→C。C は s だけが映る。
	mk := func(t *testing.T) *hb {
		h := newHB(t, hbMapEdit(addSceneCs), hbAt(camera2), hbFB(FallbackRow{Scene: nameOf(camera2), Fallback: "C"}))
		h.setStates(DOWN, OK)
		h.setAuto(camera2, camera)
		return h
	}
	t.Run("cam1", func(t *testing.T) {
		h := mk(t)
		ce := h.m.st.ConfigEpoch
		f := h.tick(1000)
		h.outP(f, "F(C;+B)")
		h.approveSet(f[0], 1100) // 期待登録、K=[A]
		h.wantStack("A")
		e := h.m.st.SceneEpoch
		h.outP(h.smc(1200, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, camera) })), "") // A 削除。同目的の Pending がある間は []
		if h.m.st.SceneEpoch != e+1 || len(h.m.st.Expectations) != 0 || len(h.m.st.LastCandidateSent) != 0 || h.m.st.ConfigEpoch != ce {
			t.Fatalf("全解除でない: scene %d→%d exps=%d", e, h.m.st.SceneEpoch, len(h.m.st.Expectations))
		}
		h.wantStack("")
		// t=0.3: 別の未承認の旧バッチは古い SceneEpoch で Reject。
		h.m.st.Pending[90] = PendingBatch{BatchID: 90, Purpose: PurposeAutoFail, SceneEpoch: e, ConnEpoch: h.m.st.ConnEpoch,
			StreamEpoch: h.m.st.StreamEpoch, ConfigEpoch: ce, Plan: ArrivalPlan{Plan: Plan{Kind: PlanEscape, Target: sceneC, Append: []string{camera2}}}}
		if r := h.m.Approve(90, SetScene{SceneUUID: sceneC}, OverrideNone{}, at(1300)); r != (Reject{}) {
			t.Fatalf("旧バッチ: %#v", r)
		}
		h.wantNoExpectations()
		delete(h.m.st.Pending, 90)
		// t=0.4: 旧 F の BD。B 不良・C 可: 新しい F(C;+B)（最下段は B、古い A なし）。
		out := h.bdone(1400, f[0].BatchID, BatchCompleted, nil)
		h.outP(out, "F(C;+B)")
		if letters(out[0].Plan.Plan.Append) != "B" {
			t.Fatalf("新しい計画: %s", pdesc(out[0]))
		}
		// t=0.5: C を不良へ更新したあと、送信済み C の遅着 SC、C→S。期待なしなので手動共通解除。旧計画は復活しない。
		h.setStates(DOWN, DOWN)
		h.outP(h.scene(1500, sceneC), "") // 同目的の Pending があるので出ない
		h.wantStack("")
		h.wantOwner(OwnerNone, "")
		// 続き: t=0.6 新 F の承認は SceneEpoch で Reject。BD Rejected で F(S;+C)。
		if r := h.m.Approve(out[0].BatchID, SetScene{SceneUUID: sceneC}, OverrideNone{}, at(1600)); r != (Reject{}) {
			t.Fatalf("新 F の承認: %#v", r)
		}
		bs := h.bdone(1700, out[0].BatchID, BatchRejected, nil)
		h.outP(bs[:1], "F(S;+C)")
	})
	t.Run("d: 新 F を先に承認してから SC(C) なら新期待と一致し auto/[B]", func(t *testing.T) {
		h := mk(t)
		f := h.tick(1000)
		h.approveSet(f[0], 1100)
		h.smc(1200, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, camera) }))
		out := h.bdone(1400, f[0].BatchID, BatchCompleted, nil)
		h.outP(out, "F(C;+B)")
		p := h.approveSet(out[0], 1450)
		h.completeBatch(out[0], p.ReqID, 1460)
		h.outP(h.scene(1500, sceneC), "")
		h.wantStack("B")
	})
	t.Run("b: auto=false なら B/C 不良でも新規 F なし", func(t *testing.T) {
		h := mk(t)
		c := h.cfg
		c.At = at(1000)
		c.AutoEnabled = false
		c.SceneFallbacks = []FallbackRow{{Scene: nameOf(camera2), Fallback: "C"}}
		h.outP(h.ev(c), "")
		h.outP(h.tick(2000), "")
	})
	t.Run("c: 途中の要素だけの削除は残りを保持し SceneEpoch 不変", func(t *testing.T) {
		h := mk(t)
		h.setAuto(talk, camera, deskScene, camera2)
		e := h.m.st.SceneEpoch
		h.smc(1000, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, deskScene) }))
		h.wantStack("A,B")
		if h.m.st.SceneEpoch != e {
			t.Fatalf("SceneEpoch が進んだ")
		}
	})
}

func TestFBSQ09(t *testing.T) {
	// 最低滞在時間なしで2段切替。
	h := newHB(t, fbAD)
	f := h.cams(1000, DOWN, OK)
	h.outP(f, "F(D;+A)")
	h.outP(h.settle(f[0], 1100), "")
	h.wantStack("A")
	f = h.cams(2000, DOWN, DOWN) // 前の F から5秒未満でも新しい要求。滞在待ちなし
	h.outP(f, "F(S;+D)")
	out := h.settle(f[0], 2100)
	h.outP(out, "Hc(s)[x,y]")
	h.wantStack("A,D")
}

func TestFBSQ11(t *testing.T) {
	// Studio ignore・表示0・上位候補。m/s ともに hide、A は m のみ（z）、D は s のみ（y）。
	h := newHB(t, fbAD, hbBothHide(), hbMapEdit(fb11Map), hbStudioHide(StudioHideIgnore))
	h.setStates(DOWN, DOWN)
	h.setAuto(standby, camera, deskScene)
	h.seedBody(kzA, camMain, inMain)
	h.seedBody(ky, camSub, inSub)
	// t=1: Studio on/ignore → 両方健康 OK でも表示0、S に残る。
	h.outP(h.ev(StudioModeChanged{At: at(1000), Enabled: true}), "")
	h.wantStack("A,D")
	// t=2: s OK → Sc(s)。D 表示待ち。
	sc := h.cams(2000, DOWN, OK)
	h.outP(sc, "Sc(s)[R(is),y]")
	// show 決着(y) → R(D;[A])。A は表示0で不可。
	r := h.showAll(sc[0], 2100)
	h.outP(r, "R(D)")
	h.outP(h.settle(r[0], 3300), "") // 3.3 scene 決着(D)
	h.wantScene(deskScene)
	h.wantStack("A")
	// t=8: m OK → Sc(m)。A 表示待ち。
	scm := h.cams(8000, OK, OK)
	h.outP(scm, "Sc(m)[m1]") // m の入力は B にも映っているので再起動しない
	r = h.showAll(scm[0], 8100)
	h.outP(r, "R(A)") // A 準備完了
	h.outP(h.settle(r[0], 9300), "")
	h.wantScene(camera)
	h.wantStack("")
}

func TestFBMP11(t *testing.T) {
	// 共有末端の false で R 先が表示0: R を生成しない。承認待ちの R も Reject。
	h := newHB(t, fbAD, hbBothHide(), hbMapEdit(fb11Map), hbStudioHide(StudioHideIgnore))
	h.setStates(OK, OK)
	h.setAuto(standby, camera)
	h.seedBody(kzA, camMain, inMain) // z（A の m）は本体が隠した記録つきで false
	h.ev(StudioModeChanged{At: at(500), Enabled: true})
	h.m.st.CameraStates[camMain] = DOWN
	h.outP(h.tick(8000), "")
	if f := h.m.enterFacts(camera, DirReturn); !f.ActualZero {
		t.Fatalf("A の表示0: %+v", f)
	}
	h.wantStack("A")
}

func TestFBSQ13(t *testing.T) {
	// 承認前後・応答順・期限・再送。
	h := newHB(t, fbAD)
	f := h.cams(1000, DOWN, OK)
	h.outP(f, "F(D;+A)")
	h.wantStack("")
	hc := h.cams(1100, DOWN, DOWN) // D 悪化。F は Pending なので出ず、項目の Hc だけ
	h.outP(hc, "Hc(s)[x,y]")
	if r := h.m.Approve(f[0].BatchID, SetScene{SceneUUID: deskScene}, OverrideNone{}, at(1200)); r != (Reject{}) {
		t.Fatalf("F(D) の承認: %#v", r)
	}
	h.wantNoExpectations()
	h.wantStack("")
	f2 := h.bdone(1300, f[0].BatchID, BatchRejected, nil)
	h.outP(f2, "F(S;+A,D)") // 新候補は即時
	p := h.approveSet(f2[0], 1400)
	h.outP(h.hideAll(hc[0], 1450), "") // x/y は H・false
	// t=0.4: s OK（D 回復）。SC(S) が SRD より先に届く: 送出計画を一度だけ適用（承認後の変化で期待を捨てない）。
	h.outP(h.cams(1500, DOWN, OK), "")
	h.outP(h.scene(1550, standby), "")
	h.wantStack("A,D")
	h.outP(h.completeBatch(f2[0], p.ReqID, 1600), "") // 二重追加なし
	h.wantStack("A,D")
	// 続き: t=5.1 で Sc(s)、show 決着後 R(D;[A])、到着で auto/[A]。
	sc := h.tick(6200)
	if len(sc) != 1 || sc[0].Purpose != PurposeShowCamera {
		t.Fatalf("Sc(s) でない: %s", pdescs(sc))
	}
	r := h.showAll(sc[0], 6300)
	h.outP(r, "R(D)")
	h.outP(h.settle(r[0], 6500), "")
	h.wantStack("A")
}

func TestFBSQ13Columns(t *testing.T) {
	t.Run("b: Timeout は期待を期限まで保持。期限ちょうどで残り、超過で消える", func(t *testing.T) {
		h := newHB(t, fbAD)
		f := h.cams(1000, DOWN, OK)
		p := h.approveSet(f[0], 1100)
		h.ev(SceneRequestDone{At: at(2000), ReqID: p.ReqID, Result: RequestTimeout})
		dl := h.m.st.Expectations[0].Deadline
		h.ev(Tick{At: dl})
		if len(h.m.st.Expectations) != 1 {
			t.Fatalf("期限ちょうどで消えた")
		}
		h.ev(Tick{At: dl.Add(1)})
		h.wantNoExpectations()
		h.wantStack("")
	})
	t.Run("c: 期限後の遅着は手動切替（V-08。旧計画は復活しない）", func(t *testing.T) {
		h := newHB(t, fbAD)
		f := h.cams(1000, DOWN, OK)
		p := h.approveSet(f[0], 1100)
		h.ev(SceneRequestDone{At: at(2000), ReqID: p.ReqID, Result: RequestTimeout})
		dl := h.m.st.Expectations[0].Deadline
		h.ev(Tick{At: dl.Add(1)})
		e := h.m.st.SceneEpoch
		h.scene(int(dl.Sub(t0)/1e6)+100, deskScene)
		if h.m.st.SceneEpoch != e+1 {
			t.Fatalf("手動切替でない")
		}
		h.wantStack("")
	})
}

func TestFBSQ14(t *testing.T) {
	// 古い設定世代・共有入力・Restart 待ち（主な列）。s=standby、m 不良、s OK。R(D) の I=[is]。
	h := newHB(t, fbAD, hbSubStandby())
	h.setStates(DOWN, OK)
	h.setAuto(standby, camera, deskScene)
	r := h.tick(1000)
	h.outP(r, "R(D)")
	h.wantRestartActions(r[0], restartOf(inSub), SetScene{SceneUUID: deskScene}) // 固定 I から現 program の実表示入力を除外済み
	if res := h.m.Approve(r[0].BatchID, r[0].Actions[0], OverrideNone{}, at(1100)); !isProceed(res) {
		t.Fatalf("Restart = %#v", res)
	}
	// t=0.5 相当: 待ち中に D の表示が false（記録なし）になる。
	h.outP(h.itemEv(1200, ky, false), "")
	if res := h.m.Approve(r[0].BatchID, r[0].Actions[1], OverrideNone{}, at(2200)); res != (Reject{}) {
		t.Fatalf("SetScene = %#v, want Reject", res)
	}
	// 表は「K 維持」だが、D は表示0（記録なし）で管理外になるため、履歴から外れて K=[A]（FB-ST-12 の整理。表の不明点として報告）。
	h.wantStack("A")
	h.outP(h.bdone(2300, r[0].BatchID, BatchRejected, nil), "")
}

func TestFBSQ10(t *testing.T) {
	// 画像終端・OFF 専用範囲。A→T→D、A 不良。
	h := newHB(t, hbFB(fbRow(camera, talk), fbRow(talk, deskScene)))
	f := h.cams(1000, DOWN, OK)
	h.outP(f, "F(T;+A)") // T に設定行があっても終端
	h.outP(h.settle(f[0], 1100), "")
	h.wantScene(talk)
	h.wantStack("A")
	// t=1: ON → ON(A): 戻り先は T ではなく A。
	on := h.mustCmd(h.cmdOn(2000))
	if len(on) != 1 || on[0].Purpose != PurposeStandbyOn || on[0].ReturnScene != camera {
		t.Fatalf("ON(A) でない: %+v", on)
	}
	h.settle(on[0], 2100)
	h.wantOwner(OwnerManual, camera)
	h.wantStack("")
	// t=2: OFF、A 不可/T 可 → OFF(T;[A]): T で止める、D まで探索しない。
	off := h.mustCmd(h.cmdOff(3000))
	h.out(off, "M(T)")
	h.settle(off[0], 3100)
	h.wantScene(talk)
	h.wantStack("A") // ON 前の履歴を合成しない（auto/[A]）
	h.wantOwner(OwnerAuto, camera)
	// t=3: A 回復、準備完了 → R(A)。T からも復帰。
	r := h.cams(9000, OK, OK)
	h.outP(r, "R(A)")
	h.outP(h.settle(r[0], 9100), "")
	h.wantScene(camera)
	h.wantStack("")
	h.wantOwner(OwnerNone, "")
}

func TestFBSQ12(t *testing.T) {
	// 再接続・auto=false・開始時切替。
	h := newHB(t, fbAD, hbStart(camera))
	// t=0: D、auto/[A]、m 不良、s OK。
	h.toDAuto(500)
	// t=1: OBSDisconnected: K 保持、ConnEpoch+1、期待消去。
	ce := h.m.st.ConnEpoch
	h.outP(h.ev(OBSDisconnected{At: at(1000)}), "")
	if h.m.st.ConnEpoch != ce+1 {
		t.Fatalf("ConnEpoch")
	}
	h.wantStack("A")
	h.wantNoExpectations()
	// t=2: 同じ 4 条件で OBSConnected: 新地図で [A] 保持、StartScene なし。
	h.outP(h.connect(2000, true, deskScene, false), "")
	h.wantStack("A")
	// t=3: auto=false: C、D に残る。
	c := h.cfg
	c.At = at(3000)
	c.AutoEnabled = false
	h.outP(h.ev(c), "")
	h.wantStack("")
	h.wantOwner(OwnerNone, "")
	h.wantScene(deskScene)
	// t=4: m OK: 旧 A へ戻らない。
	h.outP(h.cams(4000, OK, OK), "")
	h.wantScene(deskScene)
	// t=5: auto=true: D 健康 OK、過去履歴を復活しない。
	c.At = at(5000)
	c.AutoEnabled = true
	h.outP(h.ev(c), "")
	h.wantStack("")
	// t=5.5: m DOWN: D 健康 OK、履歴なし。
	h.outP(h.cams(5500, DOWN, OK), "")
	h.wantStack("")
	// t=6: 配信停止: 期待・時刻消去、K=[]。
	h.outP(h.ev(StreamStateChanged{At: at(6000), Streaming: false}), "")
	h.wantNoExpectations()
	h.wantStack("")
	// t=7: 配信開始、on_stream_start=A → StartScene(A)。A 不良でも X 中 Auto/Hide なし。
	bs := h.ev(StreamStateChanged{At: at(7000), Streaming: true})
	h.out(bs, "St(A)")
	p := h.approveSet(bs[0], 7050)
	// t=7.1: SC(A) を先に受ける: まだ StartScene Pending。
	h.outP(h.scene(7100, camera), "")
	// t=7.2: SRD/BD、A 不良 → F(D;+A)。StartScene 再送なし。
	h.ev(SceneRequestDone{At: at(7200), ReqID: p.ReqID, Result: RequestOK})
	h.outP(h.bdone(7200, bs[0].BatchID, BatchCompleted, nil), "F(D;+A)")
}

func TestFBSQ12Reconnect4Conditions(t *testing.T) {
	// 別列: 再接続の 4 条件を 1 つずつ変え、すべて C を検査。collection の差は UUID が同一でも解除。
	for name, f := range map[string]func(h *hb){
		"scene":      func(h *hb) { h.connect(2100, true, camera, false) },
		"streaming":  func(h *hb) { h.connect(2100, false, standby, false) },
		"collection": func(h *hb) { h.mp.Collection = "collection-y"; h.connect(2100, true, standby, false) },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.ev(OBSDisconnected{At: at(2000)})
			f(h)
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
		})
	}
}

func TestFBSQ14T4(t *testing.T) {
	// 別列 a〜c（d は FB-AP-10/subset）。
	t.Run("a/old_config_R_arrival_is_C", func(t *testing.T) {
		// SetScene 承認後、設定変更→古い復帰の SC 一致なら部分復帰せず C。
		h := newHB(t, fbAD)
		h.withRecoverExp()
		c := h.cfg
		c.At = at(8900)
		c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene), fbRow(deskScene, talk)}
		h.ev(c)
		h.scene(9000, camera)
		h.wantStack("")
		h.wantOwner(OwnerNone, "")
	})
	t.Run("b/old_F_and_ON_arrival", func(t *testing.T) {
		// 古い F の期待: 追加/所有権設定なし、履歴がなければ none で到着先に残る。古い ON も manual にしない。
		h := newHB(t, fbAD)
		f := h.cams(1000, DOWN, OK)
		h.outP(f, "F(D;+A)")
		h.approveSet(f[0], 1100)
		c := h.cfg
		c.At = at(1200)
		c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene), fbRow(deskScene, talk)}
		h.ev(c)
		h.scene(1300, deskScene)
		h.wantStack("")
		h.wantOwner(OwnerNone, "")
		h2 := newHB(t, fbAD)
		on := h2.mustCmd(h2.cmdOn(1000))
		h2.approveSet(on[0], 1100)
		c = h2.cfg
		c.At = at(1200)
		c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene), fbRow(deskScene, talk)}
		h2.ev(c)
		h2.scene(1300, standby)
		h2.wantOwner(OwnerNone, "")
		h2.wantStack("")
	})
	t.Run("c/shared_input_current_scene", func(t *testing.T) {
		// A と現在 B が同入力 im を共有: R(A) の生成時に im を除外、再起動なしで SetScene。
		h := newHB(t, fbAD)
		h.setAuto(camera2, camera)
		r := h.tick(1000)
		h.outP(r, "R(A)")
		h.wantRestartActions(r[0], SetScene{SceneUUID: camera})
	})
}
