package switcher

import (
	"testing"
	"time"
)

func TestHumanSceneIntentArrival(t *testing.T) {
	for _, arrivalFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "通知が先", false: "応答が先"}[arrivalFirst], func(t *testing.T) {
			m := New(Config{})
			at := time.Unix(100, 0)
			m.st.Connected = true
			m.st.Streaming = true
			m.st.AutoEnabled = true
			m.st.Scene = "a"
			m.st.Expectations = []Expectation{{Scene: "u", Purpose: PurposeAutoFail}}
			m.st.StandbyOwner = OwnerAuto
			m.st.Stack = []string{"a"}
			m.Handle(HumanSceneSelected{ID: 1, At: at, SceneUUID: "u", Deadline: at.Add(7 * time.Second)})
			if m.st.Scene != "a" {
				t.Fatal("到着前に更新")
			}
			if _, ok := m.decide(); ok {
				t.Fatal("意図の間の自動切替")
			}
			// 要求の応答はシーンを更新する入力を持たない。
			if !arrivalFirst {
				m.Handle(Tick{At: at.Add(time.Second)})
			}
			m.Handle(SceneChanged{At: at.Add(2 * time.Second), SceneUUID: "u"})
			if len(m.st.Expectations) != 0 || m.st.StandbyOwner != OwnerNone || len(m.st.Stack) != 0 || len(m.st.HumanIntents) > 0 {
				t.Fatalf("人の切替にならない: %+v", m.st)
			}
		})
	}
}
func TestHumanSceneIntentCancelExpirySame(t *testing.T) {
	at := time.Unix(100, 0)
	for _, mode := range []string{"取消", "期限", "同じシーン"} {
		t.Run(mode, func(t *testing.T) {
			m := New(Config{})
			m.st.Scene = "u"
			target := "v"
			if mode == "同じシーン" {
				target = "u"
				m.st.StandbyOwner = OwnerManual
				m.st.ReturnScene = "a"
			}
			m.Handle(HumanSceneSelected{ID: 1, At: at, SceneUUID: target, Deadline: at.Add(7 * time.Second)})
			switch mode {
			case "取消":
				m.Handle(HumanSceneSelected{ID: 2, At: at, Cancel: true})
				if len(m.st.HumanIntents) == 0 {
					t.Fatal("別の要求で取消")
				}
				m.Handle(HumanSceneSelected{ID: 1, At: at, Cancel: true})
			case "期限":
				m.Handle(Tick{At: at.Add(8 * time.Second)})
			case "同じシーン":
				if m.st.StandbyOwner != OwnerNone || m.st.ReturnScene != "" {
					t.Fatal("解除されない")
				}
			}
			if len(m.st.HumanIntents) > 0 {
				t.Fatal("意図が残った")
			}
		})
	}
}

// 期限内の遅着は人の意図を優先し、期限後は残っている本体の期待を適用する。
func TestHumanSceneIntentLateMatching(t *testing.T) {
	for _, seconds := range []int{3, 8} {
		m := New(Config{})
		at := time.Unix(100, 0)
		m.st.Scene = "a"
		m.st.Expectations = []Expectation{{Scene: "u", Purpose: PurposeStandbyOn, ReturnScene: "a"}}
		m.Handle(HumanSceneSelected{ID: 1, At: at, SceneUUID: "u", Deadline: at.Add(7 * time.Second)})
		m.Handle(SceneChanged{At: at.Add(time.Duration(seconds) * time.Second), SceneUUID: "u"})
		want := OwnerNone
		if seconds == 8 {
			want = OwnerManual
		}
		if m.st.StandbyOwner != want {
			t.Fatalf("%d秒の遅着: %v", seconds, m.st.StandbyOwner)
		}
	}
}

