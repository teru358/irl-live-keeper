package switcher

import (
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 表 0c 4.1 CamerasChanged、4.2 SceneChanged と SceneRequestDone、4.3 SceneMapChanged。

func TestFBCC(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-CC-01", func(t *testing.T) {
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			h.outP(bs, "F(D;+A)")
			if bs[0].BadCamera != camMain || len(bs[0].Actions) != 1 {
				t.Fatalf("BadCamera=%q actions=%v（再起動なし）", bs[0].BadCamera, bs[0].Actions)
			}
			h.wantStack("") // 到着前は none
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-CC-02", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.setAuto(deskScene, camera)
			bs := h.cams(1000, OK, DOWN)
			h.outP(bs, "F(S;+D)")
			h.wantStack("A") // 到着前
			h.settle(bs[0], 1100)
			h.wantStack("A,D")
		}},
		{"FB-CC-03", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.toSAD(1000)
			bs := h.cams(8000, DOWN, OK) // 前の Hide から5秒以上あと（項目の再送の間隔）
			if len(bs) != 1 || bs[0].Purpose != PurposeShowCamera {
				t.Fatalf("Sc(s) でない: %s", pdescs(bs))
			}
			h.wantStack("A,D") // Show 確認まで R なし
		}},
		{"FB-CC-04", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera, deskScene)
			bs := h.tick(1000)
			h.outP(bs, "Hc(s)[x,y]") // A は健康 OK でも、表示中の悪い s が隠れるまで戻らない
			h.outP(h.sendFalseOut(bs[0], 1100, kx, "R(A)"), "R(A)")
			h.wantStack("A,D")
		}},
		{"FB-CC-05", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.setStates(DOWN, DOWN)
			h.setAuto(standby, camera, deskScene)
			bs := h.cams(1000, OK, OK)
			h.outP(bs, "R(A)") // 下から A を選ぶ。D を経由しない
			h.wantStack("A,D")
			h.settle(bs[0], 1100)
			h.wantStack("")
			h.wantScene(camera)
		}},
		{"FB-CC-06", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.setStates(OK, OK)
			h.setAuto(deskScene, camera)
			h.outP(h.cams(1000, OK, DOWN), "F(S;+D)") // A は逆待。相互待ちを先送りしない
		}},
		{"FB-CC-07", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.outP(h.cams(1000, OK, DOWN), "Hc(s)[x,y]")
			h.wantScene(camera)
			h.wantStack("")
		}},
		{"FB-CC-08", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.outP(h.cams(1000, DOWN, DOWN), "Hc(s)[x,y]")
			h.wantOwner(OwnerManual, camera)
			h.wantScene(standby)
		}},
		{"FB-CC-09", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.cams(1000, OK, OK)
			h.outP(h.cams(1100, OK, OK), "")
			h.outP(h.ev(CamerasChanged{At: at(1200), States: map[string]scenemap.CameraState{camMain: OK, camSub: OK, "unknown": DOWN}}), "")
		}},
		{"FB-CC-10", func(t *testing.T) {
			// t=0 F(D) 失敗、t=1 CamerasChanged: 同じ候補の再送は全入口で最短5秒。
			h := newHB(t, fbAD)
			bs := h.cams(0, LOW, OK)
			h.outP(bs, "F(D;+A)")
			p := h.approveSet(bs[0], 0)
			h.failBatch(bs[0], p.ReqID, 100)
			h.outP(h.cams(1000, DOWN, OK), "")
			h.outP(h.tick(4999), "")
			h.outP(h.tick(5000), "F(D;+A)")
		}},
	})
}

// fbTwoExpectations は FB-SC-06・14 の前提: 現在 A、m/s 不良、x/y は本体が隠した H、期待は F(D;+A) と F(S;+A,D)（登録順）。
func (h *hb) fbTwoExpectations() {
	h.t.Helper()
	h.setStates(DOWN, DOWN)
	h.seedBody(kx, camSub, inSub)
	h.seedBody(ky, camSub, inSub)
	ce := h.m.st.ConfigEpoch
	for i, x := range []struct {
		to string
		ap []string
	}{{deskScene, []string{camera}}, {standby, []string{camera, deskScene}}} {
		h.m.st.Expectations = append(h.m.st.Expectations, Expectation{ReqID: i + 1, Scene: x.to, Purpose: PurposeAutoFail, ReturnScene: camera,
			ConfigEpoch: ce, Override: OverrideNone{}, Plan: ArrivalPlan{Plan: Plan{Kind: PlanEscape, Target: x.to, Append: x.ap}}})
	}
}

