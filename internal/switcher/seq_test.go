package switcher

import (
	"errors"
	"reflect"
	"testing"
)

const (
	af  = PurposeAutoFail
	ar  = PurposeAutoRecover
	son = PurposeStandbyOn
	sfm = PurposeStandbyOffMonitored
	sfo = PurposeStandbyOffOther
	ssp = PurposeStartScene
	stt = PurposeStartStream
	stp = PurposeStopStream
)

func TestSeqAutoFailRange(t *testing.T) {
	t.Run("SQ-01a", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.none(lc(2000, DOWN))
		q.none(lc(3000, OK))
	})
	t.Run("SQ-01b", func(t *testing.T) {
		q := newSeq(t).P(talk, true, OK)
		q.none(lc(1000, LOW))
		q.none(lc(2000, OK))
		q.none(tick(10000))
	})
	t.Run("SQ-01c", func(t *testing.T) {
		q := newSeq(t).P(standby, true, OK)
		q.none(lc(1000, LOW))
		q.none(lc(2000, OK))
		q.none(tick(10000))
	})
	t.Run("SQ-01d", func(t *testing.T) {
		q := newSeq(t).P(camera, false, OK)
		q.none(lc(1000, LOW))
		q.none(tick(10000))
	})
	t.Run("SQ-01e", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.none(disc(1000))
		q.none(lc(2000, LOW))
		q.none(tick(10000))
		q.batch(conn(11000, true, camera, ip(300)), 1, af, standby, camera)
	})
	t.Run("SQ-01f", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK, withAuto(false))
		q.none(lc(1000, LOW))
		q.none(tick(10000))
	})
}

func TestSeqReturnScene(t *testing.T) {
	t.Run("SQ-02a", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		e := q.epochs()
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.owner(OwnerNone, "")
		q.pending(1)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.exps(standby, 1)
		q.owner(OwnerNone, "")
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.deadline(1, 4400)
		q.owner(OwnerNone, "")
		q.none(sc(1400, standby))
		q.owner(OwnerAuto, camera)
		q.exps()
		q.epochDelta(e, 0, 0, 0, 0)
	})
	t.Run("SQ-02b", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.none(lc(1500, OK))
		q.approve(1, set(standby), 1600, Reject{})
		q.none(bd(1600, 1, BatchRejected))
		q.owner(OwnerNone, "")
		q.noErr()
	})
	t.Run("SQ-02c", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestError))
		q.none(bd(1100, 1, BatchFailed))
		q.exps()
		q.owner(OwnerNone, "")
		q.errAt(1100)
	})
	t.Run("SQ-02d", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		e := q.epochs()
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.exps(standby, 1)
		q.none(srd(1700, 1, RequestOK))
		q.none(bd(1700, 1, BatchCompleted))
		q.deadline(1, 5000)
		q.ticks(2000, 4000, 1000)
		q.exps(standby, 1)
		q.none(tick(5000))
		q.exps(standby, 1)
		q.owner(OwnerNone, "")
		q.batch(tick(6000), 2, af, standby, camera)
		q.exps()
		q.owner(OwnerNone, "")
		q.epochDelta(e, 0, 0, 0, 0)
	})
}

func TestSeqAutoRecover(t *testing.T) {
	t.Run("SQ-03a", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		e := q.epochs()
		b := q.batch(lc(1000, OK), 2, ar, camera, "")
		if _, ok := b.Actions[0].(RestartMedia); !ok {
			t.Fatalf("first action %#v, want RestartMedia", b.Actions[0])
		}
		q.pending(2)
		q.exps()
		q.lastAuto(ar, 1000)
		q.approve(2, restart, 1000, Proceed{ReqID: 0})
		q.exps()
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.exps(camera, 2)
		q.owner(OwnerAuto, camera)
		q.none(srd(1600, 2, RequestOK))
		q.none(bd(1600, 2, BatchCompleted))
		q.deadline(2, 4900)
		q.owner(OwnerAuto, camera)
		q.none(sc(1900, camera))
		q.owner(OwnerNone, "")
		q.exps()
		q.epochDelta(e, 0, 0, 0, 0)
	})
	t.Run("SQ-03b", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.none(srd(1600, 2, RequestError))
		q.none(bd(1600, 2, BatchFailed))
		q.exps()
		q.owner(OwnerAuto, camera)
		q.errAt(1600)
		q.ticks(2000, 5000, 1000)
		q.batch(tick(6000), 3, ar, camera, "")
		q.lastAuto(ar, 6000)
	})
}

func TestSeqManualToMonitored(t *testing.T) {
	t.Run("SQ-04a", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		e := q.epochs()
		q.batch(sc(1000, camera2), 2, af, standby, camera2)
		q.epochDelta(e, 1, 0, 0, 0)
		q.owner(OwnerNone, "")
		q.approve(2, set(standby), 1000, Proceed{ReqID: 2})
		q.exps(standby, 2)
		q.none(srd(1100, 2, RequestOK))
		q.none(bd(1100, 2, BatchCompleted))
		q.none(sc(1400, standby))
		q.owner(OwnerAuto, camera2)
		q.batch(lc(2000, OK), 3, ar, camera2, "")
	})
	t.Run("SQ-04b", func(t *testing.T) {
		q := newSeq(t).P(talk, true, LOW)
		q.batch(sc(1000, camera), 1, af, standby, camera)
		q.owner(OwnerNone, "")
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.none(sc(1400, standby))
		q.owner(OwnerAuto, camera)
	})
	t.Run("SQ-04c", func(t *testing.T) {
		q := newSeq(t).P(talk, true, OK)
		e := q.epochs()
		q.none(sc(1000, camera))
		q.epochDelta(e, 1, 0, 0, 0)
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-04d", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.none(sc(1100, camera2))
		q.owner(OwnerNone, "")
		q.approve(2, restart, 1200, Reject{})
		q.none(bd(1200, 2, BatchRejected))
		q.noErr()
		q.scene(camera2)
	})
}

func TestSeqRepeatedSwitching(t *testing.T) {
	t.Run("SQ-05a", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		round := func(evMs, id int, ret string) {
			q.t.Helper()
			q.approve(id, set(standby), evMs, Proceed{ReqID: id})
			q.none(srd(evMs+100, id, RequestOK))
			q.none(bd(evMs+100, id, BatchCompleted))
			q.none(sc(evMs+400, standby))
			q.owner(OwnerAuto, ret)
		}
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		round(1000, 1, camera)
		q.batch(sc(3000, camera2), 2, af, standby, camera2)
		q.owner(OwnerNone, "")
		round(3000, 2, camera2)
		q.batch(sc(4000, camera), 3, af, standby, camera)
		q.owner(OwnerNone, "")
		round(4000, 3, camera)
		q.ticks(5000, 8000, 1000)
		q.batch(lc(9000, OK), 4, ar, camera, "")
	})
	t.Run("SQ-05b", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.lastAuto(af, 1000)
		e := q.epochs()
		q.none(sc(2000, camera2))
		q.epochDelta(e, 1, 0, 0, 0)
		q.approve(1, set(standby), 2500, Reject{})
		q.none(bd(2500, 1, BatchRejected))
		q.pending()
		q.noErr()
		q.batch(tick(3000), 2, af, standby, camera2)
		q.none(tick(6000))
	})
}

