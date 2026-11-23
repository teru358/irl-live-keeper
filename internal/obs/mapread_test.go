package obs

import (
	"context"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"reflect"
	"testing"

	"github.com/andreykaipov/goobs/api/requests/config"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/typedefs"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// fakeMapReq は mapRequester の偽物。呼ばれた要求を順に記録する。
type fakeMapReq struct {
	calls     []string
	collName  string
	scenes    []*typedefs.Scene
	items     map[string][]*typedefs.SceneItem // シーンとグループの UUID → 項目
	inputs    []*typedefs.Input
	settings  map[string]map[string]any
	failAfter int // 0 でなければ、この本数目の要求から失敗する
}

func (f *fakeMapReq) call(s string) error {
	f.calls = append(f.calls, s)
	if f.failAfter > 0 && len(f.calls) >= f.failAfter {
		return errors.New("失敗")
	}
	return nil
}

func (f *fakeMapReq) GetSceneCollectionList() (*config.GetSceneCollectionListResponse, error) {
	if err := f.call("GetSceneCollectionList"); err != nil {
		return nil, err
	}
	return &config.GetSceneCollectionListResponse{CurrentSceneCollectionName: f.collName}, nil
}

func (f *fakeMapReq) GetSceneList() (*scenes.GetSceneListResponse, error) {
	if err := f.call("GetSceneList"); err != nil {
		return nil, err
	}
	return &scenes.GetSceneListResponse{Scenes: f.scenes}, nil
}

func (f *fakeMapReq) GetSceneItemList(uuid string) (*sceneitems.GetSceneItemListResponse, error) {
	if err := f.call("GetSceneItemList(" + uuid + ")"); err != nil {
		return nil, err
	}
	return &sceneitems.GetSceneItemListResponse{SceneItems: f.items[uuid]}, nil
}

func (f *fakeMapReq) GetGroupSceneItemList(uuid string) (*sceneitems.GetGroupSceneItemListResponse, error) {
	if err := f.call("GetGroupSceneItemList(" + uuid + ")"); err != nil {
		return nil, err
	}
	return &sceneitems.GetGroupSceneItemListResponse{SceneItems: f.items[uuid]}, nil
}

func (f *fakeMapReq) GetInputList(kind string) (*inputs.GetInputListResponse, error) {
	if err := f.call("GetInputList(" + kind + ")"); err != nil {
		return nil, err
	}
	return &inputs.GetInputListResponse{Inputs: f.inputs}, nil
}

func (f *fakeMapReq) GetInputSettings(uuid string) (*inputs.GetInputSettingsResponse, error) {
	if err := f.call("GetInputSettings(" + uuid + ")"); err != nil {
		return nil, err
	}
	return &inputs.GetInputSettingsResponse{InputSettings: f.settings[uuid]}, nil
}

func sceneItem(id int, uuid, name, kind string, enabled bool) *typedefs.SceneItem {
	it := &typedefs.SceneItem{SceneItemID: id, SourceUuid: uuid, SourceName: name, SceneItemEnabled: enabled}
	switch kind {
	case "input":
		it.SourceType, it.InputKind = "OBS_SOURCE_TYPE_INPUT", "ffmpeg_source"
	case "scene":
		it.SourceType = "OBS_SOURCE_TYPE_SCENE"
	case "group":
		it.SourceType, it.IsGroup = "OBS_SOURCE_TYPE_SCENE", true
	}
	return it
}

func testWorld() *fakeMapReq {
	return &fakeMapReq{
		collName: "ライブ",
		scenes:   []*typedefs.Scene{{SceneUuid: "sA", SceneName: "A"}, {SceneUuid: "sB", SceneName: "B"}},
		items: map[string][]*typedefs.SceneItem{
			"sA": {sceneItem(1, "in1", "Cam1", "input", true), sceneItem(2, "g1", "Group1", "group", false)},
			"sB": {sceneItem(5, "g1", "Group1", "group", true), sceneItem(6, "sA", "A", "scene", true)},
			"g1": {sceneItem(1, "in2", "Cam2", "input", true), sceneItem(2, "g2", "Inner", "group", true)},
			"g2": {sceneItem(1, "in4", "Other", "input", true)},
		},
		inputs: []*typedefs.Input{
			{InputUuid: "in1", InputName: "Cam1", InputKind: "ffmpeg_source"},
			{InputUuid: "in2", InputName: "Cam2", InputKind: "ffmpeg_source"},
			{InputUuid: "in3", InputName: "File", InputKind: "ffmpeg_source"},
			{InputUuid: "in4", InputName: "Wrong", InputKind: "ffmpeg_source"},
		},
		settings: map[string]map[string]any{
			"in1": {"input": "rtsp://127.0.0.1:8554/cam1", "is_local_file": false},
			"in2": {"input": "rtsp://127.0.0.1:8554/cam2", "is_local_file": false},
			"in3": {"input": "rtsp://127.0.0.1:8554/cam1"}, // is_local_file は既定（true）なので省かれる
			"in4": {"input": "rtsp://127.0.0.1:554/cam1", "is_local_file": false},
		},
	}
}

func TestReadSceneMapRequestOrderAndUUIDs(t *testing.T) {
	f := testWorld()
	m, err := readSceneMap(context.Background(), f, plainPaths("cam1", "cam2"), 8554)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GetSceneCollectionList", "GetSceneList",
		"GetSceneItemList(sA)", "GetSceneItemList(sB)",
		"GetGroupSceneItemList(g1)", "GetGroupSceneItemList(g2)", // グループは2か所に出ても1回
		"GetInputList(ffmpeg_source)",
		"GetInputSettings(in1)", "GetInputSettings(in2)", "GetInputSettings(in3)", "GetInputSettings(in4)",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("要求の順:\n got %v\nwant %v", f.calls, want)
	}
	if m.Collection != "ライブ" {
		t.Errorf("Collection = %q", m.Collection)
	}
	if got := m.Scenes["sA"]; got.Name != "A" || len(got.Items) != 2 {
		t.Errorf("sA: %+v", got)
	}
	wantItems := []scenemap.Item{
		{ID: 5, SourceUUID: "g1", SourceKind: scenemap.GroupSource, Enabled: true},
		{ID: 6, SourceUUID: "sA", SourceKind: scenemap.SceneSource, Enabled: true},
	}
	if !reflect.DeepEqual(m.Scenes["sB"].Items, wantItems) {
		t.Errorf("sB の項目: %+v", m.Scenes["sB"].Items)
	}
	if got := m.Scenes["sA"].Items[0]; got != (scenemap.Item{ID: 1, SourceUUID: "in1", SourceKind: scenemap.InputSource, Enabled: true}) {
		t.Errorf("sA の1つ目: %+v", got)
	}
	if got := m.Scenes["sA"].Items[1]; got.Enabled || got.SourceKind != scenemap.GroupSource {
		t.Errorf("sA の2つ目（非表示のグループ）: %+v", got)
	}
	if g := m.Groups["g1"]; g.Name != "Group1" || len(g.Items) != 2 {
		t.Errorf("g1: %+v", g)
	}
	if g := m.Groups["g2"]; g.Name != "Inner" || len(g.Items) != 1 {
		t.Errorf("g2: %+v", g)
	}
	wantInputs := map[string]scenemap.Input{
		"in1": {Name: "Cam1", CameraID: "cam1"},
		"in2": {Name: "Cam2", CameraID: "cam2"},
		"in3": {Name: "File"},  // ローカルファイル扱い
		"in4": {Name: "Wrong"}, // ポートが違う
	}
	if !reflect.DeepEqual(m.Inputs, wantInputs) {
		t.Errorf("Inputs: %+v", m.Inputs)
	}
	// B は A を入れているので、A のカメラも映る。g1 は A では非表示、B では表示。
	if got := scenemap.CamerasIn(*m, "sB", nil, true); !reflect.DeepEqual(got, []string{"cam1", "cam2"}) {
		t.Errorf("B に映るカメラ: %v", got)
	}
}

