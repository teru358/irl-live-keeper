package switcher

import (
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 0b の表 4.6（HiddenRestored、HiddenWriteFailed、Stopping、Command）の行。

func TestHBHiddenRestored(t *testing.T) {
	fresh := func(t *testing.T) *hb { return newHB(t, hbNoConn()) }
	other := func(h *hb, k scenemap.ItemKey, st HiddenState) HiddenEntry {
		e := h.entryOf(k, st)
		e.Collection = "collection-y"
		return e
	}
	runHB(t, []hbRow{
		{"HB-HR-01", func(t *testing.T) {
			h := fresh(t)
			in := []HiddenEntry{h.entryOf(kx, HiddenH), h.entryOf(ky, HiddenU)}
			h.out(h.ev(HiddenRestored{At: at(1), Entries: in}), "")
			h.wantRetained("x:H,y:U")
			h.wantEntries("")
			in[0].State = HiddenP // 外から変えても内部は変わらない
			h.wantRetained("x:H,y:U")
		}},
		{"HB-HR-02", func(t *testing.T) {
			h := fresh(t)
			h.yOff()
			setItem(h.mp, kx.ContainerUUID, kx.ItemID, false)
			h.sIs(10, DOWN)
			h.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{h.entryOf(kx, HiddenH)}})
			bs := h.connect(100, true, camera, false)
			h.out(bs, "")
			h.wantEntries("x:H")
			h.wantRetained("")
		}},
		{"HB-HR-03", func(t *testing.T) {
			h := fresh(t)
			h.yOff()
			h.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{h.entryOf(kx, HiddenH)}})
			rev := h.m.st.HiddenRevision
			bs := h.connect(100, true, camera, false)
			h.out(bs, "")
			h.wantEntries("")
			h.wantRetained("")
			h.wantRev(true, rev)
		}},
		{"HB-HR-04", func(t *testing.T) {
			h := fresh(t)
			h.yOff()
			setItem(h.mp, kx.ContainerUUID, kx.ItemID, false)
			h.sIs(10, DOWN)
			h.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{other(h, kx, HiddenH)}})
			bs := h.connect(100, true, camera, false)
			h.out(bs, "")
			h.wantRetained("x:H")
			h.wantEntries("")
			if n := len(h.m.Snapshot().RetainedEntries); n != 1 {
				t.Fatalf("Snapshot.RetainedEntries: %d, want 1", n)
			}
		}},
		{"HB-HR-05", func(t *testing.T) {
			h := fresh(t)
			h.yOff()
			h.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{h.entryOf(kx, HiddenU)}})
			h.out(h.connect(100, true, camera, false), "")
			h.wantEntries("x:U")
		}},
		{"HB-HR-06", func(t *testing.T) {
			h := fresh(t)
			h.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{h.entryOf(kx, HiddenH)}})
			h.out(h.ev(HiddenRestored{At: at(30), Entries: []HiddenEntry{h.entryOf(ky, HiddenH)}}), "")
			h.wantRetained("x:H")
		}},
	})
}

func TestHBHiddenWriteFailed(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-HW-01", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			p := h.proceed(b, 1100, kx, false)
			if !p.MustPersist {
				t.Fatal("MustPersist がない")
			}
			h.wantEntries("x:P,y:P")
			h.out(h.ev(HiddenWriteFailed{At: at(1150), BatchID: b.BatchID}), "")
			h.wantEntries("")
			h.wantQ("")
			h.want(h.ap(b, 1200, ky, false), ky, b, 1200, false, Reject{})
		}},
		{"HB-HW-02", func(t *testing.T) {
			h := newHB(t)
			b := h.hide(1000)
			h.proceed(b, 1100, kx, false)
			h.ev(HiddenWriteFailed{At: at(1150), BatchID: b.BatchID})
			h.wantErrContains("隠した項目を記録できないため、隠すのを止めました")
		}},
		{"HB-HW-03", func(t *testing.T) {
			h := newHB(t)
			b, _ := h.pX(1000)
			before := cloneState(h.m.st)
			h.out(h.ev(HiddenWriteFailed{At: at(1200), BatchID: 999}), "")
			h.wantUnchanged(before)
			h.bdone(1300, b.BatchID, BatchCompleted, nil)
			after := cloneState(h.m.st)
			h.out(h.ev(HiddenWriteFailed{At: at(1400), BatchID: b.BatchID}), "")
			h.wantUnchanged(after)
		}},
	})
}

