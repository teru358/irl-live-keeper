package switcher

import (
	"reflect"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 複数のカメラ（m、s、d）の新しい行（表の MC- の行と、置き換えた行の追加の列）。
// 複数のカメラの地図 multiMap は別の fixture で、既定の baseMulti から始める（A は m と s、B は m、D は d が映る）。

const groupUUID = "uuid-group"

func itm(id int, input string) scenemap.Item {
	return scenemap.Item{ID: id, SourceUUID: input, SourceKind: scenemap.InputSource, Enabled: true}
}

func itmOff(id int, input string) scenemap.Item {
	i := itm(id, input)
	i.Enabled = false
	return i
}

// mm は multiMap に edits を当てた地図。
func mm(edits ...func(m *scenemap.Map)) *scenemap.Map {
	m := multiMap()
	for _, e := range edits {
		e(m)
	}
	return m
}

// itemsOf は scene の項目の列を差し替える。
func itemsOf(scene string, items ...scenemap.Item) func(m *scenemap.Map) {
	return func(m *scenemap.Map) {
		c := m.Scenes[scene]
		c.Items = items
		m.Scenes[scene] = c
	}
}

func renamed(scene, name string) func(m *scenemap.Map) {
	return func(m *scenemap.Map) {
		c := m.Scenes[scene]
		c.Name = name
		m.Scenes[scene] = c
	}
}

func dropped(scene string) func(m *scenemap.Map) {
	return func(m *scenemap.Map) { delete(m.Scenes, scene) }
}

func collectionOf(name string) func(m *scenemap.Map) {
	return func(m *scenemap.Map) { m.Collection = name }
}

// emptyMap は古い OBS などの「空の地図」。カメラの対応と項目は空で、シーンの一覧（UUID と名前）だけ持つ。
func emptyMap() *scenemap.Map {
	m := testMap()
	m.Inputs = map[string]scenemap.Input{}
	return m
}

// standbyIsS2 は、待機の設定名 "Standby" を S2 が持ち、S は別の名前になった地図の編集。
func standbyIsS2(m *scenemap.Map) {
	renamed(standby, "BRB")(m)
	renamed(standby2, "Standby")(m)
}

// toMap は期待の状態の地図を m に差し替える（名前も引き直す。ConfigEpoch は進まない）。
func toMap(m *scenemap.Map) func(s *state) {
	return func(s *state) {
		s.Map = cloneTestMap(m)
		resolveNames(s)
	}
}

// itemTo は期待の状態の地図で、container の項目 id の Enabled を変える。
func itemTo(container string, id int, enabled bool) func(s *state) {
	return func(s *state) { setItem(s.Map, container, id, enabled) }
}

func statesTo(m, sub, d scenemap.CameraState) func(s *state) {
	return func(s *state) {
		s.CameraStates = map[string]scenemap.CameraState{camMain: m, camSub: sub, camDesk: d}
	}
}

// failed は、前の AutoFail が失敗済みで、未完了も期待もないこと（再送時刻が近くても、イベント起点の判断は出す）。
var failed = sentAt(PurposeAutoFail, 800)

// wantSnap は Snapshot の分類・健康・映るカメラ・悪い代表を確かめる。
func wantSnap(class SceneClass, has bool, h scenemap.CameraState, bad string, visible ...string) func(t *testing.T, m *Machine) {
	return func(t *testing.T, m *Machine) {
		t.Helper()
		s := m.Snapshot()
		if s.Class != class || s.HasHealth != has || (has && s.Health != h) || s.BadCamera != bad {
			t.Errorf("Snapshot: class=%v has=%v health=%v bad=%q, want class=%v has=%v health=%v bad=%q",
				s.Class, s.HasHealth, s.Health, s.BadCamera, class, has, h, bad)
		}
		if len(s.VisibleCameras) != len(visible) || (len(visible) > 0 && !reflect.DeepEqual(s.VisibleCameras, visible)) {
			t.Errorf("VisibleCameras: got %v, want %v", s.VisibleCameras, visible)
		}
	}
}

// wantHealth は戻り先などのシーン uuid の健康を確かめる（現在のシーンでなくてもよい）。
func wantHealth(uuid string, h scenemap.CameraState, bad string) func(t *testing.T, m *Machine) {
	return func(t *testing.T, m *Machine) {
		t.Helper()
		got, gotBad, has := m.healthOf(uuid)
		if !has || got != h || gotBad != bad {
			t.Errorf("H(%s) = %v %q has=%v, want %v %q", uuid, got, gotBad, has, h, bad)
		}
	}
}

func checks(fns ...func(t *testing.T, m *Machine)) func(t *testing.T, m *Machine) {
	return func(t *testing.T, m *Machine) {
		t.Helper()
		for _, f := range fns {
			f(t, m)
		}
	}
}

// ---- MC-H: 分類・健康・悪い代表（Machine を通して Snapshot で確かめる） ----

func TestMCHealth(t *testing.T) {
	// 現在 A に対する変更 Event（受信状態の組）。MC-H-2/3/4/7 は F(A)、ほかは —。
	runEvRows(t, []evRow{
		{name: "MC-H-1", base: baseMulti, ev: cc(1000, OK, OK, DOWN), change: statesTo(OK, OK, DOWN),
			check: wantSnap(ClassMonitored, true, OK, "", camMain, camSub)},
		{name: "MC-H-2", base: baseMulti, setup: failed, ev: cc(1000, OK, LOW, OK), change: statesTo(OK, LOW, OK),
			out: PurposeAutoFail, bad: camSub, in: nil,
			check: wantSnap(ClassMonitored, true, LOW, camSub, camMain, camSub)},
		{name: "MC-H-3", base: baseMulti, setup: failed, ev: cc(1000, LOW, DOWN, OK), change: statesTo(LOW, DOWN, OK),
			out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, DOWN, camSub, camMain, camSub)},
		{name: "MC-H-4", base: baseMulti, setup: failed, ev: cc(1000, DOWN, DOWN, OK), change: statesTo(DOWN, DOWN, OK),
			out: PurposeAutoFail, bad: camMain,
			check: wantSnap(ClassMonitored, true, DOWN, camMain, camMain, camSub)},
		{name: "MC-H-5", base: baseMulti, setup: useMap(mm(itemsOf(camera, itm(1, inMain), itmOff(2, inSub)))),
			ev: cc(1000, OK, DOWN, OK), change: statesTo(OK, DOWN, OK),
			check: wantSnap(ClassMonitored, true, OK, "", camMain)},
		{name: "MC-H-6", base: baseMulti, setup: useMap(mm(itemsOf(camera, itmOff(1, inMain), itmOff(2, inSub)))),
			ev: cc(1000, OK, OK, DOWN), change: statesTo(OK, OK, DOWN),
			check: wantSnap(ClassOther, false, "", "")},
		{name: "MC-H-7", base: baseMulti, setup: all(failed, func(s *state) {
			s.Cameras = []CameraConfig{cam(camMain), {ID: camSub, OnBad: scenemap.MotionHide}, cam(camDesk)}
		}), ev: cc(1000, OK, DOWN, OK), change: statesTo(OK, DOWN, OK), out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, DOWN, camSub, camMain, camSub)},
		{name: "MC-H-8", base: baseMulti, setup: all(useMap(mm(itemsOf(standby, itm(1, inMain)))), scn(standby)),
			ev: cc(1000, DOWN, OK, OK), change: statesTo(DOWN, OK, OK),
			check: wantSnap(ClassStandby, false, "", "", camMain)},
	})
}

