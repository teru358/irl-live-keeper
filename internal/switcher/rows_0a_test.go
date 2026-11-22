package switcher

import (
	"fmt"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 表の5章の「/0a」の行を、複数のカメラ（m、s、d。A は m と s）の fixture で足す。
// 第1段階から引き継いだ行は rows_events_test.go などに、同じ意味で単独のカメラ main の fixture で移してある（行名は元のまま）。
// ここは、表の書き方どおり「s の状態で A の健康が決まる」形で、同じ行を複数のカメラで確かめる。

var bothIn = []string{inMain, inSub}

// sub は s の状態だけを変える（m、d は OK）。A の健康は s で決まる。
func sub(l scenemap.CameraState) setupFn { return states(OK, l, OK) }
func ccs(ms int, l scenemap.CameraState) Event {
	return cc(ms, OK, l, OK)
}
func subTo(l scenemap.CameraState) func(s *state) { return statesTo(OK, l, OK) }

// freshMulti は、生成した直後に複数のカメラの設定を入れた状態（未接続、地図なし）。カメラの状態は st。
func freshMulti(m, s, d scenemap.CameraState) setupFn {
	return func(st *state) {
		fresh(st)
		st.Cameras = []CameraConfig{cam(camMain), cam(camSub), cam(camDesk)}
		st.CameraStates = map[string]scenemap.CameraState{camMain: m, camSub: s, camDesk: d}
	}
}

// connectedM は複数のカメラの地図での OBSConnected の共通の変化。
func connectedM(streaming bool, scene string, mp *scenemap.Map) func(s *state) {
	return func(s *state) {
		if streaming != s.PrevConnStreaming {
			s.StreamEpoch++
		}
		if streaming != s.PrevConnStreaming || scene != s.PrevConnScene || s.PrevConnCollection != mp.Collection {
			clearOwner(s)
		}
		s.Connected = true
		s.Streaming = streaming
		s.Scene = scene
		s.Map = cloneTestMap(mp)
		resolveNames(s)
		s.TransitionMs = ip(300)
	}
}

func TestRows0aCamerasChanged(t *testing.T) {
	// 5.1: 以下は A、s の状態で H(A) が決まる。
	rows := []evRow{
		{name: "LC-1/0a", ev: ccs(1000, LOW), change: subTo(LOW), out: PurposeAutoFail, bad: camSub},
		{name: "LC-2/0a", ev: ccs(1000, DOWN), change: subTo(DOWN), out: PurposeAutoFail, bad: camSub},
		{name: "LC-3/0a", setup: all(sub(LOW), p(1, PurposeAutoFail, camera)), ev: ccs(1000, DOWN), change: subTo(DOWN)},
		{name: "LC-4/0a", setup: all(sub(LOW), x(1, standby, PurposeAutoFail, camera)), ev: ccs(1000, DOWN), change: subTo(DOWN)},
		// /0c: 同じ候補の再送は全入口で最短5秒（FB-CC-10）。0a の「Tick だけの再送の歯止め」を置き換えた。
		{name: "LC-5/0a/0c", setup: all(sub(LOW), sentAt(PurposeAutoFail, 800)), ev: ccs(1000, DOWN), change: subTo(DOWN)},
		{name: "LC-6/0a", setup: sub(LOW), ev: ccs(1000, OK), change: subTo(OK)},
		{name: "LC-6/0a/DOWN", setup: sub(DOWN), ev: ccs(1000, OK), change: subTo(OK)},
		{name: "LC-7/0a", setup: all(sub(LOW), p(1, PurposeAutoFail, camera)), ev: ccs(1000, OK), change: subTo(OK),
			check: func(t *testing.T, m *Machine) {
				if _, ok := m.st.Pending[1]; !ok {
					t.Fatalf("承認待ちの F が消えた")
				}
				if got := m.Approve(1, set(standby), OverrideNone{}, at(1100)); got != (Reject{}) {
					t.Errorf("承認 = %#v, want Reject", got)
				}
			}},
		{name: "LC-8/0a", setup: all(own(OwnerAuto, camera), sub(LOW)), ev: ccs(1000, OK), change: subTo(OK), out: PurposeAutoRecover, in: bothIn},
		{name: "LC-8/0a/DOWN", setup: all(own(OwnerAuto, camera), sub(DOWN)), ev: ccs(1000, OK), change: subTo(OK), out: PurposeAutoRecover, in: bothIn},
		{name: "LC-9/0a", setup: all(own(OwnerAuto, camera), sub(LOW), p(1, PurposeAutoRecover, "")), ev: ccs(1000, OK), change: subTo(OK)},
		{name: "LC-9/0a/expecting", setup: all(own(OwnerAuto, camera), sub(LOW), x(1, camera, PurposeAutoRecover, "")), ev: ccs(1000, OK), change: subTo(OK)},
	}
	for _, tr := range [][2]scenemap.CameraState{{LOW, DOWN}, {DOWN, LOW}, {OK, LOW}, {OK, DOWN}} {
		from, to := tr[0], tr[1]
		rows = append(rows, evRow{name: fmt.Sprintf("LC-10/0a/%v_%v", from, to), setup: all(own(OwnerAuto, camera), sub(from)),
			ev: ccs(1000, to), change: subTo(to)})
	}
	for _, tr := range [][2]scenemap.CameraState{{OK, LOW}, {LOW, DOWN}, {LOW, OK}, {DOWN, OK}} {
		from, to := tr[0], tr[1]
		name := fmt.Sprintf("%v_%v", from, to)
		rows = append(rows,
			evRow{name: "LC-11/0a/" + name, setup: all(own(OwnerNone, ""), sub(from)), ev: ccs(1000, to), change: subTo(to)},
			evRow{name: "LC-12/0a/" + name, setup: all(own(OwnerManual, camera), sub(from)), ev: ccs(1000, to), change: subTo(to)},
			evRow{name: "LC-13/0a/" + name, setup: all(scn(talk), sub(from)), ev: ccs(1000, to), change: subTo(to)},
		)
	}
	rows = append(rows,
		evRow{name: "LC-14/0a", setup: notStreaming, ev: ccs(1000, LOW), change: subTo(LOW)},
		evRow{name: "LC-15/0a", setup: disconnected, ev: ccs(1000, LOW), change: subTo(LOW),
			check: func(t *testing.T, m *Machine) {
				if m.st.Map != nil {
					t.Errorf("未接続では地図なし")
				}
			}},
		evRow{name: "LC-16/0a", setup: autoOff, ev: ccs(1000, LOW), change: subTo(LOW)},
		// 切替中（X）は、States だけ更新し、抑止が解けたあとの Tick で判断する。
		evRow{name: "LC-17/0a/pending", setup: p(1, PurposeStandbyOn, camera), ev: ccs(1000, LOW), change: subTo(LOW),
			check: afterResolvedTick(1)},
		evRow{name: "LC-17/0a/expectation", setup: x(1, standby, PurposeStandbyOn, camera), ev: ccs(1000, LOW), change: subTo(LOW)},
		evRow{name: "LC-18/0a/pending", setup: all(startAt(opening), p(1, PurposeStartScene, "")), ev: ccs(1000, LOW), change: subTo(LOW),
			check: afterResolvedTick(1)},
		evRow{name: "LC-18/0a/expectation", setup: all(startAt(opening), x(1, opening, PurposeStartScene, "")), ev: ccs(1000, LOW), change: subTo(LOW)},
		evRow{name: "LC-20/0a/monitored", setup: p(1, PurposeStandbyOffMonitored, ""), ev: ccs(1000, LOW), change: subTo(LOW)},
		evRow{name: "LC-20/0a/other", setup: p(1, PurposeStandbyOffOther, ""), ev: ccs(1000, LOW), change: subTo(LOW),
			check: afterResolvedTick(1)},
		// 配信の開始・停止だけが未完了のときは X ではない。
		evRow{name: "LC-21/0a/stop", setup: p(1, PurposeStopStream, ""), ev: ccs(1000, LOW), change: subTo(LOW), out: PurposeAutoFail, bad: camSub},
		evRow{name: "LC-21/0a/start", setup: p(1, PurposeStartStream, ""), ev: ccs(1000, LOW), change: subTo(LOW), out: PurposeAutoFail, bad: camSub},
	)
	for i := range rows {
		rows[i].base = baseMulti
	}
	runEvRows(t, rows)
}

// afterResolvedTick は、抑止していた未完了のバッチ id が終わったあとの Tick で、F(A) が出ることを確かめる。
func afterResolvedTick(id int) func(t *testing.T, m *Machine) {
	return func(t *testing.T, m *Machine) {
		t.Helper()
		// /0c: 未完了が解消した BatchDone で、その場で判断する（次の Tick まで待たない）。
		b, _ := handle1(m, BatchDone{At: at(1500), BatchID: id, Result: BatchCompleted})
		if b == nil || b.Purpose != PurposeAutoFail || b.BadCamera != camSub {
			t.Errorf("抑止の解除後の BatchDone: got %+v, want AutoFail(bad=sub)", b)
		}
	}
}

func TestRows0aConnect(t *testing.T) {
	mp := multiMap()
	reconnected := func(s *state) { connectedM(s.Streaming, s.Scene, mp)(s) }
	reconnectedNoEpoch := func(s *state) {
		s.Connected = true
		s.Map = cloneTestMap(mp)
		resolveNames(s)
	}
	_ = reconnected
	rows := []evRow{
		// 最初の接続（Map つき）。配信 false なら StreamEpoch 不変、true なら +1。
		{name: "OC-1/0a", setup: freshMulti(OK, OK, OK), ev: connM(1000, false, camera, mp), change: connectedM(false, camera, mp)},
		{name: "OC-2/0a", setup: freshMulti(OK, DOWN, OK), ev: connM(1000, true, camera, mp), change: connectedM(true, camera, mp),
			out: PurposeAutoFail, bad: camSub},
		{name: "OC-2/0a/other", setup: freshMulti(OK, DOWN, OK), ev: connM(1000, true, talk, mp), change: connectedM(true, talk, mp)},
		// 同じ UUID・配信・Collection での再接続。SceneEpoch は進めない。
		{name: "OC-3/0a", setup: all(sub(LOW), disconnected), ev: connM(1000, true, camera, mp), change: reconnectedNoEpoch,
			out: PurposeAutoFail, bad: camSub},
		{name: "OC-4/0a", setup: all(own(OwnerAuto, camera), disconnected), ev: connM(1000, true, standby, mp), change: reconnectedNoEpoch,
			out: PurposeAutoRecover, in: bothIn},
		{name: "OC-5/0a/auto_bad", setup: all(own(OwnerAuto, camera), sub(LOW), disconnected), ev: connM(1000, true, standby, mp), change: reconnectedNoEpoch},
		{name: "OC-5/0a/manual", setup: all(own(OwnerManual, camera), disconnected), ev: connM(1000, true, standby, mp), change: reconnectedNoEpoch},
		{name: "OC-5/0a/none", setup: all(own(OwnerNone, ""), disconnected), ev: connM(1000, true, standby, mp), change: reconnectedNoEpoch},
		// 切断: 地図と解決した UUID を消し、ConnEpoch+1、期待を全部消す。
		{name: "OD-1/0a", setup: all(sub(LOW), p(1, PurposeAutoFail, camera), x(1, standby, PurposeAutoFail, camera)), ev: disc(1000),
			change: disconnectedChange},
		{name: "OD-2/0a/auto", setup: all(own(OwnerAuto, camera), sentAt(PurposeAutoFail, 100)), ev: disc(1000), change: disconnectedChange},
		{name: "OD-2/0a/manual", setup: own(OwnerManual, talk), ev: disc(1000), change: disconnectedChange},
		{name: "OD-3/0a", setup: all(startAt(opening), x(1, opening, PurposeStartScene, "")), ev: disc(1000), change: disconnectedChange},
	}
	for i := range rows {
		rows[i].base = baseMulti
	}
	runEvRows(t, rows)
}

func TestRows0aSceneChanged(t *testing.T) {
	arrivedOwner := func(scene string, o Owner, ret string) func(s *state) {
		return all(arrived(scene), setOwner(o, ret))
	}
	rows := []evRow{
		{name: "SCE-3/0a", setup: all(sub(LOW), x(1, standby, PurposeStandbyOn, camera)), ev: sc(1000, standby), change: arrivedOwner(standby, OwnerManual, camera)},
		{name: "SCE-4/0a", setup: all(own(OwnerAuto, camera), x(1, camera, PurposeAutoRecover, "")), ev: sc(1000, camera), change: all(arrived(camera), clearOwner)},
		{name: "SCE-6/0a", setup: all(own(OwnerManual, camera), x(1, camera, PurposeStandbyOffMonitored, "")), ev: sc(1000, camera),
			change: all(arrived(camera), clearOwner)},
		{name: "SCE-7/0a", setup: all(own(OwnerManual, camera), sub(LOW), x(1, camera, PurposeStandbyOffMonitored, "")), ev: sc(1000, camera),
			change: all(arrived(camera), clearOwner), out: PurposeAutoFail, ret: camera, bad: camSub},
		{name: "SCE-8/0a", setup: all(own(OwnerManual, camera), sub(LOW), p(1, PurposeStandbyOffMonitored, ""), x(1, camera, PurposeStandbyOffMonitored, "")),
			ev: sc(1000, camera), change: all(arrived(camera), clearOwner)},
		{name: "SCE-11/0a", setup: all(startAt(camera), scn(talk), sub(LOW), x(1, camera, PurposeStartScene, "")), ev: sc(1000, camera),
			change: arrived(camera), out: PurposeAutoFail, ret: camera, bad: camSub},
		{name: "SCE-12/0a", setup: all(startAt(camera), scn(talk), sub(LOW), p(1, PurposeStartScene, ""), x(1, camera, PurposeStartScene, "")),
			ev: sc(1000, camera), change: arrived(camera)},
		// 手動の切替
		{name: "SCM-1/0a", setup: scn(talk), ev: sc(1000, camera), change: manualTo(camera)},
		{name: "SCM-2/0a", setup: all(scn(talk), sub(LOW)), ev: sc(1000, camera), change: manualTo(camera), out: PurposeAutoFail, bad: camSub},
		{name: "SCM-3/0a", setup: all(own(OwnerAuto, camera2), sub(LOW), sentAt(PurposeAutoFail, 900)), ev: sc(1000, camera),
			change: manualTo(camera), out: PurposeAutoFail, ret: camera, bad: camSub},
		{name: "SCM-4/0a", setup: all(own(OwnerManual, camera2), sub(LOW)), ev: sc(1000, camera), change: manualTo(camera), out: PurposeAutoFail, bad: camSub},
		{name: "SCM-5/0a", setup: all(own(OwnerNone, ""), sub(LOW)), ev: sc(1000, camera), change: manualTo(camera), out: PurposeAutoFail, bad: camSub},
		{name: "SCM-10/0a", setup: all(autoOff, scn(talk), sub(LOW)), ev: sc(1000, camera), change: manualTo(camera)},
		// 期待がなく、現在と同じ UUID の Event も、手動の切替として J/0a を行う（V-09）。
		{name: "SCM-14/0a", setup: sub(LOW), ev: sc(1000, camera), change: manualTo(camera), out: PurposeAutoFail, bad: camSub},
	}
	for i := range rows {
		rows[i].base = baseMulti
	}
	runEvRows(t, rows)
}

func TestRows0aConfigured(t *testing.T) {
	rows := []evRow{
		{name: "SCF-1/0a", setup: func(s *state) { *s = newMachine().st }, ev: cfg(1000, withMulti()),
			change: func(s *state) { applyCfg(s, cfgOf(0, withMulti())) },
			check: func(t *testing.T, m *Machine) {
				for _, id := range []string{camMain, camSub, camDesk} {
					if m.st.CameraStates[id] != DOWN {
						t.Errorf("%s = %v, want DOWN", id, m.st.CameraStates[id])
					}
				}
				if m.st.ConfigEpoch != 1 {
					t.Errorf("ConfigEpoch = %d", m.st.ConfigEpoch)
				}
			}},
		{name: "SCF-6/0a/no-camera", setup: own(OwnerAuto, camera), ev: cfg(1000, withMulti(), withStandby(standby2)),
			change: cfgChange(true, withMulti(), withStandby(standby2))},
		{name: "SCF-7/0a/no-camera", setup: own(OwnerManual, camera), ev: cfg(1000, withMulti(), withStandby(standby2)),
			change: cfgChange(true, withMulti(), withStandby(standby2))},
		{name: "SCF-8/0a", setup: all(sub(LOW), x(1, standby, PurposeAutoFail, camera)), ev: cfg(1000, withMulti(), withStandby(standby2)),
			change: cfgChange(false, withMulti(), withStandby(standby2)), out: PurposeAutoFail, to: standby2, ret: camera, bad: camSub},
		{name: "SCF-11/0a", setup: own(OwnerAuto, camera), ev: cfg(1000, withMulti(), withAuto(false)), change: cfgChange(true, withMulti(), withAuto(false))},
		{name: "SCF-12/0a", setup: own(OwnerManual, camera), ev: cfg(1000, withMulti(), withAuto(false)), change: cfgChange(false, withMulti(), withAuto(false))},
	}
	for i := range rows {
		if rows[i].base == nil && rows[i].name != "SCF-1/0a" {
			rows[i].base = baseMulti
		}
	}
	runEvRows(t, rows)
}

func TestRows0aCommands(t *testing.T) {
	son := cmd(1000, CmdStandbyOn)
	soff := cmd(1000, CmdStandbyOff)
	rows := []evRow{
		{name: "CMD-SON-9/0a/ok", ev: son, out: PurposeStandbyOn},
		{name: "CMD-SON-9/0a/bad", setup: sub(DOWN), ev: son, out: PurposeStandbyOn},
		{name: "CMD-SON-10/0a", setup: all(scn(talk), sub(LOW)), ev: son, out: PurposeStandbyOn, ret: talk},
		{name: "CMD-SON-11/0a", setup: all(autoOff, sub(LOW)), ev: son, out: PurposeStandbyOn},
		{name: "CMD-SON-12/0a/stop_stream", setup: p(1, PurposeStopStream, ""), ev: son, out: PurposeStandbyOn},
		{name: "CMD-SON-12/0a/standby_off_monitored", setup: p(1, PurposeStandbyOffMonitored, ""), ev: son, out: PurposeStandbyOn},
		{name: "CMD-SON-12/0a/standby_off_other", setup: p(1, PurposeStandbyOffOther, ""), ev: son, out: PurposeStandbyOn},
		{name: "CMD-SOFF-7/0a/bad", setup: all(own(OwnerManual, talk), sub(LOW)), ev: soff, out: PurposeStandbyOffOther},
		{name: "CMD-SOFF-7/0a/ok", setup: own(OwnerManual, talk), ev: soff, out: PurposeStandbyOffOther},
		{name: "CMD-SOFF-8/0a/bad", setup: all(autoOff, own(OwnerManual, camera), sub(LOW)), ev: soff, out: PurposeStandbyOffMonitored, in: bothIn},
		{name: "CMD-SOFF-8/0a/ok", setup: all(autoOff, own(OwnerManual, camera)), ev: soff, out: PurposeStandbyOffMonitored, in: bothIn},
	}
	streamOn := func(s *state) { s.Streaming, s.StreamEpoch = true, s.StreamEpoch+1 }
	rows = append(rows,
		// 配信開始。開始時の切替が有効なら、健康によらず StartScene（Auto より先）。
		evRow{name: "SST-3/0a/monitored_bad", setup: all(notStreaming, startAt(opening), sub(LOW), sentAt(PurposeAutoFail, 100)), ev: ssc(1000, true),
			change: streamOn, out: PurposeStartScene},
		evRow{name: "SST-3/0a/other_ok", setup: all(notStreaming, startAt(opening), scn(talk)), ev: ssc(1000, true), change: streamOn, out: PurposeStartScene},
		evRow{name: "SST-3/0a/standby", setup: all(notStreaming, startAt(opening), scn(standby), sub(LOW)), ev: ssc(1000, true), change: streamOn,
			out: PurposeStartScene},
		evRow{name: "SST-4/0a", setup: all(notStreaming, startAt(opening), autoOff, sub(LOW)), ev: ssc(1000, true), change: streamOn, out: PurposeStartScene},
		evRow{name: "SST-5/0a", setup: all(notStreaming, startAt(opening), scn(opening)), ev: ssc(1000, true), change: streamOn, out: PurposeStartScene},
		// 配信停止: C、期待・再送時刻を消す。地図とカメラ状態は保つ。
		evRow{name: "SSF-2/0a", setup: all(sub(LOW), p(1, PurposeAutoFail, camera), p(2, PurposeStopStream, ""), x(1, standby, PurposeAutoFail, camera)),
			ev: ssc(1000, false), change: streamOff},
		evRow{name: "SSF-3/0a", setup: all(startAt(opening), x(1, opening, PurposeStartScene, "")), ev: ssc(1000, false), change: streamOff},
	)
	for i := range rows {
		rows[i].base = baseMulti
	}
	runEvRows(t, rows)
}

func TestRows0aApprove(t *testing.T) {
	ar := all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, ""))
	off := all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, ""))
	rows := []apRow{
		{name: "AP-AUTOREC-1/0a", setup: ar, action: restartOf(inMain, inSub), atMs: 1000, res: Proceed{}},
		{name: "AP-AUTOREC-2/0a", setup: ar, action: set(camera), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-SOFFM-1/0a", setup: off, action: restartOf(inMain, inSub), atMs: 1000, res: Proceed{}},
		{name: "AP-SOFFM-2/0a", setup: off, action: set(camera), atMs: 1000, res: Proceed{}, register: true},
	}
	for i := range rows {
		rows[i].base = baseMulti
	}
	runApRows(t, rows)
}