func TestSeqManualToOtherOrStandby(t *testing.T) {
	t.Run("SQ-06a", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.none(sc(1000, talk))
		q.owner(OwnerNone, "")
		q.none(lc(2000, OK))
		q.none(lc(3000, LOW))
		q.none(tick(10000))
	})
	t.Run("SQ-06b", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.none(sc(1000, standby))
		q.owner(OwnerNone, "")
		q.none(lc(2000, LOW))
		q.none(lc(3000, OK))
		q.none(tick(10000))
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-06c", func(t *testing.T) {
		q := newSeq(t).P(talk, true, LOW)
		e := q.epochs()
		q.none(sc(1000, opening))
		q.epochDelta(e, 1, 0, 0, 0)
	})
}

func TestSeqStandbyOn(t *testing.T) {
	t.Run("SQ-07a", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(cmd(1000, CmdStandbyOn), 1, son, standby, camera)
		q.owner(OwnerNone, "")
		q.none(lc(1100, LOW))
		q.approve(1, set(standby), 1200, Proceed{ReqID: 1})
		q.exps(standby, 1)
		q.none(srd(1300, 1, RequestOK))
		q.none(bd(1300, 1, BatchCompleted))
		q.deadline(1, 4600)
		q.none(tick(2000))
		q.none(sc(2200, standby))
		q.owner(OwnerManual, camera)
		q.none(lc(3000, OK))
		q.none(lc(4000, DOWN))
		q.none(tick(10000))
		q.owner(OwnerManual, camera)
	})
	t.Run("SQ-07b", func(t *testing.T) {
		q := newSeq(t).P(talk, true, LOW)
		q.batch(cmd(1000, CmdStandbyOn), 1, son, standby, talk)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.none(sc(1400, standby))
		q.owner(OwnerManual, talk)
		q.none(lc(2000, OK))
	})
}

func TestSeqStandbyOff(t *testing.T) {
	t.Run("SQ-08a", func(t *testing.T) {
		q := newSeq(t).PM(camera, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfm, camera, "")
		q.owner(OwnerManual, camera)
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.exps(camera, 2)
		q.none(srd(1600, 2, RequestOK))
		q.none(bd(1600, 2, BatchCompleted))
		q.owner(OwnerManual, camera)
		q.none(sc(1900, camera))
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-08b", func(t *testing.T) {
		q := newSeq(t).PM(camera, LOW)
		q.none(cmd(1000, CmdStandbyOff))
		q.owner(OwnerAuto, camera)
		q.scene(standby)
		q.none(tick(2000))
		q.batch(lc(3000, OK), 2, ar, camera, "")
		q.approve(2, restart, 3000, Proceed{})
		q.approve(2, set(camera), 3500, Proceed{ReqID: 2})
		q.none(srd(3600, 2, RequestOK))
		q.none(bd(3600, 2, BatchCompleted))
		q.none(sc(3900, camera))
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-08c", func(t *testing.T) {
		q := newSeq(t).PM(talk, LOW)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfo, talk, "")
		q.owner(OwnerManual, talk)
		q.none(lc(1100, DOWN))
		q.approve(2, set(talk), 1200, Proceed{ReqID: 2})
		q.none(srd(1300, 2, RequestOK))
		q.none(bd(1300, 2, BatchCompleted))
		q.none(sc(1600, talk))
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-08c/ok", func(t *testing.T) {
		q := newSeq(t).PM(talk, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfo, talk, "")
	})
	t.Run("SQ-08d", func(t *testing.T) {
		q := newSeq(t).P(camera, true, LOW, withAuto(false))
		q.standbyOnFrom(camera)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfm, camera, "")
		q.owner(OwnerManual, camera)
		q.approve(2, restart, 1000, Proceed{})
		q.none(lc(1200, DOWN))
		q.none(lc(1300, OK))
		q.none(lc(1400, LOW))
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.owner(OwnerManual, camera)
		q.none(srd(1600, 2, RequestOK))
		q.none(bd(1600, 2, BatchCompleted))
		q.none(sc(1900, camera))
		q.owner(OwnerNone, "")
		q.scene(camera)
	})
	t.Run("SQ-08d/other", func(t *testing.T) {
		q := newSeq(t).P(talk, true, LOW, withAuto(false))
		q.standbyOnFrom(talk)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfo, talk, "")
	})
	t.Run("SQ-08e", func(t *testing.T) {
		q := newSeq(t).PM(camera, OK)
		e := q.epochs()
		q.batch(cmd(1000, CmdStandbyOff), 2, sfm, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.none(srd(1700, 2, RequestOK))
		q.none(bd(1700, 2, BatchCompleted))
		q.deadline(2, 5000)
		q.noErr()
		q.none(tick(5000))
		q.exps(camera, 2)
		q.none(tick(6000))
		q.exps()
		q.owner(OwnerManual, camera)
		q.epochDelta(e, 0, 0, 0, 0)
		q.errAt(6000)
		q.batch(cmd(6500, CmdStandbyOff), 3, sfm, camera, "")
	})
}

