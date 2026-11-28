package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crypto/sha256"
	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"github.com/teru358/irl-live-keeper/internal/web"
)

type fakeMediaURLWriter struct {
	calls        [][2]string
	err          error
	found, media bool
	lookupErr    error
}

func (f *fakeMediaURLWriter) SetMediaInputURL(uuid, url string) error {
	f.calls = append(f.calls, [2]string{uuid, url})
	return f.err
}
func (f *fakeMediaURLWriter) SetMediaInputURLByOldURL(oldURL, newURL string) (bool, bool, error) {
	f.calls = append(f.calls, [2]string{"old:" + oldURL, newURL})
	return f.found, f.media, f.lookupErr
}

func TestOBSInputURLRewriteUsesCameraMapAndClearsMarkerAfterSuccess(t *testing.T) {
	const secretURL = "rtsp://user:secret@host/live/cam1"
	a := &App{obsInputURLs: map[string]string{"cam1": secretURL}}
	f := &fakeMediaURLWriter{}
	a.updateOBSInputs(f, switcher.OBSConnected{Map: &scenemap.Map{Inputs: map[string]scenemap.Input{
		"uuid-a": {CameraID: "cam1"}, "uuid-b": {CameraID: "cam2"}, "uuid-a2": {CameraID: "cam1"},
	}}})
	if len(f.calls) != 2 || f.calls[0] != [2]string{"uuid-a", secretURL} || f.calls[1] != [2]string{"uuid-a2", secretURL} {
		t.Fatalf("writes = %v", f.calls)
	}
	if len(a.obsInputURLs) != 0 || a.obsInputWarning.Load().Code != "" {
		t.Fatalf("marker/warning remains: %#v %#v", a.obsInputURLs, a.obsInputWarning.Load())
	}
}

func TestOBSInputURLRewriteRetainsMarkerOnAuxFailure(t *testing.T) {
	const secretURL = "rtsp://user:secret@host/live/cam1"
	a := &App{obsInputURLs: map[string]string{"cam1": secretURL}}
	f := &fakeMediaURLWriter{err: errors.New("deadline")}
	a.updateOBSInputs(f, switcher.OBSConnected{Map: &scenemap.Map{Inputs: map[string]scenemap.Input{"uuid-a": {CameraID: "cam1"}}}})
	if a.obsInputURLs["cam1"] != secretURL || a.obsInputWarning.Load().Code != "app.obs_url_pending" {
		t.Fatalf("failure state: %#v %#v", a.obsInputURLs, a.obsInputWarning.Load())
	}
	if strings.Contains(fmt.Sprint(a.obsInputWarning.Load()), "secret") {
		t.Fatal("warning leaked URL password")
	}
}

func TestOBSInputURLRewriteDropsMissingInputAndWarns(t *testing.T) {
	a := &App{obsInputURLs: map[string]string{"cam1": "rtsp://host/live/cam1"}, updateOBSInput: true}
	f := &fakeMediaURLWriter{}
	a.updateOBSInputs(f, switcher.OBSConnected{Map: &scenemap.Map{Inputs: map[string]scenemap.Input{}}})
	if a.updateOBSInput || a.obsInputWarning.Load().Code != "app.obs_url_missing" || len(f.calls) != 0 {
		t.Fatalf("missing state: %+v calls=%v", a, f.calls)
	}
}

