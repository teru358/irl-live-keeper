package obs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/mediainputs"
	"github.com/andreykaipov/goobs/api/requests/scenes"
)

var (
	errNotConnected = errors.New("OBS に未接続")
	// errSwitching は、シーンコレクションの切替中で、要求を送らなかったこと。
	errSwitching = errors.New("OBS のシーンコレクションを切り替え中")
	// errBusy は、前の要求が goobs の中で終わらず、送る順番が来なかったこと（送っていない）。
	errBusy = errors.New("OBS の前の要求が終わらず、送れなかった")
	// errConnDropped は、待っている間に Client を捨てたため、送らなかったこと（適用されていない）。
	errConnDropped = errors.New("OBS の接続を捨てたため、要求を送らなかった")
	errWSStopped   = errors.New("OBS への接続は終了した")
)

// mainConn は本来の接続の goobs の Client 1本。dead が閉じられたら、捨てられている（以後の要求を入れない）。
type mainConn struct {
	cli *goobs.Client
	rec *connRecorder
	// legacy は obs-websocket 5.4.0 未満。つないだときに1回だけ決める。
	legacy atomic.Bool

	// gate は goobs の中に入る要求を1本に絞る（容量1）。goobs は応答を1本の列から順に読むので、
	// 要求が Timeout したあとの遅れた応答が、次の要求の応答を取り違えさせる。
	gate chan struct{}

	deadOnce sync.Once
	dead     chan struct{}
	reason   error // dead を閉じる前に書く
}

func newMainConn(cli *goobs.Client, rec *connRecorder) *mainConn {
	return &mainConn{cli: cli, rec: rec, gate: make(chan struct{}, 1), dead: make(chan struct{})}
}

// kill は Client を捨てると決める。何度呼んでもよい（最初の理由だけ残る）。
func (c *mainConn) kill(reason error) {
	c.deadOnce.Do(func() {
		c.reason = reason
		close(c.dead)
	})
}

// drop は kill のあと、接続を閉じて Disconnect を片づけの goroutine で呼ぶ。
// rec.close で止まっている読み書きを戻し、Disconnect は goobs の要求の mutex を取るので、
// 止まった要求があれば、その応答待ちが切れるまで戻らないことがある。
func (w *WS) dropConn(c *mainConn, reason error) {
	w.mu.Lock()
	if w.cur == c {
		w.cur, w.open = nil, false
	}
	w.mu.Unlock()
	c.kill(reason)
	c.rec.close()
	w.spawn(func() { _ = c.cli.Disconnect() })
}

// client は外からの要求を送る Client。門が閉じている間は送らずに失敗する。
func (w *WS) client() (extConn, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	switch {
	case w.cur == nil:
		return extConn{}, errNotConnected
	case !w.open && w.switching:
		return extConn{}, errSwitching
	case !w.open:
		return extConn{}, errNotConnected
	}
	return extConn{c: w.cur, gen: w.gen}, nil
}

// extConn は、外からの要求を受け付けたときの Client と、門の世代。
type extConn struct {
	c   *mainConn
	gen uint64
}

// pre は送る直前の確かめ。門が閉じたか、世代が変わっていたら、送らずに失敗する。
// 門を閉じる処理（setGate）は同じ mutex を取るので、閉じ終えたあとに確かめる要求は必ず止まる。
// 既知の制約: 確かめてから goobs に書くまでの間に門が閉じられると、その要求は「閉じる時点で送信中だった
// 要求」と同じ扱いになる（goobs は送信の順番を見せないので、排他にできない）。
func (w *WS) pre(e extConn) func() error {
	return func() error {
		w.mu.RLock()
		defer w.mu.RUnlock()
		switch {
		case w.cur != e.c || w.gen != e.gen:
			return errConnDropped
		case !w.open && w.switching:
			return errSwitching
		case !w.open:
			return errNotConnected
		}
		return nil
	}
}

// setGate は門を開け閉めする。c が今の Client のときだけ。
func (w *WS) setGate(c *mainConn, open, switching bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cur != c {
		return
	}
	if open && !w.open {
		w.gen++
	}
	w.open, w.switching = open, switching
}

// callResult は要求の結果。
type callResult[T any] struct {
	v   T
	err error
}