func TestFBSC(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-SC-01", func(t *testing.T) {
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			e := h.m.st.SceneEpoch
			h.outP(h.settle(bs[0], 1100), "")
			h.wantStack("A")
			h.wantOwner(OwnerAuto, camera)
			if h.m.st.SceneEpoch != e {
				t.Fatalf("SceneEpoch が進んだ")
			}
		}},
		{"FB-SC-02", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.setStates(DOWN, OK)
			h.setAuto(standby, camera, deskScene)
			bs := h.tick(1000)
			h.outP(bs, "R(D)")
			h.outP(h.settle(bs[0], 1100), "")
			h.wantStack("A") // D 自身を残さない
			h.wantScene(deskScene)
		}},
		{"FB-SC-03", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.setAuto(standby, camera, deskScene)
			e := h.m.st.SceneEpoch
			bs := h.tick(1000)
			h.outP(bs, "R(A)")
			h.wantStack("A,D") // 到着でだけ解除
			h.outP(h.settle(bs[0], 1100), "")
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
			if h.m.st.SceneEpoch != e {
				t.Fatalf("SceneEpoch が進んだ")
			}
		}},
		{"FB-SC-04", func(t *testing.T) {
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.wantOwner(OwnerManual, camera)
			h.wantStack("")
		}},
		{"FB-SC-05", func(t *testing.T) {
			// 応答前に一致する到着: 1回だけ適用し、後の SRD は無作用。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			p := h.approveSet(bs[0], 1100)
			h.outP(h.scene(1200, deskScene), "")
			h.wantStack("A")
			h.ev(SceneRequestDone{At: at(1300), ReqID: p.ReqID, Result: RequestOK})
			h.bdone(1300, bs[0].BatchID, BatchCompleted, nil)
			h.wantStack("A")
		}},
		{"FB-SC-06", func(t *testing.T) {
			// 期待 F(D;+A) と F(S;+A,D)、m/s 不良、x/y H、S だけ届く: S の計画だけ適用、auto/[A,D]。
			h := newHB(t, fbAD)
			h.fbTwoExpectations()
			h.outP(h.scene(2300, standby), "")
			h.wantStack("A,D")
			h.wantNoExpectations() // D 宛の前の期待も一緒に消費
		}},
		{"FB-SC-07", func(t *testing.T) {
			// 同じ D 宛 F の期待が2件: 最初の計画だけ適用し、後の同宛先期待も消費。
			h := newHB(t, fbAD)
			h.setStates(DOWN, OK)
			ce := h.m.st.ConfigEpoch
			for i, app := range [][]string{{camera}, {camera, camera2}} {
				h.m.st.Expectations = append(h.m.st.Expectations, Expectation{ReqID: i + 1, Scene: deskScene, Purpose: PurposeAutoFail,
					ReturnScene: camera, ConfigEpoch: ce, Override: OverrideNone{},
					Plan: ArrivalPlan{Plan: Plan{Kind: PlanEscape, Target: deskScene, Append: app}}})
			}
			h.outP(h.scene(1000, deskScene), "")
			h.wantStack("A")
			h.wantNoExpectations()
		}},
		{"FB-SC-08", func(t *testing.T) {
			// 消費済みの D の重複 SC: 手動共通解除、D・none。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			h.settle(bs[0], 1100)
			e := h.m.st.SceneEpoch
			h.outP(h.scene(1300, deskScene), "")
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
			if h.m.st.SceneEpoch != e+1 {
				t.Fatalf("手動共通解除でない")
			}
		}},
		{"FB-SC-09", func(t *testing.T) {
			// 古い ConfigEpoch の F(D) 期待 → D: 照合だけ、D・none、履歴追加なし。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			h.approveSet(bs[0], 1100)
			c := h.cfg
			c.At = at(1200)
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene), fbRow(talk, standby)}
			h.ev(c)
			h.scene(1300, deskScene)
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
			h.wantNoExpectations()
		}},
		{"FB-SC-10", func(t *testing.T) {
			// 古い ConfigEpoch の R(A) 期待 → A: C。古い部分切詰めなし。
			h := newHB(t, fbAD)
			h.setAuto(standby, camera, deskScene)
			bs := h.tick(1000)
			h.outP(bs, "R(A)")
			h.approveSet(bs[0], 1100)
			c := h.cfg
			c.At = at(1200)
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene), fbRow(talk, standby)}
			h.ev(c)
			h.wantStack("A,D")
			h.scene(1300, camera)
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-SC-12", func(t *testing.T) {
			// 人が B 不良へ切替、B→S: 手動共通解除後、到着前 none、到着で auto/[B]。
			h := newHB(t, fbAD)
			h.setStates(DOWN, OK)
			bs := h.scene(1000, camera2)
			h.outP(bs, "F(S;+B)")
			h.wantStack("")
			h.settle(bs[0], 1100)
			h.wantStack("B")
		}},
		{"FB-SC-13", func(t *testing.T) {
			// 人が S/T/健康な A へ切替: 手動共通解除、回復しても旧履歴へ戻らない（R が出ない。Show は独立に出る）。
			for _, to := range []string{standby, talk, camera} {
				h := newHB(t, fbAD)
				h.toSAD(1000)
				h.setStates(OK, OK)
				bs := h.scene(8000, to)
				bs = append(bs, h.tick(9000)...)
				for _, b := range bs {
					if b.Purpose == PurposeAutoRecover || b.Purpose == PurposeAutoFail {
						t.Fatalf("%s へ切替のあとに %s", letter(to), pdesc(b))
					}
				}
				h.wantStack("")
				h.wantOwner(OwnerNone, "")
			}
		}},
		{"FB-SC-14", func(t *testing.T) {
			// F(D;+A) と F(S;+A,D) の期待、m/s 不良・x/y H、SC(D) → SC(S): [A] → [A,D]。既存の A を追加しない。
			h := newHB(t, fbAD)
			h.fbTwoExpectations()
			h.outP(h.scene(2300, deskScene), "")
			h.wantStack("A")
			h.outP(h.scene(2400, standby), "")
			h.wantStack("A,D")
		}},
		{"FB-SC-15", func(t *testing.T) {
			// ReturnUUID=D の計画、D が管理外化して K=[A]、一致到着 D: 切詰めなし、整理後 [A]。
			h := newHB(t, fbAD)
			h.setStates(DOWN, OK)
			h.setAuto(standby, camera, deskScene)
			bs := h.tick(1000)
			h.outP(bs, "R(D)")
			h.approveSet(bs[0], 1100)
			h.smc(1200, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[deskScene]
				c.Items = nil
				m.Scenes[deskScene] = c
			}))
			h.wantStack("A")
			h.scene(1300, deskScene)
			h.wantStack("A")
		}},
		{"FB-SRD-01", func(t *testing.T) {
			// SRD OK / Timeout: 期限式は0bのまま。履歴は未確定。
			for _, res := range []RequestResult{RequestOK, RequestTimeout} {
				h := newHB(t, fbAD)
				bs := h.cams(1000, DOWN, OK)
				p := h.approveSet(bs[0], 1100)
				h.outP(h.ev(SceneRequestDone{At: at(1200), ReqID: p.ReqID, Result: res}), "")
				h.wantStack("")
				if len(h.m.st.Expectations) != 1 || !h.m.st.Expectations[0].Responded {
					t.Fatalf("期待: %+v", h.m.st.Expectations)
				}
			}
		}},
		{"FB-SRD-02", func(t *testing.T) {
			// SRD Error: 該当期待を消し、履歴不変。同一要求の時刻は保持。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			p := h.approveSet(bs[0], 1100)
			h.outP(h.ev(SceneRequestDone{At: at(1200), ReqID: p.ReqID, Result: RequestError}), "")
			h.wantNoExpectations()
			h.wantStack("")
			if len(h.m.st.LastCandidateSent) != 1 {
				t.Fatalf("候補の時刻が消えた")
			}
		}},
		{"FB-SRD-03", func(t *testing.T) {
			// 到着で消費済みの ReqID の結果: 無視、二重適用なし。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			p := h.approveSet(bs[0], 1100)
			h.scene(1200, deskScene)
			h.wantStack("A")
			h.outP(h.ev(SceneRequestDone{At: at(1300), ReqID: p.ReqID, Result: RequestOK}), "")
			h.wantStack("A")
		}},
	})
}

