package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/config"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/typedefs"

	kcfg "github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
)

// 試験だけのプロファイルとシーンコレクション。
// test setup は、今のプロファイルとシーンコレクションの名前を testStateFile に残してから、
// 試験のプロファイル（配信先は PC の中の試験だけの MediaMTX 127.0.0.1:21935。本物の配信先へ流れない）と
// 試験のシーンコレクション（待機シーン、A〔1台目と2台目〕、B〔1台目〕、Sub〔2台目〕、Talk）に切り替える。
// ユーザーのプロファイルとシーンコレクションには書かない。
// test restore は、元の2つへ戻し、試験のプロファイルを消す（シーンコレクションは obs-websocket では消せないので残る。
// 次の setup で使い直す。消すときは OBS の「シーンコレクション」メニューから）。
const (
	testProfile    = "IRLKeeper-Test"
	testCollection = "IRLKeeper-Test"
	testStateFile  = "test-original.json"
	testDest       = "rtmp://127.0.0.1:21935/test"
)

type testState struct {
	Profile    string `json:"profile"`
	Collection string `json:"collection"`
}

func configPath() string {
	if p := os.Getenv("CTL_CONFIG"); p != "" {
		return p
	}
	return "config.yaml"
}

func testSetup(c *goobs.Client) error {
	if _, err := os.Stat(testStateFile); err == nil {
		return fmt.Errorf("%s がすでにある（前の試験のあと戻していない）。先に test restore を実行する", testStateFile)
	}
	if st, err := c.Stream.GetStreamStatus(); err != nil {
		return err
	} else if st.OutputActive {
		return fmt.Errorf("配信中は切り替えられない。配信を止めてから")
	}
	cfg, err := kcfg.Load(configPath())
	if err != nil {
		return err
	}
	if len(cfg.Cameras) < 2 {
		return fmt.Errorf("config.yaml の cameras が %d 台。2台目（sub）を足して本体を起動し直してから", len(cfg.Cameras))
	}
	host := cfg.Stream.ObsReadHost
	if host == "" {
		host = "127.0.0.1"
	}
	urls := supervisor.ReadURLs(cfg, host)
	cam1, cam2 := urls[0], urls[1]

	pl, err := c.Config.GetProfileList()
	if err != nil {
		return err
	}
	cl, err := c.Config.GetSceneCollectionList()
	if err != nil {
		return err
	}
	if pl.CurrentProfileName == testProfile || cl.CurrentSceneCollectionName == testCollection {
		return fmt.Errorf("すでに試験のプロファイルかシーンコレクションにいる。元が分からないので、OBS のメニューで元に戻してから")
	}
	// メディアソースの設定のひな形: 今のシーンコレクションで、1台目の URL を読んでいるメディアソースの設定（URL 以外）
	tmpl := templateSettings(c, "/"+cam1.ID)

	b, _ := json.Marshal(testState{Profile: pl.CurrentProfileName, Collection: cl.CurrentSceneCollectionName})
	if err := os.WriteFile(testStateFile, b, 0o600); err != nil {
		return err
	}
	fmt.Printf("元のプロファイル: %s、元のシーンコレクション: %s（%s に残した）\n", pl.CurrentProfileName, cl.CurrentSceneCollectionName, testStateFile)

	if err := switchProfile(c, pl.Profiles); err != nil {
		return err
	}
	if err := switchCollection(c, cl.SceneCollections); err != nil {
		return err
	}
	standby := cfg.OBS.StandbyScene
	for _, s := range []string{standby, "A", "B", "Sub", "Talk"} {
		if err := ensureScene(c, s); err != nil {
			return err
		}
		addLabel(c, s)
	}
	in1, in2 := "cam-"+cam1.ID, "cam-"+cam2.ID
	if err := ensureMediaInput(c, in1, cam1.RTSP, tmpl, "A"); err != nil {
		return err
	}
	if err := ensureMediaInput(c, in2, cam2.RTSP, tmpl, "A"); err != nil {
		return err
	}
	for _, p := range [][2]string{{"B", in1}, {"Sub", in2}} {
		if err := ensureItem(c, p[0], p[1]); err != nil {
			return err
		}
	}
	b2 := "B"
	if _, err := c.Scenes.SetCurrentProgramScene(&scenes.SetCurrentProgramSceneParams{SceneName: &b2}); err != nil {
		return err
	}
	fmt.Printf("試験のプロファイル %s（配信先 %s）と、試験のシーンコレクション %s に切り替えた\n", testProfile, testDest, testCollection)
	fmt.Printf("シーン: %s（カメラなし）、A（%s と %s）、B（%s）、Sub（%s）、Talk（カメラなし）。今のシーンは B\n", standby, in1, in2, in1, in2)
	return nil
}