func TestHBStoppingとCommand(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-SP-01", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			bs := h.ev(Stopping{At: at(1000)})
			h.out(bs, "RA[x]")
			if bs[0].RestoreScope != RestoreTrackedItems || bs[0].FromCommand {
				t.Fatalf("RA の識別: %+v", bs[0])
			}
			if !h.m.st.StoppingReceived || !h.m.st.RestoreIssued {
				t.Fatal("印が立たない")
			}
		}},
		{"HB-SP-02", func(t *testing.T) {
			h := newHB(t)
			h.ev(OBSDisconnected{At: at(500)})
			h.out(h.ev(Stopping{At: at(1000)}), "")
			if !h.m.st.StoppingReceived {
				t.Fatal("印が立たない")
			}
		}},
		{"HB-SP-03", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.out(h.ev(Stopping{At: at(1000)}), "RA[x]")
			h.out(h.ev(Stopping{At: at(1100)}), "")
		}},
		{"HB-SP-04", func(t *testing.T) {
			for _, conn := range []bool{true, false} {
				h := newHB(t)
				if !conn {
					h.ev(OBSDisconnected{At: at(500)})
				}
				h.ev(Stopping{At: at(1000)})
				for _, k := range []CommandKind{CmdStreamStart, CmdStreamStop, CmdStandbyOn, CmdStandbyOff, CmdShowAllCameras} {
					before := cloneState(h.m.st)
					bs, err := h.evErr(Command{At: at(1100), Kind: k})
					h.out(bs, "")
					h.wantReason(err, ReasonStopping)
					h.wantUnchanged(before)
				}
			}
		}},
		{"HB-SP-05", func(t *testing.T) {
			h := newHB(t)
			h.ev(Stopping{At: at(1000)})
			h.out(h.sIs(1100, DOWN), "")
			h.out(h.cams(1200, LOW, DOWN), "")
			h.out(h.tick(9000), "")
			h.out(h.ev(OBSDisconnected{At: at(9100)}), "")
			// 照合は続ける: 人の true で一覧から外れる
			h2 := newHB(t)
			h2.yOff()
			h2.seedH(kx)
			h2.ev(Stopping{At: at(1000)})
			h2.out(h2.itemEv(1100, kx, true), "")
			h2.wantEntries("")
		}},
		{"HB-SP-06", func(t *testing.T) {
			h := newHB(t)
			h.out(h.ev(Stopping{At: at(1000)}), "")
			if !h.m.st.StoppingReceived {
				t.Fatal("印が立たない")
			}
		}},
	})
}

func TestHB手動のすべて表示に戻す(t *testing.T) {
	cmdMA := func(h *hb, ms int) ([]Batch, error) { return h.evErr(Command{At: at(ms), Kind: CmdShowAllCameras}) }
	runHB(t, []hbRow{
		{"HB-MA-01", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.sIs(500, DOWN)
			bs, err := cmdMA(h, 1000)
			if err != nil {
				t.Fatal(err)
			}
			h.out(bs, "MA[x,y]")
			b := bs[0]
			if b.RestoreScope != RestoreAllCameraItems || !b.FromCommand || b.Purpose != PurposeRestoreAll {
				t.Fatalf("MA の識別: %+v", b)
			}
			h.wantEntries("x:H")
		}},
		{"HB-MA-02", func(t *testing.T) {
			h := newHB(t)
			h.ev(OBSDisconnected{At: at(500)})
			before := cloneState(h.m.st)
			bs, err := cmdMA(h, 1000)
			h.out(bs, "")
			if !hbErrIs(err, ErrNotConnected{}) {
				t.Fatalf("err = %v", err)
			}
			h.wantUnchanged(before)
		}},
		{"HB-MA-03", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			bs, _ := cmdMA(h, 1000)
			h.out(bs, "MA[x,y]")
			bs2, err := cmdMA(h, 1100)
			h.out(bs2, "")
			h.wantReason(err, ReasonSamePending)
		}},
		{"HB-MA-04", func(t *testing.T) {
			h := newHB(t)
			other := h.entryOf(kx, HiddenH)
			other.Collection = "collection-y"
			h.m.st.Retained = []HiddenEntry{other}
			bs, err := cmdMA(h, 1000)
			if err != nil {
				t.Fatal(err)
			}
			h.out(bs, "")
			h.wantRetained("x:H")
		}},
		{"HB-MA-05", func(t *testing.T) {
			h := newHB(t)
			h.setItemMapOnly(kx, false)
			h.m.st.Hidden = append(h.m.st.Hidden, h.entryOf(kx, HiddenH))
			other := h.entryOf(ky, HiddenH)
			other.Collection = "collection-y"
			h.m.st.Retained = []HiddenEntry{other}
			h.sIs(500, DOWN)
			bs, _ := cmdMA(h, 1000)
			h.out(bs, "MA[x]")
			h.wantRetained("y:H")
			h.wantEntries("x:H")
			h.sendTrue(bs[0], 1100, kx)
			h.wantEntries("")
			h.wantRetained("y:H")
		}},
		{"CMD-SOFF-5/0a/0b", func(t *testing.T) {
			h := newHB(t)
			h.standbyOnManual(t)
			h.yOff()
			h.seedH(kx)
			bs, err := h.evErr(Command{At: at(2000), Kind: CmdStandbyOff})
			if err != nil {
				t.Fatal(err)
			}
			h.out(bs, "")
			if st := h.m.Snapshot().Standby; st.Owner != OwnerAuto || st.ReturnScene != camera {
				t.Fatalf("auto/A でない: %+v", st)
			}
		}},
		{"HB-OF-01", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.standbyOnManual(t)
			h.yOff()
			h.seedH(kx)
			bs, err := h.evErr(Command{At: at(2000), Kind: CmdStandbyOff})
			if err != nil {
				t.Fatal(err)
			}
			h.out(bs, "M(A)")
		}},
		{"HB-MA-06", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.ev(OBSDisconnected{At: at(500)})
			h.ev(OBSConnected{At: at(600), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300),
				Map: &scenemap.Map{Collection: collectionX}})
			bs, err := cmdMA(h, 1000)
			if err != nil {
				t.Fatal(err)
			}
			h.out(bs, "")
			h.wantRetainedOrHidden("x:H")
		}},
	})
}

