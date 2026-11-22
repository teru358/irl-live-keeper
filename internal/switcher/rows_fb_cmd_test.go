package switcher

import (
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 表 0c 4.7（Command）FB-ON・FB-OFF・FB-MA。T4: 手動の待機の ON / OFF。

// cmdOn / cmdOff は Command を流して (出す列, cmdErr) を返す。
func (h *hb) cmdOn(ms int) ([]Batch, error) { return h.evErr(Command{At: at(ms), Kind: CmdStandbyOn}) }
func (h *hb) cmdOff(ms int) ([]Batch, error) {
	return h.evErr(Command{At: at(ms), Kind: CmdStandbyOff})
}

// mustCmd は cmdErr がないことを確かめて列を返す。
func (h *hb) mustCmd(bs []Batch, err error) []Batch {
	h.t.Helper()
	if err != nil {
		h.t.Fatalf("cmdErr = %v", err)
	}
	return bs
}

// wantInvalid は ErrInvalidState（理由つき）で、列が空であることを確かめる。
func (h *hb) wantInvalid(bs []Batch, err error, reason string) {
	h.t.Helper()
	if len(bs) != 0 {
		h.t.Fatalf("列が出た: %s", pdescs(bs))
	}
	h.wantReason(err, reason)
}

// toDAuto は A で m が DOWN（s OK）→ F(D;+A) → D、auto/[A]。
func (h *hb) toDAuto(ms int) {
	h.t.Helper()
	f := h.cams(ms, DOWN, OK)
	h.outP(f, "F(D;+A)")
	h.settle(f[0], ms+100)
	h.wantScene(deskScene)
	h.wantStack("A")
}

func TestFBON(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-ON-01", func(t *testing.T) {
			// D、auto/[A] → ON: ON(A)。到着までは auto/[A]、到着で manual/A、K=[]。
			h := newHB(t, fbAD)
			h.toDAuto(1000)
			bs := h.mustCmd(h.cmdOn(2000))
			if len(bs) != 1 || bs[0].Purpose != PurposeStandbyOn || bs[0].ReturnScene != camera ||
				len(bs[0].Actions) != 1 || bs[0].Actions[0] != (SetScene{SceneUUID: standby}) {
				t.Fatalf("ON(A) でない: %+v", bs)
			}
			h.wantOwner(OwnerAuto, camera)
			h.wantStack("A")
			h.outP(h.settle(bs[0], 2100), "")
			h.wantScene(standby)
			h.wantOwner(OwnerManual, camera)
			h.wantStack("")
		}},
		{"FB-ON-02", func(t *testing.T) {
			// T、auto/[A,D] → ON: 最下段の A を保存。T を戻り先にしない。
			h := newHB(t, fbAD)
			h.setAuto(talk, camera, deskScene)
			bs := h.mustCmd(h.cmdOn(2000))
			if len(bs) != 1 || bs[0].Purpose != PurposeStandbyOn || bs[0].ReturnScene != camera {
				t.Fatalf("ON(A) でない: %+v", bs)
			}
			h.settle(bs[0], 2100)
			h.wantOwner(OwnerManual, camera)
			h.wantStack("")
		}},
		{"FB-ON-03", func(t *testing.T) {
			// T、none → ON(T): 到着で manual/T、K=[]。
			h := newHB(t, fbAD)
			h.scene(1000, talk)
			bs := h.mustCmd(h.cmdOn(2000))
			if len(bs) != 1 || bs[0].ReturnScene != talk {
				t.Fatalf("ON(T) でない: %+v", bs)
			}
			h.settle(bs[0], 2100)
			h.wantOwner(OwnerManual, talk)
			h.wantStack("")
		}},
		{"FB-ON-04", func(t *testing.T) {
			// S、auto/[A,D]、期待あり → ON: Handle の中で manual/A、K=[]。自動期待の作用を無効化。Approve は呼ばない。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			r := h.cams(8000, OK, OK)
			sc := h.showAll(r[0], 8100)
			h.outP(sc, "R(A)")
			h.approveSet(sc[0], 8800) // R(A) の SetScene を承認: 期待が残る
			if len(h.m.st.Expectations) != 1 {
				t.Fatalf("前提の期待: %+v", h.m.st.Expectations)
			}
			e := h.m.st.SceneEpoch
			bs, err := h.cmdOn(8900)
			if err != nil || len(bs) != 0 {
				t.Fatalf("ON = %v %v, want [] / nil", bs, err)
			}
			h.wantOwner(OwnerManual, camera)
			h.wantStack("")
			if h.m.st.SceneEpoch != e {
				t.Fatalf("SceneEpoch が進んだ")
			}
			if len(h.m.st.Expectations) != 1 || !h.m.st.Expectations[0].Plan.EffectsDisabled {
				t.Fatalf("期待の作用を無効にしていない: %+v", h.m.st.Expectations)
			}
			h.wantNoErr()
		}},
		{"FB-ON-05", func(t *testing.T) {
			// D、auto/[A]、切替の期待・未完了あり → ON: S 上の auto 以外は従来の拒否（ErrInvalidState）。
			h := newHB(t, fbAD)
			h.toDAuto(1000)
			f := h.cams(2000, DOWN, DOWN)
			h.outP(f, "F(S;+D)")
			bs, err := h.cmdOn(2100) // 未完了
			h.wantInvalid(bs, err, ReasonSwitchInProcess)
			h.approveSet(f[0], 2200) // 期待だけ残る
			h.ev(BatchDone{At: at(2300), BatchID: f[0].BatchID, Result: BatchCompleted})
			bs, err = h.cmdOn(2400)
			h.wantInvalid(bs, err, ReasonSwitchInProcess)
			h.wantOwner(OwnerAuto, camera)
			h.wantStack("A")
		}},
		{"FB-ON-06", func(t *testing.T) {
			// S、none または manual → ON: ErrInvalidState、変化なし。
			h := newHB(t, fbAD)
			h.scene(1000, standby)
			bs, err := h.cmdOn(2000)
			h.wantInvalid(bs, err, ReasonAlreadyStandby)
			h.wantOwner(OwnerNone, "")
			h2 := newHB(t, fbAD)
			h2.standbyOnManual(t)
			bs, err = h2.cmdOn(2000)
			h2.wantInvalid(bs, err, ReasonAlreadyStandby)
			h2.wantOwner(OwnerManual, camera)
		}},
		{"FB-ON-07", func(t *testing.T) {
			// 未接続 / 非配信 → ON: 順は従来どおり。所有権変更なし。
			h := newHB(t, fbAD)
			h.setAuto(standby, camera)
			h.ev(OBSDisconnected{At: at(1000)})
			if _, err := h.cmdOn(1100); !hbErrIs(err, ErrNotConnected{}) {
				t.Fatalf("err = %v, want ErrNotConnected", err)
			}
			h.wantOwner(OwnerAuto, camera)
			h2 := newHB(t, fbAD)
			h2.setAuto(standby, camera)
			h2.ev(StreamStateChanged{At: at(1000), Streaming: false})
			h2.setAuto(standby, camera)
			bs, err := h2.cmdOn(1100)
			h2.wantInvalid(bs, err, ReasonNotStreaming)
			h2.wantOwner(OwnerAuto, camera)
		}},
		{"FB-ON-08", func(t *testing.T) {
			// S の名前が解決できない → ON: ErrInvalidState、履歴不変。
			h := newHB(t, fbAD, hbNoStandby())
			h.toDAuto(1000)
			bs, err := h.cmdOn(2000)
			h.wantInvalid(bs, err, ReasonStandbyNotFound)
			h.wantOwner(OwnerAuto, camera)
			h.wantStack("A")
		}},
		{"FB-ON-09", func(t *testing.T) {
			// S auto/[A]、R(A) の Restart 待ち（SetScene 未承認）で ON: 200、Handle で manual/A。後の SetScene は Reject、
			// 自動の BD Rejected は last_error に書かない。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			r := h.cams(8000, OK, OK)
			sc := h.showAll(r[0], 8100)
			h.outP(sc, "R(A)")
			if res := h.m.Approve(sc[0].BatchID, sc[0].Actions[0], OverrideNone{}, at(8800)); !isProceed(res) {
				t.Fatalf("Restart = %#v", res)
			}
			bs, err := h.cmdOn(8900)
			if err != nil || len(bs) != 0 {
				t.Fatalf("ON = %v %v, want [] / nil", bs, err)
			}
			h.wantOwner(OwnerManual, camera)
			if res := h.m.Approve(sc[0].BatchID, sc[0].Actions[1], OverrideNone{}, at(9300)); res != (Reject{}) {
				t.Fatalf("SetScene = %#v, want Reject", res)
			}
			h.bdone(9300, sc[0].BatchID, BatchRejected, nil)
			h.wantNoErr()
			h.wantOwner(OwnerManual, camera)
			h.wantNoExpectations()
		}},
	})
}

