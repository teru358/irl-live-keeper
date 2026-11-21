package switcher

import (
	"reflect"
	"testing"
)

// 0b の表 4.2（ItemEnabledChanged）、4.3（ItemRequestDone と項目の E）、4.3a（切断後の要求結果）の行。

// hxQNone は s DOWN、Hc(s)[x] 未完了、H(x)、Q なし（応答後2秒を過ぎた）の状態を作り、Hc を返す。時刻は 4000 まで進む。
func (h *hb) hxQNone() Batch {
	h.t.Helper()
	b, req := h.pX(1000)
	h.itemDone(1200, req, ItemOK)
	h.itemEv(1200, kx, false)
	h.out(h.tick(3300), "")
	h.wantQ("")
	h.wantEntries("x:H")
	return b
}

// sameState は Handle の前後で状態が変わらないことを確かめる。
func (h *hb) wantUnchanged(before state) {
	h.t.Helper()
	if d := diffState(h.m.st, before); d != "" {
		h.t.Fatalf("状態が変わった:%s", d)
	}
}

func TestHBItemEnabledChanged(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-IE-01", func(t *testing.T) {
			h := newHB(t)
			b, req := h.pX(1000)
			_ = b
			h.out(h.itemEv(1200, kx, false), "")
			h.wantEntries("x:H")
			q := h.q(kx, false)
			if q == nil || q.ReqID != req || !q.SawEvent {
				t.Fatalf("Q: %+v", q)
			}
			h.wantMap(kx, false)
		}},
		{"HB-IE-02", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.yOff()
			h.seedH(kx)
			b := h.sIs(1000, DOWN)
			h.out(b, "Sc(s)[x]")
			p := h.proceed(b[0], 1100, kx, true)
			h.out(h.itemEv(1200, kx, true), "")
			if q := h.q(kx, true); q == nil || q.ReqID != p.ItemReqID || !q.SawEvent {
				t.Fatalf("Q: %+v", q)
			}
			h.wantEntries("")
			h.wantMap(kx, true)
		}},
		{"HB-IE-03", func(t *testing.T) {
			h := newHB(t)
			h.hxQNone()
			h.out(h.itemEv(3400, kx, true), "")
			h.wantEntries("")
			h.wantU(camSub, true)
			h.wantMap(kx, true)
		}},
		{"HB-IE-04", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedH(kx)
			h.out(h.tick(1000), "Sc(s)[R(is),x]")
			h.out(h.itemEv(1200, kx, true), "")
			h.wantEntries("")
			h.wantU(camSub, false)
		}},
		{"HB-IE-05", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.out(h.ev(StreamStateChanged{At: at(500), Streaming: false}), "")
			h.seedH(kx)
			h.out(h.sIs(1000, DOWN), "Sc(s)[x]")
			h.out(h.itemEv(1200, kx, true), "")
			h.wantEntries("")
			h.wantU(camSub, false)
		}},
		{"HB-IE-06", func(t *testing.T) {
			h := newHB(t)
			h.hcX(1000)
			h.out(h.itemEv(1200, kx, false), "")
			h.wantEntries("")
			h.wantMap(kx, false)
		}},
		{"HB-IE-07", func(t *testing.T) {
			h := newHB(t)
			h.hcX(1000)
			h.itemEv(1100, kx, false)
			h.out(h.itemEv(1200, kx, true), "")
			h.wantEntries("")
			h.wantMap(kx, true)
		}},
		{"HB-IE-08", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.itemDone(1200, req, ItemTimeout)
			h.wantEntries("x:U")
			h.tick(3300)
			h.wantQ("")
			h.out(h.itemEv(3400, kx, false), "")
			h.wantEntries("x:H")
		}},
		{"HB-IE-09", func(t *testing.T) {
			h := newHB(t)
			h.yOff()
			h.seedU(kx)
			h.out(h.sIs(1000, DOWN), "")
			h.out(h.itemEv(1200, kx, true), "")
			h.wantEntries("x:U")
			h.wantU(camSub, true)
		}},
		{"HB-IE-10", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.out(h.itemEv(1200, kx, false), "")
			h.wantEntries("x:H")
			if q := h.q(kx, false); q == nil || !q.SawEvent || q.Responded {
				t.Fatalf("Q: %+v", q)
			}
			h.itemDone(1500, req, ItemOK)
			h.out(h.tick(3500), "")
			h.wantQ("x:false")
			h.tick(3501)
			h.wantQ("")
			h.wantEntries("x:H")
		}},
		{"HB-IE-11", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.yOff()
			h.seedH(kx)
			b := h.sIs(1000, DOWN)
			h.out(b, "Sc(s)[x]")
			p := h.proceed(b[0], 1100, kx, true)
			h.out(h.itemEv(1200, kx, true), "")
			h.wantEntries("")
			h.itemDone(1300, p.ItemReqID, ItemOK)
			h.wantEntries("")
		}},
		{"HB-IE-12", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.out(h.itemEv(1200, kx, true), "")
			h.wantU(camSub, true)
			h.wantEntries("x:P")
			h.wantQ("x:false")
			h.itemDone(1500, req, ItemOK)
			h.tick(3501)
			h.wantQ("")
			h.wantEntries("")
		}},
		{"MC-ITEM-9/0b", func(t *testing.T) {
			h := newHB(t)
			before := cloneState(h.m.st)
			h.out(h.itemEv(1000, kx, true), "")
			h.wantUnchanged(before)
		}},
		{"HB-IE-13", func(t *testing.T) {
			h := newHB(t)
			h.hcX(1000)
			h.seedU(kx)
			h.out(h.itemEv(1200, kx, false), "")
			h.wantEntries("x:H")
		}},
		{"HB-IE-14", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.yOff()
			h.seedH(kx)
			b := h.sIs(1000, DOWN)
			h.out(b, "Sc(s)[x]")
			h.proceed(b[0], 1100, kx, true)
			h.setMap(kx, true)
			h.out(h.itemEv(1200, kx, true), "")
			h.wantEntries("")
			if q := h.q(kx, true); q == nil || !q.SawEvent {
				t.Fatalf("Q: %+v", q)
			}
		}},
		{"MC-ITEM-10/0b", func(t *testing.T) {
			h := newHB(t)
			before := cloneState(h.m.st)
			h.out(h.itemEv(1000, scenemapKey("uuid-unknown", 9), false), "")
			h.wantUnchanged(before)
		}},
		{"HB-IE-15", func(t *testing.T) {
			h := newHB(t)
			h.hcX(1000)
			kz := scenemapKey("uuid-unknown", 9)
			h.m.st.ItemExpectations = append(h.m.st.ItemExpectations, ItemExpectation{Key: kz, Value: false, ReqID: 77, BatchID: 1, Purpose: PurposeHideCamera})
			h.out(h.itemEv(1200, kz, false), "")
			if q := h.q(kz, false); q == nil || !q.SawEvent {
				t.Fatalf("Q: %+v", q)
			}
			if _, ok := h.mapEnabled(kz); ok {
				t.Fatal("地図に項目を捏造した")
			}
		}},
		{"HB-IE-16", func(t *testing.T) {
			h := newHB(t, hbMapEdit(func(m *scenemapMap) {
				m.Groups["uuid-grp"] = scenemapContainer("Grp", scenemapItem(1, inSub, true))
				a := m.Scenes[camera]
				a.Items = append(a.Items, scenemapGroupItem(3, "uuid-grp", true))
				m.Scenes[camera] = a
			}))
			h.yOff()
			bs := h.sIs(1000, DOWN)
			if len(bs) != 1 || bs[0].Purpose != PurposeHideCamera {
				t.Fatalf("Hc がない: %s", descs(bs))
			}
			h.proceed(bs[0], 1100, kx, false)
			before := h.entries()
			grp := scenemapKey(camera, 3)
			h.out(h.itemEv(1300, grp, false), "")
			h.wantMap(grp, false)
			if h.entries() != before {
				t.Fatalf("子の一覧が変わった: %q", h.entries())
			}
		}},
	})
}

