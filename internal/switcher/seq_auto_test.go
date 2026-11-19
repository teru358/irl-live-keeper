package switcher

import (
	"fmt"
	"testing"
)

func failA() Action { return SetScene{Role: RoleFail} }
func liveA() Action { return SetScene{Role: RoleLive} }

// autoFailApproved は t=1.0 に LOW → AutoFail#1 → 承認 → r1 まで進める。
func (q *seq) autoFailApproved() {
	q.t.Helper()
	q.batch(lc(1000, LOW), 1, PurposeAutoFail)
	q.approve(1, failA(), 1000, Proceed{ReqID: 1})
	q.expRoles(RoleFail, 1)
}

func TestSeqAutoRange(t *testing.T) {
	t.Run("SQ-01a", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.batch(lc(1000, LOW), 1, PurposeAutoFail)
		q.none(lc(2000, DOWN))
		q.none(lc(3000, OK))
	})
	t.Run("SQ-01b", func(t *testing.T) {
		q := newSeq(t).p("Fail", true, LOW)
		q.none(lc(1000, DOWN))
		q.none(lc(2000, LOW))
		q.batch(lc(3000, OK), 1, PurposeAutoRecover)
	})
	for _, scene := range []string{"Intro", "Pause", otherScene} {
		t.Run("SQ-01c/"+scene, func(t *testing.T) {
			q := newSeq(t).p(scene, true, OK)
			q.none(lc(1000, LOW))
			q.none(lc(2000, OK))
			q.none(tick(10000))
		})
	}
	t.Run("SQ-01d", func(t *testing.T) {
		q := newSeq(t).p("Live", false, OK)
		q.none(lc(1000, LOW))
		q.none(tick(10000))
	})
	t.Run("SQ-01e", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.none(disc(1000))
		q.none(lc(2000, LOW))
		q.none(tick(10000))
		q.batch(conn(11000, true, "Live", ip(300)), 1, PurposeAutoFail)
	})
}

func TestSeqRecoverOrder(t *testing.T) {
	t.Run("SQ-02", func(t *testing.T) {
		q := newSeq(t).p("Fail", true, LOW)
		se, _, _ := q.epochs()
		b := q.batch(lc(1000, OK), 1, PurposeAutoRecover)
		if _, ok := b.Actions[0].(RestartMedia); !ok {
			t.Fatalf("first action %#v, want RestartMedia", b.Actions[0])
		}
		q.pending(1)
		q.expRoles()
		q.approve(1, RestartMedia{Wait: W}, 1000, Proceed{ReqID: 0})
		q.expRoles()
		q.approve(1, liveA(), 1000+wMs, Proceed{ReqID: 1})
		q.expRoles(RoleLive, 1)
		q.none(srd(1100+wMs, 1, RequestOK))
		q.none(bd(1100+wMs, 1, BatchCompleted))
		q.deadline(1, 4400+wMs)
		q.pending()
		q.none(sc(1400+wMs, "Live"))
		q.role(RoleLive)
		q.expRoles()
		q.hold(false, 0)
		if s, _, _ := q.epochs(); s != se {
			t.Fatalf("SceneEpoch moved: %d → %d", se, s)
		}
	})
}

func TestSeqManualPause(t *testing.T) {
	t.Run("SQ-04/manual Pause", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		se, _, _ := q.epochs()
		q.none(sc(1000, "Pause"))
		q.pausePrev(RoleLive)
		q.hold(false, 0)
		if s, _, _ := q.epochs(); s != se+1 {
			t.Fatalf("SceneEpoch: got %d, want %d", s, se+1)
		}
		q.none(lc(2000, LOW))
		q.pausePrev(RoleLive)
	})
	t.Run("SQ-04/from Other", func(t *testing.T) {
		q := newSeq(t).p(otherScene, true, OK)
		q.none(sc(1000, "Pause"))
		q.pausePrev(RoleUnknown)
	})
}

