package switcher

import (
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 0b の表 4.4（SceneMapChanged）、4.4a（SceneChanged）、4.5（接続、配信、設定、Studio Mode）、4.5a の行。

func TestHBSceneMapChanged(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-MP-01", func(t *testing.T) {
			h := newHB(t)
			h.pX(1000)
			m := editMap(h.mp, func(m *scenemap.Map) {
				c := m.Scenes[camera]
				c.Items = c.Items[:1]
				m.Scenes[camera] = c
			})
			h.out(h.smc(1300, m), "")
			h.wantEntries("")
			h.wantQ("")
		}},
		{"HB-MP-02", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.out(h.sIs(1000, DOWN), "")
			m := editMap(h.mp, func(m *scenemap.Map) { m.Inputs[inSub] = scenemap.Input{Name: "cam2-src", CameraID: camMain} })
			bs := h.smc(1300, m)
			h.out(bs, "Sc(s)[x]")
			if bs[0].CameraID != camSub {
				t.Fatalf("旧 id の Sc でない: %q", bs[0].CameraID)
			}
			h.wantEntries("x:H")
			h.wantA(camera, "cam1") // 健康・表示の計算では旧 s として数えない
			// 新 m を隠さない
			for _, b := range bs {
				if b.Purpose == PurposeHideCamera {
					t.Fatalf("新 m を隠す: %s", desc(b))
				}
			}
		}},
		{"HB-MP-03", func(t *testing.T) {
			h := newHB(t)
			h.pX(1000)
			m := editMap(h.mp, func(m *scenemap.Map) { setItem(m, camera, 2, false) })
			h.out(h.smc(1300, m), "")
			h.wantEntries("x:H")
		}},
		{"HB-MP-04", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.itemDone(1200, req, ItemOK)
			m := editMap(h.mp, func(m *scenemap.Map) { setItem(m, camera2, 1, false) })
			h.out(h.smc(2000, m), "")
			h.wantEntries("x:P")
		}},
		{"HB-MP-05", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.itemDone(1200, req, ItemTimeout)
			m := editMap(h.mp, func(m *scenemap.Map) { setItem(m, camera2, 1, false) })
			h.out(h.smc(20000, m), "")
			h.wantEntries("x:U")
		}},
		{"HB-MP-06", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.itemDone(1200, req, ItemTimeout)
			m := editMap(h.mp, func(m *scenemap.Map) { setItem(m, camera, 2, false) })
			h.out(h.smc(1500, m), "")
			h.wantEntries("x:H")
			h.wantMap(kx, false)
		}},
		{"HB-MP-07", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			m := editMap(h.mp, func(m *scenemap.Map) {
				m.Collection = "collection-y"
				setItem(m, camera, 2, true)
				setItem(m, deskScene, 1, true)
			})
			h.out(h.smc(1000, m), "")
			h.wantEntries("")
			h.wantRetained("x:H")
		}},
		{"HB-MP-08", func(t *testing.T) {
			h := newHB(t, hbNoConn())
			h.sIs(100, DOWN)
			h.ev(HiddenRestored{At: at(1), Entries: []HiddenEntry{h.entryOf(kx, HiddenH)}})
			h.ev(OBSConnected{At: at(400), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300),
				Map: &scenemap.Map{Collection: collectionX}})
			h.wantRetainedOrHidden("x:H")
			m := editMap(h.mp, func(m *scenemap.Map) {
				setItem(m, camera, 2, false)
				setItem(m, deskScene, 1, false)
			})
			h.out(h.smc(500, m), "")
			h.wantEntries("x:H")
			h.wantRetained("")
		}},
		{"HB-MP-09", func(t *testing.T) {
			h := newHB(t, hbAt(deskScene))
			h.seedH(ky)
			bs := h.sIs(1000, DOWN)
			h.out(bs, "F(D)")
			h.proceed2scene(bs[0], 1100)
			h.out(h.scene(1500, standby), "Hc(s)[x]") // S は監視対象でないので x を隠せる
			if st := h.m.Snapshot().Standby; st.Owner != OwnerAuto || st.ReturnScene != deskScene {
				t.Fatalf("戻り先: %+v", st)
			}
			m := editMap(h.mp, func(m *scenemap.Map) { setItem(m, camera2, 1, false) })
			h.out(h.smc(2000, m), "")
			if st := h.m.Snapshot().Standby; st.Owner != OwnerAuto || st.ReturnScene != deskScene {
				t.Fatalf("戻り先が消えた: %+v", st)
			}
			h.wantClass(deskScene, ClassMonitored)
		}},
	})
}

