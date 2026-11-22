package switcher

import (
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 表 0c 4.5（接続・配信）FB-OD・FB-OC・FB-SS、4.2 の FB-SC-11、4.8 の FB-TK-08・FB-AP-07/08/10〜12。T4。

// withRecover は「S、auto/[A,D]、R(A) の期待が残る」を作る。
func (h *hb) withRecoverExp() Batch {
	h.t.Helper()
	h.toSAD(1000)
	r := h.cams(8000, OK, OK)
	sc := h.showAll(r[0], 8100)
	h.outP(sc, "R(A)")
	h.approveSet(sc[0], 8800)
	if len(h.m.st.Expectations) != 1 {
		h.t.Fatalf("前提の期待: %+v", h.m.st.Expectations)
	}
	return sc[0]
}

func TestFBODOC(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-OD-01", func(t *testing.T) {
			// auto/[A,D] → OBSDisconnected: ConnEpoch+1、シーン期待消去、履歴は比較用保持。
			h := newHB(t, fbAD)
			h.withRecoverExp()
			ce := h.m.st.ConnEpoch
			h.outP(h.ev(OBSDisconnected{At: at(9000)}), "")
			if h.m.st.ConnEpoch != ce+1 {
				t.Fatalf("ConnEpoch %d→%d", ce, h.m.st.ConnEpoch)
			}
			h.wantNoExpectations()
			h.wantStack("A,D")
			h.wantOwner(OwnerAuto, camera)
		}},
		{"FB-OC-01", func(t *testing.T) {
			// 同一 4 条件、S で再接続、D 可/A 不可 → R(D;[A])。新地図で整理してから判断。StartScene なし。
			h := newHB(t, fbAD, hbStart(camera))
			h.setAuto(standby, camera, deskScene)
			h.setStates(DOWN, OK)
			h.ev(OBSDisconnected{At: at(2000)})
			bs := h.connect(2100, true, standby, false)
			h.outP(bs, "R(D)")
			for _, b := range bs {
				if b.Purpose == PurposeStartScene {
					t.Fatalf("StartScene が出た")
				}
			}
			h.wantStack("A,D") // 到着までは保持
		}},
		{"FB-OC-02", func(t *testing.T) {
			// collection 不一致、現在 D、m 不良/s OK → C。旧バッチは ConnEpoch で Reject。
			h := newHB(t, fbAD)
			f := h.cams(1000, DOWN, OK)
			h.outP(f, "F(D;+A)")
			h.ev(OBSDisconnected{At: at(2000)})
			h.mp.Collection = "collection-y"
			h.outP(h.connect(2100, true, deskScene, false), "")
			h.wantOwner(OwnerNone, "")
			h.wantStack("")
			if res := h.m.Approve(f[0].BatchID, SetScene{SceneUUID: deskScene}, OverrideNone{}, at(2200)); res != (Reject{}) {
				t.Fatalf("旧バッチ = %#v, want Reject", res)
			}
			// 接続先（collection 以外）の変化と同様、現在 UUID の不一致も C。
		}},
		{"FB-OC-03", func(t *testing.T) {
			// 現在 UUID / 配信状態の不一致、現在 T → C。配信差なら StreamEpoch+1。StartScene なし。
			h := newHB(t, fbAD, hbStart(camera))
			h.toSAD(1000)
			h.ev(OBSDisconnected{At: at(2000)})
			se := h.m.st.StreamEpoch
			h.outP(h.connect(2100, true, talk, false), "")
			h.wantOwner(OwnerNone, "")
			h.wantStack("")
			if h.m.st.StreamEpoch != se {
				t.Fatalf("配信差がないのに StreamEpoch が進んだ")
			}
			h2 := newHB(t, fbAD, hbStart(camera))
			h2.toSAD(1000)
			h2.ev(OBSDisconnected{At: at(2000)})
			se = h2.m.st.StreamEpoch
			bs := h2.connect(2100, false, talk, false)
			for _, b := range bs {
				if b.Purpose == PurposeStartScene {
					t.Fatalf("StartScene が出た")
				}
			}
			h2.wantOwner(OwnerNone, "")
			h2.wantStack("")
			if h2.m.st.StreamEpoch != se+1 {
				t.Fatalf("StreamEpoch %d→%d", se, h2.m.st.StreamEpoch)
			}
		}},
		{"FB-OC-04", func(t *testing.T) {
			// 同一 4 条件で S 再接続、新地図で最下段 A が削除 → 手動共通解除、SceneEpoch+1。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.ev(OBSDisconnected{At: at(2000)})
			e := h.m.st.SceneEpoch
			h.mp = editMap(h.mp, func(m *scenemap.Map) { delete(m.Scenes, camera) })
			h.connect(2100, true, standby, false)
			if h.m.st.SceneEpoch != e+1 {
				t.Fatalf("SceneEpoch %d→%d", e, h.m.st.SceneEpoch)
			}
			h.wantOwner(OwnerNone, "")
			h.wantStack("")
			h.wantNoExpectations()
		}},
	})
}

