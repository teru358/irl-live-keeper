package obs

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/requests/general"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/requests/stream"
	"github.com/andreykaipov/goobs/api/requests/transitions"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// sessState は session の状態。session の goroutine だけが読み書きする。
type sessState int

const (
	// stateReloading は、つないだとき（とコレクションの切替のあと）の読み取りの最中。
	// 届いたイベントは、読み取りの goroutine が終わるまで取っておく。
	stateReloading sessState = iota
	// stateRunning は、OBSConnected を流して、イベントを写している間。
	stateRunning
	// statePaused は、コレクションの切替の最中（門が閉じ、OBSDisconnected を流した）。
	// 届いたイベントは捨てる。Changed か、イベントの途切れで読み取りを始める。
	statePaused
)

// reloadResult は読み取りの goroutine の結果。
type reloadResult struct {
	gen       int
	legacy    bool
	connected switcher.OBSConnected // Map を含む
	// emptyReason は、地図を読めずに空の地図（シーンの一覧だけ）にしたときの理由。
	emptyReason string
	err         error
}

// epoch は OBSConnected から OBSDisconnected までの間。この間だけ動く goroutine の ctx を持つ。
type epoch struct {
	ctx    context.Context
	cancel context.CancelFunc
	dead   bool // emitMu で守る
	trans  chan struct{}
	// refresher は地図の読み直し。
	refresher *refresher
}

// heldEvent は、読み取りの最中に届いたイベント。読み取りのあと OBSConnected の次に流す。
type heldEvent struct {
	ev any
	at time.Time
}

// sess は1回の接続（goobs の Client 1本）の状態。session の goroutine だけが持つ。
type sess struct {
	w    *WS
	ctx  context.Context
	conn *mainConn

	state      sessState
	announced  bool // 最後に流したのが OBSConnected
	everUp     bool // OBSConnected を流せた
	legacy     bool
	curUUID    string // 今の program のシーン（SceneRemoved の判定に使う）
	ep         *epoch
	held       []heldEvent
	heldTrans  bool
	heldKick   bool // 読み取りの最中に、地図のイベントが届いた
	reloadGen  int
	reloadStop context.CancelFunc
	reloadCh   chan reloadResult
	quiet      <-chan time.Time
	quietD     time.Duration // 切替の最中に、次のイベントを待つ長さ
	first      bool          // まだ1回も読み取りを終えていない（版を読む）
}

// session は1回の接続を、切れるか ctx が終わるまで扱う。OBSConnected を流せたかを返す。
func (w *WS) session(ctx context.Context) (connected bool) {
	addr := net.JoinHostPort(w.cfg.Host, strconv.Itoa(w.cfg.Port))
	cli, rec, err := w.dial(ctx, addr)
	if err != nil {
		w.log.Warn("OBS に接続できない", "addr", addr, "err", err)
		return false
	}
	c := newMainConn(cli, rec)
	w.mu.Lock()
	w.cur, w.open, w.switching = c, false, false
	w.mu.Unlock()
	defer w.dropConn(c, errWSStopped)

	s := &sess{w: w, ctx: ctx, conn: c, first: true, reloadCh: make(chan reloadResult)}
	defer func() {
		s.cancelReload()
		s.endEpoch()
		w.aux.SetMainConnected(false)
		if s.announced {
			// 流せなくてもよい（ctx が終わったときなど）。
			w.emit(ctx, switcher.OBSDisconnected{At: time.Now()})
		}
		w.mu.Lock()
		w.switching = false
		w.mu.Unlock()
		connected = s.everUp
	}()
	s.startReload()
	s.run(addr)
	return s.everUp
}