func TestOBSInputURLRewriteRejectsNonMediaSourceAndHonorsUncheckedSave(t *testing.T) {
	a := &App{obsInputURLs: map[string]string{"new": "rtsp://host/new"}, obsInputOldURLs: map[string]string{"new": "rtsp://host/old"}, updateOBSInput: true}
	f := &fakeMediaURLWriter{found: true, media: false}
	a.updateOBSInputs(f, switcher.OBSConnected{Map: &scenemap.Map{Inputs: map[string]scenemap.Input{}}})
	if len(a.obsInputURLs) != 0 || a.obsInputWarning.Load().Code != "app.obs_url_missing" {
		t.Fatalf("non-media state: %#v %#v", a.obsInputURLs, a.obsInputWarning.Load())
	}
	if len(f.calls) != 1 {
		t.Fatalf("lookup calls = %v", f.calls)
	}

	a = &App{obsInputURLs: map[string]string{"old": "stale"}, updateOBSInput: true}
	(&pageBackend{app: a}).ApplySaved(nil, &config.Config{}, settings.ChangeSummary{}, false)
	if a.updateOBSInput || len(a.obsInputURLs) != 0 {
		t.Fatalf("unchecked save left marker: %#v", a.obsInputURLs)
	}
	f.calls = nil
	a.updateOBSInputs(f, switcher.OBSConnected{Map: &scenemap.Map{Inputs: map[string]scenemap.Input{"u": {CameraID: "old"}}}})
	if len(f.calls) != 0 {
		t.Fatalf("unchecked save wrote OBS: %v", f.calls)
	}
}

func TestOBSInputURLMarkerTracksLatestSavedURLInProcess(t *testing.T) {
	a := &App{}
	old := &config.Config{Cameras: []config.Camera{{ID: "cam1"}}}
	next := &config.Config{Cameras: []config.Camera{{ID: "cam1"}}}
	b := &pageBackend{app: a}
	b.ApplySaved(old, next, settings.ChangeSummary{}, true)
	if a.obsInputURLs["cam1"] == "" || len(a.obsInputURLs) != 1 {
		t.Fatalf("marker not set for new camera id: %#v", a.obsInputURLs)
	}
	first := a.obsInputURLs["cam1"]
	next.Stream.ObsReadHost = "obs.example"
	b.ApplySaved(next, next, settings.ChangeSummary{}, true)
	if a.obsInputURLs["cam1"] == "" || a.obsInputURLs["cam1"] == first {
		t.Fatalf("marker did not follow changed URL: %#v", a.obsInputURLs)
	}
}

