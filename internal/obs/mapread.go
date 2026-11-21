package obs

import (
	"context"
	"fmt"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/config"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/typedefs"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// CameraConfig はカメラの OBS の側の照合に使う設定。
type CameraConfig struct {
	// ID は MediaMTX の path（カメラの id）。メディアソースの URL の path と照合する。
	ID string
	// Name は警告に出す表示名。空なら ID。
	Name string
}

// mapRequester は地図を読むための要求。補助の接続（Aux）と、本来の接続（mainReq）が満たす。
// シーン、グループ、入力は UUID で指す。
type mapRequester interface {
	GetSceneCollectionList() (*config.GetSceneCollectionListResponse, error)
	GetSceneList() (*scenes.GetSceneListResponse, error)
	GetSceneItemList(sceneUUID string) (*sceneitems.GetSceneItemListResponse, error)
	GetGroupSceneItemList(sceneUUID string) (*sceneitems.GetGroupSceneItemListResponse, error)
	GetInputList(inputKind string) (*inputs.GetInputListResponse, error)
	GetInputSettings(inputUUID string) (*inputs.GetInputSettingsResponse, error)
}

var _ mapRequester = (*Aux)(nil)

// mainReq は本来の接続（Client c）で地図を読むための mapRequester。要求は門を通らない
// （門が閉じている間の読み取りも、この接続の持ち主が行うため）が、gate と期限は通る。
type mainReq struct {
	w   *WS
	ctx context.Context
	c   *mainConn
}

func (r mainReq) GetSceneCollectionList() (*config.GetSceneCollectionListResponse, error) {
	return mcall(r.w, r.ctx, r.c, "GetSceneCollectionList", func(c *goobs.Client) (*config.GetSceneCollectionListResponse, error) {
		return c.Config.GetSceneCollectionList()
	})
}

func (r mainReq) GetSceneList() (*scenes.GetSceneListResponse, error) {
	return mcall(r.w, r.ctx, r.c, "GetSceneList", func(c *goobs.Client) (*scenes.GetSceneListResponse, error) {
		return c.Scenes.GetSceneList()
	})
}

func (r mainReq) GetSceneItemList(uuid string) (*sceneitems.GetSceneItemListResponse, error) {
	return mcall(r.w, r.ctx, r.c, "GetSceneItemList", func(c *goobs.Client) (*sceneitems.GetSceneItemListResponse, error) {
		return c.SceneItems.GetSceneItemList(sceneitems.NewGetSceneItemListParams().WithSceneUuid(uuid))
	})
}

func (r mainReq) GetGroupSceneItemList(uuid string) (*sceneitems.GetGroupSceneItemListResponse, error) {
	return mcall(r.w, r.ctx, r.c, "GetGroupSceneItemList", func(c *goobs.Client) (*sceneitems.GetGroupSceneItemListResponse, error) {
		return c.SceneItems.GetGroupSceneItemList(sceneitems.NewGetGroupSceneItemListParams().WithSceneUuid(uuid))
	})
}

func (r mainReq) GetInputList(kind string) (*inputs.GetInputListResponse, error) {
	return mcall(r.w, r.ctx, r.c, "GetInputList", func(c *goobs.Client) (*inputs.GetInputListResponse, error) {
		p := inputs.NewGetInputListParams()
		if kind != "" {
			p.WithInputKind(kind)
		}
		return c.Inputs.GetInputList(p)
	})
}

func (r mainReq) GetInputSettings(uuid string) (*inputs.GetInputSettingsResponse, error) {
	return mcall(r.w, r.ctx, r.c, "GetInputSettings", func(c *goobs.Client) (*inputs.GetInputSettingsResponse, error) {
		return c.Inputs.GetInputSettings(inputs.NewGetInputSettingsParams().WithInputUuid(uuid))
	})
}

const ffmpegSource = "ffmpeg_source"

// readSceneMap は地図を全部読む（カメラの spec 第3節）。
//
// 要求の順: GetSceneCollectionList、GetSceneList、シーンごとの GetSceneItemList（OBS の順）、
// 見つけたグループごとの GetGroupSceneItemList（見つけた順。グループは何か所に出ても1回）、
// GetInputList（ffmpeg_source）、入力ごとの GetInputSettings。どれか1つでも失敗したら、全体が失敗。
// ctx が終わったら、次の要求を送らずに戻る。
func readSceneMap(ctx context.Context, req mapRequester, cameraIDs []string, rtspPort int) (*scenemap.Map, error) {
	m, list, err := readCollectionAndScenes(ctx, req, false)
	if err != nil {
		return nil, err
	}
	type pending struct {
		uuid  string
		group bool
	}
	queue := make([]pending, 0, len(list))
	for _, sc := range list {
		queue = append(queue, pending{uuid: sc.SceneUuid})
	}
	seenGroup := map[string]bool{}
	for i := 0; i < len(queue); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p := queue[i]
		var items []*typedefs.SceneItem
		if p.group {
			r, err := req.GetGroupSceneItemList(p.uuid)
			if err != nil {
				return nil, fmt.Errorf("GetGroupSceneItemList: %w", err)
			}
			items = r.SceneItems
		} else {
			r, err := req.GetSceneItemList(p.uuid)
			if err != nil {
				return nil, fmt.Errorf("GetSceneItemList: %w", err)
			}
			items = r.SceneItems
		}
		out := make([]scenemap.Item, 0, len(items))
		for _, it := range items {
			kind := scenemap.InputSource
			switch {
			case it.IsGroup:
				kind = scenemap.GroupSource
				if !seenGroup[it.SourceUuid] {
					seenGroup[it.SourceUuid] = true
					m.Groups[it.SourceUuid] = scenemap.Container{Name: it.SourceName}
					queue = append(queue, pending{uuid: it.SourceUuid, group: true})
				}
			case it.SourceType == "OBS_SOURCE_TYPE_SCENE":
				kind = scenemap.SceneSource
			}
			out = append(out, scenemap.Item{ID: it.SceneItemID, SourceUUID: it.SourceUuid, SourceKind: kind, Enabled: it.SceneItemEnabled})
		}
		if p.group {
			c := m.Groups[p.uuid]
			c.Items = out
			m.Groups[p.uuid] = c
		} else {
			c := m.Scenes[p.uuid]
			c.Items = out
			m.Scenes[p.uuid] = c
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	il, err := req.GetInputList(ffmpegSource)
	if err != nil {
		return nil, fmt.Errorf("GetInputList: %w", err)
	}
	for _, in := range il.Inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		st, err := req.GetInputSettings(in.InputUuid)
		if err != nil {
			return nil, fmt.Errorf("GetInputSettings: %w", err)
		}
		id, _ := matchInputSettings(st.InputSettings, cameraIDs, rtspPort)
		m.Inputs[in.InputUuid] = scenemap.Input{Name: in.InputName, CameraID: id}
	}
	return m, nil
}

// readSceneListOnly はシーンの一覧だけの地図を読む（項目とカメラの対応は空）。byName が true なら
// 古い OBS 用で、シーンの名前を鍵にし、コレクションの名前は読まない。
func readSceneListOnly(ctx context.Context, req mapRequester, byName bool) (*scenemap.Map, error) {
	m, _, err := readCollectionAndScenes(ctx, req, byName)
	return m, err
}

func readCollectionAndScenes(ctx context.Context, req mapRequester, byName bool) (*scenemap.Map, []*typedefs.Scene, error) {
	m := &scenemap.Map{
		Scenes: map[string]scenemap.Container{},
		Groups: map[string]scenemap.Container{},
		Inputs: map[string]scenemap.Input{},
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if !byName {
		cl, err := req.GetSceneCollectionList()
		if err != nil {
			return nil, nil, fmt.Errorf("GetSceneCollectionList: %w", err)
		}
		m.Collection = cl.CurrentSceneCollectionName
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	sl, err := req.GetSceneList()
	if err != nil {
		return nil, nil, fmt.Errorf("GetSceneList: %w", err)
	}
	for _, sc := range sl.Scenes {
		key := sc.SceneUuid
		if byName {
			key = sc.SceneName
		}
		m.Scenes[key] = scenemap.Container{Name: sc.SceneName}
	}
	return m, sl.Scenes, nil
}

// matchInputSettings は入力の設定（GetInputSettings か InputSettingsChanged の inputSettings）を
// カメラの id に照合する。is_local_file は、なければ既定（true）なので対象にならない。
func matchInputSettings(settings map[string]any, cameraIDs []string, rtspPort int) (string, bool) {
	url, _ := settings["input"].(string)
	var local *bool
	if v, ok := settings["is_local_file"].(bool); ok {
		local = &v
	}
	return scenemap.MatchCamera(url, local, rtspPort, cameraIDs)
}

// cameraWarnings は、メディアソースが見つからないカメラの警告（設定の順）。
func cameraWarnings(cameras []CameraConfig, m *scenemap.Map) []string {
	have := map[string]bool{}
	for _, in := range m.Inputs {
		if in.CameraID != "" {
			have[in.CameraID] = true
		}
	}
	var out []string
	for _, c := range cameras {
		if have[c.ID] {
			continue
		}
		name := c.Name
		if name == "" {
			name = c.ID
		}
		out = append(out, name+" のメディアソースが OBS に見つかりません")
	}
	return out
}