func TestSeqRetryInterval(t *testing.T) {
	t.Run("SQ-05a", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.batch(lc(1000, LOW), 1, PurposeAutoFail)
		if got := q.m.st.LastAutoSent[PurposeAutoFail]; !got.Equal(at(1000)) {
			t.Fatalf("LastAutoSent: %s", show(got))
		}
		q.approve(1, failA(), 1000, Proceed{ReqID: 1})
		q.expRoles(RoleFail, 1)
		q.none(srd(1100, 1, RequestError))
		q.none(bd(1100, 1, BatchFailed))
		q.expRoles()
		q.pending()
		q.lastErr(true)
		q.ticks(2000, 5000, 1000)
		q.batch(tick(6000), 2, PurposeAutoFail)
		if got := q.m.st.LastAutoSent[PurposeAutoFail]; !got.Equal(at(6000)) {
			t.Fatalf("LastAutoSent: %s", show(got))
		}
		q.none(tick(7000))
	})
	t.Run("SQ-05b", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.autoFailApproved()
		se, _, _ := q.epochs()
		q.none(srd(6000, 1, RequestTimeout))
		q.none(bd(6000, 1, BatchFailed))
		q.expRoles(RoleFail, 1)
		q.deadline(1, 9300)
		q.ticks(7000, 9000, 1000)
		q.expRoles(RoleFail, 1)
		q.batch(tick(10000), 2, PurposeAutoFail)
		q.expRoles()
		if s, _, _ := q.epochs(); s != se {
			t.Fatalf("SceneEpoch moved")
		}
	})
	t.Run("SQ-05c", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.batch(lc(1000, LOW), 1, PurposeAutoFail)
		q.ticks(2000, 30000, 1000)
		q.pending(1)
	})
}

func TestSeqManualHold(t *testing.T) {
	t.Run("SQ-07", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.autoFailApproved()
		se, _, _ := q.epochs()
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.deadline(1, 4400)
		q.none(sc(1400, "Fail"))
		q.hold(false, 0)
		if s, _, _ := q.epochs(); s != se {
			t.Fatalf("SceneEpoch moved")
		}
		q.none(sc(3000, "Live"))
		q.hold(true, 3000)
		q.ticks(4000, 12000, 1000)
		q.hold(true, 3000)
		q.none(lc(13000, DOWN))
		q.hold(true, 3000)
		q.none(lc(14000, LOW))
		q.hold(true, 3000)
		q.none(lc(15000, OK))
		q.hold(false, 0)
		q.batch(lc(20000, LOW), 2, PurposeAutoFail)
	})
	t.Run("SQ-08", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.autoFailApproved()
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.deadline(1, 4400)
		q.ticks(2000, 4000, 1000)
		q.expRoles(RoleFail, 1)
		se, _, _ := q.epochs()
		q.none(tick(5000))
		q.expRoles()
		q.hold(false, 0)
		if s, _, _ := q.epochs(); s != se {
			t.Fatalf("SceneEpoch moved on expiry")
		}
		q.none(sc(5500, "Fail"))
		q.hold(true, 5500)
		if s, _, _ := q.epochs(); s != se+1 {
			t.Fatalf("SceneEpoch: got %d, want %d", s, se+1)
		}
		q.none(tick(6000))
	})
	t.Run("SQ-08/other scene", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.autoFailApproved()
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		se, _, _ := q.epochs()
		q.none(sc(1200, "Intro"))
		q.expRoles()
		q.hold(false, 0)
		if s, _, _ := q.epochs(); s != se+1 {
			t.Fatalf("SceneEpoch: got %d, want %d", s, se+1)
		}
	})
	heldFail := func(t *testing.T) *seq {
		q := newSeq(t).p("Live", true, OK)
		q.none(sc(1000, "Fail"))
		q.hold(true, 1000)
		return q
	}
	t.Run("SQ-09b", func(t *testing.T) {
		q := heldFail(t)
		_, _, st := q.epochs()
		q.none(ssc(2000, false))
		q.hold(false, 0)
		q.expRoles()
		if _, _, s := q.epochs(); s != st+1 {
			t.Fatalf("StreamEpoch: got %d, want %d", s, st+1)
		}
	})
	t.Run("SQ-09c", func(t *testing.T) {
		q := heldFail(t)
		_, ce, _ := q.epochs()
		q.none(disc(2000))
		q.none(conn(4000, true, "Fail", ip(300)))
		q.hold(true, 1000)
		if _, c, _ := q.epochs(); c != ce+1 {
			t.Fatalf("ConnEpoch: got %d, want %d", c, ce+1)
		}
	})
	t.Run("SQ-09d", func(t *testing.T) {
		q := heldFail(t)
		q.none(disc(2000))
		q.none(conn(4000, true, "Live", ip(300)))
		q.hold(false, 0)
	})
	t.Run("SQ-09e", func(t *testing.T) {
		q := heldFail(t)
		_, _, st := q.epochs()
		q.none(disc(2000))
		q.none(conn(4000, false, "Fail", ip(300)))
		q.hold(false, 0)
		if _, _, s := q.epochs(); s != st+1 {
			t.Fatalf("StreamEpoch: got %d, want %d", s, st+1)
		}
	})
}

