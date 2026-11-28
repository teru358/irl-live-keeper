package core

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strings"
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
	wMain, wSub, wHand = "cam1", "cam2", "cam3"
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
	// humanTransit は、pending から取り出して本体へ送っている途中（本体がまだ受け取っていない）の人の操作の数。
	// 世界はもう変わっているのに本体は知らないので、pending にあるのと同じに数える。
	humanTransit int
	late         []lateApply
	hung         []hungReq

	// 本体が最後に true→false にした項目（人が触れば外れる）。
	appOwned map[ik]bool
	// INV-03: 人が戻した印（カメラごと）。markPend は人の true を流す前に立て、流れたあとに mark にする。
	mark map[string]bool
	// markSeq は印を立てた順（seq の値）。approvedSeq は本体が false を承認した順。承認が印より前なら、その false は
	// 人の true を知らずに決めたもの（承認から OBS への要求までの間に、人の true が本体に届きうる）。
	markSeq     map[string]int64
	approvedSeq map[ik]int64
	seq         int64
	// humanTrueAt は、人の true のイベントの At（目印）。本体の中核ループが処理した時点で印にする。
	humanTrueAt map[time.Time]ik
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
	// writeMu と writeGen は、hidden.json への書き込みを本体の世代で区切る。落ちた本体（プロセスごと死んだもの）の書き込みは、
	// 死んだあとには起きない。試験では Run を止めても persister の worker が残って、新しい世代が書いた hidden.json を古い中身で
	// 上書きしうる（trackedHiddenPath の注意書きの「既知の制約」）ので、世代が替わったあとの古い世代の書き込みは捨てる。
	writeMu  sync.Mutex
	writeGen int

	violations []string
	trace      []string
	activity   atomic.Int64
	sendMu     sync.Mutex   // イベントを本体へ流す順（取り出しから送り終えまで）を守る
	reqSeq     atomic.Int64 // 始まった OBS の要求の通し番号
	inflight   atomic.Int64 // 実行中の OBS の要求の数（0c: 静かな時点の判定）
	evCount    atomic.Int64 // 本体へ流したイベントの数（0c: 履歴が変わるのはイベントかコマンドのあと）
	stats      worldStats

	// 実時間の短い待ちの代わりに、動きを数えて待つための印（settle、fbSync、hidden.json の書き込みの待ち）。
	wake           chan struct{} // 動きがあったことを待ち手に知らせる（容量1。取りこぼさないが、余分に起きることはある）
	coreBusy       atomic.Int64  // 中核ループが列に積んだバッチのうち、BatchDone をまだ処理していない数（積んだ・実行役が受け取った・終えた）
	persistObs     atomic.Int64  // 中核ループが最後の observe で見た persistRev（hidden.json の保存を求めた最後の番号）
	settleTimedOut atomic.Int64  // settle が時間切れで諦めた回数（診断用）
	// 食い違いの診断（失敗の出力の頭に出す）。世界の program を変えた履歴と、本体に渡した SceneChanged の列。実時間つき。
	progHist []string
	sceneLog []string

	// 0c の拡張（nil なら 0b と同じ世界）。
	fb       *fbWorld
	humanWin bool // この1秒に人の操作があった（本体の判断と偽の OBS の事実がずれうる窓。0c の判定を控える）
	nextApp  bool // 次に enqueueLocked するイベントは本体の SetScene 由来
	// forceDelay は次の enqueueLocked の遅れ（期待の期限を超えさせる。0c）。
	forceDelay time.Duration
}

type worldStats struct {
	appFalse, appTrue, timeouts, lateApplies, humanRestores, restarts, crashes, showAll int
	marks, issuesWithMark, approvedBeforeMark                                           int
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
		hiddenPath: hiddenPath, appOwned: map[ik]bool{}, mark: map[string]bool{}, markSeq: map[string]int64{}, approvedSeq: map[ik]int64{}, humanTrueAt: map[time.Time]ik{}, humanRestoredOwned: map[ik]bool{}, trueIssued: map[ik]bool{}, uncertain: map[ik]*bool{}, nextItem: 100,
		kbps: map[string]float64{}, wake: make(chan struct{}, 1),
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
	w.markSeq = map[string]int64{}
	w.approvedSeq = map[ik]int64{}
	w.humanTrueAt = map[time.Time]ik{}
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
	return w.humanTransit > 0 || slices.ContainsFunc(w.pending, func(p pendEv) bool { return p.human })
}