func (h *hb) wantRetainedOrHidden(want string) {
	h.t.Helper()
	got := hbEntryString(append(append([]HiddenEntry(nil), h.m.st.Hidden...), h.m.st.Retained...))
	if got != want {
		h.t.Fatalf("記録: got %q, want %q", got, want)
	}
}

// proceed2scene は SetScene の承認、応答、BatchDone までを流す（AutoFail 等）。
func (h *hb) proceed2scene(b Batch, ms int) {
	h.t.Helper()
	to := b.Actions[len(b.Actions)-1].(SetScene).SceneUUID
	r := h.m.Approve(b.BatchID, SetScene{SceneUUID: to}, OverrideNone{}, at(ms))
	p, ok := r.(Proceed)
	if !ok {
		h.t.Fatalf("Approve SetScene = %#v", r)
	}
	h.ev(SceneRequestDone{At: at(ms), ReqID: p.ReqID, Result: RequestOK})
	h.ev(BatchDone{At: at(ms), BatchID: b.BatchID, Result: BatchCompleted})
}

func TestHBSceneChangedの0b差分(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-SC-01", func(t *testing.T) {
			h := newHB(t, hbAt(deskScene))
			bs := h.sIs(1000, DOWN)
			h.out(bs, "F(D)")
			h.proceed2scene(bs[0], 1100)
			h.out(h.scene(1500, standby), "Hc(s)[x,y]")
			if st := h.m.Snapshot().Standby; st.Owner != OwnerAuto || st.ReturnScene != deskScene {
				t.Fatalf("auto/D でない: %+v", st)
			}
		}},
		{"HB-SC-02", func(t *testing.T) {
			// 表の前提は「S、s DOWN、owner=none」。S で s が DOWN になった時点の Hide は、5秒の再送の待ちで出ていない状態にする（判断として報告）。
			h := newHB(t, hbAt(standby))
			b := h.hide(1000)
			h.bdone(1100, b.BatchID, BatchRejected, nil)
			ep := h.m.st.SceneEpoch
			h.out(h.scene(6000, camera), "Hc(s)[x,y]")
			if h.m.st.SceneEpoch != ep+1 {
				t.Fatalf("SceneEpoch %d -> %d", ep, h.m.st.SceneEpoch)
			}
		}},
		{"HB-SC-03", func(t *testing.T) {
			h := newHB(t)
			h.hide(1000)
			ep := h.m.st.SceneEpoch
			h.out(h.scene(1500, deskScene), "F(D)")
			if h.m.st.SceneEpoch != ep+1 {
				t.Fatalf("SceneEpoch %d -> %d", ep, h.m.st.SceneEpoch)
			}
			h.wantQ("")
		}},
		{"HB-SC-04", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.scene(1500, deskScene)
			// x は D に関係がないので送れる。y は D を空にするので送らない。
			h.proceed(b, 1600, kx, false)
			h.want(h.ap(b, 1700, ky, false), ky, b, 1700, false, Reject{})
		}},
		{"HB-SC-05", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			bs := h.tick(1000)
			h.out(bs, "Sc(s)[R(is),x]")
			h.proceed(bs[0], 1100, kx, true)
			ep := h.m.st.SceneEpoch
			h.out(h.scene(1500, talk), "")
			if h.m.st.SceneEpoch != ep+1 {
				t.Fatalf("SceneEpoch %d -> %d", ep, h.m.st.SceneEpoch)
			}
			h.wantQ("x:true")
			h.wantEntries("x:H")
		}},
	})
}

