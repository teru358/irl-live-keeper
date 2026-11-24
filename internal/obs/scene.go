package obs

import (
	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"sort"
)

// Scene は管理画面へ渡すシーン。
type Scene struct {
	Kind       string   `json:"kind"`
	Cameras    []string `json:"cameras"`
	FallbackOf []string `json:"fallback_of"`
	UUID       string   `json:"uuid"`
	Name       string   `json:"name"`
}

// Input はメディアソース選択画面に渡す OBS 入力。
type Input struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func (w *WS) Scenes() ([]Scene, error) {
	r, err := w.aux.GetSceneList()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(r.Scenes, func(i, j int) bool { return r.Scenes[i].SceneIndex > r.Scenes[j].SceneIndex })
	out := make([]Scene, 0, len(r.Scenes))
	for _, s := range r.Scenes {
		out = append(out, Scene{UUID: s.SceneUuid, Name: s.SceneName})
	}
	return out, nil
}

// Inputs は OBS の全入力を補助の接続で取得する。
func (w *WS) Inputs() ([]Input, error) {
	r, err := w.aux.GetInputList("")
	if err != nil {
		return nil, err
	}
	out := make([]Input, 0, len(r.Inputs))
	for _, in := range r.Inputs {
		if in != nil {
			out = append(out, Input{UUID: in.InputUuid, Name: in.InputName, Kind: in.InputKind})
		}
	}
	return out, nil
}

func (w *WS) SelectScene(uuid string) error {
	_, err := auxCall(w.aux, "SetCurrentProgramScene", func(c *goobs.Client) (*scenes.SetCurrentProgramSceneResponse, error) {
		return c.Scenes.SetCurrentProgramScene(scenes.NewSetCurrentProgramSceneParams().WithSceneUuid(uuid))
	})
	return err
}