func TestSeqReplace(t *testing.T) {
	t.Run("SQ-09a", func(t *testing.T) {
		q := newSeq(t).PM(camera, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfm, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.none(lc(1200, LOW))
		next := q.m.st.NextBatchID
		q.approve(2, set(camera), 1500, Replace{})
		q.owner(OwnerAuto, camera)
		q.scene(standby)
		q.exps()
		if q.m.st.NextBatchID != next {
			t.Fatalf("NextBatchID moved: %d → %d", next, q.m.st.NextBatchID)
		}
		q.none(bd(1500, 2, BatchRejected))
		q.noErr()
		q.pending()
		q.none(tick(2000))
		q.batch(lc(3000, OK), 3, ar, camera, "")
	})
	t.Run("SQ-09b", func(t *testing.T) {
		q := newSeq(t).PM(camera, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfm, camera, "")
		q.none(lc(1100, DOWN))
		q.approve(2, restart, 1200, Replace{})
		q.owner(OwnerAuto, camera)
		q.none(bd(1200, 2, BatchRejected))
		q.noErr()
	})
	t.Run("SQ-09c", func(t *testing.T) {
		q := newSeq(t).PM(camera, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfm, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.none(lc(1200, LOW))
		q.approve(2, set(camera), 1500, Replace{})
		q.none(lc(1600, OK))
		q.none(bd(1700, 2, BatchRejected))
		q.batch(tick(2000), 3, ar, camera, "")
	})
}

func TestSeqStartScene(t *testing.T) {
	t.Run("SQ-10a", func(t *testing.T) {
		q := newSeq(t).P(camera, false, LOW, withStart(opening))
		e := q.epochs()
		q.batch(ssc(1000, true), 1, ssp, opening, "")
		q.epochDelta(e, 0, 0, 1, 0)
		q.none(lc(1100, DOWN))
		q.approve(1, set(opening), 1300, Proceed{ReqID: 1})
		q.exps(opening, 1)
		q.none(srd(1400, 1, RequestOK))
		q.none(bd(1400, 1, BatchCompleted))
		q.deadline(1, 4700)
		q.none(tick(2000))
		q.none(sc(2200, opening))
		q.owner(OwnerNone, "")
		q.exps()
	})
	t.Run("SQ-10b1", func(t *testing.T) {
		q := newSeq(t).P(talk, false, LOW, withStart(camera))
		q.batch(ssc(1000, true), 1, ssp, camera, "")
		q.approve(1, set(camera), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.batch(sc(1400, camera), 2, af, standby, camera)
		q.owner(OwnerNone, "")
		q.approve(2, set(standby), 1400, Proceed{ReqID: 2})
		q.none(srd(1500, 2, RequestOK))
		q.none(bd(1500, 2, BatchCompleted))
		q.none(sc(1800, standby))
		q.owner(OwnerAuto, camera)
	})
	t.Run("SQ-10b2", func(t *testing.T) {
		q := newSeq(t).P(talk, false, LOW, withStart(camera))
		e := q.epochs()
		q.batch(ssc(1000, true), 1, ssp, camera, "")
		q.approve(1, set(camera), 1000, Proceed{ReqID: 1})
		q.none(sc(1300, camera))
		q.epochDelta(e, 0, 0, 1, 0)
		q.none(srd(1400, 1, RequestOK))
		q.none(bd(1400, 1, BatchCompleted))
		q.pending()
		q.batch(tick(2000), 2, af, standby, camera)
	})
	t.Run("SQ-10c", func(t *testing.T) {
		q := newSeq(t).P(camera, false, LOW)
		q.batch(ssc(1000, true), 1, af, standby, camera)
	})
	t.Run("SQ-10c/other", func(t *testing.T) {
		q := newSeq(t).P(talk, false, LOW)
		q.none(ssc(1000, true))
	})
	t.Run("SQ-10d", func(t *testing.T) {
		q := newSeq(t).P(camera, false, OK, withStart(opening))
		q.batch(ssc(1000, true), 1, ssp, opening, "")
		e := q.epochs()
		q.none(sc(1100, talk))
		q.epochDelta(e, 1, 0, 0, 0)
		q.approve(1, set(opening), 1200, Reject{})
		q.none(bd(1200, 1, BatchRejected))
		q.noErr()
		q.ticks(2000, 20000, 1000)
	})
	t.Run("SQ-10e", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK, withStart(opening))
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.none(ssc(1500, false))
		q.noLastAuto()
		q.batch(ssc(2000, true), 2, ssp, opening, "")
		q.approve(1, set(standby), 2100, Reject{})
		q.none(bd(2100, 1, BatchRejected))
		q.noErr()
		q.approve(2, set(opening), 2200, Proceed{ReqID: 1})
		q.none(srd(2300, 1, RequestOK))
		q.none(bd(2300, 2, BatchCompleted))
		q.none(sc(2600, opening))
	})
}

func TestSeqStartSceneNoRetry(t *testing.T) {
	start := func(t *testing.T) *seq {
		q := newSeq(t).P(camera, false, OK, withStart(opening))
		q.batch(ssc(1000, true), 1, ssp, opening, "")
		return q
	}
	t.Run("SQ-11a", func(t *testing.T) {
		q := start(t)
		q.approve(1, set(opening), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestError))
		q.none(bd(1100, 1, BatchFailed))
		q.exps()
		q.errAt(1100)
		q.ticks(2000, 20000, 1000)
	})
	t.Run("SQ-11b", func(t *testing.T) {
		q := start(t)
		q.none(ssc(1100, false))
		q.approve(1, set(opening), 1200, Reject{})
		q.none(bd(1200, 1, BatchRejected))
		q.errAt(1200)
		q.ticks(2000, 20000, 1000)
	})
	t.Run("SQ-11b/config", func(t *testing.T) {
		q := start(t)
		e := q.epochs()
		q.none(cfg(1100, withStart(opening), withNoop()))
		q.epochDelta(e, 0, 0, 0, 1)
		q.approve(1, set(opening), 1200, Reject{})
		q.none(bd(1200, 1, BatchRejected))
		q.errAt(1200)
		q.ticks(2000, 20000, 1000)
	})
	t.Run("SQ-11c", func(t *testing.T) {
		q := start(t)
		e := q.epochs()
		q.approve(1, set(opening), 1000, Proceed{ReqID: 1})
		q.exps(opening, 1)
		q.none(srd(1700, 1, RequestOK))
		q.none(bd(1700, 1, BatchCompleted))
		q.deadline(1, 5000)
		q.noErr()
		q.none(tick(5000))
		q.exps(opening, 1)
		q.none(tick(6000))
		q.exps()
		q.errAt(6000)
		q.epochDelta(e, 0, 0, 0, 0)
		q.ticks(7000, 20000, 1000)
	})
	t.Run("SQ-11d", func(t *testing.T) {
		q := start(t)
		q.none(sc(1100, talk))
		q.approve(1, set(opening), 1200, Reject{})
		q.none(bd(1200, 1, BatchRejected))
		q.noErr()
	})
}

func TestSeqAutoDisabled(t *testing.T) {
	t.Run("SQ-12a", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		e := q.epochs()
		q.none(cfg(1100, withAuto(false)))
		q.epochDelta(e, 0, 0, 0, 1)
		q.pending(1)
		q.approve(1, set(standby), 1200, Reject{})
		q.none(bd(1200, 1, BatchRejected))
		q.noErr()
		q.ticks(2000, 5000, 1000)
		q.none(lc(5500, DOWN))
		q.ticks(6000, 10000, 1000)
		q.batch(cmd(11000, CmdStandbyOn), 2, son, standby, camera)
	})
	t.Run("SQ-12b", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.none(cfg(1000, withAuto(false)))
		q.owner(OwnerNone, "")
		q.scene(standby)
		q.none(lc(2000, OK))
		e := q.epochs()
		q.none(cfg(3000))
		q.epochDelta(e, 0, 0, 0, 1)
	})
	t.Run("SQ-12c", func(t *testing.T) {
		q := newSeq(t).P(camera, true, LOW, withAuto(false))
		q.none(tick(1000))
		q.batch(cfg(2000), 1, af, standby, camera)
	})
}

func TestSeqConfigReclassifies(t *testing.T) {
	t.Run("SQ-13a", func(t *testing.T) {
		q := newSeq(t).P(talk, true, LOW)
		q.none(tick(1000))
		q.batch(smc(2000, camera, camera2, talk), 1, af, standby, talk)
	})
	t.Run("SQ-13b", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.none(smc(1000, camera2))
		q.none(lc(2000, LOW))
		q.none(tick(10000))
	})
}