func TestHB接続配信設定StudioMode(t *testing.T) {
	cfgS := func(h *hb, ms int, mods ...func(c *ScenesConfigured)) Event {
		c := h.cfg
		c.At = at(ms)
		c.Cameras = append([]CameraConfig(nil), h.cfg.Cameras...)
		for _, f := range mods {
			f(&c)
		}
		return c
	}
	runHB(t, []hbRow{
		{"OD-1/0a/0b", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			ce := h.m.st.ConnEpoch
			h.out(h.ev(OBSDisconnected{At: at(1500)}), "")
			if h.m.st.ConnEpoch != ce+1 {
				t.Fatalf("ConnEpoch %d -> %d", ce, h.m.st.ConnEpoch)
			}
			h.wantQ("")
			if _, ok := h.m.st.ItemReceipts[req]; !ok {
				t.Fatal("送信台帳が消えた")
			}
			h.wantEntries("x:P")
			if len(h.m.st.Pending) != 1 {
				t.Fatalf("Pending: %v", h.m.st.Pending)
			}
		}},
		{"HB-OC-01", func(t *testing.T) {
			h := newHB(t, hbStudioHide(StudioHideHide))
			_, req := h.pX(1000)
			h.itemDone(1100, req, ItemOK)
			h.ev(OBSDisconnected{At: at(1500)})
			bs := h.connect(2100, true, camera, true)
			h.out(bs, "")
			h.wantEntries("x:P")
			if !h.m.st.StudioMode {
				t.Fatal("StudioMode を導入しない")
			}
		}},
		{"HB-OC-02", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.ev(OBSDisconnected{At: at(500)})
			y := cloneTestMap(h.mp)
			y.Collection = "collection-y"
			setItem(y, camera, 2, true)
			bs := h.ev(OBSConnected{At: at(1000), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300), Map: y})
			h.out(bs, "")
			h.wantEntries("")
			h.wantRetained("x:H")
		}},
		{"HB-OC-03", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.ev(OBSDisconnected{At: at(500)})
			bs := h.ev(OBSConnected{At: at(1000), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300),
				Map: &scenemap.Map{Collection: collectionX}})
			h.out(bs, "")
			h.wantRetainedOrHidden("x:H")
		}},
		{"HB-OC-04", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx, ky)
			h.m.st.UserShown[camSub] = true
			h.sIs(300, DOWN)
			h.ev(OBSDisconnected{At: at(500)})
			setItem(h.mp, kx.ContainerUUID, kx.ItemID, false)
			bs := h.connect(1000, false, camera, false)
			h.out(bs, "Sc(s)[x,y]")
			h.wantU(camSub, false)
		}},
		{"SSF-1/0a/0b", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.sIs(500, DOWN)
			h.m.st.ItemExpectations = append(h.m.st.ItemExpectations, ItemExpectation{Key: scenemapKey(camera2, 1), Value: true, ReqID: 5, Purpose: PurposeRestoreAll})
			bs := h.ev(StreamStateChanged{At: at(1000), Streaming: false})
			h.out(bs, "Sc(s)[x]")
			h.wantQ("m2:true")
			h.noLastAuto()
		}},
		{"HB-ST-01", func(t *testing.T) {
			h := newHB(t, hbStart(opening))
			h.yOff()
			h.out(h.ev(StreamStateChanged{At: at(500), Streaming: false}), "")
			h.seedH(kx)
			h.sIs(600, OK)
			bs := h.ev(StreamStateChanged{At: at(1000), Streaming: true})
			h.out(bs, "St(O) Sc(s)[R(is),x]")
		}},
		{"HB-ST-02", func(t *testing.T) {
			h := newHB(t)
			h.m.st.UserShown[camSub] = true
			h.out(h.ev(StreamStateChanged{At: at(500), Streaming: false}), "")
			h.wantU(camSub, false)
			h.out(h.ev(StreamStateChanged{At: at(1000), Streaming: true}), "")
		}},
		{"SCF-11/0a/0b", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.sIs(500, DOWN)
			bs := h.ev(cfgS(h, 1000, func(c *ScenesConfigured) { c.AutoEnabled = false }))
			h.out(bs, "Sc(s)[x]")
		}},
		{"MC-CFG-2/0b", func(t *testing.T) {
			h := newHB(t, hbSubStandby())
			bs := h.sIs(1000, DOWN)
			h.out(bs, "F(A)")
			h.bdone(1100, bs[0].BatchID, BatchFailed, errTest)
			ce := h.m.st.ConfigEpoch
			bs = h.ev(cfgS(h, 7000, func(c *ScenesConfigured) { c.Cameras[1].OnBad = scenemap.MotionHide }))
			h.out(bs, "Hc(s)[x,y]")
			if h.m.st.ConfigEpoch != ce+1 {
				t.Fatalf("ConfigEpoch %d -> %d", ce, h.m.st.ConfigEpoch)
			}
			h.wantHealth(camera, OK)
		}},
		{"HB-CF-01", func(t *testing.T) {
			h := newHB(t)
			h.hiddenBoth(1000)
			bs := h.ev(cfgS(h, 7000, func(c *ScenesConfigured) { c.Cameras[1].OnBad = scenemap.MotionStandby }))
			h.out(bs, "F(A) Sc(s)[x,y]")
			h.wantHealth(camera, DOWN)
			h.wantV(camera, "cam1", "cam2")
		}},
		{"HB-CF-02", func(t *testing.T) {
			h := newHB(t)
			h.hiddenBoth(1000)
			h.m.st.UserShown[camSub] = true
			h.m.st.ItemFailures[camSub] = 2
			bs := h.ev(cfgS(h, 3200, func(c *ScenesConfigured) { c.Cameras = c.Cameras[:1] }))
			h.out(bs, "Sc(s)[x,y]")
			h.wantU(camSub, false)
			h.wantF(camSub, 0)
		}},
		{"HB-CF-03", func(t *testing.T) {
			h := newHB(t, hbStudio(), hbStudioHide(StudioHideIgnore))
			h.yOff()
			h.seedH(kx)
			h.out(h.sIs(500, DOWN), "")
			ce := h.m.st.ConfigEpoch
			bs := h.ev(cfgS(h, 1000, func(c *ScenesConfigured) { c.StudioModeHide = StudioHideStandby }))
			h.out(bs, "F(A) Sc(s)[x]")
			if h.m.st.ConfigEpoch != ce+1 {
				t.Fatalf("ConfigEpoch %d -> %d", ce, h.m.st.ConfigEpoch)
			}
			h.wantHealth(camera, DOWN)
		}},
		{"HB-SM-01", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.out(h.sIs(500, DOWN), "")
			h.out(h.ev(StudioModeChanged{At: at(1000), Enabled: true}), "F(A) Sc(s)[x]")
			h.wantHealth(camera, DOWN)
		}},
		{"HB-SM-02", func(t *testing.T) {
			h := newHB(t, hbStudioHide(StudioHideIgnore))
			h.yOff()
			h.seedH(kx)
			h.out(h.sIs(500, DOWN), "")
			h.out(h.ev(StudioModeChanged{At: at(1000), Enabled: true}), "")
			h.wantEntries("x:H")
			h.wantHealth(camera, OK)
		}},
		{"HB-SM-03", func(t *testing.T) {
			h := newHB(t, hbStudioHide(StudioHideHide))
			h.yOff()
			h.seedH(kx)
			h.out(h.sIs(500, DOWN), "")
			h.out(h.ev(StudioModeChanged{At: at(1000), Enabled: true}), "")
			h.wantEntries("x:H")
			h.wantHealth(camera, OK)
		}},
		{"HB-SM-04", func(t *testing.T) {
			h := newHB(t, hbStudio(), hbStudioHide(StudioHideIgnore))
			h.yOff()
			h.seedH(kx)
			bs := h.tick(1000)
			h.out(bs, "Sc(s)[R(is),x]")
			h.bdone(1100, bs[0].BatchID, BatchCompleted, errTest)
			h.out(h.ev(StudioModeChanged{At: at(7000), Enabled: false}), "Sc(s)[R(is),x]")
		}},
		// /0c: Studio off で A は健康 OK でも、表示中の悪い s が隠れるまで戻らない（逆向きの待ち。FB-CC-04）。
		// 0b の [R(A),Hc(s)] を [Hc(s)] にし、x の false を確かめたあとに R(A)。
		{"HB-SM-05/0c", func(t *testing.T) {
			h := newHB(t, hbStudio())
			bs := h.sIs(1000, DOWN)
			h.out(bs, "F(A)")
			h.proceed2scene(bs[0], 1100)
			h.scene(1500, standby)
			if st := h.m.Snapshot().Standby; st.Owner != OwnerAuto || st.ReturnScene != camera {
				t.Fatalf("auto/A でない: %+v", st)
			}
			bs = h.ev(StudioModeChanged{At: at(7000), Enabled: false})
			h.out(bs, "Hc(s)[x,y]")
			h.wantHealth(camera, OK)
			h.sendFalseOut(bs[0], 7100, kx, "R(A)") // x（A の s）が非表示になったとき、逆向きの待ちが解けて R
		}},
		{"HB-SM-06", func(t *testing.T) {
			h := newHB(t)
			h.ev(StudioModeChanged{At: at(500), Enabled: true})
			before := cloneState(h.m.st)
			h.out(h.ev(StudioModeChanged{At: at(1000), Enabled: true}), "")
			h.wantUnchanged(before)
		}},
	})
}

