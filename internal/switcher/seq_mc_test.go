package switcher

import (
	"errors"
	"reflect"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 複数のカメラの列（MC-SQ-01〜08）。時刻は表の t（秒）をミリ秒にした値。

// finish は「完了(X)」: バッチの各 Action を直前に承認し（Restart があれば W を空けて SetScene を承認し）、
// SetScene の Proceed で期待を登録し、SRD{OK} と BatchDone{Completed} のあと、対象の SceneChanged を受ける。
// すべての承認は Proceed を期待する。
func (q *seq) finish(b *Batch, startMs int) {
	q.t.Helper()
	ms := startMs
	target := ""
	req := 0
	for _, a := range b.Actions {
		switch a := a.(type) {
		case RestartMedia:
			q.approve(b.BatchID, a, ms, Proceed{})
			ms += int(W.Milliseconds())
		case SetScene:
			req = q.m.st.NextReqID
			target = a.SceneUUID
			q.approve(b.BatchID, a, ms, Proceed{ReqID: req})
		default:
			q.approve(b.BatchID, a, ms, Proceed{})
		}
	}
	if req != 0 {
		q.none(srd(ms+100, req, RequestOK))
	}
	q.none(bd(ms+100, b.BatchID, BatchCompleted))
	if target != "" {
		q.none(sc(ms+400, target))
	}
}

// health は戻り先などのシーン uuid の健康を確かめる。
func (q *seq) health(uuid string, h scenemap.CameraState, bad string) {
	q.t.Helper()
	got, gotBad, has := q.m.healthOf(uuid)
	if !has || got != h || gotBad != bad {
		q.t.Fatalf("H(%s) = %v %q has=%v, want %v %q", uuid, got, gotBad, has, h, bad)
	}
}

func (q *seq) noHealth(uuid string) {
	q.t.Helper()
	if got, bad, has := q.m.healthOf(uuid); has {
		q.t.Fatalf("H(%s) = %v %q, want no health", uuid, got, bad)
	}
}

func bothInputs() []string { return []string{inMain, inSub} }

// MC-SQ-01: 片方だけ悪化、両方悪化、順に回復。
func TestMCSeq01(t *testing.T) {
	main := func(t *testing.T, mods ...cfgMod) {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN}, mods...)
		e := q.epochs()
		b := q.batchIn(cc(1000, OK, LOW, DOWN), 1, af, standby, camera, nil, camSub)
		q.health(camera, LOW, camSub)
		q.owner(OwnerNone, "")
		q.finish(b, 1100)
		q.owner(OwnerAuto, camera)
		q.scene(standby)
		q.none(cc(2000, DOWN, DOWN, DOWN))
		q.health(camera, DOWN, camMain)
		q.none(cc(3000, OK, DOWN, DOWN))
		q.health(camera, DOWN, camSub)
		q.scene(standby)
		b = q.batchIn(cc(4000, OK, OK, DOWN), 2, ar, camera, "", bothInputs(), "")
		q.owner(OwnerAuto, camera)
		if rm, ok := b.Actions[0].(RestartMedia); !ok || !reflect.DeepEqual(rm.Inputs, bothInputs()) || len(b.Actions) != 2 {
			t.Fatalf("両入力を1つの RestartMedia で再起動する: %+v", b.Actions)
		}
		q.finish(b, 4100)
		q.scene(camera)
		q.owner(OwnerNone, "")
		q.epochDelta(e, 0, 0, 0, 0)
	}
	t.Run("MC-SQ-01", func(t *testing.T) { main(t) })
	// MC-SQ-01/hide（s を hide にして同じ手順）は 0b で hide が Hide / Show の動きを持つため、HB-SQ-01 などに置き換わった。
	t.Run("MC-SQ-01/main_only", func(t *testing.T) {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		q.batchIn(cc(1000, DOWN, OK, DOWN), 1, af, standby, camera, nil, camMain)
	})
	t.Run("MC-SQ-01/sub_only", func(t *testing.T) {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		q.batchIn(cc(1000, OK, DOWN, DOWN), 1, af, standby, camera, nil, camSub)
	})
	t.Run("MC-SQ-01/both_in_one_event", func(t *testing.T) {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		q.batchIn(cc(1000, DOWN, DOWN, DOWN), 1, af, standby, camera, nil, camMain)
		q.pending(1)
		if q.m.st.NextBatchID != 2 {
			t.Fatalf("F が1本でない: NextBatchID=%d", q.m.st.NextBatchID)
		}
	})
}