// run はイベントの列を読む。session を終えるとき（切れた、失敗した、ctx が終わった）に戻る。
func (s *sess) run(addr string) {
	w, c := s.w, s.conn
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-c.dead:
			w.log.Warn("OBS の接続を捨てた", "reason", c.reason)
			return
		case ev, ok := <-c.cli.IncomingEvents:
			if !ok {
				w.log.Warn("OBS との接続が切れた")
				return
			}
			s.onEvent(ev)
		case r := <-s.reloadCh:
			if r.gen != s.reloadGen {
				continue
			}
			if r.err != nil {
				w.log.Warn("OBS の状態を取得できない", "err", r.err)
				return
			}
			if !s.connect(r, addr) {
				return
			}
		case <-s.quiet:
			s.quiet = nil
			w.log.Info("OBS のイベントが途切れた。読み直す")
			s.startReload()
		}
	}
}

// startReload は読み取りの goroutine を始める（前の読み取りは取り消す）。
func (s *sess) startReload() {
	s.cancelReload()
	s.state = stateReloading
	s.quiet = nil
	s.held, s.heldTrans, s.heldKick = nil, false, false
	s.reloadGen++
	gen := s.reloadGen
	rctx, cancel := context.WithCancel(s.ctx)
	s.reloadStop = cancel
	first, legacy := s.first, s.legacy
	if !s.w.spawn(func() {
		res := s.w.reload(rctx, s.conn, gen, first, legacy)
		select {
		case s.reloadCh <- res:
		case <-rctx.Done():
		}
	}) {
		cancel()
	}
}

func (s *sess) cancelReload() {
	if s.reloadStop != nil {
		s.reloadStop()
		s.reloadStop = nil
	}
	s.reloadGen++ // 取り消した読み取りの結果は捨てる
}

// reload は接続直後の状態を読む。読み取りの goroutine が呼ぶ。
func (w *WS) reload(ctx context.Context, c *mainConn, gen int, first, legacy bool) reloadResult {
	res := reloadResult{gen: gen, legacy: legacy}
	if first {
		v, err := mcall(w, ctx, c, "GetVersion", func(cli *goobs.Client) (*general.GetVersionResponse, error) {
			return cli.General.GetVersion()
		})
		if err != nil {
			res.err = fmt.Errorf("GetVersion: %w", err)
			return res
		}
		res.legacy = isLegacyVersion(v.ObsWebSocketVersion)
		if res.legacy {
			w.log.Warn("OBS の obs-websocket が古い（5.4.0 未満）。UUID が使えないので、名前で扱い、地図は空にする", "version", v.ObsWebSocketVersion)
		}
	}
	conn, err := w.fetchConnected(ctx, c, res.legacy)
	if err != nil {
		res.err = err
		return res
	}
	w.checkNames(ctx, c)
	m, reason, err := w.readFirstMap(ctx, c, res.legacy)
	if err != nil {
		res.err = err
		return res
	}
	conn.Map = m
	res.connected, res.emptyReason = conn, reason
	return res
}

// maxMapFailures は、地図を続けて読めなかったとき、空の地図でつなぐまでの失敗の数。
const maxMapFailures = 3

// readFirstMap は最初の地図を本来の接続で読む。読めなければ失敗（接続の失敗として試し直す）。
// 続けて maxMapFailures 回失敗したら、シーンの一覧だけの空の地図を返す（reason に理由）。
// 古い OBS では、読めても読めなくても、シーンの一覧（名前）だけ。
func (w *WS) readFirstMap(ctx context.Context, c *mainConn, legacy bool) (m *scenemap.Map, reason string, err error) {
	req := mainReq{w: w, ctx: ctx, c: c}
	if legacy {
		m, err = readSceneListOnly(ctx, req, true)
		return m, "", err
	}
	m, err = readSceneMap(ctx, req, w.cameraIDs(), w.cfg.RTSPPort)
	if err == nil {
		return m, "", nil
	}
	if ctx.Err() != nil {
		return nil, "", err
	}
	w.mu.Lock()
	w.mapFails++
	n := w.mapFails
	w.mu.Unlock()
	w.log.Warn("シーンの地図を読めない", "err", err, "failures", n)
	if n < maxMapFailures {
		return nil, "", fmt.Errorf("シーンの地図: %w", err)
	}
	m, err2 := readSceneListOnly(ctx, req, false)
	if err2 != nil {
		return nil, "", fmt.Errorf("シーンの一覧: %w", err2)
	}
	return m, err.Error(), nil
}

