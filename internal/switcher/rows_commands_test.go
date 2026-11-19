package switcher

import "testing"

var (
	notConnected = ErrNotConnected{}
	invalid      = ErrInvalidState{Reason: "x"}
)

func TestRowsCommandStreamStart(t *testing.T) {
	ev := cmd(1000, CmdStreamStart)
	runEvRows(t, []evRow{
		{name: "CMD-START-1", setup: disconnected, ev: ev, err: notConnected},
		{name: "CMD-START-2", ev: ev, err: invalid, reason: ReasonStreaming},
		{name: "CMD-START-2/expire_first", setup: all(startAt(opening), expired(1, opening, PurposeStartScene, "", 500)),
			ev: ev, err: invalid, change: dropExps, recErr: true},
		{name: "CMD-START-3/monitored_bad", setup: all(notStreaming, link(LOW)), ev: ev, out: PurposeStartStream},
		{name: "CMD-START-3/other_auto_off_start", setup: all(notStreaming, scn(talk), autoOff, startAt(opening)), ev: ev, out: PurposeStartStream},
		{name: "CMD-START-3/standby", setup: all(notStreaming, scn(standby)), ev: ev, out: PurposeStartStream},
		{name: "CMD-START-4", setup: all(notStreaming, p(1, PurposeStartStream, "")), ev: ev, err: invalid, reason: ReasonSamePending},
	})
}

func TestRowsCommandStreamStop(t *testing.T) {
	ev := cmd(1000, CmdStreamStop)
	runEvRows(t, []evRow{
		{name: "CMD-STOP-1", setup: disconnected, ev: ev, err: notConnected},
		{name: "CMD-STOP-2", setup: notStreaming, ev: ev, err: invalid, reason: ReasonNotStreaming},
		{name: "CMD-STOP-3/monitored_bad", setup: link(LOW), ev: ev, out: PurposeStopStream},
		{name: "CMD-STOP-3/auto_standby", setup: own(OwnerAuto, camera), ev: ev, out: PurposeStopStream},
		{name: "CMD-STOP-3/manual_standby", setup: own(OwnerManual, talk), ev: ev, out: PurposeStopStream},
		{name: "CMD-STOP-4", setup: p(1, PurposeStopStream, ""), ev: ev, err: invalid, reason: ReasonSamePending},
	})
}

func TestRowsCommandStandbyOn(t *testing.T) {
	ev := cmd(1000, CmdStandbyOn)
	timedOut := func(s *state) {
		s.Expectations = []Expectation{answered(waiting(s, 1, standby, PurposeAutoFail, camera), RequestTimeout, 0, 60000)}
	}
	runEvRows(t, []evRow{
		{name: "CMD-SON-1", setup: disconnected, ev: ev, err: notConnected},
		{name: "CMD-SON-2", setup: notStreaming, ev: ev, err: invalid, reason: ReasonNotStreaming},
		{name: "CMD-SON-3", setup: own(OwnerAuto, camera), ev: ev, err: invalid, reason: ReasonAlreadyStandby},
		{name: "CMD-SON-4", setup: own(OwnerNone, ""), ev: ev, err: invalid, reason: ReasonAlreadyStandby},
		{name: "CMD-SON-5", setup: own(OwnerManual, camera), ev: ev, err: invalid, reason: ReasonAlreadyStandby},
		{name: "CMD-SON-6/start_scene", setup: all(startAt(opening), p(1, PurposeStartScene, "")), ev: ev, err: invalid, reason: ReasonSwitchInProcess},
		{name: "CMD-SON-6/auto_fail", setup: all(link(LOW), p(1, PurposeAutoFail, camera)), ev: ev, err: invalid, reason: ReasonSwitchInProcess},
		{name: "CMD-SON-6/auto_recover", setup: p(1, PurposeAutoRecover, ""), ev: ev, err: invalid, reason: ReasonSwitchInProcess},
		{name: "CMD-SON-7/timed_out", setup: timedOut, ev: ev, err: invalid, reason: ReasonSwitchInProcess},
		{name: "CMD-SON-7/start_scene", setup: all(startAt(opening), x(1, opening, PurposeStartScene, "")), ev: ev, err: invalid,
			reason: ReasonSwitchInProcess},
		{name: "CMD-SON-7/standby_off", setup: x(1, camera, PurposeStandbyOffMonitored, ""), ev: ev, err: invalid, reason: ReasonSwitchInProcess},
		{name: "CMD-SON-8", setup: p(1, PurposeStandbyOn, camera), ev: ev, err: invalid, reason: ReasonSamePending},
		{name: "CMD-SON-9/ok", ev: ev, out: PurposeStandbyOn},
		{name: "CMD-SON-9/bad", setup: link(DOWN), ev: ev, out: PurposeStandbyOn},
		{name: "CMD-SON-10", setup: all(scn(talk), link(LOW)), ev: ev, out: PurposeStandbyOn, ret: talk},
		{name: "CMD-SON-11", setup: all(autoOff, link(LOW)), ev: ev, out: PurposeStandbyOn},
		{name: "CMD-SON-12/stop_stream", setup: p(1, PurposeStopStream, ""), ev: ev, out: PurposeStandbyOn},
		{name: "CMD-SON-12/standby_off_monitored", setup: p(1, PurposeStandbyOffMonitored, ""), ev: ev, out: PurposeStandbyOn},
		{name: "CMD-SON-12/standby_off_other", setup: p(1, PurposeStandbyOffOther, ""), ev: ev, out: PurposeStandbyOn},
	})
}

