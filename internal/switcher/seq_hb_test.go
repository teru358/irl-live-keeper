package switcher

import (
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 0b の表5章の列（HB-SQ-*）。時刻は秒を ms で書く。各列は独立。送信は常に個別の Approve を挟む。

// trueAll は Sc / MA の true を全部承認し、OK とイベントを流す（Restart は承認だけ）。
func (h *hb) trueAll(b Batch, ms int, keys ...scenemap.ItemKey) {
	h.t.Helper()
	for _, a := range b.Actions {
		if _, ok := a.(RestartMedia); ok {
			if r := h.m.Approve(b.BatchID, a, OverrideNone{}, at(ms)); !isProceed(r) {
				h.t.Fatalf("Restart: %#v", r)
			}
		}
	}
	for _, k := range keys {
		h.sendTrue(b, ms, k)
	}
}

func TestHBSQ01メインstandbyサブhide(t *testing.T) {
	t.Run("HB-SQ-01", hbSQ01メインstandbyサブhide)
}

func hbSQ01メインstandbyサブhide(t *testing.T) {
	h := newHB(t)
	b := h.hide(1000)
	h.wantHealth(camera, OK)
	h.wantMap(kx, true)
	px := h.proceed(b, 1100, kx, false)
	py := h.proceed(b, 1100, ky, false)
	h.wantEntries("x:P,y:P")
	h.wantQ("x:false,y:false")
	h.wantA(camera, "main")
	h.wantV(camera, "main", "sub")
	h.out(h.itemDone(1200, px.ItemReqID, ItemOK), "")
	h.out(h.itemDone(1200, py.ItemReqID, ItemOK), "")
	h.out(h.bdone(1200, b.BatchID, BatchCompleted, nil), "")
	if q := h.q(kx, false); q == nil || !q.Deadline.Equal(at(3200)) {
		t.Fatalf("Q: %+v", q)
	}
	h.out(h.itemEv(1300, kx, false), "")
	h.out(h.itemEv(1300, ky, false), "")
	h.wantEntries("x:H,y:H")
	h.wantV(camera, "main", "sub")
	h.wantA(camera, "main")
	bs := h.sIs(7000, OK)
	h.out(bs, "Sc(s)[R(is),x,y]")
	h.wantF(camSub, 0)
	h.wantU(camSub, false)
	h.trueAll(bs[0], 7100, kx, ky)
	h.wantEntries("")
	h.out(h.bdone(7700, bs[0].BatchID, BatchCompleted, nil), "")
	h.wantA(camera, "main", "sub")
	h.wantHealth(camera, OK)
}

func TestHBSQ02サブだけのシーン(t *testing.T) {
	t.Run("HB-SQ-02", hbSQ02サブだけのシーン)
}

func hbSQ02サブだけのシーン(t *testing.T) {
	t.Run("本列", func(t *testing.T) {
		h := newHB(t, hbAt(deskScene))
		bs := h.sIs(1000, DOWN)
		h.out(bs, "F(D)")
		h.wantA(deskScene, "sub")
		r := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: standby}, OverrideNone{}, at(1100))
		p := r.(Proceed)
		h.ev(SceneRequestDone{At: at(1100), ReqID: p.ReqID, Result: RequestOK})
		h.out(h.tick(1150), "")
		h.out(h.bdone(1200, bs[0].BatchID, BatchCompleted, nil), "")
		h.out(h.tick(1250), "")
		h.out(h.scene(1300, standby), "Hc(s)[x,y]")
		hc := h.lastBatch
		h.sendFalse(hc, 1400, kx)
		h.sendFalse(hc, 1400, ky)
		h.bdone(1600, hc.BatchID, BatchCompleted, nil)
		h.wantEntries("x:H,y:H")
		sc := h.sIs(8000, OK)
		h.out(sc, "Sc(s)[R(is),x,y]") // D に待つ項目があるので R(D) はまだ出ない
		h.m.Approve(sc[0].BatchID, restartOf(inSub), OverrideNone{}, at(8100))
		h.sendTrue(sc[0], 8200, kx)
		bs3 := h.sendTrueOut(sc[0], 8300, ky, "R(D)")
		h.bdone(8800, sc[0].BatchID, BatchCompleted, nil)
		// 戻る側の Restart は hide の入力を再び Restart しない
		if len(bs3[0].Actions) != 1 {
			t.Fatalf("R(D) の Action: %+v", bs3[0].Actions)
		}
		h.proceed2scene(bs3[0], 9000)
		h.scene(9100, deskScene)
		h.wantNoOwner()
	})
	t.Run("別列 SetScene が Error", func(t *testing.T) {
		h := newHB(t, hbAt(deskScene))
		bs := h.sIs(1000, DOWN)
		h.out(bs, "F(D)")
		p := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: standby}, OverrideNone{}, at(1100)).(Proceed)
		h.ev(SceneRequestDone{At: at(1100), ReqID: p.ReqID, Result: RequestError})
		h.out(h.bdone(1200, bs[0].BatchID, BatchFailed, errTest), "")
		h.out(h.tick(1300), "")
		h.wantEntries("")
	})
}