// ---- MC-I: 再起動対象 I(R) ----

func TestMCRestartInputs(t *testing.T) {
	// MC-I-1 は R(A) の Actions で、MC-I-2、3 は地図を変えた R(R) で確かめる。
	toOK := cc(1000, OK, OK, OK)
	badAll := states(LOW, DOWN, OK)
	runEvRows(t, []evRow{
		{name: "MC-I-1", base: baseMulti, setup: all(own(OwnerAuto, camera), badAll), ev: toOK, change: statesTo(OK, OK, OK),
			out: PurposeAutoRecover, in: []string{inMain, inSub}},
		{name: "MC-I-2", base: baseMulti, setup: all(useMap(mm(
			func(m *scenemap.Map) { m.Inputs[inMain2] = scenemap.Input{Name: "main-src2", CameraID: camMain} },
			itemsOf(camera2, itm(1, inMain), itm(2, inMain)))), own(OwnerAuto, camera2), badAll),
			ev: toOK, change: statesTo(OK, OK, OK), out: PurposeAutoRecover, in: []string{inMain, inMain2}},
		{name: "MC-I-3", base: baseMulti, setup: all(own(OwnerAuto, camera2), badAll), ev: toOK, change: statesTo(OK, OK, OK),
			out: PurposeAutoRecover, in: []string{inMain}},
	})

	m := newMachine()
	baseMulti(&m.st)
	// MC-I-4: R にカメラがなければ RestartMedia 自体がなく、SetScene だけ。
	if got := m.restoreActions(talk); !reflect.DeepEqual(got, []Action{SetScene{SceneUUID: talk}}) {
		t.Errorf("MC-I-4: got %+v, want SetScene だけ", got)
	}
	// MC-I-5: 対象が空の合成バッチ生成は、Wait が 0.5秒でも空の Action を作らない。
	for _, target := range []string{talk, standby, "unknown"} {
		for _, a := range m.restoreActions(target) {
			if rm, ok := a.(RestartMedia); ok {
				t.Errorf("MC-I-5: %s に空の RestartMedia %+v", target, rm)
			}
		}
	}
	// 対象のあるシーンは、1つの RestartMedia（設定順、同カメラ内は UUID の辞書順）と SetScene。
	want := []Action{RestartMedia{Inputs: []string{inMain, inSub}, Wait: W}, SetScene{SceneUUID: camera}}
	if got := m.restoreActions(camera); !reflect.DeepEqual(got, want) {
		t.Errorf("MC-I-1: got %+v, want %+v", got, want)
	}
	// 設定の順を s、m、d にすると、入力の順も s が先になる（W-03）。
	m.st.Cameras = []CameraConfig{cam(camSub), cam(camMain), cam(camDesk)}
	if got := m.restoreActions(camera); !reflect.DeepEqual(got[0], RestartMedia{Inputs: []string{inSub, inMain}, Wait: W}) {
		t.Errorf("W-03 の順: got %+v", got[0])
	}
}

// ---- MC-CC: CamerasChanged ----

func TestMCCamerasChanged(t *testing.T) {
	runEvRows(t, []evRow{
		{name: "MC-CC-1", base: baseMulti, setup: all(states(LOW, OK, OK), failed), ev: cc(1000, OK, LOW, OK),
			change: statesTo(OK, LOW, OK), out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, LOW, camSub, camMain, camSub)},
		{name: "MC-CC-2", base: baseMulti, setup: all(own(OwnerAuto, camera), states(LOW, OK, OK)), ev: cc(1000, OK, LOW, OK),
			change: statesTo(OK, LOW, OK), check: wantHealth(camera, LOW, camSub)},
		{name: "MC-CC-3", base: baseMulti, ev: cc(1000, OK, OK, DOWN), change: statesTo(OK, OK, DOWN),
			check: wantSnap(ClassMonitored, true, OK, "", camMain, camSub)},
		{name: "MC-CC-4", base: baseMulti, setup: all(states(DOWN, OK, OK), failed), ev: cc(1000, DOWN, OK, LOW),
			change: statesTo(DOWN, OK, LOW), out: PurposeAutoFail, bad: camMain},
		{name: "MC-CC-5", base: baseMulti, setup: all(own(OwnerAuto, camera), states(DOWN, DOWN, OK)), ev: cc(1000, OK, DOWN, OK),
			change: statesTo(OK, DOWN, OK), check: wantHealth(camera, DOWN, camSub)},
		// 未知のキーは無視し、足りないキーは DOWN に整える（W-05）。同値の判定もこの整えた組で行う（LC-19/0a）。
		{name: "MC-CC-unknown_and_missing", base: baseMulti, setup: all(own(OwnerAuto, camera), states(DOWN, DOWN, OK)),
			ev:     CamerasChanged{At: at(1000), States: map[string]scenemap.CameraState{camMain: OK, "ghost": OK}},
			change: statesTo(OK, DOWN, DOWN), check: wantHealth(camera, DOWN, camSub)},
		{name: "LC-19/0a/normalized", base: baseMulti, setup: states(OK, DOWN, DOWN),
			ev: CamerasChanged{At: at(1000), States: map[string]scenemap.CameraState{camMain: OK, "ghost": LOW}}},
	})
}

// ---- MC-MAP: SceneMapChanged ----

