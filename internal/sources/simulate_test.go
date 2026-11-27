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

func TestSimulation(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "a", Protocol: "rtmp", Online: true}, {ID: "b"}}, OBS: core.OBSStatus{Scene: "Live", SceneClass: "monitored"}}
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
			if v["camera.a.kbps"].Number != tc.kbps || v["camera.a.online"].Bool != tc.online || v["camera.a.protocol"].String != tc.protocol || v["camera.a.protocol"].Defined != (tc.protocol != "") || v["camera.a.srt.rtt"].Defined {
				t.Fatal(v)
			}
			if tc.mode != "DOWN" && !v["camera.b.srt.rtt"].Defined {
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
	v, err := (Simulation{Mode: "custom", Values: map[string]any{"camera.kbps": float64(123), "obs.scene": nil}}).Apply(s, now, nil)
	if err != nil || v["camera.kbps"].Number != 123 || v["obs.scene"].Defined {
		t.Fatal(v, err)
	}
	v, _ = (Simulation{Mode: "offline"}).Apply(s, now, nil)
	if v["obs.streaming"].Bool || v["stream.elapsed"].Defined || v["standby.return"].Defined {
		t.Fatal(v)
	}
	if !reflect.DeepEqual(s, before) {
		t.Fatal("snapshot changed")
	}
	for _, sim := range []Simulation{{Mode: "bad"}, {Mode: "custom", Values: map[string]any{"secret": true}}, {Mode: "custom", Values: map[string]any{"camera.kbps": "bad"}}} {
		if _, err := sim.Apply(s, now, nil); err == nil {
			t.Fatal(sim)
		}
	}
}

func TestSimulationTable(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "a"}}, SceneDetails: map[string]obs.Scene{"a": {Name: "Live", Kind: "camera"}, "b": {Name: "Wait", Kind: "standby"}}}
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
			for n, want := range map[string]any{"obs.streaming": true, "camera.a.state_for": float64(12), "stream.elapsed": float64(5025), "stream.kbps": float64(6000), "stream.dropped": float64(0), "stream.congestion": float64(0)} {
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
				check("camera.a.protocol", nil)
				for _, n := range []string{"rtt", "loss", "mbps"} {
					check("camera.a.srt."+n, nil)
				}
			} else if mode == "OK" {
				check("camera.a.protocol", "srt")
				check("camera.a.srt.rtt", float64(80))
				check("camera.a.srt.loss", float64(0))
				check("camera.a.srt.mbps", 4.2)
			} else {
				check("camera.a.protocol", "srt")
				check("camera.a.srt.rtt", float64(250))
				check("camera.a.srt.loss", 0.05)
				check("camera.a.srt.mbps", 0.6)
			}
		})
	}
}

func TestSimulationCurrentCamera(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	fields := []string{"state", "kbps", "online", "protocol", "state_for", "srt.rtt", "srt.loss", "srt.mbps", "name", "hidden"}
	for _, protocol := range []string{"srt", "rtmp"} {
		for _, mode := range []string{"OK", "LOW", "DOWN", "standby"} {
			for _, tc := range []struct {
				name string
				ids  []string
			}{
				{"zero", nil}, {"one", []string{"b"}}, {"two", []string{"a", "b"}},
			} {
				t.Run(protocol+"/"+mode+"/"+tc.name, func(t *testing.T) {
					s := core.Snapshot{
						Cameras: []core.CameraStatus{{ID: "a"}, {ID: "b", Name: "後方", Protocol: protocol, Online: true, Hidden: 3}},
						// Actual scene points to a different camera. Presets must use SceneDetails instead.
						OBS: core.OBSStatus{Connected: true, Scene: "Z actual", SceneCameras: []string{"a"}},
						SceneDetails: map[string]obs.Scene{
							"z": {Name: "Z live", Kind: "camera", Cameras: []string{"a"}},
							"a": {Name: "A live", Kind: "camera", Cameras: tc.ids},
							"w": {Name: "Wait", Kind: "standby", Cameras: tc.ids},
						},
					}
					v, err := (Simulation{Mode: mode}).Apply(s, now, nil)
					if err != nil {
						t.Fatal(err)
					}
					wantScene := "A live"
					if mode == "standby" {
						wantScene = "Wait"
					}
					if v["obs.scene"].String != wantScene {
						t.Errorf("scene = %+v, want %s", v["obs.scene"], wantScene)
					}
					for _, field := range append(fields, "id") {
						got, ok := v["camera."+field]
						if !ok {
							t.Errorf("missing camera.%s", field)
							continue
						}
						if len(tc.ids) != 1 {
							if got.Defined {
								t.Errorf("camera.%s must be undefined: %+v", field, got)
							}
							continue
						}
						want := v["camera.b."+field]
						if field == "id" {
							want = template.Value{Defined: true, Type: template.String, String: "b"}
						}
						if got != want {
							t.Errorf("camera.%s = %+v, want %+v", field, got, want)
						}
					}
					// All cameras still get the preset, including those outside the simulated scene.
					wantKbps := float64(600)
					if mode == "OK" {
						wantKbps = 4200
					}
					if mode == "DOWN" {
						wantKbps = 0
					}
					for _, id := range []string{"a", "b"} {
						if got := v["camera."+id+".kbps"]; !got.Defined || got.Number != wantKbps {
							t.Errorf("camera.%s.kbps = %+v, want %v", id, got, wantKbps)
						}
					}
				})
			}
		}
	}
}

func TestSimulationActualAndCustomCamera(t *testing.T) {
	now := time.Now()
	for _, ids := range [][]string{nil, {"b"}, {"a", "b"}} {
		s := core.Snapshot{
			Cameras:      []core.CameraStatus{{ID: "a"}, {ID: "b", Kbps: 731, Protocol: "rtmp", Online: true}},
			OBS:          core.OBSStatus{Connected: true, Scene: "Actual", SceneCameras: ids},
			SceneDetails: map[string]obs.Scene{"live": {Name: "Live", Kind: "camera", Cameras: []string{"a"}}},
		}
		actual := Values(s, now)
		for _, mode := range []string{"", "actual", "offline", "custom"} {
			t.Run(mode+"/"+strings.Join(ids, ","), func(t *testing.T) {
				sim := Simulation{Mode: mode}
				if mode == "custom" {
					sim.Values = map[string]any{"camera.kbps": float64(123), "camera.b.kbps": float64(999), "obs.scene": "Live"}
				}
				v, err := sim.Apply(s, now, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, field := range []string{"state", "kbps", "online", "protocol", "state_for", "srt.rtt", "srt.loss", "srt.mbps", "id", "name", "hidden"} {
					want := actual["camera."+field]
					if mode == "custom" && field == "kbps" {
						want = template.Value{Defined: true, Type: template.Number, Number: 123}
					}
					if got := v["camera."+field]; got != want {
						t.Errorf("camera.%s = %+v, want %+v", field, got, want)
					}
				}
			})
		}
	}
}
