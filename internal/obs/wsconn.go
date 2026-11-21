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
func (w *WS) client() (*mainConn, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	switch {
	case w.cur == nil:
		return nil, errNotConnected
	case !w.open && w.switching:
		return nil, errSwitching
	case !w.open:
		return nil, errNotConnected
	}
	return w.cur, nil
}

// setGate は門を開け閉めする。c が今の Client のときだけ。
func (w *WS) setGate(c *mainConn, open, switching bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cur != c {
		return
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

// SceneTransitionOverride は切替先シーンのトランジションの上書きを読む。
// 上書きなしは has=false。長さがなければ ms=nil で has=true。
func (w *WS) SceneTransitionOverride(scene string) (ms *int, has bool, err error) {
	c, err := w.client()
	if err != nil {
		return nil, false, err
	}
	return w.transitionOverride(c, scenes.NewGetSceneSceneTransitionOverrideParams().WithSceneName(scene))
}

// SceneTransitionOverrideUUID は sceneUuid で読む（古い OBS では名前）。
func (w *WS) SceneTransitionOverrideUUID(uuid string) (ms *int, has bool, err error) {
	c, err := w.client()
	if err != nil {
		return nil, false, err
	}
	p := scenes.NewGetSceneSceneTransitionOverrideParams()
	if c.legacy.Load() {
		p.WithSceneName(uuid)
	} else {
		p.WithSceneUuid(uuid)
	}
	return w.transitionOverride(c, p)
}

func (w *WS) transitionOverride(c *mainConn, p *scenes.GetSceneSceneTransitionOverrideParams) (*int, bool, error) {
	r, err := mcall(w, context.Background(), c, "GetSceneSceneTransitionOverride", func(cli *goobs.Client) (*scenes.GetSceneSceneTransitionOverrideResponse, error) {
		return cli.Scenes.GetSceneSceneTransitionOverride(p)
	})
	if err != nil {
		return nil, false, err
	}
	ms, has := overrideFromResponse(r.TransitionName, r.TransitionDuration)
	return ms, has, nil
}

// SetScene は program のシーンを名前で切り替える。応答待ちの超過は ErrTimeout。
func (w *WS) SetScene(name string) error {
	c, err := w.client()
	if err != nil {
		return err
	}
	return w.setScene(c, scenes.NewSetCurrentProgramSceneParams().WithSceneName(name))
}

// SetSceneUUID は program のシーンを sceneUuid で切り替える（古い OBS では名前）。
func (w *WS) SetSceneUUID(uuid string) error {
	c, err := w.client()
	if err != nil {
		return err
	}
	p := scenes.NewSetCurrentProgramSceneParams()
	if c.legacy.Load() {
		p.WithSceneName(uuid)
	} else {
		p.WithSceneUuid(uuid)
	}
	return w.setScene(c, p)
}

func (w *WS) setScene(c *mainConn, p *scenes.SetCurrentProgramSceneParams) error {
	_, err := mcall(w, context.Background(), c, "SetCurrentProgramScene", func(cli *goobs.Client) (*scenes.SetCurrentProgramSceneResponse, error) {
		return cli.Scenes.SetCurrentProgramScene(p)
	})
	return err
}

const restartAction = "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART"

// RestartMedia はメディアソースに RESTART を送る。
func (w *WS) RestartMedia(input string) error {
	c, err := w.client()
	if err != nil {
		return err
	}
	return w.restart(c, mediainputs.NewTriggerMediaInputActionParams().WithInputName(input).WithMediaAction(restartAction))
}

// RestartMediaInputs は入力の UUID の列の順に RESTART を送る（古い OBS では名前）。
// 入力ごとの失敗（入力がない）は、続けて残りを送り、最初の失敗を返す。接続が使えなくなる失敗
// （Timeout、切替中、未接続）では、そこで止める。
func (w *WS) RestartMediaInputs(inputUUIDs []string) error {
	var first error
	for _, id := range inputUUIDs {
		c, err := w.client()
		if err != nil {
			return err
		}
		p := mediainputs.NewTriggerMediaInputActionParams().WithMediaAction(restartAction)
		if c.legacy.Load() {
			p.WithInputName(id)
		} else {
			p.WithInputUuid(id)
		}
		if err := w.restart(c, p); err != nil {
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

func (w *WS) restart(c *mainConn, p *mediainputs.TriggerMediaInputActionParams) error {
	_, err := mcall(w, context.Background(), c, "TriggerMediaInputAction", func(cli *goobs.Client) (*mediainputs.TriggerMediaInputActionResponse, error) {
		return cli.MediaInputs.TriggerMediaInputAction(p)
	})
	return err
}

// StartStream は配信を始める。
func (w *WS) StartStream() error {
	c, err := w.client()
	if err != nil {
		return err
	}
	_, err = mcall(w, context.Background(), c, "StartStream", func(cli *goobs.Client) (struct{}, error) {
		_, err := cli.Stream.StartStream()
		return struct{}{}, err
	})
	return err
}

// StopStream は配信を止める。
func (w *WS) StopStream() error {
	c, err := w.client()
	if err != nil {
		return err
	}
	_, err = mcall(w, context.Background(), c, "StopStream", func(cli *goobs.Client) (struct{}, error) {
		_, err := cli.Stream.StopStream()
		return struct{}{}, err
	})
	return err
}
