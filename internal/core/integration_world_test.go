package core

import (
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 結合の試験の「偽の OBS の世界」（カメラの spec 第5節、表 0b の6章）。
//
// core と Machine の内部は読まない。判定は、この世界の事実（誰がどの項目をいつ非表示にしたか）と、
// hidden.json のファイルの中身だけで行う。世界は obs.Client として core に差し込まれる。
//
// 世界のイベントは、応答より前に流れる（sync）、応答のあとに遅れて流れる（due まで待つ）、重なる（2回）が
// 乱数で選ばれる。順序は保つ。切断すると、まだ流れていないイベントは消える。

type ik struct {
	c  string // 持ち主のシーンかグループの UUID（名前と同じにする）
	id int
}

type wItem struct {
	id      int
	source  string
	kind    scenemap.SourceKind
	enabled bool
}

type wCont struct {
	name  string
	group bool
	items []*wItem
}

func (c *wCont) find(id int) *wItem {
	for _, it := range c.items {
		if it.id == id {
			return it
		}
	}
	return nil
}

type pendEv struct {
	due      time.Time
	ev       switcher.Event
	human    bool
	appScene bool // 0c: 本体の SetScene の適用で出た SceneChanged
	key      ik   // 人の項目の変化のとき
	val      bool
}

type lateApply struct {
	due time.Time
	key ik
	val bool
}

// camera の id と、世界のカメラの入力。
const (
	wMain, wSub, wHand = "main", "sub", "hand"
)

var wCameraIDs = []string{wMain, wSub, wHand}

type world struct {
	mu   sync.Mutex
	rng  *rand.Rand
	clk  *fakeClock
	done chan struct{} // 世界の終わり
	// 世代ごとのイベントの経路。世代が替わると古い送り手は genDone で諦める。
	gen     int
	evCh    chan switcher.Event
	genDone chan struct{}

	cont      map[string]*wCont
	order     []string // シーンとグループの並び（地図と表示の順を決める）
	inputs    map[string]scenemap.Input
	program   string
	streaming bool
	studio    bool
	connected bool
	faulty    bool
	benign    bool // 回復の区間: 要求はすべて成功し、イベントも欠けない
	autoOn    bool // この世代の auto_enabled
	studioHid string
	nextItem  int

	pending []pendEv
	late    []lateApply
	hung    []hungReq

	// 本体が最後に true→false にした項目（人が触れば外れる）。
	appOwned map[ik]bool
	// INV-03: 人が戻した印（カメラごと）。markPend は人の true を流す前に立て、流れたあとに mark にする。
	mark map[string]bool
	// humanRestoredOwned は、人が本体の隠した項目を戻した（戻してよい理由がなかった）項目。流れたら印にする。
	humanRestoredOwned map[ik]bool
	// trueIssued は、人の操作のあとに本体が true の要求を出した項目。人の true のイベントが届いたとき、本体の期待と
	// 区別できない（spec 第5節「イベントを取りこぼした人の操作は見分けられない」）ので、印の判定から外す。
	trueIssued map[ik]bool
	// uncertain は、false の応答が Timeout だった項目（適用されたかわからない）。本体が true を送って OK を得たら
	// cleared になる。そのあとに遅れて適用された false は、前提の外の既知の制約（記録を消したあとの遅れた適用）。
	uncertain map[ik]*bool

	judges map[string]*ingest.Judge // カメラごとの独立の Judge
	kbps   map[string]float64

	hiddenPath string

	violations []string
	trace      []string
	activity   atomic.Int64
	evCount    atomic.Int64 // 本体へ流したイベントの数（0c: 履歴が変わるのはイベントかコマンドのあと）
	stats      worldStats

	// 0c の拡張（nil なら 0b と同じ世界）。
	fb       *fbWorld
	humanWin bool // この1秒に人の操作があった（本体の判断と偽の OBS の事実がずれうる窓。0c の判定を控える）
	nextApp  bool // 次に enqueueLocked するイベントは本体の SetScene 由来
	// forceDelay は次の enqueueLocked の遅れ（期待の期限を超えさせる。0c）。
	forceDelay time.Duration
}

type worldStats struct {
	appFalse, appTrue, timeouts, lateApplies, humanRestores, restarts, crashes, showAll int
	marks, issuesWithMark                                                               int
}

func newWorld(seed int64, clk *fakeClock, hiddenPath string, faulty bool) *world {
	w := &world{
		rng: rand.New(rand.NewSource(seed)), clk: clk, done: make(chan struct{}),
		inputs: map[string]scenemap.Input{
			"IRL":  {Name: "IRL", CameraID: wMain},
			"SUB":  {Name: "SUB", CameraID: wSub},
			"HAND": {Name: "HAND", CameraID: wHand},
		},
		cont: map[string]*wCont{}, program: camera, streaming: true, connected: true, faulty: faulty,
		hiddenPath: hiddenPath, appOwned: map[ik]bool{}, mark: map[string]bool{}, humanRestoredOwned: map[ik]bool{}, trueIssued: map[ik]bool{}, uncertain: map[ik]*bool{}, nextItem: 100,
		kbps: map[string]float64{},
	}
	add := func(name string, group bool, items ...*wItem) {
		w.cont[name] = &wCont{name: name, group: group, items: items}
		w.order = append(w.order, name)
	}
	in := func(id int, src string) *wItem {
		return &wItem{id: id, source: src, kind: scenemap.InputSource, enabled: true}
	}
	add(standby, false)
	add(camera, false, in(1, "IRL"), in(2, "SUB"))
	add("Both", false, in(1, "SUB"), in(2, "HAND"))
	add("Hand", false, in(1, "HAND"))
	add(talk, false)
	add("Nest", false, &wItem{id: 1, source: camera, kind: scenemap.SceneSource, enabled: true}, in(2, "HAND"))
	add("G1", true, in(1, "SUB"), in(2, "IRL"))
	add("Grp", false, &wItem{id: 1, source: "G1", kind: scenemap.GroupSource, enabled: true}, in(2, "HAND"))
	w.resetJudges()
	w.newGeneration()
	return w
}

func (w *world) resetJudges() {
	w.judges = map[string]*ingest.Judge{}
	for _, id := range w.cameraIDs() {
		w.judges[id] = ingest.NewJudge(testThresholds)
	}
}

func (w *world) logf(f string, a ...any) {
	line := fmt.Sprintf("[%s] ", w.clk.Now().Format("15:04:05")) + fmt.Sprintf(f, a...)
	w.trace = append(w.trace, line)
	if len(w.trace) > 600 {
		w.trace = w.trace[len(w.trace)-400:]
	}
}

func (w *world) violate(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	w.violations = append(w.violations, msg)
	w.logf("VIOLATION %s", msg)
}

// newGeneration は新しい本体の世代のための経路を作る。古い世代の送り手は止める。
func (w *world) newGeneration() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.genDone != nil {
		close(w.genDone)
	}
	w.gen++
	w.evCh = make(chan switcher.Event)
	w.genDone = make(chan struct{})
	w.pending = nil
	w.mark = map[string]bool{}
	w.humanRestoredOwned = map[ik]bool{}
	w.trueIssued = map[ik]bool{}
	w.releaseHungLocked()
}

