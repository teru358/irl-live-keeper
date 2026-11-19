package switcher

import (
	"fmt"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

func pendStart(s *state, id int, p Purpose, startID int) {
	pend(s, id, p)
	pb := s.Pending[id]
	pb.StartID = startID
	s.Pending[id] = pb
}

func waitAt(id int, st StartStage) func(*state) {
	return func(s *state) { s.StartWait = &StartWait{ID: id, Stage: st} }
}

func noWait(s *state) { s.StartWait = nil }

// fromStart は、その番号の StreamStart の StartIntro が登録した期待にする。
func fromStart(e Expectation, startID int) Expectation {
	e.StartID = startID
	return e
}

func offAir(s *state) { s.Streaming = false }

func heldNow(s *state) { hold(s, evMs-5000) }

var links = []ingest.LinkState{OK, LOW, DOWN}

func TestEventRowsStart(t *testing.T) {
	exps := func(es ...Expectation) func(*state) { return func(s *state) { s.Expectations = es } }
	scene := func(r Role) func(*state) { return func(s *state) { setRole(s, r) } }
	rows := []evRow{
		{name: "LC-20", setup: all(link(OK), exps(responded(1, RolePause, RequestOK, evMs-100, evMs+3200))),
			ev: lc(evMs, LOW), change: setLink(LOW)},
		{name: "LC-20/Intro expectation", setup: all(link(OK), exps(expect(1, RoleIntro))),
			ev: lc(evMs, DOWN), change: setLink(DOWN)},
	}
	common := func(s *state) {
		s.Connected = false
		s.ConnEpoch++
		s.Expectations = nil
		s.PrevConnScene = s.Scene
		s.PrevConnStreaming = s.Streaming
	}
	for _, st := range []StartStage{StageWaitIntro, StageIntroConfirmed} {
		rows = append(rows, evRow{name: fmt.Sprintf("OD-3/stage %d", st),
			setup: all(offAir, waitAt(5, st), exps(expect(1, RoleIntro))),
			ev:    disc(evMs), change: all(common, noWait), recErr: true})
	}
	rows = append(rows,
		evRow{name: "SCE-9", setup: all(offAir, waitAt(5, StageWaitIntro), exps(expect(1, RoleIntro)),
			func(s *state) { pendStart(s, 1, PurposeStartIntro, 5) }),
			ev: sc(evMs, "Intro"), out: PurposeStartStream,
			change: all(scene(RoleIntro), exps(), waitAt(5, StageIntroConfirmed))},
		evRow{name: "SCE-10", setup: all(offAir, exps(responded(1, RoleIntro, RequestTimeout, evMs-1000, evMs+2300))),
			ev: sc(evMs, "Intro"), change: all(scene(RoleIntro), exps())},
		evRow{name: "SCE-11", setup: all(offAir, waitAt(6, StageWaitIntro),
			exps(responded(1, RoleIntro, RequestTimeout, evMs-1000, evMs+2300)),
			func(s *state) { pendStart(s, 2, PurposeStartIntro, 6) }),
			ev: sc(evMs, "Intro"), out: PurposeStartStream,
			change: all(scene(RoleIntro), exps(), waitAt(6, StageIntroConfirmed))},
		evRow{name: "SCE-12", setup: all(roleIs(RolePause), func(s *state) { s.PausePrev = RoleIntro }, link(LOW),
			exps(expect(1, RoleIntro))),
			ev: sc(evMs, "Intro"), change: all(scene(RoleIntro), exps())},
	)
	for _, from := range []Role{RolePause, RoleIntro} {
		rows = append(rows,
			evRow{name: "SCE-13/from " + from.String(), setup: all(roleIs(from), func(s *state) {
				if s.Role == RolePause {
					s.PausePrev = RoleLive
				}
			}, link(LOW), exps(expect(1, RoleLive))),
				ev: sc(evMs, "Live"), out: PurposeAutoFail, change: all(scene(RoleLive), exps())},
			evRow{name: "SCE-14/from " + from.String(), setup: all(roleIs(from), func(s *state) {
				if s.Role == RolePause {
					s.PausePrev = RoleFail
				}
			}, link(OK), exps(expect(1, RoleFail))),
				ev: sc(evMs, "Fail"), out: PurposeAutoRecover, change: all(scene(RoleFail), exps())},
		)
	}
	manual := func(r Role) func(*state) {
		return func(s *state) {
			setRole(s, r)
			s.SceneEpoch++
			s.Expectations = nil
		}
	}
	for _, st := range []StartStage{StageWaitIntro, StageIntroConfirmed} {
		for _, to := range []Role{RoleIntro, RoleFail} {
			from := RoleLive
			if st == StageIntroConfirmed {
				from = RoleIntro
			}
			rows = append(rows, evRow{name: fmt.Sprintf("SCM-12/stage %d →%v", st, to),
				setup: all(offAir, roleIs(from), waitAt(5, st), func(s *state) { pendStart(s, 1, PurposeStartIntro, 5) }),
				ev:    sc(evMs, sceneOf(to)), change: all(manual(to), noWait), recErr: true})
		}
	}
	up := func(s *state) {
		s.Streaming = true
		s.StreamEpoch++
		s.StartWait = nil
	}
	rows = append(rows,
		evRow{name: "SST-4", setup: all(offAir, roleIs(RoleIntro), link(LOW), waitAt(5, StageIntroConfirmed),
			func(s *state) { pendStart(s, 1, PurposeStartStream, 5) }),
			ev: ssc(evMs, true), change: up},
		evRow{name: "SST-5/Live OK", setup: all(offAir, waitAt(5, StageWaitIntro), exps(expect(1, RoleIntro))),
			ev: ssc(evMs, true), change: up},
		evRow{name: "SST-5/Live LOW", setup: all(offAir, link(LOW), waitAt(5, StageWaitIntro),
			func(s *state) { pendStart(s, 1, PurposeStartIntro, 5) }),
			ev: ssc(evMs, true), out: PurposeAutoFail, change: up},
		evRow{name: "SRD-TO-2", setup: all(offAir, roleIs(RoleIntro), waitAt(5, StageIntroConfirmed)),
			ev: srd(evMs, 1, RequestTimeout)},
		evRow{name: "SRD-ERR-2", setup: all(offAir, waitAt(5, StageWaitIntro), exps(fromStart(expect(1, RoleIntro), 5))),
			ev: srd(evMs, 1, RequestError), change: all(exps(), noWait), recErr: true},
		evRow{name: "SRD-ERR-2/earlier StreamStart", setup: all(offAir, waitAt(5, StageWaitIntro),
			exps(fromStart(expect(1, RoleIntro), 4), fromStart(expect(2, RoleIntro), 5))),
			ev: srd(evMs, 1, RequestError), change: exps(fromStart(expect(2, RoleIntro), 5))},
		evRow{name: "TK-10", setup: all(offAir, waitAt(5, StageWaitIntro),
			exps(fromStart(responded(1, RoleIntro, RequestOK, evMs-3400, evMs-100), 5))),
			ev: tick(evMs), change: all(exps(), noWait), recErr: true},
		evRow{name: "TK-10/earlier StreamStart", setup: all(offAir, waitAt(5, StageWaitIntro),
			exps(fromStart(responded(1, RoleIntro, RequestTimeout, evMs-3400, evMs-100), 4), fromStart(expect(2, RoleIntro), 5))),
			ev: tick(evMs), change: exps(fromStart(expect(2, RoleIntro), 5))},
	)
	runEvRows(t, rows)
}

func TestBatchDoneCommand(t *testing.T) {
	removed := func(s *state) { delete(s.Pending, 1) }
	rows := []evRow{
		{name: "BD-C-2", setup: all(offAir, roleIs(RoleIntro), waitAt(5, StageIntroConfirmed),
			func(s *state) { pendStart(s, 1, PurposeStartStream, 5) }),
			ev: bd(evMs, 1, BatchCompleted), change: all(removed, noWait)},
		{name: "BD-C-3", setup: all(offAir, waitAt(5, StageWaitIntro), func(s *state) {
			pendStart(s, 1, PurposeStartIntro, 5)
			s.Expectations = []Expectation{responded(1, RoleIntro, RequestOK, evMs-100, evMs+3200)}
		}), ev: bd(evMs, 1, BatchCompleted), change: removed},
		{name: "BD-C-4", setup: all(offAir, roleIs(RoleIntro), waitAt(5, StageWaitIntro),
			func(s *state) { pendStart(s, 1, PurposeStartIntro, 5) }),
			ev: bd(evMs, 1, BatchCompleted), out: PurposeStartStream,
			change: all(removed, waitAt(5, StageIntroConfirmed))},
		{name: "BD-C-4/already confirmed", setup: all(offAir, roleIs(RoleIntro), waitAt(5, StageIntroConfirmed),
			func(s *state) {
				pendStart(s, 1, PurposeStartIntro, 5)
				pendStart(s, 2, PurposeStartStream, 5)
			}),
			ev: bd(evMs, 1, BatchCompleted), change: removed},
	}
	for _, p := range []Purpose{PurposeGoLive, PurposeGoFail, PurposePauseOn, PurposePauseOffLive,
		PurposePauseOffFail, PurposePauseOffIntro, PurposeStopStream} {
		rows = append(rows,
			evRow{name: "BD-F-2/" + p.String(), setup: func(s *state) { pend(s, 1, p) },
				ev:     BatchDone{At: at(evMs), BatchID: 1, Result: BatchFailed, Err: fmt.Errorf("失敗")},
				change: removed, recErr: true},
			evRow{name: "BD-R-2/" + p.String(), setup: func(s *state) { pend(s, 1, p) },
				ev: bd(evMs, 1, BatchRejected), change: removed, recErr: true},
		)
	}
	rows = append(rows,
		evRow{name: "BD-F-3", setup: all(offAir, waitAt(5, StageWaitIntro), func(s *state) {
			pendStart(s, 1, PurposeStartIntro, 5)
			s.Expectations = []Expectation{responded(1, RoleIntro, RequestTimeout, evMs-100, evMs+3200)}
		}), ev: bd(evMs, 1, BatchFailed), change: all(removed, noWait), recErr: true},
		evRow{name: "BD-F-4", setup: all(offAir, roleIs(RoleIntro), waitAt(5, StageIntroConfirmed), func(s *state) {
			pendStart(s, 1, PurposeStartIntro, 5)
			pendStart(s, 2, PurposeStartStream, 5)
		}), ev: bd(evMs, 1, BatchFailed), change: removed},
		evRow{name: "BD-F-5", setup: all(offAir, roleIs(RoleIntro), waitAt(5, StageIntroConfirmed),
			func(s *state) { pendStart(s, 1, PurposeStartStream, 5) }),
			ev: bd(evMs, 1, BatchFailed), change: all(removed, noWait), recErr: true},
	)
	for _, p := range []Purpose{PurposeGoLive, PurposePauseOffLive} {
		rows = append(rows, evRow{name: "BD-R-3/" + p.String(), setup: all(roleIs(RoleIntro), func(s *state) {
			pend(s, 1, p)
			pb := s.Pending[1]
			pb.Replaced = true
			s.Pending[1] = pb
			pend(s, 2, replacementOf(p))
		}), ev: bd(evMs, 1, BatchRejected), change: removed})
	}
	for _, p := range []Purpose{PurposeStartIntro, PurposeStartStream} {
		rows = append(rows, evRow{name: "BD-R-4/" + p.String(), setup: all(offAir, noWait,
			func(s *state) { pendStart(s, 1, p, 5) }),
			ev: bd(evMs, 1, BatchRejected), change: removed, recErr: true})
	}
	runEvRows(t, rows)
}

func TestCommands(t *testing.T) {
	invalid := ErrInvalidState{}
	notConn := ErrNotConnected{}
	disconnected := func(s *state) { s.Connected = false }
	kinds := []CommandKind{CmdStreamStart, CmdStreamStop, CmdGoLive, CmdPauseOn, CmdPauseOff}
	kindName := map[CommandKind]string{CmdStreamStart: "START", CmdStreamStop: "STOP", CmdGoLive: "GOLIVE",
		CmdPauseOn: "PON", CmdPauseOff: "POFF"}
	var rows []evRow
	for _, k := range kinds {
		rows = append(rows, evRow{name: "CMD-" + kindName[k] + "-1", setup: all(disconnected, heldNow),
			ev: cmd(evMs, k), err: notConn})
		rows = append(rows, evRow{name: "CMD-" + kindName[k] + "-1/off air", setup: all(disconnected, offAir),
			ev: cmd(evMs, k), err: notConn})
	}
	startWait := func(st StartStage) func(*state) {
		return func(s *state) {
			s.StartWait = &StartWait{ID: s.NextStartID, Stage: st}
			s.NextStartID++
		}
	}
	rows = append(rows,
		evRow{name: "CMD-START-2", setup: roleIs(RoleIntro), ev: cmd(evMs, CmdStreamStart), err: invalid},
		evRow{name: "CMD-START-2/held", setup: heldNow, ev: cmd(evMs, CmdStreamStart), err: invalid},
		evRow{name: "CMD-START-3/wait intro", setup: all(offAir, waitAt(4, StageWaitIntro)), ev: cmd(evMs, CmdStreamStart), err: invalid},
		evRow{name: "CMD-START-3/confirmed", setup: all(offAir, roleIs(RoleIntro), waitAt(4, StageIntroConfirmed)),
			ev: cmd(evMs, CmdStreamStart), err: invalid},
		evRow{name: "CMD-START-4", setup: all(offAir, roleIs(RoleIntro), link(LOW)), ev: cmd(evMs, CmdStreamStart),
			out: PurposeStartStream, change: startWait(StageIntroConfirmed)},
	)
	for _, r := range []Role{RoleLive, RoleFail, RolePause, RoleOther} {
		rows = append(rows, evRow{name: "CMD-START-5/" + r.String(), setup: all(offAir, roleIs(r)),
			ev: cmd(evMs, CmdStreamStart), out: PurposeStartIntro, change: startWait(StageWaitIntro)})
	}
	for _, c := range []struct {
		r Role
		p Purpose
	}{{RoleIntro, PurposeStartStream}, {RoleIntro, PurposeStartIntro}, {RoleLive, PurposeStartIntro}, {RoleLive, PurposeStartStream}} {
		rows = append(rows, evRow{name: fmt.Sprintf("CMD-START-6/%v pending %v", c.r, c.p),
			setup: all(offAir, roleIs(c.r), func(s *state) { pendStart(s, 1, c.p, 4) }),
			ev:    cmd(evMs, CmdStreamStart), err: invalid})
	}
	rows = append(rows,
		evRow{name: "CMD-STOP-2", setup: offAir, ev: cmd(evMs, CmdStreamStop), err: invalid},
	)
	for _, r := range []Role{RoleIntro, RoleLive, RoleFail, RolePause, RoleOther} {
		for _, l := range links {
			rows = append(rows, evRow{name: fmt.Sprintf("CMD-STOP-3/%v %v", r, l), setup: all(roleIs(r), link(l)),
				ev: cmd(evMs, CmdStreamStop), out: PurposeStopStream})
		}
	}
	rows = append(rows,
		evRow{name: "CMD-STOP-4", setup: all(heldNow, link(LOW)), ev: cmd(evMs, CmdStreamStop), out: PurposeStopStream, change: unhold},
		evRow{name: "CMD-STOP-5", setup: func(s *state) { pend(s, 1, PurposeStopStream) }, ev: cmd(evMs, CmdStreamStop), err: invalid},
	)
	for _, r := range []Role{RoleIntro, RoleLive, RoleFail, RolePause, RoleOther} {
		rows = append(rows, evRow{name: "CMD-GOLIVE-2/" + r.String(), setup: all(offAir, roleIs(r)),
			ev: cmd(evMs, CmdGoLive), err: invalid})
	}
	for _, r := range []Role{RoleLive, RoleFail, RolePause, RoleOther} {
		rows = append(rows, evRow{name: "CMD-GOLIVE-3/" + r.String(), setup: roleIs(r), ev: cmd(evMs, CmdGoLive), err: invalid})
	}
	rows = append(rows,
		evRow{name: "CMD-GOLIVE-3/held Live", setup: all(heldNow, link(LOW)), ev: cmd(evMs, CmdGoLive), err: invalid},
		evRow{name: "CMD-GOLIVE-4", setup: roleIs(RoleIntro), ev: cmd(evMs, CmdGoLive), out: PurposeGoLive},
	)
	for _, l := range []ingest.LinkState{LOW, DOWN} {
		rows = append(rows, evRow{name: "CMD-GOLIVE-5/" + l.String(), setup: all(roleIs(RoleIntro), link(l)),
			ev: cmd(evMs, CmdGoLive), out: PurposeGoFail})
	}
	for _, p := range []Purpose{PurposeGoLive, PurposeGoFail} {
		for _, l := range []ingest.LinkState{OK, LOW} {
			rows = append(rows, evRow{name: fmt.Sprintf("CMD-GOLIVE-6/pending %v %v", p, l),
				setup: all(roleIs(RoleIntro), link(l), func(s *state) { pend(s, 1, p) }),
				ev:    cmd(evMs, CmdGoLive), err: invalid})
		}
	}
	rows = append(rows, evRow{name: "CMD-PON-2", setup: all(offAir, roleIs(RoleIntro)), ev: cmd(evMs, CmdPauseOn), err: invalid})
	for _, r := range []Role{RolePause, RoleOther} {
		rows = append(rows, evRow{name: "CMD-PON-3/" + r.String(), setup: roleIs(r), ev: cmd(evMs, CmdPauseOn), err: invalid})
	}
	for _, r := range []Role{RoleIntro, RoleLive, RoleFail} {
		for _, l := range links {
			rows = append(rows, evRow{name: fmt.Sprintf("CMD-PON-4/%v %v", r, l), setup: all(roleIs(r), link(l)),
				ev: cmd(evMs, CmdPauseOn), out: PurposePauseOn})
		}
	}
	for _, r := range []Role{RoleLive, RoleFail} {
		rows = append(rows, evRow{name: "CMD-PON-5/" + r.String(), setup: all(roleIs(r), heldNow, link(LOW)),
			ev: cmd(evMs, CmdPauseOn), out: PurposePauseOn, change: unhold})
	}
	rows = append(rows,
		evRow{name: "CMD-PON-6", setup: func(s *state) { pend(s, 1, PurposePauseOn) }, ev: cmd(evMs, CmdPauseOn), err: invalid},
		evRow{name: "CMD-POFF-2", setup: all(offAir, roleIs(RolePause)), ev: cmd(evMs, CmdPauseOff), err: invalid},
	)
	for _, r := range []Role{RoleIntro, RoleLive, RoleFail, RoleOther} {
		rows = append(rows, evRow{name: "CMD-POFF-3/" + r.String(), setup: roleIs(r), ev: cmd(evMs, CmdPauseOff), err: invalid})
	}
	paused := func(prev Role) func(*state) {
		return func(s *state) {
			setRole(s, RolePause)
			s.PausePrev = prev
		}
	}
	for _, l := range links {
		rows = append(rows, evRow{name: "CMD-POFF-4/" + l.String(), setup: all(paused(RoleIntro), link(l)),
			ev: cmd(evMs, CmdPauseOff), out: PurposePauseOffIntro})
	}
	for _, prev := range []Role{RoleLive, RoleFail, RoleUnknown} {
		rows = append(rows, evRow{name: "CMD-POFF-5/" + prev.String(), setup: all(paused(prev), link(OK)),
			ev: cmd(evMs, CmdPauseOff), out: PurposePauseOffLive})
		for _, l := range []ingest.LinkState{LOW, DOWN} {
			rows = append(rows, evRow{name: fmt.Sprintf("CMD-POFF-6/%v %v", prev, l), setup: all(paused(prev), link(l)),
				ev: cmd(evMs, CmdPauseOff), out: PurposePauseOffFail})
		}
	}
	for _, p := range []Purpose{PurposePauseOffLive, PurposePauseOffFail, PurposePauseOffIntro} {
		for _, prev := range []Role{RoleIntro, RoleLive} {
			rows = append(rows, evRow{name: fmt.Sprintf("CMD-POFF-7/pending %v prev %v", p, prev),
				setup: all(paused(prev), func(s *state) { pend(s, 1, p) }),
				ev:    cmd(evMs, CmdPauseOff), err: invalid})
		}
	}
	runEvRows(t, rows)
}

func TestApproveCommon(t *testing.T) {
	p1 := func(p Purpose) func(*state) { return func(s *state) { pend(s, 1, p) } }
	var rows []apRow
	cases := []struct {
		p    Purpose
		role Role
		l    ingest.LinkState
		a    Action
	}{
		{PurposeAutoFail, RoleLive, LOW, failA()},
		{PurposeGoLive, RoleIntro, OK, RestartMedia{Wait: W}},
		{PurposePauseOn, RoleLive, OK, SetScene{Role: RolePause}},
		{PurposeStopStream, RoleLive, OK, StopStream{}},
	}
	for _, c := range cases {
		base := all(roleIs(c.role), link(c.l), p1(c.p))
		rows = append(rows,
			apRow{name: "AP-COM-1/" + c.p.String(), setup: all(base, func(s *state) { s.Connected = false }),
				action: c.a, atMs: evMs, res: Reject{}},
			apRow{name: "AP-COM-2/" + c.p.String(), setup: all(base, func(s *state) { s.ConnEpoch++ }),
				action: c.a, atMs: evMs, res: Reject{}},
			apRow{name: "AP-COM-3/" + c.p.String(), setup: all(base, func(s *state) { s.StreamEpoch += 2 }),
				action: c.a, atMs: evMs, res: Reject{}},
			apRow{name: "AP-COM-4/" + c.p.String(), setup: all(roleIs(c.role), link(c.l), func(s *state) { pend(s, 2, c.p) }),
				action: c.a, atMs: evMs, res: Reject{}},
		)
	}
	rows = append(rows,
		apRow{name: "AP-COM-3/stopped", setup: all(roleIs(RoleIntro), p1(PurposeGoLive), func(s *state) {
			s.StreamEpoch++
			s.Streaming = false
		}), action: liveA(), atMs: evMs, res: Reject{}},
		apRow{name: "AP-COM-5/PauseOn", setup: all(p1(PurposePauseOn), func(s *state) { s.SceneEpoch++ }),
			action: SetScene{Role: RolePause}, atMs: evMs, res: Reject{}},
		apRow{name: "AP-COM-5/GoLive RestartMedia", setup: all(roleIs(RoleIntro), p1(PurposeGoLive), func(s *state) { s.SceneEpoch++ }),
			action: RestartMedia{Wait: W}, atMs: evMs, res: Reject{}},
		apRow{name: "AP-COM-5/StartIntro", setup: all(offAir, waitAt(5, StageWaitIntro),
			func(s *state) { pendStart(s, 1, PurposeStartIntro, 5); s.SceneEpoch++ }),
			action: SetScene{Role: RoleIntro}, atMs: evMs, res: Reject{}, change: noWait},
	)
	runApRows(t, rows)
}

func TestApprovePurposes(t *testing.T) {
	p1 := func(p Purpose) func(*state) { return func(s *state) { pend(s, 1, p) } }
	exps := func(es ...Expectation) func(*state) { return func(s *state) { s.Expectations = es } }
	restart := RestartMedia{Wait: W}
	introA := SetScene{Role: RoleIntro}
	pauseA := SetScene{Role: RolePause}
	both := []Action{restart, liveA()}
	actName := func(a Action) string {
		if _, ok := a.(RestartMedia); ok {
			return "RestartMedia"
		}
		return "SetScene"
	}
	rows := []apRow{
		{name: "AP-AUTOFAIL-3", setup: all(link(OK), p1(PurposeAutoFail)), action: failA(), atMs: evMs, res: Reject{}},
		{name: "AP-AUTOFAIL-4/Pause", setup: all(roleIs(RolePause), link(LOW), p1(PurposeAutoFail)), action: failA(), atMs: evMs, res: Reject{}},
		{name: "AP-AUTOFAIL-4/Intro", setup: all(roleIs(RoleIntro), link(DOWN), p1(PurposeAutoFail)), action: failA(), atMs: evMs, res: Reject{}},
		{name: "AP-AUTOFAIL-5", setup: all(roleIs(RoleFail), link(LOW), p1(PurposeAutoFail)), action: failA(), atMs: evMs, res: Skip{}},
		{name: "AP-AUTOFAIL-5/OK again", setup: all(roleIs(RoleFail), link(OK), p1(PurposeAutoFail)), action: failA(), atMs: evMs, res: Skip{}},
		{name: "AP-AUTOFAIL-6/Pause", setup: all(link(LOW), p1(PurposeAutoFail), exps(expect(1, RolePause))), action: failA(), atMs: evMs, res: Reject{}},
		{name: "AP-AUTOFAIL-6/Intro", setup: all(link(DOWN), p1(PurposeAutoFail), exps(responded(1, RoleIntro, RequestOK, evMs-100, evMs+3200))),
			action: failA(), atMs: evMs, res: Reject{}},
	}
	for _, a := range both {
		n := actName(a)
		for _, l := range []ingest.LinkState{LOW, DOWN} {
			rows = append(rows, apRow{name: "AP-AUTOREC-3/" + n + " " + l.String(),
				setup: all(roleIs(RoleFail), link(l), p1(PurposeAutoRecover)), action: a, atMs: evMs, res: Reject{}})
		}
		rows = append(rows,
			apRow{name: "AP-AUTOREC-4/" + n, setup: all(roleIs(RolePause), link(OK), p1(PurposeAutoRecover)), action: a, atMs: evMs, res: Reject{}},
			apRow{name: "AP-AUTOREC-7/" + n + " Pause", setup: all(roleIs(RoleFail), link(OK), p1(PurposeAutoRecover), exps(expect(1, RolePause))),
				action: a, atMs: evMs, res: Reject{}},
			apRow{name: "AP-AUTOREC-7/" + n + " Intro", setup: all(roleIs(RoleFail), link(OK), p1(PurposeAutoRecover), exps(expect(1, RoleIntro))),
				action: a, atMs: evMs, res: Reject{}},
		)
	}
	rows = append(rows,
		apRow{name: "AP-AUTOREC-5", setup: all(link(OK), p1(PurposeAutoRecover)), action: liveA(), atMs: evMs, res: Skip{}},
		apRow{name: "AP-AUTOREC-6", setup: all(link(OK), p1(PurposeAutoRecover)), action: restart, atMs: evMs, res: Reject{}},
		apRow{name: "AP-GOLIVE-1", setup: all(roleIs(RoleIntro), p1(PurposeGoLive)), action: restart, atMs: evMs, res: Proceed{ReqID: 0}},
		apRow{name: "AP-GOLIVE-2", setup: all(roleIs(RoleIntro), p1(PurposeGoLive)), action: liveA(), atMs: evMs, res: Proceed{}, register: RoleLive},
	)
	for _, l := range []ingest.LinkState{LOW, DOWN} {
		rows = append(rows,
			apRow{name: "AP-GOLIVE-3/" + l.String(), setup: all(roleIs(RoleIntro), link(l), p1(PurposeGoLive)),
				action: restart, atMs: evMs, res: Replace{}, replace: PurposeGoFail},
			apRow{name: "AP-GOLIVE-4/" + l.String(), setup: all(roleIs(RoleIntro), link(l), p1(PurposeGoLive)),
				action: liveA(), atMs: evMs, res: Replace{}, replace: PurposeGoFail},
		)
	}
	for _, a := range both {
		n := actName(a)
		rows = append(rows,
			apRow{name: "AP-GOLIVE-5/" + n, setup: all(roleIs(RolePause), link(OK), p1(PurposeGoLive)), action: a, atMs: evMs, res: Reject{}},
			apRow{name: "AP-GOLIVE-6/" + n, setup: all(roleIs(RolePause), link(LOW), p1(PurposeGoLive)), action: a, atMs: evMs, res: Reject{}},
		)
	}
	paused := func(s *state) {
		setRole(s, RolePause)
		s.PausePrev = RoleLive
	}
	rows = append(rows,
		apRow{name: "AP-GOLIVE-7", setup: all(link(OK), p1(PurposeGoLive)), action: liveA(), atMs: evMs, res: Skip{}},
		apRow{name: "AP-POFFLIVE-1", setup: all(paused, p1(PurposePauseOffLive)), action: restart, atMs: evMs, res: Proceed{ReqID: 0}},
		apRow{name: "AP-POFFLIVE-2", setup: all(paused, p1(PurposePauseOffLive)), action: liveA(), atMs: evMs, res: Proceed{}, register: RoleLive},
	)
	for _, a := range both {
		n := actName(a)
		for _, l := range []ingest.LinkState{LOW, DOWN} {
			rows = append(rows, apRow{name: "AP-POFFLIVE-3/" + n + " " + l.String(), setup: all(paused, link(l), p1(PurposePauseOffLive)),
				action: a, atMs: evMs, res: Replace{}, replace: PurposePauseOffFail})
		}
		for _, r := range []Role{RoleIntro, RoleFail} {
			for _, l := range []ingest.LinkState{OK, LOW} {
				rows = append(rows, apRow{name: fmt.Sprintf("AP-POFFLIVE-4/%s %v %v", n, r, l),
					setup: all(roleIs(r), link(l), p1(PurposePauseOffLive)), action: a, atMs: evMs, res: Reject{}})
			}
		}
	}
	rows = append(rows, apRow{name: "AP-POFFLIVE-5", setup: all(link(OK), p1(PurposePauseOffLive)), action: liveA(), atMs: evMs, res: Skip{}})
	for _, l := range links {
		rows = append(rows,
			apRow{name: "AP-GOFAIL-1/" + l.String(), setup: all(roleIs(RoleIntro), link(l), p1(PurposeGoFail)),
				action: failA(), atMs: evMs, res: Proceed{}, register: RoleFail},
			apRow{name: "AP-POFFFAIL-1/" + l.String(), setup: all(paused, link(l), p1(PurposePauseOffFail)),
				action: failA(), atMs: evMs, res: Proceed{}, register: RoleFail},
			apRow{name: "AP-POFFINTRO-1/" + l.String(), setup: all(paused, link(l), p1(PurposePauseOffIntro)),
				action: introA, atMs: evMs, res: Proceed{}, register: RoleIntro},
		)
	}
	rows = append(rows,
		apRow{name: "AP-GOFAIL-2", setup: all(paused, link(LOW), p1(PurposeGoFail)), action: failA(), atMs: evMs, res: Reject{}},
		apRow{name: "AP-GOFAIL-3", setup: all(link(LOW), p1(PurposeGoFail)), action: failA(), atMs: evMs, res: Reject{}},
		apRow{name: "AP-GOFAIL-4", setup: all(roleIs(RoleFail), link(LOW), p1(PurposeGoFail)), action: failA(), atMs: evMs, res: Skip{}},
		apRow{name: "AP-POFFFAIL-2", setup: all(roleIs(RoleIntro), link(LOW), p1(PurposePauseOffFail)), action: failA(), atMs: evMs, res: Reject{}},
		apRow{name: "AP-POFFFAIL-3", setup: all(link(LOW), p1(PurposePauseOffFail)), action: failA(), atMs: evMs, res: Reject{}},
		apRow{name: "AP-POFFFAIL-4", setup: all(roleIs(RoleFail), p1(PurposePauseOffFail)), action: failA(), atMs: evMs, res: Skip{}},
		apRow{name: "AP-POFFINTRO-2/Live", setup: all(p1(PurposePauseOffIntro)), action: introA, atMs: evMs, res: Reject{}},
		apRow{name: "AP-POFFINTRO-2/Fail", setup: all(roleIs(RoleFail), p1(PurposePauseOffIntro)), action: introA, atMs: evMs, res: Reject{}},
		apRow{name: "AP-POFFINTRO-3", setup: all(roleIs(RoleIntro), p1(PurposePauseOffIntro)), action: introA, atMs: evMs, res: Skip{}},
	)
	for _, r := range []Role{RoleIntro, RoleLive, RoleFail} {
		for _, l := range links {
			rows = append(rows, apRow{name: fmt.Sprintf("AP-PON-1/%v %v", r, l), setup: all(roleIs(r), link(l), p1(PurposePauseOn)),
				action: pauseA, atMs: evMs, res: Proceed{}, register: RolePause})
		}
	}
	rows = append(rows,
		apRow{name: "AP-PON-2", setup: all(p1(PurposePauseOn), roleIs(RoleFail), link(LOW)),
			action: pauseA, atMs: evMs, res: Proceed{}, register: RolePause},
		apRow{name: "AP-PON-3", setup: all(paused, p1(PurposePauseOn)), action: pauseA, atMs: evMs, res: Skip{}},
	)
	for _, r := range []Role{RoleLive, RoleFail, RolePause, RoleOther} {
		rows = append(rows, apRow{name: "AP-STARTINTRO-1/" + r.String(),
			setup:  all(offAir, roleIs(r), waitAt(5, StageWaitIntro), func(s *state) { pendStart(s, 1, PurposeStartIntro, 5) }),
			action: introA, atMs: evMs, res: Proceed{}, register: RoleIntro})
	}
	startIntro := all(offAir, func(s *state) { pendStart(s, 1, PurposeStartIntro, 5) })
	rows = append(rows,
		apRow{name: "AP-STARTINTRO-2/manual", setup: all(startIntro, roleIs(RoleFail), func(s *state) { s.SceneEpoch++ }),
			action: introA, atMs: evMs, res: Reject{}},
		apRow{name: "AP-STARTINTRO-2/disconnected", setup: all(startIntro, func(s *state) { s.Connected = false; s.ConnEpoch++ }),
			action: introA, atMs: evMs, res: Reject{}},
		apRow{name: "AP-STARTINTRO-2/stream started", setup: all(startIntro, func(s *state) { s.Streaming = true; s.StreamEpoch++ }),
			action: introA, atMs: evMs, res: Reject{}},
		apRow{name: "AP-STARTINTRO-2/no wait only", setup: startIntro, action: introA, atMs: evMs, res: Reject{}},
		apRow{name: "AP-STARTINTRO-4/wait intro", setup: all(startIntro, roleIs(RoleIntro), waitAt(5, StageWaitIntro)),
			action: introA, atMs: evMs, res: Skip{}},
		apRow{name: "AP-STARTINTRO-4/confirmed", setup: all(startIntro, roleIs(RoleIntro), waitAt(5, StageIntroConfirmed)),
			action: introA, atMs: evMs, res: Skip{}},
		apRow{name: "AP-STARTSTREAM-1", setup: all(offAir, roleIs(RoleIntro), waitAt(5, StageIntroConfirmed),
			func(s *state) { pendStart(s, 1, PurposeStartStream, 5) }),
			action: StartStream{}, atMs: evMs, res: Proceed{ReqID: 0}},
	)
	for _, r := range []Role{RoleIntro, RoleLive} {
		rows = append(rows, apRow{name: "AP-STARTSTREAM-2/" + r.String(),
			setup:  all(offAir, func(s *state) { pendStart(s, 1, PurposeStartStream, 5) }, roleIs(r), func(s *state) { s.SceneEpoch++ }),
			action: StartStream{}, atMs: evMs, res: Reject{}})
	}
	rows = append(rows,
		apRow{name: "AP-STARTSTREAM-3", setup: all(offAir, roleIs(RoleIntro), func(s *state) {
			pendStart(s, 1, PurposeStartStream, 5)
			s.Connected = false
			s.ConnEpoch++
		}), action: StartStream{}, atMs: evMs, res: Reject{}},
	)
	for _, r := range []Role{RoleIntro, RoleLive, RoleFail, RolePause, RoleOther} {
		for _, l := range links {
			rows = append(rows, apRow{name: fmt.Sprintf("AP-STOP-1/%v %v", r, l),
				setup:  all(p1(PurposeStopStream), roleIs(r), link(l)),
				action: StopStream{}, atMs: evMs, res: Proceed{ReqID: 0}})
		}
	}
	rows = append(rows,
		apRow{name: "AP-STOP-2", setup: all(p1(PurposeStopStream), func(s *state) { s.SceneEpoch += 2; hold(s, evMs-1000) }),
			action: StopStream{}, atMs: evMs, res: Proceed{ReqID: 0}},
		apRow{name: "AP-STOP-3", setup: all(p1(PurposeStopStream), func(s *state) { s.StreamEpoch += 2 }),
			action: StopStream{}, atMs: evMs, res: Reject{}},
	)
	runApRows(t, rows)
}