func (h *hb) setItemMapOnly(k scenemap.ItemKey, v bool) {
	h.t.Helper()
	setItem(h.mp, k.ContainerUUID, k.ItemID, v)
	h.itemEv(5, k, v)
}

// standbyOnManual は手動の待機（持ち主 manual、戻り先 A）にする。
func (h *hb) standbyOnManual(t *testing.T) {
	t.Helper()
	bs, err := h.evErr(Command{At: at(100), Kind: CmdStandbyOn})
	if err != nil || len(bs) != 1 {
		t.Fatalf("StandbyOn: %v %s", err, descs(bs))
	}
	h.proceed2scene(bs[0], 100)
	h.scene(500, standby)
	if st := h.m.Snapshot().Standby; st.Owner != OwnerManual || st.ReturnScene != camera {
		t.Fatalf("manual/A でない: %+v", st)
	}
}

var _ = time.Second

// 表8章: Snapshot と保存データの所有権。外から slice や要素を変えても内部は変わらない。
func TestHBSnapshotの所有権とRevision(t *testing.T) {
	h := newHB(t)
	h.yOff()
	h.seedH(kx)
	other := h.entryOf(ky, HiddenH)
	other.Collection = "collection-y"
	h.m.st.Retained = []HiddenEntry{other}
	s := h.m.Snapshot()
	if len(s.HiddenEntries) != 1 || len(s.RetainedEntries) != 1 {
		t.Fatalf("Snapshot: %+v", s)
	}
	s.HiddenEntries[0].State = HiddenP
	s.HiddenEntries[0].CameraID = "changed"
	s.RetainedEntries[0].State = HiddenP
	if got := h.entries(); got != "x:H" {
		t.Fatalf("Snapshot を変えたら内部が変わった: %q", got)
	}
	if got := h.retained(); got != "y:H" {
		t.Fatalf("Snapshot を変えたら Retained が変わった: %q", got)
	}
	// SettledAt のポインタも共有しない
	h = newHB(t)
	_, req := h.pX(1000)
	h.itemDone(1200, req, ItemOK)
	s = h.m.Snapshot()
	*s.HiddenEntries[0].SettledAt = at(99999)
	if e := h.entry(kx); e == nil || !e.SettledAt.Equal(at(1200)) {
		t.Fatalf("SettledAt を共有している: %+v", e)
	}
	// Revision は一覧・残した記録の内容が変わるたびに進み、読むだけでは進まない。
	r0 := h.m.Snapshot().HiddenRevision
	h.m.Snapshot()
	if h.m.Snapshot().HiddenRevision != r0 {
		t.Fatal("読むだけで Revision が進んだ")
	}
	h.itemEv(1300, kx, false)
	if h.m.Snapshot().HiddenRevision == r0 {
		t.Fatal("P→H で Revision が進まない")
	}
}