// connect は読み取りの結果で OBSConnected を流し、動作中に入る。session を続けられなければ false。
func (s *sess) connect(r reloadResult, addr string) bool {
	w, c := s.w, s.conn
	if s.reloadStop != nil {
		s.reloadStop() // 読み取りは終わっている。ctx の後始末
		s.reloadStop = nil
	}
	s.first = false
	s.legacy = r.legacy
	c.legacy.Store(r.legacy)
	s.curUUID = r.connected.SceneUUID
	w.mu.Lock()
	w.legacy = r.legacy
	w.mapFails = 0
	w.mu.Unlock()
	w.setMapWarnings(r.connected.Map, r.emptyReason)

	ctx, cancel := context.WithCancel(s.ctx)
	s.ep = &epoch{ctx: ctx, cancel: cancel, trans: make(chan struct{}, 1)}
	s.ep.refresher = newRefresher(w, s.ep, r.connected.Map, r.legacy)
	s.state = stateRunning
	w.setGate(c, true, false)
	w.aux.SetMainConnected(true)
	if !w.emit(s.ctx, r.connected) {
		return false
	}
	s.announced, s.everUp = true, true
	w.log.Info("OBS に接続した", "addr", addr)
	s.startEpoch()

	held, heldTrans, heldKick := s.held, s.heldTrans, s.heldKick
	s.held, s.heldTrans, s.heldKick = nil, false, false
	for _, h := range held {
		if s.state != stateRunning {
			break
		}
		s.handleRunning(h.ev, h.at)
	}
	if s.state == stateRunning {
		if heldTrans {
			s.kickTransition()
		}
		// 空の地図でつないだときは、補助の接続での読み直しで、本当の地図を早く読む。
		if heldKick || r.emptyReason != "" {
			s.ep.refresher.kick()
		}
	}
	return true
}

func (s *sess) startEpoch() {
	ep := s.ep
	s.w.spawn(func() { s.w.runTransition(ep, s.conn) })
	s.w.spawn(ep.refresher.run)
}

// endEpoch は動作中のエポックを終える。エポックの goroutine が OBSDisconnected のあとに
// 何かを流さないよう、取り消してから、流す側を止める。
func (s *sess) endEpoch() {
	if s.ep == nil {
		return
	}
	s.w.closeEpoch(s.ep)
	s.ep = nil
}

func (w *WS) closeEpoch(ep *epoch) {
	ep.cancel()
	w.emitMu.Lock()
	ep.dead = true
	w.emitMu.Unlock()
}

// emitEpoch はエポックの goroutine が Event を流す。切断のあとは流さない。
func (w *WS) emitEpoch(ep *epoch, e switcher.Event) bool {
	w.emitMu.Lock()
	defer w.emitMu.Unlock()
	if ep.dead {
		return false
	}
	select {
	case w.events <- e:
		return true
	case <-ep.ctx.Done():
		return false
	}
}

// beginSwitch はコレクションの切替の始まり（か、それとみなすイベント）を扱う。
// 門を閉じ、補助の接続を閉じ、OBSDisconnected を流す。wait は Changed が来ないときの途切れの長さ。
func (s *sess) beginSwitch(wait time.Duration) {
	w := s.w
	s.cancelReload()
	s.held, s.heldTrans, s.heldKick = nil, false, false
	if s.state == stateRunning || s.ep != nil {
		s.endEpoch()
	}
	w.setGate(s.conn, false, true)
	if s.announced {
		w.aux.SetMainConnected(false)
		w.emit(s.ctx, switcher.OBSDisconnected{At: time.Now()})
		s.announced = false
	}
	s.state = statePaused
	s.quietD = wait
	s.armQuiet()
}

// armQuiet は途切れの待ちを置き直す（イベントが来るたびに数え直す）。
func (s *sess) armQuiet() {
	s.quiet = s.w.clock.After(s.quietD)
}

