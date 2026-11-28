package config

import (
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"testing"
)

func TestNumberedCameraIDs(t *testing.T) {
	for _, id := range []string{"cam1", "cam2", "cam3", "cam4", "cam5", "cam6", "cam7", "cam8", "cam0", "cam9", "main", "Cam1", "cam01", "all", "all_others", "state"} {
		t.Run(id, func(t *testing.T) {
			c := valid()
			c.Cameras = []Camera{{ID: id, Name: "Camera", OnBad: "standby"}}
			err := c.Validate()
			good := len(id) == 4 && id >= "cam1" && id <= "cam8"
			if good {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected validation: %v", err)
			}
			if got := ve.Fields[0]; got.Field != "cameras["+id+"].id" || got.Message.Code != "config.validation_23" {
				t.Fatalf("%+v", got)
			}
			for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
				if ve.RenderFields(lang)["cameras["+id+"].id"] == "" {
					t.Fatal("missing translation")
				}
			}
		})
	}
}
func TestLoadCameraNumberOrder(t *testing.T) {
	raw := camMinimal + "cameras:\n"
	for _, i := range []int{8, 2, 1, 7, 3, 6, 5, 4} {
		raw += fmt.Sprintf("  - id: cam%d\n    name: Camera %d\n    on_bad: standby\n", i, i)
	}
	c, err := Load(writeCfg(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	for i, cam := range c.Cameras {
		if cam.ID != fmt.Sprintf("cam%d", i+1) {
			t.Fatalf("%+v", c.Cameras)
		}
	}
}
