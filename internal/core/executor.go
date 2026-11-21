package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// runExecutor は実行役。バッチを1つずつ受け取り、BatchDone を送り終えてから次を受け取る。
func (c *Core) runExecutor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-c.batches:
			c.execute(ctx, b)
		}
	}
}

// execute はバッチの Action を順に実行する。各 Action の直前に承認を取る。
// RestartMedia の失敗では続け、それ以外の失敗で残りを捨てる。
// ctx が終わったら、何も送らずに抜ける。
func (c *Core) execute(ctx context.Context, b *switcher.Batch) {
	var partial error // RestartMedia の失敗。続く Action が成功すれば Completed の Err になる
	for _, a := range b.Actions {
		set, isSet := a.(switcher.SetScene)
		var override switcher.Override = switcher.OverrideNone{}
		if isSet {
			name := set.SceneUUID
			r, ok := callOBS(ctx, func() overrideResult {
				ms, has, err := c.obs.SceneTransitionOverride(name)
				return overrideResult{ms, has, err}
			})
			if !ok {
				return
			}
			override = obs.ToOverride(r.ms, r.has, r.err)
		}

		res, ok := c.requestApproval(ctx, b, a, override)
		if !ok {
			return
		}
		var reqID int
		switch r := res.(type) {
		case switcher.Proceed:
			reqID = r.ReqID
		case switcher.Skip:
			continue
		default: // Reject、Replace（代わりのバッチは中核ループが列の先頭に入れている）
			c.report(ctx, switcher.BatchDone{At: c.now(), BatchID: b.BatchID, Result: switcher.BatchRejected})
			return
		}

		err, ok := callOBS(ctx, func() error { return c.send(a) })
		if !ok {
			return
		}
		if isSet {
			if !c.report(ctx, switcher.SceneRequestDone{At: c.now(), ReqID: reqID, Result: requestResult(err)}) {
				return
			}
		}
		if err != nil {
			err = fmt.Errorf("%s: %w", actionName(a), err)
			c.log.Warn("OBS の操作に失敗", "batch", b.BatchID, "purpose", b.Purpose.String(), "error", err)
			if _, isRestart := a.(switcher.RestartMedia); isRestart {
				// 映像の再始動に失敗しても、待機画面からは戻す。
				partial = err
				continue
			}
			c.report(ctx, switcher.BatchDone{At: c.now(), BatchID: b.BatchID, Result: switcher.BatchFailed, Err: err})
			return
		}
		if rm, ok := a.(switcher.RestartMedia); ok && rm.Wait > 0 {
			if !sleepCtx(ctx, rm.Wait) {
				return
			}
		}
	}
	c.report(ctx, switcher.BatchDone{At: c.now(), BatchID: b.BatchID, Result: switcher.BatchCompleted, Err: partial})
}

// send は Action を OBS に送る。
func (c *Core) send(a switcher.Action) error {
	switch a := a.(type) {
	case switcher.SetScene:
		return c.obs.SetScene(a.SceneUUID)
	case switcher.RestartMedia:
		// Inputs の入力に順に送る。1つ失敗しても残りに送り、最初の失敗を返す。待ちは呼び出し側が最後に1回行う。
		var first error
		for _, in := range a.Inputs {
			if err := c.obs.RestartMedia(in); err != nil && first == nil {
				first = err
			}
		}
		return first
	case switcher.StartStream:
		return c.obs.StartStream()
	case switcher.StopStream:
		return c.obs.StopStream()
	}
	return fmt.Errorf("未知の Action: %T", a)
}

// requestApproval は承認要求を中核ループに送り、応答を待つ。ctx が終われば ok=false。
func (c *Core) requestApproval(ctx context.Context, b *switcher.Batch, a switcher.Action, o switcher.Override) (switcher.ApproveResult, bool) {
	req := approveReq{batchID: b.BatchID, purpose: b.Purpose, action: a, override: o, reply: make(chan switcher.ApproveResult, 1)}
	select {
	case c.approvals <- req:
	case <-ctx.Done():
		return nil, false
	}
	select {
	case res := <-req.reply:
		return res, true
	case <-ctx.Done():
		return nil, false
	}
}

// report は SceneRequestDone か BatchDone を中核ループに送る。ctx が終われば false。
func (c *Core) report(ctx context.Context, ev switcher.Event) bool {
	select {
	case c.results <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

type overrideResult struct {
	ms  *int
	has bool
	err error
}

// callOBS は obs.Client の呼び出しを別の goroutine で行い、結果か ctx の終わりを待つ。
// obs.Client は ctx を受け取らないため、こうしないと終了時に応答を待ち続けてしまう。
// ctx が先に終わったら、呼び出しの結果は容量1のチャネルに捨てられ、goroutine は応答の時点で終わる。
func callOBS[T any](ctx context.Context, f func() T) (T, bool) {
	ch := make(chan T, 1)
	go func() { ch <- f() }()
	select {
	case v := <-ch:
		return v, true
	case <-ctx.Done():
		var zero T
		return zero, false
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func requestResult(err error) switcher.RequestResult {
	switch {
	case err == nil:
		return switcher.RequestOK
	case errors.Is(err, obs.ErrTimeout):
		return switcher.RequestTimeout
	}
	return switcher.RequestError
}

func actionName(a switcher.Action) string {
	switch a := a.(type) {
	case switcher.SetScene:
		return "SetScene(" + a.SceneUUID + ")"
	case switcher.RestartMedia:
		return "RestartMedia"
	case switcher.StartStream:
		return "StartStream"
	case switcher.StopStream:
		return "StopStream"
	}
	return fmt.Sprintf("%T", a)
}
