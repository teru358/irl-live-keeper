package switcher

import (
	"fmt"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

var (
	introA = SetScene{Role: RoleIntro}
	pauseA = SetScene{Role: RolePause}
)

func (q *seq) wait(want *StartWait) {
	q.t.Helper()
	got := q.m.st.StartWait
	if (got == nil) != (want == nil) || (got != nil && *got != *want) {
		q.t.Fatalf("StartWait: got %s, want %s", show(got), show(want))
	}
}

func (q *seq) sceneEpoch(want int) {
	q.t.Helper()
	if s, _, _ := q.epochs(); s != want {
		q.t.Fatalf("SceneEpoch: got %d, want %d", s, want)
	}
}

func (q *seq) batchCount(want int) {
	q.t.Helper()
	if got := q.m.st.NextBatchID - 1; got != want {
		q.t.Fatalf("batches emitted: got %d, want %d", got, want)
	}
}

// pauseByCommand は PauseOn を最後まで実行し、本体の切替で Pause にする。
func (q *seq) pauseByCommand(id, req int) {
	q.t.Helper()
	q.batch(cmd(1000, CmdPauseOn), id, PurposePauseOn)
	q.approve(id, pauseA, 1000, Proceed{ReqID: req})
	q.none(srd(1100, req, RequestOK))
	q.none(bd(1100, id, BatchCompleted))
	q.none(sc(1400, "Pause"))
	q.role(RolePause)
}

func TestSeqPauseOffTarget(t *testing.T) {
	type tc struct {
		id   string
		prev Role
		l    ingest.LinkState
		want Purpose
	}
	var cases []tc
	for _, l := range links {
		cases = append(cases, tc{"a", RoleIntro, l, PurposePauseOffIntro})
	}
	cases = append(cases, tc{"b", RoleLive, OK, PurposePauseOffLive}, tc{"d", RoleFail, OK, PurposePauseOffLive},
		tc{"f", RoleUnknown, OK, PurposePauseOffLive})
	for _, l := range []ingest.LinkState{LOW, DOWN} {
		cases = append(cases, tc{"c", RoleLive, l, PurposePauseOffFail}, tc{"e", RoleFail, l, PurposePauseOffFail},
			tc{"g", RoleUnknown, l, PurposePauseOffFail})
	}
	for _, c := range cases {
		if c.prev != RoleUnknown {
			t.Run(fmt.Sprintf("SQ-03%s/%v", c.id, c.l), func(t *testing.T) {
				startLink := OK
				if c.prev == RoleFail {
					startLink = LOW
				}
				q := newSeq(t).p(sceneOf(c.prev), true, startLink)
				q.pauseByCommand(1, 1)
				q.pausePrev(c.prev)
				if c.l != startLink {
					q.none(lc(1500, c.l))
				}
				q.batch(cmd(2000, CmdPauseOff), 2, c.want)
			})
			continue
		}
		t.Run(fmt.Sprintf("SQ-03%s/from Other %v", c.id, c.l), func(t *testing.T) {
			q := newSeq(t).p(otherScene, true, OK)
			q.none(sc(1000, "Pause"))
			q.pausePrev(RoleUnknown)
			if c.l != OK {
				q.none(lc(1500, c.l))
			}
			q.batch(cmd(2000, CmdPauseOff), 1, c.want)
		})
		t.Run(fmt.Sprintf("SQ-03%s/connected in Pause %v", c.id, c.l), func(t *testing.T) {
			q := newSeq(t).p("Pause", true, c.l)
			q.pausePrev(RoleUnknown)
			q.batch(cmd(2000, CmdPauseOff), 1, c.want)
		})
	}
}

func TestSeqManualPauseOff(t *testing.T) {
	t.Run("SQ-04", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.none(sc(1000, "Pause"))
		q.pausePrev(RoleLive)
		q.hold(false, 0)
		q.none(lc(2000, LOW))
		q.batch(cmd(3000, CmdPauseOff), 1, PurposePauseOffFail)
		q.none(sc(4000, "Live"))
		q.pausePrev(RoleUnknown)
		q.hold(true, 4000)
	})
}