func (w *world) close() {
	w.mu.Lock()
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	w.releaseHungLocked()
	w.mu.Unlock()
}

type hungReq struct {
	ch  chan struct{}
	due time.Time
}

func (w *world) releaseHungLocked() {
	for _, h := range w.hung {
		close(h.ch)
	}
	w.hung = nil
}

// releaseHungDue は、応えない要求を、OBS の応答待ちの上限（5秒。obs は ErrTimeout を返す）で解く。
func (w *world) releaseHungDue(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var rest []hungReq
	for _, h := range w.hung {
		if h.due.After(now) {
			rest = append(rest, h)
		} else {
			close(h.ch)
		}
	}
	w.hung = rest
}

// ---- 地図と「映る」（独立の計算） ----

func (w *world) snapshotMapLocked() *scenemap.Map {
	m := &scenemap.Map{Collection: testCollection, Scenes: map[string]scenemap.Container{}, Groups: map[string]scenemap.Container{},
		Inputs: map[string]scenemap.Input{}}
	for k, v := range w.inputs {
		m.Inputs[k] = v
	}
	for _, name := range w.order {
		c := w.cont[name]
		ct := scenemap.Container{Name: c.name}
		for _, it := range c.items {
			ct.Items = append(ct.Items, scenemap.Item{ID: it.id, SourceUUID: it.source, SourceKind: it.kind, Enabled: it.enabled})
		}
		if c.group {
			m.Groups[name] = ct
		} else {
			m.Scenes[name] = ct
		}
	}
	return m
}

// visible は sceneUUID に映るカメラ。countApp が true なら、本体が隠した項目は表示として数える。
// override は、ある項目を false として数える仮の指定。
func (w *world) visibleLocked(scene string, countApp bool, override map[ik]bool) map[string]bool {
	out := map[string]bool{}
	var walk func(c string, path map[string]bool)
	walk = func(c string, path map[string]bool) {
		ct := w.cont[c]
		if ct == nil || path[c] {
			return
		}
		path[c] = true
		defer delete(path, c)
		for _, it := range ct.items {
			k := ik{c, it.id}
			en := it.enabled
			if countApp && w.appOwned[k] {
				en = true
			}
			if v, ok := override[k]; ok {
				en = v
			}
			if !en {
				continue
			}
			switch it.kind {
			case scenemap.InputSource:
				if cam := w.inputs[it.source].CameraID; cam != "" {
					out[cam] = true
				}
			default:
				walk(it.source, path)
			}
		}
	}
	walk(scene, map[string]bool{})
	return out
}

