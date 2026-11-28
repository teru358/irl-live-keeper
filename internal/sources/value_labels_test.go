package sources_test

import (
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
)

func TestEverySourceValueHasMeaning(t *testing.T) {
	s := core.Snapshot{Cameras: []core.CameraStatus{{ID: "cam1"}, {ID: "cam2"}}}
	for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
		ui := i18n.UI(lang)
		for name := range sources.Definitions(s) {
			keyName := name
			for _, c := range s.Cameras {
				if tail, ok := strings.CutPrefix(name, c.ID+"."); ok {
					keyName = "cam<n>." + tail
				}
			}
			if ui["src_value_"+keyName] == "" {
				t.Errorf("%s missing meaning for %s (key %s)", lang, name, keyName)
			}
		}
		for _, key := range []string{"src_values", "src_values_filter", "src_values_filter_hint", "src_values_empty", "src_values_test", "src_values_camera", "src_values_standby", "src_values_obs", "src_values_stream", "src_values_other"} {
			if ui[key] == "" {
				t.Errorf("%s missing %s", lang, key)
			}
		}
		for _, placeholder := range []string{"{id}", "{name}"} {
			if !strings.Contains(ui["src_values_camera"], placeholder) {
				t.Errorf("%s camera heading missing %s", lang, placeholder)
			}
		}
		if !strings.Contains(ui["src_value_cam<n>.srt.loss"], "%") {
			t.Errorf("%s loss must use status unit %%", lang)
		}
	}
}
