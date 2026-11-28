package switcher

import (
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 0b の表 4.7（Approve）と 4.8（Hide / Show / Restore の BatchDone）の行。

var kz = scenemapKey(camera2, 2)

func (h *hb) setScOnly(f func(m *scenemap.Map)) *scenemap.Map {
	m := editMap(h.mp, f)
	return m
}

// scAuto は 自動無効で H(x)、s DOWN の Sc(s)[x] 未完了の状態を作る。
func (h *hb) scX() Batch {
	h.t.Helper()
	h.yOff()
	h.seedH(kx)
	bs := h.sIs(1000, DOWN)
	h.out(bs, "Sc(s)[x]")
	return bs[0]
}

func TestHBApprove(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-AP-01", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			rev := h.m.st.HiddenRevision
			r := h.ap(b, 1100, kx, false)
			p, ok := r.(Proceed)
			if !ok || !p.MustPersist || p.ItemReqID == 0 {
				t.Fatalf("Approve = %#v", r)
			}
			h.wantEntries("x:P,y:P")
			h.wantQ("x:false")
			h.wantRev(true, rev)
			if _, ok := h.m.st.ItemReceipts[p.ItemReqID]; !ok {
				t.Fatal("送信台帳がない")
			}
		}},
		{"HB-AP-02", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			p1 := h.proceed(b, 1100, kx, false)
			rev := h.m.st.HiddenRevision
			p2 := h.proceed(b, 1100, ky, false)
			if p2.MustPersist || p2.ItemReqID == p1.ItemReqID {
				t.Fatalf("2個目: %+v / %+v", p1, p2)
			}
			h.wantQ("x:false,y:false")
			h.wantRev(false, rev)
		}},
		{"HB-AP-03", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			m := editMap(h.mp, func(m *scenemap.Map) {
				c := m.Scenes[deskScene]
				c.Items = nil
				m.Scenes[deskScene] = c
			})
			h.smc(1050, m)
			h.want(h.ap(b, 1100, ky, false), ky, b, 1100, false, Skip{})
			h.wantQ("")
			h.wantEntries("")
		}},
		{"HB-AP-04", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			m := editMap(h.mp, func(m *scenemap.Map) { m.Inputs[inSub] = scenemap.Input{Name: "cam2-src", CameraID: camMain} })
			h.smc(1050, m)
			h.want(h.ap(b, 1100, kx, false), kx, b, 1100, false, Skip{})
			h.wantQ("")
			_ = m
		}},
		{"HB-AP-05", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false)
			h.itemEv(1150, ky, false)
			h.want(h.ap(b, 1200, ky, false), ky, b, 1200, false, Skip{})
			h.wantQ("x:false")
			h.bdone(1300, b.BatchID, BatchCompleted, nil)
			h.wantEntries("x:P")
		}},
		{"HB-AP-06", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			b := h.scX()
			h.setMap(kx, true)
			h.want(h.ap(b, 1100, kx, true), kx, b, 1100, true, Skip{})
			h.wantEntries("")
			h.wantQ("")
		}},
		{"HB-AP-27", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.yOff()
			h.seedU(kx)
			bs := h.sIs(1000, DOWN)
			h.out(bs, "Sc(s)[x]")
			h.setMap(kx, true)
			h.want(h.ap(bs[0], 1100, kx, true), kx, bs[0], 1100, true, Skip{})
			h.wantEntries("x:U")
		}},
		{"HB-AP-07", func(t *testing.T) {
			conds := []struct {
				name string
				opts []hbOpt
				f    func(h *hb)
			}{
				{"c OK", nil, func(h *hb) { h.sIs(1050, OK) }},
				{"u あり", nil, func(h *hb) { h.m.st.UserShown[camSub] = true }},
				{"自動無効", nil, func(h *hb) {
					c := h.cfg
					c.At = at(1050)
					c.AutoEnabled = false
					h.ev(c)
				}},
				{"on_bad=standby", nil, func(h *hb) {
					c := h.cfg
					c.At = at(1050)
					c.Cameras = []CameraConfig{{ID: camMain, OnBad: scenemap.MotionStandby}, {ID: camSub, OnBad: scenemap.MotionStandby}}
					h.ev(c)
				}},
				{"Studio on / standby", nil, func(h *hb) { h.ev(StudioModeChanged{At: at(1050), Enabled: true}) }},
				{"Studio on / ignore", []hbOpt{hbStudioHide(StudioHideIgnore)}, func(h *hb) { h.ev(StudioModeChanged{At: at(1050), Enabled: true}) }},
			}
			for _, c := range conds {
				t.Run(c.name, func(t *testing.T) {
					h := newHB(t, c.opts...)
					b := h.hide(1000)
					c.f(h)
					h.want(h.ap(b, 1100, kx, false), kx, b, 1100, false, Reject{})
					h.wantQ("")
				})
			}
		}},
		{"HB-AP-08", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.m.st.Pending[99] = PendingBatch{BatchID: 99, Purpose: PurposeStartScene, SceneEpoch: h.m.st.SceneEpoch,
				ConnEpoch: h.m.st.ConnEpoch, StreamEpoch: h.m.st.StreamEpoch, ConfigEpoch: h.m.st.ConfigEpoch}
			h.want(h.ap(b, 1100, kx, false), kx, b, 1100, false, Reject{})
			delete(h.m.st.Pending, 99)
			h.m.st.Expectations = append(h.m.st.Expectations, Expectation{ReqID: 50, Scene: opening, Purpose: PurposeStartScene, ConfigEpoch: h.m.st.ConfigEpoch, Override: OverrideNone{}})
			h.want(h.ap(b, 1100, kx, false), kx, b, 1100, false, Reject{})
		}},
		{"HB-AP-09", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.m.st.Expectations = append(h.m.st.Expectations, Expectation{ReqID: 50, Scene: deskScene, Purpose: PurposeAutoRecover,
				ConfigEpoch: h.m.st.ConfigEpoch, Override: OverrideNone{}, Responded: true, Deadline: at(60000)})
			h.proceed(b, 1100, kx, false)
			h.want(h.ap(b, 1100, ky, false), ky, b, 1100, false, Reject{})
		}},
		{"HB-AP-10", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.out(h.scene(1050, camera2), "")
			c := h.cfg
			c.At = at(1060)
			c.StudioModeHide = StudioHideIgnore
			h.ev(c)
			h.proceed(b, 1100, kx, false)
			h.wantQ("x:false")
		}},
		{"HB-AP-11", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.reconnect(1050, true)
			h.want(h.ap(b, 1100, kx, false), kx, b, 1100, false, Reject{})
			h.wantQ("")
			h2 := newHB(t)
			b2 := h2.hide(1000)
			h2.ev(StreamStateChanged{At: at(1050), Streaming: false})
			h2.ev(StreamStateChanged{At: at(1060), Streaming: true})
			h2.want(h2.ap(b2, 1100, kx, false), kx, b2, 1100, false, Reject{})
			h2.wantQ("")
		}},
		{"HB-AP-12", func(t *testing.T) {
			h := newHB(t)
			h.seedH(kx, ky)
			bs := h.tick(1000)
			h.out(bs, "Sc(s)[R(is),x,y]")
			r := h.m.Approve(bs[0].BatchID, restartOf(inSub), OverrideNone{}, at(1100))
			if p, ok := r.(Proceed); !ok || p.ItemReqID != 0 {
				t.Fatalf("Restart = %#v", r)
			}
			h.wantQ("")
		}},
		{"HB-AP-13", func(t *testing.T) {
			h := newHB(t)
			h.seedH(kx, ky)
			bs := h.tick(1000)
			h.sIs(1050, DOWN)
			if r := h.m.Approve(bs[0].BatchID, restartOf(inSub), OverrideNone{}, at(1100)); r != (Skip{}) {
				t.Fatalf("c 不良: %#v", r)
			}
			h2 := newHB(t)
			h2.seedH(kx, ky)
			bs2 := h2.tick(1000)
			h2.smc(1050, editMap(h2.mp, func(m *scenemap.Map) { m.Inputs[inSub] = scenemap.Input{Name: "cam2-src", CameraID: camMain} }))
			if r := h2.m.Approve(bs2[0].BatchID, restartOf(inSub), OverrideNone{}, at(1100)); r != (Skip{}) {
				t.Fatalf("不一致: %#v", r)
			}
		}},
		{"HB-AP-14", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			b := h.scX()
			p := h.proceed(b, 1100, kx, true)
			if q := h.q(kx, true); q == nil || q.ReqID != p.ItemReqID {
				t.Fatalf("Q: %+v", q)
			}
		}},
		{"HB-AP-15", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.seedH(kx, ky)
			bs := h.sIs(1000, DOWN)
			h.out(bs, "Sc(s)[x,y]")
			h.itemEv(1050, kx, true) // 人が戻した: 一覧から外れる
			h.want(h.ap(bs[0], 1100, kx, true), kx, bs[0], 1100, true, Skip{})
			h.proceed(bs[0], 1100, ky, true) // 他の項目は続ける
			// 戻す条件が消えた: 自動有効に戻すと s DOWN の hide は戻せない
			c := h.cfg
			c.At = at(1200)
			h.ev(c)
			h2 := newHB(t, hbAutoOff())
			h2.seedH(kx)
			bs2 := h2.sIs(1000, DOWN)
			c2 := h2.cfg
			c2.At = at(1050)
			c2.AutoEnabled = true
			h2.ev(c2)
			h2.want(h2.ap(bs2[0], 1100, kx, true), kx, bs2[0], 1100, true, Skip{})
		}},
		{"HB-AP-16", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.sIs(1000, DOWN)
			m := editMap(h.mp, func(m *scenemap.Map) { m.Inputs[inSub] = scenemap.Input{Name: "cam2-src", CameraID: camMain} })
			bs := h.smc(1300, m)
			h.out(bs, "Sc(s)[x]")
			h.proceed(bs[0], 1400, kx, true)
		}},
		{"HB-AP-17", func(t *testing.T) {
			h := newHB(t, hbNoY())
			h.pX(1000)
			bs := h.ev(Command{At: at(1200), Kind: CmdShowAllCameras})
			h.out(bs, "MA[x]")
			h.proceed(bs[0], 1300, kx, true)
			h.wantQ("x:false,x:true")
		}},
		{"HB-AP-18", func(t *testing.T) {
			h := newHB(t)
			h.seed(HiddenH, false, kx)
			bs := h.ev(Stopping{At: at(1000)})
			h.out(bs, "RA[x]")
			h.want(h.ap(bs[0], 1100, kx, true), kx, bs[0], 1100, true, Skip{})
		}},
		{"HB-AP-19", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			bs := h.ev(Stopping{At: at(1000)})
			h.out(bs, "RA[x]")
			h.ev(StreamStateChanged{At: at(1050), Streaming: false})
			h.proceed(bs[0], 1100, kx, true)
		}},
		{"HB-AP-20", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.ev(Stopping{At: at(1050)})
			h.want(h.ap(b, 1100, kx, false), kx, b, 1100, false, Reject{})
			h2 := newHB(t, hbAutoOff())
			b2 := h2.scX()
			h2.ev(Stopping{At: at(1050)})
			h2.want(h2.ap(b2, 1100, kx, true), kx, b2, 1100, true, Reject{})
		}},
		{"HB-AP-21", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			bs := h.ev(Stopping{At: at(1000)})
			h.ev(OBSDisconnected{At: at(1050)})
			h.want(h.ap(bs[0], 1100, kx, true), kx, bs[0], 1100, true, Reject{})
			h.wantEntries("x:H")
		}},
		{"HB-AP-22", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.sIs(500, DOWN)
			bs := h.ev(Command{At: at(1000), Kind: CmdShowAllCameras})
			h.out(bs, "MA[x,y]")
			h.proceed(bs[0], 1100, ky, true)
		}},
		// /0c: 0b の「承認では戻り先の待ちを見ない」を撤回し、承認でも逆向きの待ちと OK の非表示の記録を検査する（HB-AP-23 の置き換え）。
		// R(A) は逆向きの待ちが解けたあとに出る。生成後に s が OK に戻ると、x の記録が OK の非表示の記録になり、SetScene は却下。
		{"HB-AP-23/0c", func(t *testing.T) {
			h := newHB(t, hbStudio())
			bs := h.sIs(1000, DOWN)
			h.out(bs, "F(A)")
			h.proceed2scene(bs[0], 1100)
			h.scene(1500, standby)
			bs = h.ev(StudioModeChanged{At: at(7000), Enabled: false})
			h.out(bs, "Hc(s)[x,y]")
			r := h.sendFalseOut(bs[0], 7100, kx, "R(A)")[0]
			h.sIs(7400, OK) // 生成後に待つ項目（s の本体の非表示の記録）ができた
			got := h.m.Approve(r.BatchID, SetScene{SceneUUID: camera}, OverrideNone{}, at(7500))
			if got != (Reject{}) {
				t.Fatalf("Approve = %#v, want Reject", got)
			}
		}},
		{"HB-AP-24/0c", func(t *testing.T) {
			h := newHB(t)
			h.standbyOnManual(t)
			bs, err := h.evErr(Command{At: at(2000), Kind: CmdStandbyOff})
			if err != nil {
				t.Fatal(err)
			}
			h.out(bs, "M(A)")
			h.yOff()
			h.seedH(kx)
			// 承認時にも待ち（OK のカメラの非表示の記録）を検査する。戻り先へ入れなくなったので OFF の計画が変わり Reject。
			if got := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: camera}, OverrideNone{}, at(2100)); got != (Reject{}) {
				t.Fatalf("Approve = %#v, want Reject", got)
			}
			h2 := newHB(t)
			h2.standbyOnManual(t)
			bs2, _ := h2.evErr(Command{At: at(2000), Kind: CmdStandbyOff})
			h2.cams(2050, LOW, OK) // 健康悪化は Replace でなく Reject（FB-AP-07）
			if got := h2.m.Approve(bs2[0].BatchID, SetScene{SceneUUID: camera}, OverrideNone{}, at(2100)); got != (Reject{}) {
				t.Fatalf("Approve = %#v, want Reject", got)
			}
		}},
		{"HB-AP-25", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false)
			bs := h.ev(Command{At: at(1200), Kind: CmdShowAllCameras})
			h.out(bs, "MA[x,y]")
			h.want(h.ap(b, 1300, ky, false), ky, b, 1300, false, Reject{})
			h.bdone(1400, b.BatchID, BatchRejected, nil)
			h.wantEntries("x:P") // 未送信の y は予約前へ戻る
		}},
		{"HB-AP-26", func(t *testing.T) {
			h := newHB(t)
			bs, _ := h.evErr(Command{At: at(100), Kind: CmdStandbyOn})
			h.ev(Stopping{At: at(200)})
			if got := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: standby}, OverrideNone{}, at(300)); !isProceed(got) {
				t.Fatalf("Approve = %#v", got)
			}
		}},
		{"HB-AP-28", func(t *testing.T) {
			h := newHB(t)
			h.seed(HiddenU, false, kx)
			bs := h.ev(Stopping{At: at(1000)})
			h.out(bs, "RA[x]")
			p := h.proceed(bs[0], 1100, kx, true)
			h.itemDone(1200, p.ItemReqID, ItemOK)
			h.wantEntries("")
			// MA は true OK だけでは消さず、期待どおりの true イベントで消す
			h2 := newHB(t)
			h2.seed(HiddenU, false, kx)
			bs2 := h2.ev(Command{At: at(1000), Kind: CmdShowAllCameras})
			h2.out(bs2, "MA[x]")
			p2 := h2.proceed(bs2[0], 1100, kx, true)
			h2.itemDone(1200, p2.ItemReqID, ItemOK)
			h2.wantEntries("x:U")
			h2.itemEv(1300, kx, true)
			h2.wantEntries("")
		}},
	})
}

