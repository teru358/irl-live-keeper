package obs

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/requests/general"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/requests/stream"
	"github.com/andreykaipov/goobs/api/requests/transitions"
	"github.com/andreykaipov/goobs/api/requests/ui"

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
	bnd         boundaries
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

// errEventOverflow は、core へ流すのが遅れて、取っておくイベントが上限を超えたこと。
var errEventOverflow = errors.New("OBS のイベントの列があふれた")

// queued は、goobs から読んだイベント1つ。seq は読んだ順の通し番号（1 から）。
type queued struct {
	ev  any
	at  time.Time
	seq uint64
}

// evq は goobs の IncomingEvents を読む goroutine（pump）と、session の goroutine の間の、順を保つ列。
// session が core への送信で止まっても、pump は読み続ける（goobs の列は容量100で、あふれると古いものを
// 捨てるため）。列は max 件まで。超えたら pump が Client を捨てる（取りこぼした状態で動き続けない）。
type evq struct {
	mu     sync.Mutex
	items  []queued
	closed bool
	max    int
	sig    chan struct{} // 容量1。列に入れた、閉じた、とき
	n      atomic.Uint64 // 読んだ数

	// pump が、門を先に閉じるために持つ、今の program のシーン（SceneRemoved の判定）。
	curMu  sync.Mutex
	cur    string
	curSeq uint64
	legacy atomic.Bool // 古い OBS では、シーンの識別に名前を使う
}

func (q *evq) sceneID(uuid, name string) string {
	if q.legacy.Load() {
		return name
	}
	return uuid
}

func (q *evq) setCur(id string, seq uint64) {
	q.curMu.Lock()
	q.cur, q.curSeq = id, seq
	q.curMu.Unlock()
}

// initCur は読み取りの結果の program のシーンを入れる。pump がもっと新しい変化を読んでいたら入れない。
func (q *evq) initCur(id string, snapshotSeq uint64) {
	q.curMu.Lock()
	if q.curSeq <= snapshotSeq {
		q.cur = id
	}
	q.curMu.Unlock()
}

func (q *evq) isCur(id string) bool {
	q.curMu.Lock()
	defer q.curMu.Unlock()
	return q.cur != "" && q.cur == id
}

func newEvq(max int) *evq { return &evq{max: max, sig: make(chan struct{}, 1)} }

func (q *evq) wake() {
	select {
	case q.sig <- struct{}{}:
	default:
	}
}

// push は列に足す。上限なら false。
func (q *evq) push(e queued) bool {
	q.mu.Lock()
	if len(q.items) >= q.max {
		q.mu.Unlock()
		return false
	}
	q.items = append(q.items, e)
	q.mu.Unlock()
	q.wake()
	return true
}

func (q *evq) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.wake()
}

// pop は先頭を1つ取る。なければ ok=false（closed は goobs の列が閉じられたこと）。
// まだ残りがあるか、閉じられていれば、もう一度 sig を立てる。
func (q *evq) pop() (e queued, ok, closed bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) > 0 {
		e, q.items = q.items[0], q.items[1:]
		ok = true
	}
	closed = q.closed
	if len(q.items) > 0 || q.closed {
		q.wake()
	}
	return e, ok, closed && !ok
}

// mark は「いま、読んだイベントの数」（goobs の列に残っているものも数える）。状態を読む要求の応答の
// 直後に呼ぶと、その応答より前に届いたイベントの数になる（goobs は届いた順に、イベントを列へ、応答を
// 応答の列へ入れる）。
// 既知の制約: goobs は応答とイベントの順番を見せないので、応答を受けてから mark を取るまでの数十
// マイクロ秒の窓に届いたイベントや、pump が列から取り出した直後のイベントは、境界がずれうる。
func (q *evq) mark(cli *goobs.Client) uint64 { return q.n.Load() + uint64(len(cli.IncomingEvents)) }

// pump は goobs のイベントを読んで evq に入れる。Client を捨てたら終わる。
func (w *WS) pump(c *mainConn, q *evq) {
	for {
		select {
		case <-c.dead:
			return
		case ev, ok := <-c.cli.IncomingEvents:
			if !ok {
				q.close()
				return
			}
			seq := q.n.Add(1)
			// session が core への送信で止まっていても、門はここですぐ閉じる。core への通知
			// （OBSDisconnected など）は、session が列の順に流す。
			w.closeGateEarly(c, q, ev, seq)
			if !q.push(queued{ev: ev, at: time.Now(), seq: seq}) {
				w.log.Warn("OBS のイベントが core へ流れず、取っておく上限を超えた。接続を捨てて読み直す", "max", q.max)
				w.setGate(c, false, true)
				c.kill(errEventOverflow)
				return
			}
		}
	}
}

