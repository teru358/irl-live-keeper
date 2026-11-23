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

func (w *WS) SelectScene(uuid string) error {
	_, err := auxCall(w.aux, "SetCurrentProgramScene", func(c *goobs.Client) (*scenes.SetCurrentProgramSceneResponse, error) {
		return c.Scenes.SetCurrentProgramScene(scenes.NewSetCurrentProgramSceneParams().WithSceneUuid(uuid))
	})
	return err
}
