package switcher

import (
	"errors"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 表 0c 4.4 ItemEnabledChanged / ItemRequestDone、4.5 StudioModeChanged、4.8 Tick・Approve・BatchDone。

func TestFBIE(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-IE-01", func(t *testing.T) {
			// S、auto/[A,D]、m OK/s 不良、最後の逆待 x の false 確認: R(A)。K は到着まで保持。
			h := newHB(t, fbAD)
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera, deskScene)
			bs := h.tick(1000)
			h.outP(bs, "Hc(s)[x,y]")
			h.outP(h.sendFalseOut(bs[0], 1100, kx, "R(A)"), "R(A)")
			h.wantStack("A,D")
			if _, shown := h.mapEnabled(kx); shown {
				v, _ := h.mapEnabled(kx)
				if v {
					t.Fatalf("x の false が地図に反映されていない")
				}
			}
		}},
		{"FB-IE-02", func(t *testing.T) {
			// S、auto/[A,D]、m 不良/s OK、最後の OK の記録の true 確認: R(D)。Show 確認を経てから復帰。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			bs := h.cams(8000, DOWN, OK)
			if len(bs) != 1 || bs[0].Purpose != PurposeShowCamera {
				t.Fatalf("Sc(s) でない: %s", pdescs(bs))
			}
			out := h.showAll(bs[0], 8100)
			h.outP(out, "R(D)")
			h.wantStack("A,D")
		}},
		{"FB-IE-03", func(t *testing.T) {
			// 同上、true の応答だけ、地図は false のまま: OK の記録・表示0が残る間は R なし。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			bs := h.cams(8000, DOWN, OK)
			sc := bs[0]
			if r := h.m.Approve(sc.BatchID, sc.Actions[0], OverrideNone{}, at(8100)); r == nil {
				t.Fatalf("Restart")
			}
			p := h.proceed(sc, 8200, kx, true)
			h.outP(h.itemDone(8200, p.ItemReqID, ItemOK), "")
			p = h.proceed(sc, 8200, ky, true)
			h.outP(h.itemDone(8200, p.ItemReqID, ItemOK), "")
		}},
		{"FB-IE-04", func(t *testing.T) {
			// 応答前の false イベントで逆待が解消、Hc 未完了: R(A)。項目の Pending 自体はシーン復帰の抑止ではない。
			h := newHB(t, fbAD)
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera, deskScene)
			bs := h.tick(1000)
			h.outP(bs, "Hc(s)[x,y]")
			h.proceed(bs[0], 1100, kx, false)
			h.outP(h.itemEv(1150, kx, false), "R(A)")
			h.wantPendingHas(bs[0].BatchID)
		}},
		{"FB-IE-05", func(t *testing.T) {
			// S auto/[A]、m OK/s 不良、x/y=true、記録なし、不一致 true: u(s)=true で逆待の対象外、新規 Hide なし。
			h := newHB(t, fbAD)
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera)
			h.m.st.UserShown[camSub] = true
			h.outP(h.tick(1000), "R(A)")
		}},
		{"FB-IR-01", func(t *testing.T) {
			// ItemOK で待ちが最後に解消: 記録/実表示の更新を確認してから R(A)。
			h := newHB(t, fbAD)
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera)
			bs := h.tick(1000)
			h.outP(bs, "Hc(s)[x,y]")
			p := h.proceed(bs[0], 1100, kx, false)
			h.outP(h.itemDone(1100, p.ItemReqID, ItemOK), "R(A)")
		}},
		{"FB-IR-02", func(t *testing.T) {
			// ItemError/NotFound: 待ちは解消しない。履歴不変。
			for _, res := range []ItemResult{ItemError, ItemNotFound} {
				h := newHB(t, fbAD)
				h.setStates(OK, DOWN)
				h.setAuto(standby, camera)
				bs := h.tick(1000)
				p := h.proceed(bs[0], 1100, kx, false)
				h.outP(h.itemDone(1100, p.ItemReqID, res), "")
				h.wantStack("A")
			}
		}},
		{"FB-IR-03", func(t *testing.T) {
			// 切断後の Item 結果: シーンの生成なし（台帳で0b処理）。
			h := newHB(t, fbAD)
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera)
			bs := h.tick(1000)
			p := h.proceed(bs[0], 1100, kx, false)
			h.ev(OBSDisconnected{At: at(1200)})
			h.outP(h.itemDone(1300, p.ItemReqID, ItemOK), "")
			h.wantStack("A")
		}},
	})
}