func isProceed(r ApproveResult) bool { _, ok := r.(Proceed); return ok }

func TestHBBatchDone(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-BD-01", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false)
			h.m.st.UserShown[camSub] = true
			h.want(h.ap(b, 1200, ky, false), ky, b, 1200, false, Reject{})
			h.out(h.bdone(1300, b.BatchID, BatchRejected, nil), "")
			h.wantEntries("x:P")
			h.wantPendingNone()
			// 元の H/U は保持
			h2 := newHB(t)
			h2.seed(HiddenU, false, ky)
			b2 := h2.hide(1000)
			h2.proceed(b2, 1100, kx, false)
			h2.m.st.UserShown[camSub] = true
			h2.want(h2.ap(b2, 1200, ky, false), ky, b2, 1200, false, Reject{})
			h2.bdone(1300, b2.BatchID, BatchRejected, nil)
			h2.wantEntries("x:P,y:U")
		}},
		{"HB-BD-02", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.m.st.ItemFailures[camSub] = 2
			h.sendFalse(b, 1100, kx)
			h.sendFalse(b, 1100, ky)
			h.bdone(1200, b.BatchID, BatchCompleted, nil)
			h.wantF(camSub, 0)
			if cs := h.m.Snapshot().Cameras; cs[1].Unusable {
				t.Fatal("unusable のまま")
			}
		}},
		{"HB-BD-03", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false)
			h.out(h.bdone(1200, b.BatchID, BatchCompleted, errTest), "")
			h.wantF(camSub, 1)
			h.wantNoErr()
		}},
		{"HB-BD-04", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.m.st.ItemFailures[camSub] = 2
			h.proceed(b, 1100, kx, false)
			h.bdone(1200, b.BatchID, BatchCompleted, errTest)
			h.wantF(camSub, 3)
			if cs := h.m.Snapshot().Cameras; !cs[1].Unusable {
				t.Fatal("unusable でない")
			}
			if h.m.st.LastError == nil {
				t.Fatal("last_error を記録しない")
			}
		}},
		{"HB-BD-05", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.m.st.ItemFailures[camSub] = 1
			h.out(h.bdone(1200, b.BatchID, BatchRejected, nil), "")
			h.wantF(camSub, 1)
			h.wantEntries("")
		}},
		{"HB-BD-06", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.m.st.ItemFailures[camSub] = 1
			h.itemEv(1050, kx, false)
			h.itemEv(1050, ky, false)
			h.want(h.ap(b, 1100, kx, false), kx, b, 1100, false, Skip{})
			h.want(h.ap(b, 1100, ky, false), ky, b, 1100, false, Skip{})
			h.bdone(1200, b.BatchID, BatchCompleted, nil)
			h.wantF(camSub, 1)
			h.wantPendingNone()
		}},
		{"HB-BD-07", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			bs := h.tick(1000)
			h.out(bs, "Sc(s)[R(is),x]")
			p := h.proceed(bs[0], 1100, kx, true)
			h.itemDone(1200, p.ItemReqID, ItemOK)
			h.out(h.bdone(1300, bs[0].BatchID, BatchCompleted, nil), "")
			h.wantEntries("x:H")
			h.wantQ("x:true")
		}},
		{"HB-BD-08", func(t *testing.T) {
			h := newHB(t, hbMapEdit(func(m *scenemap.Map) {
				c := m.Scenes[camera2]
				c.Items = append(c.Items, scenemapItem(2, inSub, true))
				m.Scenes[camera2] = c
			}))
			h.seedH(kx, ky, kz)
			bs := h.ev(Stopping{At: at(1000)})
			h.out(bs, "RA[x,z,y]")
			px := h.proceed(bs[0], 1100, kx, true)
			py := h.proceed(bs[0], 1100, ky, true)
			pz := h.proceed(bs[0], 1100, kz, true)
			h.itemDone(1200, px.ItemReqID, ItemOK)
			h.itemDone(1200, py.ItemReqID, ItemTimeout)
			h.itemDone(1200, pz.ItemReqID, ItemError)
			h.out(h.bdone(1300, bs[0].BatchID, BatchCompleted, errTest), "")
			h.wantEntries("y:H,z:H")
		}},
		{"HB-BD-09", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			bs := h.ev(Stopping{At: at(1000)})
			h.out(h.bdone(1100, bs[0].BatchID, BatchRejected, nil), "")
			h.wantEntries("x:H")
			h2 := newHB(t)
			h2.yOff()
			h2.seedH(kx)
			bs2 := h2.ev(Stopping{At: at(1000)})
			h2.out(h2.bdone(1100, bs2[0].BatchID, BatchFailed, errTest), "")
			h2.wantEntries("x:H")
		}},
		{"HB-BD-10", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false)
			before := cloneState(h.m.st)
			h.out(h.bdone(1200, 999, BatchCompleted, errTest), "")
			h.wantUnchanged(before)
			h.bdone(1300, b.BatchID, BatchCompleted, errTest)
			after := cloneState(h.m.st)
			rev := h.m.st.HiddenRevision
			h.out(h.bdone(1400, b.BatchID, BatchCompleted, errTest), "")
			h.wantUnchanged(after)
			h.wantRev(false, rev)
		}},
		{"HB-BD-11", func(t *testing.T) {
			h := newHB(t)
			h.seedH(kx, ky)
			h.sIs(500, DOWN)
			bs := h.ev(Command{At: at(1000), Kind: CmdShowAllCameras})
			h.out(bs, "MA[x,y]")
			px := h.proceed(bs[0], 1100, kx, true)
			py := h.proceed(bs[0], 1100, ky, true)
			h.itemDone(1200, px.ItemReqID, ItemOK)
			h.itemDone(1200, py.ItemReqID, ItemError)
			h.out(h.bdone(1300, bs[0].BatchID, BatchCompleted, errTest), "")
			h.wantEntries("x:H,y:H")
			h.wantErrContains("表示に戻せなかった項目があります。もう一度押してください")
			if q := h.q(kx, true); q == nil || !q.Deadline.Equal(at(3200)) {
				t.Fatalf("Q: %+v", q)
			}
			h.tick(3201)
			h.wantEntries("x:H,y:H") // 読み直しの表示確認まで保持
		}},
	})
}
