package switcher

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRowsScenesConfigured(t *testing.T) {
	// 旧 Camera3 追加（監視対象の追加の設定変更）は、on_bad を standby から hide にする設定変更に置き換える（台帳）。
	camera3 := withHide()
	rows := []evRow{
		{name: "SCF-1", setup: func(s *state) { *s = newMachine().st }, ev: cfg(1000),
			change: func(s *state) { applyCfg(s, cfgOf(0)) }},
		{name: "SCF-2/same", ev: cfg(1000)},
		{name: "SCF-2/start", setup: startAt(opening), ev: cfg(1000, withStart(opening))},
		// SCF-3、4、5、9、10 は分類の変更なので、入口を設定から地図へ移す（出力・持ち主は同じ。ConfigEpoch は進まない）。
		{name: "SCF-3", setup: all(scn(talk), link(LOW)), ev: smc(1000, camera, camera2, talk),
			change: mapChange(false, camera, camera2, talk), out: PurposeAutoFail, ret: talk},
		{name: "SCF-4", setup: scn(talk), ev: smc(1000, camera, camera2, talk),
			change: mapChange(false, camera, camera2, talk)},
		{name: "SCF-5", setup: link(LOW), ev: smc(1000, camera2),
			change: mapChange(false, camera2)},
		{name: "SCF-6", setup: own(OwnerAuto, camera), ev: cfg(1000, withStandby(standby2)),
			change: cfgChange(true, withStandby(standby2))},
		{name: "SCF-7", setup: own(OwnerManual, camera), ev: cfg(1000, withStandby(standby2)),
			change: cfgChange(true, withStandby(standby2))},
		// 期待値が変わる行（台帳）: S に m=DOWN が映る追加の列は、C のあと F(S)、切替先 S2、戻り先 S。
		{name: "SCF-6/0a/bad-camera", setup: all(useMap(testMap(camera, camera2, standby)), link(DOWN), own(OwnerAuto, camera)),
			ev: cfg(1000, withStandby(standby2)), change: cfgChange(true, withStandby(standby2)),
			out: PurposeAutoFail, to: standby2, ret: standby},
		{name: "SCF-7/0a/bad-camera", setup: all(useMap(testMap(camera, camera2, standby)), link(DOWN), own(OwnerManual, camera)),
			ev: cfg(1000, withStandby(standby2)), change: cfgChange(true, withStandby(standby2)),
			out: PurposeAutoFail, to: standby2, ret: standby},
		{name: "SCF-8", setup: link(LOW), ev: cfg(1000, withStandby(standby2)),
			change: cfgChange(false, withStandby(standby2)), out: PurposeAutoFail, to: standby2, ret: camera},
		{name: "SCF-9", setup: own(OwnerAuto, camera), ev: smc(1000, camera2),
			change: mapChange(true, camera2)},
		{name: "SCF-10", setup: own(OwnerManual, camera), ev: smc(1000, camera2),
			change: mapChange(false, camera2)},
		{name: "SCF-11", setup: own(OwnerAuto, camera), ev: cfg(1000, withAuto(false)),
			change: cfgChange(true, withAuto(false))},
		{name: "SCF-12", setup: own(OwnerManual, camera), ev: cfg(1000, withAuto(false)),
			change: cfgChange(false, withAuto(false))},
		{name: "SCF-13/auto_add_monitored", setup: own(OwnerAuto, camera), ev: cfg(1000, camera3),
			change: cfgChange(false, camera3), out: PurposeAutoRecover},
		{name: "SCF-13/auto_start_only", setup: own(OwnerAuto, camera), ev: cfg(1000, withStart(opening)),
			change: cfgChange(false, withStart(opening)), out: PurposeAutoRecover},
		{name: "SCF-13/auto_recover_pending", setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, "")),
			ev: cfg(1000, camera3), change: cfgChange(false, camera3)},
		{name: "SCF-13/auto_expecting_return", setup: all(own(OwnerAuto, camera), x(1, camera, PurposeAutoRecover, "")),
			ev: cfg(1000, camera3), change: cfgChange(false, camera3)},
		{name: "SCF-13/auto_bad", setup: all(own(OwnerAuto, camera), link(LOW)), ev: cfg(1000, camera3),
			change: cfgChange(false, camera3)},
		{name: "SCF-13/manual", setup: own(OwnerManual, camera), ev: cfg(1000, camera3),
			change: cfgChange(false, camera3)},
		{name: "SCF-14", setup: all(autoOff, link(LOW)), ev: cfg(1000),
			change: cfgChange(false), out: PurposeAutoFail},
		{name: "SCF-15", setup: all(autoOff, own(OwnerNone, "")), ev: cfg(1000), change: cfgChange(false)},
		{name: "SCF-16", setup: all(link(LOW), p(1, PurposeAutoFail, camera)), ev: cfg(1000, withAuto(false)),
			change: cfgChange(false, withAuto(false))},
		{name: "SCF-17/auto_fail_in_flight", setup: all(link(LOW), p(1, PurposeAutoFail, camera), x(1, standby, PurposeAutoFail, camera)),
			ev: cfg(1000, camera3), change: cfgChange(false, camera3)},
		{name: "SCF-17/stop_stream_and_standby_on", setup: all(link(LOW), p(1, PurposeStopStream, ""), x(1, standby, PurposeStandbyOn, camera)),
			ev: cfg(1000, withAuto(false)), change: cfgChange(false, withAuto(false))},
		{name: "SCF-18/pending", setup: all(scn(talk), link(LOW), p(1, PurposeStandbyOn, talk)),
			ev: smc(1000, camera, camera2, talk), change: mapChange(false, camera, camera2, talk)},
		{name: "SCF-18/expectation", setup: all(scn(talk), link(LOW), x(1, standby, PurposeStandbyOn, talk)),
			ev: smc(1000, camera, camera2, talk), change: mapChange(false, camera, camera2, talk)},
		{name: "SCF-19", setup: all(link(LOW), x(1, standby, PurposeAutoFail, camera)), ev: cfg(1000, withStandby(standby2)),
			change: cfgChange(false, withStandby(standby2)), out: PurposeAutoFail, to: standby2, ret: camera},
		{name: "SCF-20", setup: all(link(LOW), x(1, standby, PurposeAutoFail, camera)), ev: cfg(1000, camera3),
			change: cfgChange(false, camera3)},
		{name: "SCF-21/auto_rename", setup: all(own(OwnerAuto, camera), disconnected), ev: cfg(1000, withStandby(standby2)),
			change: cfgChange(true, withStandby(standby2))},
		{name: "SCF-21/manual_rename", setup: all(own(OwnerManual, camera), disconnected), ev: cfg(1000, withStandby(standby2)),
			change: cfgChange(true, withStandby(standby2))},
		{name: "SCF-21/auto_keep", setup: all(own(OwnerAuto, camera), link(OK), disconnected), ev: cfg(1000, camera3),
			change: cfgChange(false, camera3)},
		{name: "SCF-22/add", setup: all(notStreaming, scn(talk), link(LOW)), ev: smc(1000, camera, camera2, talk),
			change: mapChange(false, camera, camera2, talk)},
		{name: "SCF-22/rename", setup: all(notStreaming, scn(standby)), ev: cfg(1000, withStandby(standby2)),
			change: cfgChange(false, withStandby(standby2))},
	}
	runEvRows(t, rows)

	t.Run("SCF-1/epoch_becomes_1", func(t *testing.T) {
		m := newMachine()
		m.Handle(cfg(0))
		if m.st.ConfigEpoch != 1 {
			t.Fatalf("ConfigEpoch = %d, want 1", m.st.ConfigEpoch)
		}
	})
	t.Run("SCF-4/judged_as_monitored", func(t *testing.T) {
		q := newSeq(t).P(talk, true, OK)
		q.none(smc(1000, camera, camera2, talk))
		q.batch(lc(2000, LOW), 1, PurposeAutoFail, standby, talk)
	})
	t.Run("SCF-5/judged_as_other", func(t *testing.T) {
		q := newSeq(t).P(camera, true, OK)
		q.none(smc(1000, camera2))
		q.none(lc(2000, LOW))
		q.none(tick(10000))
	})
	t.Run("SCF-2/copied_input", func(t *testing.T) {
		m := newMachine()
		c := cfgOf(0, withStart(opening))
		m.Handle(c)
		c.Cameras[0].ID = "X"
		*c.StartScene = "Y"
		if m.st.Cameras[0].ID != camMain || *m.st.StartScene != nameOf(opening) {
			t.Fatalf("設定が呼び出し側の値を共有している: %v %v", m.st.Cameras, *m.st.StartScene)
		}
	})
}

