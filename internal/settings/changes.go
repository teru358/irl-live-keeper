package settings

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/i18n"
)

// ApplyMode は変更を反映する方法。
type ApplyMode string

const (
	ApplyImmediate ApplyMode = "immediate"
	ApplyRestart   ApplyMode = "restart"
)

// FieldChange は変更された項目とその扱いを表す。
type FieldChange struct {
	CameraName    string    `json:"camera_name,omitempty"`
	Kind          string    `json:"kind,omitempty"` // added / removed for camera membership
	Path          string    `json:"path"`
	Apply         ApplyMode `json:"apply"`
	LocalOnly     bool      `json:"local_only,omitempty"`
	Reason        i18n.Msg  `json:"reason,omitempty"`
	OBSURLChanges bool      `json:"obs_url_changes,omitempty"`
}

// ChangeSummary は設定差分に対する確認事項。
type ChangeSummary struct {
	Fields            []FieldChange `json:"fields"`
	Apply             ApplyMode     `json:"apply"`
	AdminLANWillClose bool          `json:"admin_lan_will_close,omitempty"`
	OBSURLChanges     bool          `json:"obs_url_changes"`
	Warnings          []i18n.Msg    `json:"warnings,omitempty"`
}

// applyModes is the explicit policy for every Config leaf. Missing paths
// intentionally fall back to restart in production; the reflection test
// ensures this table stays complete as Config evolves.
var applyModes = map[string]ApplyMode{
	"language":   ApplyRestart,
	"admin.port": ApplyRestart, "admin.lan": ApplyRestart, "admin.password": ApplyImmediate,
	"mediamtx.binary": ApplyRestart, "mediamtx.rtmp_port": ApplyRestart, "mediamtx.srt_port": ApplyRestart, "mediamtx.rtsp_port": ApplyRestart, "mediamtx.api_port": ApplyRestart, "mediamtx.rtmp_enabled": ApplyRestart, "mediamtx.srt_enabled": ApplyRestart,
	"stream.app": ApplyRestart, "stream.read_user": ApplyRestart, "stream.read_pass": ApplyRestart, "stream.obs_read_host": ApplyImmediate, "stream.publish_rtmp_port": ApplyImmediate, "stream.publish_srt_port": ApplyImmediate, "stream.publish_host": ApplyImmediate,
	"cameras[].publish_pass": ApplyRestart, "cameras[].id": ApplyRestart, "cameras[].name": ApplyImmediate, "cameras[].on_bad": ApplyImmediate, "cameras[].switch.fail_kbps": ApplyImmediate, "cameras[].switch.low_sec": ApplyImmediate, "cameras[].switch.recover_kbps": ApplyImmediate, "cameras[].switch.recover_sec": ApplyImmediate, "cameras[].switch.down_sec": ApplyImmediate,
	"obs.host": ApplyRestart, "obs.port": ApplyRestart, "obs.password": ApplyRestart, "obs.standby_scene": ApplyImmediate, "obs.scene_fallbacks[].scene": ApplyImmediate, "obs.scene_fallbacks[].fallback": ApplyImmediate, "obs.studio_mode_hide": ApplyImmediate,
	"on_stream_start.enabled": ApplyImmediate, "on_stream_start.scene": ApplyImmediate,
	"switch.auto_enabled": ApplyImmediate, "switch.fail_kbps": ApplyImmediate, "switch.low_sec": ApplyImmediate, "switch.recover_kbps": ApplyImmediate, "switch.recover_sec": ApplyImmediate, "switch.down_sec": ApplyImmediate,
	"log.dir": ApplyRestart, "log.keep_days": ApplyImmediate, "gui.close_action": ApplyImmediate,
}

var arrayIndex = regexp.MustCompile(`\[(?:\d+|cam[1-8])\]`)

// ClassifyChanges は設定差分と要求元 locality を評価する純粋関数。
func ClassifyChanges(old, next *config.Config, local bool) ChangeSummary {
	paths := make(map[string]struct{})
	collectDiff(reflect.ValueOf(old), reflect.ValueOf(next), "", paths)
	ordered := make([]string, 0, len(paths))
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	out := ChangeSummary{Apply: ApplyImmediate}
	for _, p := range ordered {
		mode := modeForPath(p)
		f := FieldChange{Path: p, Apply: mode}
		if strings.HasPrefix(p, "cameras[") {
			id, tail, _ := strings.Cut(strings.TrimPrefix(p, "cameras["), "]")
			if old != nil {
				for _, cam := range old.Cameras {
					if cam.ID == id {
						f.CameraName = cam.Name
					}
				}
			}
			if next != nil {
				for _, cam := range next.Cameras {
					if cam.ID == id {
						f.CameraName = cam.Name
					}
				}
			}
			if tail == "" {
				f.Kind = "added"
				if old != nil {
					for _, cam := range old.Cameras {
						if cam.ID == id {
							f.Kind = "removed"
							break
						}
					}
				}
			}
		}
		if p == "mediamtx.binary" || p == "log.dir" {
			f.LocalOnly = true
			if !local {
				f.Reason = i18n.New("settings.local_only")
			}
		}
		f.OBSURLChanges = obsURLPath(p)
		if f.OBSURLChanges {
			out.OBSURLChanges = true
		}
		if mode == ApplyRestart {
			out.Apply = ApplyRestart
		}
		out.Fields = append(out.Fields, f)
	}
	out.AdminLANWillClose = !local && old != nil && next != nil && old.Admin.LAN && !next.Admin.LAN
	if out.AdminLANWillClose {
		out.Warnings = append(out.Warnings, i18n.New("settings.admin_lan_close"))
	}
	return out
}