func TestHBSQ03隠している途中で人が戻す(t *testing.T) {
	t.Run("HB-SQ-03", hbSQ03隠している途中で人が戻す)
}

func hbSQ03隠している途中で人が戻す(t *testing.T) {
	h := newHB(t)
	b := h.hide(1000)
	p := h.proceed(b, 1100, kx, false)
	h.itemDone(1100, p.ItemReqID, ItemOK)
	h.itemEv(1100, kx, false)
	h.wantEntries("x:H,y:P")
	h.out(h.itemEv(1200, kx, true), "")
	h.wantEntries("x:H,y:P")
	h.wantU(camSub, true)
	h.want(h.ap(b, 1300, ky, false), ky, b, 1300, false, Reject{})
	h.out(h.bdone(1400, b.BatchID, BatchRejected, nil), "")
	h.wantEntries("x:H")
	h.wantF(camSub, 0)
	h.out(h.tick(3101), "") // false の期待が終わり、地図 true なので外れる
	h.wantEntries("")
	h.out(h.tick(7000), "") // 同じ不調中は隠し直さない
	h.out(h.sIs(8000, OK), "")
	h.wantU(camSub, false)
	h.out(h.sIs(9000, DOWN), "Hc(s)[x,y]")
}

func TestHBSQ04TimeoutとSameZ(t *testing.T) {
	t.Run("HB-SQ-04", hbSQ04TimeoutとSameZ)
}

func hbSQ04TimeoutとSameZ(t *testing.T) {
	h := newHB(t)
	h.yOff()
	b := h.hcX(1000)
	px := h.proceed(b, 1100, kx, false)
	h.itemDone(6000, px.ItemReqID, ItemTimeout)
	h.out(h.bdone(6000, b.BatchID, BatchCompleted, errTest), "")
	h.wantEntries("x:U")
	if q := h.q(kx, false); q == nil || !q.Deadline.Equal(at(8000)) {
		t.Fatalf("Q: %+v", q)
	}
	h.wantF(camSub, 1)
	h.ev(OBSDisconnected{At: at(6100)})
	h.wantQ("")
	h.wantEntries("x:U")
	bs := h.connect(7000, true, camera, false)
	h.out(bs, "Hc(s)[x]")
	p2 := h.proceed(bs[0], 7100, kx, false)
	h.itemDone(7100, p2.ItemReqID, ItemOK)
	h.itemEv(7100, kx, false)
	h.out(h.bdone(7100, bs[0].BatchID, BatchCompleted, nil), "")
	h.wantEntries("x:H")
	h.wantF(camSub, 0)
	if q := h.q(kx, false); q == nil || !q.Deadline.Equal(at(9100)) {
		t.Fatalf("Q: %+v", q)
	}
	h.out(h.tick(18000), "")
	h.wantEntries("x:H")
	h.out(h.sIs(21000, OK), "Sc(s)[R(is),x]")
}

func TestHBSQ04B切断が結果より先(t *testing.T) {
	t.Run("HB-SQ-04B", hbSQ04B切断が結果より先)
}

