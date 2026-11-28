package settings

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/config"
	"os"
)

// RestoreBackup is shared by the recovery window and CLI. The backup's
// credentials are never restored, even when the current config is unreadable.
func RestoreBackup(path string) (*config.Config, bool, error) {
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		return nil, false, err
	}
	next, err := config.Parse(bak)
	if err != nil {
		return nil, false, err
	}
	raw, readErr := os.ReadFile(path)
	var current *config.Config
	if readErr == nil {
		current, _ = config.Parse(raw)
	}
	regenerated := current == nil
	if err = next.PreserveCredentials(current, true, true); err != nil {
		return nil, false, err
	}
	changes := []Change{{Path: "admin.password", Value: next.Admin.Password}, {Path: "stream.read_user", Value: next.Stream.ReadUser}, {Path: "stream.read_pass", Value: next.Stream.ReadPass}, {Path: "cameras", Value: next.Cameras}}
	out := bak
	for _, change := range changes {
		out, err = Rewrite(out, []Change{change})
		if err != nil {
			return nil, false, fmt.Errorf("restore %s: %w", change.Path, err)
		}
	}
	if err != nil {
		return nil, false, err
	}
	if readErr == nil && current != nil {
		if err = WriteFile(path+".bak", raw); err != nil {
			return nil, false, err
		}
	}
	if err = WriteFile(path, out); err != nil {
		return nil, false, err
	}
	return next, regenerated, nil
}