func (h *hb) wantPendingHas(id int) {
	h.t.Helper()
	if _, ok := h.m.st.Pending[id]; !ok {
		h.t.Fatalf("Pending に %d がない", id)
	}
}

func TestFBSM(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-SM-01", func(t *testing.T) {
			// A、none、m OK/s 不良、x/y H、off→on/standby: 有効 standby により A/D 不良。F(S;+A,D) と Sc(s)。到着で auto/[A,D]。
			h := newHB(t, fbAD)
			bs := h.sIs(1000, DOWN)
			h.outP(bs, "Hc(s)[x,y]")
			h.hideAll(bs[0], 1100)
			out := h.ev(StudioModeChanged{At: at(8000), Enabled: true})
			if len(out) != 2 || pdesc(out[0]) != "F(S;+A,D)" || out[1].Purpose != PurposeShowCamera {
				t.Fatalf("出力: %s", pdescs(out))
			}
			h.settle(out[0], 8100)
			h.wantStack("A,D")
		}},
		{"FB-SM-02", func(t *testing.T) {
			// m/s=hide、A は m のみ、D は s のみ、S auto/[A,D]、両方 DOWN・H・表示0、on/ignore: 健康 OK でも表示0で不可。
			h := newHB(t, fbAD, hbBothHide(), hbMapEdit(fb11Map), hbStudioHide(StudioHideIgnore))
			h.setStates(DOWN, DOWN)
			h.setAuto(standby, camera, deskScene)
			h.seedBody(kzA, camMain, inMain)
			h.seedBody(ky, camSub, inSub)
			h.outP(h.ev(StudioModeChanged{At: at(1000), Enabled: true}), "")
			h.wantStack("A,D")
			bs := h.cams(8000, DOWN, OK)
			if len(bs) != 1 || pdesc(bs[0]) != "Sc(s)[R(is),y]" {
				t.Fatalf("s OK の後: %s", pdescs(bs))
			}
			out := h.showAll(bs[0], 8100)
			h.outP(out, "R(D)")
		}},
		{"FB-SM-03", func(t *testing.T) {
			// S、auto/[A]、on/hide、s 不良表示、x/y=true、Hc の送出時刻なし: Hc。false 確認後 R。off と同じ逆待。
			h := newHB(t, fbAD, hbStudio(), hbStudioHide(StudioHideHide))
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera)
			bs := h.tick(1000)
			h.outP(bs, "Hc(s)[x,y]")
			h.sendFalseOut(bs[0], 1100, kx, "R(A)")
		}},
	})
}

var kzA = scenemap.ItemKey{ContainerUUID: camera, ItemID: 1} // FB-SM-02・FB-SQ-11 の z=(A,1)

// fb11Map は FB-SM-02・FB-SQ-11 の fixture: A は m のみ（項目 z=(A,1)）、D は s のみ（項目 y=(D,1)）。
func fb11Map(m *scenemapMap) {
	c := m.Scenes[camera]
	c.Items = []scenemap.Item{scenemapItem(1, inMain, true)}
	m.Scenes[camera] = c
}

// seedBody は本体が隠した項目の記録（H）を直接入れ、地図でも false にする。
func (h *hb) seedBody(k scenemap.ItemKey, cam, src string) {
	h.t.Helper()
	h.setMap(k, false) // 判断を走らせずに地図へ書く（記録を足す前に監視対象でなくならないように）
	e := h.entryOf(k, HiddenH)
	e.CameraID, e.SourceUUID = cam, src
	h.m.st.Hidden = append(h.m.st.Hidden, e)
	h.m.sortHidden()
}

