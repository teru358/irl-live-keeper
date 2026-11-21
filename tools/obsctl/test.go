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

// 試験だけのプロファイルと、今のシーンコレクションに足す試験のシーン。
//
// test setup は、今のプロファイル、シーンコレクション、シーンを testStateFile に残してから、
//   - 試験のプロファイル（配信先は PC の中の試験だけの MediaMTX 127.0.0.1:21935。本物の配信先へ流れない）に切り替え
//   - 今のシーンコレクションに、T-A（1台目と2台目）、T-B（1台目）、T-Sub（2台目）のシーンと、
//     メディアソース cam-<id>、シーンの名前の文字 label-T-* を足す（作ったものの UUID を testStateFile に書く）
//
// test restore は、testStateFile に書いたもの（ctl が作ったもの）だけを消し、元のシーンとプロファイルに戻す。
//
// シーンコレクションは WebSocket で作ったり切り替えたりしない。Windows の OBS 33.0.1 で、CreateSceneCollection のあと
// obs-websocket が NotReady のまま戻らず、そのあと手で切り替えると OBS が落ちた（2026-11-21、M-9 の F-4）。
const (
	testProfile   = "IRLKeeper-Test"
	testStateFile = "test-original.json"
	testDest      = "rtmp://127.0.0.1:21935/test"
	testPrefix    = "T-"
)

type created struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

type testState struct {
	Profile    string    `json:"profile"`
	Collection string    `json:"collection"`
	Scene      string    `json:"scene"`
	Scenes     []created `json:"scenes"`
	Inputs     []created `json:"inputs"`
}