func TestSeqConfigAndOwner(t *testing.T) {
	camera3 := withNoop() // 旧 Camera3 追加は、挙動に影響しない設定変更（withNoop）に置き換える（台帳。0b で hide が挙動を持つため hide から変更）
	t.Run("SQ-14a", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		e := q.epochs()
		q.none(cfg(1000, withStandby(standby2)))
		q.epochDelta(e, 0, 0, 0, 1)
		q.owner(OwnerNone, "")
		q.scene(standby)
		q.none(lc(2000, OK))
	})
	t.Run("SQ-14b", func(t *testing.T) {
		q := newSeq(t).PM(camera, OK)
		q.none(cfg(1000, withStandby(standby2)))
		q.owner(OwnerNone, "")
		q.scene(standby)
		q.cmdErr(cmd(2000, CmdStandbyOff), invalid)
	})
	t.Run("SQ-14c", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.none(smc(1000, camera2))
		q.owner(OwnerNone, "")
		q.scene(standby)
		q.none(lc(2000, OK))
	})
	t.Run("SQ-14d", func(t *testing.T) {
		q := newSeq(t).PM(camera, LOW)
		q.none(smc(1000, camera2))
		q.owner(OwnerManual, camera)
		q.batch(cmd(2000, CmdStandbyOff), 2, sfo, camera, "")
	})
	t.Run("SQ-14e", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.none(cfg(1000, camera3))
		q.owner(OwnerAuto, camera)
		q.batch(lc(2000, OK), 2, ar, camera, "")
	})
	t.Run("SQ-14f", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		before := cloneState(q.m.st)
		q.none(cfg(1000))
		if d := diffState(q.m.st, before); d != "" {
			t.Fatalf("state changed:%s", d)
		}
	})
	t.Run("SQ-14g", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(cmd(1000, CmdStandbyOn), 1, son, standby, camera)
		q.batch(cmd(1100, CmdStreamStop), 2, stp, "", "")
		e := q.epochs()
		q.none(cfg(1200, camera3))
		q.epochDelta(e, 0, 0, 0, 1)
		q.pending(1, 2)
		q.approve(1, set(standby), 1300, Reject{})
		q.none(bd(1300, 1, BatchRejected))
		q.errAt(1300)
		q.approve(2, StopStream{}, 1400, Proceed{})
	})
	t.Run("SQ-14h1", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.none(cfg(1100, camera3))
		q.approve(1, set(standby), 1200, Reject{})
		q.none(bd(1200, 1, BatchRejected))
		q.noErr()
		q.ticks(2000, 5000, 1000)
		q.batch(tick(6000), 2, af, standby, camera)
	})
	t.Run("SQ-14h2", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.none(cfg(1100, camera3))
		q.owner(OwnerAuto, camera)
		q.approve(2, restart, 1200, Reject{})
		q.none(bd(1200, 2, BatchRejected))
		q.noErr()
		q.batch(tick(6000), 3, ar, camera, "")
	})
	t.Run("SQ-14h3", func(t *testing.T) {
		q := newSeq(t).PM(camera, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfm, camera, "")
		q.none(cfg(1100, camera3))
		q.approve(2, restart, 1200, Reject{})
		q.none(bd(1200, 2, BatchRejected))
		q.errAt(1200)
		q.owner(OwnerManual, camera)
		q.batch(cmd(2000, CmdStandbyOff), 3, sfm, camera, "")
	})
	t.Run("SQ-14h4", func(t *testing.T) {
		q := newSeq(t).P(camera, false, OK, withStart(opening))
		q.batch(ssc(1000, true), 1, ssp, opening, "")
		q.none(cfg(1100, withStart(opening), camera3))
		q.approve(1, set(opening), 1200, Reject{})
		q.none(bd(1200, 1, BatchRejected))
		q.errAt(1200)
	})
	t.Run("SQ-14h5", func(t *testing.T) {
		q := newSeq(t).P(camera, false, OK)
		q.batch(cmd(1000, CmdStreamStart), 1, stt, "", "")
		q.none(cfg(1100, camera3))
		q.approve(1, StartStream{}, 1200, Proceed{})
	})
}

func TestSeqExpectationConfigEpoch(t *testing.T) {
	sent := func(t *testing.T) *seq {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.deadline(1, 4400)
		return q
	}
	t.Run("SQ-15a", func(t *testing.T) {
		q := sent(t)
		e := q.epochs()
		q.batch(cfg(1200, withStandby(standby2)), 2, af, standby2, camera)
		q.epochDelta(e, 0, 0, 0, 1)
		q.exps(standby, 1)
		q.owner(OwnerNone, "")
		q.none(sc(1400, standby))
		q.epochDelta(e, 0, 0, 0, 1)
		q.owner(OwnerNone, "")
		q.exps()
		q.approve(2, set(standby2), 1500, Reject{})
		q.none(bd(1500, 2, BatchRejected))
		q.noErr()
		q.none(lc(2000, OK))
		q.scene(standby)
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-15b", func(t *testing.T) {
		q := sent(t)
		e := q.epochs()
		q.none(cfg(1200, withAuto(false)))
		q.exps(standby, 1)
		q.none(sc(1400, standby))
		q.epochDelta(e, 0, 0, 0, 1)
		q.owner(OwnerNone, "")
		q.none(lc(2000, OK))
	})
	t.Run("SQ-15c", func(t *testing.T) {
		q := sent(t)
		e := q.epochs()
		q.none(smc(1200, camera2)) // 分類の変更は地図の入口。ConfigEpoch は進まない
		q.none(sc(1400, standby))
		q.epochDelta(e, 0, 0, 0, 0)
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-15d", func(t *testing.T) {
		q := sent(t)
		e := q.epochs()
		q.none(cfg(1200, withNoop()))
		q.exps(standby, 1)
		q.none(sc(1400, standby))
		q.epochDelta(e, 0, 0, 0, 1)
		q.owner(OwnerNone, "")
		q.none(lc(2000, OK))
	})
	t.Run("SQ-15e", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.exps(camera, 2)
		q.none(srd(1600, 2, RequestOK))
		q.none(bd(1600, 2, BatchCompleted))
		e := q.epochs()
		q.none(cfg(1700, withNoop()))
		q.epochDelta(e, 0, 0, 0, 1)
		q.owner(OwnerAuto, camera)
		q.none(sc(1900, camera))
		q.owner(OwnerNone, "")
		q.batch(lc(3000, LOW), 3, af, standby, camera)
	})
}

func TestSeqStreamStop(t *testing.T) {
	stopped := func(t *testing.T) *seq {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.exps(camera, 2)
		e := q.epochs()
		q.none(ssc(1600, false))
		q.epochDelta(e, 0, 0, 1, 0)
		q.owner(OwnerNone, "")
		q.exps()
		q.noLastAuto()
		q.pending(2)
		return q
	}
	t.Run("SQ-16a", func(t *testing.T) {
		q := stopped(t)
		q.none(srd(1700, 2, RequestOK))
		q.none(bd(1700, 2, BatchCompleted))
		q.exps()
		q.pending()
		q.none(tick(2000))
	})
	t.Run("SQ-16b", func(t *testing.T) {
		q := stopped(t)
		q.none(srd(1700, 2, RequestOK))
		q.none(bd(1700, 2, BatchCompleted))
		q.none(lc(1800, LOW))
		e := q.epochs()
		q.none(ssc(1900, true))
		q.epochDelta(e, 0, 0, 1, 0)
		q.batch(sc(2000, camera), 3, af, standby, camera)
		q.epochDelta(e, 1, 0, 1, 0)
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-16c", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.none(ssc(1200, false))
		q.none(lc(1300, OK))
		q.none(ssc(1400, true))
		e := q.epochs()
		q.none(sc(1500, standby))
		q.epochDelta(e, 1, 0, 0, 0)
		q.owner(OwnerNone, "")
	})
}