func TestSeqCommandConflicts(t *testing.T) {
	t.Run("SQ-06", func(t *testing.T) {
		q := newSeq(t)
		inv, nc := ErrInvalidState{}, ErrNotConnected{}
		for _, k := range []CommandKind{CmdStreamStart, CmdStreamStop, CmdGoLive, CmdPauseOn, CmdPauseOff} {
			q.cmdErr(cmd(0, k), nc)
		}
		q.none(lc(500, OK))
		q.none(conn(500, false, "Live", ip(300)))
		for _, k := range []CommandKind{CmdStreamStop, CmdGoLive, CmdPauseOn, CmdPauseOff} {
			q.cmdErr(cmd(1000, k), inv)
		}
		q.batch(cmd(2000, CmdStreamStart), 1, PurposeStartIntro)
		q.cmdErr(cmd(2100, CmdStreamStart), inv)
		q.approve(1, introA, 2200, Proceed{ReqID: 1})
		q.none(srd(2300, 1, RequestOK))
		q.none(bd(2300, 1, BatchCompleted))
		q.batch(sc(2400, "Intro"), 2, PurposeStartStream)
		q.approve(2, StartStream{}, 2500, Proceed{ReqID: 0})
		q.none(bd(2500, 2, BatchCompleted))
		q.none(ssc(2600, true))
		q.cmdErr(cmd(3000, CmdStreamStart), inv)
		q.cmdErr(cmd(3100, CmdPauseOff), inv)
		q.batch(cmd(3200, CmdGoLive), 3, PurposeGoLive)
		q.approve(3, RestartMedia{Wait: W}, 3300, Proceed{ReqID: 0})
		q.approve(3, liveA(), 3400, Proceed{ReqID: 2})
		q.none(srd(3500, 2, RequestOK))
		q.none(bd(3500, 3, BatchCompleted))
		q.none(sc(3900, "Live"))
		q.role(RoleLive)
		q.cmdErr(cmd(4000, CmdGoLive), inv)
		q.batch(cmd(4100, CmdPauseOn), 4, PurposePauseOn)
		q.approve(4, pauseA, 4200, Proceed{ReqID: 3})
		q.none(srd(4300, 3, RequestOK))
		q.none(bd(4300, 4, BatchCompleted))
		q.none(sc(4600, "Pause"))
		q.role(RolePause)
		q.cmdErr(cmd(5000, CmdPauseOn), inv)
		q.cmdErr(cmd(5000, CmdGoLive), inv)
		q.none(sc(5100, otherScene))
		q.role(RoleOther)
		for _, k := range []CommandKind{CmdPauseOn, CmdPauseOff, CmdGoLive} {
			q.cmdErr(cmd(5200, k), inv)
		}
		q.batch(cmd(5300, CmdStreamStop), 5, PurposeStopStream)
	})
}

func TestSeqHoldByCommand(t *testing.T) {
	heldFail := func(t *testing.T) *seq {
		q := newSeq(t).p("Live", true, OK)
		q.none(sc(1000, "Fail"))
		q.hold(true, 1000)
		return q
	}
	t.Run("SQ-09a", func(t *testing.T) {
		q := heldFail(t)
		q.batch(cmd(2000, CmdPauseOn), 1, PurposePauseOn)
		q.hold(false, 0)
	})
	t.Run("SQ-09f", func(t *testing.T) {
		q := heldFail(t)
		q.cmdErr(cmd(2000, CmdGoLive), ErrInvalidState{})
		q.hold(true, 1000)
	})
}