// closeGateEarly は、コレクションの切替の始まり（Changing と、今の program のシーンの SceneRemoved）を
// 読んだ時点で、外からの要求の門を閉じる。
func (w *WS) closeGateEarly(c *mainConn, q *evq, ev any, seq uint64) {
	switch e := ev.(type) {
	case *events.CurrentSceneCollectionChanging:
		w.setGate(c, false, true)
	case *events.CurrentProgramSceneChanged:
		q.setCur(q.sceneID(e.SceneUuid, e.SceneName), seq)
	case *events.SceneRemoved:
		if !e.IsGroup && q.isCur(q.sceneID(e.SceneUuid, e.SceneName)) {
			w.setGate(c, false, true)
		}
	}
}

// boundaries は、状態を読む要求の応答の時点の evq.mark。これ以前に届いたイベントは、読んだ状態に含まれる。
type boundaries struct {
	scene, stream, trans, studio uint64
	items                        map[string]uint64 // シーンかグループの UUID（古い OBS では空）
}

// heldEvent は、読み取りの最中に届いたイベント。読み取りのあと OBSConnected の次に流す。
type heldEvent struct {
	ev  any
	at  time.Time
	seq uint64
}

// sess は1回の接続（goobs の Client 1本）の状態。session の goroutine だけが持つ。
type sess struct {
	w    *WS
	ctx  context.Context
	conn *mainConn
	q    *evq
	bnd  *boundaries // 今の接続の、読んだ状態の境界。列から取り出す動作中のイベントにも当てはめる

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

	// Studio Mode を切るときの一瞬の切替（spec F-9）を捨てるための状態。
	studio    bool             // 今の Studio Mode（読み取りの結果と、そのあとの StudioModeStateChanged）
	pend      []pendingEv      // 取っておいているイベント（届いた順）
	progPrev  string           // 取っておき始める前の program のシーン（sceneID）
	bounceEnd <-chan time.Time // 取っておく窓の終わり。窓がなければ nil
}

// pendingEv は、Studio Mode の間に取っておいているイベント1つ。
type pendingEv struct {
	ev any
	at time.Time
}

// studioBounceWindow は、Studio Mode の間の program の切替を取っておく長さ（最初の1つが届いてから）。
const studioBounceWindow = 250 * time.Millisecond

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

	s := &sess{w: w, ctx: ctx, conn: c, q: newEvq(w.eventQueueMax), first: true, reloadCh: make(chan reloadResult)}
	w.spawn(func() { w.pump(c, s.q) })
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
		case <-s.q.sig:
			qe, ok, closed := s.q.pop()
			if ok {
				s.onEvent(qe)
			} else if closed {
				w.log.Warn("OBS との接続が切れた")
				return
			}
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
		case <-s.bounceEnd:
			// 窓の終わりまでに StudioModeStateChanged false が来なかった。人の切替として流す。
			s.flushBounce()
		}
	}
}

