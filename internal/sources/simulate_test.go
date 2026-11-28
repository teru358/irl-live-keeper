package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"reflect"
	"strings"
	"testing"
	"time"
)

// 実際の値が、どの軸の差し替えとも違う値になるスナップショット。
func simSnapshot(connected bool, sceneClass string, details bool) core.Snapshot {
	rate := core.SRTStatus{RTTMs: 33, LossRate: 0.5, RecvMbps: 9}
	s := core.Snapshot{
		Cameras: []core.CameraStatus{
			{ID: "cam1", Name: "正面", Hidden: 2, State: "OK", Kbps: 3000, Online: true, Protocol: "srt", SRT: &rate, StateSince: time.Unix(100, 0)},
			{ID: "cam2", Name: "後方", State: "DOWN", Kbps: 0, Online: false},
			{ID: "cam3", Name: "横", State: "OK", Kbps: 1000, Online: true, Protocol: "rtmp", StateSince: time.Unix(100, 0)},
		},
		MediaMTX: "stopped",
		OBS: core.OBSStatus{Connected: connected, Streaming: true, Scene: "Real", SceneClass: sceneClass, SceneCameras: nil,
			Standby: core.StandbyStatus{Owner: "manual"}},
	}
	if sceneClass == "monitored" {
		s.OBS.SceneCameras = []string{"cam1"}
		h := "OK"
		s.OBS.SceneHealth = &h
	}
	if details {
		s.SceneDetails = map[string]obs.Scene{"a": {Name: "Live", Kind: "camera", Cameras: []string{"cam1"}}, "b": {Name: "Wait", Kind: "standby"}, "c": {Name: "Other", Kind: "none"}}
	}
	return s
}

type want struct {
	defined bool
	str     string
	num     float64
	boolean bool
}

func str(x string) want  { return want{defined: true, str: x} }
func num(x float64) want { return want{defined: true, num: x} }
func yesno(x bool) want  { return want{defined: true, boolean: x} }
func undef() want        { return want{} }
func valueOf(v template.Value) want {
	if !v.Defined {
		return undef()
	}
	return want{defined: true, str: v.String, num: v.Number, boolean: v.Bool}
}