// enqueueLocked はイベントを、応答のあとに遅れて流すものとして入れる（順序は保つ）。
func (w *world) enqueueLocked(ev switcher.Event, human bool, k ik, val bool) {
	if !w.connected {
		return // 切断中はイベントを受け取る相手がいない
	}
	if ie, ok := ev.(switcher.ItemEnabledChanged); ok && human && val {
		// 人の true の目印。At を1ナノ秒ずつ過去へずらして一意にする（Machine は At を期限の判定にしか使わず、過去へのずれは影響しない）。
		w.seq++
		ie.At = ie.At.Add(-time.Duration(w.seq) * time.Nanosecond)
		w.humanTrueAt[ie.At] = k
		ev = ie
	}
	due := w.clk.Now()
	if !w.benign && w.rng.Intn(3) > 0 {
		due = due.Add(time.Duration(w.rng.Intn(3000)) * time.Millisecond)
	}
	if _, isMap := ev.(switcher.SceneMapChanged); isMap && w.fb != nil && !w.benign && w.rng.Intn(4) == 0 {
		// 地図の読み直しは補助の接続の結果で、本来の接続のイベントと順が入れ替わりうる（先に届く）。
		due = w.clk.Now()
		idx := len(w.pending)
		for i, p := range w.pending {
			if p.due.After(due) {
				idx = i
				w.fb.count("地図の読み直しが先のイベントを追い越した")
				break
			}
		}
		// 地図の読み直し同士の順は保つ（古い地図が新しい地図のあとに届かない）。
		for i := len(w.pending) - 1; i >= 0; i-- {
			if _, ok := w.pending[i].ev.(switcher.SceneMapChanged); ok {
				idx = max(idx, i+1)
				break
			}
		}
		w.pending = slices.Insert(w.pending, idx, pendEv{due: due, ev: ev, human: human, key: k, val: val})
		return
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
		switch x := ev.(type) {
		case switcher.SceneChanged:
			w.mu.Lock()
			w.logf("届けた: SceneChanged %s", x.Scene)
			w.sceneLog = appendCap(w.sceneLog, fmt.Sprintf("%s 本体に渡した SceneChanged %s（%s）", time.Now().Format("15:04:05.000000"), x.Scene, x.SceneUUID), diagKeep)
			w.mu.Unlock()
		case switcher.OBSConnected: // 本体は OBSConnected の Scene からも今のシーンを決めるので、列に入れる
			w.mu.Lock()
			w.sceneLog = appendCap(w.sceneLog, fmt.Sprintf("%s 本体に渡した OBSConnected（シーン %s、%s）", time.Now().Format("15:04:05.000000"), x.Scene, x.SceneUUID), diagKeep)
			w.mu.Unlock()
		case switcher.OBSDisconnected:
			w.mu.Lock()
			w.sceneLog = appendCap(w.sceneLog, fmt.Sprintf("%s 本体に渡した OBSDisconnected", time.Now().Format("15:04:05.000000")), diagKeep)
			w.mu.Unlock()
		}
		w.evCount.Add(1)
		w.activity.Add(1)
		w.poke()
	case <-gd:
	case <-w.done:
	}
}

// deliverDue は due を過ぎたイベントを順に流す。
func (w *world) deliverDue(all bool) {
	w.sendMu.Lock()
	defer w.sendMu.Unlock()
	w.deliverDueSerial(all)
}

// deliverDueSerial は sendMu を持って呼ぶ（取り出してから送り終えるまでの間に、別の goroutine が新しいイベントを先に送らない）。
func (w *world) deliverDueSerial(all bool) {
	for {
		w.mu.Lock()
		if len(w.pending) == 0 || (!all && w.pending[0].due.After(w.clk.Now())) {
			w.mu.Unlock()
			return
		}
		p := w.pending[0]
		w.pending = w.pending[1:]
		ch, gd := w.evCh, w.genDone
		if p.human {
			w.humanTransit++ // 取り出してから本体が受け取るまでの間も、人の操作は未着
		}
		w.mu.Unlock()
		w.send(ch, gd, p.ev)
		if p.human {
			w.mu.Lock()
			w.humanTransit--
			w.mu.Unlock()
		}
	}
}

