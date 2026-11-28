package settings

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/config"
	"os"
)

// RestoreBackup is shared by the recovery window and CLI. The backup's
// credentials are never restored, even when the current config is unreadable.
func RestoreBackup(path string) (*config.Config, bool, error) {
	return restoreBackup(path, os.Rename)
}

// restoreBackup writes config.yaml first and only then swaps the old content
// into .bak. Both writes go through a temporary file and a rename, so a failure
// or a crash at any point leaves at least one startable version behind: before
// the first rename .bak is untouched, after it config.yaml holds the restored
// version.
func restoreBackup(path string, rename func(string, string) error) (*config.Config, bool, error) {
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
	if err = writeFile(path, out, rename); err != nil {
		return nil, false, err
	}
	if readErr == nil && current != nil {
		// The restore is done; keeping the replaced version is best effort. If it
		// fails, .bak still holds the version that was just restored.
		_ = writeFile(path+".bak", raw, rename)
	}
	return next, regenerated, nil
}
