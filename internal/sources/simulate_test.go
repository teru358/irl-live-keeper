package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"reflect"
	"testing"
	"time"
)

func TestSimulation(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1", Protocol: "rtmp", Online: true}, {ID: "cam2"}}, OBS: core.OBSStatus{Scene: "Live", SceneClass: "monitored"}}
	before := s
	now := time.Now()
	for _, tc := range []struct {
		mode     string
		kbps     float64
		online   bool
		protocol string
	}{{"OK", 4200, true, "rtmp"}, {"LOW", 600, true, "rtmp"}, {"DOWN", 0, false, ""}, {"standby", 600, true, "rtmp"}} {
		t.Run(tc.mode, func(t *testing.T) {
			v, err := (Simulation{Mode: tc.mode}).Apply(s, now, nil)
			if err != nil {
				t.Fatal(err)
			}
			if v["cam1.kbps"].Number != tc.kbps || v["cam1.online"].Bool != tc.online || v["cam1.protocol"].String != tc.protocol || v["cam1.protocol"].Defined != (tc.protocol != "") || v["cam1.srt.rtt"].Defined {
				t.Fatal(v)
			}
			if tc.mode != "DOWN" && !v["cam2.srt.rtt"].Defined {
				t.Fatal("offline camera must use srt")
			}
			w, _ := (Simulation{Mode: tc.mode}).Apply(s, now.Add(time.Second), nil)
			if w["stream.elapsed"] != v["stream.elapsed"] || w["now"] == v["now"] {
				t.Fatal("clock")
			}
			if tc.mode == "standby" && (v["standby.return"].String != "Live" || v["standby.for"].Number != 12) {
				t.Fatal(v)
			}
		})
	}
	v, err := (Simulation{Mode: "custom", Values: map[string]any{"cam1.kbps": float64(123), "obs.scene": nil}}).Apply(s, now, nil)
	if err != nil || v["cam1.kbps"].Number != 123 || v["obs.scene"].Defined {
		t.Fatal(v, err)
	}
	v, _ = (Simulation{Mode: "offline"}).Apply(s, now, nil)
	if v["obs.streaming"].Bool || v["stream.elapsed"].Defined || v["standby.return"].Defined {
		t.Fatal(v)
	}
	if !reflect.DeepEqual(s, before) {
		t.Fatal("snapshot changed")
	}
	for _, sim := range []Simulation{{Mode: "bad"}, {Mode: "custom", Values: map[string]any{"secret": true}}, {Mode: "custom", Values: map[string]any{"cam1.kbps": "bad"}}} {
		if _, err := sim.Apply(s, now, nil); err == nil {
			t.Fatal(sim)
		}
	}
}

func TestSimulationTable(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1"}}, SceneDetails: map[string]obs.Scene{"a": {Name: "Live", Kind: "camera"}, "b": {Name: "Wait", Kind: "standby"}}}
	for _, mode := range []string{"OK", "LOW", "DOWN", "standby", "offline"} {
		t.Run(mode, func(t *testing.T) {
			v, err := (Simulation{Mode: mode}).Apply(s, time.Now(), nil)
			if err != nil {
				t.Fatal(err)
			}
			check := func(n string, want any) {
				t.Helper()
				got := v[n]
				if want == nil {
					if got.Defined {
						t.Errorf("%s defined", n)
					}
					return
				}
				if !got.Defined {
					t.Errorf("%s undefined", n)
					return
				}
				var actual any
				switch got.Type {
				case template.String:
					actual = got.String
				case template.Bool:
					actual = got.Bool
				default:
					actual = got.Number
				}
				if actual != want {
					t.Errorf("%s = %v want %v", n, actual, want)
				}
			}
			for n, want := range map[string]any{"obs.connected": true, "obs.fps": float64(60), "obs.render_skipped": float64(0), "mediamtx.state": "running"} {
				check(n, want)
			}
			if mode == "offline" {
				for _, n := range []string{"stream.elapsed", "stream.kbps", "stream.dropped", "stream.congestion", "standby.return", "standby.for"} {
					check(n, nil)
				}
				check("obs.streaming", false)
				return
			}
			for n, want := range map[string]any{"obs.streaming": true, "cam1.state_for": float64(12), "stream.elapsed": float64(5025), "stream.kbps": float64(6000), "stream.dropped": float64(0), "stream.congestion": float64(0)} {
				check(n, want)
			}
			if mode == "standby" {
				check("standby.owner", "auto")
				check("standby.return", "Live")
				check("standby.for", float64(12))
				check("obs.scene", "Wait")
				check("obs.scene_class", "standby")
			} else {
				check("standby.owner", "none")
				check("standby.return", nil)
				check("standby.for", nil)
				check("obs.scene", "Live")
				check("obs.scene_class", "monitored")
			}
			// session.* は区切り3の保留のあいだ一覧にない（task 表 v1.14）
			if _, ok := v["session.low_count"]; ok {
				t.Fatal("session.low_count が保留なのに値にある")
			}
			if mode == "DOWN" {
				check("cam1.protocol", nil)
				for _, n := range []string{"rtt", "loss", "mbps"} {
					check("cam1.srt."+n, nil)
				}
			} else if mode == "OK" {
				check("cam1.protocol", "srt")
				check("cam1.srt.rtt", float64(80))
				check("cam1.srt.loss", float64(0))
				check("cam1.srt.mbps", 4.2)
			} else {
				check("cam1.protocol", "srt")
				check("cam1.srt.rtt", float64(250))
				check("cam1.srt.loss", 0.05)
				check("cam1.srt.mbps", 0.6)
			}
		})
	}
}

func TestSimulationRejectsOldCameraNames(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1", Kbps: 42}}}
	for _, name := range []string{"camera.kbps", "camera.cam1.kbps"} {
		if _, err := (Simulation{Mode: "custom", Values: map[string]any{name: float64(123)}}).Apply(s, time.Now(), nil); err == nil {
			t.Fatal(name)
		}
	}
	v, err := (Simulation{Mode: "custom", Values: map[string]any{"cam1.kbps": float64(123)}}).Apply(s, time.Now(), nil)
	if err != nil || v["cam1.kbps"].Number != 123 {
		t.Fatal(v, err)
	}
}