// failThenPause は Fail の直後に Pause を送り、期待 [Fail r1, Pause r2] まで進める。
func failThenPause(t *testing.T) *seq {
	q := newSeq(t).p("Live", true, OK)
	q.batch(lc(1000, LOW), 1, PurposeAutoFail)
	q.approve(1, failA(), 1000, Proceed{ReqID: 1})
	q.none(srd(1050, 1, RequestOK))
	q.none(bd(1050, 1, BatchCompleted))
	q.deadline(1, 4350)
	q.batch(cmd(1100, CmdPauseOn), 2, PurposePauseOn)
	q.approve(2, pauseA, 1100, Proceed{ReqID: 2})
	q.expRoles(RoleFail, 1, RolePause, 2)
	q.none(srd(1150, 2, RequestOK))
	q.none(bd(1150, 2, BatchCompleted))
	q.deadline(2, 4450)
	return q
}

func TestSeqInterruptedTransition(t *testing.T) {
	t.Run("SQ-14", func(t *testing.T) {
		q := failThenPause(t)
		se, _, _ := q.epochs()
		q.none(sc(1450, "Pause"))
		q.expRoles()
		q.pausePrev(RoleLive)
		q.sceneEpoch(se)
		q.hold(false, 0)
		q.ticks(2000, 10000, 1000)
	})
	t.Run("SQ-15", func(t *testing.T) {
		q := failThenPause(t)
		se, _, _ := q.epochs()
		q.none(sc(1300, "Intro"))
		q.expRoles()
		q.sceneEpoch(se + 1)
		q.hold(false, 0)
		q.none(sc(1450, "Pause"))
		q.sceneEpoch(se + 2)
		q.pausePrev(RoleIntro)
	})
}

func TestSeqRecoverWait(t *testing.T) {
	cases := []struct {
		id  string
		mid Event
		res ApproveResult
	}{
		{"a", lc(1000+wMs/2, LOW), Reject{}},
		{"b", sc(1000+wMs/2, "Pause"), Reject{}},
		{"c", ssc(1000+wMs/2, false), Reject{}},
		{"d", disc(1000 + wMs/2), Reject{}},
		{"e", nil, Proceed{ReqID: 1}},
	}
	for _, c := range cases {
		t.Run("SQ-16"+c.id, func(t *testing.T) {
			q := newSeq(t).p("Fail", true, LOW)
			q.batch(lc(1000, OK), 1, PurposeAutoRecover)
			q.approve(1, RestartMedia{Wait: W}, 1000, Proceed{ReqID: 0})
			if c.mid != nil {
				q.none(c.mid)
			}
			if c.id == "b" {
				q.pausePrev(RoleFail)
			}
			q.approve(1, liveA(), 1000+wMs, c.res)
			if c.id == "e" {
				q.expRoles(RoleLive, 1)
				return
			}
			q.none(bd(1000+wMs, 1, BatchRejected))
			q.lastErr(false)
			q.expRoles()
		})
	}
}

func TestSeqStopAndPauseIgnoreLink(t *testing.T) {
	t.Run("SQ-17", func(t *testing.T) {
		q := newSeq(t).p("Intro", true, OK)
		q.batch(cmd(1000, CmdStreamStop), 1, PurposeStopStream)
		q.batch(cmd(1100, CmdPauseOn), 2, PurposePauseOn)
		for i, l := range []ingest.LinkState{LOW, DOWN, LOW, DOWN, LOW} {
			q.none(lc(1200+100*i, l))
		}
		q.approve(1, StopStream{}, 2000, Proceed{ReqID: 0})
		q.none(bd(2100, 1, BatchCompleted))
		q.pending(2)
		if !q.m.st.Streaming {
			t.Fatal("streaming should continue until STOPPED")
		}
		q.approve(2, pauseA, 2200, Proceed{ReqID: 1})
		q.expRoles(RolePause, 1)
	})
}

