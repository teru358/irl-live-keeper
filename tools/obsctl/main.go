// obsctl は、実機の確認のときに人の OBS の操作の代わりをする試験用の道具（配布物には入れない）。
// Windows 向けには ctl.exe の名前でビルドして、試験のフォルダに置く:
//
//	GOOS=windows GOARCH=amd64 go build -o tmp/windows-test/ctl.exe ./tools/obsctl
//
// 接続先は OBS_WS_ADDR（既定 127.0.0.1:4455）、パスワードは OBS_WS_PASSWORD_FILE のファイル（1行）。
// URL やパスワードを画面に出さない（試験の記録に残るため）。
//
// 使い方:
//
//	scene <名前> | current | scenes | stream start|stop | streamstatus | profile
//	setinput <url>（入力 IRL）| mediastatus [入力名] | inputs（メディアソースの一覧と再生の状態）
//	item <シーン> <ソース> on|off（シーンの項目の表示・非表示）| hwdecode on|off（試験のシーンコレクションの中だけ）
//	tdur <ms> | override <シーン> <トランジション> <ms> | transition
//	onstarting <シーン> <秒> | watch <秒> [ファイル]（ファイルを渡すと1行ごとに直接書く。裏で動かして止めても欠けない）
//	test setup | test restore（試験のプロファイルとシーンコレクション。test.go）
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/mediainputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/requests/transitions"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("使い方は tools/obsctl/main.go の先頭を見る")
		os.Exit(2)
	}
	pw, err := os.ReadFile(os.Getenv("OBS_WS_PASSWORD_FILE"))
	if err != nil {
		fmt.Println("FAIL password:", err)
		os.Exit(1)
	}
	addr := os.Getenv("OBS_WS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:4455"
	}
	opts := []goobs.Option{goobs.WithPassword(strings.TrimSpace(string(pw)))}
	if cmd := os.Args[1]; cmd != "watch" && cmd != "onstarting" {
		// イベントを使わない命令は購読しない。goobs は Disconnect とイベントの受信が重なると
		// panic（send on closed channel）することがあるので、Disconnect もせずに終わる（接続はプロセスの終わりで閉じる）
		opts = append(opts, goobs.WithEventSubscriptions(0))
	}
	c, err := goobs.New(addr, opts...)
	if err != nil {
		fmt.Println("FAIL connect:", err)
		os.Exit(1)
	}
	if err := run(c, os.Args[1:]); err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func need(a []string, n int, usage string) error {
	if len(a) < n {
		return fmt.Errorf("引数が足りない: %s", usage)
	}
	return nil
}