// expectedAxis は仕様の表（第9章「軸ごとの値」）を、実装とは別に書いたもの。
// 戻りの2つ目が false なら、その値は実際のまま。
func expectedAxis(s core.Snapshot, line, standby, stream, name string) (want, bool) {
	anyAxis := line != "actual" || standby != "actual" || stream != "actual"
	monitored := ""
	if s.SceneDetails != nil {
		for _, sc := range s.SceneDetails {
			if sc.Kind == "camera" {
				monitored = sc.Name
			}
		}
	} else if s.OBS.SceneClass == "monitored" {
		monitored = s.OBS.Scene
	}
	standbyScene := ""
	if s.SceneDetails != nil {
		standbyScene = "Wait"
	}
	optional := func(x string) want {
		if x == "" {
			return undef()
		}
		return str(x)
	}
	switch name {
	case "obs.connected":
		return yesno(true), anyAxis
	case "obs.fps":
		return num(60), anyAxis
	case "obs.render_skipped":
		return num(0), anyAxis
	case "mediamtx.state":
		return str("running"), anyAxis
	case "obs.scene_health":
		if line == "actual" {
			return want{}, false
		}
		shows := s.OBS.Connected && s.OBS.SceneClass == "monitored"
		switch standby {
		case "none":
			shows = monitored != ""
		case "auto":
			shows = false
		}
		if shows {
			return str(line), true
		}
		return undef(), true
	}
	if strings.HasPrefix(name, "cam") {
		field := name[strings.Index(name, ".")+1:]
		if line == "actual" || field == "name" || field == "hidden" {
			return want{}, false
		}
		id := name[:strings.Index(name, ".")]
		var cam core.CameraStatus
		for _, c := range s.Cameras {
			if c.ID == id {
				cam = c
			}
		}
		p := "srt"
		if cam.Online && cam.Protocol != "" {
			p = cam.Protocol
		}
		kbps := map[string]float64{"OK": 4200, "LOW": 600, "DOWN": 0}[line]
		srt := map[string][3]float64{"OK": {80, 0, 4.2}, "LOW": {250, 0.05, 0.6}}[line]
		switch field {
		case "state":
			return str(line), true
		case "kbps":
			return num(kbps), true
		case "online":
			return yesno(line != "DOWN"), true
		case "protocol":
			if line == "DOWN" {
				return undef(), true
			}
			return str(p), true
		case "state_for":
			return num(12), true
		case "srt.rtt", "srt.loss", "srt.mbps":
			if line == "DOWN" || p != "srt" {
				return undef(), true
			}
			return num(srt[map[string]int{"srt.rtt": 0, "srt.loss": 1, "srt.mbps": 2}[field]]), true
		}
	}
	switch name {
	case "standby.owner":
		return str(map[string]string{"none": "none", "auto": "auto"}[standby]), standby != "actual"
	case "standby.return":
		if standby == "auto" {
			return optional(monitored), true
		}
		return undef(), standby == "none"
	case "standby.for":
		if standby == "auto" {
			return num(12), true
		}
		return undef(), standby == "none"
	case "obs.scene":
		if standby == "auto" {
			return optional(standbyScene), true
		}
		return optional(monitored), standby == "none"
	case "obs.scene_class":
		if standby == "auto" {
			return str("standby"), true
		}
		if monitored == "" {
			return undef(), standby == "none"
		}
		return str("monitored"), standby == "none"
	case "obs.streaming":
		return yesno(stream == "streaming"), stream != "actual"
	case "stream.elapsed":
		return num(5025), stream == "streaming"
	case "stream.kbps":
		return num(6000), stream == "streaming"
	case "stream.dropped", "stream.congestion":
		return num(0), stream == "streaming"
	}
	if strings.HasPrefix(name, "stream.") {
		return undef(), stream == "offline"
	}
	return want{}, false
}

func TestSimulationAxisCombinations(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, sc := range []struct {
		name string
		s    core.Snapshot
	}{
		{"monitored", simSnapshot(true, "monitored", true)},
		{"disconnected", simSnapshot(false, "", false)},
		{"on-standby", simSnapshot(true, "standby", true)},
		{"monitored-without-map", simSnapshot(true, "monitored", false)},
		{"no-monitored-scene", simSnapshot(true, "other", true)},
	} {
		s := sc.s
		if sc.name == "no-monitored-scene" {
			s.SceneDetails = map[string]obs.Scene{"b": {Name: "Wait", Kind: "standby"}}
		}
		before := s
		actual := Values(s, now, nil)
		for _, line := range []string{"actual", "OK", "LOW", "DOWN"} {
			for _, standby := range []string{"actual", "none", "auto"} {
				for _, stream := range []string{"actual", "streaming", "offline"} {
					sim := Simulation{Line: line, Standby: standby, Stream: stream}
					v, err := sim.Apply(s, now, nil)
					if err != nil {
						t.Fatal(err)
					}
					for n := range Definitions(s) {
						if n == "now" {
							if v[n] != actual[n] {
								t.Errorf("%s/%v: now changed", sc.name, sim)
							}
							continue
						}
						exp, simulated := expectedAxis(s, line, standby, stream, n)
						if !simulated {
							exp = valueOf(actual[n])
						}
						if got := valueOf(v[n]); got != exp {
							t.Errorf("%s/%s/%s/%s %s = %+v want %+v (simulated=%v)", sc.name, line, standby, stream, n, got, exp, simulated)
						}
						if v[n].Type != actual[n].Type {
							t.Errorf("%s type changed", n)
						}
					}
				}
			}
		}
		if !reflect.DeepEqual(s, before) {
			t.Fatal("snapshot changed")
		}
	}
}

