package core

import (
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"reflect"
	"testing"
)

func TestSceneDetails(t *testing.T) {
	m := &scenemap.Map{Scenes: map[string]scenemap.Container{}, Inputs: map[string]scenemap.Input{"input": {CameraID: "cam1"}}}
	for _, name := range []string{"Camera", "OtherCamera", "Wait", "Image", "Talk"} {
		sc := scenemap.Container{Name: name}
		if name == "Camera" || name == "OtherCamera" {
			sc.Items = []scenemap.Item{{SourceUUID: "input", SourceKind: scenemap.InputSource, Enabled: true}}
		}
		m.Scenes[name] = sc
	}
	for _, tc := range []struct {
		name    string
		rows    []switcher.FallbackRow
		target  string
		kind    string
		parents []string
	}{
		{"共通", nil, "Wait", "standby", []string{"Camera", "OtherCamera"}},
		{"個別カメラあり", []switcher.FallbackRow{{Scene: "Camera", Fallback: "OtherCamera"}}, "OtherCamera", "camera", []string{"Camera"}},
		{"個別カメラなし", []switcher.FallbackRow{{Scene: "Camera", Fallback: "Image"}}, "Image", "standby", []string{"Camera"}},
		{"ただのカメラなし", nil, "Talk", "none", []string{}},
		{"共通から個別を除く", []switcher.FallbackRow{{Scene: "Camera", Fallback: "Image"}}, "Wait", "standby", []string{"OtherCamera"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sceneDetails(m, "Wait", tc.rows)[tc.target]
			if got.Kind != tc.kind || !reflect.DeepEqual(got.FallbackOf, tc.parents) {
				t.Fatalf("%+v", got)
			}
			if got.Kind == "camera" && !reflect.DeepEqual(got.Cameras, []string{"cam1"}) {
				t.Fatal(got)
			}
		})
	}
}
