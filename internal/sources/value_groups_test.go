package sources

import (
	"reflect"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
)

func TestValueGroupsOrder(t *testing.T) {
	for _, cameras := range [][]core.CameraStatus{nil, {{ID: "cam1", Name: "正面"}, {ID: "cam2", Name: "後方"}}} {
		s := core.Snapshot{Cameras: cameras}
		groups, err := ValueGroups(s, Values(s, time.Now()))
		if err != nil {
			t.Fatal(err)
		}
		cameraFields := []string{"state", "kbps", "online", "protocol", "state_for", "srt.rtt", "srt.loss", "srt.mbps", "id", "name", "hidden"}
		wantKinds := []string{"current_camera"}
		wantNames := [][]string{{}}
		for _, field := range cameraFields {
			wantNames[0] = append(wantNames[0], "camera."+field)
		}
		for _, c := range cameras {
			wantKinds = append(wantKinds, "camera")
			ns := []string{}
			for _, field := range cameraFields {
				if field != "id" {
					ns = append(ns, "camera."+c.ID+"."+field)
				}
			}
			wantNames = append(wantNames, ns)
		}
		wantKinds = append(wantKinds, "standby", "obs", "stream", "other")
		wantNames = append(wantNames,
			[]string{"standby.owner", "standby.return", "standby.for"},
			[]string{"obs.connected", "obs.streaming", "obs.scene", "obs.scene_class", "obs.scene_health", "obs.fps", "obs.render_skipped"},
			[]string{"stream.elapsed", "stream.kbps", "stream.dropped", "stream.congestion"},
			[]string{"now", "mediamtx.state"})
		seen := map[string]bool{}
		if len(groups) != len(wantKinds) {
			t.Fatalf("groups=%+v", groups)
		}
		for i, g := range groups {
			if g.Kind != wantKinds[i] {
				t.Errorf("group %d=%s", i, g.Kind)
			}
			if g.Kind == "camera" {
				if g.Camera == nil || g.Camera.ID != cameras[i-1].ID || g.Camera.Name != cameras[i-1].Name {
					t.Errorf("camera=%+v", g.Camera)
				}
			} else if g.Camera != nil {
				t.Errorf("camera on %s", g.Kind)
			}
			ns := []string{}
			for _, v := range g.Values {
				ns = append(ns, v.Name)
				if seen[v.Name] {
					t.Errorf("duplicate %s", v.Name)
				}
				seen[v.Name] = true
			}
			if !reflect.DeepEqual(ns, wantNames[i]) {
				t.Errorf("%s names=%v want=%v", g.Kind, ns, wantNames[i])
			}
		}
		defs := Definitions(s)
		if len(seen) != len(defs) {
			t.Errorf("listed=%d definitions=%d", len(seen), len(defs))
		}
		for name := range defs {
			if !seen[name] {
				t.Errorf("missing %s", name)
			}
		}
	}
}

func TestValueGroupsDefaultText(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 34, 0, 0, time.Local)
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1", Name: "", State: "LOW", Kbps: 600.4, Online: true, Protocol: "srt", StateSince: now.Add(-5025 * time.Second), SRT: &core.SRTStatus{RTTMs: 80.6, LossRate: 0.05, RecvMbps: 4.23}}}, OBS: core.OBSStatus{Connected: true, SceneCameras: []string{"cam1"}}}
	values := Values(s, now)
	values["password"] = template.Value{Defined: true, Type: template.String, String: "secret"}
	groups, err := ValueGroups(s, values)
	if err != nil {
		t.Fatal(err)
	}
	flat := map[string]ListedValue{}
	for _, g := range groups {
		for _, v := range g.Values {
			flat[v.Name] = v
		}
	}
	for name, want := range map[string]string{"camera.state": "LOW", "camera.kbps": "600", "camera.online": "true", "camera.state_for": "1:23:45", "camera.srt.rtt": "81", "camera.srt.loss": "0.05", "camera.srt.mbps": "4.2", "camera.id": "cam1", "camera.name": "", "now": "12:34"} {
		v := flat[name]
		if v.Text == nil || *v.Text != want {
			t.Errorf("%s=%+v want %q", name, v, want)
		}
		p, e := template.Compile("{"+name+"}", Definitions(s))
		if e != nil {
			t.Fatal(e)
		}
		if v.Text != nil && *v.Text != p.Render(values) {
			t.Errorf("%s differs from template", name)
		}
	}
	for _, name := range []string{"standby.return", "standby.for", "stream.elapsed", "obs.scene_health"} {
		if flat[name].Text != nil {
			t.Errorf("%s should be null", name)
		}
	}
	if _, ok := flat["password"]; ok {
		t.Error("leaked undefined name")
	}
	for name, want := range map[string]struct{ typ, format string }{"camera.kbps": {"number", "0"}, "camera.state": {"string", ""}, "camera.online": {"bool", ""}, "camera.state_for": {"seconds", "hms"}, "now": {"clock", "hm"}} {
		v := flat[name]
		if v.Type != want.typ || v.Format != want.format {
			t.Errorf("%s=%+v", name, v)
		}
	}
}
