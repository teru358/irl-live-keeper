package obs

import (
	"context"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/gorilla/websocket"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// Client は OBS への操作とイベントの受け口。中核ループはこれだけに依存する。
type Client interface {
	// Events は OBSConnected / OBSDisconnected / SceneChanged / TransitionChanged /
	// StreamStateChanged / StudioModeChanged を流す。閉じられない。
	Events() <-chan switcher.Event
	StartStream() error
	StopStream() error

	// 以下は UUID で送る要求（カメラの spec 第3節「識別」）。古い OBS（obs-websocket 5.4.0 未満）では、
	// UUID の代わりに名前を渡し、名前で送る。

	// SetSceneUUID は SetCurrentProgramScene を sceneUuid で送る。応答待ちの超過は ErrTimeout で返す。
	SetSceneUUID(uuid string) error
	// SceneTransitionOverrideUUID は GetSceneSceneTransitionOverride を sceneUuid で送る。
	SceneTransitionOverrideUUID(uuid string) (ms *int, has bool, err error)
	// RestartMediaInputs は、入力の UUID の列の順に、TriggerMediaInputAction（RESTART）を inputUuid で送る。
	RestartMediaInputs(inputUUIDs []string) error
	// SetSceneItemEnabled は SetSceneItemEnabled を sceneUuid で送る。グループの中の項目は、グループの UUID を渡す。
	// 応答待ちの超過は ErrTimeout で返す。結果は ClassifyItemResult で分ける。
	SetSceneItemEnabled(sceneUUID string, itemID int, enabled bool) error
}

// Config は OBS への接続設定と、存在を確かめる名前。
type Config struct {
	Host     string
	Port     int
	Password string // 空なら認証なしで接続する
	// Scenes は config で割り当てたシーン名（待機シーンと開始時のシーン）。接続時に存在を確かめる。
	Scenes []string
	// Cameras は地図で、メディアソースの URL を照合するカメラ（設定の順）。
	Cameras []CameraConfig
	// RTSPPort は MediaMTX の RTSP のポート。メディアソースの URL のポートと照合する。
	RTSPPort int
}

const (
	responseTimeout = 5 * time.Second
	backoffMin      = 1 * time.Second
	backoffMax      = 5 * time.Second
	eventBuffer     = 64
	// eventQueueMax は、goobs から読んだイベントを core へ流すまで取っておく件数の上限。
	eventQueueMax = 4096
	// callMargin は、要求を goobs に渡したあと、goobs の応答待ち（timeout）に足す余裕。goobs は
	// 受け手のいない応答で止まると、応答待ちの期限も効かないまま戻らないことがある（M-6 で確かめた）ので、
	// 呼び出し側でも期限を持つ。
	callMargin = time.Second
	// quietAfterRemoved は、program のシーンの SceneRemoved のあと、Changed が来なくても読み直すまでの、
	// イベントの途切れ（GUI の「複製」は Changing を出さない）。
	quietAfterRemoved = 2 * time.Second
	// switchMax は、Changing のあと Changed が来ないまま、読み直すまでのイベントの途切れ。
	switchMax = 30 * time.Second
)

// 古い OBS（obs-websocket 5.4.0 未満）の警告。
const legacyWarning = "OBS が古いため、自動の切替と隠す・戻すを止めています"

// switchingWarning は、シーンコレクションの切替中の警告。
const switchingWarning = "OBS のシーンコレクションを切り替え中"

// WS は goobs による Client の実装。接続は1本だけ保つ。
//
// goroutine の持ち主（すべて wg に数え、Run が戻るときに上限つきで待つ）:
//   - Run を呼んだ goroutine: session を回す。session の goroutine はイベントの列を読み、状態の遷移
//     （接続の読み取り中 / 動作中 / 切替の待ち）を持つ唯一の持ち主で、goobs に要求を送らない。
//   - 読み取りの goroutine: つないだとき（と、コレクションの切替のあと）に、版・配信状態・今のシーン・
//     トランジション・名前の確かめ・地図を読む。読む間も session はイベントを読み続ける。
//   - エポックの goroutine: OBSConnected から OBSDisconnected までの間だけ動く。トランジションの取り直し。
//     切断のときに ctx を取り消す。
//   - 要求の goroutine: goobs の要求を呼ぶ。Client ごとに goobs の中に入れるのは1本（gate）。
//   - 片づけの goroutine: 捨てた Client の Disconnect を呼ぶ（goobs の mutex で待つことがある）。
//
// 本来の接続の Client は、要求が Timeout か ID のずれになったら捨てて、つなぎ直す。
type WS struct {
	cfg    Config
	log    *slog.Logger
	events chan switcher.Event
	// timeout は goobs の応答待ち。テストで短くする。
	timeout time.Duration
	// clock は途切れの待ちに使う。テストで差し替える。
	clock auxClock
	// quiet と switchMax は、切替の途切れの長さ。テストで差し替える。
	quiet, switchMax time.Duration
	// backoffMin と backoffMax は再接続の待ち。テストで短くする。
	backoffMin, backoffMax time.Duration
	// eventQueueMax は、goobs から読んだイベントを core へ流すまで取っておく上限。
	eventQueueMax int
	aux           *Aux

	// bounceArmed と bounceBuffered は、Studio Mode の一瞬の切替の取っておく処理の進みの印（テストが待つ）。
	// 窓の待ちを置いた回数と、取っておいたイベントの数。
	bounceArmed, bounceBuffered atomic.Int64

	// wg は goroutine を数える。stopped が true になったら、spawn は何も始めない。
	spawnMu sync.Mutex
	stopped bool
	wg      sync.WaitGroup

	// emitMu は、エポックの goroutine の送信と、OBSDisconnected の送信の順を守る。
	emitMu sync.Mutex

	mu        sync.RWMutex
	cur       *mainConn // つないでいる Client。切れたら nil
	open      bool      // 外からの要求を受けるか（門）
	switching bool      // シーンコレクションの切替中
	legacy    bool      // 古い OBS（警告のため）
	gen       uint64    // 門が開くたびに進む世代
	warnBase  []i18n.Msg
	warnMap   string     // 地図を読めずに空の地図でつないだときの理由
	warnCams  []i18n.Msg // 地図で見つからなかったカメラ
	mapFails  int        // 続けて地図を読めなかった接続の数
}

var _ Client = (*WS)(nil)

// New は WS を作る。接続は Run で行う。
func New(cfg Config, log *slog.Logger) *WS {
	if log == nil {
		log = slog.Default()
	}
	return &WS{
		cfg: cfg, log: log, events: make(chan switcher.Event, eventBuffer), timeout: responseTimeout,
		clock: realClock{}, quiet: quietAfterRemoved, switchMax: switchMax,
		backoffMin: backoffMin, backoffMax: backoffMax, eventQueueMax: eventQueueMax,
		aux: NewAux(cfg, log),
	}
}

// Events は switcher に渡すイベントの列。
func (w *WS) Events() <-chan switcher.Event { return w.events }

// Aux は補助の接続。本来の接続がつながっている間だけ開く。
func (w *WS) Aux() *Aux { return w.aux }

// Warnings は最後の接続で見つかった警告（存在しないシーン名やメディアソース、古い OBS、切替中）。
func (w *WS) Warnings() []string { return i18n.Strings(i18n.JA, w.WarningMessages()) }

func (w *WS) WarningMessages() []i18n.Msg {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []i18n.Msg
	if w.switching {
		out = append(out, i18n.New("obs.switching"))
	}
	if w.legacy {
		out = append(out, i18n.New("obs.legacy"))
	}
	if w.warnMap != "" {
		out = append(out, i18n.New("obs.map", "reason", w.warnMap))
	}
	out = append(out, w.warnCams...)
	return append(out, w.warnBase...)
}

// spawn は goroutine を wg に数えて始める。Run が終わったあとは始めずに false を返す。
func (w *WS) spawn(f func()) bool {
	w.spawnMu.Lock()
	defer w.spawnMu.Unlock()
	if w.stopped {
		return false
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		f()
	}()
	return true
}

// Run は ctx が終わるまで接続を保つ。切れたら 1→2→4 秒と延ばし、上限 5 秒で再接続する。
// 戻るときは、補助の接続を閉じ、開いた goroutine が終わるのを待つ（goobs の中で止まったものは、
// goobs の応答待ち + 1秒で待つのをやめて警告に出す。外から戻せない）。
func (w *WS) Run(ctx context.Context) {
	defer w.shutdown()
	backoff := w.backoffMin
	for ctx.Err() == nil {
		connected := w.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = w.backoffMin
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, w.backoffMax)
	}
}