func TestSeqExpectations(t *testing.T) {
	t.Run("SQ-10", func(t *testing.T) {
		q := newSeq(t).prep("Live", true, OK, ip(5000))
		q.batch(lc(1000, LOW), 1, PurposeAutoFail)
		q.expRoles()
		q.approve(1, failA(), 2000, Proceed{ReqID: 1})
		q.expRoles(RoleFail, 1)
		if got := q.m.st.Expectations[0].RegisteredAt; !got.Equal(at(2000)) {
			t.Fatalf("RegisteredAt: %s", show(got))
		}
		q.none(srd(2100, 1, RequestOK))
		q.none(bd(2100, 1, BatchCompleted))
		q.deadline(1, 10100)
		q.ticks(3000, 7000, 1000)
		q.expRoles(RoleFail, 1)
		se, _, _ := q.epochs()
		q.none(sc(7100, "Fail"))
		q.hold(false, 0)
		if s, _, _ := q.epochs(); s != se {
			t.Fatalf("SceneEpoch moved")
		}
	})
	t.Run("SQ-11", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.autoFailApproved()
		q.ticks(2000, 30000, 1000)
		q.expRoles(RoleFail, 1)
		se, _, _ := q.epochs()
		q.none(sc(30500, "Fail"))
		q.hold(false, 0)
		if s, _, _ := q.epochs(); s != se {
			t.Fatalf("SceneEpoch moved")
		}
	})
	t.Run("SQ-12", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.autoFailApproved()
		q.none(srd(6000, 1, RequestTimeout))
		q.none(bd(6000, 1, BatchFailed))
		q.expRoles(RoleFail, 1)
		q.deadline(1, 9300)
		se, _, _ := q.epochs()
		q.none(sc(7500, "Fail"))
		q.hold(false, 0)
		q.expRoles()
		if s, _, _ := q.epochs(); s != se {
			t.Fatalf("SceneEpoch moved")
		}
		q.ticks(8000, 12000, 1000)
	})
	t.Run("SQ-13", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.autoFailApproved()
		q.none(srd(1100, 1, RequestError))
		q.expRoles()
		se, _, _ := q.epochs()
		q.none(sc(1500, "Fail"))
		q.hold(true, 1500)
		if s, _, _ := q.epochs(); s != se+1 {
			t.Fatalf("SceneEpoch: got %d, want %d", s, se+1)
		}
	})
}

func TestSeqDuplicateGuard(t *testing.T) {
	t.Run("SQ-20", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.batch(lc(1000, LOW), 1, PurposeAutoFail)
		q.none(lc(2000, DOWN))
		q.approve(1, failA(), 2500, Proceed{ReqID: 1})
		q.none(srd(2600, 1, RequestOK))
		q.none(bd(2600, 1, BatchCompleted))
		q.deadline(1, 5900)
		q.none(lc(2700, LOW))
		q.none(sc(2900, "Fail"))
		q.hold(false, 0)
		if q.m.st.NextBatchID != 2 {
			t.Fatalf("batches emitted: %d, want 1", q.m.st.NextBatchID-1)
		}
	})
}

