package obs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/requests/mediainputs"
	"github.com/andreykaipov/goobs/api/requests/scenes"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// Client は OBS への操作とイベントの受け口。中核ループはこれだけに依存する。
type Client interface {
	// Events は OBSConnected / OBSDisconnected / SceneChanged / TransitionChanged /
	// StreamStateChanged を流す。閉じられない。
	Events() <-chan switcher.Event
	// SceneTransitionOverride は GetSceneSceneTransitionOverride。
	SceneTransitionOverride(scene string) (ms *int, has bool, err error)
	// SetScene は SetCurrentProgramScene。応答待ちの超過は ErrTimeout で返す。
	SetScene(name string) error
	RestartMedia(input string) error
	StartStream() error
	StopStream() error
}

// Config は OBS への接続設定と、存在を確かめる名前。
type Config struct {
	Host     string
	Port     int
	Password string // 空なら認証なしで接続する
	// MediaSource は RestartMedia の対象の入力名。
	MediaSource string
	// Scenes は config で割り当てたシーン名。接続時に存在を確かめる。
	Scenes []string
}

const (
	responseTimeout = 5 * time.Second
	backoffMin      = 1 * time.Second
	backoffMax      = 5 * time.Second
	eventBuffer     = 64
)

// WS は goobs による Client の実装。接続は1本だけ保つ。
type WS struct {
	cfg    Config
	log    *slog.Logger
	events chan switcher.Event

	mu       sync.RWMutex
	cli      *goobs.Client
	warnings []string
}

var _ Client = (*WS)(nil)

// New は WS を作る。接続は Run で行う。
func New(cfg Config, log *slog.Logger) *WS {
	if log == nil {
		log = slog.Default()
	}
	return &WS{cfg: cfg, log: log, events: make(chan switcher.Event, eventBuffer)}
}

// Events は switcher に渡すイベントの列。
func (w *WS) Events() <-chan switcher.Event { return w.events }

// Warnings は最後の接続で見つかった警告（存在しないシーン名やメディアソース）。
func (w *WS) Warnings() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]string(nil), w.warnings...)
}

// Run は ctx が終わるまで接続を保つ。切れたら 1→2→4 秒と延ばし、上限 5 秒で再接続する。
func (w *WS) Run(ctx context.Context) {
	backoff := backoffMin
	for ctx.Err() == nil {
		connected := w.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = backoffMin
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, backoffMax)
	}
}

// session は1回の接続を、切れるか ctx が終わるまで扱う。OBSConnected を流せたかを返す。
func (w *WS) session(ctx context.Context) (connected bool) {
	addr := net.JoinHostPort(w.cfg.Host, strconv.Itoa(w.cfg.Port))
	opts := []goobs.Option{goobs.WithResponseTimeoutDuration(responseTimeout)}
	if w.cfg.Password != "" {
		opts = append(opts, goobs.WithPassword(w.cfg.Password))
	}
	cli, err := goobs.New(addr, opts...)
	if err != nil {
		w.log.Warn("OBS に接続できない", "addr", addr, "err", err)
		return false
	}
	defer cli.Disconnect()

	first, err := w.fetchConnected(cli)
	if err != nil {
		w.log.Warn("OBS の状態を取得できない", "err", err)
		return false
	}
	w.checkNames(cli)

	w.mu.Lock()
	w.cli = cli
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.cli = nil
		w.mu.Unlock()
	}()

	if !w.emit(ctx, first) {
		return false
	}
	w.log.Info("OBS に接続した", "addr", addr)

	for {
		select {
		case <-ctx.Done():
			return true
		case ev, ok := <-cli.IncomingEvents:
			if !ok {
				w.log.Warn("OBS との接続が切れた")
				w.emit(ctx, switcher.OBSDisconnected{At: time.Now()})
				return true
			}
			w.handle(ctx, cli, ev)
		}
	}
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

// handle は goobs のイベント1つを写して流す。
func (w *WS) handle(ctx context.Context, cli *goobs.Client, ev any) {
	at := time.Now()
	if _, ok := ev.(*events.CurrentSceneTransitionChanged); ok {
		// 切り替わったトランジションの長さは、取り直さないとわからない。
		ms, err := currentTransitionMs(cli)
		if err != nil {
			w.log.Warn("トランジションの長さを取得できない", "err", err)
		}
		w.emit(ctx, switcher.TransitionChanged{At: at, TransitionMs: ms})
		return
	}
	if out, ok := mapEvent(ev, at); ok {
		w.emit(ctx, out)
	}
}