func TestSeqReplace(t *testing.T) {
	t.Run("SQ-18", func(t *testing.T) {
		q := newSeq(t).p("Intro", true, OK)
		q.batch(cmd(1000, CmdGoLive), 1, PurposeGoLive)
		q.approve(1, RestartMedia{Wait: W}, 1000, Proceed{ReqID: 0})
		q.none(lc(1000+wMs/2, LOW))
		r := q.approve(1, liveA(), 1000+wMs, Replace{}).(Replace)
		q.checkBatch(r.Batch, 2, PurposeGoFail)
		if !q.m.st.Pending[1].Replaced {
			t.Fatal("#1 should be marked Replaced")
		}
		q.pending(1, 2)
		q.expRoles()
		q.none(bd(1000+wMs, 1, BatchRejected))
		q.lastErr(false)
		q.pending(2)
		q.none(lc(1100+wMs, OK))
		q.approve(2, failA(), 1200+wMs, Proceed{ReqID: 1})
		q.expRoles(RoleFail, 1)
		q.none(srd(1300+wMs, 1, RequestOK))
		q.none(bd(1300+wMs, 2, BatchCompleted))
		q.lastErr(false)
		q.batch(sc(1600+wMs, "Fail"), 3, PurposeAutoRecover)
	})
	t.Run("SQ-18/before RestartMedia", func(t *testing.T) {
		q := newSeq(t).p("Intro", true, OK)
		q.batch(cmd(1000, CmdGoLive), 1, PurposeGoLive)
		q.none(lc(1050, LOW))
		r := q.approve(1, RestartMedia{Wait: W}, 1100, Replace{}).(Replace)
		q.checkBatch(r.Batch, 2, PurposeGoFail)
	})
}

func TestSeqSkip(t *testing.T) {
	t.Run("SQ-19a", func(t *testing.T) {
		q := newSeq(t).p("Live", false, OK)
		q.batch(cmd(1000, CmdStreamStart), 1, PurposeStartIntro)
		q.approve(1, introA, 1000, Proceed{ReqID: 1})
		q.wait(&StartWait{ID: 1, Stage: StageWaitIntro})
		q.expRoles(RoleIntro, 1)
		q.none(srd(6000, 1, RequestTimeout))
		q.none(bd(6000, 1, BatchFailed))
		q.expRoles(RoleIntro, 1)
		q.deadline(1, 9300)
		q.wait(nil)
		q.lastErr(true)
		q.batch(cmd(6500, CmdStreamStart), 2, PurposeStartIntro)
		q.wait(&StartWait{ID: 2, Stage: StageWaitIntro})
		se, _, _ := q.epochs()
		q.batch(sc(7000, "Intro"), 3, PurposeStartStream)
		q.role(RoleIntro)
		q.sceneEpoch(se)
		q.wait(&StartWait{ID: 2, Stage: StageIntroConfirmed})
		q.approve(2, introA, 7100, Skip{})
		q.expRoles()
		q.none(bd(7100, 2, BatchCompleted))
		q.wait(&StartWait{ID: 2, Stage: StageIntroConfirmed})
		q.pending(3)
	})
	t.Run("SQ-19b", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.batch(cmd(1000, CmdPauseOn), 1, PurposePauseOn)
		q.approve(1, pauseA, 1000, Proceed{ReqID: 1})
		q.none(srd(6000, 1, RequestTimeout))
		q.none(bd(6000, 1, BatchFailed))
		q.expRoles(RolePause, 1)
		q.deadline(1, 9300)
		q.lastErr(true)
		q.batch(cmd(6500, CmdPauseOn), 2, PurposePauseOn)
		q.none(sc(7000, "Pause"))
		q.role(RolePause)
		q.pausePrev(RoleLive)
		q.approve(2, pauseA, 7100, Skip{})
		q.expRoles()
	})
}