// sendNow は、先に流すべきイベントをすべて流してから、このイベントを流す（応答より前のイベント）。
func (w *world) sendNow(ev switcher.Event) {
	w.sendMu.Lock()
	defer w.sendMu.Unlock()
	w.sendNowSerial(ev)
}

// sendNowSerial は sendMu を持って呼ぶ。世界の状態を変えてからこのイベントを流すまでを、sendMu の下で続けて行う呼び出し側
// （偽の OBS の要求、人の操作）は、世界が変わった順と、本体に届くイベントの順が入れ替わらない。ロックの順は sendMu → mu。
func (w *world) sendNowSerial(ev switcher.Event) {
	w.deliverDueSerial(true)
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
	if w.isStandbyCam(cam) { // cam1（と alt）は on_bad: standby
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
		w.seq++
		w.markSeq[cam] = w.seq
		w.stats.marks++
		w.logf("印を立てた: %s（人の true %v が本体に届いた）", cam, k)
	}
}

// observeCore は本体の中核ループが処理した Event と承認を、中核ループの goroutine で受ける（INV-03 の順序の判定）。
// 人の true は、中核ループが処理した時点で印にする（受け取ったあとに承認される要求は、それを知っている）。
// false の承認の順も記録する（承認から OBS への要求までの間に人の true が届いても、承認の時点では知らなかった）。
func (w *world) observeCore(v any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch x := v.(type) {
	case switcher.ItemEnabledChanged:
		if k, ok := w.humanTrueAt[x.At]; ok && x.Enabled {
			w.onHumanTrueDeliveredLocked(k)
		}
	case approval:
		if _, ok := x.Result.(switcher.Proceed); !ok {
			return
		}
		if a, ok := x.Action.(switcher.SetItemEnabled); ok && !a.Enabled {
			w.seq++
			w.approvedSeq[ik{a.SceneUUID, a.ItemID}] = w.seq
		}
	}
}

// ---- 動きを数えて待つ（実時間の短い待ちの代わり） ----

const diagKeep = 60 // 診断に残す履歴の数

func appendCap(l []string, s string, n int) []string {
	l = append(l, s)
	if len(l) > n*2 {
		l = slices.Clone(l[len(l)-n:])
	}
	return l
}

// noteProgramLocked は世界の program を変えた直後に呼ぶ（診断の履歴。実時間と偽の時計つき）。
func (w *world) noteProgramLocked(why string) {
	w.progHist = appendCap(w.progHist, fmt.Sprintf("%s [%s] 世界の program を %s（%s）に: %s", time.Now().Format("15:04:05.000000"), w.clk.Now().Format("15:04:05"), w.sceneNameLocked(w.program), w.program, why), diagKeep)
}

// diagString は、状態の食い違いのときに読む診断（世界の program の履歴と、本体に渡した SceneChanged の列）。
func (w *world) diagString() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	last := func(l []string) string {
		if len(l) > diagKeep {
			l = l[len(l)-diagKeep:]
		}
		return strings.Join(l, "\n")
	}
	return fmt.Sprintf("世界の program の履歴（古い順、直近 %d）:\n%s\n本体に渡した SceneChanged の列（古い順、直近 %d）:\n%s", diagKeep, last(w.progHist), diagKeep, last(w.sceneLog))
}

// poke は待ち手に動きを知らせる。待ち手がいなくても、1つは残る。
func (w *world) poke() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *world) drainWake() {
	select {
	case <-w.wake:
	default:
	}
}

// integWaits は、動きを待つ処理が、知らせで起きた回数と、保険の時間切れで起きた回数（時間切れが多ければ、知らせの抜けがある）。
var integWaits struct{ poked, timedOut, settleGaveUp atomic.Int64 }