func TestSeqCommandsConflict(t *testing.T) {
	all4 := []CommandKind{CmdStreamStart, CmdStreamStop, CmdStandbyOn, CmdStandbyOff}
	t.Run("SQ-17", func(t *testing.T) {
		q := newSeq(t)
		q.none(cfg(0))
		for _, k := range all4 {
			q.cmdErr(cmd(0, k), notConnected)
		}
		q.none(lc(500, OK))
		q.none(conn(500, false, camera, ip(300)))
		for _, k := range []CommandKind{CmdStreamStop, CmdStandbyOn, CmdStandbyOff} {
			q.cmdErr(cmd(1000, k), invalid)
		}
		q.batch(cmd(2000, CmdStreamStart), 1, stt, "", "")
		q.cmdErr(cmd(2100, CmdStreamStart), invalid)
		q.approve(1, StartStream{}, 2200, Proceed{})
		q.none(ssc(2300, true))
		q.none(bd(2400, 1, BatchCompleted))
		q.cmdErr(cmd(3000, CmdStreamStart), invalid)
		q.cmdErr(cmd(3100, CmdStandbyOff), invalid)
		q.batch(cmd(3200, CmdStandbyOn), 2, son, standby, camera)
		q.cmdErr(cmd(3300, CmdStandbyOn), invalid)
		q.cmdErr(cmd(3400, CmdStandbyOff), invalid)
		q.approve(2, set(standby), 3500, Proceed{ReqID: 1})
		q.none(srd(3600, 1, RequestOK))
		q.none(bd(3600, 2, BatchCompleted))
		q.none(sc(3900, standby))
		q.owner(OwnerManual, camera)
		q.cmdErr(cmd(4000, CmdStandbyOn), invalid)
		q.batch(cmd(4100, CmdStandbyOff), 3, sfm, camera, "")
		q.cmdErr(cmd(4200, CmdStandbyOff), invalid)
		q.approve(3, restart, 4300, Proceed{})
		q.approve(3, set(camera), 4800, Proceed{ReqID: 2})
		q.none(srd(4900, 2, RequestOK))
		q.none(bd(4900, 3, BatchCompleted))
		q.none(sc(5200, camera))
		q.owner(OwnerNone, "")
		q.cmdErr(cmd(5500, CmdStandbyOff), invalid)
		q.batch(cmd(6000, CmdStreamStop), 4, stp, "", "")
		q.cmdErr(cmd(6100, CmdStreamStop), invalid)
		q.approve(4, StopStream{}, 6200, Proceed{})
		q.none(ssc(6300, false))
		q.none(bd(6400, 4, BatchCompleted))
		q.none(disc(7000))
		for _, k := range all4 {
			q.cmdErr(cmd(7000, k), notConnected)
		}
	})
	t.Run("SQ-17/auto_owner", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.cmdErr(cmd(1000, CmdStandbyOff), invalid)
	})
}

func TestSeqStandbyOnAcceptance(t *testing.T) {
	t.Run("SQ-18a", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.cmdErr(cmd(1000, CmdStandbyOn), invalid)
	})
	t.Run("SQ-18b", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.none(sc(1000, standby))
		q.cmdErr(cmd(1100, CmdStandbyOn), invalid)
	})
	t.Run("SQ-18c", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.cmdErr(cmd(1100, CmdStandbyOn), invalid)
	})
	t.Run("SQ-18d", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		e := q.epochs()
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.exps(standby, 1)
		q.none(srd(6000, 1, RequestTimeout))
		q.none(bd(6000, 1, BatchFailed))
		q.deadline(1, 9300)
		q.pending()
		q.errAt(6000)
		q.cmdErr(cmd(7000, CmdStandbyOn), invalid)
		q.none(lc(8000, OK))
		q.cmdErr(cmd(9300, CmdStandbyOn), invalid)
		q.batch(cmd(9400, CmdStandbyOn), 2, son, standby, camera)
		q.exps()
		q.epochDelta(e, 0, 0, 0, 0)
	})
	t.Run("SQ-18e", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.none(sc(1600, camera))
		q.owner(OwnerNone, "")
		q.scene(camera)
		q.cmdErr(cmd(1700, CmdStandbyOn), invalid)
		q.none(srd(1800, 2, RequestOK))
		q.none(bd(1800, 2, BatchCompleted))
		q.pending()
		q.batch(cmd(1900, CmdStandbyOn), 3, son, standby, camera)
	})
	t.Run("SQ-18f", func(t *testing.T) {
		q := newSeq(t).P(camera, false, OK, withStart(opening))
		q.batch(ssc(1000, true), 1, ssp, opening, "")
		q.cmdErr(cmd(1100, CmdStandbyOn), invalid)
		q.approve(1, set(opening), 1200, Proceed{ReqID: 1})
		q.none(srd(1300, 1, RequestOK))
		q.none(bd(1300, 1, BatchCompleted))
		q.cmdErr(cmd(1400, CmdStandbyOn), invalid)
		q.none(sc(1600, opening))
		q.batch(cmd(1700, CmdStandbyOn), 2, son, standby, opening)
	})
	t.Run("SQ-18g", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(cmd(1000, CmdStandbyOn), 1, son, standby, camera)
		e := q.epochs()
		q.none(sc(1100, standby))
		q.epochDelta(e, 1, 0, 0, 0)
		q.owner(OwnerNone, "")
		q.approve(1, set(standby), 1200, Reject{})
		q.none(bd(1200, 1, BatchRejected))
		q.errAt(1200)
		q.owner(OwnerNone, "")
	})
}

func TestSeqRecoverThenDrop(t *testing.T) {
	run := func(t *testing.T, doneFirst bool) {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.exps(camera, 2)
		q.none(lc(1600, DOWN))
		if doneFirst {
			q.none(srd(1700, 2, RequestOK))
			q.none(bd(1700, 2, BatchCompleted))
		}
		e := q.epochs()
		q.batch(sc(1900, camera), 3, af, standby, camera)
		q.epochDelta(e, 0, 0, 0, 0)
		q.owner(OwnerNone, "")
		if !doneFirst {
			q.none(srd(1900, 2, RequestOK))
			q.none(bd(1900, 2, BatchCompleted))
		}
		q.approve(3, set(standby), 1900, Proceed{ReqID: 3})
		q.none(srd(2000, 3, RequestOK))
		q.none(bd(2000, 3, BatchCompleted))
		q.none(sc(2300, standby))
		q.owner(OwnerAuto, camera)
	}
	t.Run("SQ-19", func(t *testing.T) { run(t, true) })
	t.Run("SQ-19/scene_first", func(t *testing.T) { run(t, false) })
}