func TestSettingsTransactionAndHandoff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configName)
	cfg, err := config.Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{opts: Options{BaseDir: dir}}
	b := &pageBackend{app: a, senderConfig: cfg}
	err = b.SettingsTransaction(func(s *web.SettingsStore) {
		out, e := settings.Rewrite(s.Raw, []settings.Change{{Path: "log.keep_days", Value: 3}})
		if e != nil {
			t.Fatal(e)
		}
		if e = s.Write(out); e != nil {
			t.Fatal(e)
		}
		next, e := config.Parse(out)
		if e != nil {
			t.Fatal(e)
		}
		s.ApplySaved(cfg, next, settings.ClassifyChanges(cfg, next, true), true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.pendingSettings == nil || !a.pendingSettings.UpdateOBSInput || a.pendingSettings.Next.Log.KeepDays != 3 || a.apply.State != ApplyApplying {
		t.Fatal("missing handoff")
	}
	if err = b.SettingsTransaction(func(s *web.SettingsStore) {
		if !s.Applying {
			t.Fatal("save must start applying")
		}
		raw, _ := os.ReadFile(path)
		if settings.Hash(raw) != settings.Hash(s.Raw) {
			t.Fatal("stale bytes")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsBackupOnlyCurrentGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configName)
	cfg, err := config.Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{opts: Options{BaseDir: dir}, hash: sha256.Sum256(raw)}
	b := &pageBackend{app: a, senderConfig: cfg}
	callBackup := func() error {
		return b.SettingsTransaction(func(s *web.SettingsStore) {
			if e := s.Backup(); e != nil {
				t.Error(e)
			}
		})
	}
	if err = callBackup(); err != nil {
		t.Fatal(err)
	}
	bak, _ := os.ReadFile(path + ".bak")
	if string(bak) != string(raw) {
		t.Fatal("current generation was not backed up")
	}
	external := append(append([]byte(nil), raw...), []byte("# external edit\n")...)
	if err = os.WriteFile(path, external, 0600); err != nil {
		t.Fatal(err)
	}
	if err = callBackup(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path + ".bak")
	if string(after) != string(bak) {
		t.Fatal("external edit replaced backup")
	}
}

func TestRestoreBackupSwapsFilesAndSchedulesRevert(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configName)
	old, err := config.Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	oldRaw, _ := os.ReadFile(path)
	next := *old
	next.Log.KeepDays++
	nextRaw, err := settings.Rewrite(oldRaw, []settings.Change{{Path: "log.keep_days", Value: next.Log.KeepDays}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, nextRaw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path+".bak", oldRaw, 0600); err != nil {
		t.Fatal(err)
	}
	a := &App{opts: Options{BaseDir: dir}, apply: Apply{State: ApplyFailed, Kind: ApplyRestart}, restarts: make(chan struct{}, 1)}
	if err = a.RestoreBackup(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	rotated, _ := os.ReadFile(path + ".bak")
	if string(got) != string(oldRaw) || string(rotated) != string(nextRaw) || a.apply.State != ApplyApplying || a.apply.Kind != ApplyRevert || len(a.restarts) != 1 {
		t.Fatal("backup was not swapped and restart scheduled")
	}
}

type settingsCore struct {
	coreRunner
	change core.SettingsChange
	err    error
}

func (c *settingsCore) ApplySettings(_ context.Context, v core.SettingsChange) error {
	c.change = v
	return c.err
}

type settingsOBS struct {
	obsRunner
	names []string
}

func (o *settingsOBS) UpdateScenes(v []string) { o.names = v }

type settingsLog struct {
	logHandle
	secrets []string
	days    int
}

func (l *settingsLog) AddSecrets(v ...string) { l.secrets = append(l.secrets, v...) }
func (l *settingsLog) SetKeepDays(v int)      { l.days = v }
func TestApplyLiveDeliveryAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			cfg := testConfig(t)
			cfg.GUI.CloseAction = "exit"
			cfg.Log.KeepDays = 3
			cfg.OBS.StandbyScene = "new-scene"
			c := &settingsCore{}
			if fail {
				c.err = errors.New("secret must not appear")
			}
			o, l := &settingsOBS{}, &settingsLog{}
			a := &App{auth: web.NewAuth(nil), apply: Apply{State: ApplyApplying}, opts: Options{BaseDir: t.TempDir()}}
			g := &generation{ctx: context.Background(), core: c, obs: o, logger: l}
			a.applyLive(g, &savedSettings{Next: cfg})
			if fail {
				if a.Apply().State != ApplyFailed || strings.Contains(a.Apply().Reason, "secret") {
					t.Fatal(a.Apply())
				}
				return
			}
			if a.Apply().State != ApplyApplied || a.windowClose != "exit" || l.days != 3 || len(l.secrets) != 1 || o.names[0] != "new-scene" {
				t.Fatal("届け先の不一致")
			}
			if c.change.Scenes.Standby != "new-scene" || len(c.change.Cameras) != len(cfg.Cameras) || len(c.change.URLs) != len(cfg.Cameras) {
				t.Fatal("core の反映が不足")
			}
		})
	}
}

func TestSavedSettingsRoutesAndBusy(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprint(restart), func(t *testing.T) {
			old := testConfig(t)
			next := *old
			next.Admin.Password = "changed-password"
			next.Log.KeepDays = 3
			if restart {
				next.Admin.Port++
			}
			a := &App{running: true, restarts: make(chan struct{}, 1)}
			b := &pageBackend{app: a}
			b.ApplySaved(old, &next, settings.ClassifyChanges(old, &next, true), true)
			want := ApplyKind("live")
			if restart {
				want = ApplyRestart
			}
			if a.apply.State != ApplyApplying || a.apply.Kind != want || !a.updateOBSInput {
				t.Fatal(a.apply)
			}
			if err := a.Restart(); err != ErrApplying {
				t.Fatal("反映中の再起動", err)
			}
			if len(a.restarts) != 0 {
				t.Fatal("202 より前に実行を開始した")
			}
		})
	}
}