func TestSeqStreamStart(t *testing.T) {
	startIntro := func(t *testing.T) *seq {
		q := newSeq(t).p("Live", false, OK)
		q.batch(cmd(1000, CmdStreamStart), 1, PurposeStartIntro)
		q.wait(&StartWait{ID: 1, Stage: StageWaitIntro})
		q.approve(1, introA, 1000, Proceed{ReqID: 1})
		q.expRoles(RoleIntro, 1)
		return q
	}
	t.Run("SQ-21a", func(t *testing.T) {
		q := startIntro(t)
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.wait(&StartWait{ID: 1, Stage: StageWaitIntro})
		q.cmdErr(cmd(1200, CmdStreamStart), ErrInvalidState{})
		se, _, st := q.epochs()
		q.batch(sc(1400, "Intro"), 2, PurposeStartStream)
		q.sceneEpoch(se)
		q.wait(&StartWait{ID: 1, Stage: StageIntroConfirmed})
		q.approve(2, StartStream{}, 1400, Proceed{ReqID: 0})
		q.none(bd(1500, 2, BatchCompleted))
		q.wait(nil)
		q.lastErr(false)
		q.none(ssc(1600, true))
		if _, _, s := q.epochs(); s != st+1 || !q.m.st.Streaming {
			t.Fatalf("StreamEpoch %d (want %d), streaming %v", s, st+1, q.m.st.Streaming)
		}
	})
	t.Run("SQ-21b", func(t *testing.T) {
		q := newSeq(t).p("Intro", false, OK)
		q.batch(cmd(1000, CmdStreamStart), 1, PurposeStartStream)
		q.wait(&StartWait{ID: 1, Stage: StageIntroConfirmed})
		q.approve(1, StartStream{}, 1000, Proceed{ReqID: 0})
	})
	t.Run("SQ-21c", func(t *testing.T) {
		q := startIntro(t)
		q.batch(sc(1300, "Intro"), 2, PurposeStartStream)
		q.expRoles()
		q.wait(&StartWait{ID: 1, Stage: StageIntroConfirmed})
		before := cloneState(q.m.st)
		q.none(srd(6000, 1, RequestTimeout))
		if d := diffState(q.m.st, before); d != "" {
			t.Fatalf("state changed:%s", d)
		}
		q.none(bd(6000, 1, BatchFailed))
		q.wait(&StartWait{ID: 1, Stage: StageIntroConfirmed})
		q.lastErr(false)
		q.approve(2, StartStream{}, 6000, Proceed{ReqID: 0})
		q.none(bd(6100, 2, BatchCompleted))
		q.none(ssc(6100, true))
		q.wait(nil)
		q.lastErr(false)
		if !q.m.st.Streaming {
			t.Fatal("not streaming")
		}
	})
	t.Run("SQ-21d", func(t *testing.T) {
		q := startIntro(t)
		q.none(srd(1100, 1, RequestError))
		q.expRoles()
		q.wait(nil)
		q.lastErr(true)
		q.none(bd(1100, 1, BatchFailed))
		q.batchCount(1)
		q.batch(cmd(2000, CmdStreamStart), 2, PurposeStartIntro)
	})
	t.Run("SQ-21e", func(t *testing.T) {
		q := newSeq(t).p("Live", false, OK)
		q.batch(cmd(1000, CmdStreamStart), 1, PurposeStartIntro)
		se, _, _ := q.epochs()
		q.none(sc(1200, "Fail"))
		q.hold(false, 0)
		q.wait(nil)
		q.lastErr(true)
		q.sceneEpoch(se + 1)
		q.approve(1, introA, 1300, Reject{})
		q.none(bd(1300, 1, BatchRejected))
		if e := q.m.Snapshot().LastError; e == nil || !e.At.Equal(at(1300)) {
			t.Fatalf("LastError: got %s, want a record at 1300ms", show(e))
		}
		q.batchCount(1)
		q.batch(cmd(2000, CmdStreamStart), 2, PurposeStartIntro)
	})
	t.Run("SQ-21f", func(t *testing.T) {
		q := startIntro(t)
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.deadline(1, 4400)
		q.wait(&StartWait{ID: 1, Stage: StageWaitIntro})
		q.ticks(2000, 4000, 1000)
		q.wait(&StartWait{ID: 1, Stage: StageWaitIntro})
		q.lastErr(false)
		q.none(tick(5000))
		q.expRoles()
		q.wait(nil)
		q.lastErr(true)
		se, _, _ := q.epochs()
		q.none(sc(5500, "Intro"))
		q.sceneEpoch(se + 1)
		q.hold(false, 0)
		q.batch(cmd(6000, CmdStreamStart), 2, PurposeStartStream)
		q.wait(&StartWait{ID: 2, Stage: StageIntroConfirmed})
	})
	t.Run("SQ-21g", func(t *testing.T) {
		q := startIntro(t)
		q.none(srd(6000, 1, RequestTimeout))
		q.deadline(1, 9300)
		q.none(bd(6000, 1, BatchFailed))
		q.wait(nil)
		q.lastErr(true)
		se, _, _ := q.epochs()
		q.none(sc(7000, "Intro"))
		q.sceneEpoch(se)
		q.expRoles()
		q.batchCount(1)
	})
}