// mcall は fn を、Client c に対して1本ずつ、期限つきで呼ぶ。
//   - goobs の中に入れるのは1本だけ（gate）。待つ間も、捨てられたか ctx が終わったかを見る。
//   - 応答待ちの超過か ID のずれ（ErrTimeout）なら、gate を離す前に Client を捨てる。
//   - goobs が戻らなくても、呼び出し側は timeout + callMargin で戻り、Client を捨てる（ErrTimeout）。
//
// fn の結果は channel だけで受け取る（呼び出し側が戻ったあとも fn の goroutine は残りうる）。
// 送らなかった要求は、ErrTimeout にしない（適用されていないので）。
func mcall[T any](w *WS, ctx context.Context, c *mainConn, op string, fn func(*goobs.Client) (T, error)) (T, error) {
	return mcallPre(w, ctx, c, op, nil, fn)
}

// mcallPre は mcall に、送る直前の確かめ pre を足したもの。外からの要求は、受け付けたときの門の世代を
// 持ち、gate を取って goobs に渡す直前に、門が開いていて世代が同じことを確かめる（待っている間に
// コレクションの切替が始まっていたら送らない）。pre が nil なら確かめない（内部の読み取り）。
func mcallPre[T any](w *WS, ctx context.Context, c *mainConn, op string, pre func() error, fn func(*goobs.Client) (T, error)) (T, error) {
	var zero T
	wait := time.NewTimer(w.timeout)
	defer wait.Stop()
	select {
	case c.gate <- struct{}{}:
	case <-c.dead:
		return zero, fmt.Errorf("%w: %s（%v）", errConnDropped, op, c.reason)
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-wait.C:
		return zero, fmt.Errorf("%w: %s", errBusy, op)
	}
	select {
	case <-c.dead: // 待っている間に捨てられた。goobs には入れない。
		<-c.gate
		return zero, fmt.Errorf("%w: %s（%v）", errConnDropped, op, c.reason)
	default:
	}

	res := make(chan callResult[T], 1)
	if !w.spawn(func() {
		if pre != nil {
			if err := pre(); err != nil {
				<-c.gate
				res <- callResult[T]{zero, fmt.Errorf("%w: %s", err, op)}
				return
			}
		}
		v, err := fn(c.cli)
		err = classifyError(err)
		if errors.Is(err, ErrTimeout) {
			// gate を離す前に捨てる（次の要求が、応答の列がずれた Client に入らないように）。
			c.kill(err)
		}
		<-c.gate
		res <- callResult[T]{v, err}
	}) {
		<-c.gate
		return zero, errWSStopped
	}

	limit := time.NewTimer(w.timeout + callMargin)
	defer limit.Stop()
	select {
	case r := <-res:
		return r.v, r.err
	case <-c.dead:
		select { // 自分の Timeout で捨てたときは、その結果を返す
		case r := <-res:
			return r.v, r.err
		default:
		}
		return zero, fmt.Errorf("%w: %s（%v）", ErrTimeout, op, c.reason)
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-limit.C:
		err := fmt.Errorf("%w: %s（goobs が戻らない）", ErrTimeout, op)
		c.kill(err)
		return zero, err
	}
}

// isLegacyVersion は obs-websocket の版が 5.4.0 より古い（か、読めない）こと。
// UUID の項目と UUID の要求は 5.4.0 で入った。rpcVersion では見分けられない。
func isLegacyVersion(v string) bool {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return true
	}
	major, ok1 := leadingInt(parts[0])
	minor, ok2 := leadingInt(parts[1])
	if !ok1 || !ok2 {
		return true
	}
	return major < 5 || (major == 5 && minor < 4)
}

// leadingInt は s の頭の数字を読む（"4-beta1" は 4）。
func leadingInt(s string) (int, bool) {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(s[:end])
	return n, err == nil
}

// 以下は外からの要求。門が閉じている間と、未接続のときは、送らずに失敗する。