func testRestore(c *goobs.Client) error {
	b, err := os.ReadFile(testStateFile)
	if err != nil {
		return fmt.Errorf("%s が読めない（setup をしていないか、もう戻した）: %v", testStateFile, err)
	}
	var s testState
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if st, err := c.Stream.GetStreamStatus(); err != nil {
		return err
	} else if st.OutputActive {
		return fmt.Errorf("配信中は切り替えられない。配信を止めてから")
	}
	if _, err := c.Config.SetCurrentSceneCollection(&config.SetCurrentSceneCollectionParams{SceneCollectionName: &s.Collection}); err != nil {
		return err
	}
	if _, err := c.Config.SetCurrentProfile(&config.SetCurrentProfileParams{ProfileName: &s.Profile}); err != nil {
		return err
	}
	if err := waitReady(c); err != nil {
		return err
	}
	pl, err := c.Config.GetProfileList()
	if err != nil {
		return err
	}
	cl, err := c.Config.GetSceneCollectionList()
	if err != nil {
		return err
	}
	if pl.CurrentProfileName != s.Profile || cl.CurrentSceneCollectionName != s.Collection {
		return fmt.Errorf("元に戻らなかった（今のプロファイル %s、シーンコレクション %s）。OBS のメニューで %s と %s に戻す",
			pl.CurrentProfileName, cl.CurrentSceneCollectionName, s.Profile, s.Collection)
	}
	name := testProfile
	if _, err := c.Config.RemoveProfile(&config.RemoveProfileParams{ProfileName: &name}); err != nil {
		fmt.Println("注意: 試験のプロファイルを消せなかった（OBS の「プロファイル」メニューで消せる）:", err)
	}
	if err := os.Remove(testStateFile); err != nil {
		return err
	}
	fmt.Printf("元のプロファイル %s と、元のシーンコレクション %s に戻した（試験のシーンコレクション %s は残る）\n", s.Profile, s.Collection, testCollection)
	return nil
}

func switchProfile(c *goobs.Client, existing []string) error {
	name := testProfile
	var err error
	if contains(existing, name) {
		_, err = c.Config.SetCurrentProfile(&config.SetCurrentProfileParams{ProfileName: &name})
	} else {
		_, err = c.Config.CreateProfile(&config.CreateProfileParams{ProfileName: &name})
	}
	if err != nil {
		return err
	}
	if err := waitReady(c); err != nil {
		return err
	}
	pl, err := c.Config.GetProfileList()
	if err != nil {
		return err
	}
	if pl.CurrentProfileName != name {
		return fmt.Errorf("%s に切り替わらなかった（今: %s）", name, pl.CurrentProfileName)
	}
	typ := "rtmp_custom"
	if _, err := c.Config.SetStreamServiceSettings(&config.SetStreamServiceSettingsParams{
		StreamServiceType:     &typ,
		StreamServiceSettings: &typedefs.StreamServiceSettings{Server: testDest, Key: "t"},
	}); err != nil {
		return err
	}
	for _, kv := range [][3]string{{"Output", "Reconnect", "true"}, {"Output", "RetryDelay", "2"}, {"Output", "MaxRetries", "25"}} {
		cat, n, v := kv[0], kv[1], kv[2]
		if _, err := c.Config.SetProfileParameter(&config.SetProfileParameterParams{ParameterCategory: &cat, ParameterName: &n, ParameterValue: &v}); err != nil {
			return err
		}
	}
	return nil
}