func TestSeqStreamBoundary(t *testing.T) {
	t.Run("SQ-23", func(t *testing.T) {
		q := newSeq(t).p("Intro", true, OK)
		b := q.batch(cmd(1000, CmdStreamStop), 1, PurposeStopStream)
		e := b.StreamEpoch
		q.none(ssc(1500, false))
		if _, _, s := q.epochs(); s != e+1 {
			t.Fatalf("StreamEpoch %d, want %d", s, e+1)
		}
		q.none(ssc(2000, true))
		if _, _, s := q.epochs(); s != e+2 || !q.m.st.Streaming {
			t.Fatalf("StreamEpoch %d (want %d), streaming %v", s, e+2, q.m.st.Streaming)
		}
		q.approve(1, StopStream{}, 2500, Reject{})
		if !q.m.st.Streaming {
			t.Fatal("not streaming")
		}
		q.lastErr(false)
		q.none(bd(2500, 1, BatchRejected))
		q.lastErr(true)
	})
	t.Run("SQ-23/control", func(t *testing.T) {
		q := newSeq(t).p("Intro", true, OK)
		q.batch(cmd(1000, CmdStreamStop), 1, PurposeStopStream)
		q.approve(1, StopStream{}, 2500, Proceed{ReqID: 0})
	})
}

func TestSeqFailureInBatch(t *testing.T) {
	t.Run("SQ-26a", func(t *testing.T) {
		q := newSeq(t).p("Fail", true, LOW)
		q.batch(lc(1000, OK), 1, PurposeAutoRecover)
		q.approve(1, RestartMedia{Wait: W}, 1000, Proceed{ReqID: 0})
		q.approve(1, liveA(), 1000+wMs, Proceed{ReqID: 1})
		q.expRoles(RoleLive, 1)
		q.none(srd(1100+wMs, 1, RequestOK))
		q.none(BatchDone{At: at(1100 + wMs), BatchID: 1, Result: BatchCompleted, Err: fmt.Errorf("RESTART 失敗")})
		q.lastErr(true)
		q.pending()
	})
	t.Run("SQ-26b", func(t *testing.T) {
		q := newSeq(t).p("Intro", true, OK)
		q.batch(cmd(1000, CmdGoLive), 1, PurposeGoLive)
		q.approve(1, RestartMedia{Wait: W}, 1000, Proceed{ReqID: 0})
		q.approve(1, liveA(), 1000+wMs, Proceed{ReqID: 1})
		q.none(srd(1100+wMs, 1, RequestError))
		q.none(bd(1100+wMs, 1, BatchFailed))
		q.expRoles()
		q.pending()
		q.lastErr(true)
	})
}

func TestSeqPauseThenLinkDrop(t *testing.T) {
	t.Run("SQ-27", func(t *testing.T) {
		q := newSeq(t).p("Live", true, OK)
		q.batch(cmd(1000, CmdPauseOn), 1, PurposePauseOn)
		q.approve(1, pauseA, 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.none(lc(1200, LOW))
		q.role(RoleLive)
		q.expRoles(RolePause, 1)
		q.none(tick(2000))
		q.none(sc(1400+1000, "Pause"))
		q.pausePrev(RoleLive)
	})
}