func TestRowsCommandStandbyOff(t *testing.T) {
	ev := cmd(1000, CmdStandbyOff)
	toAuto := func(s *state) { s.StandbyOwner = OwnerAuto }
	timedOutTo := func(scene string, pp Purpose) setupFn {
		return func(s *state) {
			s.Expectations = []Expectation{answered(waiting(s, 1, scene, pp, ""), RequestTimeout, 0, 60000)}
		}
	}
	runEvRows(t, []evRow{
		{name: "CMD-SOFF-1", setup: all(own(OwnerManual, camera), disconnected), ev: ev, err: notConnected},
		{name: "CMD-SOFF-2", setup: notStreaming, ev: ev, err: invalid, reason: ReasonNotStreaming},
		{name: "CMD-SOFF-3/monitored", ev: ev, err: invalid, reason: ReasonNotManual},
		{name: "CMD-SOFF-3/other", setup: scn(talk), ev: ev, err: invalid, reason: ReasonNotManual},
		{name: "CMD-SOFF-3/standby", setup: own(OwnerNone, ""), ev: ev, err: invalid, reason: ReasonNotManual},
		{name: "CMD-SOFF-4", setup: own(OwnerAuto, camera), ev: ev, err: invalid, reason: ReasonNotManual},
		{name: "CMD-SOFF-5", setup: own(OwnerManual, camera), ev: ev, out: PurposeStandbyOffMonitored},
		{name: "CMD-SOFF-6/LOW", setup: all(own(OwnerManual, camera), link(LOW)), ev: ev, change: toAuto},
		{name: "CMD-SOFF-6/DOWN", setup: all(own(OwnerManual, camera), link(DOWN)), ev: ev, change: toAuto},
		{name: "CMD-SOFF-7/bad", setup: all(own(OwnerManual, talk), link(LOW)), ev: ev, out: PurposeStandbyOffOther},
		{name: "CMD-SOFF-7/ok", setup: own(OwnerManual, talk), ev: ev, out: PurposeStandbyOffOther},
		{name: "CMD-SOFF-8/bad", setup: all(autoOff, own(OwnerManual, camera), link(LOW)), ev: ev, out: PurposeStandbyOffMonitored},
		{name: "CMD-SOFF-8/ok", setup: all(autoOff, own(OwnerManual, camera)), ev: ev, out: PurposeStandbyOffMonitored},
		{name: "CMD-SOFF-9", setup: all(autoOff, own(OwnerManual, talk), link(LOW)), ev: ev, out: PurposeStandbyOffOther},
		{name: "CMD-SOFF-10/monitored", setup: all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, "")), ev: ev, err: invalid,
			reason: ReasonSamePending},
		{name: "CMD-SOFF-10/monitored_bad", setup: all(own(OwnerManual, camera), link(LOW), p(1, PurposeStandbyOffMonitored, "")), ev: ev,
			err: invalid, reason: ReasonSamePending},
		{name: "CMD-SOFF-10/other", setup: all(own(OwnerManual, talk), p(1, PurposeStandbyOffOther, "")), ev: ev, err: invalid,
			reason: ReasonSamePending},
		{name: "CMD-SOFF-11/monitored", setup: all(own(OwnerManual, camera), timedOutTo(camera, PurposeStandbyOffMonitored)), ev: ev,
			out: PurposeStandbyOffMonitored},
		{name: "CMD-SOFF-11/other", setup: all(own(OwnerManual, talk), timedOutTo(talk, PurposeStandbyOffOther)), ev: ev,
			out: PurposeStandbyOffOther},
		{name: "CMD-SOFF-12/monitored", setup: all(own(OwnerManual, camera), p(1, PurposeStandbyOn, camera)), ev: ev, out: PurposeStandbyOffMonitored},
		{name: "CMD-SOFF-12/other", setup: all(own(OwnerManual, talk), p(1, PurposeStandbyOn, talk)), ev: ev, out: PurposeStandbyOffOther},
	})
}
