package switcher

import (
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 0b の表 3.2（健康）と 4.1（CamerasChanged と Tick）の行。行の名前を t.Run の名前にそのまま使う。

func runHB(t *testing.T, rows []hbRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("panic: %v", p)
				}
			}()
			r.f(t)
		})
	}
}

type hbRow struct {
	name string
	f    func(t *testing.T)
}

func TestHB健康(t *testing.T) {
	runHB(t, []hbRow{
		{"MC-H-7/0b", func(t *testing.T) {
			h := newHB(t)
			bs := h.sIs(1000, DOWN)
			h.out(bs, "Hc(s)[x,y]")
			h.wantClass(camera, ClassMonitored)
			h.wantHealth(camera, OK)
		}},
		{"HB-H-01", func(t *testing.T) {
			h := newHB(t, hbAt(deskScene))
			bs := h.sIs(1000, DOWN)
			h.out(bs, "F(D)")
			h.wantClass(deskScene, ClassMonitored)
			h.wantHealth(deskScene, DOWN)
		}},
		{"HB-H-02", func(t *testing.T) {
			h := newHB(t, hbAt(deskScene))
			h.m.st.ItemFailures[camSub] = 3
			bs := h.tick(1000)
			h.out(bs, "F(D)")
			h.wantHealth(deskScene, DOWN)
		}},
		{"HB-H-03", func(t *testing.T) {
			h := newHB(t)
			bs := h.cams(1000, LOW, OK)
			h.out(bs, "F(A)")
			h.wantClass(camera, ClassMonitored)
			h.wantHealth(camera, LOW)
		}},
		{"HB-H-04", func(t *testing.T) {
			// 表の出力欄は F(A) だけだが、HB-CC-07 と H-09 の決定は [F(A),Hc(m)]。食い違いとして報告し、後者に従う。
			h := newHB(t, hbBothHide())
			bs := h.cams(1000, DOWN, DOWN)
			h.out(bs, "F(A) Hc(m)[m1,m2]")
			h.wantClass(camera, ClassMonitored)
			h.wantHealth(camera, DOWN)
			if bs[0].BadCamera != camMain {
				t.Fatalf("代表: got %q, want m", bs[0].BadCamera)
			}
		}},
		{"HB-H-05", func(t *testing.T) {
			h := newHB(t, hbBothHide())
			bs := h.cams(1000, OK, DOWN)
			h.out(bs, "Hc(s)[x,y]")
			h.wantHealth(camera, OK)
		}},
		{"HB-H-06", func(t *testing.T) {
			h := newHB(t, hbStudioHide(StudioHideIgnore))
			h.out(h.ev(StudioModeChanged{At: at(500), Enabled: true}), "")
			bs := h.sIs(1000, DOWN)
			h.out(bs, "")
			h.wantClass(camera, ClassMonitored)
			h.wantHealth(camera, OK)
		}},
	})
}