// SceneTransitionOverrideUUID は切替先シーンのトランジションの上書きを sceneUuid で読む（古い OBS では名前）。
// 上書きなしは has=false。長さがなければ ms=nil で has=true。
func (w *WS) SceneTransitionOverrideUUID(uuid string) (ms *int, has bool, err error) {
	e, err := w.client()
	if err != nil {
		return nil, false, err
	}
	p := scenes.NewGetSceneSceneTransitionOverrideParams()
	if e.c.legacy.Load() {
		p.WithSceneName(uuid)
	} else {
		p.WithSceneUuid(uuid)
	}
	return w.transitionOverride(e, p)
}

func (w *WS) transitionOverride(e extConn, p *scenes.GetSceneSceneTransitionOverrideParams) (*int, bool, error) {
	r, err := mcallPre(w, context.Background(), e.c, "GetSceneSceneTransitionOverride", w.pre(e), func(cli *goobs.Client) (*scenes.GetSceneSceneTransitionOverrideResponse, error) {
		return cli.Scenes.GetSceneSceneTransitionOverride(p)
	})
	if err != nil {
		return nil, false, err
	}
	ms, has := overrideFromResponse(r.TransitionName, r.TransitionDuration)
	return ms, has, nil
}

// SetSceneUUID は program のシーンを sceneUuid で切り替える（古い OBS では名前）。
func (w *WS) SetSceneUUID(uuid string) error {
	e, err := w.client()
	if err != nil {
		return err
	}
	p := scenes.NewSetCurrentProgramSceneParams()
	if e.c.legacy.Load() {
		p.WithSceneName(uuid)
	} else {
		p.WithSceneUuid(uuid)
	}
	return w.setScene(e, p)
}

func (w *WS) setScene(e extConn, p *scenes.SetCurrentProgramSceneParams) error {
	_, err := mcallPre(w, context.Background(), e.c, "SetCurrentProgramScene", w.pre(e), func(cli *goobs.Client) (*scenes.SetCurrentProgramSceneResponse, error) {
		return cli.Scenes.SetCurrentProgramScene(p)
	})
	return err
}

const restartAction = "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART"

// RestartMediaInputs は入力の UUID の列の順に RESTART を送る（古い OBS では名前）。
// 入力ごとの失敗（入力がない）は、続けて残りを送り、最初の失敗を返す。接続が使えなくなる失敗
// （Timeout、切替中、未接続）では、そこで止める。
func (w *WS) RestartMediaInputs(inputUUIDs []string) error {
	var first error
	for _, id := range inputUUIDs {
		e, err := w.client()
		if err != nil {
			return err
		}
		p := mediainputs.NewTriggerMediaInputActionParams().WithMediaAction(restartAction)
		if e.c.legacy.Load() {
			p.WithInputName(id)
		} else {
			p.WithInputUuid(id)
		}
		if err := w.restart(e, p); err != nil {
			if first == nil {
				first = err
			}
			if sessionLost(err) {
				return first
			}
		}
	}
	return first
}

// sessionLost は、以後の要求も送れない失敗。
func sessionLost(err error) bool {
	return errors.Is(err, ErrTimeout) || errors.Is(err, errNotConnected) || errors.Is(err, errSwitching) ||
		errors.Is(err, errConnDropped) || errors.Is(err, errWSStopped)
}

func (w *WS) restart(e extConn, p *mediainputs.TriggerMediaInputActionParams) error {
	_, err := mcallPre(w, context.Background(), e.c, "TriggerMediaInputAction", w.pre(e), func(cli *goobs.Client) (*mediainputs.TriggerMediaInputActionResponse, error) {
		return cli.MediaInputs.TriggerMediaInputAction(p)
	})
	return err
}

// StartStream は配信を始める。
func (w *WS) StartStream() error {
	e, err := w.client()
	if err != nil {
		return err
	}
	_, err = mcallPre(w, context.Background(), e.c, "StartStream", w.pre(e), func(cli *goobs.Client) (struct{}, error) {
		_, err := cli.Stream.StartStream()
		return struct{}{}, err
	})
	return err
}

// StopStream は配信を止める。
func (w *WS) StopStream() error {
	e, err := w.client()
	if err != nil {
		return err
	}
	_, err = mcallPre(w, context.Background(), e.c, "StopStream", w.pre(e), func(cli *goobs.Client) (struct{}, error) {
		_, err := cli.Stream.StopStream()
		return struct{}{}, err
	})
	return err
}