func TestOBSURLRewriteOnlySharedIDs(t *testing.T) {
	for _, connected := range []bool{false, true} {
		t.Run(fmt.Sprint(connected), func(t *testing.T) {
			old := &config.Config{Cameras: []config.Camera{{ID: "cam1"}, {ID: "cam2"}, {ID: "cam3"}}}
			old.Stream.App = "before"
			old.MediaMTX.RTSPPort = 8554
			next := *old
			next.Stream.App = "after"
			next.Cameras = []config.Camera{{ID: "cam3"}, {ID: "cam4"}, {ID: "cam1"}}
			a := &App{}
			(&pageBackend{app: a}).ApplySaved(old, &next, settings.ClassifyChanges(old, &next, true), true)
			if len(a.obsInputURLs) != 2 || a.obsInputURLs["cam2"] != "" || a.obsInputURLs["cam4"] != "" {
				t.Fatalf("targets: %v", a.obsInputURLs)
			}
			for _, id := range []string{"cam1", "cam3"} {
				if !strings.HasSuffix(a.obsInputOldURLs[id], "/before/"+id) {
					t.Fatalf("old URLs: %v", a.obsInputOldURLs)
				}
			}
			m := &scenemap.Map{Inputs: map[string]scenemap.Input{"removed": {CameraID: "cam2"}, "one": {CameraID: "cam1"}, "three": {CameraID: "cam3"}, "new": {CameraID: "cam4"}}}
			f := &fakeMediaURLWriter{found: true, media: true}
			if connected {
				a.captureOBSInputUUIDs(&generation{obs: &cameraMapOBS{m: m}}, &savedSettings{Old: old, Next: &next})
			} else {
				m = &scenemap.Map{Inputs: map[string]scenemap.Input{}}
			}
			a.updateOBSInputs(f, switcher.OBSConnected{Map: m})
			if len(f.calls) != 2 {
				t.Fatalf("calls=%v", f.calls)
			}
			for _, call := range f.calls {
				if call[0] == "removed" || call[0] == "new" || strings.HasSuffix(call[1], "/cam2") || strings.HasSuffix(call[1], "/cam4") {
					t.Fatalf("calls=%v", f.calls)
				}
			}
		})
	}
}

type cameraMapOBS struct {
	obsRunner
	m *scenemap.Map
}

func (f *cameraMapOBS) ConnectedMap() (*scenemap.Map, bool) { return f.m, true }

func TestOBSURLRewriteWhenCameraCountChanges(t *testing.T) {
	old := &config.Config{Cameras: []config.Camera{{ID: "cam1"}, {ID: "cam2"}, {ID: "cam3"}}}
	old.Stream.App = "before"
	next := *old
	next.Stream.App = "after"
	next.Cameras = []config.Camera{{ID: "cam3"}}
	a := &App{}
	(&pageBackend{app: a}).ApplySaved(old, &next, settings.ClassifyChanges(old, &next, true), true)
	m := &scenemap.Map{Inputs: map[string]scenemap.Input{"deleted": {CameraID: "cam1"}, "retained": {CameraID: "cam3"}}}
	a.captureOBSInputUUIDs(&generation{obs: &cameraMapOBS{m: m}}, &savedSettings{Old: old, Next: &next})
	if len(a.obsInputUUIDs) != 1 || len(a.obsInputUUIDs["cam3"]) != 1 || a.obsInputUUIDs["cam3"][0] != "retained" {
		t.Fatalf("pinned=%v", a.obsInputUUIDs)
	}
	f := &fakeMediaURLWriter{}
	a.updateOBSInputs(f, switcher.OBSConnected{Map: m})
	if len(f.calls) != 1 || f.calls[0][0] != "retained" || !strings.HasSuffix(f.calls[0][1], "/after/cam3") {
		t.Fatalf("calls=%v", f.calls)
	}
}
