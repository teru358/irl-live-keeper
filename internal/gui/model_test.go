package gui

import (
	"github.com/teru358/irl-live-keeper/internal/app"
	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"strings"
	"testing"
)

func TestModel(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
		for _, dark := range []bool{false, true} {
			s := app.WindowState{Language: lang}
			m := Build(s, dark)
			if m.CanOpen || len(m.Rows) != 4 {
				t.Fatal(m)
			}
			for _, r := range m.Rows {
				if !strings.Contains(r.Text, "—") {
					t.Fatal(r)
				}
			}
			for n := 1; n <= 3; n++ {
				v := core.Snapshot{MediaMTX: "running"}
				for i := 0; i < n; i++ {
					v.Cameras = append(v.Cameras, core.CameraStatus{Name: "camera", State: []string{"OK", "LOW", "DOWN"}[i]})
				}
				s.Snapshot = &v
				m = Build(s, dark)
				if len(m.Rows) != n+3 || !m.CanOpen {
					t.Fatal(m)
				}
				for i := 0; i < n; i++ {
					if i > 0 && m.Rows[i].Color == m.Rows[i-1].Color {
						t.Fatal(m)
					}
				}
			}
			s.Apply = app.Apply{State: app.ApplyApplying}
			if Build(s, dark).Notice != Text(lang, "window_applying") {
				t.Fatal("applying")
			}
			s.Apply = app.Apply{State: app.ApplyFailed, Reason: "bad http://user:password@host/path"}
			m = Build(s, dark)
			if !m.Retry || strings.Contains(m.Notice, "password") {
				t.Fatal(m)
			}
			windowNotice := NoticeForWindow(m)
			if !strings.Contains(windowNotice, "\r\n") || strings.Contains(strings.ReplaceAll(windowNotice, "\r\n", ""), "\n") {
				t.Fatalf("窓用の案内文が CRLF ではない: %q", windowNotice)
			}
			s.Apply = app.Apply{}
			s.Snapshot.LastError = &core.LastError{Message: i18n.New("console.logger_open", "error", "http://secret@host")}
			if strings.Contains(Build(s, dark).Notice, "secret") {
				t.Fatal("secret")
			}
		}
	}
}
func TestClose(t *testing.T) {
	if !ExitConfirmation() {
		t.Fatal("終了ボタンは常に確認")
	}
	for _, v := range []struct {
		a    string
		m, c bool
	}{{"minimize", true, false}, {"confirm", false, true}, {"quit", false, false}} {
		m, c := CloseDecision(v.a, true)
		if m != v.m || c != v.c {
			t.Fatal(v)
		}
	}
	for _, l := range []i18n.Lang{i18n.JA, i18n.EN} {
		if ExitPrompt(l, true) == ExitPrompt(l, false) {
			t.Fatal(l)
		}
	}
}
func TestServiceRows(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, connected := range []bool{false, true} {
			for _, media := range []string{"running", "starting", "restarting", "stopped"} {
				for _, owner := range []string{"none", "auto", "manual"} {
					target := "Camera"
					s := app.WindowState{Language: i18n.JA, Snapshot: &core.Snapshot{MediaMTX: media, OBS: core.OBSStatus{Connected: connected, Streaming: true, Standby: core.StandbyStatus{Owner: owner, ReturnScene: &target}}}}
					m := Build(s, dark)
					if len(m.Rows) != 3 || strings.Contains(m.Rows[0].Text, "—") {
						t.Fatal(m)
					}
					if connected == (m.Rows[0].Color == m.Rows[2].Color) && media == "stopped" {
						t.Fatal(m)
					}
				}
			}
		}
	}
	for _, l := range []i18n.Lang{i18n.JA, i18n.EN} {
		m := Build(app.WindowState{Language: l, Generated: true}, false)
		if m.Notice != Text(l, "window_generated") {
			t.Fatal(m)
		}
	}
}

func TestCameraDisplay(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.JA, i18n.EN} {
		for _, dark := range []bool{false, true} {
			s := app.WindowState{Language: lang, Snapshot: &core.Snapshot{Cameras: []core.CameraStatus{{Name: "メインカメラ", ID: "main", State: "DOWN"}, {Name: "sub", State: "OK", Kbps: 2450, Protocol: "srt"}}}}
			m := Build(s, dark)
			if !strings.Contains(m.Rows[0].Text, "メインカメラ (main)") || strings.Contains(m.Rows[0].Text, "()") || !strings.Contains(m.Rows[1].Text, "2,450 kbps (SRT)") {
				t.Fatal(m.Rows)
			}
			for _, kind := range []app.ApplyKind{app.ApplyRestart, ""} {
				s.Apply = app.Apply{State: app.ApplyFailed, Kind: kind}
				key := "window_start_failed"
				if kind == app.ApplyRestart {
					key = "window_restart_failed"
				}
				if !strings.HasPrefix(Build(s, dark).Notice, Text(lang, key)) {
					t.Fatal(Build(s, dark).Notice)
				}
			}
		}
	}
}

func TestRestoreButtonOnlyForFailedStartupOrRestartWithBackup(t *testing.T) {
	for _, tc := range []struct {
		apply        app.Apply
		backup, want bool
	}{
		{app.Apply{State: app.ApplyFailed}, true, true},
		{app.Apply{State: app.ApplyFailed, Kind: app.ApplyRestart}, true, true},
		{app.Apply{State: app.ApplyFailed, Kind: "live"}, true, false},
		{app.Apply{State: app.ApplyApplying, Kind: app.ApplyRestart}, true, false},
		{app.Apply{State: app.ApplyFailed}, false, false},
	} {
		m := Build(app.WindowState{Apply: tc.apply, Restore: tc.backup}, false)
		if m.Restore != tc.want {
			t.Fatalf("%+v: Restore=%v", tc, m.Restore)
		}
	}
}
