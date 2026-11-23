package core

import (
	"context"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

var ErrSceneMissing = errors.New("シーンが見つからない")

type Scene = obs.Scene

type sceneAccess interface {
	Scenes() ([]obs.Scene, error)
	SelectScene(string) error
}

func (c *Core) Scenes() ([]obs.Scene, error) {
	if a, ok := c.obs.(sceneAccess); ok {
		return a.Scenes()
	}
	return nil, obs.ErrAuxUnavailable
}

// SelectScene は補助の接続でシーンを切り替え、成功したら人の切替の入力を中核ループへ渡す。
// OBS への要求（一覧の確認と切り替え。それぞれ補助の接続の期限つき）は呼び出し側の goroutine で行い、
// 中核ループを待たせない（コアの spec 9章: 中核ループは止まらない）。OBS からの SceneChanged が
// この入力より先に届いても、あとの入力で期待・スタック・持ち主を消すので、結果は人の切替になる。
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
	if err := a.SelectScene(uuid); err != nil {
		return err
	}
	req := cmdReq{scene: uuid, reply: make(chan error, 1)}
	select {
	case c.cmds <- req:
	case <-c.stopped:
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// selectScene は中核ループの中で、人の切替の入力を Machine に渡すだけ（OBS には何も要求しない）。
func (c *Core) selectScene(uuid string) error {
	return c.handle(switcher.HumanSceneSelected{At: c.now(), SceneUUID: uuid})
}