func TestMCSceneMapChanged(t *testing.T) {
	aOnlyM := mm(itemsOf(camera, itm(1, inMain)))
	aNone := mm(itemsOf(camera))
	runEvRows(t, []evRow{
		{name: "MC-MAP-1", base: baseMulti, setup: all(useMap(aNone), states(DOWN, OK, OK), failed),
			ev: smcMap(1000, mm(itemsOf(camera, itm(1, inMain)))), change: toMap(aOnlyM), out: PurposeAutoFail, bad: camMain,
			check: wantSnap(ClassMonitored, true, DOWN, camMain, camMain)},
		{name: "MC-MAP-2", base: baseMulti, setup: useMap(aNone), ev: smcMap(1000, aOnlyM), change: toMap(aOnlyM),
			check: wantSnap(ClassMonitored, true, OK, "", camMain)},
		{name: "MC-MAP-3", base: baseMulti, setup: useMap(aOnlyM), ev: smcMap(1000, aNone), change: toMap(aNone),
			check: wantSnap(ClassOther, false, "", "")},
		{name: "MC-MAP-4", base: baseMulti, setup: all(useMap(aOnlyM), states(OK, DOWN, OK), failed), ev: smcMap(1000, multiMap()),
			change: toMap(multiMap()), out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, DOWN, camSub, camMain, camSub)},
		{name: "MC-MAP-5", base: baseMulti, setup: all(states(OK, DOWN, OK), failed), ev: smcMap(1000, aOnlyM), change: toMap(aOnlyM),
			check: wantSnap(ClassMonitored, true, OK, "", camMain)},
		{name: "MC-MAP-5/pending_then_reject", base: baseMulti, setup: all(states(OK, DOWN, OK), p(1, PurposeAutoFail, camera)),
			ev: smcMap(1000, aOnlyM), change: toMap(aOnlyM), check: func(t *testing.T, m *Machine) {
				if _, ok := m.st.Pending[1]; !ok {
					t.Fatalf("承認待ちの F が消えた")
				}
				if got := m.Approve(1, set(standby), OverrideNone{}, at(1100)); got != (Reject{}) {
					t.Errorf("Approve = %#v, want Reject", got)
				}
			}},
		{name: "MC-MAP-6", base: baseMulti, setup: all(useMap(aOnlyM), own(OwnerAuto, camera)), ev: smcMap(1000, aNone),
			change: all(toMap(aNone), clearOwner), check: wantSnap(ClassStandby, false, "", "")},
		{name: "MC-MAP-7", base: baseMulti, setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, "")),
			ev: smcMap(1000, mm(dropped(camera))), change: all(toMap(mm(dropped(camera))), clearOwner),
			check: func(t *testing.T, m *Machine) {
				if got := m.Approve(1, restartOf(inMain, inSub), OverrideNone{}, at(1100)); got != (Reject{}) {
					t.Errorf("R の Approve = %#v, want Reject", got)
				}
			}},
		{name: "MC-MAP-8", base: baseMulti, setup: all(useMap(aOnlyM), own(OwnerManual, camera)), ev: smcMap(1000, aNone),
			change: toMap(aNone), check: func(t *testing.T, m *Machine) {
				b, err := m.Handle(cmd(2000, CmdStandbyOff))
				if err != nil || b == nil || b.Purpose != PurposeStandbyOffOther || !reflect.DeepEqual(b.Actions, []Action{set(camera)}) {
					t.Errorf("次の Off は Other: got %+v %v", b, err)
				}
			}},
		{name: "MC-MAP-9", base: baseMulti, setup: all(own(OwnerAuto, camera), states(OK, DOWN, OK)), ev: smcMap(1000, aOnlyM),
			change: toMap(aOnlyM), out: PurposeAutoRecover, in: []string{inMain}},
		{name: "MC-MAP-10", base: baseMulti, setup: all(states(DOWN, OK, OK), failed),
			ev: smcMap(1000, mm(renamed(camera, "Camera-renamed"))), change: toMap(mm(renamed(camera, "Camera-renamed"))),
			out: PurposeAutoFail, bad: camMain},
		{name: "MC-MAP-11/manual", base: baseMulti, setup: own(OwnerManual, camera), ev: smcMap(1000, mm(renamed(standby, "Gone"))),
			change: all(toMap(mm(renamed(standby, "Gone"))), clearOwner), check: wantSnap(ClassOther, false, "", "")},
		{name: "MC-MAP-11/auto", base: baseMulti, setup: own(OwnerAuto, camera), ev: smcMap(1000, mm(renamed(standby, "Gone"))),
			change: all(toMap(mm(renamed(standby, "Gone"))), clearOwner), check: wantSnap(ClassOther, false, "", "")},
		{name: "MC-MAP-12", base: baseMulti, setup: all(useMap(mm(renamed(standby, "Gone"))), states(DOWN, OK, OK), failed),
			ev: smcMap(1000, multiMap()), change: toMap(multiMap()), out: PurposeAutoFail, to: standby, bad: camMain},
		{name: "MC-MAP-13", base: baseMulti, setup: all(useMap(emptyMap()), states(DOWN, OK, OK), failed),
			ev: smcMap(1000, multiMap()), change: toMap(multiMap()), out: PurposeAutoFail, bad: camMain},
		{name: "MC-MAP-14", base: baseMulti, setup: all(states(DOWN, OK, OK), failed),
			ev: smcMap(1000, mm(func(m *scenemap.Map) { m.Groups = nil }))},
		{name: "MC-MAP-17", base: baseMulti, setup: all(own(OwnerAuto, camera), failed, sentAt(PurposeAutoRecover, 800)),
			ev: smcMap(1000, mm(renamed(camera, "Camera-renamed"))), change: toMap(mm(renamed(camera, "Camera-renamed"))),
			out: PurposeAutoRecover, in: []string{inMain, inSub}},
		// J の抑止（切替中・未完了・期待）。地図は更新するが、バッチは出さない。
		{name: "MC-MAP-X/pending", base: baseMulti, setup: all(scn(talk), states(DOWN, OK, OK), p(1, PurposeStandbyOn, talk)),
			ev: smcMap(1000, mm(itemsOf(talk, itm(1, inMain)))), change: toMap(mm(itemsOf(talk, itm(1, inMain))))},
		{name: "MC-MAP-X/expectation", base: baseMulti, setup: all(scn(talk), states(DOWN, OK, OK), x(1, standby, PurposeStandbyOn, talk)),
			ev: smcMap(1000, mm(itemsOf(talk, itm(1, inMain)))), change: toMap(mm(itemsOf(talk, itm(1, inMain))))},
		{name: "MC-MAP-P", base: baseMulti, setup: all(states(DOWN, OK, OK), p(1, PurposeAutoFail, camera)),
			ev: smcMap(1000, mm(renamed(deskScene, "Desk2"))), change: toMap(mm(renamed(deskScene, "Desk2")))},
		{name: "MC-MAP-E", base: baseMulti, setup: all(states(DOWN, OK, OK), x(1, standby, PurposeAutoFail, camera)),
			ev: smcMap(1000, mm(renamed(deskScene, "Desk2"))), change: toMap(mm(renamed(deskScene, "Desk2")))},
		{name: "MC-MAP-NS", base: baseMulti, setup: all(useMap(aNone), notStreaming, states(DOWN, OK, OK)),
			ev: smcMap(1000, aOnlyM), change: toMap(aOnlyM)},
		{name: "MC-MAP-OFF", base: baseMulti, setup: all(useMap(aNone), autoOff, states(DOWN, OK, OK)),
			ev: smcMap(1000, aOnlyM), change: toMap(aOnlyM)},
		{name: "MC-MAP-DC", base: baseMulti, setup: all(disconnected, states(DOWN, OK, OK)), ev: smcMap(1000, mm(itemsOf(deskScene)))},
		{name: "MC-MAP-RX", base: baseMulti, setup: all(own(OwnerAuto, camera), p(2, PurposeAutoRecover, ""), x(1, camera, PurposeAutoRecover, "")),
			ev: smcMap(1000, aNone), change: all(toMap(aNone), clearOwner), check: func(t *testing.T, m *Machine) {
				e := m.st.SceneEpoch
				b, err := m.Handle(sc(1100, camera))
				if b != nil || err != nil || m.st.SceneEpoch != e || m.st.Scene != camera || len(m.st.Expectations) != 0 {
					t.Errorf("続く SceneChanged(A): batch=%+v err=%v epoch=%d→%d expectations=%d", b, err, e, m.st.SceneEpoch, len(m.st.Expectations))
				}
				wantSnap(ClassOther, false, "", "")(t, m)
			}},
		{name: "MC-MAP-S2", base: baseMulti, setup: all(states(DOWN, OK, OK), x(1, standby, PurposeAutoFail, camera)),
			ev: smcMap(1000, mm(standbyIsS2)), change: toMap(mm(standbyIsS2)), out: PurposeAutoFail, to: standby2, bad: camMain},
		{name: "MC-MAP-OWNER/auto", base: baseMulti, setup: own(OwnerAuto, camera), ev: smcMap(1000, mm(standbyIsS2)),
			change: all(toMap(mm(standbyIsS2)), clearOwner), check: func(t *testing.T, m *Machine) {
				if m.st.StandbyUUID != standby2 {
					t.Errorf("StandbyUUID = %q, want S2", m.st.StandbyUUID)
				}
				wantSnap(ClassOther, false, "", "")(t, m)
			}},
		{name: "MC-MAP-OWNER/manual", base: baseMulti, setup: own(OwnerManual, camera), ev: smcMap(1000, mm(standbyIsS2)),
			change: all(toMap(mm(standbyIsS2)), clearOwner), check: wantSnap(ClassOther, false, "", "")},
	})
	// MC-MAP-15（切断中の古い読み直し結果）と MC-MAP-16（読み直し失敗）は core / obs 側の約束で、Machine には届かない（M-8）。
	t.Run("MC-MAP-15", func(t *testing.T) {
		t.Skip("core が切断中の古い読み直し結果を捨てる。Machine には届かない（M-8 で確かめる）")
	})
	t.Run("MC-MAP-16", func(t *testing.T) {
		t.Skip("読み直し失敗では Event を流さない。Machine には届かない（M-8 で確かめる）")
	})
}