func TestFBTK(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-TK-01/02", func(t *testing.T) {
			// 同じ F 候補、送出 t=0、失敗済み、t=4.999: 時刻保持、履歴未確定。t=5 ちょうどは出す。
			h := newHB(t, fbAD)
			bs := h.cams(0, DOWN, OK)
			p := h.approveSet(bs[0], 0)
			h.failBatch(bs[0], p.ReqID, 100)
			h.outP(h.tick(4999), "")
			h.wantStack("")
			h.outP(h.tick(5000), "F(D;+A)")
		}},
		{"FB-TK-03/BD-06", func(t *testing.T) {
			// A、m/s 不良、t=0 F(D) 失敗後 t=1: 別の候補 F(S;+A,D) は即時。
			h := newHB(t, fbAD)
			bs := h.cams(0, DOWN, OK)
			p := h.approveSet(bs[0], 0)
			h.failBatch(bs[0], p.ReqID, 100)
			out := h.cams(1000, DOWN, DOWN)
			if len(out) == 0 || pdesc(out[0]) != "F(S;+A,D)" {
				t.Fatalf("出力: %s", pdescs(out))
			}
		}},
		{"FB-TK-04", func(t *testing.T) {
			// 候補 D→S→D と戻る: D の前回から5秒未満は、候補別の時刻が残り迂回して再送しない。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			p := h.approveSet(bs[0], 1000)
			h.failBatch(bs[0], p.ReqID, 1100)
			out := h.cams(1500, DOWN, DOWN)
			p = h.approveSet(out[0], 1500)
			h.failBatch(out[0], p.ReqID, 1600)
			h.outP(h.cams(2000, DOWN, OK), "")
			h.outP(h.tick(5999), "")
			h.outP(h.tick(6000), "F(D;+A)")
		}},
		{"FB-TK-05", func(t *testing.T) {
			// 新候補でも、同じ目的の Pending（未完了）か同じ宛先の期待がある間は出さない。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			// 新候補 F(S;+A,D) でも、F の Pending がある間は出さない（出るのは項目の Hc だけ）。
			h.outP(h.cams(1500, DOWN, DOWN), "Hc(s)[x,y]")
			h.wantPendingHas(bs[0].BatchID)
			if r := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: deskScene}, OverrideNone{}, at(1600)); r != (Reject{}) {
				t.Fatalf("Approve = %#v", r)
			}
			// 未完了が解消した BatchDone で、新しい候補を判断する。
			out := h.bdone(1700, bs[0].BatchID, BatchRejected, nil)
			if len(out) == 0 || pdesc(out[0]) != "F(S;+A,D)" {
				t.Fatalf("BatchDone のあと: %s", pdescs(out))
			}
		}},
		{"FB-TK-06", func(t *testing.T) {
			// F(D) の期待、期限ちょうど、送出から5秒以上: 出さない（期待を保持）。期限超過の Tick は F(D;+A)。履歴不変。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			p := h.approveSet(bs[0], 1100)
			h.completeBatch(bs[0], p.ReqID, 3000) // 応答 t=3 → 期限 3000+300+3000
			dl := h.m.st.Expectations[0].Deadline
			h.outP(h.ev(Tick{At: dl}), "")
			h.wantStack("")
			h.outP(h.ev(Tick{At: dl.Add(1)}), "F(D;+A)")
			h.wantStack("")
		}},
		{"FB-TK-07", func(t *testing.T) {
			// D auto/[A]、R(A) の期待あり、m/s DOWN: 別の行き先 S でも F を出さない。SC(A) のあと F(S;+A,D)、到着で auto/[A,D]。
			h := newHB(t, fbAD)
			h.setStates(OK, OK)
			h.setAuto(deskScene, camera)
			bs := h.tick(1000)
			h.outP(bs, "R(A)")
			p := h.approveSet(bs[0], 1100)
			h.completeBatch(bs[0], p.ReqID, 1200)
			h.outP(h.cams(2000, DOWN, DOWN), "")
			h.outP(h.tick(3000), "")
			out := h.scene(4000, camera)
			if len(out) == 0 || pdesc(out[0]) != "F(S;+A,D)" {
				t.Fatalf("SC(A) のあと: %s", pdescs(out))
			}
			h.settle(out[0], 4100)
			h.wantStack("A,D")
		}},
		{"FB-TK-07/pending", func(t *testing.T) {
			// R(A) が未完了のまま D が悪化: F を出さない。R は承認で却下（A が不可）、BatchDone Rejected で F。
			h := newHB(t, fbAD)
			h.setStates(OK, OK)
			h.setAuto(deskScene, camera)
			bs := h.tick(1000)
			h.outP(bs, "R(A)")
			h.outP(h.cams(2000, DOWN, DOWN), "")
			if r := h.m.Approve(bs[0].BatchID, bs[0].Actions[len(bs[0].Actions)-1], OverrideNone{}, at(2100)); r != (Reject{}) {
				t.Fatalf("Approve = %#v", r)
			}
			out := h.bdone(2200, bs[0].BatchID, BatchRejected, nil)
			if len(out) == 0 || pdesc(out[0]) != "F(S;+D)" {
				t.Fatalf("BD Rejected のあと: %s", pdescs(out))
			}
		}},
	})
}

