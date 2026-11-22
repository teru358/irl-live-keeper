package scenemap

import "testing"

func TestMatchCamera(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		local *bool
		port  int
		want  string
	}{
		{"normal", "rtsp://127.0.0.1:8554/main", boolPtr(false), 8554, "main"},
		{"lan host credentials", "rtsp://u:p@192.168.1.8:8554/main/", boolPtr(false), 8554, "main"},
		{"hostname", "rtsp://camera.local:8554//main//", boolPtr(false), 8554, "main"},
		{"default port", "rtsp://camera/main", boolPtr(false), 554, "main"},
		{"port omitted uses default", "rtsp://camera/main", boolPtr(false), 8554, ""},
		{"wrong port", "rtsp://camera:554/main", boolPtr(false), 8554, ""},
		{"rtmp", "rtmp://camera/main", boolPtr(false), 554, ""},
		{"srt", "srt://camera/main", boolPtr(false), 554, ""},
		{"no scheme", "//camera/main", boolPtr(false), 554, ""},
		{"similar path", "rtsp://camera/main2", boolPtr(false), 554, ""},
		{"nested path", "rtsp://camera/a/main", boolPtr(false), 554, ""},
		{"empty path", "rtsp://camera", boolPtr(false), 554, ""},
		{"nil local", "rtsp://camera/main", nil, 554, ""},
		{"local", "rtsp://camera/main", boolPtr(true), 554, ""},
		{"malformed", "http://[::1", boolPtr(false), 554, ""},
		{"case sensitive", "rtsp://camera/Main", boolPtr(false), 554, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MatchCamera(tt.raw, tt.local, tt.port, []CameraPath{{ID: "main", Path: "main"}})
			if ok != (tt.want != "") || got != tt.want {
				t.Fatalf("MatchCamera() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.want != "")
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

func TestCamerasInTraversesVisiblePaths(t *testing.T) {
	m := Map{
		Scenes: map[string]Container{
			"S":    {Items: []Item{{ID: 1, SourceUUID: "cam1", SourceKind: InputSource, Enabled: false}, {ID: 2, SourceUUID: "N", SourceKind: SceneSource, Enabled: true}, {ID: 3, SourceUUID: "G", SourceKind: GroupSource, Enabled: true}, {ID: 4, SourceUUID: "N", SourceKind: SceneSource, Enabled: false}, {ID: 5, SourceUUID: "S", SourceKind: SceneSource, Enabled: true}}},
			"N":    {Items: []Item{{ID: 1, SourceUUID: "cam2", SourceKind: InputSource, Enabled: false}, {ID: 2, SourceUUID: "deep", SourceKind: SceneSource, Enabled: true}}},
			"deep": {Items: []Item{{ID: 1, SourceUUID: "cam3", SourceKind: InputSource, Enabled: true}}},
		},
		Groups: map[string]Container{
			"G": {Items: []Item{{ID: 1, SourceUUID: "cam4", SourceKind: InputSource, Enabled: true}, {ID: 2, SourceUUID: "cam5", SourceKind: InputSource, Enabled: false}}},
		},
		Inputs: map[string]Input{
			"cam1": {CameraID: "a"}, "cam1b": {CameraID: "a"}, "cam2": {CameraID: "b"}, "cam3": {CameraID: "c"}, "cam4": {CameraID: "d"}, "cam5": {CameraID: "e"},
		},
	}
	// 本体が隠した項目は「映る」では表示、「実際に表示」では非表示として数える。
	hidden := func(k ItemKey) bool {
		return (k.ContainerUUID == "S" && k.ItemID == 1) || (k.ContainerUUID == "N" && k.ItemID == 1)
	}
	got := CamerasIn(m, "S", hidden, true)
	want := []string{"a", "b", "c", "d"}
	assertStrings(t, got, want)
	got = CamerasIn(m, "S", hidden, false)
	assertStrings(t, got, []string{"c", "d"})
	m.Scenes["S"] = Container{Items: []Item{{ID: 1, SourceUUID: "G", SourceKind: GroupSource, Enabled: false}}}
	assertStrings(t, CamerasIn(m, "S", nil, true), nil) // 非表示のグループの中は映らない
}

func TestCamerasInDiamondHumanHiddenAndDeduplicatesSources(t *testing.T) {
	m := Map{
		Scenes: map[string]Container{
			"S": {Items: []Item{{ID: 1, SourceUUID: "N", SourceKind: SceneSource, Enabled: true}, {ID: 2, SourceUUID: "M", SourceKind: SceneSource, Enabled: true}, {ID: 3, SourceUUID: "camA", SourceKind: InputSource, Enabled: true}, {ID: 4, SourceUUID: "camB", SourceKind: InputSource, Enabled: true}}},
			"N": {Items: []Item{{ID: 1, SourceUUID: "X", SourceKind: SceneSource, Enabled: false}}},
			"M": {Items: []Item{{ID: 1, SourceUUID: "X", SourceKind: SceneSource, Enabled: true}}},
			"X": {Items: []Item{{ID: 1, SourceUUID: "camA", SourceKind: InputSource, Enabled: true}}},
		},
		Inputs: map[string]Input{"camA": {CameraID: "a"}, "camB": {CameraID: "a"}},
	}
	hidden := func(k ItemKey) bool { return k.ContainerUUID == "N" }
	assertStrings(t, CamerasIn(m, "S", hidden, true), []string{"a"})
	assertStrings(t, CamerasIn(m, "S", hidden, false), []string{"a"}) // ひし形のもう一方の道で映る
	m.Scenes["S"] = Container{Items: []Item{{ID: 1, SourceUUID: "N", SourceKind: SceneSource, Enabled: true}, {ID: 2, SourceUUID: "M", SourceKind: SceneSource, Enabled: true}, {ID: 3, SourceUUID: "camB", SourceKind: InputSource, Enabled: true}}}
	assertStrings(t, CamerasIn(m, "S", hidden, true), []string{"a"}) // 同じカメラの2つのソースは1つにまとまる
}

func TestCamerasInSortedAndMissingScene(t *testing.T) {
	m := Map{Scenes: map[string]Container{"S": {Items: []Item{{ID: 1, SourceUUID: "z", SourceKind: InputSource, Enabled: true}, {ID: 2, SourceUUID: "a", SourceKind: InputSource, Enabled: true}}}}, Inputs: map[string]Input{"z": {CameraID: "z"}, "a": {CameraID: "a"}}}
	assertStrings(t, CamerasIn(m, "S", nil, true), []string{"a", "z"})
	assertStrings(t, CamerasIn(m, "missing", nil, true), nil)
}

func TestClassify(t *testing.T) {
	m := Map{Scenes: map[string]Container{"empty": {}, "watched": {Items: []Item{{ID: 1, SourceUUID: "input", SourceKind: InputSource, Enabled: true}}}}, Inputs: map[string]Input{"input": {CameraID: "cam"}}}
	for _, tt := range []struct {
		scene, standby string
		want           Class
	}{
		{"empty", "empty", Standby}, {"watched", "empty", Monitored}, {"empty", "empty2", Other}, {"missing", "", Other},
	} {
		if got := Classify(m, tt.scene, tt.standby); got != tt.want {
			t.Errorf("Classify(%q)=%v want %v", tt.scene, got, tt.want)
		}
	}
	if got := ClassifyWithMap(false, m, "watched", "empty"); got != Unknown {
		t.Fatalf("unknown map class = %v", got)
	}
}

func TestHealth(t *testing.T) {
	m := Map{Scenes: map[string]Container{"S": {Items: []Item{{ID: 1, SourceUUID: "a", SourceKind: InputSource, Enabled: true}, {ID: 2, SourceUUID: "b", SourceKind: InputSource, Enabled: true}}}}, Inputs: map[string]Input{"a": {CameraID: "a"}, "b": {CameraID: "b"}}}
	cameras := CamerasIn(m, "S", nil, true)
	tests := []struct {
		name     string
		states   map[string]CameraState
		motions  map[string]Motion
		studio   bool
		unusable map[string]bool
		order    []string
		want     CameraState
		bad      string
		has      bool
	}{
		{"standby one bad", map[string]CameraState{"a": StateLow, "b": StateOK}, map[string]Motion{"a": MotionStandby, "b": MotionHide}, false, nil, []string{"b", "a"}, StateLow, "a", true},
		{"hide only bad but standby okay", map[string]CameraState{"a": StateDown, "b": StateOK}, map[string]Motion{"a": MotionHide, "b": MotionStandby}, false, nil, []string{"a", "b"}, StateOK, "", true},
		{"all hide all bad", map[string]CameraState{"a": StateLow, "b": StateDown}, map[string]Motion{"a": MotionHide, "b": MotionHide}, false, nil, []string{"a", "b"}, StateDown, "b", true},
		{"all hide one okay", map[string]CameraState{"a": StateOK, "b": StateDown}, map[string]Motion{"a": MotionHide, "b": MotionHide}, false, nil, []string{"a", "b"}, StateOK, "", true},
		{"unusable overrides okay", map[string]CameraState{"a": StateOK, "b": StateDown}, map[string]Motion{"a": MotionHide, "b": MotionHide}, false, map[string]bool{"a": true}, []string{"a", "b"}, StateDown, "a", true},
		{"low down worst and configured order", map[string]CameraState{"a": StateLow, "b": StateDown}, map[string]Motion{"a": MotionStandby, "b": MotionStandby}, false, nil, []string{"a", "b"}, StateDown, "b", true},
		{"same state configured order", map[string]CameraState{"a": StateDown, "b": StateDown}, map[string]Motion{"a": MotionStandby, "b": MotionStandby}, false, nil, []string{"b", "a"}, StateDown, "b", true},
		{"studio hide treated as standby", map[string]CameraState{"a": StateOK, "b": StateDown}, map[string]Motion{"a": MotionHide, "b": MotionHide}, true, nil, []string{"a", "b"}, StateDown, "b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bad, ok := Health(cameras, tt.states, tt.motions, tt.studio, tt.unusable, tt.order)
			if got != tt.want || bad != tt.bad || ok != tt.has {
				t.Fatalf("Health()=(%v,%q,%v), want (%v,%q,%v)", got, bad, ok, tt.want, tt.bad, tt.has)
			}
		})
	}
	if state, id, ok := Health(nil, nil, nil, false, nil, nil); ok || state != "" || id != "" {
		t.Fatalf("empty scene health = (%v,%q,%v)", state, id, ok)
	}
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// 送ったがイベントがまだの「隠す」（地図ではまだ表示、本体の一覧にはある）は、「実際に表示」では非表示、
// 「映る」では表示として数える（カメラの spec 第5節の2）。
func TestCamerasInPendingHideCountsAsHidden(t *testing.T) {
	m := Map{
		Scenes: map[string]Container{"S": {Items: []Item{{ID: 1, SourceUUID: "camA", SourceKind: InputSource, Enabled: true}}}},
		Inputs: map[string]Input{"camA": {CameraID: "a"}},
	}
	pending := func(k ItemKey) bool { return k.ContainerUUID == "S" && k.ItemID == 1 }
	assertStrings(t, CamerasIn(m, "S", pending, true), []string{"a"})
	assertStrings(t, CamerasIn(m, "S", pending, false), nil)
	assertStrings(t, CamerasIn(m, "S", nil, false), []string{"a"})
}

// 0b: MotionIgnore は集合に残すが、不良と数えない。全 hide 不良の判定にも再投入しない。
func TestHealthのMotionIgnore(t *testing.T) {
	states := map[string]CameraState{"m": StateOK, "s": StateDown}
	motions := map[string]Motion{"m": MotionStandby, "s": MotionIgnore}
	h, bad, has := Health([]string{"m", "s"}, states, motions, false, nil, []string{"m", "s"})
	if !has || h != StateOK || bad != "" {
		t.Fatalf("ignore の s が DOWN: got %v %q %v, want OK", h, bad, has)
	}
	// ignore だけが映るシーンの健康は OK（全 hide 不良に再投入しない）
	h, bad, has = Health([]string{"s"}, states, motions, false, nil, []string{"m", "s"})
	if !has || h != StateOK || bad != "" {
		t.Fatalf("ignore だけ: got %v %q %v, want OK", h, bad, has)
	}
}

// CamerasInWith は項目ごとに「表示として数えるか」を呼び側が決める（CamerasIn はその特別な場合）。
func TestCamerasInWith(t *testing.T) {
	m := Map{
		Scenes: map[string]Container{"A": {Items: []Item{
			{ID: 1, SourceUUID: "im", SourceKind: InputSource, Enabled: true},
			{ID: 2, SourceUUID: "is", SourceKind: InputSource, Enabled: true},
		}}},
		Groups: map[string]Container{},
		Inputs: map[string]Input{"im": {CameraID: "m"}, "is": {CameraID: "s"}},
	}
	got := CamerasInWith(m, "A", func(k ItemKey, it Item) bool { return k.ItemID != 2 && it.Enabled })
	if len(got) != 1 || got[0] != "m" {
		t.Fatalf("got %v, want [m]", got)
	}
	got = CamerasInWith(m, "A", func(k ItemKey, it Item) bool { return true })
	if len(got) != 2 {
		t.Fatalf("got %v, want [m s]", got)
	}
}

func TestMatchCameraTwoLevel(t *testing.T) {
	cams := []CameraPath{{ID: "main", Path: "live/main"}, {ID: "sub", Path: "live/sub"}}
	f := boolPtr(false)
	tests := []struct {
		name       string
		raw        string
		wantID     string
		wantLegacy string
	}{
		{"2段を見分ける", "rtsp://127.0.0.1:8554/live/main", "main", ""},
		{"2段（資格情報、末尾の /）", "rtsp://u:p@10.0.0.2:8554/live/sub/", "sub", ""},
		{"1段は見分けない（古い形）", "rtsp://127.0.0.1:8554/main", "", "main"},
		{"app だけは見分けない", "rtsp://127.0.0.1:8554/live", "", ""},
		{"別の app は見分けない", "rtsp://127.0.0.1:8554/other/main", "", ""},
		{"3段は見分けない", "rtsp://127.0.0.1:8554/live/main/x", "", ""},
		{"知らない1段は古い形でもない", "rtsp://127.0.0.1:8554/zzz", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := MatchCamera(tt.raw, f, 8554, cams)
			if ok != (tt.wantID != "") || id != tt.wantID {
				t.Errorf("MatchCamera = (%q, %v), want %q", id, ok, tt.wantID)
			}
			lid, lok := MatchLegacy(tt.raw, f, 8554, cams)
			if lok != (tt.wantLegacy != "") || lid != tt.wantLegacy {
				t.Errorf("MatchLegacy = (%q, %v), want %q", lid, lok, tt.wantLegacy)
			}
		})
	}
	// app が空（path = id）: 1段を見分け、古い形はない
	plain := []CameraPath{{ID: "main", Path: "main"}}
	if id, ok := MatchCamera("rtsp://h:8554/main", f, 8554, plain); !ok || id != "main" {
		t.Errorf("app が空で1段を見分けない: %q %v", id, ok)
	}
	if _, ok := MatchLegacy("rtsp://h:8554/main", f, 8554, plain); ok {
		t.Error("app が空なのに古い形と判定した")
	}
	if _, ok := MatchCamera("rtsp://h:8554/live/main", f, 8554, plain); ok {
		t.Error("app が空なのに2段を見分けた")
	}
}