// ---- MC-ITEM: ItemEnabledChanged ----

func TestMCItemEnabledChanged(t *testing.T) {
	aOnlyM := mm(itemsOf(camera, itm(1, inMain)))
	groupMap := mm(func(m *scenemap.Map) {
		// A: m（直接）と、s を持つグループ G。さらに s を A の直接の項目でも持つ（別の表示経路）。
		m.Groups[groupUUID] = scenemap.Container{Name: "G", Items: []scenemap.Item{itm(1, inSub)}}
		itemsOf(camera, itm(1, inMain), itm(2, inSub), scenemap.Item{ID: 3, SourceUUID: groupUUID, SourceKind: scenemap.GroupSource, Enabled: true})(m)
	})
	nestedMap := mm(func(m *scenemap.Map) {
		// A: m と s のどちらも、入れ子のグループ G を通る唯一の経路。
		m.Groups[groupUUID] = scenemap.Container{Name: "G", Items: []scenemap.Item{itm(1, inMain), itm(2, inSub)}}
		itemsOf(camera, scenemap.Item{ID: 1, SourceUUID: groupUUID, SourceKind: scenemap.GroupSource, Enabled: true})(m)
	})
	talkOff := mm(itemsOf(talk, itmOff(1, inMain)))
	aOff := mm(itemsOf(camera, itmOff(1, inMain), itmOff(2, inSub)))
	runEvRows(t, []evRow{
		{name: "MC-ITEM-1", base: baseMulti, setup: all(states(OK, DOWN, OK), failed), ev: item(1000, camera, 2, false),
			change: itemTo(camera, 2, false), check: wantSnap(ClassMonitored, true, OK, "", camMain)},
		{name: "MC-ITEM-2", base: baseMulti, setup: all(useMap(mm(itemsOf(camera, itm(1, inMain), itmOff(2, inSub)))), states(OK, DOWN, OK), failed),
			ev: item(1000, camera, 2, true), change: itemTo(camera, 2, true), out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, DOWN, camSub, camMain, camSub)},
		{name: "MC-ITEM-3", base: baseMulti, setup: useMap(aOnlyM), ev: item(1000, camera, 1, false), change: itemTo(camera, 1, false),
			check: wantSnap(ClassOther, false, "", "")},
		{name: "MC-ITEM-4", base: baseMulti, setup: all(useMap(aOnlyM), own(OwnerAuto, camera)), ev: item(1000, camera, 1, false),
			change: all(itemTo(camera, 1, false), clearOwner)},
		{name: "MC-ITEM-5", base: baseMulti, setup: all(own(OwnerAuto, camera), states(OK, DOWN, OK)), ev: item(1000, camera, 2, false),
			change: itemTo(camera, 2, false), out: PurposeAutoRecover, in: []string{inMain}},
		{name: "MC-ITEM-6", base: baseMulti, setup: all(useMap(aOnlyM), own(OwnerManual, camera)), ev: item(1000, camera, 1, false),
			change: itemTo(camera, 1, false), check: func(t *testing.T, m *Machine) {
				b, err := m.Handle(cmd(2000, CmdStandbyOff))
				if err != nil || b == nil || b.Purpose != PurposeStandbyOffOther {
					t.Errorf("Off は Other: got %+v %v", b, err)
				}
			}},
		{name: "MC-ITEM-7", base: baseMulti, setup: all(useMap(groupMap), states(OK, DOWN, OK), failed), ev: item(1000, groupUUID, 1, false),
			change: itemTo(groupUUID, 1, false), out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, DOWN, camSub, camMain, camSub)},
		{name: "MC-ITEM-8", base: baseMulti, setup: useMap(nestedMap), ev: item(1000, camera, 1, false), change: itemTo(camera, 1, false),
			check: wantSnap(ClassOther, false, "", "")},
		{name: "MC-ITEM-9", base: baseMulti, setup: all(states(DOWN, OK, OK), failed), ev: item(1000, camera, 1, true)},
		{name: "MC-ITEM-10/unknown_item", base: baseMulti, setup: all(states(DOWN, OK, OK), failed), ev: item(1000, camera, 99, true)},
		{name: "MC-ITEM-10/unknown_container", base: baseMulti, setup: all(states(DOWN, OK, OK), failed), ev: item(1000, "uuid-unknown", 1, true)},
		{name: "MC-ITEM-X/pending", base: baseMulti, setup: all(useMap(talkOff), scn(talk), states(DOWN, OK, OK), p(1, PurposeStandbyOn, talk)),
			ev: item(1000, talk, 1, true), change: itemTo(talk, 1, true)},
		{name: "MC-ITEM-X/expectation", base: baseMulti, setup: all(useMap(talkOff), scn(talk), states(DOWN, OK, OK), x(1, standby, PurposeStandbyOn, talk)),
			ev: item(1000, talk, 1, true), change: itemTo(talk, 1, true)},
		{name: "MC-ITEM-P", base: baseMulti, setup: all(states(DOWN, OK, OK), p(1, PurposeAutoFail, camera)),
			ev: item(1000, deskScene, 1, false), change: itemTo(deskScene, 1, false)},
		{name: "MC-ITEM-E", base: baseMulti, setup: all(states(DOWN, OK, OK), x(1, standby, PurposeAutoFail, camera)),
			ev: item(1000, deskScene, 1, false), change: itemTo(deskScene, 1, false)},
		{name: "MC-ITEM-NS", base: baseMulti, setup: all(useMap(mm(itemsOf(camera, itmOff(1, inMain)))), notStreaming, states(DOWN, OK, OK)),
			ev: item(1000, camera, 1, true), change: itemTo(camera, 1, true)},
		{name: "MC-ITEM-OFF", base: baseMulti, setup: all(useMap(mm(itemsOf(camera, itmOff(1, inMain)))), autoOff, states(DOWN, OK, OK)),
			ev: item(1000, camera, 1, true), change: itemTo(camera, 1, true)},
		{name: "MC-ITEM-DC", base: baseMulti, setup: all(disconnected, states(DOWN, OK, OK)), ev: item(1000, camera, 1, false)},
		{name: "MC-ITEM-RX", base: baseMulti, setup: all(useMap(aOnlyM), own(OwnerAuto, camera), p(2, PurposeAutoRecover, ""), x(1, camera, PurposeAutoRecover, "")),
			ev: item(1000, camera, 1, false), change: all(itemTo(camera, 1, false), clearOwner), check: func(t *testing.T, m *Machine) {
				e := m.st.SceneEpoch
				b, err := m.Handle(sc(1100, camera))
				if b != nil || err != nil || m.st.SceneEpoch != e || len(m.st.Expectations) != 0 {
					t.Errorf("到着した A: batch=%+v err=%v", b, err)
				}
				wantSnap(ClassOther, false, "", "")(t, m)
			}},
		// 人が非表示にした項目の下の入れ子の子は変わらない（MC-ITEM-8 の続き）と、
		// 全項目を false にした A は、表示を戻すとまた映る。
		{name: "MC-ITEM-8/restore", base: baseMulti, setup: all(useMap(aOff), states(OK, DOWN, OK), failed), ev: item(1000, camera, 2, true),
			change: itemTo(camera, 2, true), out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, DOWN, camSub, camSub)},
	})
	t.Run("MC-ITEM-11", func(t *testing.T) {
		t.Skip("切断中の旧接続の項目 Event は obs/core が捨てる（M-8）。Machine では MC-ITEM-DC")
	})
}