// startReload は読み取りの goroutine を始める（前の読み取りは取り消す）。
func (s *sess) startReload() {
	s.cancelReload()
	s.state = stateReloading
	s.quiet = nil
	s.held, s.heldTrans, s.heldKick = nil, false, false
	s.dropBounce()
	s.reloadGen++
	gen := s.reloadGen
	rctx, cancel := context.WithCancel(s.ctx)
	s.reloadStop = cancel
	first, legacy := s.first, s.legacy
	if !s.w.spawn(func() {
		res := s.w.reload(rctx, s.conn, s.q, gen, first, legacy)
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
func (w *WS) reload(ctx context.Context, c *mainConn, q *evq, gen int, first, legacy bool) reloadResult {
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
	now := func() uint64 { return q.mark(c.cli) }
	res.bnd.items = map[string]uint64{}
	conn, err := w.fetchConnected(ctx, c, res.legacy, now, &res.bnd)
	if err != nil {
		res.err = err
		return res
	}
	w.checkNames(ctx, c)
	m, reason, err := w.readFirstMap(ctx, c, res.legacy, now, &res.bnd)
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
func (w *WS) readFirstMap(ctx context.Context, c *mainConn, legacy bool, now func() uint64, bnd *boundaries) (m *scenemap.Map, reason string, err error) {
	req := mainReq{w: w, ctx: ctx, c: c, now: now, bnd: bnd}
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
	s.bnd = &r.bnd
	s.q.legacy.Store(r.legacy)
	s.q.initCur(r.connected.SceneUUID, r.bnd.scene)
	c.legacy.Store(r.legacy)
	s.curUUID = r.connected.SceneUUID
	s.studio = r.connected.StudioMode
	s.dropBounce()
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
		if !s.afterSnapshot(h, &r.bnd) {
			continue // 読んだ状態に含まれる変化。流すと、接続時点より古い状態に戻してしまう
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

// afterSnapshot は、取っておいたイベントが、その状態を読んだ要求の応答より後に届いたものか。
// 前に届いたものは、読んだ状態に含まれる。
func (s *sess) afterSnapshot(h heldEvent, b *boundaries) bool {
	switch e := h.ev.(type) {
	case *events.CurrentProgramSceneChanged:
		return h.seq > b.scene
	case *events.StreamStateChanged:
		return h.seq > b.stream
	case *events.CurrentSceneTransitionDurationChanged:
		return h.seq > b.trans
	case *events.StudioModeStateChanged:
		return h.seq > b.studio
	case *events.SceneItemEnableStateChanged:
		if at, ok := b.items[s.sceneID(e.SceneUuid, e.SceneName)]; ok {
			return h.seq > at
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
	s.dropBounce()
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
func (s *sess) onEvent(qe queued) {
	ev, at := qe.ev, qe.at
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
		s.hold(ev, at, qe.seq)
	case stateRunning:
		// 読み取りの完了と列が同時に準備できたとき、列に残っている、読んだ状態より古いイベントは流さない。
		if s.bnd != nil && !s.afterSnapshot(heldEvent{ev, at, qe.seq}, s.bnd) {
			return
		}
		s.handleRunning(ev, at)
	}
}

// hold は読み取りの最中に届いたイベントを、OBSConnected のあとに流すために取っておく。
func (s *sess) hold(ev any, at time.Time, seq uint64) {
	switch ev.(type) {
	case *events.CurrentSceneTransitionChanged:
		s.heldTrans = true
	case *events.CurrentProgramSceneChanged, *events.CurrentSceneTransitionDurationChanged, *events.StreamStateChanged,
		*events.SceneItemEnableStateChanged, *events.StudioModeStateChanged:
		s.held = append(s.held, heldEvent{ev, at, seq})
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

// dropBounce は、取っておいたイベントを流さずに捨てる（切断、読み直し、コレクションの切替）。
// 今のシーンは、つなぎ直しの読み取りで読む。
func (s *sess) dropBounce() {
	s.pend, s.bounceEnd, s.progPrev = nil, nil, ""
}

// bounceHandle は Studio Mode を切るときの一瞬の切替（spec 第1節の F-9）の、取っておく・捨てる・流す。
// 取っておく・捨てるなら true（呼び出し側は何も流さない）。
//   - Studio Mode がオンと知っている間の CurrentProgramSceneChanged は、すぐ流さず、最初の1つから
//     studioBounceWindow の間、取っておく。取っておく間に届いた、Event に写して流すもの
//     （ItemEnabledChanged など）も、順を保つため後ろに並べる。
//   - Event を流さないもの（地図の読み直しを頼むだけの refresher.kick、トランジションの取り直しの kick、
//     InputSettingsChanged）は、順を保つ必要がないので、今までどおりすぐ扱う（false を返す）。
//   - curUUID（SceneRemoved の判定）は、届いたとおりに更新する。pump の closeGateEarly も届いたとおりに
//     更新して門を閉じており、実際の OBS の program は届いたシーンなので、それと揃える。
//     取っておく前の program のシーンは progPrev に別に覚える。
func (s *sess) bounceHandle(ev any, at time.Time) bool {
	pending := len(s.pend) > 0
	switch e := ev.(type) {
	case *events.CurrentProgramSceneChanged:
		if !s.studio && !pending {
			return false
		}
		if !pending {
			s.progPrev = s.curUUID
			s.bounceEnd = s.w.clock.After(studioBounceWindow)
		}
		s.curUUID = s.sceneID(e.SceneUuid, e.SceneName)
		s.pend = append(s.pend, pendingEv{ev, at})
		return true
	case *events.StudioModeStateChanged:
		s.studio = e.StudioModeEnabled
		if !pending {
			return false
		}
		if e.StudioModeEnabled {
			s.pend = append(s.pend, pendingEv{ev, at})
		} else {
			s.resolveBounce(ev, at)
		}
		return true
	case *events.CurrentSceneTransitionDurationChanged, *events.StreamStateChanged, *events.SceneItemEnableStateChanged:
		if !pending {
			return false
		}
		s.pend = append(s.pend, pendingEv{ev, at})
		return true
	}
	return false
}

// resolveBounce は、取っておく間に StudioModeStateChanged false が届いたときの扱い。
func (s *sess) resolveBounce(off any, at time.Time) {
	pend, prev := s.pend, s.progPrev
	s.dropBounce()
	last := -1
	var names []string
	for i, p := range pend {
		if e, ok := p.ev.(*events.CurrentProgramSceneChanged); ok {
			last = i
			names = append(names, e.SceneName)
		}
	}
	le := pend[last].ev.(*events.CurrentProgramSceneChanged)
	bounce := s.sceneID(le.SceneUuid, le.SceneName) == prev
	if bounce {
		s.w.log.Info("studio mode off bounce ignored", "scenes", names)
	}
	for i, p := range pend {
		if _, ok := p.ev.(*events.CurrentProgramSceneChanged); ok && (bounce || i != last) {
			continue // 捨てる。最後が違うときも、間の一瞬の切替は流さない
		}
		s.dispatch(p.ev, p.at)
	}
	s.dispatch(off, at)
}

// flushBounce は、窓が終わったときに、取っておいたイベントを届いた順に、届いた時刻のまま流す。
func (s *sess) flushBounce() {
	pend := s.pend
	s.dropBounce()
	if s.state != stateRunning {
		return
	}
	for _, p := range pend {
		s.dispatch(p.ev, p.at)
	}
}

// handleRunning は動作中のイベント1つを写して流す（Studio Mode を切るときの一瞬の切替は取っておく）。
func (s *sess) handleRunning(ev any, at time.Time) {
	if s.bounceHandle(ev, at) {
		return
	}
	s.dispatch(ev, at)
}

// dispatch は動作中のイベント1つを写して流す。要求は送らない。
func (s *sess) dispatch(ev any, at time.Time) {
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
func (w *WS) fetchConnected(ctx context.Context, c *mainConn, legacy bool, now func() uint64, bnd *boundaries) (switcher.OBSConnected, error) {
	st, err := mcall(w, ctx, c, "GetStreamStatus", func(cli *goobs.Client) (*stream.GetStreamStatusResponse, error) {
		return cli.Stream.GetStreamStatus()
	})
	if err != nil {
		return switcher.OBSConnected{}, fmt.Errorf("GetStreamStatus: %w", err)
	}
	bnd.stream = now()
	sc, err := mcall(w, ctx, c, "GetCurrentProgramScene", func(cli *goobs.Client) (*scenes.GetCurrentProgramSceneResponse, error) {
		return cli.Scenes.GetCurrentProgramScene()
	})
	if err != nil {
		return switcher.OBSConnected{}, fmt.Errorf("GetCurrentProgramScene: %w", err)
	}
	bnd.scene = now()
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
	// Studio Mode。読めなければ、GetStreamStatus などと同じに接続の失敗にする（つなぎ直す）。
	sm, err := mcall(w, ctx, c, "GetStudioModeEnabled", func(cli *goobs.Client) (*ui.GetStudioModeEnabledResponse, error) {
		return cli.Ui.GetStudioModeEnabled()
	})
	if err != nil {
		return switcher.OBSConnected{}, fmt.Errorf("GetStudioModeEnabled: %w", err)
	}
	bnd.studio = now()
	// 長さが取れなくても接続は成り立つ。nil にする。
	ms, err := w.currentTransitionMs(ctx, c)
	if err == nil {
		bnd.trans = now()
	}
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
		StudioMode:   sm.StudioModeEnabled,
	}, nil
}

// checkNames は config のシーン名（待機シーンと開始時のシーン）が OBS にあるかを確かめ、なければ警告にする。
// カメラのメディアソースの警告は、地図から作る。
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
	for _, m := range warns {
		w.log.Warn("OBS の設定の警告", "msg", m)
	}
	w.mu.Lock()
	w.warnBase = warns
	w.mu.Unlock()
}