func TestFBSS(t *testing.T) {
	// 配信を止め、A が悪い（m DOWN）まま、開始時の切替先を A にして D から配信を始める。
	prep := func(t *testing.T) (*hb, Batch) {
		h := newHB(t, fbAD, hbStart(camera), hbAt(deskScene))
		h.ev(StreamStateChanged{At: at(1000), Streaming: false})
		h.outP(h.cams(1100, DOWN, OK), "")
		bs := h.ev(StreamStateChanged{At: at(2000), Streaming: true})
		h.out(bs, "St(A)") // A 不良でも StartScene。Auto / Hide は出さない
		return h, bs[0]
	}
	t.Run("FB-SS-01", func(t *testing.T) {
		h, b := prep(t)
		// StartScene 未完了・期待中は Auto / Hide なし。
		h.outP(h.tick(9000), "")
		h.outP(h.cams(9100, DOWN, DOWN), "")
		if !h.m.hasPending(PurposeStartScene) {
			t.Fatalf("StartScene が未完了でない: %+v", b)
		}
	})
	t.Run("FB-SS-02", func(t *testing.T) {
		// StartScene の期待が一致、BatchDone 済み、A 不良 → F(D;+A)。到着後に通常判断、F の到着で初めて [A]。
		h, b := prep(t)
		p := h.approveSet(b, 2100)
		h.ev(SceneRequestDone{At: at(2200), ReqID: p.ReqID, Result: RequestOK})
		h.outP(h.bdone(2300, b.BatchID, BatchCompleted, nil), "") // 期待が残っている間は判断しない
		h.wantStack("")
		out := h.scene(2400, camera)
		h.outP(out, "F(D;+A)")
		h.wantStack("") // F が届くまで履歴なし
		h.settle(out[0], 2500)
		h.wantStack("A")
	})
	t.Run("FB-SS-03", func(t *testing.T) {
		// StartScene の到着が BatchDone より先: Pending による X を保持、BatchDone の解消後に判断。
		h, b := prep(t)
		p := h.approveSet(b, 2100)
		h.ev(SceneRequestDone{At: at(2200), ReqID: p.ReqID, Result: RequestOK})
		h.outP(h.scene(2300, camera), "")
		h.wantStack("")
		h.outP(h.bdone(2400, b.BatchID, BatchCompleted, nil), "F(D;+A)")
	})
	t.Run("FB-SS-04", func(t *testing.T) {
		// 配信停止、auto/[A,D]、全 OK・記録なし → C、期待・時刻消去。
		h := newHB(t, fbAD)
		h.setAuto(standby, camera, deskScene)
		r := h.tick(1000)
		h.outP(r, "R(A)")
		h.approveSet(r[0], 1100)
		h.ev(StreamStateChanged{At: at(2000), Streaming: false})
		h.wantOwner(OwnerNone, "")
		h.wantStack("")
		h.wantNoExpectations()
		if len(h.m.st.LastCandidateSent) != 0 {
			t.Fatalf("候補の時刻が残る: %v", h.m.st.LastCandidateSent)
		}
	})
	t.Run("FB-SS-05", func(t *testing.T) {
		// 非配信、記録なし、全 OK の CamerasChanged / Tick → []、stack 空、自動切替なし。
		h := newHB(t, fbAD)
		h.ev(StreamStateChanged{At: at(1000), Streaming: false})
		h.outP(h.cams(1100, OK, DOWN), "")
		h.outP(h.cams(1200, OK, OK), "")
		h.outP(h.tick(9000), "")
		h.wantStack("")
		h.wantOwner(OwnerNone, "")
	})
}