func TestHB照合と予約の補足(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-OC-05", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.ev(OBSDisconnected{At: at(500)})
			setItem(h.mp, kx.ContainerUUID, kx.ItemID, true)
			h.out(h.connect(1000, true, camera, false), "")
			h.wantEntries("")
			h.wantU(camSub, false)
		}},
		{"HB-OC-06", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.sIs(300, DOWN)
			h.ev(OBSDisconnected{At: at(500)})
			h.out(h.ev(Stopping{At: at(600)}), "")
			bs := h.connect(1000, true, camera, true)
			h.out(bs, "")
			h.wantEntries("x:H")
			if !h.m.st.StudioMode {
				t.Fatal("StudioMode を導入しない")
			}
		}},
		{"HB-MP-10", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false)
			h.wantEntries("x:P,y:P")
			m := editMap(h.mp, func(m *scenemap.Map) { setItem(m, deskScene, 1, false) })
			m2 := cloneTestMap(m)
			setItem(m2, camera, 2, true) // x は地図 true のまま（まだ適用されていない）
			h.out(h.smc(1200, m2), "")
			h.wantEntries("x:P,y:P")
			h.wantMap(ky, false)
		}},
		{"HB-IE-17", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false)
			h.out(h.itemEv(1200, ky, false), "")
			h.wantMap(ky, false)
			h.wantEntries("x:P,y:P")
			h.want(h.ap(b, 1300, ky, false), ky, b, 1300, false, Skip{})
			h.bdone(1400, b.BatchID, BatchCompleted, nil)
			h.wantEntries("x:P")
		}},
		{"HB-HR-07", func(t *testing.T) {
			h := newHB(t)
			bs := h.ev(HiddenRestored{At: at(500), Entries: []HiddenEntry{h.entryOf(kx, HiddenH)}})
			h.out(bs, "")
			h.wantEntries("")
			h.wantRetained("")
		}},
	})
}
