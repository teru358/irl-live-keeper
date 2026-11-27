package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"testing"
	"time"
)

func TestValues(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	ret, health := "Camera", "LOW"
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "main", Name: "正面", State: "LOW", Kbps: 600, Online: true, Protocol: "srt", StateSince: now.Add(-12 * time.Second), Hidden: 2, SRT: &core.SRTStatus{RTTMs: 80, LossRate: 0.05, RecvMbps: 4.2}}, {ID: "rear", Name: "後方", State: "DOWN"}}, OBS: core.OBSStatus{Connected: true, Streaming: true, Scene: "Standby", SceneClass: "standby", SceneCameras: []string{"main"}, SceneHealth: &health, Standby: core.StandbyStatus{Owner: "auto", ReturnScene: &ret, Since: now.Add(-30 * time.Second)}}, MediaMTX: "running"}
	d, v := Definitions(s), Values(s, now)
	expected := map[string]template.Definition{}
	add := func(names []string, typ template.Type, format string) {
		for _, n := range names {
			expected[n] = template.Definition{Type: typ, Format: format}
		}
	}
	add([]string{"standby.owner", "standby.return", "obs.scene", "obs.scene_class", "obs.scene_health", "mediamtx.state"}, template.String, "")
	add([]string{"obs.connected", "obs.streaming"}, template.Bool, "")
	add([]string{"standby.for", "stream.elapsed"}, template.Seconds, "hms")
	add([]string{"now"}, template.Clock, "hm")
	add([]string{"stream.kbps", "stream.dropped", "obs.render_skipped"}, template.Number, "0")
	add([]string{"stream.congestion"}, template.Number, "2")
	add([]string{"obs.fps"}, template.Number, "1")
	for _, p := range []string{"camera.", "camera.main.", "camera.rear."} {
		add([]string{p + "state", p + "protocol"}, template.String, "")
		add([]string{p + "kbps"}, template.Number, "0")
		add([]string{p + "online"}, template.Bool, "")
		add([]string{p + "state_for"}, template.Seconds, "hms")
		add([]string{p + "name"}, template.String, "")
		add([]string{p + "hidden", p + "srt.rtt"}, template.Number, "0")
		add([]string{p + "srt.loss"}, template.Number, "2")
		add([]string{p + "srt.mbps"}, template.Number, "1")
	}
	add([]string{"camera.id"}, template.String, "")
	// session.* は区切り3の保留のあいだ一覧に入れない（task 表 v1.14）
	for _, n := range []string{"session.low_count", "session.standby_total"} {
		if _, ok := d[n]; ok {
			t.Fatalf("%s が保留なのに一覧にある", n)
		}
	}
	if len(d) != len(expected) {
		t.Fatalf("一覧の数 = %d, want %d", len(d), len(expected))
	}
	for n, want := range expected {
		if got, ok := d[n]; !ok || got != want {
			t.Errorf("%s 定義 = %+v, want %+v", n, got, want)
		}
	}
	for _, n := range []string{"state", "protocol", "kbps", "online", "state_for", "srt.rtt", "srt.loss", "srt.mbps", "name", "hidden"} {
		if v["camera."+n] != v["camera.main."+n] {
			t.Errorf("別名 %s", n)
		}
	}

	for n, def := range d {
		if v[n].Type != def.Type {
			t.Errorf("%s 型", n)
		}
		p, err := template.Compile("{"+n+"}", d)
		if err != nil {
			t.Fatal(n, err)
		}
		_ = p.Render(v)
	}
	for n, want := range map[string]string{"camera.state": "LOW", "camera.main.name": "正面", "camera.rear.state": "DOWN", "standby.return": "Camera", "obs.scene": "Standby", "obs.scene_class": "standby", "obs.scene_health": "LOW", "mediamtx.state": "running", "standby.owner": "auto", "camera.protocol": "srt"} {
		if !v[n].Defined || v[n].String != want {
			t.Errorf("%s = %+v", n, v[n])
		}
	}
	for n, want := range map[string]float64{"camera.kbps": 600, "camera.state_for": 12, "standby.for": 30, "camera.srt.rtt": 80, "camera.srt.loss": 0.05, "camera.srt.mbps": 4.2, "camera.main.hidden": 2} {
		if !v[n].Defined || v[n].Number != want {
			t.Errorf("%s = %+v", n, v[n])
		}
	}
	for _, n := range []string{"camera.online", "obs.connected", "obs.streaming"} {
		if !v[n].Defined || !v[n].Bool {
			t.Error(n)
		}
	}
	if v["now"].Time != now {
		t.Error("now")
	}
	for _, n := range []string{"stream.elapsed", "stream.kbps", "stream.dropped", "stream.congestion", "obs.fps", "obs.render_skipped"} {
		if v[n].Defined {
			t.Error(n)
		}
	}
	for _, n := range []string{"urls", "password"} {
		if _, ok := d[n]; ok {
			t.Error(n)
		}
	}
	s.Cameras[0].Protocol = "rtmp"
	s.OBS = core.OBSStatus{Standby: core.StandbyStatus{Owner: "none"}}
	v = Values(s, now)
	for _, n := range []string{"camera.srt.rtt", "camera.main.srt.rtt", "standby.for", "standby.return", "obs.scene", "obs.scene_health"} {
		if v[n].Defined {
			t.Error(n)
		}
	}
	s.Cameras = nil
	if Values(s, now)["camera.state"].Defined {
		t.Error("カメラなし")
	}
}