func TestFBSC11(t *testing.T) {
	t.Run("FB-SC-11", func(t *testing.T) {
		// ON 特例後、作用無効の R(A) の期待 → A、全 OK: SceneEpoch 不変で照合して C、A・none。通知を last_error へ。待機へ送り直さない。
		h := newHB(t, fbAD)
		h.withRecoverExp()
		if _, err := h.cmdOn(8900); err != nil {
			t.Fatal(err)
		}
		e := h.m.st.SceneEpoch
		h.outP(h.scene(9000, camera), "")
		h.wantScene(camera)
		h.wantOwner(OwnerNone, "")
		if h.m.st.SceneEpoch != e {
			t.Fatalf("SceneEpoch %d→%d", e, h.m.st.SceneEpoch)
		}
		le := h.m.Snapshot().LastError
		if le == nil || !strings.HasPrefix(le.Msg, "待機 ON の前に送った戻りの切替が届いたため、"+nameOf(camera)+" に戻りました。") {
			t.Fatalf("last_error = %+v", le)
		}
		h.outP(h.tick(20000), "")
		h.wantScene(camera)
	})
	t.Run("stale_ConfigEpoch_R_has_no_notice", func(t *testing.T) {
		// FB-SC-10: 古い設定世代の R は ON 特例でなければ通知しない。
		h := newHB(t, fbAD)
		h.withRecoverExp()
		c := h.cfg
		c.At = at(8900)
		c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene), fbRow(deskScene, talk)}
		h.ev(c)
		h.outP(h.scene(9000, camera), "")
		h.wantOwner(OwnerNone, "")
		h.wantNoErr()
	})
}

func TestFBTK08(t *testing.T) {
	// manual/A、OFF(A) 未完了または期待あり、m/s DOWN・x/y H → [] F 抑止。OFF 到着後に通常判断。
	noAutoScene := func(t *testing.T, out []Batch) {
		t.Helper()
		for _, b := range out {
			if b.Purpose.IsAuto() {
				t.Fatalf("OFF の未完了・期待中に自動のシーン切替が出た: %s", pdesc(b))
			}
		}
	}
	t.Run("未完了", func(t *testing.T) {
		h := newHB(t, fbAD)
		h.standbyOnManual(t)
		bs := h.mustCmd(h.cmdOff(2000))
		h.out(bs, "M(A)")
		noAutoScene(t, h.cams(2100, DOWN, DOWN)) // Hc(s) などの項目は出てよい。シーンの切替は出さない
		noAutoScene(t, h.tick(9000))
		if !h.m.recoverInFlight() {
			t.Fatalf("OFF の未完了で recoverInFlight が false")
		}
	})
	t.Run("期待あり_到着後に通常判断", func(t *testing.T) {
		h := newHB(t, fbAD)
		h.standbyOnManual(t)
		bs := h.mustCmd(h.cmdOff(2000))
		p := h.approveSet(bs[0], 2050) // 承認は A が健康なうちに済ませる
		noAutoScene(t, h.cams(2100, DOWN, DOWN))
		noAutoScene(t, h.tick(9000))
		noAutoScene(t, h.bdone(9200, bs[0].BatchID, BatchCompleted, nil)) // 未完了が解けても期待が残る間は出さない
		if !h.m.recoverInFlight() {
			t.Fatalf("OFF の期待で recoverInFlight が false")
		}
		h.ev(SceneRequestDone{At: at(9300), ReqID: p.ReqID, Result: RequestOK})
		out := h.scene(9400, camera) // 到着: none。A 不良なので通常判断で逃がす
		if len(out) == 0 || out[0].Purpose != PurposeAutoFail {
			t.Fatalf("到着後の判断: %s", pdescs(out))
		}
		h.wantOwner(OwnerNone, "")
	})
}

