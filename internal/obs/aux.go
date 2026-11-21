package obs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events/subscriptions"
	"github.com/andreykaipov/goobs/api/requests/config"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
)

// 補助の接続の要求が返すエラー。errors.Is で区別する。
var (
	// ErrAuxUnavailable は、補助の接続が開いていない（本来の接続が切れている、試す間隔の待ち、
	// 開けなかった）こと。要求は待たずに失敗する。
	ErrAuxUnavailable = errors.New("補助の接続が開いていない")
	// ErrAuxTimeout は、要求が期限（送るまでの待ちと応答の待ちを合わせて2秒）を過ぎたこと。
	// 適用されたかどうかはわからない。Client は作り直す。
	ErrAuxTimeout = errors.New("補助の接続の要求が期限を過ぎた")
	// ErrAuxMismatch は、ID の合わない応答を受けたこと。ErrAuxTimeout と同じく適用されたか
	// わからない。Client は作り直す。
	ErrAuxMismatch = errors.New("補助の接続で ID の合わない応答を受けた")
	// ErrAuxClosed は、Aux が Close されたこと。
	ErrAuxClosed = errors.New("補助の接続は閉じられた")
)

const (
	auxRequestTimeout = 2 * time.Second
	auxRetryMin       = 5 * time.Second
	auxRetryMax       = 30 * time.Second
)

// auxClock は Aux の時計。試す間隔と、要求の期限に使う。テストで差し替える。
type auxClock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// auxConn は開いている goobs の Client 1本。dead が閉じられたら、捨てられている。
type auxConn struct {
	cli    *goobs.Client
	rec    *connRecorder
	dead   chan struct{}
	reason error // dead を閉じる前に書く
}

// auxOpen は進行中の接続の試み。done が閉じられたあとに conn / err を読める。
type auxOpen struct {
	done chan struct{}
	conn *auxConn
	err  error
}

// Aux は goobs の Client をもう1本包む「補助の接続」。管理ページと地図の読み取りの要求を、
// 本来の接続（自動切替の SetScene など）と mutex を取り合わずに送る。
//
// goroutine の持ち主（すべて wg に数え、Close が戻ったときには全部終わっている）:
//   - 開く goroutine（runOpen）: dialOBS で1回だけ開き、結果を cur に置く。ctx は本来の接続の
//     「つながっている間」ごとに作り、切れたら取り消す。
//   - 要求の goroutine: goobs の要求（mutex の待ちと応答の待ちを含む）を呼ぶ。呼び出し側は
//     2秒の timer と select するだけで、この goroutine は goobs が戻るまで残りうる。
//   - 見張りの goroutine: cli.IncomingEvents が閉じられる（OBS から切られた）のを見て捨てる。
//   - 片づけの goroutine: 捨てた Client の接続を閉じ、Disconnect を呼ぶ。
//
// 捨てる順（drop）: dead を閉じて要求の待ちを解く → cur を外す → 片づけの goroutine で
// rec.close（止まっている読み書きを戻す）→ cli.Disconnect。Disconnect は goobs の要求の
// mutex を取るので、止まっている要求の応答待ち（respTimeout）が切れるまで戻らないことがある。
type Aux struct {
	cfg Config
	log *slog.Logger

	// reqTimeout は要求の期限、respTimeout は goobs の応答待ち（と認証の待ち）。テストで短くする。
	reqTimeout, respTimeout time.Duration
	clock                   auxClock
	dialFn                  func(ctx context.Context) (*goobs.Client, *connRecorder, error)

	mu       sync.Mutex
	closed   bool
	main     bool
	epoch    context.Context // 本来の接続がつながっている間だけ（切れたら取り消す）
	cancel   context.CancelFunc
	cur      *auxConn
	opening  *auxOpen
	nextTry  time.Time
	interval time.Duration // 次に開けなかったときにあける間隔
	wg       sync.WaitGroup
}

// NewAux は Aux を作る。本来の接続がつながったと SetMainConnected(true) で知らせるまで、開かない。
func NewAux(cfg Config, log *slog.Logger) *Aux {
	if log == nil {
		log = slog.Default()
	}
	a := &Aux{
		cfg:         cfg,
		log:         log,
		reqTimeout:  auxRequestTimeout,
		respTimeout: auxRequestTimeout,
		clock:       realClock{},
		interval:    auxRetryMin,
	}
	a.dialFn = func(ctx context.Context) (*goobs.Client, *connRecorder, error) {
		addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
		// 補助の接続はイベントを使わない。
		return dialOBS(ctx, addr, cfg.Password, a.respTimeout, goobs.WithEventSubscriptions(subscriptions.None))
	}
	return a
}