func currentTransitionMs(cli *goobs.Client) (*int, error) {
	r, err := cli.Transitions.GetCurrentSceneTransition()
	if err != nil {
		return nil, classifyError(err)
	}
	return transitionMs(r.TransitionFixed, r.TransitionDuration), nil
}

// fetchConnected は接続直後の状態を取得して OBSConnected を作る。
func (w *WS) fetchConnected(cli *goobs.Client) (switcher.OBSConnected, error) {
	st, err := cli.Stream.GetStreamStatus()
	if err != nil {
		return switcher.OBSConnected{}, fmt.Errorf("GetStreamStatus: %w", classifyError(err))
	}
	sc, err := cli.Scenes.GetCurrentProgramScene()
	if err != nil {
		return switcher.OBSConnected{}, fmt.Errorf("GetCurrentProgramScene: %w", classifyError(err))
	}
	name := sc.SceneName
	if name == "" {
		name = sc.CurrentProgramSceneName
	}
	// 長さが取れなくても接続は成り立つ。nil にする。
	ms, err := currentTransitionMs(cli)
	if err != nil {
		w.log.Warn("トランジションの長さを取得できない", "err", err)
		ms = nil
	}
	return switcher.OBSConnected{
		At:           time.Now(),
		Streaming:    streamingFromStatus(st.OutputActive, st.OutputReconnecting),
		Scene:        name,
		TransitionMs: ms,
	}, nil
}

// checkNames は config のシーン名とメディアソースが OBS にあるかを確かめ、なければ警告にする。
func (w *WS) checkNames(cli *goobs.Client) {
	var warns []string
	if l, err := cli.Scenes.GetSceneList(); err != nil {
		warns = append(warns, "シーン一覧を取得できない: "+err.Error())
	} else {
		have := map[string]bool{}
		for _, s := range l.Scenes {
			have[s.SceneName] = true
		}
		for _, n := range w.cfg.Scenes {
			if n != "" && !have[n] {
				warns = append(warns, fmt.Sprintf("シーン %q が OBS にない", n))
			}
		}
	}
	if w.cfg.MediaSource != "" {
		if l, err := cli.Inputs.GetInputList(); err != nil {
			warns = append(warns, "入力一覧を取得できない: "+err.Error())
		} else {
			found := false
			for _, in := range l.Inputs {
				if in.InputName == w.cfg.MediaSource {
					found = true
				}
			}
			if !found {
				warns = append(warns, fmt.Sprintf("メディアソース %q が OBS にない", w.cfg.MediaSource))
			}
		}
	}
	for _, m := range warns {
		w.log.Warn("OBS の設定の警告", "msg", m)
	}
	w.mu.Lock()
	w.warnings = warns
	w.mu.Unlock()
}

var errNotConnected = errors.New("OBS に未接続")

func (w *WS) client() (*goobs.Client, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.cli == nil {
		return nil, errNotConnected
	}
	return w.cli, nil
}

// SceneTransitionOverride は切替先シーンのトランジションの上書きを読む。
// 上書きなしは has=false。長さがなければ ms=nil で has=true。
func (w *WS) SceneTransitionOverride(scene string) (ms *int, has bool, err error) {
	cli, err := w.client()
	if err != nil {
		return nil, false, err
	}
	r, err := cli.Scenes.GetSceneSceneTransitionOverride(scenes.NewGetSceneSceneTransitionOverrideParams().WithSceneName(scene))
	if err != nil {
		return nil, false, classifyError(err)
	}
	ms, has = overrideFromResponse(r.TransitionName, r.TransitionDuration)
	return ms, has, nil
}

// SetScene は program のシーンを切り替える。応答待ちの超過は ErrTimeout。
func (w *WS) SetScene(name string) error {
	cli, err := w.client()
	if err != nil {
		return err
	}
	_, err = cli.Scenes.SetCurrentProgramScene(scenes.NewSetCurrentProgramSceneParams().WithSceneName(name))
	return classifyError(err)
}

// RestartMedia はメディアソースに RESTART を送る。
func (w *WS) RestartMedia(input string) error {
	cli, err := w.client()
	if err != nil {
		return err
	}
	_, err = cli.MediaInputs.TriggerMediaInputAction(mediainputs.NewTriggerMediaInputActionParams().
		WithInputName(input).WithMediaAction("OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART"))
	return classifyError(err)
}

// StartStream は配信を始める。
func (w *WS) StartStream() error {
	cli, err := w.client()
	if err != nil {
		return err
	}
	_, err = cli.Stream.StartStream()
	return classifyError(err)
}

// StopStream は配信を止める。
func (w *WS) StopStream() error {
	cli, err := w.client()
	if err != nil {
		return err
	}
	_, err = cli.Stream.StopStream()
	return classifyError(err)
}
