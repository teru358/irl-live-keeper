package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gopkg.in/yaml.v3"
	"log/slog"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/settings"
)

type settingsTestBackend struct {
	fakeBackend
	mu     sync.Mutex
	store  SettingsStore
	writes int
}

func (b *settingsTestBackend) SettingsTransaction(fn func(*SettingsStore)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	fn(&b.store)
	return nil
}
func settingsFixture(t *testing.T) *settingsTestBackend {
	t.Helper()
	c, err := config.Generate(t.TempDir() + "/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// Use the generated file contents as the optimistic concurrency version.
	b := &settingsTestBackend{}
	data := configBytes(t, c)
	b.store = SettingsStore{Raw: data, Current: c, CheckPorts: func(*config.Config, *config.Config, []settings.ListeningPort) []settings.PortFailure { return nil }, Write: func(out []byte) error { b.writes++; b.store.Raw = out; return nil }, ApplySaved: func(*config.Config, *config.Config, settings.ChangeSummary, bool) {}}
	return b
}
func configBytes(t *testing.T, c *config.Config) []byte {
	t.Helper()
	data, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGetSettingsIncludesEveryApplyMode(t *testing.T) {
	b := settingsFixture(t)
	w := settingsCall(b, "GET", "/api/settings", "", "127.0.0.1:42", true)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var response struct {
		ApplyModes map[string]settings.ApplyMode `json:"apply_modes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	want := settings.ConfigFieldModes()
	for path, mode := range want {
		if got := response.ApplyModes[path]; got != mode {
			t.Errorf("apply_modes[%q] = %q, want %q", path, got, mode)
		}
	}
	if len(response.ApplyModes) != len(want) {
		t.Errorf("apply_modes has %d entries, want %d", len(response.ApplyModes), len(want))
	}
}

func settingsCall(b *settingsTestBackend, method, path, body, remote string, header bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = remote
	r.Host = "[::1]:8787"
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	if header {
		r.Header.Set(headerName, "1")
	}
	w := httptest.NewRecorder()
	apiHandler(b, nil).ServeHTTP(w, r)
	return w
}
func TestSettingsOrder(t *testing.T) {
	for _, tc := range []struct {
		name                                      string
		applying, hash, lan, invalid, port, write bool
		code                                      int
		key                                       string
	}{
		{name: "applying", applying: true, code: 409, key: "applying"},
		{name: "hash", lan: true, invalid: true, code: 409, key: "settings.hash_conflict"},
		{name: "local", hash: true, lan: true, invalid: true, code: 403, key: "settings.local_only"},
		{name: "validation", hash: true, invalid: true, port: true, code: 422, key: "fields"},
		{name: "ports", hash: true, port: true, code: 422, key: "admin.port"},
		{name: "write", hash: true, write: true, code: 500, key: "api.internal"},
		{name: "saved", hash: true, code: 202, key: "immediate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := settingsFixture(t)
			b.store.Applying = tc.applying
			h := "stale"
			if tc.hash {
				h = settings.Hash(b.store.Raw)
			}
			values := map[string]any{"log": map[string]any{"keep_days": 3}}
			if tc.lan {
				values["log"].(map[string]any)["dir"] = "elsewhere"
			}
			if tc.invalid {
				values["log"].(map[string]any)["keep_days"] = -1
			}
			if tc.port {
				b.store.CheckPorts = func(*config.Config, *config.Config, []settings.ListeningPort) []settings.PortFailure {
					return []settings.PortFailure{{Path: "admin.port", Reason: i18n.New("settings.port_missing")}}
				}
			}
			if tc.write {
				b.store.Write = func([]byte) error { return errors.New("secret") }
			}
			raw, _ := json.Marshal(map[string]any{"hash": h, "values": values})
			w := settingsCall(b, "PUT", "/api/settings", string(raw), "192.168.1.3:42", true)
			if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.key) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if w.Code != 202 && b.writes != 0 {
				t.Fatal("wrote on failure")
			}
		})
	}
}

func TestRegenerateSettings(t *testing.T) {
	for _, tc := range []struct {
		target string
		apply  settings.ApplyMode
		obs    bool
	}{
		{target: "admin", apply: settings.ApplyImmediate},
		{target: "publish", apply: settings.ApplyRestart},
		{target: "read", apply: settings.ApplyRestart, obs: true},
	} {
		t.Run(tc.target, func(t *testing.T) {
			b := settingsFixture(t)
			b.store.Current.Admin.Password = "existing-password"
			b.store.Raw = configBytes(t, b.store.Current)
			before := append([]byte(nil), b.store.Raw...)
			backupCalls := 0
			b.store.Backup = func() error { backupCalls++; return nil }
			var got settings.ChangeSummary
			var gotOBS bool
			b.store.ApplySaved = func(_, _ *config.Config, summary settings.ChangeSummary, updateOBS bool) {
				got, gotOBS = summary, updateOBS
			}
			body, _ := json.Marshal(map[string]any{"target": tc.target, "hash": settings.Hash(b.store.Raw), "update_obs_input": tc.obs})
			var logOutput bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logOutput, nil))
			r := httptest.NewRequest("POST", "/api/settings/regenerate", strings.NewReader(string(body)))
			r.RemoteAddr = "127.0.0.1:4"
			r.Header.Set("X-Forwarded-For", "127.0.0.1")
			r.Header.Set(headerName, "1")
			r = r.WithContext(context.WithValue(r.Context(), loggerKey{}, logger))
			w := httptest.NewRecorder()
			apiHandler(b, nil).ServeHTTP(w, r)
			if w.Code != 202 || strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "pass") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if backupCalls != 1 || string(before) == string(b.store.Raw) {
				t.Fatal("backup or rewrite missing")
			}
			if !strings.Contains(logOutput.String(), "settings saved") || !strings.Contains(logOutput.String(), "password regenerated") {
				t.Fatal("missing audit log")
			}
			cfg, err := config.Parse(b.store.Raw)
			if err != nil {
				t.Fatal(err)
			}
			var value string
			switch tc.target {
			case "admin":
				value = cfg.Admin.Password
			case "publish":
				value = cfg.Stream.PublishPass
			case "read":
				value = cfg.Stream.ReadPass
			}
			if !regexp.MustCompile(`^[A-Za-z0-9_-]{24}$`).MatchString(value) || cfg.Validate() != nil || got.Apply != tc.apply || gotOBS != tc.obs || strings.Contains(logOutput.String(), value) {
				t.Fatalf("generated value rejected or wrong apply: %q %+v", value, got)
			}
		})
	}
}

func TestRegenerateSettingsChecksConflictsAndUnsetAdmin(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		applying     bool
		stale        bool
		password     string
		code         int
	}{
		{name: "applying", target: "read", applying: true, code: 409},
		{name: "hash", target: "read", stale: true, code: 409},
		{name: "initial admin unset", target: "admin", code: 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := settingsFixture(t)
			b.store.Applying = tc.applying
			b.store.Current.Admin.Password = tc.password
			b.store.Raw = configBytes(t, b.store.Current)
			h := settings.Hash(b.store.Raw)
			if tc.stale {
				h = "stale"
			}
			body, _ := json.Marshal(map[string]any{"target": tc.target, "hash": h})
			w := settingsCall(b, "POST", "/api/settings/regenerate", string(body), "127.0.0.1:4", true)
			if w.Code != tc.code || b.writes != 0 {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestSettingsLocalAndCSRF(t *testing.T) {
	for _, remote := range []string{"127.0.0.2:42", "[::1]:42", "192.168.1.2:42"} {
		b := settingsFixture(t)
		w := settingsCall(b, "GET", "/api/settings", "", remote, true)
		var v map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		if w.Code != 200 || v["local"] != (remote != "192.168.1.2:42") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Body.String())
		}
	}
	b := settingsFixture(t)
	if w := settingsCall(b, "PUT", "/api/settings", "{}", "127.0.0.1:42", false); w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestSettingsAdminURL(t *testing.T) {
	for _, tc := range []struct {
		values, remote, want string
		present              bool
	}{
		{`{"admin":{"port":8788}}`, "127.0.0.1:4", "http://[::1]:8788", true},
		{`{"admin":{"lan":true}}`, "127.0.0.1:4", "http://[::1]:8787", true},
		{`{"admin":{"lan":false}}`, "192.168.1.2:4", "", true},
		{`{"log":{"keep_days":3}}`, "127.0.0.1:4", "", false},
	} {
		b := settingsFixture(t)
		if strings.Contains(tc.remote, "192.") {
			b.store.Current.Admin.LAN = true
			b.store.Raw = configBytes(t, b.store.Current)
		}
		body := `{"hash":"` + settings.Hash(b.store.Raw) + `","values":` + tc.values + `}`
		w := settingsCall(b, "PUT", "/api/settings", body, tc.remote, true)
		var v map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		u, present := v["admin_url"]
		if w.Code != 202 || present != tc.present || (tc.want != "" && u != tc.want) || (tc.want == "" && u != nil) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}
func TestSettingsConcurrent(t *testing.T) {
	b := settingsFixture(t)
	body := `{"hash":"` + settings.Hash(b.store.Raw) + `","values":{"log":{"keep_days":3}}}`
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- settingsCall(b, "PUT", "/api/settings", body, "127.0.0.1:4", true).Code
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for c := range codes {
		counts[c]++
	}
	if counts[202] != 1 || counts[409] != 1 || b.writes != 1 {
		t.Fatal(counts)
	}
}

func TestSettingsPreviewAndBackup(t *testing.T) {
	b := settingsFixture(t)
	b.store.BackupRaw = append([]byte(nil), b.store.Raw...)
	w := settingsCall(b, "POST", "/api/settings/preview", `{"values":{"log":{"dir":"new"}}}`, "192.168.1.2:4", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"local_only":true`) || b.writes != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = settingsCall(b, "POST", "/api/settings/preview", `{"revert":true}`, "127.0.0.1:4", true)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	b.store.Backup = func() error { return errors.New("backup failed") }
	w = settingsCall(b, "PUT", "/api/settings", `{"hash":"`+settings.Hash(b.store.Raw)+`","values":{"log":{"keep_days":3}}}`, "127.0.0.1:4", true)
	if w.Code != 500 || b.writes != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSettingsRevertAndPreview(t *testing.T) {
	b := settingsFixture(t)
	previous := append([]byte(nil), b.store.Raw...)
	current := *b.store.Current
	current.Log.KeepDays++
	b.store.Current = &current
	b.store.Raw = configBytes(t, &current)
	b.store.BackupRaw = previous
	b.store.Backup = func() error { b.store.BackupRaw = append([]byte(nil), b.store.Raw...); return nil }
	w := settingsCall(b, "POST", "/api/settings/preview", `{"revert":true}`, "127.0.0.1:4", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "log.keep_days") || b.writes != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = settingsCall(b, "POST", "/api/settings/revert", `{}`, "127.0.0.1:4", true)
	if w.Code != 202 || !strings.Contains(w.Body.String(), `"kind":"revert"`) || b.writes != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	got, err := config.Parse(b.store.Raw)
	want, _ := config.Parse(previous)
	if err != nil || got.Log.KeepDays != want.Log.KeepDays {
		t.Fatal(err, got.Log.KeepDays, want.Log.KeepDays)
	}
	// Since Backup rotates on every write, a second revert restores the value that was just replaced.
	w = settingsCall(b, "POST", "/api/settings/revert", `{}`, "127.0.0.1:4", true)
	if w.Code != 202 || b.writes != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSettingsRevertConflicts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		backup   bool
		applying bool
		want     int
	}{
		{"missing", false, false, 409}, {"no-difference", true, false, 409}, {"applying", true, true, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := settingsFixture(t)
			b.store.Applying = tc.applying
			if tc.backup {
				b.store.BackupRaw = append([]byte(nil), b.store.Raw...)
			}
			w := settingsCall(b, "POST", "/api/settings/revert", `{}`, "127.0.0.1:4", true)
			if w.Code != tc.want || b.writes != 0 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestSettingsFieldsAndPartialUpdate(t *testing.T) {
	b := settingsFixture(t)
	w := settingsCall(b, "PUT", "/api/settings", `{"hash":"`+settings.Hash(b.store.Raw)+`","values":{"admin":{"password":"short"},"log":{"keep_days":-1}}}`, "127.0.0.1:4", true)
	var failure struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if w.Code != 422 || failure.Error == "" || failure.Fields["admin.password"] == "" || failure.Fields["log.keep_days"] == "" || strings.Contains(w.Body.String(), "short") {
		t.Fatal(w.Code, w.Body.String())
	}
	b.store.Current.Log.Dir = "custom-logs"
	b.store.Raw = configBytes(t, b.store.Current)
	w = settingsCall(b, "PUT", "/api/settings", `{"hash":"`+settings.Hash(b.store.Raw)+`","values":{"log":{"keep_days":3}}}`, "127.0.0.1:4", true)
	cfg, err := config.Parse(b.store.Raw)
	if w.Code != 202 || err != nil || cfg.Log.Dir != "custom-logs" || cfg.Log.KeepDays != 3 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
}

func TestSettingsFullValuesPreserveFile(t *testing.T) {
	for _, path := range []string{"log.keep_days", "cameras[1].name", "none"} {
		t.Run(path, func(t *testing.T) {
			b := settingsFixture(t)
			c := b.store.Current
			second := c.Cameras[0]
			second.ID, second.Name = "cam2", "Second"
			c.Cameras = append(c.Cameras, second)
			original := "# user config\n" + string(configBytes(t, c)) + "# end\n"
			original = strings.Replace(original, "name: Second", "name: Second # camera comment", 1)
			b.store.Raw = []byte(original)
			w := settingsCall(b, "GET", "/api/settings", "", "127.0.0.1:4", true)
			var req struct {
				Values map[string]any `json:"values"`
				Hash   string         `json:"hash"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &req); err != nil {
				t.Fatal(err)
			}
			want := original
			switch path {
			case "log.keep_days":
				req.Values["log"].(map[string]any)["keep_days"] = 3
				// Use the actual generated default rather than assuming it.
				want = strings.Replace(original, "keep_days: "+strconv.Itoa(c.Log.KeepDays), "keep_days: 3", 1)
			case "cameras[1].name":
				req.Values["cameras"].([]any)[1].(map[string]any)["name"] = "Renamed"
				start, end := strings.Index(original, "cameras:\n"), strings.Index(original, "obs:\n")
				want = original[:start] + "cameras:\n  - id: cam1\n    name: メインカメラ\n    on_bad: standby\n  - id: cam2\n    name: Renamed\n    on_bad: standby\n" + original[end:]
			case "none":
				b.store.CheckPorts = func(*config.Config, *config.Config, []settings.ListeningPort) []settings.PortFailure {
					t.Fatal("no-op checked ports")
					return nil
				}
				b.store.Backup = func() error { t.Fatal("no-op backed up"); return nil }
				b.store.ApplySaved = func(*config.Config, *config.Config, settings.ChangeSummary, bool) { t.Fatal("no-op applied") }
			}
			body, _ := json.Marshal(req)
			w = settingsCall(b, "PUT", "/api/settings", string(body), "127.0.0.1:4", true)
			if w.Code != 202 || string(b.store.Raw) != want {
				t.Fatalf("status=%d body=%s\ngot:\n%s\nwant:\n%s", w.Code, w.Body.String(), b.store.Raw, want)
			}
			if path == "none" && (b.writes != 0 || !strings.Contains(w.Body.String(), `"apply":"none"`)) {
				t.Fatal("no-op wrote", w.Body.String())
			}
		})
	}
}

func TestSettingsChangesListLength(t *testing.T) {
	b := settingsFixture(t)
	old := *b.store.Current
	next := old
	second := old.Cameras[0]
	second.ID = "cam2"
	next.Cameras = []config.Camera{second, old.Cameras[0]}
	changes := settingsChanges(&next, settings.ClassifyChanges(&old, &next, true))
	if len(changes) != 1 || changes[0].Path != "cameras" {
		t.Fatalf("%+v", changes)
	}
}

func TestRevertAuditOnly(t *testing.T) {
	b := settingsFixture(t)
	backup := *b.store.Current
	backup.Log.KeepDays++
	b.store.BackupRaw = configBytes(t, &backup)
	var output bytes.Buffer
	r := httptest.NewRequest("POST", "/api/settings/revert", strings.NewReader("{}"))
	r.RemoteAddr = "127.0.0.1:4"
	r.Header.Set(headerName, "1")
	r = r.WithContext(context.WithValue(r.Context(), loggerKey{}, slog.New(slog.NewTextHandler(&output, nil))))
	w := httptest.NewRecorder()
	apiHandler(b, nil).ServeHTTP(w, r)
	if w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(output.String(), "settings reverted") || strings.Contains(output.String(), "settings saved") {
		t.Fatal(output.String())
	}
}

func TestSettingsCameraIDPathsAndOrder(t *testing.T) {
	b := settingsFixture(t)
	// The third camera is first in the draft. A 422 (name over 32 chars) must address its ID.
	body := `{"values":{"cameras":[{"id":"cam3","name":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx","on_bad":"hide"},{"id":"cam1","name":"One","on_bad":"standby"}]}}`
	w := settingsCall(b, "POST", "/api/settings/preview", body, "127.0.0.1:4", true)
	if w.Code != 422 || !strings.Contains(w.Body.String(), `"cameras[cam3].name"`) || strings.Contains(w.Body.String(), "cameras[0]") {
		t.Fatal(w.Code, w.Body.String())
	}
	body = strings.Replace(body, `"name":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"`, `"name":"Three"`, 1)
	w = settingsCall(b, "POST", "/api/settings/preview", body, "127.0.0.1:4", true)
	var preview settings.ChangeSummary
	if json.Unmarshal(w.Body.Bytes(), &preview) != nil || w.Code != 200 || preview.Apply != settings.ApplyRestart || preview.OBSURLChanges {
		t.Fatal(w.Code, w.Body.String())
	}
	body = strings.Replace(body, `{"values":`, `{"hash":"`+settings.Hash(b.store.Raw)+`","values":`, 1)
	w = settingsCall(b, "PUT", "/api/settings", body, "127.0.0.1:4", true)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Index(string(b.store.Raw), "cam1") > strings.Index(string(b.store.Raw), "cam3") {
		t.Fatal("saved out of order")
	}
	// Removing cam1 leaves cam3's URL unchanged and requires only a restart.
	body = `{"values":{"cameras":[{"id":"cam3","name":"Three","on_bad":"hide"}]}}`
	w = settingsCall(b, "POST", "/api/settings/preview", body, "127.0.0.1:4", true)
	if json.Unmarshal(w.Body.Bytes(), &preview) != nil || w.Code != 200 || preview.OBSURLChanges {
		t.Fatal(w.Code, w.Body.String())
	}
}
