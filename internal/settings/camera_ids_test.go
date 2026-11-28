package settings

import (
	"encoding/json"
	"github.com/teru358/irl-live-keeper/internal/config"
	"reflect"
	"strings"
	"testing"
)

func TestCameraDiffByID(t *testing.T) {
	cam := func(id, name string) config.Camera { return config.Camera{ID: id, Name: name, OnBad: "standby"} }
	a, b, c := cam("cam1", "One"), cam("cam2", "Two"), cam("cam3", "Three")
	for _, tc := range []struct {
		name      string
		old, next []config.Camera
		paths     []string
		restart   bool
	}{
		{"delete and edit", []config.Camera{a, b, c}, []config.Camera{a, cam("cam3", "Changed")}, []string{"cameras[cam2]", "cameras[cam3].name"}, true},
		{"same count replacement", []config.Camera{a, b}, []config.Camera{b, c}, []string{"cameras[cam1]", "cameras[cam3]"}, true},
		{"reorder", []config.Camera{a, b}, []config.Camera{b, a}, nil, false},
		{"live edit", []config.Camera{a, b}, []config.Camera{a, cam("cam2", "Changed")}, []string{"cameras[cam2].name"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, next := testConfig(t), testConfig(t)
			old.Cameras = tc.old
			next.Cameras = tc.next
			got := ClassifyChanges(old, next, true)
			var paths []string
			for _, f := range got.Fields {
				paths = append(paths, f.Path)
				if f.Path == "cameras[cam1]" || f.Path == "cameras[cam2]" || f.Path == "cameras[cam3]" {
					raw, _ := json.Marshal(f)
					var metadata map[string]any
					_ = json.Unmarshal(raw, &metadata)
					kind := "removed"
					name := "One"
					if f.Path == "cameras[cam2]" {
						name = "Two"
					}
					if f.Path == "cameras[cam3]" {
						kind = "added"
						name = "Three"
					}
					if metadata["kind"] != kind || metadata["camera_name"] != name {
						t.Fatalf("metadata=%v", metadata)
					}
				}
			}
			if !reflect.DeepEqual(paths, tc.paths) || got.OBSURLChanges || (got.Apply == ApplyRestart) != tc.restart {
				t.Fatalf("%+v paths=%v", got, paths)
			}
		})
	}
}
func TestRewriteCameraNumberOrder(t *testing.T) {
	cams := []config.Camera{{ID: "cam3", Name: "Three", OnBad: "hide"}, {ID: "cam1", Name: "One", OnBad: "standby"}}
	out, err := Rewrite([]byte(base), []Change{{Path: "cameras", Value: cams}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(out), "cam1") > strings.Index(string(out), "cam3") {
		t.Fatalf("%s", out)
	}
	// Saving another section must also normalize a manually reversed camera list.
	reversed := strings.Replace(base, "id: cam1", "id: cam3", 1) + "  - id: cam1\n    name: One\n    on_bad: standby\n"
	out, err = Rewrite([]byte(reversed), []Change{{Path: "log.keep_days", Value: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(out), "cam1") > strings.Index(string(out), "cam3") {
		t.Fatalf("%s", out)
	}
}