func hbSQ04B切断が結果より先(t *testing.T) {
	setup := func(t *testing.T) (*hb, Batch, uint64) {
		h := newHB(t)
		b := h.hcX(1000)
		p := h.proceed(b, 1100, kx, false)
		h.ev(OBSDisconnected{At: at(2000)})
		h.wantQ("")
		h.wantEntries("x:P")
		return h, b, p.ItemReqID
	}
	finish := func(t *testing.T, h *hb, b Batch, req uint64, restored bool) {
		h.out(h.itemDone(2100, req, ItemTimeout), "")
		h.wantEntries("x:U")
		h.wantQ("")
		h.out(h.bdone(2200, b.BatchID, BatchCompleted, errTest), "")
		h.wantF(camSub, 1)
		h.wantPendingNone()
		h.out(h.sIs(3000, OK), "")
		h.wantF(camSub, 0)
		if restored {
			// 再起動: 新しい Machine に U の記録を渡す
			h2 := newHB(t, hbNoConn())
			h2.yOff()
			h2.sIs(10, OK)
			h2.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{h.entryOf(kx, HiddenU)}})
			setItem(h2.mp, kx.ContainerUUID, kx.ItemID, true)
			h = h2
			h.out(h.connect(7000, true, camera, false), "")
		} else {
			h.out(h.connect(7000, true, camera, false), "")
		}
		h.wantEntries("x:U")
		h.out(h.itemEv(20000, kx, false), "Sc(s)[R(is),x]")
		h.wantEntries("x:H")
	}
	t.Run("本列", func(t *testing.T) { h, b, r := setup(t); finish(t, h, b, r, false) })
	t.Run("再起動を挟む列", func(t *testing.T) { h, b, r := setup(t); finish(t, h, b, r, true) })
}

func TestHBSQ05配信停止(t *testing.T) {
	t.Run("HB-SQ-05", hbSQ05配信停止)
}

func hbSQ05配信停止(t *testing.T) {
	h := newHB(t)
	b := h.hcX(1000)
	p := h.proceed(b, 1000, kx, false)
	h.itemDone(1000, p.ItemReqID, ItemOK)
	h.out(h.ev(StreamStateChanged{At: at(2000), Streaming: false}), "")
	h.wantQ("x:false")
	h.wantU(camSub, false)
	h.out(h.itemEv(2100, kx, false), "")
	h.wantEntries("x:H")
	h.wantU(camSub, false)
	h.out(h.bdone(2200, b.BatchID, BatchCompleted, nil), "")
	bs := h.tick(6000)
	h.out(bs, "Sc(s)[x]")
	h.trueAll(bs[0], 6100, kx)
	h.out(h.bdone(6300, bs[0].BatchID, BatchCompleted, nil), "")
	h.wantEntries("")
}

func TestHBSQ06本体の停止と2秒(t *testing.T) {
	t.Run("HB-SQ-06", hbSQ06本体の停止と2秒)
}

func hbSQ06本体の停止と2秒(t *testing.T) {
	h := newHB(t)
	b := h.hide(1000)
	h.proceed(b, 1000, kx, false)
	h.wantEntries("x:P,y:P")
	bs := h.ev(Stopping{At: at(1100)})
	h.out(bs, "RA[x,y]")
	ra := bs[0]
	h.want(h.ap(b, 1200, ky, false), ky, b, 1200, false, Reject{})
	h.out(h.bdone(1200, b.BatchID, BatchRejected, nil), "")
	h.wantEntries("x:P")
	p := h.proceed(ra, 1300, kx, true) // 地図 true でも P と false の期待があるので送る
	h.want(h.ap(ra, 1300, ky, true), ky, ra, 1300, true, Skip{})
	h.out(h.itemDone(1400, p.ItemReqID, ItemOK), "")
	h.out(h.bdone(1400, ra.BatchID, BatchCompleted, nil), "")
	h.wantEntries("")
	// RA の要求が戻らない別列は core の停止待ち（M-12）。未復旧の記録を消さないことだけ確かめる。
	h2 := newHB(t)
	h2.yOff()
	h2.seedH(kx)
	ra2 := h2.ev(Stopping{At: at(1000)})
	h2.proceed(ra2[0], 1100, kx, true)
	h2.out(h2.bdone(3100, ra2[0].BatchID, BatchFailed, errTest), "")
	h2.wantEntries("x:H")
}

func TestHBSQ07hiddenJsonからの起動(t *testing.T) {
	t.Run("HB-SQ-07", hbSQ07hiddenJsonからの起動)
}