// onSpecial は「S、auto/[A]、R(A) 送信済み・BatchDone 済み、期待あり」を作る（FB-SQ-08 の t=0）。全 OK、A は健康。
func (h *hb) onSpecial() Batch {
	h.t.Helper()
	h.setAuto(standby, camera)
	r := h.tick(1000)
	h.outP(r, "R(A)")
	h.m.Approve(r[0].BatchID, r[0].Actions[0], OverrideNone{}, at(1100))
	p := h.approveSet(r[0], 1200)
	h.completeBatch(r[0], p.ReqID, 1300)
	if len(h.m.st.Expectations) != 1 {
		h.t.Fatalf("前提の期待: %+v", h.m.st.Expectations)
	}
	return r[0]
}

func TestFBSQ08(t *testing.T) {
	// ON 特例の受付と遅着。
	t.Run("main", func(t *testing.T) {
		h := newHB(t, fbAD)
		h.onSpecial()
		bs, err := h.cmdOn(2000)
		if err != nil || len(bs) != 0 {
			t.Fatalf("ON = %v %v", bs, err)
		}
		h.wantOwner(OwnerManual, camera)
		h.wantStack("")
		e := h.m.st.SceneEpoch
		// 1.1: 残存期待と一致する SC(A): A・none、戻り先なし、K=[]、SceneEpoch 不変、通知。
		h.outP(h.scene(2100, camera), "")
		h.wantScene(camera)
		h.wantOwner(OwnerNone, "")
		h.wantStack("")
		if h.m.st.SceneEpoch != e {
			t.Fatalf("SceneEpoch %d→%d", e, h.m.st.SceneEpoch)
		}
		want := "待機 ON の前に送った戻りの切替が届いたため、" + nameOf(camera) + " に戻りました。待機にするには、もう一度 ON を押してください"
		if le := h.m.Snapshot().LastError; le == nil || le.Msg != want || !le.At.Equal(at(2100)) {
			t.Fatalf("last_error = %+v, want %q", le, want)
		}
		// 2: 全 OK のカメラ入力: A に残る。S へ送り直さない。
		h.outP(h.cams(3000, OK, OK), "")
		h.outP(h.tick(9000), "")
		h.wantScene(camera)
	})
	t.Run("a/restart_wait", func(t *testing.T) {
		// R が Restart 待ち（SetScene 未承認）の ON も 200。後の SetScene は manual で Reject、BD Rejected は last_error に書かない。
		// （FB-ON-09 と同じ形。承認前後の入口の確認）
		h := newHB(t, fbAD)
		h.setAuto(standby, camera)
		r := h.tick(1000)
		h.outP(r, "R(A)")
		h.m.Approve(r[0].BatchID, r[0].Actions[0], OverrideNone{}, at(1100))
		bs, err := h.cmdOn(1200)
		if err != nil || len(bs) != 0 {
			t.Fatalf("ON = %v %v", bs, err)
		}
		h.wantOwner(OwnerManual, camera)
		if res := h.m.Approve(r[0].BatchID, r[0].Actions[1], OverrideNone{}, at(1700)); res != (Reject{}) {
			t.Fatalf("SetScene = %#v, want Reject", res)
		}
		h.bdone(1700, r[0].BatchID, BatchRejected, nil)
		h.wantNoErr()
		h.wantOwner(OwnerManual, camera)
	})
	t.Run("b/disabled_F_to_S", func(t *testing.T) {
		// 作用無効の F(S) の期待に SC(S) が一致しても、[] で manual/A のまま。
		h := newHB(t, fbAD)
		h.toDAuto(1000)
		f := h.cams(2000, DOWN, DOWN)
		h.outP(f, "F(S;+D)")
		p := h.approveSet(f[0], 2100)
		h.completeBatch(f[0], p.ReqID, 2200)
		// D auto で期待あり: ON は拒否（S ではない）。S に着く前に S 上 auto を作るため、状態を直接そろえる。
		h.m.st.Scene = standby
		h.m.setAuto([]string{camera})
		if _, err := h.cmdOn(2300); err != nil {
			t.Fatal(err)
		}
		h.wantOwner(OwnerManual, camera)
		for _, b := range h.scene(2400, standby) {
			if b.Purpose.IsAuto() {
				t.Fatalf("自動のシーン切替が出た: %s", pdesc(b))
			}
		}
		h.wantOwner(OwnerManual, camera)
		h.wantStack("")
		h.wantNoErr()
	})
	t.Run("c/D_or_T_auto", func(t *testing.T) {
		// D/T にいる auto の期待中 ON は拒否、期待なしなら ON(A)。
		h := newHB(t, fbAD)
		h.toDAuto(1000)
		f := h.cams(2000, DOWN, DOWN)
		h.outP(f, "F(S;+D)")
		h.approveSet(f[0], 2100)
		bs, err := h.cmdOn(2200)
		h.wantInvalid(bs, err, ReasonSwitchInProcess)
		h2 := newHB(t, fbAD)
		h2.toDAuto(1000)
		bs, err = h2.cmdOn(2000)
		if err != nil || len(bs) != 1 || bs[0].Purpose != PurposeStandbyOn {
			t.Fatalf("ON = %v %v", bs, err)
		}
	})
}