// cameraOfItem は項目のソースのカメラ（入力でなければ空）。
func (w *world) cameraOfItemLocked(k ik) string {
	if c := w.cont[k.c]; c != nil {
		if it := c.find(k.id); it != nil && it.kind == scenemap.InputSource {
			return w.inputs[it.source].CameraID
		}
	}
	return ""
}

// ---- イベントの流し方 ----

func (w *world) humanPendingLocked() bool {
	return slices.ContainsFunc(w.pending, func(p pendEv) bool { return p.human })
}

// enqueueLocked はイベントを、応答のあとに遅れて流すものとして入れる（順序は保つ）。
func (w *world) enqueueLocked(ev switcher.Event, human bool, k ik, val bool) {
	if !w.connected {
		return // 切断中はイベントを受け取る相手がいない
	}
	due := w.clk.Now()
	if !w.benign && w.rng.Intn(3) > 0 {
		due = due.Add(time.Duration(w.rng.Intn(3000)) * time.Millisecond)
	}
	if w.forceDelay > 0 {
		due = w.clk.Now().Add(w.forceDelay)
		w.forceDelay = 0
	}
	if n := len(w.pending); n > 0 && w.pending[n-1].due.After(due) {
		due = w.pending[n-1].due
	}
	app := w.nextApp
	w.nextApp = false
	w.pending = append(w.pending, pendEv{due: due, ev: ev, human: human, key: k, val: val, appScene: app})
	if !w.benign && w.rng.Intn(8) == 0 { // 重なり
		w.pending = append(w.pending, pendEv{due: due, ev: ev, human: human, key: k, val: val, appScene: app})
	}
}

// send はイベントを本体へ流す。mu を持たずに呼ぶ。世代が替われば諦める。
func (w *world) send(ch chan switcher.Event, gd chan struct{}, ev switcher.Event) {
	select {
	case ch <- ev:
		w.evCount.Add(1)
		w.activity.Add(1)
	case <-gd:
	case <-w.done:
	}
}

// deliverDue は due を過ぎたイベントを順に流す。
func (w *world) deliverDue(all bool) {
	for {
		w.mu.Lock()
		if len(w.pending) == 0 || (!all && w.pending[0].due.After(w.clk.Now())) {
			w.mu.Unlock()
			return
		}
		p := w.pending[0]
		w.pending = w.pending[1:]
		ch, gd := w.evCh, w.genDone
		w.mu.Unlock()
		w.send(ch, gd, p.ev)
		// 印は、本体の中核ループがイベントを受け取ったあとに立てる（受け取る前に承認された要求は、人の操作を知らない）。
		if p.human && p.ev != nil {
			if ie, ok := p.ev.(switcher.ItemEnabledChanged); ok && ie.Enabled {
				w.mu.Lock()
				w.onHumanTrueDeliveredLocked(p.key)
				w.mu.Unlock()
			}
		}
	}
}

// sendNow は、先に流すべきイベントをすべて流してから、このイベントを流す（応答より前のイベント）。
func (w *world) sendNow(ev switcher.Event) {
	w.deliverDue(true)
	w.mu.Lock()
	ch, gd := w.evCh, w.genDone
	connected := w.connected
	w.mu.Unlock()
	if connected {
		w.send(ch, gd, ev)
	}
}

// ---- INV-03: 人が戻した印 ----

// canRestoreOracle は、独立に計算した「そのカメラの本体の項目を戻してよい」条件（Machine の分類は使わない）。
func (w *world) canRestoreOracleLocked(cam string) bool {
	if !w.streaming || !w.autoOn || w.judges[cam].State() == ingest.LinkOK {
		return true
	}
	if w.isStandbyCam(cam) { // main（と alt）は on_bad: standby
		return true
	}
	return w.studio && w.studioHid != switcher.StudioHideHide
}

func (w *world) onHumanTrueDeliveredLocked(k ik) {
	// 人が戻した項目が、本体の隠した項目で、戻してよい理由がなければ、そのカメラに印を立てる。
	cam := w.cameraOfItemLocked(k)
	if cam == "" || !w.humanRestoredOwned[k] {
		return
	}
	delete(w.humanRestoredOwned, k)
	if w.trueIssued[k] {
		w.logf("印を立てない: 人の true %v が届くまでに、本体も true を出していた（区別できない）", k)
		return
	}
	if !w.canRestoreOracleLocked(cam) {
		w.mark[cam] = true
		w.stats.marks++
		w.logf("印を立てた: %s（人の true %v が本体に届いた）", cam, k)
	}
}