// SetMainConnected は本来の接続が OBS につながっているかを知らせる。つながった時点で開き始め、
// 切れたら閉じる。
func (a *Aux) SetMainConnected(connected bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.main == connected {
		return
	}
	a.main = connected
	if connected {
		a.epoch, a.cancel = context.WithCancel(context.Background())
		if !a.clock.Now().Before(a.nextTry) {
			a.startOpenLocked()
		}
		return
	}
	a.cancel() // 開きかけの接続は、できても捨てられる
	a.epoch, a.cancel = nil, nil
	a.opening = nil
	if a.cur != nil {
		a.dropLocked(a.cur, ErrAuxUnavailable)
	}
}

// Close は補助の接続を閉じる。待っている要求は ErrAuxClosed で戻り、以後の要求も失敗する。
// 開いた接続と goroutine がすべて終わってから戻る（止まった要求があれば、goobs の応答待ちの
// 長さだけかかりうる）。何度呼んでもよい。
func (a *Aux) Close() {
	a.mu.Lock()
	if !a.closed {
		a.closed = true
		a.main = false
		if a.cancel != nil {
			a.cancel()
		}
		a.opening = nil
		if a.cur != nil {
			a.dropLocked(a.cur, ErrAuxClosed)
		}
	}
	a.mu.Unlock()
	a.wg.Wait()
}

// startOpenLocked は開く goroutine を始める。mu を持って呼ぶ。closed でないこと、main であること。
func (a *Aux) startOpenLocked() *auxOpen {
	o := &auxOpen{done: make(chan struct{})}
	a.opening = o
	ctx := a.epoch
	a.wg.Add(1)
	go a.runOpen(ctx, o)
	return o
}

func (a *Aux) runOpen(ctx context.Context, o *auxOpen) {
	defer a.wg.Done()
	cli, rec, err := a.dialFn(ctx)

	a.mu.Lock()
	if a.opening == o {
		a.opening = nil
	}
	switch {
	case err == nil && (a.closed || a.epoch != ctx || ctx.Err() != nil):
		// 開いている間に、閉じられたか本来の接続が切れた。捨てる（失敗には数えない）。
		a.mu.Unlock()
		rec.close()
		_ = cli.Disconnect()
		o.err = ErrAuxUnavailable
		close(o.done)
		return
	case err != nil:
		if ctx.Err() == nil {
			a.nextTry = a.clock.Now().Add(a.interval)
			a.interval = min(a.interval*2, auxRetryMax)
			a.log.Warn("補助の接続を開けない", "err", err, "retry_in", a.nextTry.Sub(a.clock.Now()))
		}
		o.err = fmt.Errorf("%w: %v", ErrAuxUnavailable, err)
	default:
		c := &auxConn{cli: cli, rec: rec, dead: make(chan struct{})}
		a.cur = c
		a.interval = auxRetryMin
		a.nextTry = time.Time{}
		o.conn = c
		a.wg.Add(1)
		go a.watch(c)
	}
	a.mu.Unlock()
	close(o.done)
}

// watch は OBS から切られたこと（cli.IncomingEvents が閉じる）を見て、Client を捨てる。
// 補助の接続はイベントを購読しないので、流れてくるものは読み捨てる。
func (a *Aux) watch(c *auxConn) {
	defer a.wg.Done()
	for range c.cli.IncomingEvents {
	}
	a.mu.Lock()
	a.dropLocked(c, ErrAuxUnavailable)
	a.mu.Unlock()
}

// dropLocked は c を捨てる。mu を持って呼ぶ。すでに捨てていれば何もしない。
// 片づけの goroutine は、Close が mu を取って closed にする前に c が cur だったときだけ作られる
// （そのとき Close はまだ wg.Wait を呼んでいない）。
func (a *Aux) dropLocked(c *auxConn, reason error) {
	select {
	case <-c.dead:
		return
	default:
	}
	c.reason = reason
	close(c.dead)
	if a.cur == c {
		a.cur = nil
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		c.rec.close()
		_ = c.cli.Disconnect()
	}()
}

func (a *Aux) drop(c *auxConn, reason error) {
	a.mu.Lock()
	a.dropLocked(c, reason)
	a.mu.Unlock()
}

// acquire は要求を送る Client を返す。まだなら、進行中の試み（なければ新しい試み）を返す。
// 開いていなくて、試す間隔の待ちの途中なら、待たずに ErrAuxUnavailable。
func (a *Aux) acquire() (*auxConn, *auxOpen, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case a.closed:
		return nil, nil, ErrAuxClosed
	case !a.main:
		return nil, nil, fmt.Errorf("%w: 本来の接続が切れている", ErrAuxUnavailable)
	case a.cur != nil:
		return a.cur, nil, nil
	case a.opening != nil:
		return nil, a.opening, nil
	}
	if wait := a.nextTry.Sub(a.clock.Now()); wait > 0 {
		return nil, nil, fmt.Errorf("%w: 試し直しまであと %v", ErrAuxUnavailable, wait)
	}
	return nil, a.startOpenLocked(), nil
}

// spawn は要求の goroutine を始める。closed なら false。
func (a *Aux) spawn(f func()) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return false
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		f()
	}()
	return true
}

