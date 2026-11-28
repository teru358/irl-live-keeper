package settings

import (
	"github.com/teru358/irl-live-keeper/internal/config"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestFieldLocationsComplete(t *testing.T) {
	locations := ConfigFieldLocations()
	if len(locations) != len(ConfigFieldPaths()) {
		t.Fatalf("locations=%d paths=%d", len(locations), len(ConfigFieldPaths()))
	}
	for _, p := range ConfigFieldPaths() {
		l, ok := locations[p]
		if !ok || (l.Page != "cameras" && l.Page != "settings") || l.Group == "" {
			t.Errorf("invalid location %s: %+v", p, l)
		}
	}
}
func TestPageHashNormalization(t *testing.T) {
	a := &config.Config{}
	b := &config.Config{}
	if PageHash(a, "settings") != PageHash(b, "settings") {
		t.Fatal("equal values differ")
	}
	a.Cameras = []config.Camera{{PublishPass: "publishpass", ID: "cam2"}, {ID: "cam1", PublishPass: "publishpass"}}
	b.Cameras = []config.Camera{{PublishPass: "publishpass", ID: "cam1"}, {ID: "cam2", PublishPass: "publishpass"}}
	if PageHash(a, "cameras") != PageHash(b, "cameras") {
		t.Fatal("camera order changes hash")
	}
	b.Log.KeepDays++
	if PageHash(a, "cameras") != PageHash(b, "cameras") {
		t.Fatal("other page changes hash")
	}
	if PageHash(a, "settings") == PageHash(b, "settings") {
		t.Fatal("owned value missing")
	}
}

func TestPageHashDefaultsAndFormatting(t *testing.T) {
	// Parse supplies defaults before hashing; comments and explicit defaults agree.
	a, err := config.Generate(t.TempDir() + "/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := yaml.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	b, err := config.Parse(append([]byte("# comment\n"), raw...))
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"cameras", "settings"} {
		if PageHash(a, page) != PageHash(b, page) {
			t.Fatal("formatting affects", page)
		}
	}
	b.Cameras[0].Switch = &config.SwitchOverride{}
	if PageHash(a, "cameras") != PageHash(b, "cameras") {
		t.Fatal("empty overrides differ from absent overrides")
	}
}

func TestPageHashDistinguishesInheritedAndExplicitZero(t *testing.T) {
	a := &config.Config{Cameras: []config.Camera{{PublishPass: "publishpass", ID: "cam1"}}}
	b := &config.Config{Cameras: []config.Camera{{ID: "cam1", PublishPass: "publishpass", Switch: &config.SwitchOverride{LowSec: new(int)}}}}
	if PageHash(a, "cameras") == PageHash(b, "cameras") {
		t.Fatal("inherited and explicit zero must have different versions")
	}
}