func TestHBItemRequestDone(t *testing.T) {
	runHB(t, []hbRow{
		{"HB-IR-01", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.out(h.itemDone(1500, req, ItemOK), "")
			e := h.entry(kx)
			if e == nil || e.State != HiddenP || e.SettledAt == nil || !e.SettledAt.Equal(at(1500)) {
				t.Fatalf("entry: %+v", e)
			}
			if q := h.q(kx, false); q == nil || !q.Responded || !q.Deadline.Equal(at(3500)) {
				t.Fatalf("Q: %+v", q)
			}
			h.wantMap(kx, true)
		}},
		{"HB-IR-02", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			rev := h.m.st.HiddenRevision
			h.out(h.itemDone(1500, req, ItemTimeout), "")
			e := h.entry(kx)
			if e == nil || e.State != HiddenU || e.SettledAt == nil || !e.SettledAt.Equal(at(1500)) {
				t.Fatalf("entry: %+v", e)
			}
			if q := h.q(kx, false); q == nil || !q.Deadline.Equal(at(3500)) {
				t.Fatalf("Q: %+v", q)
			}
			h.wantRev(true, rev)
		}},
		{"HB-IR-03", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.out(h.itemDone(1500, req, ItemError), "")
			h.wantQ("")
			h.wantEntries("")
			if r, ok := h.m.st.ItemReceipts[req]; !ok || r.Result != ItemError || !r.ResultAt.Equal(at(1500)) {
				t.Fatalf("台帳: %+v", r)
			}
		}},
		{"HB-IR-04", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.out(h.itemDone(1500, req, ItemNotFound), "")
			h.wantQ("")
			h.wantEntries("")
		}},
		{"HB-IR-05", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.yOff()
			h.seedH(kx)
			b := h.sIs(1000, DOWN)
			p := h.proceed(b[0], 1100, kx, true)
			h.out(h.itemDone(1500, p.ItemReqID, ItemOK), "")
			if q := h.q(kx, true); q == nil || !q.Deadline.Equal(at(3500)) {
				t.Fatalf("Q: %+v", q)
			}
			h.wantEntries("x:H")
		}},
		{"HB-IR-06", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.yOff()
			h.seedH(kx)
			b := h.sIs(1000, DOWN)
			p := h.proceed(b[0], 1100, kx, true)
			h.out(h.itemDone(1500, p.ItemReqID, ItemTimeout), "")
			if q := h.q(kx, true); q == nil || !q.Deadline.Equal(at(3500)) {
				t.Fatalf("Q: %+v", q)
			}
			h.wantEntries("x:H")
		}},
		{"HB-IR-07", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.yOff()
			h.seedH(kx)
			b := h.sIs(1000, DOWN)
			p := h.proceed(b[0], 1100, kx, true)
			h.out(h.itemDone(1500, p.ItemReqID, ItemError), "")
			h.wantQ("")
			h.wantEntries("x:H")
		}},
		{"HB-IR-08", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.itemEv(1200, kx, false)
			h.out(h.itemDone(1500, req, ItemOK), "")
			h.wantEntries("x:H")
			if q := h.q(kx, false); q == nil || !q.Deadline.Equal(at(3500)) {
				t.Fatalf("Q: %+v", q)
			}
			if r := h.m.st.ItemReceipts[req]; r.Result != ItemOK {
				t.Fatalf("台帳: %+v", r)
			}
		}},
		{"HB-IR-09", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.itemEv(1200, kx, false)
			h.out(h.itemDone(1500, req, ItemTimeout), "")
			h.wantEntries("x:H")
		}},
		{"HB-IR-10", func(t *testing.T) {
			h := newHB(t, hbAutoOff())
			h.yOff()
			h.seedH(kx)
			b := h.sIs(1000, DOWN)
			p := h.proceed(b[0], 1100, kx, true)
			h.itemEv(1200, kx, true)
			h.wantEntries("")
			h.out(h.itemDone(1500, p.ItemReqID, ItemError), "")
			h.wantEntries("")
			if r := h.m.st.ItemReceipts[p.ItemReqID]; r.Result != ItemError {
				t.Fatalf("台帳: %+v", r)
			}
		}},
		{"HB-IR-11", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			before := cloneState(h.m.st)
			h.out(h.itemDone(1500, 9999, ItemOK), "")
			h.wantUnchanged(before)
			h.itemDone(1600, req, ItemTimeout)
			after := cloneState(h.m.st)
			h.out(h.itemDone(1600, req, ItemTimeout), "")
			h.wantUnchanged(after)
		}},
	})
}