func TestRowsLinkChanged(t *testing.T) {
	rows := []evRow{
		{name: "LC-1", ev: lc(1000, LOW), change: linkTo(LOW), out: PurposeAutoFail},
		{name: "LC-2", ev: lc(1000, DOWN), change: linkTo(DOWN), out: PurposeAutoFail},
		{name: "LC-3", setup: all(link(LOW), p(1, PurposeAutoFail, camera)), ev: lc(1000, DOWN), change: linkTo(DOWN)},
		{name: "LC-4", setup: all(link(LOW), x(1, standby, PurposeAutoFail, camera)), ev: lc(1000, DOWN), change: linkTo(DOWN)},
		{name: "LC-5", setup: all(link(LOW), sentAt(PurposeAutoFail, 800)), ev: lc(1000, DOWN), change: linkTo(DOWN), out: PurposeAutoFail},
		{name: "LC-6/LOW", setup: link(LOW), ev: lc(1000, OK), change: linkTo(OK)},
		{name: "LC-6/DOWN", setup: link(DOWN), ev: lc(1000, OK), change: linkTo(OK)},
		{name: "LC-7", setup: all(link(LOW), p(1, PurposeAutoFail, camera)), ev: lc(1000, OK), change: linkTo(OK)},
		{name: "LC-8/LOW", setup: all(own(OwnerAuto, camera), link(LOW)), ev: lc(1000, OK), change: linkTo(OK), out: PurposeAutoRecover},
		{name: "LC-8/DOWN", setup: all(own(OwnerAuto, camera), link(DOWN)), ev: lc(1000, OK), change: linkTo(OK), out: PurposeAutoRecover},
		{name: "LC-9/pending", setup: all(own(OwnerAuto, camera), link(LOW), p(1, PurposeAutoRecover, "")), ev: lc(1000, OK), change: linkTo(OK)},
		{name: "LC-9/expecting", setup: all(own(OwnerAuto, camera), link(LOW), x(1, camera, PurposeAutoRecover, "")), ev: lc(1000, OK), change: linkTo(OK)},
	}
	for _, tr := range [][2]LinkStateT{{LOW, DOWN}, {DOWN, LOW}, {OK, LOW}, {OK, DOWN}} {
		from, to := tr[0], tr[1]
		rows = append(rows, evRow{name: fmt.Sprintf("LC-10/%v_%v", from, to), setup: all(own(OwnerAuto, camera), link(from)),
			ev: lc(1000, to), change: linkTo(to)})
	}
	everyChange := [][2]LinkStateT{{OK, LOW}, {OK, DOWN}, {LOW, DOWN}, {DOWN, LOW}, {LOW, OK}, {DOWN, OK}}
	for _, tr := range everyChange {
		from, to := tr[0], tr[1]
		name := fmt.Sprintf("%v_%v", from, to)
		rows = append(rows,
			evRow{name: "LC-11/" + name, setup: all(own(OwnerNone, ""), link(from)), ev: lc(1000, to), change: linkTo(to)},
			evRow{name: "LC-12/" + name, setup: all(own(OwnerManual, camera), link(from)), ev: lc(1000, to), change: linkTo(to)},
			evRow{name: "LC-13/" + name, setup: all(scn(talk), link(from)), ev: lc(1000, to), change: linkTo(to)},
			evRow{name: "LC-15/" + name, setup: all(own(OwnerAuto, camera), link(from), disconnected), ev: lc(1000, to), change: linkTo(to)},
		)
	}
	rows = append(rows,
		evRow{name: "LC-14", setup: notStreaming, ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-15/monitored", setup: disconnected, ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-16", setup: autoOff, ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-17/pending", setup: p(1, PurposeStandbyOn, camera), ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-17/expectation", setup: x(1, standby, PurposeStandbyOn, camera), ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-18/pending", setup: all(startAt(opening), p(1, PurposeStartScene, "")), ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-18/expectation", setup: all(startAt(opening), x(1, opening, PurposeStartScene, "")), ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-19", setup: link(LOW), ev: lc(1000, LOW)},
		evRow{name: "LC-20/monitored", setup: p(1, PurposeStandbyOffMonitored, ""), ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-20/other", setup: p(1, PurposeStandbyOffOther, ""), ev: lc(1000, LOW), change: linkTo(LOW)},
		evRow{name: "LC-21/stop", setup: p(1, PurposeStopStream, ""), ev: lc(1000, LOW), change: linkTo(LOW), out: PurposeAutoFail},
		evRow{name: "LC-21/start", setup: p(1, PurposeStartStream, ""), ev: lc(1000, LOW), change: linkTo(LOW), out: PurposeAutoFail},
	)
	runEvRows(t, rows)
}

// connected は OBSConnected の共通の変化。
func connected(streaming bool, scene string, tr *int) func(s *state) {
	return func(s *state) {
		if streaming != s.PrevConnStreaming {
			s.StreamEpoch++
		}
		if streaming != s.PrevConnStreaming || scene != s.PrevConnScene || s.PrevConnCollection != collectionX {
			clearOwner(s)
		}
		s.Map = testMap(camera, camera2)
		resolveNames(s)
		s.Connected = true
		s.Streaming = streaming
		s.Scene = scene
		s.TransitionMs = tr
	}
}

func TestRowsOBSConnected(t *testing.T) {
	first := func(s *state) { fresh(s) }
	rows := []evRow{
		{name: "OC-1", setup: first, ev: conn(1000, false, camera, ip(300)), change: connected(false, camera, ip(300))},
		{name: "OC-2/monitored_down", setup: first, ev: conn(1000, true, camera, ip(300)), change: connected(true, camera, ip(300)),
			out: PurposeAutoFail},
		{name: "OC-2/other", setup: first, ev: conn(1000, true, talk, ip(300)), change: connected(true, talk, ip(300))},
		{name: "OC-3", setup: all(link(LOW), disconnected), ev: conn(1000, true, camera, ip(300)),
			change: connected(true, camera, ip(300)), out: PurposeAutoFail},
		{name: "OC-4", setup: all(own(OwnerAuto, camera), disconnected), ev: conn(1000, true, standby, ip(300)),
			change: connected(true, standby, ip(300)), out: PurposeAutoRecover},
		{name: "OC-5/auto_bad", setup: all(own(OwnerAuto, camera), link(LOW), disconnected), ev: conn(1000, true, standby, ip(300)),
			change: connected(true, standby, ip(300))},
		{name: "OC-5/manual", setup: all(own(OwnerManual, camera), disconnected), ev: conn(1000, true, standby, ip(300)),
			change: connected(true, standby, ip(300))},
		{name: "OC-5/standby_none", setup: all(own(OwnerNone, ""), disconnected), ev: conn(1000, true, standby, ip(300)),
			change: connected(true, standby, ip(300))},
		{name: "OC-5/other", setup: all(scn(talk), link(LOW), disconnected), ev: conn(1000, true, talk, ip(300)),
			change: connected(true, talk, ip(300))},
		{name: "OC-5/monitored_ok", setup: disconnected, ev: conn(1000, true, camera, ip(300)),
			change: connected(true, camera, ip(300))},
		{name: "OC-6/auto_bad", setup: all(own(OwnerAuto, camera), link(LOW), disconnected), ev: conn(1000, true, camera2, ip(300)),
			change: connected(true, camera2, ip(300)), out: PurposeAutoFail, ret: camera2},
		{name: "OC-6/manual_ok", setup: all(own(OwnerManual, camera), disconnected), ev: conn(1000, true, camera2, ip(300)),
			change: connected(true, camera2, ip(300))},
		{name: "OC-7/auto", setup: all(own(OwnerAuto, camera), sentAt(PurposeAutoFail, 100), disconnected),
			ev: conn(1000, false, standby, ip(300)), change: connected(false, standby, ip(300))},
		{name: "OC-7/manual", setup: all(own(OwnerManual, camera), disconnected),
			ev: conn(1000, false, standby, ip(300)), change: connected(false, standby, ip(300))},
		{name: "OC-8/ok", setup: all(startAt(opening), notStreaming, disconnected), ev: conn(1000, true, camera, ip(300)),
			change: connected(true, camera, ip(300))},
		{name: "OC-8/bad", setup: all(startAt(opening), notStreaming, link(LOW), disconnected), ev: conn(1000, true, camera, ip(300)),
			change: connected(true, camera, ip(300)), out: PurposeAutoFail},
		{name: "OC-9", setup: all(link(LOW), p(1, PurposeAutoFail, camera), disconnected, func(s *state) { s.ConnEpoch++ }),
			ev: conn(1000, true, camera, ip(300)), change: connected(true, camera, ip(300))},
		{name: "OC-10", setup: all(link(LOW), p(1, PurposeStandbyOn, camera), disconnected, func(s *state) { s.ConnEpoch++ }),
			ev: conn(1000, true, camera, ip(300)), change: connected(true, camera, ip(300))},
		{name: "OC-11", setup: all(scn(talk), disconnected), ev: conn(1000, true, talk, nil), change: connected(true, talk, nil)},
		{name: "OC-12", setup: all(autoOff, link(LOW), disconnected), ev: conn(1000, true, camera, ip(300)),
			change: connected(true, camera, ip(300))},
	}
	runEvRows(t, rows)
}

// disconnectedChange は OBSDisconnected の共通の変化。
func disconnectedChange(s *state) {
	disconnected(s)
	s.ConnEpoch++
}

func TestRowsOBSDisconnected(t *testing.T) {
	rows := []evRow{
		{name: "OD-1", setup: all(link(LOW), p(1, PurposeAutoFail, camera), x(1, standby, PurposeAutoFail, camera)),
			ev: disc(1000), change: disconnectedChange},
		{name: "OD-2/auto", setup: all(own(OwnerAuto, camera), sentAt(PurposeAutoFail, 100)), ev: disc(1000), change: disconnectedChange},
		{name: "OD-2/manual", setup: own(OwnerManual, talk), ev: disc(1000), change: disconnectedChange},
		{name: "OD-3", setup: all(startAt(opening), x(1, opening, PurposeStartScene, "")), ev: disc(1000), change: disconnectedChange},
	}
	runEvRows(t, rows)
}

// arrived は期待どおりの SceneChanged の共通の変化（期待の列は keep にする）。
func arrived(scene string, keep ...Expectation) func(s *state) {
	return func(s *state) {
		s.Scene = scene
		s.Expectations = keep
	}
}

func setOwner(o Owner, ret string) func(s *state) {
	return func(s *state) { s.StandbyOwner, s.ReturnScene = o, ret }
}

func chain(fns ...func(s *state)) func(s *state) { return all(fns...) }

// oldExp は1つ前の設定の世代で登録した期待を足す。
func oldExp(req int, scene string, pp Purpose, ret string) setupFn {
	return func(s *state) {
		e := answered(waiting(s, req, scene, pp, ret), RequestOK, 0, 60000)
		e.ConfigEpoch = s.ConfigEpoch - 1
		s.Expectations = append(s.Expectations, e)
	}
}

func TestRowsSceneChangedExpected(t *testing.T) {
	var fromStart Expectation
	{
		s := state{ConfigEpoch: 5}
		fromStart = answered(waiting(&s, 2, opening, PurposeStartScene, ""), RequestOK, 0, 60000)
	}
	rows := []evRow{
		{name: "SCE-1", setup: all(link(LOW), x(1, standby, PurposeAutoFail, camera)), ev: sc(1000, standby),
			change: chain(arrived(standby), setOwner(OwnerAuto, camera))},
		{name: "SCE-2", setup: x(1, standby, PurposeAutoFail, camera), ev: sc(1000, standby),
			change: chain(arrived(standby), setOwner(OwnerAuto, camera)), out: PurposeAutoRecover},
		{name: "SCE-3/monitored_bad", setup: all(link(LOW), x(1, standby, PurposeStandbyOn, camera)), ev: sc(1000, standby),
			change: chain(arrived(standby), setOwner(OwnerManual, camera))},
		{name: "SCE-3/other_ok", setup: all(scn(talk), x(1, standby, PurposeStandbyOn, talk)), ev: sc(1000, standby),
			change: chain(arrived(standby), setOwner(OwnerManual, talk))},
		{name: "SCE-4", setup: all(own(OwnerAuto, camera), x(1, camera, PurposeAutoRecover, "")), ev: sc(1000, camera),
			change: chain(arrived(camera), clearOwner)},
		{name: "SCE-5", setup: all(own(OwnerAuto, camera), link(LOW), x(1, camera, PurposeAutoRecover, "")), ev: sc(1000, camera),
			change: chain(arrived(camera), clearOwner), out: PurposeAutoFail, ret: camera},
		{name: "SCE-6", setup: all(own(OwnerManual, camera), x(1, camera, PurposeStandbyOffMonitored, "")), ev: sc(1000, camera),
			change: chain(arrived(camera), clearOwner)},
		{name: "SCE-7", setup: all(own(OwnerManual, camera), link(LOW), x(1, camera, PurposeStandbyOffMonitored, "")), ev: sc(1000, camera),
			change: chain(arrived(camera), clearOwner), out: PurposeAutoFail, ret: camera},
		{name: "SCE-8", setup: all(own(OwnerManual, camera), link(LOW), p(1, PurposeStandbyOffMonitored, ""), x(1, camera, PurposeStandbyOffMonitored, "")),
			ev: sc(1000, camera), change: chain(arrived(camera), clearOwner)},
		{name: "SCE-9/bad", setup: all(own(OwnerManual, talk), link(LOW), x(1, talk, PurposeStandbyOffOther, "")), ev: sc(1000, talk),
			change: chain(arrived(talk), clearOwner)},
		{name: "SCE-9/ok", setup: all(own(OwnerManual, talk), x(1, talk, PurposeStandbyOffOther, "")), ev: sc(1000, talk),
			change: chain(arrived(talk), clearOwner)},
		{name: "SCE-10", setup: all(startAt(opening), link(LOW), x(1, opening, PurposeStartScene, "")), ev: sc(1000, opening),
			change: arrived(opening)},
		{name: "SCE-11", setup: all(startAt(camera2), scn(talk), link(LOW), x(1, camera2, PurposeStartScene, "")), ev: sc(1000, camera2),
			change: arrived(camera2), out: PurposeAutoFail, ret: camera2},
		{name: "SCE-12", setup: all(startAt(camera2), scn(talk), link(LOW), p(1, PurposeStartScene, ""), x(1, camera2, PurposeStartScene, "")),
			ev: sc(1000, camera2), change: arrived(camera2)},
		{name: "SCE-13/auto_fail", setup: oldExp(1, standby, PurposeAutoFail, camera), ev: sc(1000, standby), change: arrived(standby)},
		{name: "SCE-13/standby_on", setup: oldExp(1, standby, PurposeStandbyOn, camera), ev: sc(1000, standby), change: arrived(standby)},
		{name: "SCE-14/auto_recover", setup: all(own(OwnerAuto, camera), oldExp(1, camera, PurposeAutoRecover, "")), ev: sc(1000, camera),
			change: chain(arrived(camera), clearOwner)},
		{name: "SCE-14/auto_recover_bad", setup: all(own(OwnerAuto, camera), link(LOW), oldExp(1, camera, PurposeAutoRecover, "")),
			ev: sc(1000, camera), change: chain(arrived(camera), clearOwner), out: PurposeAutoFail, ret: camera},
		{name: "SCE-14/standby_off_monitored", setup: all(own(OwnerManual, camera), oldExp(1, camera, PurposeStandbyOffMonitored, "")),
			ev: sc(1000, camera), change: chain(arrived(camera), clearOwner)},
		{name: "SCE-14/standby_off_other", setup: all(own(OwnerManual, talk), oldExp(1, talk, PurposeStandbyOffOther, "")),
			ev: sc(1000, talk), change: chain(arrived(talk), clearOwner)},
		{name: "SCE-15", setup: all(startAt(opening), link(LOW), x(1, opening, PurposeStartScene, ""), x(2, standby, PurposeAutoFail, camera)),
			ev: sc(1000, standby), change: chain(arrived(standby), setOwner(OwnerAuto, camera))},
		{name: "SCE-16", setup: all(startAt(opening), link(LOW), x(1, standby, PurposeAutoFail, camera), func(s *state) {
			s.Expectations = append(s.Expectations, fromStart)
		}), ev: sc(1000, standby), change: chain(arrived(standby, fromStart), setOwner(OwnerAuto, camera))},
		{name: "SCE-17", setup: all(own(OwnerManual, talk), x(1, talk, PurposeStandbyOffOther, ""), x(2, talk, PurposeStandbyOffOther, "")),
			ev: sc(1000, talk), change: chain(arrived(talk), clearOwner)},
		{name: "SCE-17/later_other_scene_kept", setup: all(link(LOW), x(1, standby, PurposeAutoFail, camera), x(3, standby, PurposeAutoFail, camera), func(s *state) {
			e := s.Expectations
			s.Expectations = []Expectation{e[0], fromStart, e[1]}
		}), ev: sc(1000, standby), change: chain(arrived(standby, fromStart), setOwner(OwnerAuto, camera))},
		{name: "SCE-18/standby_on", setup: all(autoOff, link(LOW), x(1, standby, PurposeStandbyOn, camera)), ev: sc(1000, standby),
			change: chain(arrived(standby), setOwner(OwnerManual, camera))},
		{name: "SCE-18/standby_off_monitored", setup: all(autoOff, own(OwnerManual, camera), link(LOW), x(1, camera, PurposeStandbyOffMonitored, "")),
			ev: sc(1000, camera), change: chain(arrived(camera), clearOwner)},
		{name: "SCE-18/standby_off_other", setup: all(autoOff, own(OwnerManual, talk), link(LOW), x(1, talk, PurposeStandbyOffOther, "")),
			ev: sc(1000, talk), change: chain(arrived(talk), clearOwner)},
	}
	runEvRows(t, rows)
}

func manualTo(scene string) func(s *state) { return func(s *state) { manualChange(s, scene) } }

func TestRowsSceneChangedManual(t *testing.T) {
	rows := []evRow{
		{name: "SCM-1", setup: scn(talk), ev: sc(1000, camera2), change: manualTo(camera2)},
		{name: "SCM-2", setup: all(scn(talk), link(LOW)), ev: sc(1000, camera), change: manualTo(camera), out: PurposeAutoFail},
		{name: "SCM-3", setup: all(own(OwnerAuto, camera), link(LOW), sentAt(PurposeAutoFail, 900)), ev: sc(1000, camera2),
			change: manualTo(camera2), out: PurposeAutoFail, ret: camera2},
		{name: "SCM-4", setup: all(own(OwnerManual, camera2), link(LOW)), ev: sc(1000, camera), change: manualTo(camera), out: PurposeAutoFail},
		{name: "SCM-5", setup: all(own(OwnerNone, ""), link(LOW)), ev: sc(1000, camera), change: manualTo(camera), out: PurposeAutoFail},
		{name: "SCM-6/ok", ev: sc(1000, standby), change: manualTo(standby)},
		{name: "SCM-6/bad", setup: link(LOW), ev: sc(1000, standby), change: manualTo(standby)},
		{name: "SCM-7/auto", setup: all(own(OwnerAuto, camera), link(LOW)), ev: sc(1000, talk), change: manualTo(talk)},
		{name: "SCM-7/manual", setup: own(OwnerManual, camera), ev: sc(1000, talk), change: manualTo(talk)},
		{name: "SCM-8", setup: all(scn(talk), link(LOW), p(1, PurposeAutoFail, talk), sentAt(PurposeAutoFail, 900)),
			ev: sc(1000, camera), change: manualTo(camera)},
		{name: "SCM-9/standby_on", setup: all(scn(talk), link(LOW), p(1, PurposeStandbyOn, talk)), ev: sc(1000, camera), change: manualTo(camera)},
		{name: "SCM-9/start_scene", setup: all(startAt(opening), scn(talk), link(LOW), p(1, PurposeStartScene, "")), ev: sc(1000, camera),
			change: manualTo(camera)},
		{name: "SCM-10", setup: all(autoOff, scn(talk), link(LOW)), ev: sc(1000, camera), change: manualTo(camera)},
		{name: "SCM-11/monitored", setup: all(notStreaming, scn(talk), link(LOW)), ev: sc(1000, camera), change: manualTo(camera)},
		{name: "SCM-11/standby", setup: all(notStreaming, scn(talk), link(LOW)), ev: sc(1000, standby), change: manualTo(standby)},
		{name: "SCM-12", setup: all(link(LOW), x(1, standby, PurposeAutoFail, camera)), ev: sc(1000, camera2),
			change: manualTo(camera2), out: PurposeAutoFail, ret: camera2},
		{name: "SCM-13", setup: all(link(LOW), func(s *state) {
			s.Expectations = []Expectation{answered(waiting(s, 1, standby, PurposeAutoFail, camera), RequestOK, 0, 900)}
		}), ev: sc(1000, standby), change: manualTo(standby)},
		{name: "SCM-14/monitored", ev: sc(1000, camera), change: manualTo(camera)},
		{name: "SCM-14/auto_standby", setup: own(OwnerAuto, camera), ev: sc(1000, standby), change: manualTo(standby)},
		{name: "SCM-15", setup: all(startAt(opening), x(1, opening, PurposeStartScene, "")), ev: sc(1000, talk), change: manualTo(talk)},
		{name: "SCM-16", setup: all(scn(camera2), link(LOW), p(2, PurposeAutoFail, camera2)), ev: sc(1000, standby), change: manualTo(standby)},
	}
	runEvRows(t, rows)
}

func TestRowsTransitionChanged(t *testing.T) {
	runEvRows(t, []evRow{
		{name: "TC-1", ev: tc(1000, ip(5000)), change: func(s *state) { s.TransitionMs = ip(5000) }},
		{name: "TC-2", ev: tc(1000, nil), change: func(s *state) { s.TransitionMs = nil }},
		{name: "TC-2/no_judge", setup: link(LOW), ev: tc(1000, nil), change: func(s *state) { s.TransitionMs = nil }},
	})
	t.Run("TC-3", func(t *testing.T) {
		m := newMachine()
		baseState(&m.st)
		m.st.CameraStates[camMain] = LOW
		m.st.Expectations = []Expectation{waiting(&m.st, 1, standby, PurposeAutoFail, camera)}
		if b, _ := m.Handle(tc(500, ip(5000))); b != nil {
			t.Fatalf("batch %s", showBatch(b))
		}
		m.Handle(srd(1000, 1, RequestOK))
		if got := m.st.Expectations[0].Deadline; !got.Equal(at(4300)) {
			t.Fatalf("deadline = %s, want t0+4.3s", show(got))
		}
	})
}

func streamOn(s *state) {
	s.Streaming = true
	s.StreamEpoch++
}

func streamOff(s *state) {
	s.Streaming = false
	s.StreamEpoch++
	clearOwner(s)
	s.Expectations = nil
	s.LastAutoSent = map[Purpose]time.Time{}
}

func TestRowsStreamStateChanged(t *testing.T) {
	rows := []evRow{
		{name: "SST-1", setup: all(notStreaming, link(LOW)), ev: ssc(1000, true), change: streamOn, out: PurposeAutoFail},
		{name: "SST-2/monitored_ok", setup: notStreaming, ev: ssc(1000, true), change: streamOn},
		{name: "SST-2/other", setup: all(notStreaming, scn(talk), link(LOW)), ev: ssc(1000, true), change: streamOn},
		{name: "SST-2/standby", setup: all(notStreaming, scn(standby), link(OK)), ev: ssc(1000, true), change: streamOn},
		{name: "SST-3/monitored_bad", setup: all(notStreaming, startAt(opening), link(LOW), sentAt(PurposeAutoFail, 100)),
			ev: ssc(1000, true), change: streamOn, out: PurposeStartScene},
		{name: "SST-3/other_ok", setup: all(notStreaming, startAt(opening), scn(talk)), ev: ssc(1000, true), change: streamOn, out: PurposeStartScene},
		{name: "SST-3/standby", setup: all(notStreaming, startAt(opening), scn(standby), link(LOW)), ev: ssc(1000, true), change: streamOn,
			out: PurposeStartScene},
		{name: "SST-4", setup: all(notStreaming, startAt(opening), autoOff, link(LOW)), ev: ssc(1000, true), change: streamOn, out: PurposeStartScene},
		{name: "SST-5", setup: all(notStreaming, startAt(opening), scn(opening)), ev: ssc(1000, true), change: streamOn, out: PurposeStartScene},
		{name: "SST-6", setup: all(notStreaming, startAt(opening), link(LOW), p(1, PurposeStartScene, "")), ev: ssc(1000, true), change: streamOn},
		{name: "SST-7", setup: all(notStreaming, autoOff, link(LOW)), ev: ssc(1000, true), change: streamOn},
		{name: "SST-8", setup: link(LOW), ev: ssc(1000, true)},
		{name: "SSF-1/auto", setup: all(own(OwnerAuto, camera), x(1, camera, PurposeAutoRecover, ""), sentAt(PurposeAutoFail, 100)),
			ev: ssc(1000, false), change: streamOff},
		{name: "SSF-1/manual", setup: all(own(OwnerManual, talk), x(1, talk, PurposeStandbyOffOther, "")), ev: ssc(1000, false), change: streamOff},
		{name: "SSF-2", setup: all(link(LOW), p(1, PurposeAutoFail, camera), p(2, PurposeStopStream, ""), x(1, standby, PurposeAutoFail, camera)),
			ev: ssc(1000, false), change: streamOff},
		{name: "SSF-3", setup: all(startAt(opening), x(1, opening, PurposeStartScene, "")), ev: ssc(1000, false), change: streamOff},
		{name: "SSF-4", setup: all(notStreaming, x(1, opening, PurposeStartScene, "")), ev: ssc(1000, false)},
	}
	runEvRows(t, rows)
}

func TestRowsSceneRequestDone(t *testing.T) {
	waitR := func(req int, scene string, pp Purpose) setupFn {
		return func(s *state) { s.Expectations = append(s.Expectations, waiting(s, req, scene, pp, "")) }
	}
	respond := func(i int, res RequestResult, respMs, dlMs int) func(s *state) {
		return func(s *state) { s.Expectations[i] = answered(s.Expectations[i], res, respMs, dlMs) }
	}
	drop := func(i int) func(s *state) {
		return func(s *state) { s.Expectations = append(s.Expectations[:i:i], s.Expectations[i+1:]...) }
	}
	rows := []evRow{
		{name: "SRD-OK-1", setup: waitR(1, standby, PurposeAutoFail), ev: srd(1000, 1, RequestOK), change: respond(0, RequestOK, 1000, 4300)},
		{name: "SRD-OK-2", setup: waitR(1, standby, PurposeAutoFail), ev: srd(1000, 7, RequestOK)},
		{name: "SRD-OK-2/no_judge", setup: link(LOW), ev: srd(1000, 7, RequestOK)},
		{name: "SRD-TO-1", setup: waitR(1, standby, PurposeAutoFail), ev: srd(1000, 1, RequestTimeout), change: respond(0, RequestTimeout, 1000, 4300)},
		{name: "SRD-ERR-1", setup: all(waitR(1, standby, PurposeAutoFail), waitR(2, camera, PurposeAutoRecover)), ev: srd(1000, 1, RequestError),
			change: drop(0)},
		{name: "SRD-ERR-1/later", setup: all(waitR(1, standby, PurposeAutoFail), waitR(2, camera, PurposeAutoRecover)), ev: srd(1000, 2, RequestError),
			change: drop(1)},
		{name: "SRD-ERR-2", setup: all(startAt(opening), waitR(1, opening, PurposeStartScene)), ev: srd(1000, 1, RequestError), change: drop(0)},
		{name: "SRD-ERR-3", setup: waitR(1, standby, PurposeAutoFail), ev: srd(1000, 9, RequestError)},
	}
	runEvRows(t, rows)
}

func TestRowsDeadline(t *testing.T) {
	cases := []struct {
		name string
		ov   Override
		tr   *int
		dlMs int
	}{
		{"DL-1", OverrideNone{}, ip(300), 5300},
		{"DL-1/long", OverrideNone{}, ip(5000), 10000},
		{"DL-2", OverrideNone{}, nil, 12000},
		{"DL-3/short", OverrideKnown{Ms: 500}, ip(300), 12000},
		{"DL-3/long", OverrideKnown{Ms: 9000}, ip(300), 14000},
		{"DL-3/no_global", OverrideKnown{Ms: 500}, nil, 12000},
		{"DL-4/short", OverrideUnknown{}, ip(300), 12000},
		{"DL-4/long", OverrideUnknown{}, ip(9000), 14000},
		{"DL-5", OverrideUnknown{}, nil, 12000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newMachine()
			baseState(&m.st)
			e := waiting(&m.st, 1, standby, PurposeAutoFail, camera)
			e.Override, e.TransitionMs = c.ov, c.tr
			m.st.Expectations = []Expectation{e}
			m.Handle(srd(2000, 1, RequestOK))
			if got := m.st.Expectations[0].Deadline; !got.Equal(at(c.dlMs)) {
				t.Fatalf("deadline = %s, want %s", show(got), show(at(c.dlMs)))
			}
		})
	}
}

func TestRowsBatchDone(t *testing.T) {
	gone := func(id int) func(s *state) { return func(s *state) { delete(s.Pending, id) } }
	restartErr := errors.New("restart failed")
	bdErr := func(ms, id int, r BatchResult, err error) Event {
		return BatchDone{At: at(ms), BatchID: id, Result: r, Err: err}
	}
	rows := []evRow{}
	every := []Purpose{PurposeAutoFail, PurposeAutoRecover, PurposeStandbyOn, PurposeStandbyOffMonitored, PurposeStandbyOffOther,
		PurposeStartScene, PurposeStartStream, PurposeStopStream}
	for _, pp := range every {
		rows = append(rows, evRow{name: "BD-C-1/" + pp.String(), setup: all(startAt(opening), p(1, pp, "")), ev: bd(1000, 1, BatchCompleted), change: gone(1)})
	}
	rows = append(rows,
		evRow{name: "BD-C-2/AutoRecover", setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, "")),
			ev: bdErr(1000, 1, BatchCompleted, restartErr), change: gone(1), recErr: true},
		evRow{name: "BD-C-2/StandbyOffMonitored", setup: all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, "")),
			ev: bdErr(1000, 1, BatchCompleted, restartErr), change: gone(1), recErr: true},
		evRow{name: "BD-F-1/AutoFail", setup: all(link(LOW), p(1, PurposeAutoFail, camera)), ev: bd(1000, 1, BatchFailed), change: gone(1), recErr: true},
		evRow{name: "BD-F-1/AutoRecover", setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, "")), ev: bd(1000, 1, BatchFailed),
			change: gone(1), recErr: true},
		evRow{name: "BD-F-2/StandbyOn", setup: p(1, PurposeStandbyOn, camera), ev: bd(1000, 1, BatchFailed), change: gone(1), recErr: true},
		evRow{name: "BD-F-2/StandbyOffMonitored", setup: all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, "")),
			ev: bd(1000, 1, BatchFailed), change: gone(1), recErr: true},
		evRow{name: "BD-F-2/StandbyOffOther", setup: all(own(OwnerManual, talk), p(1, PurposeStandbyOffOther, "")),
			ev: bd(1000, 1, BatchFailed), change: gone(1), recErr: true},
		evRow{name: "BD-F-2/StartStream", setup: all(notStreaming, p(1, PurposeStartStream, "")), ev: bd(1000, 1, BatchFailed), change: gone(1), recErr: true},
		evRow{name: "BD-F-2/StopStream", setup: p(1, PurposeStopStream, ""), ev: bd(1000, 1, BatchFailed), change: gone(1), recErr: true},
		evRow{name: "BD-F-3", setup: all(startAt(opening), p(1, PurposeStartScene, "")), ev: bd(1000, 1, BatchFailed), change: gone(1), recErr: true},
		evRow{name: "BD-R-1/AutoFail", setup: p(1, PurposeAutoFail, camera), ev: bd(1000, 1, BatchRejected), change: gone(1)},
		evRow{name: "BD-R-1/AutoRecover", setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, "")), ev: bd(1000, 1, BatchRejected), change: gone(1)},
	)
	for _, pp := range []Purpose{PurposeStandbyOn, PurposeStandbyOffMonitored, PurposeStandbyOffOther, PurposeStartStream, PurposeStopStream} {
		rows = append(rows, evRow{name: "BD-R-2/" + pp.String(), setup: p(1, pp, ""), ev: bd(1000, 1, BatchRejected), change: gone(1), recErr: true})
	}
	replaced := func(s *state) {
		pb := s.Pending[1]
		pb.Replaced = true
		s.Pending[1] = pb
	}
	manualRej := func(s *state) {
		pb := s.Pending[1]
		pb.ManualRejected = true
		s.Pending[1] = pb
	}
	rows = append(rows,
		evRow{name: "BD-R-3", setup: all(own(OwnerAuto, camera), link(LOW), p(1, PurposeStandbyOffMonitored, ""), replaced),
			ev: bd(1000, 1, BatchRejected), change: gone(1)},
		evRow{name: "BD-R-4", setup: all(startAt(opening), p(1, PurposeStartScene, ""), manualRej), ev: bd(1000, 1, BatchRejected), change: gone(1)},
		evRow{name: "BD-R-5", setup: all(startAt(opening), p(1, PurposeStartScene, "")), ev: bd(1000, 1, BatchRejected), change: gone(1), recErr: true},
		evRow{name: "BD-J-1", setup: all(link(LOW), p(1, PurposeStandbyOn, camera)), ev: bd(1000, 1, BatchCompleted), change: gone(1)},
		evRow{name: "BD-J-1/start_scene", setup: all(startAt(camera), link(LOW), p(1, PurposeStartScene, "")), ev: bd(1000, 1, BatchCompleted),
			change: gone(1)},
		evRow{name: "BD-X-1", setup: p(1, PurposeAutoFail, camera), ev: bd(1000, 9, BatchFailed)},
	)
	runEvRows(t, rows)

	t.Run("BD-R-6", func(t *testing.T) {
		m := newMachine()
		baseState(&m.st)
		m.st.StartScene = sp(opening)
		pend(&m.st, 1, PurposeStartScene, "")
		m.st.SceneEpoch++
		m.st.Streaming = false
		m.st.StreamEpoch++
		if got := m.Approve(1, set(opening), OverrideNone{}, at(900)); got != (Reject{}) {
			t.Fatalf("Approve = %#v, want Reject", got)
		}
		if !m.st.Pending[1].ManualRejected {
			t.Fatalf("ManualRejected が付いていない")
		}
		m.Handle(bd(1000, 1, BatchRejected))
		if m.st.LastError != nil {
			t.Fatalf("LastError = %s, want none", show(m.st.LastError))
		}
	})
}

