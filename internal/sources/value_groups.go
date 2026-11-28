package sources

import (
	"sort"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/sources/template"
)

// ListedValue is one public definition rendered with its default template format.
// Text distinguishes an undefined value (nil) from a defined empty string.
type ListedValue struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Format string  `json:"format"`
	Text   *string `json:"text"`
}

type ValueCamera struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ValueGroup struct {
	Kind   string        `json:"kind"`
	Camera *ValueCamera  `json:"camera,omitempty"`
	Values []ListedValue `json:"values"`
}

// ValueGroups orders only Definitions, independently of map iteration and values
// supplied by statistics or simulation. Cameras are sorted by their numbered ID.
// It reads no state or clock and does not modify its inputs.
func ValueGroups(s core.Snapshot, values map[string]template.Value) ([]ValueGroup, error) {
	defs := Definitions(s)
	cameraFields := []string{"state", "kbps", "online", "protocol", "state_for", "srt.rtt", "srt.loss", "srt.mbps", "id", "name", "hidden"}
	groups := []ValueGroup{}
	add := func(kind string, camera *ValueCamera, prefix string, fields []string) error {
		g := ValueGroup{Kind: kind, Camera: camera, Values: []ListedValue{}}
		for _, field := range fields {
			name := prefix + field
			def, ok := defs[name]
			if !ok {
				continue
			}
			typ := map[template.Type]string{template.Number: "number", template.String: "string", template.Bool: "bool", template.Seconds: "seconds", template.Clock: "clock"}[def.Type]
			row := ListedValue{Name: name, Type: typ, Format: def.Format}
			if values[name].Defined {
				program, err := template.Compile("{"+name+"}", defs)
				if err != nil {
					return err
				}
				text := program.Render(values)
				row.Text = &text
			}
			g.Values = append(g.Values, row)
		}
		groups = append(groups, g)
		return nil
	}
	cameras := append([]core.CameraStatus(nil), s.Cameras...)
	sort.Slice(cameras, func(i, j int) bool { return cameras[i].ID < cameras[j].ID })
	for _, c := range cameras {
		// Per-camera definitions have no id field; the group carries that id.
		fields := []string{}
		for _, field := range cameraFields {
			if field != "id" {
				fields = append(fields, field)
			}
		}
		if err := add("camera", &ValueCamera{ID: c.ID, Name: c.Name}, c.ID+".", fields); err != nil {
			return nil, err
		}
	}
	for _, g := range []struct {
		kind, prefix string
		fields       []string
	}{
		{"standby", "standby.", []string{"owner", "return", "for"}},
		{"obs", "obs.", []string{"connected", "streaming", "scene", "scene_class", "scene_health", "fps", "render_skipped"}},
		{"stream", "stream.", []string{"elapsed", "kbps", "dropped", "congestion"}},
		{"other", "", []string{"now", "mediamtx.state"}},
	} {
		if err := add(g.kind, nil, g.prefix, g.fields); err != nil {
			return nil, err
		}
	}
	return groups, nil
}
