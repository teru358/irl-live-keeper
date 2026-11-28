package settings

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/config"
)

// FieldLocation is the single owner of a configuration leaf.
type FieldLocation struct {
	Page  string `json:"page"`
	Group string `json:"group"`
}

var fieldLocations = map[string]FieldLocation{
	"language":                       {"settings", "other"},
	"admin.port":                     {"settings", "admin"},
	"admin.lan":                      {"settings", "admin"},
	"admin.password":                 {"settings", "admin"},
	"mediamtx.binary":                {"settings", "admin"},
	"mediamtx.rtmp_port":             {"settings", "admin"},
	"mediamtx.srt_port":              {"settings", "admin"},
	"mediamtx.rtsp_port":             {"settings", "admin"},
	"mediamtx.api_port":              {"settings", "admin"},
	"mediamtx.rtmp_enabled":          {"cameras", "common"},
	"mediamtx.srt_enabled":           {"cameras", "common"},
	"stream.app":                     {"cameras", "common"},
	"stream.read_user":               {"settings", "stream"},
	"stream.read_pass":               {"settings", "stream"},
	"stream.obs_read_host":           {"settings", "stream"},
	"stream.publish_rtmp_port":       {"cameras", "common"},
	"stream.publish_srt_port":        {"cameras", "common"},
	"stream.publish_host":            {"cameras", "common"},
	"cameras[].publish_pass":         {"cameras", "camera"},
	"cameras[].id":                   {"cameras", "camera"},
	"cameras[].name":                 {"cameras", "camera"},
	"cameras[].on_bad":               {"cameras", "camera"},
	"cameras[].switch.fail_kbps":     {"cameras", "camera"},
	"cameras[].switch.low_sec":       {"cameras", "camera"},
	"cameras[].switch.recover_kbps":  {"cameras", "camera"},
	"cameras[].switch.recover_sec":   {"cameras", "camera"},
	"cameras[].switch.down_sec":      {"cameras", "camera"},
	"obs.host":                       {"settings", "obs"},
	"obs.port":                       {"settings", "obs"},
	"obs.password":                   {"settings", "obs"},
	"obs.standby_scene":              {"settings", "obs"},
	"obs.scene_fallbacks[].scene":    {"settings", "obs"},
	"obs.scene_fallbacks[].fallback": {"settings", "obs"},
	"obs.studio_mode_hide":           {"settings", "obs"},
	"on_stream_start.enabled":        {"settings", "obs"},
	"on_stream_start.scene":          {"settings", "obs"},
	"switch.auto_enabled":            {"cameras", "common"},
	"switch.fail_kbps":               {"cameras", "common"},
	"switch.low_sec":                 {"cameras", "common"},
	"switch.recover_kbps":            {"cameras", "common"},
	"switch.recover_sec":             {"cameras", "common"},
	"switch.down_sec":                {"cameras", "common"},
	"log.dir":                        {"settings", "other"},
	"log.keep_days":                  {"settings", "other"},
	"gui.close_action":               {"settings", "other"},
}

func ConfigFieldLocations() map[string]FieldLocation {
	out := make(map[string]FieldLocation, len(fieldLocations))
	for p, l := range fieldLocations {
		out[p] = l
	}
	return out
}

// PageHash hashes typed, defaulted Config values. YAML spelling and comments do
// not participate. Camera membership and values are ordered by immutable ID.
func PageHash(cfg *config.Config, page string) string {
	paths := ConfigFieldPaths()
	rows := make([]any, 0)
	for _, p := range paths {
		if fieldLocations[p].Page == page {
			rows = append(rows, []any{p, pageValue(reflect.ValueOf(cfg), strings.Split(p, "."))})
		}
	}
	raw, _ := json.Marshal(rows)
	return Hash(raw)
}
func pageValue(v reflect.Value, parts []string) any {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			if len(parts) == 0 {
				return nil
			}
			v = reflect.Zero(v.Type().Elem())
		} else {
			v = v.Elem()
		}
	}
	if len(parts) == 0 {
		return v.Interface()
	}
	key := strings.TrimSuffix(parts[0], "[]")
	for i := 0; i < v.NumField(); i++ {
		if strings.Split(v.Type().Field(i).Tag.Get("yaml"), ",")[0] != key {
			continue
		}
		f := v.Field(i)
		if strings.HasSuffix(parts[0], "[]") {
			indexes := make([]int, f.Len())
			for j := range indexes {
				indexes[j] = j
			}
			if key == "cameras" {
				sort.Slice(indexes, func(a, b int) bool {
					return f.Index(indexes[a]).FieldByName("ID").String() < f.Index(indexes[b]).FieldByName("ID").String()
				})
			}
			out := make([]any, 0, len(indexes))
			for _, j := range indexes {
				out = append(out, pageValue(f.Index(j), parts[1:]))
			}
			return out
		}
		return pageValue(f, parts[1:])
	}
	return nil
}

// PageOwnsValues rejects unknown leaves, including empty foreign containers.
func PageOwnsValues(page string, values map[string]any) bool {
	if page != "cameras" && page != "settings" {
		return false
	}
	var walk func(string, any) bool
	walk = func(path string, value any) bool {
		if m, ok := value.(map[string]any); ok {
			if len(m) == 0 {
				for p, l := range fieldLocations {
					if l.Page == page && strings.HasPrefix(p, path+".") {
						return true
					}
				}
				return false
			}
			for k, v := range m {
				p := k
				if path != "" {
					p = path + "." + k
				}
				if !walk(p, v) {
					return false
				}
			}
			return true
		}
		if a, ok := value.([]any); ok {
			prefix := path + "[]"
			owned := false
			for p, l := range fieldLocations {
				if strings.HasPrefix(p, prefix+".") {
					if l.Page != page {
						return false
					}
					owned = true
				}
			}
			if !owned {
				return false
			}
			for _, v := range a {
				if !walk(prefix, v) {
					return false
				}
			}
			return true
		}
		if value == nil {
			owned := false
			for p, l := range fieldLocations {
				if strings.HasPrefix(p, path+".") {
					if l.Page != page {
						return false
					}
					owned = true
				}
			}
			if owned {
				return true
			}
		}
		l, ok := fieldLocations[path]
		return ok && l.Page == page
	}
	return values != nil && walk("", values)
}
