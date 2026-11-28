package i18n

import "testing"

func TestCameraName(t *testing.T) {
	for _, tc := range []struct {
		lang       Lang
		name, want string
	}{{JA, "", "カメラ 8"}, {EN, "", "Camera 8"}, {JA, "後方", "カメラ 8（後方）"}, {EN, "Rear", "Camera 8 (Rear)"}} {
		if got := CameraName(tc.lang, "cam8", tc.name); got != tc.want {
			t.Fatalf("%q want %q", got, tc.want)
		}
	}
}