func TestHumanSceneConcurrentRequests(t *testing.T) {
	for _, mode := range []string{"取消", "同じシーン", "同じUUID", "切断"} {
		t.Run(mode, func(t *testing.T) {
			m := New(Config{})
			at := time.Unix(100, 0)
			m.st.Scene = "a"
			m.st.Expectations = []Expectation{{Scene: "u", Purpose: PurposeStandbyOn, ReturnScene: "a"}}
			m.Handle(HumanSceneSelected{ID: 1, At: at, SceneUUID: "u", Deadline: at.Add(7 * time.Second)})
			target := "v"
			if mode == "同じシーン" {
				target = "a"
			}
			if mode == "同じUUID" {
				target = "u"
			}
			m.Handle(HumanSceneSelected{ID: 2, At: at, SceneUUID: target, Deadline: at.Add(10 * time.Second)})
			if mode == "取消" {
				m.Handle(HumanSceneSelected{ID: 2, At: at, Cancel: true})
			}
			if mode == "切断" {
				m.Handle(OBSDisconnected{At: at})
			}
			m.Handle(SceneChanged{At: at.Add(3 * time.Second), SceneUUID: "u"})
			if m.st.StandbyOwner != OwnerNone || len(m.st.Expectations) != 0 {
				t.Fatalf("先の遅着を人の操作として扱わない: %+v", m.st)
			}
			if mode == "同じUUID" && len(m.st.HumanIntents) != 0 {
				t.Fatal("同じUUIDの意図が残る")
			}
		})
	}
}

func TestHumanSceneApproveAndResume(t *testing.T) {
	for _, recover := range []bool{false, true} {
		t.Run(map[bool]string{false: "AutoFail", true: "AutoRecover"}[recover], func(t *testing.T) {
			q := newSeq(t)
			id, purpose, target, ret, health := 1, af, standby, camera, LOW
			if recover {
				q.PA(camera)
				id, purpose, target, ret, health = 2, ar, camera, "", OK
			} else {
				q.P(camera, true, OK)
			}
			q.batch(lc(1000, health), id, purpose, target, ret)
			intent := HumanSceneSelected{ID: 99, At: at(1100), SceneUUID: talk, Deadline: at(8100)}
			q.none(intent)
			q.approve(id, set(target), 1200, Reject{})
			q.none(bd(1200, id, BatchRejected))
			// 明確な失敗だけを表す取消のあと、同じ自動判断が戻る。
			intent.Cancel = true
			intent.At = at(1300)
			q.m.Handle(intent)
			bs, err := q.m.Handle(tick(7000))
			if err != nil {
				t.Fatal(err)
			}
			_ = bs
			d, ok := q.m.decide()
			if !ok || d.Purpose != purpose || d.Target != target {
				t.Fatalf("自動判断が戻らない: %+v %v", d, ok)
			}
			found := false
			for _, pb := range q.m.st.Pending {
				if pb.Purpose == purpose {
					found = true
				}
			}
			if !found {
				t.Fatal("自動バッチが再生成されない")
			}
		})
	}
}

func TestHumanSceneHiddenItems(t *testing.T) {
	h := newHB(t)
	h.hiddenBoth(1000)
	h.m.Handle(HumanSceneSelected{ID: 1, At: at(2000), SceneUUID: talk, Deadline: at(9000)})
	h.m.Handle(SceneChanged{At: at(2100), SceneUUID: talk})
	h.wantEntries("x:H,y:H")
	h.wantMap(kx, false)
	h.wantMap(ky, false)
	bs := h.sIs(7000, OK)
	h.out(bs, "Sc(s)[R(is),x,y]")
	h.trueAll(bs[0], 7100, kx, ky)
	h.wantEntries("")
}

func TestHumanSceneUUIDDeadline(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		m := New(Config{})
		now := time.Unix(100, 0)
		m.st.Scene = "a"
		m.st.Expectations = []Expectation{{Scene: "u", Purpose: PurposeStandbyOn, ReturnScene: "a"}}
		m.Handle(HumanSceneSelected{ID: 1, At: now, SceneUUID: "u", Deadline: now.Add(7 * time.Second)})
		m.Handle(HumanSceneSelected{ID: 2, At: now, SceneUUID: "u", Deadline: now.Add(10 * time.Second)})
		if cancel {
			m.Handle(HumanSceneSelected{ID: 2, At: now, Cancel: true})
		}
		m.Handle(SceneChanged{At: now.Add(8 * time.Second), SceneUUID: "u"})
		want := OwnerNone
		if cancel {
			want = OwnerManual
		}
		if m.st.StandbyOwner != want {
			t.Fatalf("cancel=%v owner=%v", cancel, m.st.StandbyOwner)
		}
	}
}