func hbSQ07hiddenJsonからの起動(t *testing.T) {
	start := func(t *testing.T, streaming bool, mapOpt func(m *scenemap.Map), camID string) (*hb, []Batch) {
		h := newHB(t, hbNoConn())
		h.yOff()
		h.sIs(10, DOWN)
		ex := h.entryOf(kx, HiddenH)
		ex.CameraID = camID
		z := h.entryOf(kz, HiddenH)
		z.Collection = "collection-y"
		h.out(h.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{ex, z}}), "")
		h.wantRetainedOrHidden("x:H,z:H")
		setItem(h.mp, kx.ContainerUUID, kx.ItemID, false)
		if mapOpt != nil {
			mapOpt(h.mp)
		}
		return h, h.connect(1000, streaming, camera, false)
	}
	t.Run("本列 非配信", func(t *testing.T) {
		h, bs := start(t, false, nil, camSub)
		h.out(bs, "Sc(s)[x]")
		h.wantEntries("x:H")
		h.wantRetained("z:H")
		h.sendTrue(bs[0], 1100, kx)
		h.out(h.bdone(1200, bs[0].BatchID, BatchCompleted, nil), "")
		h.wantEntries("")
		h.wantRetained("z:H")
		if n := len(h.m.Snapshot().RetainedEntries); n != 1 {
			t.Fatalf("Snapshot.RetainedEntries: %d", n)
		}
	})
	t.Run("別列 配信中 s DOWN は隠したまま", func(t *testing.T) {
		h, bs := start(t, true, nil, camSub)
		h.out(bs, "")
		h.wantEntries("x:H")
	})
	t.Run("別列 設定にないカメラは配信中でも戻す", func(t *testing.T) {
		h, bs := start(t, true, nil, "gone")
		h.out(bs, "Sc(gone)[x]")
	})
	t.Run("別列 空地図なら出さず、あとで有効地図が来たら照合", func(t *testing.T) {
		h := newHB(t, hbNoConn())
		h.yOff()
		h.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{h.entryOf(kx, HiddenH)}})
		h.out(h.ev(OBSConnected{At: at(1000), Streaming: false, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300),
			Map: &scenemap.Map{Collection: collectionX}}), "")
		h.wantRetainedOrHidden("x:H")
		setItem(h.mp, kx.ContainerUUID, kx.ItemID, false)
		h.out(h.smc(2000, cloneTestMap(h.mp)), "Sc(s)[R(is),x]")
		h.wantEntries("x:H")
	})
}

func TestHBSQ08StudioMode(t *testing.T) {
	t.Run("HB-SQ-08", hbSQ08StudioMode)
}

func hbSQ08StudioMode(t *testing.T) {
	base := func(t *testing.T, mode string) *hb {
		h := newHB(t, hbStudioHide(mode))
		h.seedH(kx, ky)
		h.sIs(100, DOWN)
		return h
	}
	t.Run("standby", func(t *testing.T) {
		h := base(t, StudioHideStandby)
		bs := h.ev(StudioModeChanged{At: at(1000), Enabled: true})
		h.out(bs, "F(A) Sc(s)[x,y]")
		p := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: standby}, OverrideNone{}, at(1100)).(Proceed)
		h.ev(SceneRequestDone{At: at(1100), ReqID: p.ReqID, Result: RequestError})
		h.bdone(1100, bs[0].BatchID, BatchFailed, errTest)
		h.trueAll(bs[1], 1100, kx, ky)
		h.out(h.bdone(1100, bs[1].BatchID, BatchCompleted, nil), "")
		h.scene2(camera)
		h.wantEntries("")
		h.out(h.sIs(8000, OK), "")
		h.wantU(camSub, false)
		h.wantF(camSub, 0)
		h.out(h.ev(StudioModeChanged{At: at(9000), Enabled: false}), "")
		h.scene2(camera)
	})
	for _, mode := range []string{StudioHideIgnore, StudioHideHide} {
		t.Run(mode, func(t *testing.T) {
			h := base(t, mode)
			h.out(h.ev(StudioModeChanged{At: at(1000), Enabled: true}), "")
			h.wantEntries("x:H,y:H")
			bs := h.sIs(8000, OK)
			h.out(bs, "Sc(s)[R(is),x,y]")
			h.trueAll(bs[0], 8100, kx, ky)
			h.out(h.bdone(8200, bs[0].BatchID, BatchCompleted, nil), "")
			h.wantEntries("")
			h.out(h.ev(StudioModeChanged{At: at(9000), Enabled: false}), "")
			h.scene2(camera)
		})
	}
}

func (h *hb) scene2(want string) {
	h.t.Helper()
	if got := h.m.st.Scene; got != want {
		h.t.Fatalf("現在のシーン: got %q, want %q", got, want)
	}
}