func TestFBMP(t *testing.T) {
	noD := func(m *scenemapMap) { delete(m.Scenes, deskScene) }
	runHB(t, []hbRow{
		{"FB-MP-01", func(t *testing.T) {
			// A 不良、D 名復活、D 入れる: 新しい解決を使う。
			h := newHB(t, fbAD, hbMapEdit(noD))
			h.setStates(DOWN, OK)
			full := hbMap()
			h.outP(h.smc(1000, full), "F(D;+A)")
			if h.m.st.Fallbacks[0].Fallback != "Desk" {
				t.Fatalf("設定の文字列が変わった")
			}
		}},
		{"FB-MP-02", func(t *testing.T) {
			// auto/[A,D]、S、A 削除: 手動共通解除、SceneEpoch+1、期待・時刻全消去。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			e := h.m.st.SceneEpoch
			h.outP(h.smc(2000, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, camera) })), "")
			if h.m.st.SceneEpoch != e+1 || len(h.m.st.LastCandidateSent) != 0 || len(h.m.st.Expectations) != 0 {
				t.Fatalf("全消去でない: epoch %d→%d", e, h.m.st.SceneEpoch)
			}
			h.wantStack("")
		}},
		{"FB-MP-03", func(t *testing.T) {
			// auto/[A]、B 不良、B→D、A 削除: 解除後の同じ入口で新規判断。到着で [B]。
			h := newHB(t, hbFB(fbRow(camera2, deskScene)), hbAt(camera2))
			h.setStates(DOWN, OK)
			h.setAuto(camera2, camera)
			bs := h.smc(1000, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, camera) }))
			h.outP(bs, "F(D;+B)")
			h.wantStack("")
			h.settle(bs[0], 1100)
			h.wantStack("B")
		}},
		{"FB-MP-04", func(t *testing.T) {
			// [A,D,B]、T、途中の D 削除、A/B 不可: [A,B]、auto 維持。全解除しない。
			h := newHB(t, fbAD)
			h.setStates(DOWN, OK)
			h.setAuto(talk, camera, deskScene, camera2)
			e := h.m.st.SceneEpoch
			h.outP(h.smc(1000, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, deskScene) })), "")
			h.wantStack("A,B")
			if h.m.st.SceneEpoch != e {
				t.Fatalf("SceneEpoch が進んだ")
			}
		}},
		{"FB-MP-05", func(t *testing.T) {
			// [A,D]、S、A 管理外化、D 不良: [D]、auto。A へ戻さない。
			h := newHB(t, fbAD)
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera, deskScene)
			h.seedBody(ky, camSub, inSub) // y は本体が隠した H
			h.outP(h.smc(1000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[camera]
				c.Items = nil
				m.Scenes[camera] = c
			})), "")
			h.wantStack("D")
			h.wantScene(standby)
		}},
		{"FB-MP-06", func(t *testing.T) {
			// [A]、S、A 管理外化: none。削除ではないので手動共通解除なし。
			h := newHB(t, fbAD)
			h.setAuto(standby, camera)
			e := h.m.st.SceneEpoch
			h.smc(1000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[camera]
				c.Items = nil
				m.Scenes[camera] = c
			}))
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
			if h.m.st.SceneEpoch != e {
				t.Fatalf("SceneEpoch が進んだ")
			}
		}},
		{"FB-MP-07", func(t *testing.T) {
			// manual/A、S、A 管理外化: manual/A、OFF で Other として戻す。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.smc(1000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[camera]
				c.Items = nil
				m.Scenes[camera] = c
			}))
			h.wantOwner(OwnerManual, camera)
			bs, err := h.evErr(Command{At: at(2000), Kind: CmdStandbyOff})
			if err != nil || len(bs) != 1 || bs[0].Purpose != PurposeStandbyOffOther {
				t.Fatalf("OFF: %v %s", err, pdescs(bs))
			}
		}},
		{"FB-MP-08", func(t *testing.T) {
			// 現在 D の削除の Map が先、移動先 T の SC は未着、m 不良/s OK: Map だけで架空の到着なし。後の SC(T) で手動共通解除。
			h := newHB(t, fbAD)
			h.setStates(DOWN, OK)
			h.setAuto(deskScene, camera)
			h.outP(h.smc(1000, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, deskScene) })), "")
			h.wantStack("A")
			e := h.m.st.SceneEpoch
			h.outP(h.scene(1100, talk), "")
			h.wantStack("")
			if h.m.st.SceneEpoch != e+1 {
				t.Fatalf("手動共通解除でない")
			}
		}},
		{"FB-MP-09", func(t *testing.T) {
			// S、auto/[A,D]、D 改名（UUID 不変）: 履歴維持、SceneEpoch 不変、名前対応を引き直す。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			e := h.m.st.SceneEpoch
			h.outP(h.smc(2000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[deskScene]
				c.Name = "Desk2"
				m.Scenes[deskScene] = c
			})), "")
			h.wantStack("A,D")
			if h.m.st.SceneEpoch != e || h.m.resolved().Next[camera] != "" {
				t.Fatalf("epoch %d→%d next=%v", e, h.m.st.SceneEpoch, h.m.resolved().Next)
			}
		}},
		{"FB-MP-10", func(t *testing.T) {
			// D、auto/[A]、m 不良/s OK、S 消失: この入口で C。旧期待の作用は無効。
			h := newHB(t, fbAD)
			h.setStates(DOWN, OK)
			h.setAuto(deskScene, camera)
			h.m.st.Expectations = append(h.m.st.Expectations, Expectation{ReqID: 1, Scene: standby, Purpose: PurposeAutoFail, ReturnScene: deskScene,
				ConfigEpoch: h.m.st.ConfigEpoch, Override: OverrideNone{}, Plan: ArrivalPlan{Plan: Plan{Kind: PlanEscape, Target: standby, Append: []string{deskScene}}}})
			h.smc(1000, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, standby) }))
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
			if !h.m.st.Expectations[0].Plan.EffectsDisabled {
				t.Fatalf("旧期待の作用が無効でない")
			}
		}},
		{"FB-MP-12", func(t *testing.T) {
			// D 削除の SC(T) が Map より先、後で Map: SC で手動共通解除、Map で架空の到着なし。
			h := newHB(t, fbAD)
			h.setStates(DOWN, OK)
			h.setAuto(deskScene, camera)
			e := h.m.st.SceneEpoch
			h.outP(h.scene(1000, talk), "")
			if h.m.st.SceneEpoch != e+1 {
				t.Fatalf("SC で手動共通解除でない")
			}
			h.outP(h.smc(1100, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, deskScene) })), "")
			h.wantStack("")
			h.wantScene(talk)
		}},
	})
}

