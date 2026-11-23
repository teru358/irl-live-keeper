package gui

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/app"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"regexp"
	"strings"
)

type Row struct{ Text, Color string }
type Model struct {
	Rows           []Row
	Notice         string
	CanOpen, Retry bool
}

func Text(lang i18n.Lang, key string) string { return i18n.UI(lang)[key] }

var urls = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s]+`)

func safe(s string) string { return urls.ReplaceAllString(s, "***") }
func Build(s app.WindowState, dark bool) Model {
	t := func(k string) string { return Text(s.Language, k) }
	colors := []string{"#0F7F62", "#A86A00", "#C2352C", "#5B6773"}
	if dark {
		colors = []string{"#45C99C", "#F2B544", "#FF6F61", "#93A1B0"}
	}
	m := Model{CanOpen: s.Snapshot != nil, Retry: s.Apply.State == app.ApplyFailed}
	add := func(text string, c int) { m.Rows = append(m.Rows, Row{safe(text), colors[c]}) }
	if s.Snapshot == nil {
		for _, k := range []string{"links", "obs", "standby", "mediamtx"} {
			add(t(k)+"  —", 3)
		}
	} else {
		v := s.Snapshot
		for _, cam := range v.Cameras {
			c := 2
			if cam.State == "OK" {
				c = 0
			} else if cam.State == "LOW" {
				c = 1
			}
			protocol := ""
			if cam.Protocol != "" {
				protocol = " (" + strings.ToUpper(cam.Protocol) + ")"
			}
			add(fmt.Sprintf("%s %s  ● %s  %s kbps%s", t("links"), cam.Name, cam.State, groupedKbps(cam.Kbps), protocol), c)
		}
		c := 2
		state := t("offline")
		if v.OBS.Connected {
			c = 0
			state = t("connected")
		}
		stream := t("not_streaming")
		if v.OBS.Streaming {
			stream = t("streaming")
		}
		add(t("obs")+"  ● "+state+"  "+stream+"  "+v.OBS.Scene, c)
		owner := v.OBS.Standby.Owner
		if owner == "" {
			owner = "none"
		}
		line := t("standby") + "  " + t(owner)
		if v.OBS.Standby.ReturnScene != nil {
			line += " (" + *v.OBS.Standby.ReturnScene + ")"
		}
		add(line, 3)
		c = 2
		if v.MediaMTX == "running" {
			c = 0
		} else if v.MediaMTX == "starting" || v.MediaMTX == "restarting" {
			c = 1
		}
		add(t("mediamtx")+"  ● "+t(v.MediaMTX), c)
		if v.LastError != nil {
			m.Notice = safe(i18n.Render(s.Language, v.LastError.Message))
		}
	}
	if s.Generated && m.Notice == "" {
		m.Notice = t("window_generated")
	}
	if s.Apply.State == app.ApplyApplying {
		m.Notice = t("window_applying")
	} else if m.Retry {
		key := "window_start_failed"
		if s.Apply.Kind == app.ApplyRestart {
			key = "window_restart_failed"
		}
		m.Notice = t(key) + "\n" + safe(s.Apply.Reason)
	}
	return m
}
func CloseDecision(action string, streaming bool) (minimize, confirm bool) {
	return action == "minimize", action != "minimize" && action != "quit"
}
func ExitPrompt(lang i18n.Lang, streaming bool) string {
	v := Text(lang, "window_exit_confirm")
	if streaming {
		v += "\n" + Text(lang, "window_stream_warning")
	}
	return v
}

// groupedKbps は表示言語によらず3桁ごとに区切る。
func groupedKbps(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
