package i18n

import "strings"

// CameraName is the numbered camera heading, with an optional configured name.
func CameraName(lang Lang, id, name string) string {
	label := UI(lang)["settings_camera"] + " " + strings.TrimPrefix(id, "cam")
	if name != "" {
		if lang == JA {
			label += "（" + name + "）"
		} else {
			label += " (" + name + ")"
		}
	}
	return label
}
