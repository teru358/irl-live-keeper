package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"github.com/teru358/irl-live-keeper/internal/web"
)

func (b *pageBackend) SettingsTransaction(fn func(*web.SettingsStore)) error {
	a := b.app
	a.mu.Lock()
	defer a.mu.Unlock()
	path := filepath.Join(a.opts.BaseDir, configName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fn(&web.SettingsStore{Raw: raw, Current: b.senderConfig, Applying: a.apply.State == ApplyApplying,
		Owned: ownedSettingsPorts(b.senderConfig), Backup: func() error { return nil },
		Write: func(out []byte) error { return settings.WriteFile(path, out) }, ApplySaved: b.ApplySaved})
	return nil
}

// ApplySaved is the S2-28 handoff. Called with App.mu held after a successful
// save; retain both versions and the OBS option until the apply worker consumes them.
func (b *pageBackend) ApplySaved(old, next *config.Config, summary settings.ChangeSummary, updateOBSInput bool) {
	b.app.pendingSettings = &savedSettings{old, next, summary, updateOBSInput}
	// S2-28 will set applying together with the apply worker lifecycle.
	// Until then, retain the saved versions without changing the apply state.
}

type savedSettings struct {
	Old, Next      *config.Config
	Summary        settings.ChangeSummary
	UpdateOBSInput bool
}

func ownedSettingsPorts(c *config.Config) []settings.ListeningPort {
	if c == nil {
		return nil
	}
	var out []settings.ListeningPort
	add := func(protocol string, port int) {
		out = append(out, settings.ListeningPort{Protocol: protocol, Address: fmt.Sprintf("0.0.0.0:%d", port)}, settings.ListeningPort{Protocol: protocol, Address: fmt.Sprintf(":%d", port)})
	}
	if c.MediaMTX.RTMPEnabled {
		add("tcp", c.MediaMTX.RTMPPort)
	}
	if c.MediaMTX.SRTEnabled {
		add("udp", c.MediaMTX.SRTPort)
	}
	add("tcp", c.MediaMTX.RTSPPort)
	return append(out, settings.ListeningPort{Protocol: "tcp", Address: fmt.Sprintf("127.0.0.1:%d", c.MediaMTX.APIPort)})
}