func switchCollection(c *goobs.Client, existing []string) error {
	name := testCollection
	var err error
	if contains(existing, name) {
		_, err = c.Config.SetCurrentSceneCollection(&config.SetCurrentSceneCollectionParams{SceneCollectionName: &name})
	} else {
		_, err = c.Config.CreateSceneCollection(&config.CreateSceneCollectionParams{SceneCollectionName: &name})
	}
	if err != nil {
		return err
	}
	if err := waitReady(c); err != nil {
		return err
	}
	cl, err := c.Config.GetSceneCollectionList()
	if err != nil {
		return err
	}
	if cl.CurrentSceneCollectionName != name {
		return fmt.Errorf("%s に切り替わらなかった（今: %s）", name, cl.CurrentSceneCollectionName)
	}
	return nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// templateSettings は、URL が suffix で終わるメディアソースの設定（input を除く）を返す。なければ空。
func templateSettings(c *goobs.Client, suffix string) map[string]any {
	kind := "ffmpeg_source"
	r, err := c.Inputs.GetInputList(&inputs.GetInputListParams{InputKind: &kind})
	if err != nil {
		return map[string]any{}
	}
	for _, in := range r.Inputs {
		n := in.InputName
		s, err := c.Inputs.GetInputSettings(&inputs.GetInputSettingsParams{InputName: &n})
		if err != nil {
			continue
		}
		if u, _ := s.InputSettings["input"].(string); strings.HasSuffix(u, suffix) {
			out := map[string]any{}
			for k, v := range s.InputSettings {
				if k != "input" {
					out[k] = v
				}
			}
			return out
		}
	}
	return map[string]any{}
}

func ensureScene(c *goobs.Client, name string) error {
	r, err := c.Scenes.GetSceneList()
	if err != nil {
		return err
	}
	for _, s := range r.Scenes {
		if s.SceneName == name {
			return nil
		}
	}
	_, err = c.Scenes.CreateScene(&scenes.CreateSceneParams{SceneName: &name})
	return err
}

func inputExists(c *goobs.Client, name string) bool {
	_, err := c.Inputs.GetInputSettings(&inputs.GetInputSettingsParams{InputName: &name})
	return err == nil
}

// ensureMediaInput はメディアソースを作る（あれば URL とハードウェアデコードを入れ直す）。scene に項目を置く。
func ensureMediaInput(c *goobs.Client, name, url string, tmpl map[string]any, scene string) error {
	settings := map[string]any{}
	for k, v := range tmpl {
		settings[k] = v
	}
	settings["is_local_file"] = false
	settings["input"] = url
	settings["hw_decode"] = true
	if inputExists(c, name) {
		if _, err := c.Inputs.SetInputSettings(&inputs.SetInputSettingsParams{InputName: &name, InputSettings: settings}); err != nil {
			return err
		}
		return ensureItem(c, scene, name)
	}
	kind := "ffmpeg_source"
	_, err := c.Inputs.CreateInput(&inputs.CreateInputParams{SceneName: &scene, InputName: &name, InputKind: &kind, InputSettings: settings})
	return err
}

// ensureItem は scene に source の項目を置く。すでにあれば表示に戻す（前の試験で隠したままのことがあるため）。
func ensureItem(c *goobs.Client, scene, source string) error {
	if id, err := c.SceneItems.GetSceneItemId(&sceneitems.GetSceneItemIdParams{SceneName: &scene, SourceName: &source}); err == nil {
		on, item := true, id.SceneItemId
		_, err := c.SceneItems.SetSceneItemEnabled(&sceneitems.SetSceneItemEnabledParams{SceneName: &scene, SceneItemId: &item, SceneItemEnabled: &on})
		return err
	}
	_, err := c.SceneItems.CreateSceneItem(&sceneitems.CreateSceneItemParams{SceneName: &scene, SourceName: &source})
	return err
}

// addLabel はシーンの名前の文字を置く（見分けやすくするため。失敗しても続ける）。
func addLabel(c *goobs.Client, scene string) {
	name := "label-" + scene
	if inputExists(c, name) {
		return
	}
	kind := "text_ft2_source_v2"
	if runtime.GOOS == "windows" {
		kind = "text_gdiplus_v3"
	}
	settings := map[string]any{"text": scene, "font": map[string]any{"face": "Arial", "size": 96}}
	if _, err := c.Inputs.CreateInput(&inputs.CreateInputParams{SceneName: &scene, InputName: &name, InputKind: &kind, InputSettings: settings}); err != nil {
		fmt.Println("注意: シーンの名前の文字を置けなかった:", scene, err)
	}
}

// setHWDecode は、試験のシーンコレクションの中で、名前が cam- で始まるメディアソースのハードウェアデコードを切り替える。
func setHWDecode(c *goobs.Client, on bool) error {
	cl, err := c.Config.GetSceneCollectionList()
	if err != nil {
		return err
	}
	if cl.CurrentSceneCollectionName != testCollection {
		return fmt.Errorf("試験のシーンコレクション %s にいない（今: %s）。ユーザーのメディアソースは変えない", testCollection, cl.CurrentSceneCollectionName)
	}
	kind := "ffmpeg_source"
	r, err := c.Inputs.GetInputList(&inputs.GetInputListParams{InputKind: &kind})
	if err != nil {
		return err
	}
	for _, in := range r.Inputs {
		if !strings.HasPrefix(in.InputName, "cam-") {
			continue
		}
		n := in.InputName
		if _, err := c.Inputs.SetInputSettings(&inputs.SetInputSettingsParams{InputName: &n, InputSettings: map[string]any{"hw_decode": on}}); err != nil {
			return err
		}
		fmt.Println(n, "hw_decode", on)
	}
	return nil
}

// waitReady は、プロファイルやシーンコレクションの切替のあと、OBS が要求に答えられるようになるまで待つ。
// 切替の間、obs-websocket は NotReady（207）を返す。Windows の OBS 33.0.1 では、プロファイルを作った直後に
// シーンコレクションを作ると NotReady のまま戻らなかったことがある（2026-11-21）。
func waitReady(c *goobs.Client) error {
	time.Sleep(time.Second)
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err := c.Scenes.GetSceneList()
		if err == nil {
			// 答えられるようになってから、もう少し待つ（切替のあとの OBS の処理が続いていることがあるため）
			time.Sleep(2 * time.Second)
			return nil
		}
		if !strings.Contains(err.Error(), "207") && !strings.Contains(strings.ToLower(err.Error()), "not ready") && !strings.Contains(strings.ToLower(err.Error()), "notready") {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("OBS が30秒たっても準備中（NotReady）のまま。OBS の画面にダイアログが出ていないか見て、出ていなければ OBS のメニューで元のプロファイルとシーンコレクション（%s）に戻す", testStateFile)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