// onEvent は goobs のイベント1つを扱う。
func (s *sess) onEvent(ev any) {
	at := time.Now()
	w := s.w
	switch ev.(type) {
	case *events.CurrentSceneCollectionChanging:
		w.log.Info("OBS のシーンコレクションの切替が始まった")
		s.beginSwitch(w.switchMax)
		return
	case *events.CurrentSceneCollectionChanged:
		w.log.Info("OBS のシーンコレクションの切替が終わった")
		if s.state != statePaused {
			s.beginSwitch(w.switchMax)
		}
		s.startReload()
		return
	}
	switch s.state {
	case statePaused:
		s.armQuiet()
	case stateReloading:
		s.hold(ev, at)
	case stateRunning:
		s.handleRunning(ev, at)
	}
}

// hold は読み取りの最中に届いたイベントを、OBSConnected のあとに流すために取っておく。
func (s *sess) hold(ev any, at time.Time) {
	switch ev.(type) {
	case *events.CurrentSceneTransitionChanged:
		s.heldTrans = true
	case *events.CurrentProgramSceneChanged, *events.CurrentSceneTransitionDurationChanged, *events.StreamStateChanged,
		*events.SceneItemEnableStateChanged:
		s.held = append(s.held, heldEvent{ev, at})
	case *events.SceneCreated, *events.SceneRemoved, *events.SceneNameChanged, *events.SceneItemCreated, *events.SceneItemRemoved,
		*events.InputCreated, *events.InputRemoved, *events.InputNameChanged, *events.InputSettingsChanged:
		// 読み取りの途中の変化は、読み取りの値に入っていないかもしれない。OBSConnected のあとに読み直す。
		s.heldKick = true
	}
}

// sceneID は、シーンの識別（UUID。古い OBS では名前）。
func (s *sess) sceneID(uuid, name string) string {
	if s.legacy {
		return name
	}
	return uuid
}

// handleRunning は動作中のイベント1つを写して流す。要求は送らない。
func (s *sess) handleRunning(ev any, at time.Time) {
	w := s.w
	switch e := ev.(type) {
	case *events.CurrentSceneTransitionChanged:
		// 切り替わったトランジションの長さは取り直さないとわからない。イベントを読む goroutine では
		// 取り直さず、エポックの goroutine に頼む。
		s.kickTransition()
		return
	case *events.SceneRemoved:
		if !e.IsGroup && s.curUUID != "" && s.sceneID(e.SceneUuid, e.SceneName) == s.curUUID {
			// GUI の「複製」は Changing を出さずに、全部のシーンの SceneRemoved から始まる（M-0）。
			w.log.Info("OBS の program のシーンが削除された。シーンコレクションの切替とみなす")
			s.beginSwitch(w.quiet)
			return
		}
		s.ep.refresher.kick()
		return
	case *events.SceneCreated, *events.SceneNameChanged, *events.SceneItemCreated, *events.SceneItemRemoved,
		*events.InputCreated, *events.InputRemoved, *events.InputNameChanged:
		s.ep.refresher.kick()
		return
	case *events.InputSettingsChanged:
		s.ep.refresher.inputSettings(e.InputUuid, e.InputSettings)
		return
	case *events.SceneItemEnableStateChanged:
		// 本来の接続のイベントを、表示・非表示の正とする。グループの中の項目は、グループの UUID で届く。
		w.emit(s.ctx, switcher.ItemEnabledChanged{At: at, SceneUUID: s.sceneID(e.SceneUuid, e.SceneName), ItemID: e.SceneItemId, Enabled: e.SceneItemEnabled})
		return
	case *events.CurrentProgramSceneChanged:
		s.curUUID = s.sceneID(e.SceneUuid, e.SceneName)
	}
	out, ok := mapEvent(ev, at)
	if !ok {
		return
	}
	if sc, isScene := out.(switcher.SceneChanged); isScene && s.legacy {
		sc.SceneUUID = sc.Scene
		out = sc
	}
	w.emit(s.ctx, out)
}