func TestFBAP(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-AP-01", func(t *testing.T) {
			// F(D;+A)、最新の探索結果も同じ: Proceed。SetScene の期待に計画のコピー、履歴不変。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			h.approveSet(bs[0], 1100)
			x := h.m.st.Expectations[0]
			if x.Plan.Plan.Kind != PlanEscape || letters(x.Plan.Plan.Append) != "A" || x.Scene != deskScene {
				t.Fatalf("期待: %+v", x)
			}
			bs[0].Plan.Plan.Append[0] = "tampered" // バッチ側の後変更は期待に届かない
			if letters(h.m.st.Expectations[0].Plan.Plan.Append) != "A" {
				t.Fatalf("期待がバッチと配列を共有している")
			}
			h.wantStack("")
		}},
		{"FB-AP-02", func(t *testing.T) {
			// D 悪化で最新の候補が S へ変化: Reject。バッチを書き換えず、期待なし。BatchDone のあと再判断。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			h.cams(1100, DOWN, DOWN)
			if r := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: deskScene}, OverrideNone{}, at(1200)); r != (Reject{}) {
				t.Fatalf("Approve = %#v", r)
			}
			h.wantNoExpectations()
			if pb := h.m.st.Pending[bs[0].BatchID]; pb.Plan.Plan.Target != deskScene {
				t.Fatalf("バッチを書き換えた")
			}
			out := h.bdone(1300, bs[0].BatchID, BatchRejected, nil)
			if len(out) == 0 || pdesc(out[0]) != "F(S;+A,D)" {
				t.Fatalf("再判断: %s", pdescs(out))
			}
		}},
		{"FB-AP-03", func(t *testing.T) {
			// 行き先が同じでも追加列が変わる: Reject（UUID 一致だけで承認しない）。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			pb := h.m.st.Pending[bs[0].BatchID]
			pb.Plan.Plan.Append = []string{camera, camera2}
			h.m.st.Pending[bs[0].BatchID] = pb
			if r := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: deskScene}, OverrideNone{}, at(1100)); r != (Reject{}) {
				t.Fatalf("追加列の違い: %#v", r)
			}
			pb.Plan.Plan = Plan{Kind: PlanReturn, Target: deskScene}
			h.m.st.Pending[bs[0].BatchID] = pb
			if r := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: deskScene}, OverrideNone{}, at(1100)); r != (Reject{}) {
				t.Fatalf("方向の違い: %#v", r)
			}
		}},
		{"FB-AP-04", func(t *testing.T) {
			// R 先が健康 OK でも、逆待が発生: Reject。Restart 後の SetScene 直前にも同じ検査。
			h := newHB(t, fbAD)
			h.setStates(OK, OK)
			h.setAuto(standby, camera)
			bs := h.tick(1000)
			h.outP(bs, "R(A)")
			if r := h.m.Approve(bs[0].BatchID, bs[0].Actions[0], OverrideNone{}, at(1100)); !isProceed(r) {
				t.Fatalf("Restart = %#v", r)
			}
			h.setStates(OK, DOWN) // 再起動の待ちの間に s が悪化し、表示中
			if r := h.m.Approve(bs[0].BatchID, bs[0].Actions[1], OverrideNone{}, at(2100)); r != (Reject{}) {
				t.Fatalf("SetScene = %#v", r)
			}
			h.wantNoExpectations()
		}},
		{"FB-AP-05", func(t *testing.T) {
			// Conn/Stream/Scene/ConfigEpoch のいずれかが古い: Reject。期待を増やさない。
			for name, f := range map[string]func(m *Machine){
				"conn":   func(m *Machine) { m.st.ConnEpoch++ },
				"stream": func(m *Machine) { m.st.StreamEpoch++ },
				"scene":  func(m *Machine) { m.st.SceneEpoch++ },
				"config": func(m *Machine) { m.st.ConfigEpoch++ },
			} {
				h := newHB(t, fbAD)
				bs := h.cams(1000, DOWN, OK)
				f(h.m)
				if r := h.m.Approve(bs[0].BatchID, bs[0].Actions[0], OverrideNone{}, at(1100)); r != (Reject{}) {
					t.Fatalf("%s: %#v", name, r)
				}
				h.wantNoExpectations()
			}
		}},
		{"FB-AP-06", func(t *testing.T) {
			// Auto の承認、auto=false / 非配信 / X: Reject。履歴不変、時刻は失敗だけで消さない。
			for name, f := range map[string]func(m *Machine){
				"auto":   func(m *Machine) { m.st.AutoEnabled = false },
				"stream": func(m *Machine) { m.st.Streaming = false },
				"X":      func(m *Machine) { m.st.Pending[99] = PendingBatch{BatchID: 99, Purpose: PurposeStartScene} },
			} {
				h := newHB(t, fbAD)
				bs := h.cams(1000, DOWN, OK)
				f(h.m)
				if r := h.m.Approve(bs[0].BatchID, bs[0].Actions[0], OverrideNone{}, at(1100)); r != (Reject{}) {
					t.Fatalf("%s: %#v", name, r)
				}
				if len(h.m.st.LastCandidateSent) != 1 {
					t.Fatalf("%s: 時刻が消えた", name)
				}
				h.wantStack("")
			}
		}},
		{"FB-AP-09", func(t *testing.T) {
			// SetScene の対象が現在の UUID: Skip。期待なし、到着とみなさない。
			h := newHB(t, fbAD)
			h.setStates(OK, OK)
			h.setAuto(standby, camera)
			bs := h.tick(1000)
			h.m.st.Scene = camera
			if r := h.m.Approve(bs[0].BatchID, bs[0].Actions[1], OverrideNone{}, at(1100)); r != (Skip{}) {
				t.Fatalf("Approve = %#v", r)
			}
			h.wantNoExpectations()
		}},
	})
}