func TestHB期限と掃除(t *testing.T) {
	// 期限切れの行は、Tick と、E が先に動く入口（拒否される Command、未知のバッチの Approve）の3通りを同じ状態で確かめる。
	type ent struct {
		suffix string
		run    func(h *hb)
	}
	entries := []ent{
		{"", func(h *hb) { h.out(h.tick(4000), "") }},
		{"-C", func(h *hb) {
			h.m.st.StoppingReceived = true
			bs, err := h.evErr(Command{At: at(4000), Kind: CmdShowAllCameras})
			h.out(bs, "")
			h.wantReason(err, ReasonStopping)
		}},
		{"-A", func(h *hb) {
			if r := h.m.Approve(9999, h.itemAct(kx, false), OverrideNone{}, at(4000)); r != (Reject{}) {
				h.t.Fatalf("未知バッチ: %#v", r)
			}
		}},
	}
	// 期限切れの状態を作る関数（t=1500 に応答、期限 3500）と、E のあとの期待。
	type exRow struct {
		name  string
		opts  []hbOpt
		setup func(h *hb)
		check func(h *hb)
	}
	rows := []exRow{
		{"HB-EX-02", nil, func(h *hb) {
			_, req := h.pX(1000)
			h.itemDone(1500, req, ItemOK)
		}, func(h *hb) {
			h.wantMap(kx, false)
			h.wantEntries("x:H")
			h.wantQ("")
		}},
		{"HB-EX-03", []hbOpt{hbAutoOff()}, func(h *hb) {
			h.yOff()
			h.seedH(kx)
			b := h.sIs(1000, DOWN)
			p := h.proceed(b[0], 1100, kx, true)
			h.itemDone(1500, p.ItemReqID, ItemOK)
		}, func(h *hb) {
			h.wantMap(kx, true)
			h.wantEntries("")
			h.wantQ("")
		}},
		{"HB-EX-08", []hbOpt{hbNoY()}, func(h *hb) {
			h.yOff()
			h.seedH(kx)
			h.sIs(500, DOWN)
			bs := h.ev(Command{At: at(1000), Kind: CmdShowAllCameras})
			h.out(bs, "MA[x]")
			p := h.proceed(bs[0], 1100, kx, true)
			h.itemDone(1500, p.ItemReqID, ItemOK)
			h.bdone(1600, bs[0].BatchID, BatchCompleted, nil)
		}, func(h *hb) {
			h.wantMap(kx, false)
			h.wantEntries("x:H")
			h.wantQ("")
		}},
		{"HB-EX-04", nil, func(h *hb) {
			_, req := h.pX(1000)
			h.itemEv(1200, kx, false)
			h.itemDone(1500, req, ItemOK)
		}, func(h *hb) {
			h.wantMap(kx, false)
			h.wantEntries("x:H")
			h.wantQ("")
		}},
		{"HB-EX-05", nil, func(h *hb) {
			_, req := h.pX(1000)
			h.itemDone(1500, req, ItemTimeout)
		}, func(h *hb) {
			h.wantEntries("x:U")
			h.wantQ("")
			h.wantMap(kx, true)
		}},
	}
	for _, r := range rows {
		for _, e := range entries {
			t.Run(r.name+e.suffix, func(t *testing.T) {
				h := newHB(t, r.opts...)
				r.setup(h)
				e.run(h)
				r.check(h)
			})
		}
	}
	// HB-EX-06 は 10秒ちょうどは保持（接続し直して Q が消えた P）。-C / -A も同じ状態で。
	for _, e := range []ent{
		{"", func(h *hb) { h.out(h.tick(11201), "") }},
		{"-C", func(h *hb) {
			h.m.st.StoppingReceived = true
			bs, err := h.evErr(Command{At: at(11201), Kind: CmdShowAllCameras})
			h.out(bs, "")
			h.wantReason(err, ReasonStopping)
		}},
		{"-A", func(h *hb) {
			if r := h.m.Approve(9999, h.itemAct(kx, false), OverrideNone{}, at(11201)); r != (Reject{}) {
				h.t.Fatalf("未知バッチ: %#v", r)
			}
		}},
	} {
		t.Run("HB-EX-06"+e.suffix, func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.itemDone(1200, req, ItemOK)
			h.reconnect(1300, true)
			h.out(h.tick(11200), "")
			h.wantEntries("x:P")
			e.run(h)
			h.wantEntries("")
		})
	}
	runHB(t, []hbRow{
		{"HB-EX-01", func(t *testing.T) {
			h := newHB(t)
			_, req := h.pX(1000)
			h.itemDone(1500, req, ItemOK)
			h.out(h.tick(3500), "")
			h.wantQ("x:false")
			h.wantEntries("x:P")
		}},
		{"HB-EX-07", func(t *testing.T) {
			h := newHB(t)
			h.pX(1000)
			h.out(h.tick(60000), "")
			h.wantQ("x:false")
			h.wantEntries("x:P")
		}},
	})
}