// MC-SQ-02: 同じ健康でも原子的に組を扱う。
func TestMCSeq02(t *testing.T) {
	t.Run("MC-SQ-02", func(t *testing.T) {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		b := q.batchIn(cc(100, LOW, OK, DOWN), 1, af, standby, camera, nil, camMain)
		q.finish(b, 200)
		q.owner(OwnerAuto, camera)
		q.pending()
		q.none(cc(1000, OK, LOW, DOWN))
		q.health(camera, LOW, camSub)
		sent := q.m.st.LastAutoSent[ar]
		before := cloneState(q.m.st)
		q.none(CamerasChanged{At: at(2000), States: map[string]scenemap.CameraState{camDesk: DOWN, camSub: LOW, camMain: OK}})
		if d := diffState(q.m.st, before); d != "" {
			t.Fatalf("同じ組で状態が変わった:%s", d)
		}
		if got := q.m.st.LastAutoSent[ar]; !got.Equal(sent) {
			t.Fatalf("再送時刻が動いた")
		}
		q.batchIn(cc(3000, OK, OK, DOWN), 2, ar, camera, "", bothInputs(), "")
	})
}

// MC-SQ-03: 手元カメラを使わない日から使う。
func TestMCSeq03(t *testing.T) {
	t.Run("MC-SQ-03", func(t *testing.T) {
		ccMD := func(ms int, m, d scenemap.CameraState) Event {
			return CamerasChanged{At: at(ms), States: map[string]scenemap.CameraState{camMain: m, camDesk: d}}
		}
		q := newSeq(t)
		q.none(cfg(0, withCameras(cam(camMain), cam(camDesk))))
		q.none(ccMD(0, OK, DOWN))
		q.none(connM(0, true, camera2, multiMap()))
		q.none(tick(1000))
		e := q.epochs()
		b := q.batchIn(sc(2000, deskScene), 1, af, standby, deskScene, nil, camDesk)
		q.epochDelta(e, 1, 0, 0, 0)
		q.finish(b, 2100)
		q.owner(OwnerAuto, deskScene)
		q.none(tick(3000))
		q.none(ccMD(3500, OK, DOWN))
		b = q.batchIn(ccMD(4000, OK, OK), 2, ar, deskScene, "", []string{inDesk}, "")
		q.finish(b, 4100)
		q.scene(deskScene)
		q.owner(OwnerNone, "")
		e = q.epochs()
		q.none(sc(5000, camera2))
		q.epochDelta(e, 1, 0, 0, 0)
		q.none(ccMD(6000, OK, DOWN))
		q.scene(camera2)
		q.health(camera2, OK, "")
	})
}

// MC-SQ-04: 人が OBS の項目を非表示にする。
func TestMCSeq04(t *testing.T) {
	t.Run("MC-SQ-04", func(t *testing.T) {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		e := q.epochs()
		q.none(item(1000, camera, 2, false))
		q.epochDelta(e, 0, 0, 0, 0)
		q.health(camera, OK, "")
		q.none(cc(2000, OK, DOWN, DOWN))
		q.health(camera, OK, "")
		b := q.batchIn(item(3000, camera, 2, true), 1, af, standby, camera, nil, camSub)
		q.health(camera, DOWN, camSub)
		q.finish(b, 3100)
		q.owner(OwnerAuto, camera)
		b = q.batchIn(item(4000, camera, 2, false), 2, ar, camera, "", []string{inMain}, "")
		q.finish(b, 4100)
		q.scene(camera)
		q.owner(OwnerNone, "")
		if q.m.st.Map.Scenes[camera].Items[1].Enabled {
			t.Fatalf("非表示の s はそのまま")
		}
	})
	t.Run("MC-SQ-04/last_camera", func(t *testing.T) {
		// S、auto/A のとき最後の m も非表示にすれば C、戻らない。
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		b := q.batchIn(cc(1000, OK, LOW, DOWN), 1, af, standby, camera, nil, camSub)
		q.finish(b, 1100)
		q.owner(OwnerAuto, camera)
		q.none(item(2000, camera, 1, false)) // m を隠す。s=LOW だけが映る
		q.owner(OwnerAuto, camera)
		q.none(item(3000, camera, 2, false)) // 最後のカメラも隠す
		q.owner(OwnerNone, "")
		q.scene(standby)
		q.none(cc(4000, OK, OK, DOWN))
		q.none(tick(10000))
	})
	t.Run("MC-SQ-04/group_parent", func(t *testing.T) {
		// 同じ操作をグループの親項目に行い、すべての経路への影響を確認する。
		gm := mm(func(m *scenemap.Map) {
			m.Groups[groupUUID] = scenemap.Container{Name: "G", Items: []scenemap.Item{itm(1, inMain), itm(2, inSub)}}
			itemsOf(camera, scenemap.Item{ID: 1, SourceUUID: groupUUID, SourceKind: scenemap.GroupSource, Enabled: true})(m)
		})
		q := newSeq(t)
		q.none(cfg(0, withMulti()))
		q.none(cc(0, OK, OK, DOWN))
		q.none(connM(0, true, camera, gm))
		q.health(camera, OK, "")
		q.none(item(1000, camera, 1, false)) // 親を隠す: m も s も映らない
		q.noHealth(camera)
		q.none(item(1500, groupUUID, 2, false)) // 子の変更は、親が隠れている間は A の健康を変えない
		q.noHealth(camera)
		q.none(item(2000, camera, 1, true)) // 親を戻す: s は子で隠したので、m だけが映る
		q.health(camera, OK, "")
		q.none(cc(3000, OK, DOWN, DOWN)) // 映らない s は健康に影響しない
		q.batchIn(item(3500, groupUUID, 2, true), 1, af, standby, camera, nil, camSub)
	})
}