func modeForPath(p string) ApplyMode {
	// Fallback row count changes remain live scene settings. Camera membership
	// paths have no leaf mode and fall back to restart.
	if p == "obs.scene_fallbacks" {
		return ApplyImmediate
	}
	p = arrayIndex.ReplaceAllString(p, "[]")
	if mode, ok := applyModes[p]; ok {
		return mode
	}
	return ApplyRestart
}

func obsURLPath(p string) bool {
	return p == "stream.read_user" || p == "stream.read_pass" || p == "stream.app" || p == "stream.obs_read_host" || p == "mediamtx.rtsp_port"
}

func collectDiff(a, b reflect.Value, path string, out map[string]struct{}) {
	if !a.IsValid() || !b.IsValid() {
		if a.IsValid() != b.IsValid() {
			out[path] = struct{}{}
		}
		return
	}
	if a.Type() != b.Type() {
		out[path] = struct{}{}
		return
	}
	if a.Kind() == reflect.Pointer {
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				zero := reflect.Zero(a.Type().Elem())
				av, bv := zero, zero
				if !a.IsNil() {
					av = a.Elem()
				}
				if !b.IsNil() {
					bv = b.Elem()
				}
				collectDiff(av, bv, path, out)
			}
			return
		}
		collectDiff(a.Elem(), b.Elem(), path, out)
		return
	}
	switch a.Kind() {
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			name := strings.Split(a.Type().Field(i).Tag.Get("yaml"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			p := name
			if path != "" {
				p = path + "." + name
			}
			collectDiff(a.Field(i), b.Field(i), p, out)
		}
	case reflect.Slice:
		if path == "cameras" {
			old, next := make(map[string]reflect.Value), make(map[string]reflect.Value)
			for i := 0; i < a.Len(); i++ {
				old[a.Index(i).FieldByName("ID").String()] = a.Index(i)
			}
			for i := 0; i < b.Len(); i++ {
				next[b.Index(i).FieldByName("ID").String()] = b.Index(i)
			}
			for id, av := range old {
				p := fmt.Sprintf("cameras[%s]", id)
				if bv, ok := next[id]; ok {
					collectDiff(av, bv, p, out)
				} else {
					out[p] = struct{}{}
				}
			}
			for id := range next {
				if _, ok := old[id]; !ok {
					out[fmt.Sprintf("cameras[%s]", id)] = struct{}{}
				}
			}
			return
		}
		if a.Len() != b.Len() {
			out[path] = struct{}{}
			return // A whole-list replacement already includes every element change.
		}
		limit := a.Len()
		if b.Len() < limit {
			limit = b.Len()
		}
		for i := 0; i < limit; i++ {
			collectDiff(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i), out)
		}
	default:
		if !reflect.DeepEqual(a.Interface(), b.Interface()) {
			out[path] = struct{}{}
		}
	}
}

// ConfigFieldPaths は Config の全ての leaf の YAML path を返す。スライスは [] で表す。
func ConfigFieldPaths() []string {
	var paths []string
	walkType(reflect.TypeOf(config.Config{}), "", &paths)
	sort.Strings(paths)
	return paths
}

func walkType(t reflect.Type, path string, paths *[]string) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		*paths = append(*paths, path)
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		p := name
		if path != "" {
			p = path + "." + name
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Slice {
			ft = ft.Elem()
			p += "[]"
		}
		if ft.Kind() == reflect.Struct {
			walkType(ft, p, paths)
		} else {
			*paths = append(*paths, p)
		}
	}
}

// ConfigFieldMode 返回所有設定項目的區分，適合網頁或設定畫面顯示。
func ConfigFieldModes() map[string]ApplyMode {
	m := make(map[string]ApplyMode)
	for _, p := range ConfigFieldPaths() {
		m[p] = modeForPath(strings.ReplaceAll(p, "[*]", "[0]"))
	}
	return m
}