func TestSimulationSceneHealthEdges(t *testing.T) {
	now := time.Unix(1000, 0)
	health := func(s core.Snapshot, sim Simulation) want {
		t.Helper()
		v, err := sim.Apply(s, now, nil)
		if err != nil {
			t.Fatal(err)
		}
		return valueOf(v["obs.scene_health"])
	}
	// 回線を変えて、シーンにカメラが映るなら状態になる。
	if h := health(simSnapshot(true, "monitored", true), Simulation{Line: "LOW"}); h != str("LOW") {
		t.Error(h)
	}
	// 待機のシーンはカメラが映らないので未定義。
	if h := health(simSnapshot(true, "monitored", true), Simulation{Line: "OK", Standby: "auto"}); h != undef() {
		t.Error(h)
	}
	// 実際の OBS が未接続なら、回線だけの差し替えで scene は実際（未定義）のまま、scene_health も未定義。
	disconnected := simSnapshot(false, "", false)
	v, _ := (Simulation{Line: "OK"}).Apply(disconnected, now, nil)
	if !v["obs.connected"].Bool || v["obs.scene"].Defined || v["obs.scene_health"].Defined {
		t.Error("line only on disconnected OBS", v["obs.connected"], v["obs.scene"], v["obs.scene_health"])
	}
	// 監視対象のシーンがなければ、なしの obs.scene は未定義、自動の standby.return も未定義。
	none := simSnapshot(true, "other", true)
	none.SceneDetails = map[string]obs.Scene{"b": {Name: "Wait", Kind: "standby"}}
	v, _ = (Simulation{Line: "DOWN", Standby: "none"}).Apply(none, now, nil)
	if v["obs.scene"].Defined || v["obs.scene_class"].Defined || v["obs.scene_health"].Defined || v["standby.owner"].String != "none" {
		t.Error("no monitored scene / none")
	}
	v, _ = (Simulation{Standby: "auto"}).Apply(none, now, nil)
	if v["standby.return"].Defined || v["obs.scene"].String != "Wait" || v["standby.for"].Number != 12 {
		t.Error("no monitored scene / auto")
	}
	// DOWN の protocol は未定義、DOWN 以外は受信していなければ srt。
	v, _ = (Simulation{Line: "DOWN"}).Apply(simSnapshot(true, "monitored", true), now, nil)
	if v["cam1.protocol"].Defined || v["cam3.protocol"].Defined || v["cam1.srt.rtt"].Defined {
		t.Error("DOWN protocol")
	}
	v, _ = (Simulation{Line: "OK"}).Apply(simSnapshot(true, "monitored", true), now, nil)
	if v["cam2.protocol"].String != "srt" || !v["cam2.srt.rtt"].Defined || v["cam3.protocol"].String != "rtmp" || v["cam3.srt.rtt"].Defined {
		t.Error("P rule")
	}
}

func TestSimulationFreezesSimulatedValues(t *testing.T) {
	s := simSnapshot(true, "monitored", true)
	now := time.Unix(1000, 0)
	a, _ := (Simulation{Line: "OK", Standby: "auto", Stream: "streaming"}).Apply(s, now, nil)
	b, _ := (Simulation{Line: "OK", Standby: "auto", Stream: "streaming"}).Apply(s, now.Add(time.Minute), nil)
	for _, n := range []string{"cam1.state_for", "standby.for", "stream.elapsed"} {
		if a[n] != b[n] {
			t.Errorf("%s moved", n)
		}
	}
	if a["now"] == b["now"] {
		t.Error("now must advance")
	}
	// 実際のままの軸は進む（cam1.state_for は実際の経過）。
	c, _ := (Simulation{Stream: "streaming"}).Apply(s, now, nil)
	d, _ := (Simulation{Stream: "streaming"}).Apply(s, now.Add(time.Minute), nil)
	if d["cam1.state_for"].Number != c["cam1.state_for"].Number+60 {
		t.Error("actual value must advance")
	}
}

