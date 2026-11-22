package obs

import (
	"time"

	"errors"
	"fmt"
	"github.com/andreykaipov/goobs/api/events"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

func TestOutputState(t *testing.T) {
	cases := []struct {
		state     string
		emit      bool
		streaming bool
	}{
		{"STARTED", true, true},
		{"STOPPED", true, false},
		{"STARTING", false, false},
		{"STOPPING", false, false},
		{"RECONNECTING", false, false},
		{"RECONNECTED", false, false},
		{"PAUSED", false, false},
		{"RESUMED", false, false},
		{"", false, false},
		{"SOMETHING_NEW", false, false},
		// obs-websocket が実際に送る接頭辞つきの値
		{"OBS_WEBSOCKET_OUTPUT_STARTED", true, true},
		{"OBS_WEBSOCKET_OUTPUT_STOPPED", true, false},
		{"OBS_WEBSOCKET_OUTPUT_STARTING", false, false},
		{"OBS_WEBSOCKET_OUTPUT_STOPPING", false, false},
		{"OBS_WEBSOCKET_OUTPUT_RECONNECTING", false, false},
		{"OBS_WEBSOCKET_OUTPUT_RECONNECTED", false, false},
		{"OBS_WEBSOCKET_OUTPUT_PAUSED", false, false},
		{"OBS_WEBSOCKET_OUTPUT_RESUMED", false, false},
		{"OBS_WEBSOCKET_OUTPUT_UNKNOWN", false, false},
	}
	for _, c := range cases {
		emit, streaming := outputState(c.state)
		if emit != c.emit || (emit && streaming != c.streaming) {
			t.Errorf("outputState(%q) = (%v,%v), want (%v,%v)", c.state, emit, streaming, c.emit, c.streaming)
		}
	}
}

func TestStreamingFromStatus(t *testing.T) {
	for _, c := range []struct{ active, reconnecting, want bool }{
		{false, false, false}, {true, false, true}, {false, true, true}, {true, true, true},
	} {
		if got := streamingFromStatus(c.active, c.reconnecting); got != c.want {
			t.Errorf("streamingFromStatus(%v,%v) = %v", c.active, c.reconnecting, got)
		}
	}
}

func TestOverrideFromResponse(t *testing.T) {
	ms, has := overrideFromResponse("", 0)
	if has || ms != nil {
		t.Errorf("名前なしは上書きなし: %v %v", ms, has)
	}
	ms, has = overrideFromResponse("Fade", 300)
	if !has || ms == nil || *ms != 300 {
		t.Errorf("長さありは Known: %v %v", ms, has)
	}
	// 長さが null（goobs では 0 と区別できない）は長さ不明
	ms, has = overrideFromResponse("Fade", 0)
	if !has || ms != nil {
		t.Errorf("長さなしは Unknown: %v %v", ms, has)
	}
}

func TestToOverride(t *testing.T) {
	v := 250
	if _, ok := ToOverride(nil, false, nil).(switcher.OverrideNone); !ok {
		t.Error("none")
	}
	if k, ok := ToOverride(&v, true, nil).(switcher.OverrideKnown); !ok || k.Ms != 250 {
		t.Error("known")
	}
	if _, ok := ToOverride(nil, true, nil).(switcher.OverrideUnknown); !ok {
		t.Error("unknown (duration null)")
	}
	if _, ok := ToOverride(&v, true, errors.New("x")).(switcher.OverrideUnknown); !ok {
		t.Error("unknown (error)")
	}
	if _, ok := ToOverride(nil, false, ErrTimeout).(switcher.OverrideUnknown); !ok {
		t.Error("unknown (timeout)")
	}
}

func TestClassifyError(t *testing.T) {
	if classifyError(nil) != nil {
		t.Error("nil はそのまま")
	}
	// goobs（api.Client.SendRequest）が応答待ちの超過で返す文
	to := fmt.Errorf("request %s: timeout waiting for response from server", "SetCurrentProgramScene")
	if got := classifyError(to); !errors.Is(got, ErrTimeout) {
		t.Errorf("タイムアウトは ErrTimeout: %v", got)
	}
	other := fmt.Errorf("request %s: %s (%d)", "SetCurrentProgramScene", "NotFound", 600)
	if got := classifyError(other); errors.Is(got, ErrTimeout) || got == nil {
		t.Errorf("他のエラーは ErrTimeout にしない: %v", got)
	}
	if got := classifyError(fmt.Errorf("request X: client already disconnected")); errors.Is(got, ErrTimeout) {
		t.Error("切断は ErrTimeout にしない")
	}
}

func TestTransitionMs(t *testing.T) {
	if transitionMs(true, 300) != nil {
		t.Error("固定長は nil")
	}
	if p := transitionMs(false, 300); p == nil || *p != 300 {
		t.Errorf("300: %v", p)
	}
	if p := transitionMs(false, 0); p == nil || *p != 0 {
		t.Errorf("0: %v", p)
	}
	if transitionMs(false, -1) != nil {
		t.Error("負は nil")
	}
}

func TestMapEvent(t *testing.T) {
	at := time.Unix(100, 0)
	if e, ok := mapEvent(&events.CurrentProgramSceneChanged{SceneName: "Live"}, at); !ok || e != (switcher.SceneChanged{At: at, Scene: "Live"}) {
		t.Errorf("scene: %v %v", e, ok)
	}
	e, ok := mapEvent(&events.CurrentSceneTransitionDurationChanged{TransitionDuration: 500}, at)
	tc, isTC := e.(switcher.TransitionChanged)
	if !ok || !isTC || tc.At != at || tc.TransitionMs == nil || *tc.TransitionMs != 500 {
		t.Errorf("duration: %v %v", e, ok)
	}
	if e, ok := mapEvent(&events.StreamStateChanged{OutputState: "OBS_WEBSOCKET_OUTPUT_STOPPED"}, at); !ok || e != (switcher.StreamStateChanged{At: at, Streaming: false}) {
		t.Errorf("stopped: %v %v", e, ok)
	}
	if _, ok := mapEvent(&events.StreamStateChanged{OutputState: "OBS_WEBSOCKET_OUTPUT_RECONNECTING"}, at); ok {
		t.Error("RECONNECTING は流さない")
	}
	if _, ok := mapEvent(&events.SceneCreated{}, at); ok {
		t.Error("関係のないイベントは流さない")
	}
}

// 接続が切れたことによる失敗は、切断のイベントより先に返ることがある。適用されたかわからないので、
// 文から Timeout に分ける（送れなかった "client already disconnected" は分けない）。
func TestConnectionLostIsTimeout(t *testing.T) {
	for _, msg := range []string{
		"write tcp 127.0.0.1:1->127.0.0.1:4455: use of closed network connection",
		"write tcp 127.0.0.1:1->127.0.0.1:4455: write: broken pipe",
		"read tcp 127.0.0.1:1->127.0.0.1:4455: read: connection reset by peer",
		"websocket: close sent",
		"websocket: close 1006 (abnormal closure): unexpected EOF",
		"EOF",
	} {
		err := fmt.Errorf("request SetSceneItemEnabled: %s", msg)
		if got := classifyError(err); !errors.Is(got, ErrTimeout) {
			t.Errorf("classifyError(%q) = %v, want ErrTimeout", msg, got)
		}
		if got := ClassifyItemResult(err); got != switcher.ItemTimeout {
			t.Errorf("ClassifyItemResult(%q) = %v, want Timeout", msg, got)
		}
	}
	notSent := fmt.Errorf("request X: client already disconnected")
	if got := ClassifyItemResult(notSent); got != switcher.ItemError {
		t.Errorf("送れなかった失敗は Error のまま: %v", got)
	}
}