func TestHBCamerasChangedとTick(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-CC-01", func(t *testing.T) {
			h := newHB(t)
			bs := h.sIs(1000, LOW)
			h.out(bs, "Hc(s)[x,y]")
			h.wantHealth(camera, OK)
			h.wantF(camSub, 0)
			h.wantSent(camSub, 1000)
		}},
		{"HB-CC-02", func(t *testing.T) {
			h := newHB(t)
			h.out(h.sIs(1000, LOW), "Hc(s)[x,y]")
			h.m.st.ItemFailures[camSub] = 2
			h.out(h.sIs(1500, DOWN), "")
			h.wantF(camSub, 0)
			h.wantSent(camSub, 1000)
			if len(h.m.st.Pending) != 1 {
				t.Fatalf("Pending: %d, want 1（重ねない）", len(h.m.st.Pending))
			}
		}},
		{"HB-CC-03", func(t *testing.T) {
			h := newHB(t)
			h.hiddenBoth(1000)
			h.m.st.UserShown[camSub] = true
			h.m.st.ItemFailures[camSub] = 2
			bs := h.sIs(7000, OK)
			h.out(bs, "Sc(s)[R(is),x,y]")
			h.wantU(camSub, false)
			h.wantF(camSub, 0)
			h.wantMap(kx, false)
			h.wantMap(ky, false)
		}},
		{"HB-CC-04", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.sendFalse(b, 1100, kx)
			h.sendFalse(b, 1100, ky)
			h.out(h.sIs(1500, OK), "")
			h.out(h.bdone(1600, b.BatchID, BatchCompleted, nil), "")
			h.out(h.tick(5999), "")
			h.out(h.tick(6000), "Sc(s)[R(is),x,y]")
		}},
		{"HB-CC-05", func(t *testing.T) {
			h := newHB(t)
			bs := h.cams(1000, LOW, DOWN)
			h.out(bs, "F(A) Hc(s)[x,y]")
			h.wantHealth(camera, LOW)
		}},
		{"HB-CC-06", func(t *testing.T) {
			h := newHB(t, hbAt(deskScene))
			h.out(h.sIs(1000, DOWN), "F(D)")
		}},
		{"HB-CC-07", func(t *testing.T) {
			h := newHB(t, hbBothHide())
			bs := h.cams(1000, DOWN, DOWN)
			h.out(bs, "F(A) Hc(m)[m1,m2]")
		}},
		{"HB-CC-08", func(t *testing.T) {
			h := newHB(t)
			h.ev(OBSDisconnected{At: at(500)})
			h.m.st.ItemFailures[camSub] = 2
			h.m.st.UserShown[camSub] = true
			h.out(h.sIs(1000, DOWN), "")
			if h.m.st.CameraStates[camSub] != DOWN {
				t.Fatal("状態が更新されない")
			}
			h.wantF(camSub, 0)
		}},
		{"HB-CC-09", func(t *testing.T) {
			h := newHB(t)
			h.out(h.ev(StreamStateChanged{At: at(500), Streaming: false}), "")
			h.out(h.sIs(600, LOW), "")
			h.seedH(kx, ky)
			bs := h.sIs(1000, DOWN)
			h.out(bs, "Sc(s)[x,y]")
		}},
		{"HB-CC-10", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.out(h.sIs(600, LOW), "")
			h.seedH(kx, ky)
			h.out(h.sIs(1000, DOWN), "Sc(s)[x,y]")
		}},
		{"HB-CC-11", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.bdone(1100, b.BatchID, BatchRejected, nil)
			h.out(h.sIs(7000, DOWN), "")
			h.out(h.tick(7000), "Hc(s)[x,y]")
		}},
		{"HB-TK-01", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.bdone(1100, b.BatchID, BatchRejected, nil)
			h.out(h.tick(5999), "")
		}},
		{"HB-TK-02", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.bdone(1100, b.BatchID, BatchRejected, nil)
			h.out(h.tick(6000), "Hc(s)[x,y]")
			h.wantSent(camSub, 6000)
		}},
		{"HB-TK-03", func(t *testing.T) {
			h := newHB(t)
			b := h.hcX(1000)
			h.proceed(b, 1100, kx, false)
			h.bdone(1200, b.BatchID, BatchRejected, nil)
			h.out(h.tick(6000), "")
			h.wantQ("x:false")
		}},
		{"HB-TK-04", func(t *testing.T) {
			h := newHB(t)
			h.seedH(kx, ky)
			h.m.st.ItemFailures[camSub] = 3
			h.out(h.tick(6000), "Sc(s)[R(is),x,y]")
			h.wantF(camSub, 3)
		}},
		{"HB-TK-05", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			b, req := h.pX(1000)
			h.itemDone(1200, req, ItemOK)
			h.reconnect(1300, true)
			h.out(h.tick(11200), "")
			h.wantEntries("x:P")
			_ = b
		}},
		{"HB-TK-06", func(t *testing.T) {
			h := newHB(t)
			b, req := h.pX(1000)
			h.itemDone(1200, req, ItemTimeout)
			h.bdone(1300, b.BatchID, BatchCompleted, errTest)
			h.sIs(1400, OK)
			h.out(h.tick(11201), "")
			h.wantEntries("x:U")
		}},
	})
}

var errTest = errString("要求の失敗")

type errString string

func (e errString) Error() string { return string(e) }

var _ = scenemap.StateOK