// Each field must follow the visible camera, even when it is not first in config.
func TestCurrentCameraValues(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	for _, protocol := range []string{"srt", "rtmp"} {
		for _, tc := range []struct {
			name      string
			connected bool
			ids       []string
			defined   bool
		}{
			{"zero", true, nil, false},
			{"one", true, []string{"rear"}, true},
			{"two", true, []string{"main", "rear"}, false},
			{"disconnected", false, []string{"rear"}, false},
			{"unknown", true, []string{"missing"}, false},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				s := core.Snapshot{
					Cameras: []core.CameraStatus{{ID: "main", State: "DOWN"}, {
						ID: "rear", Name: "後方", State: "LOW", Kbps: 601.5, Online: true,
						Protocol: protocol, StateSince: now.Add(-12500 * time.Millisecond), Hidden: 2,
						SRT: &core.SRTStatus{RTTMs: 81, LossRate: 0.06, RecvMbps: 4.3},
					}},
					OBS: core.OBSStatus{Connected: tc.connected, SceneCameras: tc.ids},
				}
				want := map[string]template.Value{
					"state":     {Defined: true, Type: template.String, String: "LOW"},
					"kbps":      {Defined: true, Type: template.Number, Number: 601.5},
					"online":    {Defined: true, Type: template.Bool, Bool: true},
					"protocol":  {Defined: true, Type: template.String, String: protocol},
					"state_for": {Defined: true, Type: template.Seconds, Number: 12.5},
					"srt.rtt":   {Defined: protocol == "srt", Type: template.Number, Number: 81},
					"srt.loss":  {Defined: protocol == "srt", Type: template.Number, Number: 0.06},
					"srt.mbps":  {Defined: protocol == "srt", Type: template.Number, Number: 4.3},
					"id":        {Defined: true, Type: template.String, String: "rear"},
					"name":      {Defined: true, Type: template.String, String: "後方"},
					"hidden":    {Defined: true, Type: template.Number, Number: 2},
				}
				v := Values(s, now)
				for field, expected := range want {
					if !tc.defined || !expected.Defined {
						expected = template.Value{Type: expected.Type}
					}
					if got, ok := v["camera."+field]; !ok || got != expected {
						t.Errorf("camera.%s = %+v (present %v), want %+v", field, got, ok, expected)
					}
				}
				// Fixed cameras remain available regardless of scene/OBS connection.
				if got := v["camera.rear.kbps"]; !got.Defined || got.Number != 601.5 {
					t.Errorf("fixed camera changed: %+v", got)
				}
			})
		}
	}
}

func TestRemovedValueNames(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "main"}}}
	d, v := Definitions(s), Values(s, time.Now())
	for _, name := range []string{"link.state", "link.kbps", "link.online", "link.protocol", "link.state_for", "srt.rtt", "srt.loss", "srt.mbps"} {
		t.Run(name, func(t *testing.T) {
			if _, ok := d[name]; ok {
				t.Error("removed name still listed")
			}
			if _, ok := v[name]; ok {
				t.Error("removed name still in values")
			}
			_, err := template.Compile("{"+name+"}", d)
			if e, ok := err.(*template.Error); !ok || e.Message.Code != "template.unknown" {
				t.Fatalf("want unknown name, got %v", err)
			}
		})
	}
}
