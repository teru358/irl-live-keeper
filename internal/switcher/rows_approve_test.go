package switcher

import "testing"

func sceneEpochUp(s *state)  { s.SceneEpoch++ }
func connEpochUp(s *state)   { s.ConnEpoch++ }
func streamEpochUp(s *state) { s.StreamEpoch++ }
func configEpochUp(s *state) { s.ConfigEpoch++ }

func markManualRejected(s *state) {
	pb := s.Pending[1]
	pb.ManualRejected = true
	s.Pending[1] = pb
}

func TestRowsApproveCommon(t *testing.T) {
	af := p(1, PurposeAutoFail, camera)
	bad := link(LOW)
	rows := []apRow{
		{name: "AP-COM-1", setup: all(bad, p(2, PurposeAutoFail, camera)), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-COM-2", setup: all(bad, af, disconnected), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-COM-2/stop_stream", setup: all(p(1, PurposeStopStream, ""), disconnected), action: StopStream{}, atMs: 1000, res: Reject{}},
		{name: "AP-COM-3", setup: all(bad, af, connEpochUp), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-COM-3/start_stream", setup: all(notStreaming, p(1, PurposeStartStream, ""), connEpochUp), action: StartStream{}, atMs: 1000,
			res: Reject{}},
		{name: "AP-COM-4/auto_fail", setup: all(bad, af, streamEpochUp), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-COM-4/stop_stream", setup: all(p(1, PurposeStopStream, ""), streamEpochUp), action: StopStream{}, atMs: 1000, res: Reject{}},
		{name: "AP-COM-5/auto_fail", setup: all(bad, af, sceneEpochUp), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-COM-5/auto_recover_restart", setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, ""), sceneEpochUp),
			action: restart, atMs: 1000, res: Reject{}},
		{name: "AP-COM-5/standby_on", setup: all(p(1, PurposeStandbyOn, camera), sceneEpochUp), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-COM-5/standby_off_monitored_restart", setup: all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, ""), sceneEpochUp),
			action: restart, atMs: 1000, res: Reject{}},
		{name: "AP-COM-5/standby_off_other", setup: all(own(OwnerManual, talk), p(1, PurposeStandbyOffOther, ""), sceneEpochUp),
			action: set(talk), atMs: 1000, res: Reject{}},
		{name: "AP-COM-5/start_scene", setup: all(startAt(opening), p(1, PurposeStartScene, ""), sceneEpochUp),
			action: set(opening), atMs: 1000, res: Reject{}, change: markManualRejected},
		{name: "AP-COM-6/auto_fail", setup: all(bad, af, configEpochUp), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-COM-6/auto_recover", setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, ""), configEpochUp),
			action: set(camera), atMs: 1000, res: Reject{}},
		{name: "AP-COM-6/standby_on", setup: all(p(1, PurposeStandbyOn, camera), configEpochUp), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-COM-6/standby_off_monitored", setup: all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, ""), configEpochUp),
			action: restart, atMs: 1000, res: Reject{}},
		{name: "AP-COM-6/standby_off_other", setup: all(own(OwnerManual, talk), p(1, PurposeStandbyOffOther, ""), configEpochUp),
			action: set(talk), atMs: 1000, res: Reject{}},
		{name: "AP-COM-6/start_scene", setup: all(startAt(opening), p(1, PurposeStartScene, ""), configEpochUp),
			action: set(opening), atMs: 1000, res: Reject{}},
		{name: "AP-COM-7/start_stream", setup: all(notStreaming, p(1, PurposeStartStream, ""), sceneEpochUp, configEpochUp),
			action: StartStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-COM-7/stop_stream", setup: all(p(1, PurposeStopStream, ""), sceneEpochUp, configEpochUp),
			action: StopStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-COM-8", setup: all(bad, af, startAt(opening), expired(1, opening, PurposeStartScene, "", 900)),
			action: set(standby), atMs: 1000, res: Proceed{}, register: true, change: dropExps, recErr: true},
	}
	runApRows(t, rows)
}

func TestRowsApproveAutoFail(t *testing.T) {
	af := p(1, PurposeAutoFail, camera)
	runApRows(t, []apRow{
		{name: "AP-AUTOFAIL-1/LOW", setup: all(link(LOW), af), action: set(standby), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-AUTOFAIL-1/LOW_to_DOWN", setup: all(link(LOW), af, link(DOWN)), action: set(standby), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-AUTOFAIL-2", setup: all(link(LOW), af, link(OK)), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-AUTOFAIL-3", setup: all(link(LOW), af, scn(standby)), action: set(standby), atMs: 1000, res: Skip{}},
		{name: "AP-AUTOFAIL-4/other", setup: all(link(LOW), af, scn(talk)), action: set(standby), atMs: 1000, res: Reject{}},
		// /0c: 持ち主 auto でも AutoFail は有効になった（スタックへ積む）。持ち主の不適格は manual で確かめる。
		{name: "AP-AUTOFAIL-4/owner_not_none", setup: all(link(LOW), af, func(s *state) { ownerIs(s, OwnerManual, camera2) }),
			action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-AUTOFAIL-5/pending", setup: all(link(LOW), af, p(2, PurposeStandbyOn, camera)), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-AUTOFAIL-5/expectation", setup: all(link(LOW), af, startAt(opening), x(1, opening, PurposeStartScene, "")),
			action: set(standby), atMs: 1000, res: Reject{}},
	})
}

func TestRowsApproveAutoRecover(t *testing.T) {
	ar := all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, ""))
	runApRows(t, []apRow{
		{name: "AP-AUTOREC-1", setup: ar, action: restart, atMs: 1000, res: Proceed{}},
		{name: "AP-AUTOREC-2", setup: ar, action: set(camera), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-AUTOREC-3/restart", setup: all(ar, link(LOW)), action: restart, atMs: 1000, res: Reject{}},
		{name: "AP-AUTOREC-3/set_scene", setup: all(ar, link(DOWN)), action: set(camera), atMs: 1000, res: Reject{}},
		{name: "AP-AUTOREC-4", setup: all(ar, scn(camera)), action: set(camera), atMs: 1000, res: Skip{}},
		{name: "AP-AUTOREC-5", setup: all(ar, scn(camera)), action: restart, atMs: 1000, res: Reject{}},
		{name: "AP-AUTOREC-6/restart", setup: all(ar, p(2, PurposeStandbyOffOther, "")), action: restart, atMs: 1000, res: Reject{}},
		{name: "AP-AUTOREC-6/set_scene", setup: all(ar, startAt(opening), x(1, opening, PurposeStartScene, "")), action: set(camera),
			atMs: 1000, res: Reject{}},
	})
}

func TestRowsApproveStandbyOn(t *testing.T) {
	on := p(1, PurposeStandbyOn, camera)
	runApRows(t, []apRow{
		{name: "AP-SON-1/ok", setup: on, action: set(standby), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-SON-1/bad", setup: all(on, link(DOWN)), action: set(standby), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-SON-1/other", setup: all(scn(talk), p(1, PurposeStandbyOn, talk), link(LOW)), action: set(standby), atMs: 1000,
			res: Proceed{}, register: true},
		{name: "AP-SON-2", setup: all(on, scn(standby)), action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-SON-3", setup: all(on, func(s *state) { ownerIs(s, OwnerManual, talk) }), action: set(standby),
			atMs: 1000, res: Reject{}},
	})
}

func TestRowsApproveStandbyOffMonitored(t *testing.T) {
	off := all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, ""))
	runApRows(t, []apRow{
		{name: "AP-SOFFM-1", setup: off, action: restart, atMs: 1000, res: Proceed{}},
		{name: "AP-SOFFM-2", setup: off, action: set(camera), atMs: 1000, res: Proceed{}, register: true},
		// 0c（FB-AP-07）: 戻り先の健康だけが悪くなっても auto 化へ置き換えず、OFF の計画が変われば Reject（押し直しで選び直す）。
		{name: "AP-SOFFM-3/FB-AP-07/restart", setup: all(off, link(LOW)), action: restart, atMs: 1000, res: Reject{}},
		{name: "AP-SOFFM-3/FB-AP-07/set_scene", setup: all(off, link(DOWN)), action: set(camera), atMs: 1000, res: Reject{}},
		{name: "AP-SOFFM-4/restart_bad", setup: all(autoOff, off, link(LOW)), action: restart, atMs: 1000, res: Proceed{}},
		{name: "AP-SOFFM-4/set_scene_bad", setup: all(autoOff, off, link(DOWN)), action: set(camera), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-SOFFM-4/set_scene_ok", setup: all(autoOff, off), action: set(camera), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-SOFFM-5/ok", setup: all(off, scn(camera), clearOwner), action: set(camera), atMs: 1000, res: Skip{}},
		{name: "AP-SOFFM-5/bad", setup: all(off, scn(camera), clearOwner, link(LOW)), action: set(camera), atMs: 1000, res: Skip{}},
		{name: "AP-SOFFM-6", setup: all(off, scn(camera), clearOwner), action: restart, atMs: 1000, res: Reject{}},
	})
}

