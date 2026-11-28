package settings

import (
	"github.com/teru358/irl-live-keeper/internal/config"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreBackupCredentials(t *testing.T) {
	for _, readable := range []bool{true, false} {
		t.Run(map[bool]string{true: "readable", false: "unreadable"}[readable], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			old, err := config.Generate(path)
			if err != nil {
				t.Fatal(err)
			}
			old.Admin.Password = "originalAdmin"
			if err = os.WriteFile(path+".bak", mustConfig(t, old), 0600); err != nil {
				t.Fatal(err)
			}
			current := *old
			current.Admin.Password = "currentAdmin"
			current.Stream.ReadPass = "currentRead"
			current.Cameras = append([]config.Camera(nil), old.Cameras...)
			current.Cameras[0].PublishPass = "currentPublish"
			raw := mustConfig(t, &current)
			if !readable {
				raw = []byte("invalid: [")
			}
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			next, reset, err := RestoreBackup(path)
			if err != nil {
				t.Fatal(err)
			}
			if reset == readable {
				t.Fatal("wrong recovery notice")
			}
			if readable {
				if next.Admin.Password != current.Admin.Password || next.Stream.ReadPass != current.Stream.ReadPass || next.Cameras[0].PublishPass != current.Cameras[0].PublishPass {
					t.Fatal("lost current credentials")
				}
			} else {
				if next.Admin.Password == old.Admin.Password || next.Stream.ReadPass == old.Stream.ReadPass || next.Cameras[0].PublishPass == old.Cameras[0].PublishPass {
					t.Fatal("restored revoked credentials")
				}
			}
		})
	}
}

func mustConfig(t *testing.T, c *config.Config) []byte {
	t.Helper()
	raw, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