// ---- 接続・切断（MC-OC） ----

func TestMCConnect(t *testing.T) {
	reconnected := func(m *scenemap.Map) func(s *state) {
		return func(s *state) {
			s.Connected = true
			s.Map = cloneTestMap(m)
			resolveNames(s)
		}
	}
	aGone := mm(dropped(camera))
	rows := []evRow{
		{name: "MC-OC-1", base: baseMulti, setup: all(states(DOWN, OK, OK), failed, disconnected),
			ev: connM(1000, true, camera, mm(renamed(camera, "Camera-renamed"))), change: reconnected(mm(renamed(camera, "Camera-renamed"))),
			out: PurposeAutoFail, bad: camMain},
		{name: "MC-OC-2/auto", base: baseMulti, setup: all(own(OwnerAuto, camera), disconnected),
			ev: connM(1000, true, standby, aGone), change: all(reconnected(aGone), clearOwner)},
		{name: "MC-OC-2/manual", base: baseMulti, setup: all(own(OwnerManual, camera), disconnected),
			ev: connM(1000, true, standby, aGone), change: reconnected(aGone)},
		{name: "MC-OC-3", base: baseMulti, setup: all(own(OwnerAuto, camera), disconnected),
			ev: connM(1000, true, standby, emptyMap()), change: all(reconnected(emptyMap()), clearOwner),
			check: wantSnap(ClassStandby, false, "", "")},
		{name: "MC-OC-4/auto", base: baseMulti, setup: all(own(OwnerAuto, camera), disconnected),
			ev: connM(1000, true, standby, mm(collectionOf("collection-y"))), change: all(reconnected(mm(collectionOf("collection-y"))), clearOwner),
			check: wantSnap(ClassStandby, false, "", "")},
		{name: "MC-OC-4/manual", base: baseMulti, setup: all(own(OwnerManual, camera), disconnected),
			ev: connM(1000, true, standby, mm(collectionOf("collection-y"))), change: all(reconnected(mm(collectionOf("collection-y"))), clearOwner),
			check: wantSnap(ClassStandby, false, "", "")},
		{name: "MC-OC-5/auto", base: baseMulti, setup: all(own(OwnerAuto, camera), disconnected),
			ev: connM(1000, true, standby, mm(standbyIsS2)), change: all(reconnected(mm(standbyIsS2)), clearOwner),
			check: wantSnap(ClassOther, false, "", "")},
		{name: "MC-OC-5/manual", base: baseMulti, setup: all(own(OwnerManual, camera), disconnected),
			ev: connM(1000, true, standby, mm(standbyIsS2)), change: all(reconnected(mm(standbyIsS2)), clearOwner),
			check: wantSnap(ClassOther, false, "", "")},
		// OC-6/0a: 名前は同じでも program UUID が別なら、持ち主を消す。SceneEpoch は進めない（U-23）。
		{name: "OC-6/0a/uuid_differs", base: baseMulti, setup: all(own(OwnerAuto, camera), disconnected),
			ev: OBSConnected{At: at(1000), Streaming: true, Scene: "Standby", SceneUUID: camera2, TransitionMs: ip(300), Map: multiMap()},
			change: all(reconnected(multiMap()), func(s *state) {
				s.Scene = camera2
				clearOwner(s)
			})},
	}
	runEvRows(t, rows)
}