func TestFBAPT4(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-AP-07", func(t *testing.T) {
			// OFF 計画が変化、現在 S が新最善 → Reject。承認で auto 化へ Replace しない。押し直しは専用探索。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			bs := h.mustCmd(h.cmdOff(2000))
			h.out(bs, "M(A)")
			h.setStates(DOWN, DOWN) // A も D も不可、S（今いる）が新最善
			if res := h.m.Approve(bs[0].BatchID, bs[0].Actions[0], OverrideNone{}, at(2100)); res != (Reject{}) {
				t.Fatalf("Approve = %#v, want Reject", res)
			}
			h.wantOwner(OwnerManual, camera)
			h.wantStack("")
			h.bdone(2200, bs[0].BatchID, BatchRejected, nil)
			h.wantErrContains("取りやめた")
			// 押し直し: 切替なしで auto/[A,D]。
			bs2, err := h.cmdOff(3000)
			if err != nil || len(bs2) != 0 {
				t.Fatalf("OFF = %v %v", bs2, err)
			}
			h.wantStack("A,D")
		}},
		{"FB-AP-08", func(t *testing.T) {
			// ON、最下段 / 待機 UUID / 受付条件が変化 → Reject。manual 化しない。LastError は BD で処理。
			h := newHB(t, fbAD)
			h.toDAuto(1000)
			bs := h.mustCmd(h.cmdOn(2000))
			h.m.st.Stack = []string{camera2} // 最下段が変わった（直接）
			h.m.st.ReturnScene = camera2
			if res := h.m.Approve(bs[0].BatchID, bs[0].Actions[0], OverrideNone{}, at(2100)); res != (Reject{}) {
				t.Fatalf("最下段の変化: %#v, want Reject", res)
			}
			h.wantOwner(OwnerAuto, camera2)
			h.wantNoErr()
			h.bdone(2200, bs[0].BatchID, BatchRejected, nil)
			h.wantErrContains("取りやめた")
			// 待機 UUID の変化
			h2 := newHB(t, fbAD)
			h2.toDAuto(1000)
			b2 := h2.mustCmd(h2.cmdOn(2000))
			h2.m.st.StandbyUUID = standby2
			if res := h2.m.Approve(b2[0].BatchID, b2[0].Actions[0], OverrideNone{}, at(2100)); res != (Reject{}) {
				t.Fatalf("待機 UUID の変化: %#v, want Reject", res)
			}
			// 受付条件（owner が manual になった）
			h3 := newHB(t, fbAD)
			h3.toDAuto(1000)
			b3 := h3.mustCmd(h3.cmdOn(2000))
			h3.m.st.StandbyOwner = OwnerManual
			if res := h3.m.Approve(b3[0].BatchID, b3[0].Actions[0], OverrideNone{}, at(2100)); res != (Reject{}) {
				t.Fatalf("受付条件の変化: %#v, want Reject", res)
			}
		}},
	})
}

// restartSubset は A の再起動の対象が 2 入力（m・s ともに standby）の fixture で R(A) の Restart を作る。
func (h *hb) restartBatch2() Batch {
	h.t.Helper()
	h.setAuto(standby, camera)
	r := h.tick(1000)
	h.outP(r, "R(A)")
	rm, ok := r[0].Actions[0].(RestartMedia)
	if !ok || len(rm.Inputs) != 2 {
		h.t.Fatalf("Restart = %+v", r[0].Actions)
	}
	return r[0]
}