// MC-SQ-05: 戻り先が消える。
func TestMCSeq05(t *testing.T) {
	aGone := mm(dropped(camera))
	setup := func(t *testing.T) *seq {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, OK})
		b := q.batchIn(cc(100, LOW, DOWN, DOWN), 1, af, standby, camera, nil, camSub)
		q.finish(b, 200)
		q.owner(OwnerAuto, camera)
		return q
	}
	t.Run("MC-SQ-05", func(t *testing.T) {
		q := setup(t)
		e := q.epochs()
		q.none(smcMap(1000, aGone))
		q.owner(OwnerNone, "")
		q.scene(standby)
		q.epochDelta(e, 0, 0, 0, 0)
		q.none(cc(2000, OK, OK, OK))
		q.none(tick(10000))
		a2 := "uuid-camera-A2"
		withA2 := mm(dropped(camera), func(m *scenemap.Map) {
			m.Scenes[a2] = scenemap.Container{Name: "Camera", Items: []scenemap.Item{itm(1, inMain), itm(2, inSub)}}
		})
		q.none(smcMap(3000, withA2))
		q.owner(OwnerNone, "")
		q.none(tick(20000))
		q.none(sc(4000, a2))
		q.scene(a2)
		q.health(a2, OK, "")
	})
	t.Run("MC-SQ-05/approve_pending", func(t *testing.T) {
		q := setup(t)
		b := q.batchIn(cc(900, OK, OK, OK), 2, ar, camera, "", bothInputs(), "")
		q.none(smcMap(1000, aGone))
		q.approve(b.BatchID, restartOf(inMain, inSub), 1100, Reject{})
		q.none(bd(1100, 2, BatchRejected))
		q.noErr()
	})
	t.Run("MC-SQ-05/manual", func(t *testing.T) {
		// manual/A は保持し、Off の実行失敗を記録する（W-04）。
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		b := q.batch(cmd(100, CmdStandbyOn), 1, son, standby, camera)
		q.finish(b, 200)
		q.owner(OwnerManual, camera)
		q.none(smcMap(1000, aGone))
		q.owner(OwnerManual, camera)
		b = q.batch(cmd(2000, CmdStandbyOff), 2, sfo, camera, "")
		q.approve(2, set(camera), 2100, Proceed{ReqID: q.m.st.NextReqID})
		q.none(srd(2200, 2, RequestError))
		q.none(BatchDone{At: at(2200), BatchID: b.BatchID, Result: BatchFailed, Err: errors.New("SetScene: scene not found")})
		q.errAt(2200)
		q.owner(OwnerManual, camera)
		q.scene(standby)
	})
}