func TestSeqMultipleExpectations(t *testing.T) {
	two := func(t *testing.T) *seq {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.batch(cfg(1200, withStandby(standby2)), 2, af, standby2, camera)
		q.approve(2, set(standby2), 1200, Proceed{ReqID: 2})
		q.none(srd(1300, 2, RequestOK))
		q.none(bd(1300, 2, BatchCompleted))
		q.exps(standby, 1, standby2, 2)
		return q
	}
	t.Run("SQ-20a", func(t *testing.T) {
		q := two(t)
		e := q.epochs()
		q.none(sc(1600, standby2))
		q.exps()
		q.owner(OwnerAuto, camera)
		q.epochDelta(e, 0, 0, 0, 0)
	})
	t.Run("SQ-20b", func(t *testing.T) {
		q := two(t)
		q.none(sc(1600, standby))
		q.exps(standby2, 2)
		q.owner(OwnerNone, "")
		q.none(sc(1900, standby2))
		q.owner(OwnerAuto, camera)
	})
	t.Run("SQ-20c", func(t *testing.T) {
		q := newSeq(t).PM(talk, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfo, talk, "")
		q.approve(2, set(talk), 1000, Proceed{ReqID: 2})
		q.exps(talk, 2)
		q.none(srd(6000, 2, RequestTimeout))
		q.none(bd(6000, 2, BatchFailed))
		q.deadline(2, 9300)
		q.errAt(6000)
		q.owner(OwnerManual, talk)
		q.batch(cmd(6500, CmdStandbyOff), 3, sfo, talk, "")
		q.approve(3, set(talk), 6500, Proceed{ReqID: 3})
		q.exps(talk, 2, talk, 3)
		q.none(srd(6600, 3, RequestOK))
		q.none(bd(6600, 3, BatchCompleted))
		q.deadline(3, 9900)
		q.none(sc(7000, talk))
		q.owner(OwnerNone, "")
		q.exps()
		q.batch(cmd(7100, CmdStandbyOn), 4, son, standby, talk)
	})
}

func TestSeqExpectationLifetime(t *testing.T) {
	t.Run("SQ-21a", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.none(tc(900, ip(5000)))
		e := q.epochs()
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1000, 1, RequestOK))
		q.none(bd(1000, 1, BatchCompleted))
		q.deadline(1, 9000)
		q.none(tc(1200, ip(300)))
		q.deadline(1, 9000)
		q.ticks(2000, 8000, 1000)
		q.exps(standby, 1)
		q.none(tick(9000))
		q.exps(standby, 1)
		q.batch(tick(10000), 2, af, standby, camera)
		q.exps()
		q.epochDelta(e, 0, 0, 0, 0)
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-21a/arrives_late", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.none(tc(900, ip(5000)))
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1000, 1, RequestOK))
		q.none(bd(1000, 1, BatchCompleted))
		e := q.epochs()
		q.none(sc(8500, standby))
		q.epochDelta(e, 0, 0, 0, 0)
		q.owner(OwnerAuto, camera)
	})
	t.Run("SQ-21b", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.ticks(2000, 60000, 1000)
		q.exps(standby, 1)
		q.none(sc(60500, standby))
		q.owner(OwnerAuto, camera)
	})
	t.Run("SQ-21c", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(6000, 1, RequestTimeout))
		q.none(bd(6000, 1, BatchFailed))
		q.exps(standby, 1)
		q.deadline(1, 9300)
		q.errAt(6000)
		q.none(tick(7000))
		e := q.epochs()
		q.none(sc(8000, standby))
		q.epochDelta(e, 0, 0, 0, 0)
		q.owner(OwnerAuto, camera)
	})
	t.Run("SQ-21d", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestError))
		q.none(bd(1100, 1, BatchFailed))
		e := q.epochs()
		q.none(sc(1200, standby))
		q.epochDelta(e, 1, 0, 0, 0)
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-21e", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		e := q.epochs()
		q.none(sc(1200, talk))
		q.epochDelta(e, 1, 0, 0, 0)
		q.exps()
	})
	expiredThenResent := func(t *testing.T) *seq {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1700, 1, RequestOK))
		q.none(bd(1700, 1, BatchCompleted))
		q.none(tick(5000))
		q.batch(tick(6000), 2, af, standby, camera)
		return q
	}
	t.Run("SQ-21f", func(t *testing.T) {
		q := expiredThenResent(t)
		q.approve(2, set(standby), 6100, Proceed{ReqID: 2})
		e := q.epochs()
		q.none(sc(6200, standby))
		q.epochDelta(e, 0, 0, 0, 0)
		q.owner(OwnerAuto, camera)
	})
	t.Run("SQ-21f/before_approve", func(t *testing.T) {
		q := expiredThenResent(t)
		e := q.epochs()
		q.none(sc(6200, standby))
		q.epochDelta(e, 1, 0, 0, 0)
		q.owner(OwnerNone, "")
		q.approve(2, set(standby), 6300, Reject{})
	})
}

func TestSeqOverrideDeadline(t *testing.T) {
	cases := []struct {
		id   string
		ov   Override
		tr   *int
		res  RequestResult
		dlMs int
	}{
		{"SQ-22a", OverrideNone{}, ip(300), RequestOK, 5300},
		{"SQ-22b", OverrideNone{}, ip(5000), RequestOK, 10000},
		{"SQ-22c", OverrideNone{}, nil, RequestOK, 12000},
		{"SQ-22d", OverrideKnown{Ms: 500}, ip(300), RequestOK, 12000},
		{"SQ-22e", OverrideKnown{Ms: 9000}, ip(300), RequestOK, 14000},
		{"SQ-22f", OverrideKnown{Ms: 500}, nil, RequestOK, 12000},
		{"SQ-22g", OverrideUnknown{}, ip(300), RequestOK, 12000},
		{"SQ-22h", OverrideUnknown{}, ip(9000), RequestOK, 14000},
		{"SQ-22i", OverrideUnknown{}, nil, RequestOK, 12000},
		{"SQ-22j", OverrideNone{}, ip(300), RequestTimeout, 5300},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			q := newSeq(t).prep(camera, true, OK, c.tr)
			q.batch(lc(1000, LOW), 1, af, standby, camera)
			q.approveOv(1, set(standby), c.ov, 1000, Proceed{ReqID: 1})
			q.none(srd(2000, 1, c.res))
			q.deadline(1, c.dlMs)
			q.none(tick(c.dlMs))
			q.exps(standby, 1)
			q.none(tick(c.dlMs + 100))
			q.exps()
		})
	}
}