func TestFBW03(t *testing.T) {
	// W-03/0c: 戻り先の再起動の対象から、現在の配信（program）に実際に表示されている入力を除く（共有入力も入力 UUID で）。
	h := newHB(t, fbAD)
	h.m.st.Scene = standby
	if got := h.m.restartInputs(camera2); len(got) != 1 || got[0] != inMain {
		t.Fatalf("program が待機: %v", got)
	}
	h.m.st.Scene = camera // A に im が映っている。B（im だけ）へ戻すときの im は再起動しない
	if got := h.m.restartInputs(camera2); len(got) != 0 {
		t.Fatalf("program の A に映る im を再起動している: %v", got)
	}
	// 入れ子の経路: program の A が、B をシーンのソースとして映す場合も数える。
	h.m.st.Scene = talk
	h.m.st.Map.Scenes[talk] = scenemapContainer("Talk",
		scenemap.Item{ID: 1, SourceUUID: camera2, SourceKind: scenemap.SceneSource, Enabled: true})
	if got := h.m.restartInputs(camera2); len(got) != 0 {
		t.Fatalf("入れ子の経路の im を再起動している: %v", got)
	}
	// 実表示でなければ（A の項目が非表示）再起動の対象に残る。
	h.m.st.Scene = camera
	setItem(h.m.st.Map, camera, 1, false)
	if got := h.m.restartInputs(camera2); len(got) != 1 {
		t.Fatalf("非表示の入力は対象のまま: %v", got)
	}
}

