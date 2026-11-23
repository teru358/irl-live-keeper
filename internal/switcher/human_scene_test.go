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
			if len(m.st.Expectations) != 0 || m.st.StandbyOwner != OwnerNone || len(m.st.Stack) != 0 || m.st.HumanIntent != nil {
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
				if m.st.HumanIntent == nil {
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
			if m.st.HumanIntent != nil {
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