func TestSeqValidity(t *testing.T) {
	recovering := func(t *testing.T) *seq {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		return q
	}
	t.Run("SQ-23a", func(t *testing.T) {
		q := recovering(t)
		q.none(lc(1200, LOW))
		q.approve(2, set(camera), 1500, Reject{})
		q.none(bd(1500, 2, BatchRejected))
		q.noErr()
		q.owner(OwnerAuto, camera)
		q.batch(lc(3000, OK), 3, ar, camera, "")
	})
	t.Run("SQ-23b", func(t *testing.T) {
		q := recovering(t)
		q.none(sc(1200, talk))
		q.approve(2, set(camera), 1500, Reject{})
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-23c", func(t *testing.T) {
		q := recovering(t)
		q.none(ssc(1200, false))
		q.approve(2, set(camera), 1500, Reject{})
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-23d1", func(t *testing.T) {
		q := recovering(t)
		q.none(disc(1200))
		q.approve(2, set(camera), 1500, Reject{})
		q.owner(OwnerAuto, camera)
		q.none(bd(1500, 2, BatchRejected))
		q.batch(conn(1600, true, standby, ip(300)), 3, ar, camera, "")
	})
	t.Run("SQ-23d2", func(t *testing.T) {
		q := recovering(t)
		q.none(disc(1200))
		q.none(conn(1600, true, standby, ip(300)))
		q.approve(2, set(camera), 1700, Reject{})
		q.none(bd(1700, 2, BatchRejected))
		q.owner(OwnerAuto, camera)
		q.ticks(2000, 5000, 1000)
		q.batch(tick(6000), 3, ar, camera, "")
	})
	t.Run("SQ-23e", func(t *testing.T) {
		q := newSeq(t).P(talk, true, OK)
		q.batch(cmd(1000, CmdStreamStop), 1, stp, "", "")
		q.none(lc(1100, LOW))
		q.none(lc(1200, OK))
		q.none(lc(1300, DOWN))
		q.approve(1, StopStream{}, 1400, Proceed{})
	})
	t.Run("SQ-23f", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(cmd(1000, CmdStandbyOn), 1, son, standby, camera)
		q.none(lc(1100, LOW))
		q.none(lc(1200, OK))
		q.none(lc(1300, DOWN))
		q.approve(1, set(standby), 1400, Proceed{ReqID: 1})
	})
}

func TestSeqSkip(t *testing.T) {
	t.Run("SQ-24a", func(t *testing.T) {
		q := newSeq(t).P(opening, false, OK, withStart(opening))
		q.batch(ssc(1000, true), 1, ssp, opening, "")
		q.approve(1, set(opening), 1100, Skip{})
		q.exps()
		q.none(bd(1100, 1, BatchCompleted))
		q.noErr()
	})
	t.Run("SQ-24b", func(t *testing.T) {
		q := newSeq(t).PM(talk, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfo, talk, "")
		q.approve(2, set(talk), 1000, Proceed{ReqID: 2})
		q.none(srd(6000, 2, RequestTimeout))
		q.none(bd(6000, 2, BatchFailed))
		q.deadline(2, 9300)
		q.errAt(6000)
		q.batch(cmd(6500, CmdStandbyOff), 3, sfo, talk, "")
		q.none(sc(7000, talk))
		q.owner(OwnerNone, "")
		q.approve(3, set(talk), 7100, Skip{})
		q.exps()
		prev := q.err()
		q.none(bd(7100, 3, BatchCompleted))
		q.sameErr(prev)
	})
	monitoredRetry := func(t *testing.T) *seq {
		q := newSeq(t).PM(camera, OK)
		q.batch(cmd(1000, CmdStandbyOff), 2, sfm, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.none(srd(6500, 2, RequestTimeout))
		q.none(bd(6500, 2, BatchFailed))
		q.deadline(2, 9800)
		q.errAt(6500)
		q.batch(cmd(7000, CmdStandbyOff), 3, sfm, camera, "")
		return q
	}
	t.Run("SQ-24c", func(t *testing.T) {
		q := monitoredRetry(t)
		q.approve(3, restart, 7000, Proceed{})
		q.none(sc(7200, camera))
		q.owner(OwnerNone, "")
		q.approve(3, set(camera), 7500, Skip{})
		prev := q.err()
		q.none(bd(7500, 3, BatchCompleted))
		q.sameErr(prev)
	})
	t.Run("SQ-24d", func(t *testing.T) {
		q := monitoredRetry(t)
		q.none(sc(7100, camera))
		q.owner(OwnerNone, "")
		q.approve(3, restart, 7200, Reject{})
		q.none(bd(7200, 3, BatchRejected))
		q.errAt(7200)
	})
}

func TestSeqDuplicateGuard(t *testing.T) {
	t.Run("SQ-25", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.none(lc(2000, DOWN))
		q.ticks(3000, 10000, 1000)
		q.approve(1, set(standby), 10500, Proceed{ReqID: 1})
		q.exps(standby, 1)
		q.none(lc(10600, LOW))
		q.none(lc(10600, DOWN))
		if q.m.st.NextBatchID != 2 {
			t.Fatalf("出たバッチが1本でない: NextBatchID=%d", q.m.st.NextBatchID)
		}
	})
}

func TestSeqStreamBoundary(t *testing.T) {
	t.Run("SQ-26", func(t *testing.T) {
		q := newSeq(t).P(talk, true, OK)
		q.batch(cmd(1000, CmdStreamStop), 1, stp, "", "")
		e := q.epochs()
		q.none(ssc(1100, false))
		q.epochDelta(e, 0, 0, 1, 0)
		q.none(ssc(1200, true))
		q.epochDelta(e, 0, 0, 2, 0)
		q.approve(1, StopStream{}, 1300, Reject{})
		q.none(bd(1300, 1, BatchRejected))
		q.errAt(1300)
	})
}

func TestSeqOutputReconnect(t *testing.T) {
	t.Run("SQ-27", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.batch(cmd(1000, CmdStreamStop), 2, stp, "", "")
		before := cloneState(q.m.st)
		q.ticks(2000, 5000, 1000)
		q.owner(OwnerAuto, camera)
		q.pending(2)
		if d := diffState(q.m.st, before); d != "" {
			t.Fatalf("state changed:%s", d)
		}
	})
}

func TestSeqReconnect(t *testing.T) {
	dropped := func(t *testing.T) (*seq, [4]int) {
		q := newSeq(t).PA(camera)
		e := q.epochs()
		q.none(disc(1000))
		q.epochDelta(e, 0, 1, 0, 0)
		q.exps()
		q.owner(OwnerAuto, camera)
		return q, q.epochs()
	}
	t.Run("SQ-28a", func(t *testing.T) {
		q, e := dropped(t)
		q.none(conn(2000, true, standby, ip(300)))
		q.owner(OwnerAuto, camera)
		q.epochDelta(e, 0, 0, 0, 0)
		q.batch(lc(3000, OK), 2, ar, camera, "")
	})
	t.Run("SQ-28b", func(t *testing.T) {
		q, e := dropped(t)
		q.batch(conn(2000, true, camera2, ip(300)), 2, af, standby, camera2)
		q.owner(OwnerNone, "")
		q.epochDelta(e, 0, 0, 0, 0)
	})
	t.Run("SQ-28c", func(t *testing.T) {
		q, e := dropped(t)
		q.none(conn(2000, false, standby, ip(300)))
		q.owner(OwnerNone, "")
		q.epochDelta(e, 0, 0, 1, 0)
	})
	t.Run("SQ-28d", func(t *testing.T) {
		q := newSeq(t).P(camera, false, LOW, withStart(opening))
		q.none(disc(1000))
		e := q.epochs()
		q.batch(conn(2000, true, camera, ip(300)), 1, af, standby, camera)
		q.epochDelta(e, 0, 0, 1, 0)
	})
	t.Run("SQ-28e", func(t *testing.T) {
		q := newSeq(t).PM(talk, LOW)
		q.none(disc(1000))
		q.none(conn(2000, true, standby, ip(300)))
		q.owner(OwnerManual, talk)
		q.batch(cmd(3000, CmdStandbyOff), 2, sfo, talk, "")
	})
}