// ---- SceneChanged の追加の行 ----

func TestMCSceneChanged(t *testing.T) {
	renamedA := mm(renamed(camera, "Camera-renamed"))
	runEvRows(t, []evRow{
		{name: "MC-SC-1", base: baseMulti, setup: all(useMap(renamedA), own(OwnerAuto, camera), x(1, camera, PurposeAutoRecover, "")),
			ev: sc(1000, camera), change: all(arrived(camera), clearOwner)},
		{name: "MC-SC-2", base: baseMulti, setup: all(own(OwnerAuto, camera), x(1, camera, PurposeAutoRecover, "")),
			ev: SceneChanged{At: at(1000), Scene: "Camera", SceneUUID: camera2}, change: manualTo(camera2)},
		{name: "MC-SC-3/auto_fail", base: baseMulti, setup: all(useMap(mm(standbyIsS2)), x(1, standby, PurposeAutoFail, camera)),
			ev: sc(1000, standby), change: arrived(standby), check: wantSnap(ClassOther, false, "", "")},
		{name: "MC-SC-3/standby_on", base: baseMulti, setup: all(useMap(mm(standbyIsS2)), x(1, standby, PurposeStandbyOn, camera)),
			ev: sc(1000, standby), change: arrived(standby), check: wantSnap(ClassOther, false, "", "")},
		// SCE-1/0a、SCE-2/0a: AutoFail の期待の戻り先が地図で監視対象外なら、到着時も auto を入れない（W-02）。
		{name: "SCE-1/0a/return_other", base: baseMulti, setup: all(useMap(mm(itemsOf(camera))), x(1, standby, PurposeAutoFail, camera)),
			ev: sc(1000, standby), change: arrived(standby)},
		{name: "SCE-2/0a/multi", base: baseMulti, setup: x(1, standby, PurposeAutoFail, camera), ev: sc(1000, standby),
			change: all(arrived(standby), setOwner(OwnerAuto, camera)), out: PurposeAutoRecover, in: []string{inMain, inSub}},
		{name: "SCE-5/0a/multi", base: baseMulti, setup: all(own(OwnerAuto, camera), states(OK, LOW, OK), x(1, camera, PurposeAutoRecover, "")),
			ev: sc(1000, camera), change: all(arrived(camera), clearOwner), out: PurposeAutoFail, ret: camera, bad: camSub},
	})
}

// ---- ScenesConfigured の追加の行 ----

func TestMCConfigured(t *testing.T) {
	reorder := withCameras(cam(camSub), cam(camMain), cam(camDesk))
	subHide := withCameras(cam(camMain), CameraConfig{ID: camSub, OnBad: scenemap.MotionHide}, cam(camDesk))
	aOnlyS := mm(itemsOf(camera, itm(2, inSub)))
	noSuchStandby := func(c *ScenesConfigured) { c.Standby = "NoSuchScene" }
	runEvRows(t, []evRow{
		{name: "MC-CFG-1", base: baseMulti, setup: all(states(DOWN, DOWN, OK), failed), ev: cfg(1000, reorder),
			change: cfgChange(false, reorder), out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, DOWN, camSub, camMain, camSub)},
		{name: "MC-CFG-2", base: baseMulti, setup: all(states(OK, DOWN, OK), failed), ev: cfg(1000, subHide),
			change: cfgChange(false, subHide), out: PurposeAutoFail, bad: camSub,
			check: wantSnap(ClassMonitored, true, DOWN, camSub, camMain, camSub)},
		{name: "MC-CFG-3", base: baseMulti, setup: all(useMap(aOnlyS), own(OwnerAuto, camera), states(OK, LOW, OK)),
			ev: cfg(1000, withCameras(cam(camMain), cam(camDesk))), change: cfgChange(true, withCameras(cam(camMain), cam(camDesk))),
			check: func(t *testing.T, m *Machine) {
				if _, ok := m.st.CameraStates[camSub]; ok {
					t.Errorf("s の状態が残っている")
				}
				if _, _, has := m.healthOf(camera); has {
					t.Errorf("CalcMap で A は Other のはず")
				}
			}},
		{name: "MC-CFG-3/add", base: baseMulti, setup: own(OwnerAuto, camera),
			ev:     cfg(1000, withCameras(cam(camMain), cam(camSub), cam(camDesk), cam("extra"))),
			change: cfgChange(false, withCameras(cam(camMain), cam(camSub), cam(camDesk), cam("extra"))),
			out:    PurposeAutoRecover, in: []string{inMain, inSub},
			check: func(t *testing.T, m *Machine) {
				if got := m.st.CameraStates["extra"]; got != DOWN {
					t.Errorf("追加した id = %v, want DOWN", got)
				}
			}},
		{name: "MC-CFG-4", base: baseMulti, setup: all(states(DOWN, OK, OK), failed), ev: cfg(1000, withMulti(), noSuchStandby),
			change: cfgChange(false, withMulti(), noSuchStandby)},
		{name: "MC-CFG-5", base: baseMulti, setup: all(states(DOWN, OK, OK), failed, startAt(opening)), ev: cfg(1000, withMulti(), withStart(talk)),
			change: cfgChange(false, withMulti(), withStart(talk)), out: PurposeAutoFail, bad: camMain},
		// SCF-9/0a、SCF-10/0a: カメラ一覧の変更で戻り先が監視対象外になる。auto は C、manual は保持。
		{name: "SCF-9/0a", base: baseMulti, setup: all(useMap(aOnlyS), own(OwnerAuto, camera)),
			ev: cfg(1000, withCameras(cam(camMain), cam(camDesk))), change: cfgChange(true, withCameras(cam(camMain), cam(camDesk)))},
		{name: "SCF-10/0a", base: baseMulti, setup: all(useMap(aOnlyS), own(OwnerManual, camera)),
			ev: cfg(1000, withCameras(cam(camMain), cam(camDesk))), change: cfgChange(false, withCameras(cam(camMain), cam(camDesk)))},
		// SCF-21/0a: 切断中は監視対象の判定を再接続まで保留する（W-05）。
		{name: "SCF-21/0a/pending_until_reconnect", base: baseMulti, setup: all(useMap(aOnlyS), own(OwnerAuto, camera), disconnected),
			ev: cfg(1000, withCameras(cam(camMain), cam(camDesk))), change: cfgChange(false, withCameras(cam(camMain), cam(camDesk))),
			check: func(t *testing.T, m *Machine) {
				if m.st.StandbyOwner != OwnerAuto {
					t.Fatalf("切断中に C した")
				}
				m.Handle(connM(2000, true, standby, aOnlyS))
				if m.st.StandbyOwner != OwnerNone {
					t.Errorf("再接続で C するはず")
				}
			}},
		{name: "SCF-2/0a/on_bad", base: baseMulti, ev: cfg(1000, subHide), change: cfgChange(false, subHide)},
		{name: "SCF-2/0a/same_multi", base: baseMulti, ev: cfg(1000, withMulti())},
	})
}