func TestFBOFF(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-OFF-01", func(t *testing.T) {
			// manual/A、A が戻る方向で可 → OFF(A;[]): 到着で none。ON 前の途中履歴は復活しない。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			bs := h.mustCmd(h.cmdOff(2000))
			h.out(bs, "M(A)")
			h.wantRestartActions(bs[0], restartOf(inMain), SetScene{SceneUUID: camera})
			h.settle(bs[0], 2100)
			h.wantScene(camera)
			h.wantOwner(OwnerNone, "")
			h.wantStack("")
		}},
		{"FB-OFF-02", func(t *testing.T) {
			// manual/A、A 不可、D が逃がす方向で可 → OFF(D;[A]): 到着で auto/[A]。Restart を付けない。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.setStates(DOWN, OK)
			bs := h.mustCmd(h.cmdOff(2000))
			h.out(bs, "M(D)")
			h.wantRestartActions(bs[0], SetScene{SceneUUID: deskScene}) // FB-AP-12: 逃がす方向は SetScene のみ
			if bs[0].Plan.Plan.Kind != PlanEscape || letters(bs[0].Plan.Plan.Append) != "A" {
				t.Fatalf("計画 = %+v", bs[0].Plan)
			}
			h.wantOwner(OwnerManual, camera)
			h.settle(bs[0], 2100)
			h.wantScene(deskScene)
			h.wantStack("A")
			h.wantOwner(OwnerAuto, camera)
		}},
		{"FB-OFF-03", func(t *testing.T) {
			// manual/A、A/D 不可、S が最初の可候補 → 切替なしで auto/[A,D] 確定（200）。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.setStates(DOWN, DOWN)
			bs, err := h.cmdOff(2000)
			if err != nil || len(bs) != 0 {
				t.Fatalf("OFF = %v %v, want [] / nil", bs, err)
			}
			h.wantScene(standby)
			h.wantStack("A,D")
			h.wantOwner(OwnerAuto, camera)
		}},
		{"FB-OFF-04", func(t *testing.T) {
			// manual/A、A 不可、途中 T 可 → OFF(T;[A]): T は終端、D まで探索しない。到着で auto/[A]。
			h := newHB(t, hbFB(fbRow(camera, talk), fbRow(talk, deskScene)))
			h.standbyOnManual(t)
			h.setStates(DOWN, OK)
			bs := h.mustCmd(h.cmdOff(2000))
			h.out(bs, "M(T)")
			h.settle(bs[0], 2100)
			h.wantScene(talk)
			h.wantStack("A")
		}},
		{"FB-OFF-05", func(t *testing.T) {
			// manual/T、T 存在 → StandbyOffOther[SetScene(T)]: 再起動なし、到着で C。
			h := newHB(t, fbAD)
			h.scene(1000, talk)
			on := h.mustCmd(h.cmdOn(1100))
			h.settle(on[0], 1200)
			h.wantOwner(OwnerManual, talk)
			bs := h.mustCmd(h.cmdOff(2000))
			if len(bs) != 1 || bs[0].Purpose != PurposeStandbyOffOther || len(bs[0].Actions) != 1 ||
				bs[0].Actions[0] != (SetScene{SceneUUID: talk}) {
				t.Fatalf("OFF(T) でない: %+v", bs)
			}
			h.settle(bs[0], 2100)
			h.wantScene(talk)
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-OFF-06", func(t *testing.T) {
			// manual/A、auto=false、A 不良・待ちあり → OFF(A;[]): 入場の健康条件を無視、到着で C。
			h := newHB(t, fbAD, hbAutoOff())
			h.standbyOnManual(t)
			h.setStates(DOWN, DOWN)
			h.yOff()
			h.seedH(kx)
			bs := h.mustCmd(h.cmdOff(2000))
			h.out(bs, "M(A)")
			h.settle(bs[0], 2100)
			h.wantScene(camera)
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-OFF-07", func(t *testing.T) {
			// manual/A、先の名前未解決、S も引けない → ErrInvalidState（409）、last_error。manual/A 維持。直接作った状態だけ。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.setStates(DOWN, DOWN)
			delete(h.m.st.Map.Scenes, standby) // 通常の取得済み地図では到達不能（C-05）
			h.m.st.StandbyUUID = ""
			bs, err := h.cmdOff(2000)
			h.wantInvalid(bs, err, ReasonOffNoTarget)
			h.wantErrContains("切替先が OBS にないため、待機のままです")
			h.wantOwner(OwnerManual, camera)
			h.wantStack("")
		}},
		{"FB-OFF-08", func(t *testing.T) {
			// manual/A、A の UUID は存在（OBS で改名）、全 OK → OFF(A;[]): 到着で C。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.smc(1500, editMap(h.mp, func(m *scenemap.Map) {
				c := m.Scenes[camera]
				c.Name = "Renamed"
				m.Scenes[camera] = c
			}))
			h.wantOwner(OwnerManual, camera)
			bs := h.mustCmd(h.cmdOff(2000))
			h.out(bs, "M(A)")
			h.settle(bs[0], 2100)
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-OFF-09", func(t *testing.T) {
			// manual/A、A の UUID が削除済み → ErrInvalidState。通常は地図の更新で全解除済み、直接作った状態でも強制 OFF しない。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			delete(h.m.st.Map.Scenes, camera)
			bs, err := h.cmdOff(2000)
			h.wantInvalid(bs, err, ReasonNotManual)
			h2 := newHB(t, fbAD)
			h2.standbyOnManual(t)
			h2.smc(1500, editMap(h2.mp, func(m *scenemap.Map) { delete(m.Scenes, camera) }))
			h2.wantOwner(OwnerNone, "")
			bs, err = h2.cmdOff(2000)
			h2.wantInvalid(bs, err, ReasonNotManual)
		}},
		{"FB-OFF/ConfigEpoch_stale", func(t *testing.T) {
			// OFF 専用計画の到着前に設定が変わった: 古い計画は部分的に当てず、到着で全解除する。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.setStates(DOWN, OK)
			bs := h.mustCmd(h.cmdOff(2000))
			h.approveSet(bs[0], 2100)
			c := h.cfg
			c.At = at(2150)
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene), fbRow(deskScene, talk)}
			h.ev(c)
			h.scene(2200, deskScene)
			h.wantOwner(OwnerNone, "")
			h.wantStack("")
		}},
	})
}

