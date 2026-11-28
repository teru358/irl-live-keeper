package sources

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
	"sort"
	"time"
)

// Simulation は表示だけの差し替え（第9章「回線の状況のテスト」）。
// 3つの軸（回線・待機・配信）は、それぞれ別の値だけを決める。空か "actual" の軸は実際の値のまま。
// Values は「自由」で、軸で決めた値（軸がすべて実際なら実際の値）の上に重ねる。
type Simulation struct {
	Line    string         `json:"line,omitempty"`
	Standby string         `json:"standby,omitempty"`
	Stream  string         `json:"stream,omitempty"`
	Values  map[string]any `json:"values,omitempty"`
}

const actualChoice = "actual"

func axisChoice(v string, choices ...string) (string, bool) {
	if v == "" {
		return actualChoice, true
	}
	for _, c := range append(choices, actualChoice) {
		if v == c {
			return v, true
		}
	}
	return "", false
}

// IsActual は、すべての軸が実際で、自由の値もないときに true を返す。
func (sim Simulation) IsActual() bool {
	return (sim.Line == "" || sim.Line == actualChoice) && (sim.Standby == "" || sim.Standby == actualChoice) &&
		(sim.Stream == "" || sim.Stream == actualChoice) && len(sim.Values) == 0
}

// SameChoices は軸の選びと自由の有無が同じか（ログで状況が変わったかを見るため。値の中身は見ない）。
func (sim Simulation) SameChoices(o Simulation) bool {
	norm := func(s Simulation) [4]string {
		line, _ := axisChoice(s.Line, "OK", "LOW", "DOWN")
		standby, _ := axisChoice(s.Standby, "none", "auto")
		stream, _ := axisChoice(s.Stream, "streaming", "offline")
		free := "no"
		if len(s.Values) > 0 {
			free = "yes"
		}
		return [4]string{line, standby, stream, free}
	}
	return norm(sim) == norm(o)
}

// LogFields はログに書く状況の名前（値は書かない）。
func (sim Simulation) LogFields() []any {
	line, _ := axisChoice(sim.Line, "OK", "LOW", "DOWN")
	standby, _ := axisChoice(sim.Standby, "none", "auto")
	stream, _ := axisChoice(sim.Stream, "streaming", "offline")
	return []any{"line", line, "standby", standby, "stream", stream, "free", len(sim.Values) > 0}
}

// Apply は実際の値に軸と自由の差し替えを当てる。s は変えない。
func (sim Simulation) Apply(s core.Snapshot, now time.Time, stats map[string]template.Value) (map[string]template.Value, error) {
	line, ok1 := axisChoice(sim.Line, "OK", "LOW", "DOWN")
	standby, ok2 := axisChoice(sim.Standby, "none", "auto")
	stream, ok3 := axisChoice(sim.Stream, "streaming", "offline")
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("invalid simulation axis")
	}
	v := Values(s, now, stats)
	defs := Definitions(s)
	set := func(n string, x any) error {
		d, ok := defs[n]
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
	put := func(n string, x any) { _ = set(n, x) }
	// 自由の値は軸の値の上に重ねる。軸がすべて実際でも、知らない名前や型の違いは誤り。
	free := func() (map[string]template.Value, error) {
		for n, x := range sim.Values {
			if err := set(n, x); err != nil {
				return nil, err
			}
		}
		return v, nil
	}
	if line == actualChoice && standby == actualChoice && stream == actualChoice {
		return free()
	}
	put("obs.connected", true)
	put("obs.fps", float64(60))
	put("obs.render_skipped", float64(0))
	put("mediamtx.state", "running")

	monitored, standbyScene := scenesFor(s)
	if line != actualChoice {
		for _, c := range s.Cameras {
			p := "srt"
			if c.Online && c.Protocol != "" {
				p = c.Protocol
			}
			state, kbps, online := line, float64(0), false
			switch line {
			case "OK":
				kbps, online = 4200, true
			case "LOW":
				kbps, online = 600, true
			default:
				p = ""
			}
			prefix := c.ID + "."
			put(prefix+"state", state)
			put(prefix+"kbps", kbps)
			put(prefix+"online", online)
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
				if line == "OK" {
					rtt, loss = 80, 0
				}
				put(sp+"rtt", rtt)
				put(sp+"loss", loss)
				put(sp+"mbps", kbps/1000)
			}
		}
		// scene_health は、OBS のシーンにカメラが映るときだけその状態になる。
		shows := s.OBS.Connected && (s.OBS.SceneClass == "monitored" || len(s.OBS.SceneCameras) > 0)
		switch standby {
		case "none":
			shows = monitored != ""
		case "auto":
			shows = false
		}
		if shows {
			put("obs.scene_health", line)
		} else {
			put("obs.scene_health", nil)
		}
	}
	switch standby {
	case "none":
		put("standby.owner", "none")
		put("standby.return", nil)
		put("standby.for", nil)
		putText(put, "obs.scene", monitored)
		if monitored != "" {
			put("obs.scene_class", "monitored")
		} else {
			put("obs.scene_class", nil)
		}
	case "auto":
		put("standby.owner", "auto")
		putText(put, "standby.return", monitored)
		put("standby.for", float64(12))
		putText(put, "obs.scene", standbyScene)
		put("obs.scene_class", "standby")
	}
	switch stream {
	case "streaming":
		put("obs.streaming", true)
		put("stream.elapsed", float64(5025))
		put("stream.kbps", float64(6000))
		put("stream.dropped", float64(0))
		put("stream.congestion", float64(0))
	case "offline":
		put("obs.streaming", false)
		for _, n := range []string{"stream.elapsed", "stream.kbps", "stream.dropped", "stream.congestion"} {
			put(n, nil)
		}
	}
	return free()
}

// putText は空の文字を未定義として入れる。
func putText(put func(string, any), n, text string) {
	if text == "" {
		put(n, nil)
		return
	}
	put(n, text)
}

// scenesFor は監視対象のシーンの最初のものと待機シーンの名前を返す（ないときは空）。
func scenesFor(s core.Snapshot) (monitored, standby string) {
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
	for _, k := range keys {
		sc := s.SceneDetails[k]
		if monitored == "" && (sc.Kind == "camera" || sc.Kind == "monitored") {
			monitored = sc.Name
		}
		if standby == "" && sc.Kind == "standby" {
			standby = sc.Name
		}
	}
	if monitored == "" && s.OBS.SceneClass == "monitored" {
		monitored = s.OBS.Scene // 地図がなければ、今のシーン名だけを使う。カメラは推測しない。
	}
	return monitored, standby
}
func RenderSimulation(src Source, s core.Snapshot, now time.Time, stats map[string]template.Value, sim Simulation) (string, error) {
	if src.Broken != nil {
		return "", nil
	}
	p, err := CompileHTML(src.Body, Definitions(s))
	if err != nil {
		return "", err
	}
	v, err := sim.Apply(s, now, stats)
	if err != nil {
		return "", err
	}
	out := p.Render(v)
	if err := ValidateHTML(out, "rendered"); err != nil {
		return "", err
	}
	return out, nil
}
