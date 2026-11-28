package supervisor

import (
	"gopkg.in/yaml.v3"
	"testing"
)

func TestPublisherPermissionsAreIsolatedAndDeleted(t *testing.T) {
	cfg := loadCfgWith(t, camsTwo, "")
	for _, count := range []int{2, 1} {
		cfg.Cameras = cfg.Cameras[:count]
		raw, err := GenerateYAML(cfg, "APIUSER", "APIPASS")
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Users []struct {
				User        string
				Permissions []struct{ Action, Path string }
			} `yaml:"authInternalUsers"`
		}
		if err = yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		publishers := 0
		for _, u := range doc.Users {
			if u.User == "obs" || u.User == "APIUSER" {
				continue
			}
			publishers++
			if len(u.Permissions) != 1 || u.Permissions[0].Action != "publish" || u.Permissions[0].Path != cfg.CameraPath(u.User) {
				t.Fatal("publisher has cross-camera permissions")
			}
		}
		if publishers != count {
			t.Fatal("deleted camera retained credentials")
		}
	}
}