// ---- Command と配信開始の追加の行 ----

func TestMCCommands(t *testing.T) {
	startMissing := func(s *state) {
		s.StartScene = sp("NoSuchScene")
		resolveNames(s)
	}
	runEvRows(t, []evRow{
		{name: "MC-CMD-1", base: baseMulti, setup: all(own(OwnerManual, camera), useMap(mm(dropped(camera)))), ev: cmd(1000, CmdStandbyOff),
			out: PurposeStandbyOffOther},
		{name: "CMD-SOFF-5/0a/multi", base: baseMulti, setup: own(OwnerManual, camera), ev: cmd(1000, CmdStandbyOff),
			out: PurposeStandbyOffMonitored, in: []string{inMain, inSub}},
		{name: "CMD-SOFF-6/0a/multi", base: baseMulti, setup: all(own(OwnerManual, camera), states(OK, LOW, OK)), ev: cmd(1000, CmdStandbyOff),
			change: func(s *state) { s.StandbyOwner = OwnerAuto }},
		{name: "MC-CMD-2", base: baseMulti, setup: useMap(mm(renamed(standby, "Gone"))), ev: cmd(1000, CmdStandbyOn),
			err: invalid, reason: ReasonStandbyNotFound},
		{name: "MC-CMD-2/after_streaming_check", base: baseMulti, setup: all(useMap(mm(renamed(standby, "Gone"))), notStreaming),
			ev: cmd(1000, CmdStandbyOn), err: invalid, reason: ReasonNotStreaming},
		{name: "MC-CMD-2/before_already_standby", base: baseMulti, setup: all(useMap(mm(renamed(standby, "Gone"))), func(s *state) { s.StandbyOwner = OwnerManual }),
			ev: cmd(1000, CmdStandbyOn), err: invalid, reason: ReasonStandbyNotFound},
		{name: "MC-START-1/bad", base: baseMulti, setup: all(notStreaming, startMissing, states(DOWN, OK, OK)), ev: ssc(1000, true),
			change: func(s *state) { s.Streaming, s.StreamEpoch = true, s.StreamEpoch+1 }, out: PurposeAutoFail, bad: camMain, recErr: true},
		{name: "MC-START-1/ok", base: baseMulti, setup: all(notStreaming, startMissing), ev: ssc(1000, true),
			change: func(s *state) { s.Streaming, s.StreamEpoch = true, s.StreamEpoch+1 }, recErr: true},
		{name: "SST-1/0a/multi", base: baseMulti, setup: all(notStreaming, states(OK, DOWN, OK)), ev: ssc(1000, true),
			change: func(s *state) { s.Streaming, s.StreamEpoch = true, s.StreamEpoch+1 }, out: PurposeAutoFail, bad: camSub},
		// SSF-1/0a: 配信停止でも、地図とカメラ状態は保つ。
		{name: "SSF-1/0a/keeps_map", base: baseMulti, setup: all(own(OwnerAuto, camera), states(OK, DOWN, OK)), ev: ssc(1000, false),
			change: streamOff},
	})
}

// ---- Approve の追加の行 ----

func TestMCApprove(t *testing.T) {
	renamedA := mm(renamed(camera, "Camera-renamed"))
	standbyMoved := mm(standbyIsS2)
	aBad := states(OK, DOWN, OK)
	toAuto := func(s *state) { s.StandbyOwner = OwnerAuto }
	runApRows(t, []apRow{
		{name: "MC-AP-1", base: baseMulti, setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, ""), useMap(mm(dropped(camera))), clearOwner),
			action: set(camera), atMs: 1000, res: Reject{}},
		{name: "MC-AP-2", base: baseMulti, setup: all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, ""), useMap(mm(itemsOf(camera)))),
			action: set(camera), atMs: 1000, res: Reject{}},
		{name: "MC-AP-3/auto_recover", base: baseMulti, setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, ""), aBad),
			action: set(camera), atMs: 1000, res: Reject{}},
		{name: "MC-AP-3/off_monitored", base: baseMulti, setup: all(own(OwnerManual, camera), p(1, PurposeStandbyOffMonitored, ""), aBad),
			action: set(camera), atMs: 1000, res: Replace{}, change: all(toAuto, markReplaced)},
		{name: "MC-AP-4/auto_fail", base: baseMulti, setup: all(states(DOWN, OK, OK), p(1, PurposeAutoFail, camera), useMap(renamedA)),
			action: set(standby), atMs: 1000, res: Proceed{}, register: true},
		{name: "MC-AP-4/auto_recover", base: baseMulti, setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, ""), useMap(renamedA)),
			action: set(camera), atMs: 1000, res: Proceed{}, register: true},
		{name: "MC-AP-5", base: baseMulti, setup: p(1, PurposeAutoFail, camera), action: set(camera), atMs: 1000, res: Skip{}},
		{name: "MC-AP-6", base: baseMulti, setup: all(own(OwnerAuto, camera), p(1, PurposeAutoRecover, "")),
			action: restartOf("in-vanished"), atMs: 1000, res: Proceed{}},
		{name: "MC-AP-7", base: baseMulti, setup: all(states(DOWN, OK, OK), p(1, PurposeAutoFail, camera), useMap(standbyMoved)),
			action: set(standby), atMs: 1000, res: Reject{}},
		{name: "MC-AP-8", base: baseMulti, setup: all(p(1, PurposeStandbyOn, camera), useMap(standbyMoved)),
			action: set(standby), atMs: 1000, res: Reject{}, check: func(t *testing.T, m *Machine) {
				m.Handle(bd(1100, 1, BatchRejected))
				if m.st.LastError == nil {
					t.Errorf("BD-R-2 に従い BatchDone で LastError を記録するはず")
				}
			}},
		{name: "MC-AP-9", base: baseMulti, setup: all(own(OwnerManual, talk), p(1, PurposeStandbyOffOther, ""), useMap(mm(itemsOf(talk, itm(1, inMain))))),
			action: set(talk), atMs: 1000, res: Reject{}},
	})
}

