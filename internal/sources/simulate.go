package sources

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"sort"
	"time"
)

// Simulation は表示だけの差し替え。Values は自由のときだけ使う。
type Simulation struct {
	Mode   string         `json:"mode"`
	Values map[string]any `json:"values,omitempty"`
}

func (sim Simulation) Apply(s core.Snapshot, now time.Time, stats map[string]template.Value) (map[string]template.Value, error) {
	v := Values(s, now, stats)
	set := func(n string, x any) error {
		d, ok := Definitions(s)[n]
		if !ok || n == "now" {
			return fmt.Errorf("unknown simulation value: %s", n)
		}
		a := template.Value{Type: d.Type, Defined: x != nil}
		if x != nil {
			switch d.Type {
			case template.String:
				var ok bool
				a.String, ok = x.(string)
				if !ok {
					return fmt.Errorf("invalid %s", n)
				}
			case template.Bool:
				var ok bool
				a.Bool, ok = x.(bool)
				if !ok {
					return fmt.Errorf("invalid %s", n)
				}
			default:
				var ok bool
				a.Number, ok = x.(float64)
				if !ok {
					return fmt.Errorf("invalid %s", n)
				}
			}
		}
		v[n] = a
		return nil
	}
	if sim.Mode == "" || sim.Mode == "actual" {
		if len(sim.Values) > 0 {
			return nil, fmt.Errorf("values require custom")
		}
		return v, nil
	}
	if sim.Mode == "custom" {
		for n, x := range sim.Values {
			if err := set(n, x); err != nil {
				return nil, err
			}
		}
		return v, nil
	}
	if len(sim.Values) > 0 {
		return nil, fmt.Errorf("values require custom")
	}
	if sim.Mode != "OK" && sim.Mode != "LOW" && sim.Mode != "DOWN" && sim.Mode != "standby" && sim.Mode != "offline" {
		return nil, fmt.Errorf("invalid simulation mode")
	}
	put := func(n string, x any) { _ = set(n, x) }
	put("obs.connected", true)
	put("obs.streaming", sim.Mode != "offline")
	put("obs.fps", float64(60))
	put("obs.render_skipped", float64(0))
	put("mediamtx.state", "running")
	put("standby.owner", "none")
	put("standby.return", nil)
	put("standby.for", nil)
	for _, n := range []string{"stream.elapsed", "stream.kbps", "stream.dropped", "stream.congestion"} {
		put(n, nil)
	}
	if sim.Mode == "offline" {
		return v, nil
	}
	state, kbps := "LOW", float64(600)
	if sim.Mode == "OK" {
		state, kbps = "OK", 4200
	}
	if sim.Mode == "DOWN" {
		state, kbps = "DOWN", 0
	}
	for _, c := range s.Cameras {
		p := "srt"
		if c.Online && c.Protocol != "" {
			p = c.Protocol
		}
		if state == "DOWN" {
			p = ""
		}
		prefix := "camera." + c.ID + "."
		put(prefix+"state", state)
		put(prefix+"kbps", kbps)
		put(prefix+"online", state != "DOWN")
		if p == "" {
			put(prefix+"protocol", nil)
		} else {
			put(prefix+"protocol", p)
		}
		put(prefix+"state_for", float64(12))
		sp := prefix + "srt."
		for _, n := range []string{"rtt", "loss", "mbps"} {
			put(sp+n, nil)
		}
		if p == "srt" {
			rtt, loss := float64(250), 0.05
			if state == "OK" {
				rtt, loss = 80, 0
			}
			put(sp+"rtt", rtt)
			put(sp+"loss", loss)
			put(sp+"mbps", kbps/1000)
		}
	}
	keys := make([]string, 0, len(s.SceneDetails))
	for k := range s.SceneDetails {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := s.SceneDetails[keys[i]], s.SceneDetails[keys[j]]
		if a.Name == b.Name {
			return keys[i] < keys[j]
		}
		return a.Name < b.Name
	})
	scene, standby := "", ""
	var sceneCameras, standbyCameras []string
	for _, k := range keys {
		sc := s.SceneDetails[k]
		if scene == "" && (sc.Kind == "camera" || sc.Kind == "monitored") {
			scene = sc.Name
			sceneCameras = sc.Cameras
		}
		if standby == "" && sc.Kind == "standby" {
			standby = sc.Name
			standbyCameras = sc.Cameras
		}
	}
	if scene == "" && s.OBS.SceneClass == "monitored" {
		scene = s.OBS.Scene
		// 地図がなければ、シーン名だけ既存の表示に使う。カメラは推測しない。
	}
	put("obs.scene", scene)
	put("obs.scene_class", "monitored")
	if sim.Mode == "standby" {
		put("standby.owner", "auto")
		if scene != "" {
			put("standby.return", scene)
		}
		put("standby.for", float64(12))
		put("obs.scene", standby)
		put("obs.scene_class", "standby")
		sceneCameras = standbyCameras
	}
	currentCameraValues(v, sceneCameras)
	put("stream.elapsed", float64(5025))
	put("stream.kbps", float64(6000))
	put("stream.dropped", float64(0))
	put("stream.congestion", float64(0))
	return v, nil
}
func RenderSimulation(src Source, s core.Snapshot, now time.Time, stats map[string]template.Value, sim Simulation) (string, error) {
	if src.Broken != nil {
		return "", nil
	}
	p, err := template.Compile(src.Template, Definitions(s))
	if err != nil {
		return "", err
	}
	v, err := sim.Apply(s, now, stats)
	if err != nil {
		return "", err
	}
	return p.Render(v), nil
}