func expired(req int, scene string, pp Purpose, ret string, dlMs int) setupFn {
	return func(s *state) {
		s.Expectations = append(s.Expectations, answered(waiting(s, req, scene, pp, ret), RequestOK, 0, dlMs))
	}
}

func dropExps(s *state) { s.Expectations = nil }

func TestRowsTick(t *testing.T) {
	tk1 := all(link(LOW), sentAt(PurposeAutoFail, 1000))
	tk6 := all(own(OwnerAuto, camera), sentAt(PurposeAutoRecover, 1000))
	rows := []evRow{
		{name: "TK-1", setup: tk1, ev: tick(6000), out: PurposeAutoFail},
		{name: "TK-2", setup: tk1, ev: tick(5900)},
		{name: "TK-3", setup: all(tk1, p(1, PurposeAutoFail, camera)), ev: tick(6000)},
		{name: "TK-4/waiting", setup: all(tk1, func(s *state) {
			s.Expectations = []Expectation{waiting(s, 1, standby, PurposeAutoFail, camera)}
		}), ev: tick(6000)},
		{name: "TK-4/answered", setup: all(tk1, expired(1, standby, PurposeAutoFail, camera, 7000)), ev: tick(6000)},
		{name: "TK-5", setup: link(LOW), ev: tick(6000), out: PurposeAutoFail},
		{name: "TK-6", setup: tk6, ev: tick(6000), out: PurposeAutoRecover},
		{name: "TK-6/never_sent", setup: own(OwnerAuto, camera), ev: tick(6000), out: PurposeAutoRecover},
		{name: "TK-7", setup: tk6, ev: tick(5999)},
		{name: "TK-8/auto_fail", setup: all(tk1, p(1, PurposeStandbyOn, camera)), ev: tick(6000)},
		{name: "TK-8/auto_recover", setup: all(tk6, startAt(opening), x(1, opening, PurposeStartScene, "")), ev: tick(6000)},
		{name: "TK-9/auto_disabled", setup: all(tk1, autoOff), ev: tick(6000)},
		{name: "TK-9/not_streaming", setup: all(tk1, notStreaming), ev: tick(6000)},
		{name: "TK-9/disconnected", setup: all(tk1, disconnected), ev: tick(6000)},
		{name: "TK-9/other", setup: all(tk1, scn(talk)), ev: tick(6000)},
		{name: "TK-9/standby_none", setup: all(own(OwnerNone, "")), ev: tick(6000)},
		{name: "TK-9/standby_manual", setup: all(own(OwnerManual, camera)), ev: tick(6000)},
		{name: "TK-9/monitored_ok", ev: tick(6000)},
		{name: "TK-9/auto_bad", setup: all(own(OwnerAuto, camera), link(LOW)), ev: tick(6000)},
		{name: "TK-10/auto_fail", setup: all(tk1, expired(1, standby, PurposeAutoFail, camera, 5000)), ev: tick(6000),
			change: dropExps, out: PurposeAutoFail},
		{name: "TK-10/auto_recover", setup: all(tk6, expired(1, camera, PurposeAutoRecover, "", 5000)), ev: tick(6000),
			change: dropExps, out: PurposeAutoRecover},
		{name: "TK-11", setup: all(tk1, expired(1, standby, PurposeAutoFail, camera, 6000)), ev: tick(6000)},
		{name: "TK-12", setup: all(tk1, func(s *state) {
			s.Expectations = []Expectation{waiting(s, 1, standby, PurposeAutoFail, camera)}
		}), ev: tick(100000)},
		{name: "TK-13", setup: all(startAt(opening), link(LOW), expired(1, opening, PurposeStartScene, "", 5000)), ev: tick(6000),
			change: dropExps, recErr: true, out: PurposeAutoFail},
		{name: "TK-14", setup: all(startAt(opening), scn(talk)), ev: tick(6000)},
		{name: "TK-14/monitored_bad", setup: all(startAt(opening), link(LOW)), ev: tick(6000), out: PurposeAutoFail},
		{name: "TK-15/standby_on", setup: all(link(LOW), expired(1, standby, PurposeStandbyOn, camera, 5000)), ev: tick(6000),
			change: dropExps, recErr: true, out: PurposeAutoFail},
		{name: "TK-15/standby_off_monitored", setup: all(own(OwnerManual, camera), expired(1, camera, PurposeStandbyOffMonitored, "", 5000)),
			ev: tick(6000), change: dropExps, recErr: true},
		{name: "TK-15/standby_off_other", setup: all(own(OwnerManual, talk), expired(1, talk, PurposeStandbyOffOther, "", 5000)),
			ev: tick(6000), change: dropExps, recErr: true},
	}
	runEvRows(t, rows)
}