func TestHB切断後の要求結果(t *testing.T) {
	setup := func(t *testing.T) (*hb, uint64) {
		h := newHB(t)
		_, req := h.pX(1000)
		h.ev(OBSDisconnected{At: at(1300)})
		h.wantQ("")
		return h, req
	}
	runHB(t, []hbRow{
		{"HB-IR-12", func(t *testing.T) {
			h, req := setup(t)
			h.out(h.itemDone(1400, req, ItemTimeout), "")
			e := h.entry(kx)
			if e == nil || e.State != HiddenU || e.SettledAt == nil || !e.SettledAt.Equal(at(1400)) {
				t.Fatalf("entry: %+v", e)
			}
			h.wantQ("")
		}},
		{"HB-IR-13", func(t *testing.T) {
			h, req := setup(t)
			h.out(h.itemDone(1400, req, ItemError), "")
			h.wantEntries("")
			if r := h.m.st.ItemReceipts[req]; r.Result != ItemError {
				t.Fatalf("台帳: %+v", r)
			}
			h.wantQ("")
		}},
		{"HB-IR-14", func(t *testing.T) {
			h, req := setup(t)
			h.out(h.itemDone(1400, req, ItemOK), "")
			e := h.entry(kx)
			if e == nil || e.State != HiddenP || e.SettledAt == nil || !e.SettledAt.Equal(at(1400)) {
				t.Fatalf("entry: %+v", e)
			}
			h.wantQ("")
			h.connect(2000, true, camera, false)
			h.tick(11400)
			h.wantEntries("x:P")
			h.tick(11401)
			h.wantEntries("")
		}},
		{"HB-IR-15", func(t *testing.T) {
			h, req := setup(t)
			h.out(h.itemDone(1400, req, ItemNotFound), "")
			h.wantEntries("")
			h.wantQ("")
		}},
	})
}

var _ = reflect.DeepEqual