func TestFBAPRestart(t *testing.T) {
	// 現在の program（S）に入力を映すため、承認の直前に地図へ項目を足す。
	show := func(h *hb, ins ...string) {
		c := h.m.st.Map.Scenes[standby]
		for i, in := range ins {
			c.Items = append(c.Items, scenemap.Item{ID: 90 + i, SourceUUID: in, SourceKind: scenemap.InputSource, Enabled: true})
		}
		h.m.st.Map.Scenes[standby] = c
	}
	t.Run("FB-AP-10/subset", func(t *testing.T) {
		// 固定 I=[im,is] のうち im が承認時に実表示 → Proceed{RestartInputs=[is]}。新規入力を追加しない（C-03）。
		h := newHB(t, fbAD, hbSubStandby())
		b := h.restartBatch2()
		show(h, inMain)
		res := h.m.Approve(b.BatchID, b.Actions[0], OverrideNone{}, at(1100))
		p, ok := res.(Proceed)
		if !ok || len(p.RestartInputs) != 1 || p.RestartInputs[0] != inSub {
			t.Fatalf("Approve = %#v, want Proceed{[is]}", res)
		}
	})
	t.Run("FB-AP-10/empty_is_skip", func(t *testing.T) {
		// 全部が実表示なら Skip（再起動も待ちもしない）。SetScene へ進む。
		h := newHB(t, fbAD, hbSubStandby())
		b := h.restartBatch2()
		show(h, inMain, inSub)
		if res := h.m.Approve(b.BatchID, b.Actions[0], OverrideNone{}, at(1100)); res != (Skip{}) {
			t.Fatalf("Approve = %#v, want Skip", res)
		}
		if res := h.m.Approve(b.BatchID, b.Actions[1], OverrideNone{}, at(1200)); !isProceed(res) {
			t.Fatalf("SetScene = %#v, want Proceed", res)
		}
	})
	t.Run("FB-AP-11", func(t *testing.T) {
		// 固定 Restart 入力が今非表示、共通/目的条件成立 → Proceed。設定順→UUID 順で全入力。
		h := newHB(t, fbAD, hbSubStandby())
		b := h.restartBatch2()
		res := h.m.Approve(b.BatchID, b.Actions[0], OverrideNone{}, at(1100))
		p, ok := res.(Proceed)
		if !ok || strings.Join(p.RestartInputs, ",") != inMain+","+inSub {
			t.Fatalf("Approve = %#v, want Proceed{[im,is]}", res)
		}
	})
	t.Run("FB-AP-12/OFF_to_origin_excludes_shown", func(t *testing.T) {
		// 戻り先自身へ戻る OFF は R と同じ除外（生成時に実表示の入力を外す）。逃がす方向（FB-OFF-02）は SetScene のみ。
		h := newHB(t, fbAD, hbSubStandby())
		h.standbyOnManual(t)
		show(h, inMain)
		bs := h.mustCmd(h.cmdOff(2000))
		h.wantRestartActions(bs[0], restartOf(inSub), SetScene{SceneUUID: camera})
		// 承認時にさらに is も実表示になれば Skip。
		show(h, inSub)
		if res := h.m.Approve(bs[0].BatchID, bs[0].Actions[0], OverrideNone{}, at(2100)); res != (Skip{}) {
			t.Fatalf("Approve = %#v, want Skip", res)
		}
	})
	t.Run("FB-AP-12/F_has_no_restart", func(t *testing.T) {
		h := newHB(t, fbAD)
		f := h.cams(1000, DOWN, OK)
		h.outP(f, "F(D;+A)")
		h.wantRestartActions(f[0], SetScene{SceneUUID: deskScene})
	})
}

func TestFBSC11StaleConfig(t *testing.T) {
	// FB-SC-11 別列: R(A) の SetScene 承認のあと、ON より前に設定が変わって期待が古い世代になっても、ON 特例のあとの到着で通知する。
	h := newHB(t, fbAD)
	h.withRecoverExp()
	c := h.cfg
	c.At = at(8850)
	c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene), fbRow(deskScene, talk)}
	h.ev(c)
	if _, err := h.cmdOn(8900); err != nil {
		t.Fatal(err)
	}
	h.wantOwner(OwnerManual, camera)
	h.scene(9000, camera)
	h.wantScene(camera)
	h.wantOwner(OwnerNone, "")
	le := h.m.Snapshot().LastError
	if le == nil || !strings.HasPrefix(le.Msg, "待機 ON の前に送った戻りの切替が届いたため、") {
		t.Fatalf("last_error = %+v", le)
	}
}