func (s *sess) kickTransition() {
	select {
	case s.ep.trans <- struct{}{}:
	default: // 取り直しが重なったら、最後の1回だけ
	}
}

// runTransition はトランジションの取り直しを担うエポックの goroutine。取り直しが重なったら、
// 終わったあとに最後の1回だけ取り直す。
func (w *WS) runTransition(ep *epoch, c *mainConn) {
	for {
		select {
		case <-ep.ctx.Done():
			return
		case <-ep.trans:
		}
		ms, err := w.currentTransitionMs(ep.ctx, c)
		if ep.ctx.Err() != nil || sessionLost(err) {
			return
		}
		if err != nil {
			w.log.Warn("トランジションの長さを取得できない", "err", err)
		}
		w.emitEpoch(ep, switcher.TransitionChanged{At: time.Now(), TransitionMs: ms})
	}
}

func (w *WS) currentTransitionMs(ctx context.Context, c *mainConn) (*int, error) {
	r, err := mcall(w, ctx, c, "GetCurrentSceneTransition", func(cli *goobs.Client) (*transitions.GetCurrentSceneTransitionResponse, error) {
		return cli.Transitions.GetCurrentSceneTransition()
	})
	if err != nil {
		return nil, err
	}
	return transitionMs(r.TransitionFixed, r.TransitionDuration), nil
}

// fetchConnected は接続直後の状態を取得して OBSConnected を作る。
func (w *WS) fetchConnected(ctx context.Context, c *mainConn, legacy bool) (switcher.OBSConnected, error) {
	st, err := mcall(w, ctx, c, "GetStreamStatus", func(cli *goobs.Client) (*stream.GetStreamStatusResponse, error) {
		return cli.Stream.GetStreamStatus()
	})
	if err != nil {
		return switcher.OBSConnected{}, fmt.Errorf("GetStreamStatus: %w", err)
	}
	sc, err := mcall(w, ctx, c, "GetCurrentProgramScene", func(cli *goobs.Client) (*scenes.GetCurrentProgramSceneResponse, error) {
		return cli.Scenes.GetCurrentProgramScene()
	})
	if err != nil {
		return switcher.OBSConnected{}, fmt.Errorf("GetCurrentProgramScene: %w", err)
	}
	name := sc.SceneName
	if name == "" {
		name = sc.CurrentProgramSceneName
	}
	uuid := sc.SceneUuid
	if uuid == "" {
		uuid = sc.CurrentProgramSceneUuid
	}
	if legacy {
		uuid = name
	}
	// 長さが取れなくても接続は成り立つ。nil にする。
	ms, err := w.currentTransitionMs(ctx, c)
	if err != nil {
		if sessionLost(err) || ctx.Err() != nil {
			return switcher.OBSConnected{}, fmt.Errorf("GetCurrentSceneTransition: %w", err)
		}
		w.log.Warn("トランジションの長さを取得できない", "err", err)
		ms = nil
	}
	return switcher.OBSConnected{
		At:           time.Now(),
		Streaming:    streamingFromStatus(st.OutputActive, st.OutputReconnecting),
		Scene:        name,
		SceneUUID:    uuid,
		TransitionMs: ms,
	}, nil
}

// checkNames は config のシーン名とメディアソースが OBS にあるかを確かめ、なければ警告にする。
func (w *WS) checkNames(ctx context.Context, c *mainConn) {
	var warns []string
	if l, err := mcall(w, ctx, c, "GetSceneList", func(cli *goobs.Client) (*scenes.GetSceneListResponse, error) {
		return cli.Scenes.GetSceneList()
	}); err != nil {
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
		if l, err := mcall(w, ctx, c, "GetInputList", func(cli *goobs.Client) (*inputs.GetInputListResponse, error) {
			return cli.Inputs.GetInputList()
		}); err != nil {
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
	w.warnBase = warns
	w.mu.Unlock()
}
