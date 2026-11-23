package core

import (
	"context"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/switcher"
	"sync/atomic"
	"time"
)

var ErrSceneMissing = errors.New("シーンが見つからない")

type Scene = obs.Scene

type sceneAccess interface {
	Scenes() ([]obs.Scene, error)
	SelectScene(string) error
}

func (c *Core) Scenes() ([]obs.Scene, error) {
	if a, ok := c.obs.(sceneAccess); ok {
		list, err := a.Scenes()
		details := c.Snapshot().SceneDetails
		for i, sc := range list {
			if d, ok := details[sc.UUID]; ok {
				list[i] = d
				list[i].Name = sc.Name
			} else {
				list[i].Kind = "none"
				list[i].Cameras = []string{}
				list[i].FallbackOf = []string{}
			}
		}
		return list, err
	}
	return nil, obs.ErrAuxUnavailable
}

var sceneIntentID atomic.Uint64
var errSceneAlreadyCurrent = errors.New("今のシーン")

// SelectScene は意図を先に中核へ登録し、補助接続の要求は呼び出し側で待つ。
func (c *Core) SelectScene(ctx context.Context, uuid string) error {
	a, ok := c.obs.(sceneAccess)
	if !ok || !c.Snapshot().OBS.Connected {
		return obs.ErrAuxUnavailable
	}
	list, err := a.Scenes()
	if err != nil {
		return err
	}
	found := false
	for _, s := range list {
		if s.UUID == uuid {
			found = true
		}
	}
	if !found {
		return ErrSceneMissing
	}

	intent := switcher.HumanSceneSelected{ID: sceneIntentID.Add(1), SceneUUID: uuid}
	if err := c.sendSceneIntent(ctx, intent); err != nil {
		if errors.Is(err, errSceneAlreadyCurrent) {
			return nil
		}
		return err
	}
	err = a.SelectScene(uuid)
	if err != nil && !errors.Is(err, obs.ErrAuxUncertain) && (errors.Is(err, obs.ErrAuxRejected) || errors.Is(err, obs.ErrAuxUnavailable) || errors.Is(err, obs.ErrAuxNotSent)) {
		intent.Cancel = true
		// 登録済みの意図の後始末は、呼び出し元の期限に左右されない。
		_ = c.sendSceneIntent(context.Background(), intent)
	}
	return err
}

func (c *Core) sendSceneIntent(ctx context.Context, intent switcher.HumanSceneSelected) error {
	req := cmdReq{intent: &intent, reply: make(chan error, 1)}
	select {
	case c.cmds <- req:
	case <-c.stopped:
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	// 受理された入力の結果まで確認してから OBS に送る。
	select {
	case err := <-req.reply:
		return err
	case <-c.stopped:
		return ErrStopped
	}
}

func (c *Core) selectScene(intent switcher.HumanSceneSelected) error {
	if !intent.Cancel && !c.m.Snapshot().Connected {
		return obs.ErrAuxUnavailable
	}
	intent.At = c.now()
	intent.Deadline = intent.At.Add(7 * time.Second) // 補助接続の2秒 + 余裕5秒
	same := !intent.Cancel && c.m.Snapshot().Scene == intent.SceneUUID
	err := c.handle(intent)
	if err == nil && same {
		return errSceneAlreadyCurrent
	}
	return err
}