// MC-SQ-06: コレクション切替と再接続（W-04）。
func TestMCSeq06(t *testing.T) {
	prep := func(t *testing.T) (*seq, *Batch) {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		b := q.batchIn(cc(100, OK, LOW, DOWN), 1, af, standby, camera, nil, camSub)
		q.finish(b, 200)
		r := q.batchIn(cc(900, OK, OK, DOWN), 2, ar, camera, "", bothInputs(), "")
		q.pending(2)
		return q, r
	}
	t.Run("MC-SQ-06", func(t *testing.T) {
		q, _ := prep(t)
		e := q.epochs()
		q.none(disc(1000))
		q.epochDelta(e, 0, 1, 0, 0)
		q.exps()
		q.pending(2)
		if q.m.st.Map != nil {
			t.Fatalf("切断で地図が残っている")
		}
		// X の遅れた地図・項目の通知は、切断中は何も変えない（core も捨てる）。
		q.none(smcMap(1100, multiMap()))
		q.none(item(1150, camera, 1, false))
		if q.m.st.Map != nil {
			t.Fatalf("切断中の地図の Event で地図が復活した")
		}
		q.none(connM(2000, true, standby, mm(collectionOf("collection-y"))))
		q.owner(OwnerNone, "")
		q.epochDelta(e, 0, 1, 0, 0)
		q.approve(2, restartOf(inMain, inSub), 2100, Reject{})
		q.none(bd(2200, 2, BatchRejected))
		q.noErr()
		q.pending()
		e = q.epochs()
		q.none(sc(3000, camera))
		q.epochDelta(e, 1, 0, 0, 0)
		q.scene(camera)
	})
	t.Run("MC-SQ-06/same_collection", func(t *testing.T) {
		// 同じ collection で UUID・配信が同じなら、C にならない限り owner を保つ。SceneEpoch も進めず、StartScene も出さない。
		q, _ := prep(t)
		e := q.epochs()
		q.none(disc(1000))
		q.none(connM(2000, true, standby, multiMap()))
		q.owner(OwnerAuto, camera)
		q.epochDelta(e, 0, 1, 0, 0)
		q.none(tick(3000)) // R#2 は未完了のまま
	})
}

// MC-SQ-07: 古い OBS（最初の地図取得に3回失敗）。UUID は名前と同じ値。
func TestMCSeq07(t *testing.T) {
	t.Run("MC-SQ-07", func(t *testing.T) {
		const (
			oldA = "Camera"
			oldS = "Standby"
			oldO = "Opening"
		)
		oldMap := &scenemap.Map{Collection: collectionX, Groups: map[string]scenemap.Container{}, Inputs: map[string]scenemap.Input{},
			Scenes: map[string]scenemap.Container{oldA: {Name: oldA}, oldS: {Name: oldS}, oldO: {Name: oldO}}}
		q := newSeq(t)
		q.none(cfg(0, withMulti(), withStart(opening)))
		q.none(cc(0, DOWN, DOWN, DOWN))
		q.none(OBSConnected{At: at(0), Streaming: false, Scene: oldA, SceneUUID: oldA, TransitionMs: ip(300), Map: oldMap})
		if s := q.m.Snapshot(); !s.Connected || s.Class != ClassOther || s.HasHealth {
			t.Fatalf("接続後の Snapshot = %+v", s)
		}
		b := q.batch(cmd(1000, CmdStreamStart), 1, stt, "", "")
		q.approve(1, StartStream{}, 1000, Proceed{})
		q.none(bd(1100, 1, BatchCompleted))
		_ = b
		b = q.batch(ssc(2000, true), 2, ssp, oldO, "")
		q.finish(b, 2100)
		q.scene(oldO)
		q.owner(OwnerNone, "")
		b = q.batch(cmd(3000, CmdStandbyOn), 3, son, oldS, oldO)
		q.finish(b, 3100)
		q.owner(OwnerManual, oldO)
		b = q.batch(cmd(4000, CmdStandbyOff), 4, sfo, oldO, "")
		for _, a := range b.Actions {
			if _, ok := a.(RestartMedia); ok {
				t.Fatalf("再起動なし")
			}
		}
		q.finish(b, 4100)
		q.scene(oldO)
		q.owner(OwnerNone, "")
		q.none(cc(5000, LOW, OK, DOWN))
		q.none(tick(6000))
		q.none(tick(12000))
	})
}