func TestSimulationFreeValuesLayered(t *testing.T) {
	s := simSnapshot(true, "monitored", true)
	now := time.Unix(1000, 0)
	// 軸がすべて実際なら、実際の値の上に重ねる。
	v, err := (Simulation{Values: map[string]any{"cam1.kbps": float64(123), "obs.scene": nil}}).Apply(s, now, nil)
	if err != nil || v["cam1.kbps"].Number != 123 || v["obs.scene"].Defined || v["cam2.state"].String != "DOWN" || !v["obs.connected"].Bool {
		t.Fatal(v, err)
	}
	d, _ := (Simulation{}).Apply(simSnapshot(false, "", false), now, nil)
	if d["obs.connected"].Bool || d["obs.fps"].Defined {
		t.Fatal("all actual must keep actual connected / fps")
	}
	d, _ = (Simulation{Values: map[string]any{"cam1.kbps": float64(1)}}).Apply(simSnapshot(false, "", false), now, nil)
	if d["obs.connected"].Bool {
		t.Fatal("free values alone must not touch connected")
	}
	// 軸の値の上に重ねる（軸の値より自由が勝つ）。
	v, err = (Simulation{Line: "DOWN", Stream: "offline", Values: map[string]any{"cam1.kbps": float64(777), "cam2.state": "LOW", "stream.kbps": float64(5)}}).Apply(s, now, nil)
	if err != nil || v["cam1.kbps"].Number != 777 || v["cam2.state"].String != "LOW" || v["cam2.kbps"].Number != 0 || v["cam1.state"].String != "DOWN" || v["stream.kbps"].Number != 5 || v["stream.elapsed"].Defined {
		t.Fatal(v, err)
	}
}

func TestSimulationRejectsInvalid(t *testing.T) {
	s := simSnapshot(true, "monitored", true)
	for _, sim := range []Simulation{
		{Line: "bad"}, {Line: "ok"}, {Line: "none"}, {Standby: "OK"}, {Standby: "standby"}, {Stream: "online"}, {Stream: "Streaming"},
		{Values: map[string]any{"secret": true}}, {Values: map[string]any{"cam1.kbps": "bad"}}, {Values: map[string]any{"now": 1.0}},
		{Line: "OK", Values: map[string]any{"obs.streaming": 1.0}},
		{Values: map[string]any{"camera.kbps": float64(1)}}, {Values: map[string]any{"camera.cam1.kbps": float64(1)}},
	} {
		if _, err := sim.Apply(s, time.Now(), nil); err == nil {
			t.Errorf("accepted %+v", sim)
		}
	}
	for _, sim := range []Simulation{{}, {Line: "actual", Standby: "actual", Stream: "actual"}} {
		if !sim.IsActual() {
			t.Errorf("%+v must be actual", sim)
		}
	}
	for _, sim := range []Simulation{{Line: "OK"}, {Standby: "none"}, {Stream: "offline"}, {Values: map[string]any{"cam1.kbps": 1.0}}} {
		if sim.IsActual() {
			t.Errorf("%+v must not be actual", sim)
		}
	}
	if (Simulation{Values: map[string]any{}}).IsActual() != true {
		t.Error("empty values")
	}
}

func TestSimulationSameChoices(t *testing.T) {
	a := Simulation{Line: "OK", Values: map[string]any{"cam1.kbps": 1.0}}
	if !a.SameChoices(Simulation{Line: "OK", Standby: "actual", Values: map[string]any{"cam1.kbps": 2.0}}) {
		t.Error("values must not count")
	}
	if a.SameChoices(Simulation{Line: "OK"}) || a.SameChoices(Simulation{Line: "LOW", Values: a.Values}) {
		t.Error("choices must count")
	}
	for _, f := range (Simulation{Values: map[string]any{"obs.scene": "secret-value"}}).LogFields() {
		if f == "secret-value" {
			t.Error("values in log")
		}
	}
}
