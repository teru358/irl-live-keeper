package gui

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/app"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"regexp"
	"strings"
)

func noticeAreaHeight(textHeight, minimum, padding int) int {
	if textHeight <= 0 {
		return 0
	}
	return max(minimum, textHeight+padding)
}

func windowHeight(top, noticeHeight, padding, frame, minimum int) int {
	return max(minimum, top+noticeHeight+padding+frame)
}

type windowLayout struct {
	Width, Left, RowHeight, RowStep                                   int
	ButtonTop, ButtonStep, ButtonHeight                               int
	NoticeTop, NoticeWidth, NoticeTextWidth, NoticeMinHeight, Padding int
	MinimumHeight                                                     int
}

// layoutForDPI contains only logical geometry, so the layout can be checked
// without creating Windows controls.
func layoutForDPI(dpi, rows, visibleButtons int) (l windowLayout) {
	s := func(v int) int { return scaleDPI(v, dpi) }
	l = windowLayout{
		Width: s(540), Left: s(12), RowHeight: s(36), RowStep: s(40),
		ButtonTop: s(16 + rows*40), ButtonStep: s(43), ButtonHeight: s(38),
		NoticeWidth: s(480), NoticeTextWidth: s(456),
		NoticeMinHeight: s(100), Padding: s(12), MinimumHeight: s(431),
	}
	// Buttons occupy consecutive rows; hidden actions must not reserve space.
	l.NoticeTop = l.ButtonTop + max(0, visibleButtons-1)*l.ButtonStep + l.ButtonHeight + s(8)
	return
}

func layoutWindowHeight(l windowLayout, rows, visibleButtons, noticeHeight, frame int) int {
	rowsBottom := l.Left + max(0, rows-1)*l.RowStep + l.RowHeight
	buttonsBottom := l.ButtonTop + max(0, visibleButtons-1)*l.ButtonStep + l.ButtonHeight
	noticeBottom := 0
	if noticeHeight > 0 {
		noticeBottom = l.NoticeTop + noticeHeight
	}
	contentBottom := max(rowsBottom, max(buttonsBottom, noticeBottom))
	return contentBottom + l.Padding + frame
}

func scaleDPI(value, dpi int) int {
	if dpi <= 0 {
		dpi = 96
	}
	return (value*dpi + 48) / 96
}

type Row struct{ Text, Color string }
type Model struct {
	Rows                    []Row
	Notice                  string
	CanOpen, Retry, Restore bool
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
	restoreFailure := s.Apply.Kind == "" || s.Apply.Kind == app.ApplyRestart
	m := Model{CanOpen: s.Snapshot != nil, Retry: s.Apply.State == app.ApplyFailed, Restore: s.Apply.State == app.ApplyFailed && restoreFailure && s.Restore}
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
			add(fmt.Sprintf("%s %s  ● %s  %s kbps%s", t("links"), i18n.CameraName(s.Language, cam.ID, cam.Name), cam.State, groupedKbps(cam.Kbps), protocol), c)
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
	if s.Snapshot != nil {
		for _, warning := range s.Snapshot.OBS.Warnings {
			if warning.Code == "mediamtx.missing" {
				if m.Notice != "" {
					m.Notice += "\n"
				}
				m.Notice += safe(i18n.Render(s.Language, warning))
			}
		}
	}
	if s.PasswordUnset {
		m.Notice = t("window_password_unset") + "\n" + m.Notice
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
	if m.Notice != "" {
		seen := make(map[string]bool)
		lines := strings.Split(m.Notice, "\n")
		unique := lines[:0]
		for _, line := range lines {
			if !seen[line] {
				seen[line] = true
				unique = append(unique, line)
			}
		}
		m.Notice = strings.Join(unique, "\n")
	}
	return m
}

// NoticeForWindow converts model line breaks to the CRLF form required by Win32 controls.
func NoticeForWindow(m Model) string { return strings.ReplaceAll(m.Notice, "\n", "\r\n") }

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

// ExitConfirmation は「×」の設定と独立して、終了ボタンで必ず確認する。
func ExitConfirmation() bool { return true }