func TestFBBD(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-BD-01", func(t *testing.T) {
			// F(D) Completed、未着: Pending だけ外す。期待は残り、履歴は未確定。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			p := h.approveSet(bs[0], 1100)
			h.outP(h.completeBatch(bs[0], p.ReqID, 1200), "")
			if len(h.m.st.Pending) != 0 || len(h.m.st.Expectations) != 1 {
				t.Fatalf("pending=%d exps=%d", len(h.m.st.Pending), len(h.m.st.Expectations))
			}
			h.wantStack("")
		}},
		{"FB-BD-02", func(t *testing.T) {
			// t=0 F(D) の送出後 Failed/Rejected、同じ候補、t=1: 同一キー5秒制限。履歴不変。Auto の却下を手動のエラーにしない。
			for _, res := range []BatchResult{BatchFailed, BatchRejected} {
				h := newHB(t, fbAD)
				bs := h.cams(0, DOWN, OK)
				out := h.bdone(100, bs[0].BatchID, res, errors.New("x"))
				h.outP(out, "")
				h.outP(h.tick(1000), "")
				h.wantStack("")
				if res == BatchRejected && h.m.st.LastError != nil {
					t.Fatalf("Auto の Reject を LastError に書いた: %+v", h.m.st.LastError)
				}
			}
		}},
		{"FB-BD-03", func(t *testing.T) {
			// S manual/A、OFF(A) Failed/Rejected: manual/A 保持、LastError で押し直しを案内。
			for _, res := range []BatchResult{BatchFailed, BatchRejected} {
				h := newHB(t, fbAD)
				h.standbyOnManual(t)
				bs, _ := h.evErr(Command{At: at(1000), Kind: CmdStandbyOff})
				h.bdone(1100, bs[0].BatchID, res, errors.New("x"))
				h.wantOwner(OwnerManual, camera)
				if h.m.st.LastError == nil {
					t.Fatalf("LastError がない")
				}
			}
		}},
		{"FB-BD-04", func(t *testing.T) {
			// S auto/[A]、m OK/s 不良、x/y=true、Hide の失敗が f=3 へ: 逆待の対象外、R(A)。
			h := newHB(t, fbAD)
			h.setStates(OK, DOWN)
			h.setAuto(standby, camera)
			h.m.st.ItemFailures[camSub] = 2
			bs := h.tick(1000)
			h.outP(bs, "Hc(s)[x,y]")
			p := h.proceed(bs[0], 1100, kx, false)
			h.itemDone(1100, p.ItemReqID, ItemError)
			out := h.bdone(1200, bs[0].BatchID, BatchCompleted, nil)
			h.outP(out, "R(A)")
			if !h.m.unusable(camSub) {
				t.Fatalf("使えないになっていない")
			}
		}},
		{"FB-BD-05", func(t *testing.T) {
			// StartScene の未完了が解消、期待もなし、A 不良: F(D;+A)。StartScene は再送しない。
			h := newHB(t, fbAD, hbStart(camera), hbNoConn())
			h.connect(0, false, talk, false)
			h.setStates(DOWN, OK)
			bs := h.ev(StreamStateChanged{At: at(1000), Streaming: true})
			if len(bs) == 0 || bs[0].Purpose != PurposeStartScene {
				t.Fatalf("StartScene: %s", pdescs(bs))
			}
			p := h.approveSet(bs[0], 1100)
			h.completeBatch(bs[0], p.ReqID, 1200)
			h.scene(1300, camera)
			out := h.bdone(1400, bs[0].BatchID, BatchCompleted, nil)
			_ = out
		}},
	})
}