func (h *hb) wantNoOwner() {
	h.t.Helper()
	if st := h.m.Snapshot().Standby; st.Owner != OwnerNone {
		h.t.Fatalf("持ち主: %+v", st)
	}
}

func TestHBSQ08Bignore中の他の戻す理由(t *testing.T) {
	t.Run("HB-SQ-08B", hbSQ08Bignore中の他の戻す理由)
}

func hbSQ08Bignore中の他の戻す理由(t *testing.T) {
	base := func(t *testing.T) *hb {
		h := newHB(t, hbStudio(), hbStudioHide(StudioHideIgnore))
		h.seedH(kx, ky)
		h.sIs(100, DOWN)
		return h
	}
	runHB(t, []hbRow{
		{"HB-SM-07", func(t *testing.T) {
			h := base(t)
			h.out(h.ev(StreamStateChanged{At: at(1000), Streaming: false}), "Sc(s)[x,y]")
			h.wantEntries("x:H,y:H")
		}},
		{"HB-SM-08", func(t *testing.T) {
			h := base(t)
			c := h.cfg
			c.At = at(1000)
			c.AutoEnabled = false
			h.out(h.ev(c), "Sc(s)[x,y]")
			h.wantEntries("x:H,y:H")
		}},
		{"HB-SM-09", func(t *testing.T) {
			h := base(t)
			ce := h.m.st.ConfigEpoch
			c := h.cfg
			c.At = at(1000)
			c.Cameras = []CameraConfig{{ID: camMain, OnBad: scenemap.MotionStandby}, {ID: camSub, OnBad: scenemap.MotionStandby}}
			h.out(h.ev(c), "F(A) Sc(s)[x,y]")
			if h.m.st.ConfigEpoch != ce+1 {
				t.Fatalf("ConfigEpoch %d -> %d", ce, h.m.st.ConfigEpoch)
			}
		}},
	})
}

func TestHBSQ09コレクションを切り替えて戻る(t *testing.T) {
	t.Run("HB-SQ-09", hbSQ09コレクションを切り替えて戻る)
}

func hbSQ09コレクションを切り替えて戻る(t *testing.T) {
	h := newHB(t)
	h.yOff()
	h.seedH(kx)
	h.sIs(100, OK)
	h.ev(OBSDisconnected{At: at(1000)})
	h.wantEntries("x:H")
	y := cloneTestMap(h.mp)
	y.Collection = "collection-y"
	setItem(y, kx.ContainerUUID, kx.ItemID, true)
	setItem(y, ky.ContainerUUID, ky.ItemID, true)
	h.out(h.ev(OBSConnected{At: at(2000), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300), Map: y}), "")
	h.wantRetained("x:H")
	h.wantEntries("")
	ye := h.entryOf(ky, HiddenH)
	ye.Collection = "collection-y"
	h.m.st.Hidden = append(h.m.st.Hidden, ye)
	h.ev(OBSDisconnected{At: at(3000)})
	bs := h.connect(4000, true, camera, false)
	h.out(bs, "Sc(s)[R(is),x]")
	h.wantEntries("x:H")
	h.wantRetained("y:H")
	h.trueAll(bs[0], 4100, kx)
	h.out(h.bdone(4200, bs[0].BatchID, BatchCompleted, nil), "")
	h.wantEntries("")
	h.wantRetained("y:H")
}

func TestHBSQ09B本体切替の期待中の空検査(t *testing.T) {
	t.Run("HB-SQ-09B", hbSQ09B本体切替の期待中の空検査)
}