func TestFBEnterMissing(t *testing.T) {
	// 地図にない UUID は、classOf が Other を返しても画像として入れず、逃がす・戻るのどちらでも Missing で、戻り先に選ばれない。
	const gone = "uuid-gone"
	h := newHB(t, fbAD)
	for _, dir := range []Direction{DirEscape, DirReturn} {
		f := h.m.enterFacts(gone, dir)
		if f.Exists {
			t.Fatalf("存在しない UUID が Exists: %+v", f)
		}
		if r := CanEnter(f, dir); r.Allowed || r.Reason != ReasonMissing {
			t.Fatalf("dir=%d: %+v", dir, r)
		}
	}
	if got := FindReturn([]string{gone, camera}, h.m.enterFunc()); got.Target != camera {
		t.Fatalf("戻り先に地図にない UUID を選んだ: %+v", got)
	}
	// 逃がす列の途中にある地図にない UUID は飛ばされ、積まれない。
	h2 := newHB(t, hbFB(FallbackRow{Scene: nameOf(camera), Fallback: "Gone"}))
	h2.m.st.Map.Scenes[gone] = scenemapContainer("Gone")
	h2.m.refine()
	delete(h2.m.st.Map.Scenes, gone)
	plan := FindEscape(EscapeInput{Current: camera, Chain: Resolved{Next: map[string]string{camera: gone}, Standby: standby}, Enter: h2.m.enterFunc()})
	if plan.Target != standby || len(plan.Append) != 1 || plan.Append[0] != camera {
		t.Fatalf("地図にない UUID を飛ばして S へ: %+v", plan)
	}
}