// MC-SQ-08: 承認までに地図が変わる。
func TestMCSeq08(t *testing.T) {
	manualA := func(t *testing.T) *seq {
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		b := q.batch(cmd(100, CmdStandbyOn), 1, son, standby, camera)
		q.finish(b, 200)
		q.owner(OwnerManual, camera)
		return q
	}
	withD := mm(itemsOf(camera, itm(1, inMain), itm(2, inSub), itm(3, inDesk)))
	t.Run("MC-SQ-08", func(t *testing.T) {
		q := manualA(t)
		e := q.epochs()
		b := q.batchIn(cmd(1000, CmdStandbyOff), 2, sfm, camera, "", bothInputs(), "")
		q.owner(OwnerManual, camera)
		q.approve(2, b.Actions[0], 1100, Proceed{ReqID: 0})
		q.exps()
		q.none(smcMap(1200, withD))
		q.epochDelta(e, 0, 0, 0, 0)
		q.health(camera, DOWN, camDesk)
		q.approve(2, b.Actions[1], 1600, Replace{})
		q.owner(OwnerAuto, camera)
		q.exps()
		q.none(bd(1700, 2, BatchRejected))
		q.noErr()
		q.batchIn(smcMap(2000, multiMap()), 3, ar, camera, "", bothInputs(), "")
		q.health(camera, OK, "")
	})
	t.Run("MC-SQ-08/auto_recover", func(t *testing.T) {
		// #1 を AutoRecover にすれば t=1.6 は Reject、auto/A を保つ。
		q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, DOWN})
		b := q.batchIn(cc(100, OK, LOW, DOWN), 1, af, standby, camera, nil, camSub)
		q.finish(b, 200)
		r := q.batchIn(cc(900, OK, OK, DOWN), 2, ar, camera, "", bothInputs(), "")
		q.approve(2, r.Actions[0], 1000, Proceed{})
		q.none(smcMap(1200, withD))
		q.approve(2, r.Actions[1], 1600, Reject{})
		q.owner(OwnerAuto, camera)
		q.none(bd(1700, 2, BatchRejected))
		q.noErr()
	})
	t.Run("MC-SQ-08/to_other", func(t *testing.T) {
		// A 自体が Other へ変わる manual の列は Reject と LastError（W-02）。
		q := manualA(t)
		b := q.batchIn(cmd(1000, CmdStandbyOff), 2, sfm, camera, "", bothInputs(), "")
		q.approve(2, b.Actions[0], 1100, Proceed{ReqID: 0})
		q.none(smcMap(1200, mm(itemsOf(camera))))
		q.approve(2, b.Actions[1], 1600, Reject{})
		q.none(bd(1700, 2, BatchRejected))
		q.errAt(1700)
		q.owner(OwnerManual, camera)
	})
}

// MC-CC-4 の続き: Failed の0.5秒後に d=LOW、再度 Failed 後 d=DOWN でも、各 F を認める（意図した再送）。
func TestMCSeqCC4Resend(t *testing.T) {
	q := newSeq(t).multi(camera, true, [3]scenemap.CameraState{OK, OK, OK})
	b := q.batchIn(cc(1000, DOWN, OK, OK), 1, af, standby, camera, nil, camMain)
	q.approve(1, b.Actions[0], 1000, Proceed{ReqID: 1})
	q.none(srd(1100, 1, RequestError))
	q.none(bd(1100, 1, BatchFailed))
	q.batchIn(cc(1600, DOWN, OK, LOW), 2, af, standby, camera, nil, camMain)
	q.approve(2, set(standby), 1600, Proceed{ReqID: 2})
	q.none(srd(1700, 2, RequestError))
	q.none(bd(1700, 2, BatchFailed))
	q.batchIn(cc(2200, DOWN, OK, DOWN), 3, af, standby, camera, nil, camMain)
}

// MC-MAP-X / MC-ITEM-X の続き: BatchDone と期待の解消のあと、Tick で F(T) になる。
func TestMCSeqMapXThenTick(t *testing.T) {
	q := newSeq(t).multi(talk, true, [3]scenemap.CameraState{DOWN, OK, OK})
	b := q.batch(cmd(100, CmdStandbyOn), 1, son, standby, talk)
	q.none(smcMap(200, mm(itemsOf(talk, itm(1, inMain))))) // 切替中: 地図は更新するが、バッチは出さない
	q.health(talk, DOWN, camMain)
	q.approve(1, b.Actions[0], 300, Proceed{ReqID: 1})
	q.none(tick(400))
	q.none(srd(500, 1, RequestError)) // 期待は消える
	q.none(tick(600))                 // BatchDone がまだ: 未完了の StandbyOn が止める
	q.none(BatchDone{At: at(700), BatchID: 1, Result: BatchFailed, Err: errors.New("x")})
	q.batchIn(tick(800), 2, af, standby, talk, nil, camMain)
}