func TestReadSceneMapFailsWhenAnyRequestFails(t *testing.T) {
	probe := testWorld()
	if _, err := readSceneMap(context.Background(), probe, plainPaths("cam1"), 8554); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= len(probe.calls); n++ {
		f := testWorld()
		f.failAfter = n
		if m, err := readSceneMap(context.Background(), f, plainPaths("cam1"), 8554); err == nil || m != nil {
			t.Errorf("%d 本目の要求の失敗が、結果の失敗にならない: %v %v", n, m, err)
		}
	}
}

func TestReadSceneMapStopsWhenContextEnds(t *testing.T) {
	f := testWorld()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readSceneMap(ctx, f, plainPaths("cam1"), 8554); !errors.Is(err, context.Canceled) {
		t.Fatalf("context.Canceled のはずが %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("ctx が終わっているのに要求を送った: %v", f.calls)
	}
}

func TestReadSceneListOnly(t *testing.T) {
	f := testWorld()
	m, err := readSceneListOnly(context.Background(), f, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"GetSceneCollectionList", "GetSceneList"}) {
		t.Fatalf("要求: %v", f.calls)
	}
	if m.Collection != "ライブ" || m.Scenes["sA"].Name != "A" || len(m.Scenes["sA"].Items) != 0 || len(m.Inputs) != 0 || len(m.Groups) != 0 {
		t.Fatalf("シーンの一覧だけの地図: %+v", m)
	}

	f = testWorld()
	m, err = readSceneListOnly(context.Background(), f, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"GetSceneList"}) {
		t.Fatalf("古い版の要求: %v", f.calls)
	}
	if got := m.Scenes["A"]; got.Name != "A" {
		t.Fatalf("古い版では名前が鍵: %+v", m.Scenes)
	}
}