func run(c *goobs.Client, a []string) error {
	now := func() string { return time.Now().Format("15:04:05.000") }
	switch a[0] {
	case "scene":
		if err := need(a, 2, "scene <名前>"); err != nil {
			return err
		}
		_, err := c.Scenes.SetCurrentProgramScene(&scenes.SetCurrentProgramSceneParams{SceneName: &a[1]})
		return err
	case "current":
		r, err := c.Scenes.GetCurrentProgramScene()
		if err == nil {
			fmt.Println(r.SceneName)
		}
		return err
	case "scenes":
		cl, err := c.Config.GetSceneCollectionList()
		if err != nil {
			return err
		}
		r, err := c.Scenes.GetSceneList()
		if err != nil {
			return err
		}
		fmt.Println("シーンコレクション:", cl.CurrentSceneCollectionName, " 今のシーン:", r.CurrentProgramSceneName)
		for _, s := range r.Scenes {
			fmt.Println(" -", s.SceneName)
		}
		return nil
	case "setinput":
		if err := need(a, 2, "setinput <url>"); err != nil {
			return err
		}
		name := "IRL"
		_, err := c.Inputs.SetInputSettings(&inputs.SetInputSettingsParams{InputName: &name, InputSettings: map[string]any{"input": a[1]}})
		return err
	case "stream":
		if err := need(a, 2, "stream start|stop"); err != nil {
			return err
		}
		var err error
		if a[1] == "start" {
			_, err = c.Stream.StartStream()
		} else {
			_, err = c.Stream.StopStream()
		}
		return err
	case "streamstatus":
		r, err := c.Stream.GetStreamStatus()
		if err == nil {
			fmt.Println("outputActive", r.OutputActive, "outputReconnecting", r.OutputReconnecting)
		}
		return err
	case "profile":
		r, err := c.Config.GetProfileList()
		if err == nil {
			fmt.Println(r.CurrentProfileName)
		}
		return err
	case "mediastatus":
		name := "IRL"
		if len(a) >= 2 {
			name = a[1]
		}
		r, err := c.MediaInputs.GetMediaInputStatus(&mediainputs.GetMediaInputStatusParams{InputName: &name})
		if err == nil {
			fmt.Println(r.MediaState, "cursor", r.MediaCursor)
		}
		return err
	case "inputs":
		return listMediaInputs(c)
	case "item":
		if err := need(a, 4, "item <シーン> <ソース> on|off"); err != nil {
			return err
		}
		id, err := c.SceneItems.GetSceneItemId(&sceneitems.GetSceneItemIdParams{SceneName: &a[1], SourceName: &a[2]})
		if err != nil {
			return err
		}
		on := a[3] == "on"
		item := id.SceneItemId
		_, err = c.SceneItems.SetSceneItemEnabled(&sceneitems.SetSceneItemEnabledParams{SceneName: &a[1], SceneItemId: &item, SceneItemEnabled: &on})
		return err
	case "hwdecode":
		if err := need(a, 2, "hwdecode on|off"); err != nil {
			return err
		}
		return setHWDecode(c, a[1] == "on")
	case "tdur":
		if err := need(a, 2, "tdur <ms>"); err != nil {
			return err
		}
		var ms float64
		fmt.Sscan(a[1], &ms)
		_, err := c.Transitions.SetCurrentSceneTransitionDuration(&transitions.SetCurrentSceneTransitionDurationParams{TransitionDuration: &ms})
		return err
	case "override":
		if err := need(a, 4, "override <シーン> <トランジション> <ms>"); err != nil {
			return err
		}
		var ms float64
		fmt.Sscan(a[3], &ms)
		_, err := c.Scenes.SetSceneSceneTransitionOverride(&scenes.SetSceneSceneTransitionOverrideParams{SceneName: &a[1], TransitionName: &a[2], TransitionDuration: &ms})
		return err
	case "transition":
		r, err := c.Transitions.GetCurrentSceneTransition()
		if err == nil {
			fmt.Println(r.TransitionName, r.TransitionDuration)
		}
		return err
	case "test":
		if err := need(a, 2, "test setup|restore"); err != nil {
			return err
		}
		switch a[1] {
		case "setup":
			return testSetup(c)
		case "restore":
			return testRestore(c)
		}
		return fmt.Errorf("test setup|restore")
	case "onstarting":
		// onstarting <シーン> <秒>: イベントを出しながら待ち、STARTING を受けた瞬間に <シーン> へ切り替える（1回だけ）
		if err := need(a, 3, "onstarting <シーン> <秒>"); err != nil {
			return err
		}
		scene := a[1]
		var d time.Duration
		fmt.Sscan(a[2], &d)
		end := time.After(d * time.Second)
		sent := false
		for {
			select {
			case ev := <-c.IncomingEvents:
				switch e := ev.(type) {
				case *events.StreamStateChanged:
					fmt.Printf("%s StreamStateChanged %s active=%v\n", now(), e.OutputState, e.OutputActive)
					if !sent && e.OutputState == "OBS_WEBSOCKET_OUTPUT_STARTING" {
						sent = true
						t := now()
						_, err := c.Scenes.SetCurrentProgramScene(&scenes.SetCurrentProgramSceneParams{SceneName: &scene})
						fmt.Printf("%s SetCurrentProgramScene %s を送った（応答 %s、err=%v）\n", t, scene, now(), err)
					}
				case *events.CurrentProgramSceneChanged:
					fmt.Printf("%s SceneChanged %s\n", now(), e.SceneName)
				}
			case <-end:
				return nil
			}
		}
	case "watch":
		// 1行ごとに書き出す（裏で動かしたまま止めても、それまでの行が残るように）
		if err := need(a, 2, "watch <秒>"); err != nil {
			return err
		}
		var d time.Duration
		fmt.Sscan(a[1], &d)
		end := time.After(d * time.Second)
		out := os.Stdout
		if len(a) >= 3 {
			// PowerShell の > で書くと、止めたときにパイプの中の行が失われる（M-9 で2回起きた）。直接ファイルに書く
			f, err := os.OpenFile(a[2], os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return err
			}
			defer f.Close()
			out = f
		}
		for {
			select {
			case ev := <-c.IncomingEvents:
				line := ""
				switch e := ev.(type) {
				case *events.CurrentProgramSceneChanged:
					line = "SceneChanged " + e.SceneName
				case *events.MediaInputActionTriggered:
					line = "MediaInputActionTriggered " + e.InputName + " " + e.MediaAction
				case *events.MediaInputPlaybackStarted:
					line = "MediaInputPlaybackStarted " + e.InputName
				case *events.MediaInputPlaybackEnded:
					line = "MediaInputPlaybackEnded " + e.InputName
				case *events.StreamStateChanged:
					line = fmt.Sprintf("StreamStateChanged %s active=%v", e.OutputState, e.OutputActive)
				case *events.SceneItemEnableStateChanged:
					line = fmt.Sprintf("SceneItemEnableStateChanged %s item=%d enabled=%v", e.SceneName, e.SceneItemId, e.SceneItemEnabled)
				case *events.CurrentSceneCollectionChanged:
					line = "CurrentSceneCollectionChanged " + e.SceneCollectionName
				case *events.CurrentProfileChanged:
					line = "CurrentProfileChanged " + e.ProfileName
				}
				if line != "" {
					fmt.Fprintln(out, now(), line)
					out.Sync()
				}
			case <-end:
				return nil
			}
		}
	}
	return fmt.Errorf("知らない命令: %s", a[0])
}

// listMediaInputs はメディアソース（ffmpeg_source）の名前、ハードウェアデコード、再生の状態を出す。URL は出さない。
func listMediaInputs(c *goobs.Client) error {
	kind := "ffmpeg_source"
	r, err := c.Inputs.GetInputList(&inputs.GetInputListParams{InputKind: &kind})
	if err != nil {
		return err
	}
	names := make([]string, 0, len(r.Inputs))
	for _, in := range r.Inputs {
		names = append(names, in.InputName)
	}
	sort.Strings(names)
	for _, n := range names {
		n := n
		hw := "?"
		if s, err := c.Inputs.GetInputSettings(&inputs.GetInputSettingsParams{InputName: &n}); err == nil {
			hw = fmt.Sprint(s.InputSettings["hw_decode"] == true)
		}
		st, err := c.MediaInputs.GetMediaInputStatus(&mediainputs.GetMediaInputStatusParams{InputName: &n})
		if err != nil {
			fmt.Printf("%s hw_decode=%s status=? (%v)\n", n, hw, err)
			continue
		}
		fmt.Printf("%s hw_decode=%s %s cursor=%v\n", n, hw, st.MediaState, st.MediaCursor)
	}
	return nil
}