// ---- 0a の Approve の入口（AP-…/0a）で、複数のカメラの健康を使う行 ----

func TestApproveMultiHealth(t *testing.T) {
	runApRows(t, []apRow{
		{name: "AP-AUTOFAIL-1/0a/multi", base: baseMulti, setup: all(states(OK, LOW, OK), p(1, PurposeAutoFail, camera)),
			action: set(standby), atMs: 1000, res: Proceed{}, register: true},
		{name: "AP-AUTOFAIL-2/0a/multi", base: baseMulti, setup: all(states(OK, OK, DOWN), p(1, PurposeAutoFail, camera)),
			action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-AUTOFAIL-4/0a/multi", base: baseMulti, setup: all(states(OK, DOWN, OK), p(1, PurposeAutoFail, camera), useMap(mm(itemsOf(camera)))),
			action: set(standby), atMs: 1000, res: Reject{}},
		{name: "AP-AUTOREC-3/0a/multi", base: baseMulti, setup: all(own(OwnerAuto, camera), states(OK, LOW, OK), p(1, PurposeAutoRecover, "")),
			action: set(camera), atMs: 1000, res: Reject{}},
		{name: "AP-SOFFM-3/0a/multi", base: baseMulti, setup: all(own(OwnerManual, camera), states(LOW, OK, OK), p(1, PurposeStandbyOffMonitored, "")),
			action: restartOf(inMain, inSub), atMs: 1000, res: Replace{}, change: all(func(s *state) { s.StandbyOwner = OwnerAuto }, markReplaced)},
		{name: "AP-SOFFM-4/0a/multi", base: baseMulti, setup: all(autoOff, own(OwnerManual, camera), states(LOW, OK, OK), p(1, PurposeStandbyOffMonitored, "")),
			action: set(camera), atMs: 1000, res: Proceed{}, register: true},
	})
}

// ---- U-15/0a: 初期値 ----

func TestInitial0a(t *testing.T) {
	m := newMachine()
	s := m.st
	if s.Connected || s.Map != nil || len(s.CameraStates) != 0 || len(s.Cameras) != 0 || s.Streaming || s.PrevConnStreaming ||
		s.Scene != "" || s.PrevConnScene != "" || s.PrevConnCollection != "" ||
		s.SceneEpoch != 0 || s.ConnEpoch != 0 || s.StreamEpoch != 0 || s.ConfigEpoch != 0 ||
		s.StandbyOwner != OwnerNone || s.ReturnScene != "" || len(s.Expectations) != 0 || len(s.Pending) != 0 || len(s.LastAutoSent) != 0 {
		t.Fatalf("New 直後の状態 = %+v", s)
	}
	// 最初の ScenesConfigured 後: 各カメラ DOWN、設定の名前を保持、ConfigEpoch=1。地図がないので UUID は空、分類は不明、健康なし。
	if b, err := m.Handle(cfg(0, withMulti())); b != nil || err != nil {
		t.Fatalf("batch %+v err %v", b, err)
	}
	s = m.st
	want := map[string]scenemap.CameraState{camMain: DOWN, camSub: DOWN, camDesk: DOWN}
	if !reflect.DeepEqual(s.CameraStates, want) || s.ConfigEpoch != 1 || s.Standby != "Standby" || s.StandbyUUID != "" {
		t.Fatalf("設定後の状態 = %+v", s)
	}
	if snap := m.Snapshot(); snap.Class != ClassUnknown || snap.HasHealth {
		t.Fatalf("設定後の Snapshot = %+v", snap)
	}
	// 切断では地図を消すが、カメラ状態は DOWN に戻さない。切断中に組が変われば保存し、Action は出さない。
	m.Handle(cc(1, OK, OK, OK))
	m.Handle(connM(2, true, camera, multiMap()))
	m.Handle(disc(3))
	if b, _ := m.Handle(cc(4, LOW, OK, OK)); b != nil {
		t.Fatalf("切断中に Action を出した: %+v", b)
	}
	if m.st.Map != nil || m.st.CameraStates[camMain] != LOW || m.st.PrevConnScene != camera || m.st.PrevConnCollection != collectionX {
		t.Fatalf("切断後の状態 = %+v", m.st)
	}
}

// 0章: LastError の切替先の文には現在の地図の名前を使い、名前がなければ UUID を使う。
func TestLastErrorUsesMapName(t *testing.T) {
	expire := func(t *testing.T, mk func(m *Machine)) string {
		m := newMachine()
		baseState(&m.st)
		m.st.StartScene = sp("Opening")
		resolveNames(&m.st)
		addExpWithDeadline := answered(waiting(&m.st, 1, opening, PurposeStartScene, ""), RequestOK, 0, 500)
		m.st.Expectations = []Expectation{addExpWithDeadline}
		mk(m)
		m.Handle(tick(1000))
		if m.st.LastError == nil {
			t.Fatalf("LastError なし")
		}
		return m.st.LastError.Msg
	}
	if msg := expire(t, func(m *Machine) {}); !strings.Contains(msg, "Opening") || strings.Contains(msg, opening) {
		t.Errorf("名前を使う: %q", msg)
	}
	if msg := expire(t, func(m *Machine) { m.st.Map = nil }); !strings.Contains(msg, opening) {
		t.Errorf("名前がなければ UUID: %q", msg)
	}
}