func TestFBMA(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-MA-01", func(t *testing.T) {
			// auto 履歴・復帰待ち中にすべて表示に戻す: MA。0b の対象・保存規則、stack は受付で変えない。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			bs := h.mustCmd(h.evErr(Command{At: at(2000), Kind: CmdShowAllCameras}))
			h.out(bs, "MA[x,y]")
			h.wantStack("A,D")
			h.wantOwner(OwnerAuto, camera)
		}},
		{"FB-MA-02", func(t *testing.T) {
			// S auto、全 OK、MA の最後の true 確認で OK 記録が解消 → R(A)。MA 自体は履歴を消さない。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			bs := h.mustCmd(h.evErr(Command{At: at(2000), Kind: CmdShowAllCameras}))
			h.out(bs, "MA[x,y]")
			h.outP(h.cams(2100, OK, OK), "") // 記録があるうちは R なし
			h.wantStack("A,D")
			out := h.showAll(bs[0], 2200)
			h.outP(out, "R(A)")
			h.wantStack("A,D")
		}},
	})
}

var _ = strings.Contains

// status（Snapshot）: standby.stack（名前の並び、下→上）と return_scene、引けない scene_fallbacks の行（1.1）。
func TestFBStatus(t *testing.T) {
	t.Run("stack_and_return_scene", func(t *testing.T) {
		h := newHB(t, fbAD)
		h.toSAD(1000)
		st := h.m.Snapshot().Standby
		if strings.Join(st.StackNames, ",") != nameOf(camera)+","+nameOf(deskScene) || st.ReturnScene != camera || st.ReturnSceneName != nameOf(camera) {
			t.Fatalf("Standby = %+v", st)
		}
		// 外から変えても内部は変わらない。
		st.Stack[0] = "x"
		st.StackNames[0] = "x"
		h.wantStack("A,D")
		// manual は単独の戻り先で、スタックは空。
		h2 := newHB(t, fbAD)
		h2.standbyOnManual(t)
		st = h2.m.Snapshot().Standby
		if len(st.Stack) != 0 || st.ReturnScene != camera {
			t.Fatalf("manual: %+v", st)
		}
	})
	t.Run("warnings_for_unresolved_rows", func(t *testing.T) {
		h := newHB(t, hbFB(FallbackRow{Scene: "NoSuch", Fallback: nameOf(deskScene)}, FallbackRow{Scene: nameOf(camera), Fallback: "Gone"}))
		ws := h.m.Snapshot().FallbackWarnings
		if len(ws) != 2 || ws[0] != (NameWarning{Index: 0, Field: "scene", Name: "NoSuch"}) || ws[1] != (NameWarning{Index: 1, Field: "fallback", Name: "Gone"}) {
			t.Fatalf("warnings = %+v", ws)
		}
		// 未接続（地図なし）では出さない。
		h.ev(OBSDisconnected{At: at(1000)})
		if ws := h.m.Snapshot().FallbackWarnings; len(ws) != 0 {
			t.Fatalf("未接続: %+v", ws)
		}
	})
}