func TestCameraWarnings(t *testing.T) {
	cams := []CameraConfig{{ID: "cam1", Name: "メイン"}, {ID: "cam2", Name: "サブ"}, {ID: "cam3"}}
	m := &scenemap.Map{Inputs: map[string]scenemap.Input{"i": {Name: "x", CameraID: "cam1"}, "j": {Name: "y"}}}
	got := i18n.Strings(i18n.JA, cameraWarnings(cams, m))
	want := []string{"サブ のメディアソースが OBS に見つかりません", "cam3 のメディアソースが OBS に見つかりません"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("警告: %v", got)
	}
	if got := cameraWarnings(cams, &scenemap.Map{Inputs: map[string]scenemap.Input{"i": {CameraID: "cam1"}, "k": {CameraID: "cam2"}, "l": {CameraID: "cam3"}}}); len(got) != 0 {
		t.Fatalf("全部あるのに警告: %v", got)
	}
	_ = fmt.Sprint
}

// plainPaths は path がカメラの id と同じ（app が空）の照合の対象。
func plainPaths(ids ...string) []scenemap.CameraPath {
	out := make([]scenemap.CameraPath, len(ids))
	for i, id := range ids {
		out[i] = scenemap.CameraPath{ID: id, Path: id}
	}
	return out
}

func TestMatchInputSettingsTwoLevel(t *testing.T) {
	cams := []scenemap.CameraPath{{ID: "main", Path: "live/main"}}
	mk := func(u string) map[string]any { return map[string]any{"input": u, "is_local_file": false} }
	if id, lg := matchInputSettings(mk("rtsp://127.0.0.1:8554/live/main"), cams, 8554); id != "main" || lg != "" {
		t.Errorf("2段: %q %q", id, lg)
	}
	if id, lg := matchInputSettings(mk("rtsp://127.0.0.1:8554/main"), cams, 8554); id != "" || lg != "main" {
		t.Errorf("1段: %q %q", id, lg)
	}
}

func TestCameraWarningsLegacyURL(t *testing.T) {
	cams := []CameraConfig{{ID: "main", Name: "メイン", Path: "live/main"}}
	m := &scenemap.Map{Inputs: map[string]scenemap.Input{"i": {Name: "旧メイン", LegacyCameraID: "main"}}}
	got := i18n.Strings(i18n.JA, cameraWarnings(cams, m))
	want := []string{
		"メイン のメディアソースが OBS に見つかりません",
		"メディアソース 旧メイン の URL が古い形（…/main）です。…/live/main に直してください",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("警告: %q", got)
	}
}