// auxCall は fn を、期限つきで Client に対して実行する。期限は、接続を開く待ちと goobs の
// mutex の待ちと応答の待ちを合わせて reqTimeout。fn の結果は channel だけで受け取る
// （期限で戻ったあとも fn の goroutine は残りうるので、変数を共有しない）。
func auxCall[T any](a *Aux, op string, fn func(*goobs.Client) (T, error)) (T, error) {
	var zero T
	deadline := a.clock.After(a.reqTimeout)
	c, o, err := a.acquire()
	if err != nil {
		return zero, err
	}
	if c == nil {
		select {
		case <-o.done:
		case <-deadline:
			return zero, fmt.Errorf("%w: %s（接続を開く待ち）", ErrAuxTimeout, op)
		}
		if o.err != nil {
			return zero, o.err
		}
		c = o.conn
	}

	type result struct {
		v   T
		err error
	}
	res := make(chan result, 1)
	if !a.spawn(func() {
		v, err := fn(c.cli)
		res <- result{v, err}
	}) {
		return zero, ErrAuxClosed
	}
	select {
	case r := <-res:
		if r.err == nil {
			return r.v, nil
		}
		select {
		case <-c.dead: // 捨てられたあとの失敗は、捨てた理由で返す
			return zero, fmt.Errorf("%w: %s: %v", c.reason, op, r.err)
		default:
		}
		switch {
		case strings.Contains(r.err.Error(), goobsTimeoutText):
			a.drop(c, ErrAuxTimeout)
			return zero, fmt.Errorf("%w: %s: %v", ErrAuxTimeout, op, r.err)
		case strings.Contains(r.err.Error(), "mismatched ID"):
			a.drop(c, ErrAuxMismatch)
			return zero, fmt.Errorf("%w: %s: %v", ErrAuxMismatch, op, r.err)
		}
		return zero, fmt.Errorf("%s: %w", op, r.err)
	case <-deadline:
		a.drop(c, ErrAuxTimeout)
		return zero, fmt.Errorf("%w: %s", ErrAuxTimeout, op)
	case <-c.dead:
		return zero, fmt.Errorf("%w: %s", c.reason, op)
	}
}

// 以下の要求は、シーンとグループと入力を UUID で指す。戻り値は goobs の型のまま。

func (a *Aux) GetSceneList() (*scenes.GetSceneListResponse, error) {
	return auxCall(a, "GetSceneList", func(c *goobs.Client) (*scenes.GetSceneListResponse, error) {
		return c.Scenes.GetSceneList()
	})
}

func (a *Aux) GetSceneItemList(sceneUUID string) (*sceneitems.GetSceneItemListResponse, error) {
	return auxCall(a, "GetSceneItemList", func(c *goobs.Client) (*sceneitems.GetSceneItemListResponse, error) {
		return c.SceneItems.GetSceneItemList(sceneitems.NewGetSceneItemListParams().WithSceneUuid(sceneUUID))
	})
}

func (a *Aux) GetGroupSceneItemList(sceneUUID string) (*sceneitems.GetGroupSceneItemListResponse, error) {
	return auxCall(a, "GetGroupSceneItemList", func(c *goobs.Client) (*sceneitems.GetGroupSceneItemListResponse, error) {
		return c.SceneItems.GetGroupSceneItemList(sceneitems.NewGetGroupSceneItemListParams().WithSceneUuid(sceneUUID))
	})
}

// GetInputList は入力の一覧。inputKind が空なら全部。
func (a *Aux) GetInputList(inputKind string) (*inputs.GetInputListResponse, error) {
	return auxCall(a, "GetInputList", func(c *goobs.Client) (*inputs.GetInputListResponse, error) {
		p := inputs.NewGetInputListParams()
		if inputKind != "" {
			p.WithInputKind(inputKind)
		}
		return c.Inputs.GetInputList(p)
	})
}

func (a *Aux) GetInputSettings(inputUUID string) (*inputs.GetInputSettingsResponse, error) {
	return auxCall(a, "GetInputSettings", func(c *goobs.Client) (*inputs.GetInputSettingsResponse, error) {
		return c.Inputs.GetInputSettings(inputs.NewGetInputSettingsParams().WithInputUuid(inputUUID))
	})
}

func (a *Aux) GetSceneCollectionList() (*config.GetSceneCollectionListResponse, error) {
	return auxCall(a, "GetSceneCollectionList", func(c *goobs.Client) (*config.GetSceneCollectionListResponse, error) {
		return c.Config.GetSceneCollectionList()
	})
}

// SetInputSettings は入力の設定を書く。overlay が true なら今の設定の上に重ねる。
func (a *Aux) SetInputSettings(inputUUID string, settings map[string]any, overlay bool) (*inputs.SetInputSettingsResponse, error) {
	return auxCall(a, "SetInputSettings", func(c *goobs.Client) (*inputs.SetInputSettingsResponse, error) {
		return c.Inputs.SetInputSettings(inputs.NewSetInputSettingsParams().
			WithInputUuid(inputUUID).WithInputSettings(settings).WithOverlay(overlay))
	})
}