func (w *WS) shutdown() {
	w.aux.Close()
	w.spawnMu.Lock()
	w.stopped = true
	w.spawnMu.Unlock()
	done := make(chan struct{})
	go func() { w.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(w.timeout + time.Second):
		w.log.Warn("OBS の接続の goroutine が goobs の中で止まったまま残った（既知の制約）")
	}
}

// connRecorder は dial で開いた接続を記録し、あとから閉じられるようにする。
// goobs.New は失敗しても接続を閉じないため、こちらで閉じる。
// close のあとに add された接続は記録せず、その場で閉じる。何度 close してもよい。
type connRecorder struct {
	mu     sync.Mutex
	closed bool
	conns  []net.Conn
}

// add は c を記録する。close のあとなら c を閉じて、net.ErrClosed を包んだエラーを返す。
func (r *connRecorder) add(c net.Conn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		_ = c.Close()
		return fmt.Errorf("OBS への接続は閉じられたあとに開かれた: %w", net.ErrClosed)
	}
	r.conns = append(r.conns, c)
	return nil
}

func (r *connRecorder) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	for _, c := range r.conns {
		_ = c.Close()
	}
	r.conns = nil
}

// dial は goobs.New で OBS に接続する。失敗したときと、実行中に ctx が終わったときは、
// 開いた接続を閉じる。成功したら接続は cli.Disconnect と、返す記録で閉じる。
func (w *WS) dial(ctx context.Context, addr string) (*goobs.Client, *connRecorder, error) {
	return dialOBS(ctx, addr, w.cfg.Password, w.timeout)
}

// dialOBS は addr の OBS に goobs.New で接続する（WS と Aux が共通で使う）。
// timeout は goobs の応答待ち（認証の待ちも同じ値）。extra は goobs の追加のオプション。
func dialOBS(ctx context.Context, addr, password string, timeout time.Duration, extra ...goobs.Option) (*goobs.Client, *connRecorder, error) {
	rec := &connRecorder{}
	nd := &net.Dialer{}
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
		NetDialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			c, err := nd.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			if err := rec.add(c); err != nil {
				return nil, err
			}
			return c, nil
		},
	}
	opts := []goobs.Option{
		goobs.WithResponseTimeoutDuration(timeout),
		goobs.WithDialer(dialer),
	}
	if password != "" {
		opts = append(opts, goobs.WithPassword(password))
	}
	opts = append(opts, extra...)

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			rec.close()
		case <-done:
		}
	}()
	cli, err := goobs.New(addr, opts...)
	close(done)
	<-stopped
	if err != nil {
		rec.close()
		return nil, nil, err
	}
	return cli, rec, nil
}

// emit は Event を流す。ctx が終わったら false。
func (w *WS) emit(ctx context.Context, e switcher.Event) bool {
	select {
	case w.events <- e:
		return true
	case <-ctx.Done():
		return false
	}
}