func hbSQ09B本体切替の期待中の空検査(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-GD-01", func(t *testing.T) {
			h := newHB(t, hbAt(deskScene))
			bs := h.sIs(1000, DOWN)
			h.out(bs, "F(D)")
			h.proceed2scene(bs[0], 1100)
			h.scene(1200, standby)
			h.bdone2(1250)
			bs = h.sIs(8000, OK)
			h.out(bs, "R(D)")
			if r := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: deskScene}, OverrideNone{}, at(8100)); !isProceed(r) {
				t.Fatalf("SetScene(D): %#v", r)
			}
			h.out(h.sIs(8200, LOW), "")
		}},
		{"HB-GD-02", func(t *testing.T) {
			h := newHB(t, hbAt(talk), hbStart(deskScene))
			h.ev(StreamStateChanged{At: at(500), Streaming: false})
			bs := h.ev(StreamStateChanged{At: at(1000), Streaming: true})
			h.out(bs, "St(D)")
			h.proceed2scene(bs[0], 1100)
			h.out(h.sIs(1200, LOW), "")
			h.out(h.tick(2000), "")
		}},
		{"HB-GD-03", func(t *testing.T) {
			h := newHB(t, hbAt(deskScene))
			bs := h.sIs(1000, DOWN)
			h.out(bs, "F(D)")
			h.proceed2scene(bs[0], 1100)
			hc := h.scene(1200, standby)
			h.out(hc, "Hc(s)[x,y]")
			h.m.st.Expectations = append(h.m.st.Expectations, Expectation{ReqID: 50, Scene: deskScene, Purpose: PurposeAutoRecover,
				ConfigEpoch: h.m.st.ConfigEpoch, Override: OverrideNone{}, Responded: true, Deadline: at(60000)})
			h.proceed(hc[0], 1300, kx, false)
			h.want(h.ap(hc[0], 1300, ky, false), ky, hc[0], 1300, false, Reject{})
		}},
	})
}

func (h *hb) bdone2(ms int) {}

func TestHBSQ10すべて表示に戻す(t *testing.T) {
	t.Run("HB-SQ-10", hbSQ10すべて表示に戻す)
}

func hbSQ10すべて表示に戻す(t *testing.T) {
	h := newHB(t)
	h.yOff()
	h.seedH(kx)
	other := h.entryOf(kz, HiddenH)
	other.Collection = "collection-y"
	h.m.st.Retained = []HiddenEntry{other}
	h.sIs(500, DOWN)
	h.wantNoSent(camSub)
	bs, err := h.evErr(Command{At: at(2000), Kind: CmdShowAllCameras})
	if err != nil {
		t.Fatal(err)
	}
	h.out(bs, "MA[x,y]")
	h.wantEntries("x:H")
	h.wantNoSent(camSub)
	ma := bs[0]
	px := h.proceed(ma, 2100, kx, true)
	py := h.proceed(ma, 2100, ky, true)
	h.itemDone(2100, px.ItemReqID, ItemOK)
	h.itemDone(2100, py.ItemReqID, ItemOK)
	h.out(h.bdone(2100, ma.BatchID, BatchCompleted, nil), "")
	h.wantEntries("x:H")
	h.wantRetained("z:H")
	h.wantNoSent(camSub)
	if q := h.q(kx, true); q == nil || !q.Deadline.Equal(at(4100)) {
		t.Fatalf("Q(x): %+v", q)
	}
	h.out(h.itemEv(2200, kx, true), "")
	h.wantEntries("")
	if q := h.q(kx, true); q == nil || !q.SawEvent {
		t.Fatalf("Q(x): %+v", q)
	}
	h.out(h.itemEv(2300, ky, true), "")
	h.wantQ("x:true,y:true")
	h.out(h.tick(2400), "")
	h.wantMap(kx, true)
	h.wantMap(ky, true)
	h.out(h.tick(4100), "")
	h.wantQ("x:true,y:true")
	bs = h.tick(4200)
	h.out(bs, "Hc(s)[x,y]")
	h.wantQ("")
	h.wantSent(camSub, 4200)
	h.sendFalse(bs[0], 4300, kx)
	h.sendFalse(bs[0], 4300, ky)
	h.out(h.bdone(4300, bs[0].BatchID, BatchCompleted, nil), "")
	h.wantEntries("x:H,y:H")
	if q := h.q(kx, false); q == nil || !q.Deadline.Equal(at(6300)) {
		t.Fatalf("Q(false): %+v", q)
	}
	h.out(h.tick(8000), "")
	h.wantQ("")
	h.wantEntries("x:H,y:H")
}

func TestHBSQ10AMAが実行中Hideの残りを止める(t *testing.T) {
	t.Run("HB-SQ-10A", hbSQ10AMAが実行中Hideの残りを止める)
}