func (s *testState) save() error {
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(testStateFile, b, 0o600)
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
	in1, in2 := "cam-"+cam1.ID, "cam-"+cam2.ID
	sA, sB, sSub := testPrefix+"A", testPrefix+"B", testPrefix+"Sub"

	pl, err := c.Config.GetProfileList()
	if err != nil {
		return err
	}
	if pl.CurrentProfileName == testProfile {
		return fmt.Errorf("すでに試験のプロファイルにいる。元が分からないので、OBS の「プロファイル」メニューで元に戻してから")
	}
	cl, err := c.Config.GetSceneCollectionList()
	if err != nil {
		return err
	}
	sl, err := c.Scenes.GetSceneList()
	if err != nil {
		return err
	}
	// 作る名前が、もうないこと（ユーザーのものを消さないため）。待機シーンはあること
	have := map[string]bool{}
	for _, s := range sl.Scenes {
		have[s.SceneName] = true
	}
	if !have[cfg.OBS.StandbyScene] {
		return fmt.Errorf("待機シーン %s が今のシーンコレクションにない", cfg.OBS.StandbyScene)
	}
	for _, n := range []string{sA, sB, sSub} {
		if have[n] {
			return fmt.Errorf("シーン %s がもうある（前の試験の残りか、ユーザーのもの）。OBS で名前を変えるか消してから", n)
		}
	}
	for _, n := range []string{in1, in2, "label-" + sA, "label-" + sB, "label-" + sSub} {
		if inputExists(c, n) {
			return fmt.Errorf("ソース %s がもうある（前の試験の残りか、ユーザーのもの）。OBS で名前を変えるか消してから", n)
		}
	}
	// メディアソースの設定のひな形: 今のシーンコレクションで、1台目の URL を読んでいるメディアソースの設定（URL 以外）
	tmpl := templateSettings(c, "/"+cam1.ID)

	st := &testState{Profile: pl.CurrentProfileName, Collection: cl.CurrentSceneCollectionName, Scene: sl.CurrentProgramSceneName}
	if err := st.save(); err != nil {
		return err
	}
	fmt.Printf("元のプロファイル: %s、シーンコレクション: %s、シーン: %s（%s に残した）\n", st.Profile, st.Collection, st.Scene, testStateFile)

	if err := switchProfile(c, pl.Profiles); err != nil {
		return err
	}
	for _, n := range []string{sA, sB, sSub} {
		n := n
		r, err := c.Scenes.CreateScene(&scenes.CreateSceneParams{SceneName: &n})
		if err != nil {
			return err
		}
		st.Scenes = append(st.Scenes, created{Name: n, UUID: r.SceneUuid})
		if err := st.save(); err != nil {
			return err
		}
		if u := addLabel(c, n); u != "" {
			st.Inputs = append(st.Inputs, created{Name: "label-" + n, UUID: u})
			if err := st.save(); err != nil {
				return err
			}
		}
	}
	for _, p := range []struct{ name, url string }{{in1, cam1.RTSP}, {in2, cam2.RTSP}} {
		u, err := createMediaInput(c, p.name, p.url, tmpl, sA)
		if err != nil {
			return err
		}
		st.Inputs = append(st.Inputs, created{Name: p.name, UUID: u})
		if err := st.save(); err != nil {
			return err
		}
	}
	for _, p := range [][2]string{{sB, in1}, {sSub, in2}} {
		p := p
		if _, err := c.SceneItems.CreateSceneItem(&sceneitems.CreateSceneItemParams{SceneName: &p[0], SourceName: &p[1]}); err != nil {
			return err
		}
	}
	if _, err := c.Scenes.SetCurrentProgramScene(&scenes.SetCurrentProgramSceneParams{SceneName: &sB}); err != nil {
		return err
	}
	fmt.Printf("試験のプロファイル %s（配信先 %s）に切り替え、シーンコレクション %s に試験のシーンを足した\n", testProfile, testDest, st.Collection)
	fmt.Printf("シーン: %s（%s と %s）、%s（%s）、%s（%s）。待機シーンは %s。今のシーンは %s\n", sA, in1, in2, sB, in1, sSub, in2, cfg.OBS.StandbyScene, sB)
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
		return fmt.Errorf("配信中は戻せない。配信を止めてから")
	}
	cl, err := c.Config.GetSceneCollectionList()
	if err != nil {
		return err
	}
	if cl.CurrentSceneCollectionName != s.Collection {
		return fmt.Errorf("今のシーンコレクションが %s で、setup のとき（%s）と違う。OBS のメニューで %s に戻してから、もう一度 restore する", cl.CurrentSceneCollectionName, s.Collection, s.Collection)
	}
	// 試験のシーンにいるなら、先に元のシーンへ
	if s.Scene != "" {
		if _, err := c.Scenes.SetCurrentProgramScene(&scenes.SetCurrentProgramSceneParams{SceneName: &s.Scene}); err != nil {
			fmt.Println("注意: 元のシーンへ戻せなかった:", s.Scene, err)
		}
		// トランジションが終わってから消す（program のシーンを消すと、本体はシーンコレクションの切替とみなして
		// 読み直す。M-9 の C の2回目で起きた）。GetCurrentProgramScene はトランジションの途中でも行き先を答えるので、
		// トランジションの長さ + 1秒を待つ（最大 11 秒）
		wait := 1300 * time.Millisecond
		if tr, err := c.Transitions.GetCurrentSceneTransition(); err == nil && tr.TransitionDuration > 0 {
			wait = time.Duration(tr.TransitionDuration)*time.Millisecond + time.Second
		}
		if wait > 11*time.Second {
			wait = 11 * time.Second
		}
		time.Sleep(wait)
	}
	failed := false
	for _, sc := range s.Scenes {
		u := sc.UUID
		if _, err := c.Scenes.RemoveScene(&scenes.RemoveSceneParams{SceneUuid: &u}); err != nil && !notFound(err) {
			fmt.Println("注意: シーンを消せなかった:", sc.Name, err)
			failed = true
		}
	}
	for _, in := range s.Inputs {
		u := in.UUID
		if _, err := c.Inputs.RemoveInput(&inputs.RemoveInputParams{InputUuid: &u}); err != nil && !notFound(err) {
			// シーンを消すと、そのシーンだけで使っていたソースは OBS が消す（ResourceNotFound は消えたものとする）
			fmt.Println("注意: ソースを消せなかった:", in.Name, err)
			failed = true
		}
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
	if pl.CurrentProfileName != s.Profile {
		return fmt.Errorf("元のプロファイル %s に戻らなかった（今: %s）。OBS の「プロファイル」メニューで戻す", s.Profile, pl.CurrentProfileName)
	}
	name := testProfile
	if _, err := c.Config.RemoveProfile(&config.RemoveProfileParams{ProfileName: &name}); err != nil {
		fmt.Println("注意: 試験のプロファイルを消せなかった（OBS の「プロファイル」メニューで消せる）:", err)
	}
	if failed {
		return fmt.Errorf("消せなかったものがある（上の「注意」）。%s は残した。OBS で手で消してから、このファイルを消す", testStateFile)
	}
	if err := os.Remove(testStateFile); err != nil {
		return err
	}
	fmt.Printf("試験のシーンとソースを消し、元のプロファイル %s とシーン %s に戻した\n", s.Profile, s.Scene)
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

func inputExists(c *goobs.Client, name string) bool {
	_, err := c.Inputs.GetInputSettings(&inputs.GetInputSettingsParams{InputName: &name})
	return err == nil
}

// createMediaInput はメディアソースを scene に作り、その UUID を返す（ハードウェアデコードを入れる）。
func createMediaInput(c *goobs.Client, name, url string, tmpl map[string]any, scene string) (string, error) {
	settings := map[string]any{}
	for k, v := range tmpl {
		settings[k] = v
	}
	settings["is_local_file"] = false
	settings["input"] = url
	settings["hw_decode"] = true
	kind := "ffmpeg_source"
	r, err := c.Inputs.CreateInput(&inputs.CreateInputParams{SceneName: &scene, InputName: &name, InputKind: &kind, InputSettings: settings})
	if err != nil {
		return "", err
	}
	return r.InputUuid, nil
}

// addLabel はシーンの名前の文字を置き、その UUID を返す（見分けやすくするため。失敗したら空を返して続ける）。
func addLabel(c *goobs.Client, scene string) string {
	name := "label-" + scene
	kind := "text_ft2_source_v2"
	if runtime.GOOS == "windows" {
		kind = "text_gdiplus_v3"
	}
	settings := map[string]any{"text": scene, "font": map[string]any{"face": "Arial", "size": 96}}
	r, err := c.Inputs.CreateInput(&inputs.CreateInputParams{SceneName: &scene, InputName: &name, InputKind: &kind, InputSettings: settings})
	if err != nil {
		fmt.Println("注意: シーンの名前の文字を置けなかった:", scene, err)
		return ""
	}
	return r.InputUuid
}

// setHWDecode は、ctl が作ったメディアソース（testStateFile に書いたもの）のハードウェアデコードを切り替える。
func setHWDecode(c *goobs.Client, on bool) error {
	b, err := os.ReadFile(testStateFile)
	if err != nil {
		return fmt.Errorf("%s が読めない。test setup のあとだけ使える（ユーザーのメディアソースは変えない）", testStateFile)
	}
	var s testState
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for _, in := range s.Inputs {
		if !strings.HasPrefix(in.Name, "cam-") {
			continue
		}
		u := in.UUID
		if _, err := c.Inputs.SetInputSettings(&inputs.SetInputSettingsParams{InputUuid: &u, InputSettings: map[string]any{"hw_decode": on}}); err != nil {
			return err
		}
		fmt.Println(in.Name, "hw_decode", on)
	}
	return nil
}

// waitReady は、プロファイルの切替のあと、OBS が要求に答えられるようになるまで待つ（切替の間、NotReady〔207〕を返す）。
func waitReady(c *goobs.Client) error {
	time.Sleep(time.Second)
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err := c.Scenes.GetSceneList()
		if err == nil {
			time.Sleep(2 * time.Second)
			return nil
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "207") && !strings.Contains(msg, "notready") && !strings.Contains(msg, "not ready") {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("OBS が30秒たっても準備中（NotReady）のまま。OBS の画面にダイアログが出ていないか見て、出ていなければ OBS の「プロファイル」メニューで元のプロファイル（%s）に戻す", testStateFile)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// notFound は、OBS が「その UUID のものはない」（ResourceNotFound、600）と答えたかを返す。
func notFound(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "ResourceNotFound") || strings.Contains(msg, "(600)")
}