func TestSeqOverrideDeadline(t *testing.T) {
	cases := []struct {
		id string
		tr *int
		ov Override
		dl int
	}{
		{"a", ip(300), OverrideNone{}, 4400},
		{"b", ip(5000), OverrideNone{}, 9100},
		{"c", nil, OverrideNone{}, 11100},
		{"d", ip(300), OverrideKnown{Ms: 500}, 11100},
		{"e", ip(300), OverrideKnown{Ms: 8000}, 12100},
		{"f", nil, OverrideKnown{Ms: 8000}, 12100},
		{"g", nil, OverrideUnknown{}, 11100},
		{"h", ip(300), OverrideUnknown{}, 11100},
		{"i", ip(8000), OverrideUnknown{}, 12100},
	}
	for _, c := range cases {
		start := func(t *testing.T) *seq {
			q := newSeq(t).prep("Live", true, OK, c.tr)
			q.batch(lc(1000, LOW), 1, PurposeAutoFail)
			q.approveOv(1, failA(), c.ov, 1000, Proceed{ReqID: 1})
			q.none(srd(1100, 1, RequestOK))
			q.deadline(1, c.dl)
			return q
		}
		t.Run(fmt.Sprintf("SQ-22%s/before deadline", c.id), func(t *testing.T) {
			q := start(t)
			se, _, _ := q.epochs()
			q.none(sc(c.dl-100, "Fail"))
			q.hold(false, 0)
			if s, _, _ := q.epochs(); s != se {
				t.Fatalf("SceneEpoch moved")
			}
		})
		t.Run(fmt.Sprintf("SQ-22%s/after deadline", c.id), func(t *testing.T) {
			q := start(t)
			se, _, _ := q.epochs()
			q.none(tick(c.dl + 50))
			q.expRoles()
			q.none(sc(c.dl+100, "Fail"))
			q.hold(true, c.dl+100)
			if s, _, _ := q.epochs(); s != se+1 {
				t.Fatalf("SceneEpoch: got %d, want %d", s, se+1)
			}
		})
	}
}

func TestSeqStreamReconnecting(t *testing.T) {
	t.Run("SQ-24a", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.none(sc(1000, "Fail"))
		_, _, st := q.epochs()
		q.ticks(2000, 12000, 1000)
		q.hold(true, 1000)
		if !q.m.st.Streaming {
			t.Fatal("not streaming")
		}
		if _, _, s := q.epochs(); s != st {
			t.Fatalf("StreamEpoch moved")
		}
	})
	t.Run("SQ-24b", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.batch(lc(500, LOW), 1, PurposeAutoFail)
		q.none(sc(1000, "Pause"))
		q.pausePrev(RoleLive)
		_, _, st := q.epochs()
		q.ticks(2000, 12000, 1000)
		q.pausePrev(RoleLive)
		q.pending(1)
		if !q.m.st.Streaming {
			t.Fatal("not streaming")
		}
		if _, _, s := q.epochs(); s != st {
			t.Fatalf("StreamEpoch moved")
		}
	})
}

func TestSeqManualLiveOffAir(t *testing.T) {
	t.Run("SQ-25", func(t *testing.T) {
		q := newSeq(t).p("Intro", false, LOW)
		se, _, st := q.epochs()
		q.none(sc(1000, "Live"))
		q.hold(false, 0)
		if s, _, _ := q.epochs(); s != se+1 {
			t.Fatalf("SceneEpoch: got %d, want %d", s, se+1)
		}
		q.batch(ssc(2000, true), 1, PurposeAutoFail)
		if _, _, s := q.epochs(); s != st+1 || !q.m.st.Streaming {
			t.Fatalf("StreamEpoch %d (want %d), streaming %v", s, st+1, q.m.st.Streaming)
		}
	})
}