func TestRowsApproveStandbyOffOther(t *testing.T) {
	off := all(own(OwnerManual, talk), p(1, PurposeStandbyOffOther, ""))
	runApRows(t, []apRow{
		{name: "AP-SOFFO-1/ok", setup: off, action: set(talk), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-SOFFO-1/bad_auto_off", setup: all(autoOff, off, link(LOW)), action: set(talk), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-SOFFO-2", setup: all(off, scn(talk), clearOwner), action: set(talk), atMs: 1000, res: Skip{}},
		{name: "AP-SOFFO-3/not_manual", setup: all(off, toAutoOwner), action: set(talk), atMs: 1000, res: Reject{}},
		{name: "AP-SOFFO-3/elsewhere", setup: all(off, scn(camera2), clearOwner), action: set(talk), atMs: 1000, res: Reject{}},
	})
}

func TestRowsApproveStartScene(t *testing.T) {
	ss := all(startAt(opening), p(1, PurposeStartScene, ""))
	runApRows(t, []apRow{
		{name: "AP-STARTSCENE-1", setup: all(ss, link(LOW), autoOff), action: set(opening), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-STARTSCENE-2", setup: all(ss, scn(opening)), action: set(opening), atMs: 1000, res: Skip{}},
		{name: "AP-STARTSCENE-3", setup: all(ss, sceneEpochUp), action: set(opening), atMs: 1000, res: Reject{}, change: markManualRejected},
		{name: "AP-STARTSCENE-4/stream", setup: all(ss, notStreaming, streamEpochUp), action: set(opening), atMs: 1000, res: Reject{}},
		{name: "AP-STARTSCENE-4/conn", setup: all(ss, connEpochUp), action: set(opening), atMs: 1000, res: Reject{}},
		{name: "AP-STARTSCENE-4/disconnected", setup: all(ss, disconnected), action: set(opening), atMs: 1000, res: Reject{}},
		{name: "AP-STARTSCENE-4/config", setup: all(ss, configEpochUp), action: set(opening), atMs: 1000, res: Reject{}},
	})
}

func TestRowsApproveStream(t *testing.T) {
	start := all(notStreaming, p(1, PurposeStartStream, ""))
	stop := p(1, PurposeStopStream, "")
	runApRows(t, []apRow{
		{name: "AP-STARTSTREAM-1", setup: start, action: StartStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-STARTSTREAM-2", setup: all(start, func(s *state) { s.Streaming = true }, streamEpochUp), action: StartStream{}, atMs: 1000,
			res: Reject{}},
		{name: "AP-STARTSTREAM-3/scene", setup: all(start, sceneEpochUp), action: StartStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-STARTSTREAM-3/config", setup: all(start, configEpochUp), action: StartStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-STOP-1/monitored_bad", setup: all(stop, link(LOW)), action: StopStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-STOP-1/manual_standby", setup: all(own(OwnerManual, talk), stop), action: StopStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-STOP-2/scene", setup: all(stop, sceneEpochUp), action: StopStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-STOP-2/config", setup: all(stop, configEpochUp), action: StopStream{}, atMs: 1000, res: Proceed{}},
		{name: "AP-STOP-3", setup: all(stop, func(s *state) { s.StreamEpoch += 2 }), action: StopStream{}, atMs: 1000, res: Reject{}},
	})
}