// waitWake は動きの知らせか、max（取りこぼしの保険）のどちらか早いほうまで待つ。
func (w *world) waitWake(max time.Duration) {
	t := time.NewTimer(max)
	defer t.Stop()
	select {
	case <-w.wake:
		integWaits.poked.Add(1)
	case <-t.C:
		integWaits.timedOut.Add(1)
	}
}

// reportIntegWaits は、知らせで起きた回数と時間切れの回数を、IRL_INTEG_STATS があれば標準エラーに出す（負荷の試験の確かめ用）。
func reportIntegWaits(name string) {
	if os.Getenv("IRL_INTEG_STATS") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "INTEG-WAITS %s: 知らせで起きた=%d 保険の時間切れ=%d settle の打ち切り=%d\n", name, integWaits.poked.Load(), integWaits.timedOut.Load(), integWaits.settleGaveUp.Load())
}

// reqStart と reqEnd は、OBS の要求の実行中の数（inflight）の出入り。終わりは待ち手に知らせる。
func (w *world) reqStart() { w.inflight.Add(1) }
func (w *world) reqEnd() {
	w.inflight.Add(-1)
	w.poke()
}

// coreIdle は、世界の側から見て、本体が静かかを答える。実行中の OBS の要求がなく、中核ループが積んだバッチがすべて終わっている。
// 応えない要求（hung）を待っている実行役は、偽の時計が進むまで進めないので、静かと数える。
func (w *world) coreIdle() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.hung) > 0 {
		return true
	}
	return w.inflight.Load() == 0 && w.coreBusy.Load() == 0
}

// observeGen は本体の1世代の中核ループのフック（中核ループの goroutine で呼ばれる）。observeCore に加えて、
// 列に積まれたバッチと BatchDone を数える（積んだ数から終えた数を引いた残りが coreBusy）。Replace で列の先頭に入る代わりのバッチも、
// その approval の observe で列を見て拾う。保存を求めた最後の番号（persistRev）も写す。
func (w *world) observeGen(c *Core) func(any) {
	seen, done := map[int]bool{}, map[int]bool{}
	busy := 0
	return func(v any) {
		w.observeCore(v)
		for _, b := range c.queue {
			if !seen[b.BatchID] {
				seen[b.BatchID] = true
				busy++
			}
		}
		if d, ok := v.(switcher.BatchDone); ok && seen[d.BatchID] && !done[d.BatchID] {
			done[d.BatchID] = true
			busy--
		}
		changed := w.coreBusy.Swap(int64(busy)) != int64(busy)
		w.persistObs.Store(int64(c.persistRev))
		if _, tick := v.(switcher.Tick); !tick || changed {
			w.poke() // Tick は待ち手自身が送るので、知らせない（知らせると待ちが空回りする）
		}
	}
}

// pokeAfterWrite は hidden.json の書き込みの関数を包み、書き終えたら待ち手に知らせる（inner が nil なら既定の書き込み）。
func (w *world) pokeAfterWrite(inner func(string, []byte) error) func(string, []byte) error {
	if inner == nil {
		inner = atomicWrite
	}
	return func(p string, b []byte) error {
		err := inner(p, b)
		w.poke()
		return err
	}
}

var errDeadGen = errors.New("この世代の本体は終わっている（書き込みは起きない）")

// gateWrite は世代 gen の hidden.json の書き込みの関数。writeGen が gen でなければ（古い世代なら）何も書かずに失敗を返す。
// 書き込み中に世代が替わるときは、writeMu で書き込みの終わりを待つ。
func (w *world) gateWrite(gen int, inner func(string, []byte) error) func(string, []byte) error {
	if inner == nil {
		inner = atomicWrite
	}
	return func(p string, b []byte) error {
		w.writeMu.Lock()
		defer w.writeMu.Unlock()
		if w.writeGen != gen {
			return errDeadGen
		}
		return inner(p, b)
	}
}

// switchWriteGen は書き込みの世代を替える（古い世代の書き込み中なら、その終わりを待つ。以後の古い世代の書き込みは捨てられる）。
func (w *world) switchWriteGen(gen int) {
	w.writeMu.Lock()
	w.writeGen = gen
	w.writeMu.Unlock()
}
