package settings

import (
	"net"
	"reflect"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/config"
)

func TestClassifyChanges(t *testing.T) {
	old, next := *testConfig(t), *testConfig(t)
	next.Switch.LowSec++
	next.Log.KeepDays++
	next.Admin.LAN = true
	next.Stream.ReadPass = "different1"
	got := ClassifyChanges(&old, &next, true)
	if got.Apply != ApplyRestart || !got.OBSURLChanges || got.AdminLANWillClose {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if len(got.Fields) != 4 {
		t.Fatalf("got %d fields", len(got.Fields))
	}
	old.Admin.LAN = true
	next.Admin.LAN = false
	got = ClassifyChanges(&old, &next, false)
	if !got.AdminLANWillClose || len(got.Warnings) != 1 || got.Warnings[0].Code != "settings.admin_lan_close" {
		t.Fatalf("missing LAN close warning: %+v", got)
	}
	old, next = *testConfig(t), *testConfig(t)
	next.MediaMTX.Binary = "new"
	got = ClassifyChanges(&old, &next, false)
	if !got.Fields[0].LocalOnly || got.Fields[0].Reason.Code != "settings.local_only" {
		t.Fatalf("missing local-only refusal: %+v", got)
	}
}

func TestClassifyLocalOnlyAndOBSURLFields(t *testing.T) {
	for _, path := range []string{"mediamtx.binary", "log.dir"} {
		old, next := testConfig(t), testConfig(t)
		if path == "mediamtx.binary" {
			next.MediaMTX.Binary = "custom"
		} else {
			next.Log.Dir = "other"
		}
		got := ClassifyChanges(old, next, false)
		if len(got.Fields) != 1 || !got.Fields[0].LocalOnly || got.Fields[0].Reason.Code != "settings.local_only" {
			t.Errorf("LAN change %s not refused: %+v", path, got)
		}
		got = ClassifyChanges(old, next, true)
		if len(got.Fields) != 1 || !got.Fields[0].LocalOnly || got.Fields[0].Reason.Code != "" {
			t.Errorf("local change %s marked refused: %+v", path, got)
		}
	}
	for _, tc := range []struct {
		path string
		set  func(*config.Config)
	}{
		{"stream.read_user", func(c *config.Config) { c.Stream.ReadUser += "x" }},
		{"stream.read_pass", func(c *config.Config) { c.Stream.ReadPass += "x" }},
		{"stream.obs_read_host", func(c *config.Config) { c.Stream.ObsReadHost = "localhost" }},
		{"mediamtx.rtsp_port", func(c *config.Config) { c.MediaMTX.RTSPPort++ }},
		{"stream.app", func(c *config.Config) { c.Stream.App += "x" }},
	} {
		old, next := testConfig(t), testConfig(t)
		tc.set(next)
		got := ClassifyChanges(old, next, true)
		if !got.OBSURLChanges {
			t.Errorf("OBS URL change missing for %s: %+v", tc.path, got)
		}
	}
	old, next := testConfig(t), testConfig(t)
	next.Cameras[0].Name += "x"
	if got := ClassifyChanges(old, next, true); got.OBSURLChanges {
		t.Fatal("camera display name incorrectly changes OBS URL")
	}
}

func TestEveryConfigFieldHasApplyClassification(t *testing.T) {
	paths := ConfigFieldPaths()
	for _, path := range paths {
		if _, ok := applyModes[path]; !ok {
			t.Errorf("Config field has no classification: %s", path)
		}
	}
	for path := range applyModes {
		found := false
		for _, configPath := range paths {
			if path == configPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("classification table has stale entry: %s", path)
		}
	}
	for _, p := range paths {
		if got := modeForPath(p); got != applyModes[p] {
			t.Errorf("%s: got %s, want %s", p, got, applyModes[p])
		}
	}
}

func TestCameraNameAppliesImmediately(t *testing.T) {
	old, next := testConfig(t), testConfig(t)
	next.Cameras[0].Name = "Outside"
	got := ClassifyChanges(old, next, true)
	if got.Apply != ApplyImmediate || len(got.Fields) != 1 || got.Fields[0].Path != "cameras[cam1].name" {
		t.Fatalf("camera name should apply immediately: %+v", got)
	}
}

func TestCheckPortsExclusionAndOccupied(t *testing.T) {
	next := *testConfig(t)
	next.MediaMTX.RTMPEnabled = false
	next.MediaMTX.SRTEnabled = false
	// Reserve a TCP port, then ask the checker to verify that port.
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("socket listen unavailable: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	next.Admin.Port = port
	// Excluding an exact current-generation endpoint permits it; a different
	// address does not, and therefore returns the item path while the socket is held.
	owned := []ListeningPort{{Protocol: "tcp", Address: ln.Addr().String()}, {Protocol: "tcp", Address: "0.0.0.0:8554"}, {Protocol: "tcp", Address: ":8554"}, {Protocol: "tcp", Address: "127.0.0.1:9997"}}
	if fails := CheckPorts(&next, nil, owned); len(fails) != 0 {
		t.Fatalf("exact owned endpoint was not excluded: %+v", fails)
	}
	ln.Close()
	ln, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("socket listen failed after a successful socket probe: %v", err)
	}
	defer ln.Close()
	port = ln.Addr().(*net.TCPAddr).Port
	next.Admin.Port = port
	fails := CheckPorts(&next, nil, ownedForBaseline())
	found := false
	for _, f := range fails {
		if f.Path == "admin.port" && f.Reason.Code == "settings.port_busy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("busy admin port did not produce path reason: %+v", fails)
	}
}

func TestPortEndpointEnumeration(t *testing.T) {
	c := testConfig(t)
	checks := makeChecks(c)
	want := map[string]int{
		"mediamtx.rtmp_port|tcp|tcp4|0.0.0.0:1935":  1,
		"mediamtx.rtmp_port|tcp|tcp|:1935":          1,
		"mediamtx.srt_port|udp|udp4|0.0.0.0:8890":   1,
		"mediamtx.srt_port|udp|udp|:8890":           1,
		"mediamtx.rtsp_port|tcp|tcp4|0.0.0.0:8554":  1,
		"mediamtx.rtsp_port|tcp|tcp|:8554":          1,
		"mediamtx.api_port|tcp|tcp4|127.0.0.1:9997": 1,
		"admin.port|tcp|tcp4|127.0.0.1:8787":        1,
	}
	got := make(map[string]int)
	for _, c := range checks {
		got[c.path+"|"+c.protocol+"|"+c.network+"|"+c.address]++
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("port endpoints mismatch: got %#v", got)
	}
	c.Admin.LAN = true
	checks = makeChecks(c)
	admin := 0
	for _, c := range checks {
		if c.path == "admin.port" {
			admin++
			if c.address != "0.0.0.0:8787" && c.address != ":8787" {
				t.Errorf("unexpected LAN admin endpoint %q", c.address)
			}
		}
	}
	if admin != 2 {
		t.Fatalf("LAN admin endpoints = %d, want 2", admin)
	}
}

func TestCheckPortsSameAdminPortSkipped(t *testing.T) {
	old, next := *testConfig(t), *testConfig(t)
	// An admin-only LAN toggle cannot bind to the same still-open listener.
	next.Admin.LAN = true
	fails := CheckPorts(&next, &old, ownedForBaseline())
	for _, f := range fails {
		if f.Path == "admin.port" {
			t.Fatalf("same admin.port was not excluded: %+v", fails)
		}
	}
}

func TestCheckPortsDoesNotExcludeDifferentProtocol(t *testing.T) {
	next := testConfig(t)
	next.MediaMTX.RTMPEnabled, next.MediaMTX.SRTEnabled = false, false
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("socket listen unavailable: %v", err)
	}
	defer ln.Close()
	next.Admin.Port = ln.Addr().(*net.TCPAddr).Port
	owned := ownedForBaseline()
	owned = append(owned, ListeningPort{Protocol: "udp", Address: ln.Addr().String()})
	fails := CheckPorts(next, nil, owned)
	for _, f := range fails {
		if f.Path == "admin.port" {
			return
		}
	}
	t.Fatalf("same address/port with different protocol was excluded: %+v", fails)
}

func ownedForBaseline() []ListeningPort {
	return []ListeningPort{{"tcp", "0.0.0.0:8554"}, {"tcp", ":8554"}, {"tcp", "127.0.0.1:9997"}}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	c, err := config.Parse([]byte("admin:\n  password: abcdefgh\nstream:\n  read_pass: abcdefgh\ncameras:\n  - id: cam1\n    publish_pass: publishpass\n    name: Main\n    on_bad: standby\n"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