func TestSeqNotStreamingManual(t *testing.T) {
	t.Run("SQ-29", func(t *testing.T) {
		q := newSeq(t).P(talk, false, LOW)
		e := q.epochs()
		q.none(sc(1000, camera))
		q.none(sc(2000, standby))
		q.none(sc(3000, camera2))
		q.none(lc(4000, OK))
		q.none(lc(5000, DOWN))
		q.none(tick(10000))
		q.owner(OwnerNone, "")
		q.epochDelta(e, 3, 0, 0, 0)
	})
}

func TestSeqBatchFailure(t *testing.T) {
	t.Run("SQ-30a", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.none(srd(1600, 2, RequestOK))
		q.none(BatchDone{At: at(1600), BatchID: 2, Result: BatchCompleted, Err: errors.New("RestartMedia: input not found")})
		q.errAt(1600)
		q.none(sc(1900, camera))
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-30b", func(t *testing.T) {
		q := newSeq(t).PA(camera)
		q.batch(lc(1000, OK), 2, ar, camera, "")
		q.approve(2, restart, 1000, Proceed{})
		q.approve(2, set(camera), 1500, Proceed{ReqID: 2})
		q.none(srd(1600, 2, RequestError))
		q.none(bd(1600, 2, BatchFailed))
		q.owner(OwnerAuto, camera)
		q.ticks(2000, 5000, 1000)
		q.batch(tick(6000), 3, ar, camera, "")
	})
}

func TestSeqLateOwnSwitch(t *testing.T) {
	sent := func(t *testing.T) *seq {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.exps(standby, 1)
		q.none(srd(1100, 1, RequestOK))
		q.none(bd(1100, 1, BatchCompleted))
		q.batch(sc(1200, camera2), 2, af, standby, camera2)
		q.exps()
		return q
	}
	t.Run("SQ-31", func(t *testing.T) {
		q := sent(t)
		e := q.epochs()
		q.none(sc(1300, standby))
		q.epochDelta(e, 1, 0, 0, 0)
		q.owner(OwnerNone, "")
		q.approve(2, set(standby), 1400, Reject{})
		q.none(bd(1400, 2, BatchRejected))
		q.none(lc(2000, OK))
		q.owner(OwnerNone, "")
	})
	t.Run("SQ-31/approved_first", func(t *testing.T) {
		q := sent(t)
		q.approve(2, set(standby), 1200, Proceed{ReqID: 2})
		e := q.epochs()
		q.none(sc(1300, standby))
		q.epochDelta(e, 0, 0, 0, 0)
		q.owner(OwnerAuto, camera2)
	})
}

func TestSeqRetryInterval(t *testing.T) {
	t.Run("SQ-32a", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.lastAuto(af, 1000)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.exps(standby, 1)
		q.none(srd(1100, 1, RequestError))
		q.none(bd(1100, 1, BatchFailed))
		q.exps()
		q.pending()
		q.errAt(1100)
		q.ticks(2000, 5000, 1000)
		q.none(tick(5900))
		q.batch(tick(6000), 2, af, standby, camera)
		q.lastAuto(af, 6000)
		q.none(tick(7000))
	})
	t.Run("SQ-32b", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		e := q.epochs()
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(6000, 1, RequestTimeout))
		q.none(bd(6000, 1, BatchFailed))
		q.exps(standby, 1)
		q.deadline(1, 9300)
		q.ticks(7000, 9000, 1000)
		q.exps(standby, 1)
		q.batch(tick(10000), 2, af, standby, camera)
		q.exps()
		q.epochDelta(e, 0, 0, 0, 0)
	})
	t.Run("SQ-32c", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.ticks(2000, 30000, 1000)
	})
	t.Run("SQ-32d", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.batch(lc(1000, LOW), 1, af, standby, camera)
		q.approve(1, set(standby), 1000, Proceed{ReqID: 1})
		q.none(srd(1100, 1, RequestError))
		q.none(bd(1100, 1, BatchFailed))
		q.batch(lc(2000, DOWN), 2, af, standby, camera)
	})
}

func TestSnapshot(t *testing.T) {
	q := newSeq(t)
	q.none(cfg(0))
	s := q.m.Snapshot()
	if s.Class != ClassUnknown || s.Connected || !s.AutoEnabled || s.Standby.Owner != OwnerNone || s.HasHealth {
		t.Fatalf("初期の Snapshot = %+v", s)
	}
	if got := q.m.st.CameraStates[camMain]; got != DOWN {
		t.Fatalf("設定後の main = %v, want DOWN", got)
	}
	q.none(lc(0, OK))
	q.none(conn(0, true, camera, ip(300)))
	q.batch(lc(100, LOW), 1, af, standby, camera)
	q.approve(1, set(standby), 100, Proceed{ReqID: 1})
	q.none(srd(100, 1, RequestOK))
	q.none(bd(100, 1, BatchCompleted))
	q.none(sc(500, standby))
	s = q.m.Snapshot()
	want := Snapshot{Connected: true, Streaming: true, Scene: standby, SceneName: "Standby", Class: ClassStandby,
		Standby:     StandbyStatus{Owner: OwnerAuto, ReturnScene: camera, ReturnSceneName: "Camera"},
		AutoEnabled: true}
	// 0b で足した読み口（記録の写し、Studio Mode、カメラ別の状態）は別の行で確かめる。
	s.HiddenEntries, s.RetainedEntries, s.HiddenRevision, s.StudioMode, s.Cameras = nil, nil, 0, false, nil
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("Snapshot = %+v, want %+v", s, want)
	}
	for _, c := range []struct {
		scene string
		want  SceneClass
	}{{camera, ClassMonitored}, {camera2, ClassMonitored}, {talk, ClassOther}} {
		q.m.st.Scene = c.scene
		if got := q.m.Snapshot().Class; got != c.want {
			t.Errorf("Class(%q) = %v, want %v", c.scene, got, c.want)
		}
	}
	q.m.st.Scene = camera
	if s := q.m.Snapshot(); !s.HasHealth || s.Health != LOW || s.BadCamera != camMain || !reflect.DeepEqual(s.VisibleCameras, []string{camMain}) {
		t.Fatalf("監視対象の Snapshot = %+v", s)
	}
	q.none(disc(1000))
	if s := q.m.Snapshot(); s.Class != ClassUnknown || s.Connected || s.HasHealth {
		t.Fatalf("切断後の Snapshot = %+v", s)
	}
	q.m.st.LastError = &ErrorInfo{At: at(1), Msg: "x"}
	s = q.m.Snapshot()
	s.LastError.Msg = "changed"
	if q.m.st.LastError.Msg != "x" {
		t.Fatalf("Snapshot が LastError を共有している")
	}
}
