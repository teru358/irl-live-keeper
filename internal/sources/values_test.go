package sources

import (
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"strings"
	"testing"
	"time"
)

func TestValues(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	ret, health := "Camera", "LOW"
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1", Name: "正面", State: "LOW", Kbps: 600, Online: true, Protocol: "srt", StateSince: now.Add(-12 * time.Second), Hidden: 2, SRT: &core.SRTStatus{RTTMs: 80, LossRate: 0.05, RecvMbps: 4.2}}, {ID: "cam2", Name: "後方", State: "DOWN"}}, OBS: core.OBSStatus{Connected: true, Streaming: true, Scene: "Standby", SceneClass: "standby", SceneCameras: []string{"cam1"}, SceneHealth: &health, Standby: core.StandbyStatus{Owner: "auto", ReturnScene: &ret, Since: now.Add(-30 * time.Second)}}, MediaMTX: "running"}
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
	for _, p := range []string{"cam1.", "cam2."} {
		add([]string{p + "state", p + "protocol"}, template.String, "")
		add([]string{p + "kbps"}, template.Number, "0")
		add([]string{p + "online"}, template.Bool, "")
		add([]string{p + "state_for"}, template.Seconds, "hms")
		add([]string{p + "name"}, template.String, "")
		add([]string{p + "hidden", p + "srt.rtt"}, template.Number, "0")
		add([]string{p + "srt.loss"}, template.Number, "2")
		add([]string{p + "srt.mbps"}, template.Number, "1")
	}
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
	for n, want := range map[string]string{"cam1.state": "LOW", "cam1.name": "正面", "cam2.state": "DOWN", "standby.return": "Camera", "obs.scene": "Standby", "obs.scene_class": "standby", "obs.scene_health": "LOW", "mediamtx.state": "running", "standby.owner": "auto", "cam1.protocol": "srt"} {
		if !v[n].Defined || v[n].String != want {
			t.Errorf("%s = %+v", n, v[n])
		}
	}
	for n, want := range map[string]float64{"cam1.kbps": 600, "cam1.state_for": 12, "standby.for": 30, "cam1.srt.rtt": 80, "cam1.srt.loss": 0.05, "cam1.srt.mbps": 4.2, "cam1.hidden": 2} {
		if !v[n].Defined || v[n].Number != want {
			t.Errorf("%s = %+v", n, v[n])
		}
	}
	for _, n := range []string{"cam1.online", "obs.connected", "obs.streaming"} {
		if !v[n].Defined || !v[n].Bool {
			t.Error(n)
		}
	}
	if v["now"].Time != now {
		t.Error("now")
	}
	// v0.26: 受信していないカメラの protocol は空の文字でなく未定義
	if v["cam2.protocol"].Defined {
		t.Errorf("cam2.protocol = %+v, want 未定義", v["cam2.protocol"])
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
	for _, n := range []string{"cam1.srt.rtt", "cam1.srt.rtt", "standby.for", "standby.return", "obs.scene", "obs.scene_health"} {
		if v[n].Defined {
			t.Error(n)
		}
	}
	s.Cameras = nil
	if Values(s, now)["cam1.state"].Defined {
		t.Error("カメラなし")
	}
}

func TestSceneIndependentCameraValues(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1", Kbps: 123}}}
	for _, ids := range [][]string{nil, {"cam1"}, {"cam1", "cam2"}} {
		s.OBS.SceneCameras = ids
		v := Values(s, time.Now())
		if !v["cam1.kbps"].Defined || v["cam1.kbps"].Number != 123 {
			t.Fatal(v)
		}
		for n := range Definitions(s) {
			if strings.HasPrefix(n, "camera.") {
				t.Fatal(n)
			}
		}
	}
}

func TestRemovedValueNames(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1"}}}
	d, v := Definitions(s), Values(s, time.Now())
	for _, name := range []string{"camera.kbps", "camera.cam1.kbps", "link.state", "link.kbps", "link.online", "link.protocol", "link.state_for", "srt.rtt", "srt.loss", "srt.mbps"} {
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