func hbSQ10AMAが実行中Hideの残りを止める(t *testing.T) {
	h := newHB(t)
	h1 := h.hide(1000)
	p := h.proceed(h1, 1000, kx, false)
	h.wantEntries("x:P,y:P")
	bs, _ := h.evErr(Command{At: at(2000), Kind: CmdShowAllCameras})
	h.out(bs, "MA[x,y]")
	ma := bs[0]
	h.want(h.ap(h1, 2100, ky, false), ky, h1, 2100, false, Reject{})
	h.out(h.bdone(2200, h1.BatchID, BatchRejected, nil), "")
	h.wantEntries("x:P")
	if _, ok := h.m.st.ItemReceipts[p.ItemReqID]; !ok {
		t.Fatal("台帳が消えた")
	}
	h.itemDone(2300, p.ItemReqID, ItemOK)
	h.out(h.itemEv(2300, kx, false), "")
	h.wantEntries("x:H")
	h.out(h.tick(2350), "")
	px := h.proceed(ma, 2400, kx, true)
	h.want(h.ap(ma, 2400, ky, true), ky, ma, 2400, true, Skip{}) // y は隠していない（地図 true、記録なし）
	h.itemDone(2400, px.ItemReqID, ItemOK)
	h.itemEv(2450, kx, true)
	h.out(h.bdone(2500, ma.BatchID, BatchCompleted, nil), "")
	h.wantEntries("")
	h.wantMap(kx, true)
	h.wantMap(ky, true)
}

func TestHBSQ10BMAとイベント未着と失敗(t *testing.T) {
	t.Run("HB-SQ-10B", hbSQ10BMAとイベント未着と失敗)
}

func hbSQ10BMAとイベント未着と失敗(t *testing.T) {
	h := newHB(t, hbNoY())
	h.yOff()
	b := h.hcX(1000)
	p := h.proceed(b, 1000, kx, false)
	h.itemDone(1000, p.ItemReqID, ItemOK)
	bs, _ := h.evErr(Command{At: at(2000), Kind: CmdShowAllCameras})
	h.out(bs, "MA[x]")
	ma := bs[0]
	h.out(h.itemEv(2100, kx, false), "")
	h.wantEntries("x:H")
	if q := h.q(kx, false); q == nil || !q.SawEvent {
		t.Fatalf("Q: %+v", q)
	}
	h.out(h.bdone(2200, b.BatchID, BatchCompleted, nil), "")
	pt := h.proceed(ma, 2200, kx, true)
	h.itemDone(2200, pt.ItemReqID, ItemError)
	h.out(h.bdone(2200, ma.BatchID, BatchCompleted, errTest), "")
	h.wantEntries("x:H")
	h.wantErrContains("もう一度押してください")
	if n := len(h.m.Snapshot().HiddenEntries); n != 1 {
		t.Fatalf("Snapshot.HiddenEntries: %d", n)
	}
}

func TestHBSQ10CMAのOK後もイベントが来ない(t *testing.T) {
	t.Run("HB-SQ-10C", hbSQ10CMAのOK後もイベントが来ない)
}

func hbSQ10CMAのOK後もイベントが来ない(t *testing.T) {
	run := func(t *testing.T, readback bool) {
		h := newHB(t, hbNoY())
		h.yOff()
		h.seedH(kx)
		h.sIs(500, DOWN)
		bs, _ := h.evErr(Command{At: at(2000), Kind: CmdShowAllCameras})
		h.out(bs, "MA[x]")
		p := h.proceed(bs[0], 2100, kx, true)
		h.itemDone(2100, p.ItemReqID, ItemOK)
		h.out(h.bdone(2100, bs[0].BatchID, BatchCompleted, nil), "")
		h.wantEntries("x:H")
		h.wantMap(kx, false)
		if q := h.q(kx, true); q == nil || !q.Deadline.Equal(at(4100)) {
			t.Fatalf("Q: %+v", q)
		}
		h.out(h.tick(4100), "")
		h.wantQ("x:true")
		h.wantEntries("x:H")
		h.out(h.tick(4200), "")
		h.wantQ("")
		h.wantEntries("x:H")
		h.wantMap(kx, false)
		m := cloneTestMap(h.mp)
		setItem(m, kx.ContainerUUID, kx.ItemID, readback)
		if readback {
			h.out(h.smc(5000, m), "Hc(s)[x]")
			h.wantEntries("")
			h.wantU(camSub, false)
		} else {
			m2 := editMap(m, func(m *scenemap.Map) { setItem(m, camera2, 1, false) }) // 地図が変わったことにする
			h.out(h.smc(5000, m2), "")
			h.wantEntries("x:H")
		}
	}
	t.Run("本列 読み直しで x true", func(t *testing.T) { run(t, true) })
	t.Run("別列 読み直しが x false", func(t *testing.T) { run(t, false) })
}
