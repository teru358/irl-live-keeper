package switcher

import (
	"testing"
	"time"
)

func TestHumanSceneClearsMatchingExpectation(t *testing.T) {
	m := &Machine{}
	m.st.Expectations = []Expectation{{Scene: "u", Purpose: PurposeAutoFail}}
	m.st.StandbyOwner = OwnerAuto
	_, err := m.Handle(HumanSceneSelected{At: time.Now(), SceneUUID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.st.Expectations) != 0 || m.st.StandbyOwner != OwnerNone || len(m.st.Stack) != 0 {
		t.Fatalf("人の操作を期待と取り違えた: %+v", m.st)
	}
}
