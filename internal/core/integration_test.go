package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 結合の試験: 乱数の入力の列を、偽の OBS の世界に対して流し、第5節の4つ（HB-INV-01〜04）をいつも確かめる。
// 判定は、世界の事実と hidden.json のファイルの中身だけ。core と Machine の内部（observe、Machine の状態）は読まない。
// 落ちたら、種、プロファイル、入力の列、世界の記録の末尾を出す。
// 回数: -short で少なく、ふだんは数百。IRL_INTEG_SEEDS で種の数を変えられる（深く回すとき）。

type gen struct {
	core    *Core
	cancel  context.CancelFunc
	done    chan struct{}
	samples chan ingest.SampleSet
	ticks   chan time.Time
}

type runner struct {
	t     *testing.T
	w     *world
	clk   *fakeClock
	rng   *rand.Rand
	path  string
	hw    *hiddenWrites // hidden.json の書き込みを数える（後片づけで待つ）
	cur   *gen
	steps []string
	fail  bool

	logBuf *syncBuf // 0c: 本体のログ（nil なら捨てる）
}

func (r *runner) stepf(f string, a ...any) {
	line := fmt.Sprintf(f, a...)
	r.steps = append(r.steps, line)
	r.w.mu.Lock()
	r.w.logf("入力: %s", line)
	r.w.mu.Unlock()
}

func (r *runner) fatalf(f string, a ...any) {
	r.fail = true
	r.w.mu.Lock()
	r.w.violate(f, a...)
	r.w.mu.Unlock()
}

func integCameras() []Camera {
	return []Camera{
		testCamera(wMain, "メイン", scenemap.MotionStandby),
		testCamera(wSub, "サブ", scenemap.MotionHide),
		testCamera(wHand, "手元", scenemap.MotionHide),
	}
}

// startGen は本体の世代を始める。hidden.json から読み、OBS につながっていれば OBSConnected を流す。
func (r *runner) startGen(autoOn bool, studioHide string) {
	w := r.w
	w.newGeneration()
	w.mu.Lock()
	w.autoOn, w.studioHid = autoOn, studioHide
	w.resetJudges()
	gnum := w.gen
	w.fbOnNewGenLocked()
	w.mu.Unlock()
	entries, warn := LoadHidden(r.path)
	g := &gen{samples: make(chan ingest.SampleSet), ticks: make(chan time.Time), done: make(chan struct{})}
	deps := Deps{
		OBS: &genClient{w: w, gen: gnum}, StandbyScene: standby, Cameras: w.deviceCameras(), AutoEnabled: autoOn,
		StudioModeHide: studioHide, HiddenPath: r.path, HiddenRestored: entries, HiddenLoadWarning: warn,
		HiddenWrite: r.hw.wrap(nil), HiddenRetry: 5 * time.Millisecond, StopTimeout: 300 * time.Millisecond,
		Samples: g.samples, Tick: g.ticks, Now: r.clk.Now,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	deps.Machine = switcher.New(switcher.Config{RestartWait: time.Millisecond, Endpoint: testEndpoint})
	if w.fb != nil {
		w.mu.Lock()
		deps.SceneFallbacks = slices.Clone(w.fb.rows)
		w.mu.Unlock()
		deps.Warnings = func() []string { return []string{fbObsWarning} }
		if r.logBuf != nil {
			deps.Log = slog.New(slog.NewTextHandler(r.logBuf, nil))
		}
	}
	g.core = New(deps)
	g.core.observe = w.observeCore
	if w.fb != nil {
		w.mu.Lock()
		w.fb.snap = g.core.Snapshot
		w.mu.Unlock()
	}
	ctx, cancel := context.WithCancel(context.Background())
	g.cancel = cancel
	go func() { defer close(g.done); g.core.Run(ctx) }()
	r.cur = g
	if warn != "" {
		r.fatalf("hidden.json を読めなかった: %s", warn)
	}
	w.mu.Lock()
	connected := w.connected
	w.mu.Unlock()
	if connected {
		r.sendConnected()
	}
}

func (r *runner) sendConnected() {
	w := r.w
	w.sendMu.Lock() // 接続の状態を立ててから OBSConnected を流すまでの間に、実行役のイベントが先に流れないように
	defer w.sendMu.Unlock()
	w.mu.Lock()
	w.pending = nil
	ev := switcher.OBSConnected{At: r.clk.Now(), Streaming: w.streaming, Scene: w.sceneNameLocked(w.program), SceneUUID: w.program,
		Map: w.snapshotMapLocked(), StudioMode: w.studio}
	ch, gd := w.evCh, w.genDone
	w.connected = true
	w.mu.Unlock()
	w.send(ch, gd, ev)
}

// stopGen は本体を止める。graceful なら Stopping を渡してから、そうでなければ（落ちた）すぐ止める。
func (r *runner) stopGen(graceful bool) {
	g := r.cur
	if graceful {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		start := time.Now()
		g.core.Stopping(ctx)
		cancel()
		if d := time.Since(start); d > 1500*time.Millisecond {
			r.fatalf("Stopping が %v かかった（2秒以内のはず）", d)
		}
	}
	g.cancel()
	select {
	case <-g.done:
	case <-time.After(5 * time.Second):
		r.fatalf("Run が戻らない（goroutine が残る）")
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		r.w.mu.Lock()
		r.w.logf("goroutine の一覧:\n%s", buf[:n])
		r.w.mu.Unlock()
		<-g.done
	}
}

// ---- 1秒の進め方 ----

func (r *runner) sendTick() {
	select {
	case r.cur.ticks <- r.clk.Now():
	case <-time.After(5 * time.Second):
		r.fatalf("Tick を受け取らない")
	}
}

// settle は、本体と偽の OBS のやりとりが静まるまで待つ（Tick を流しながら、偽の OBS の動きが続けて止まるのを見る）。
func (r *runner) settle() {
	quiet := 0
	for i := 0; i < 600 && quiet < 3; i++ {
		before := r.w.activity.Load()
		r.sendTick()
		r.sendTick()
		time.Sleep(80 * time.Microsecond)
		if r.w.activity.Load() == before && (r.w.fb == nil || r.w.faulty || r.w.inflight.Load() == 0) {
			quiet++
		} else {
			quiet = 0
		}
	}
}

func (r *runner) second() {
	w := r.w
	now := r.clk.Advance(time.Second)
	w.mu.Lock()
	w.humanWin = false
	w.mu.Unlock()
	w.runLate(now)
	w.releaseHungDue(now)
	w.deliverDue(false)
	if w.fb != nil {
		r.settle() // 配った SceneChanged などで起きた本体の動きを、Sample より前に済ませる（0c の判定が競合しないように）
	}
	// 独立の Judge で、カメラの状態を追う。OK になったら、人が戻した印を消す。
	w.mu.Lock()
	set := ingest.SampleSet{At: now}
	for _, id := range w.cameraIDs() {
		k := w.kbps[id]
		s := ingest.Sample{Camera: id, At: now, Interval: time.Second, Online: k > 0, Protocol: "srt", Kbps: k}
		set.Cameras = append(set.Cameras, s)
		if w.judges[id].Update(s) == ingest.LinkOK && w.mark[id] {
			delete(w.mark, id)
			w.logf("印を消した: %s が OK", id)
		}
	}
	w.mu.Unlock()
	select {
	case r.cur.samples <- set:
	case <-time.After(5 * time.Second):
		r.fatalf("Sample の組を受け取らない")
		return
	}
	r.sendTick()
	r.settle()
	for i := 0; i < 200; i++ {
		w.mu.Lock()
		msg := ""
		if !w.fatalLocked() {
			msg = w.persistentProblemLocked()
		}
		if msg != "" && i == 199 {
			w.violate("%s", msg)
		}
		w.mu.Unlock()
		if msg == "" {
			break
		}
		time.Sleep(time.Millisecond) // 非同期の書き直しを待つ
	}
	r.fbCheck("1秒の終わり")
}

func (w *world) fatalLocked() bool { return len(w.violations) > 0 }

// runLate は、Timeout のあとに遅れて適用される要求を、due が来たら適用する。
func (w *world) runLate(now time.Time) {
	w.mu.Lock()
	var rest []lateApply
	var evs []switcher.Event
	for _, l := range w.late {
		if l.due.After(now) {
			rest = append(rest, l)
			continue
		}
		w.stats.lateApplies++
		if ev, changed := w.applyLocked(l.key, l.val, "本体（遅れて適用）", false); changed && ev != nil {
			w.enqueueLocked(ev, false, ik{}, false)
		}
		_ = evs
	}
	w.late = rest
	w.mu.Unlock()
}

// ---- 入力 ----

func (r *runner) camPattern() {
	id := wCameraIDs[r.rng.Intn(len(wCameraIDs))]
	var k float64
	switch x := r.rng.Intn(100); {
	case x < 45:
		k = goodKbps
	case x < 80:
		k = 0
	default:
		k = 500
	}
	n := 1 + r.rng.Intn(8)
	r.stepf("カメラ %s を %.0f kbps で %d 秒", id, k, n)
	r.w.mu.Lock()
	r.w.kbps[id] = k
	r.w.mu.Unlock()
	for range n {
		r.second()
	}
}

func (r *runner) idle() {
	n := 1 + r.rng.Intn(12)
	r.stepf("そのまま %d 秒", n)
	for range n {
		r.second()
	}
}

func (r *runner) allItems() []ik {
	var ks []ik
	for _, name := range r.w.order {
		for _, it := range r.w.cont[name].items {
			ks = append(ks, ik{name, it.id})
		}
	}
	return ks
}

func (r *runner) humanToggle() {
	w := r.w
	w.mu.Lock()
	ks := r.allItems()
	k := ks[r.rng.Intn(len(ks))]
	if owned := ownedList2(w.appOwned); len(owned) > 0 && r.rng.Intn(2) == 0 {
		k = owned[r.rng.Intn(len(owned))] // 本体が隠した項目を、人が戻す（INV-03）
	}
	it := w.cont[k.c].find(k.id)
	v := !it.enabled
	r.stepfLocked("人が項目 %v を %v に", k, v)
	if v && w.appOwned[k] {
		if cam := w.cameraOfItemLocked(k); cam != "" && !w.canRestoreOracleLocked(cam) {
			w.humanRestoredOwned[k] = true
			w.stats.humanRestores++
		}
	}
	it.enabled = v
	delete(w.appOwned, k)
	delete(w.trueIssued, k)
	ev := switcher.ItemEnabledChanged{At: r.clk.Now(), SceneUUID: k.c, ItemID: k.id, Enabled: v}
	w.enqueueLocked(ev, true, k, v)
	w.fbOnHumanLocked("人の項目の切替", false)
	w.fbMapMutatedLocked()
	w.mu.Unlock()
	if r.rng.Intn(2) == 0 {
		w.deliverDue(true)
	}
	r.settle()
}

// stepfLocked は w.mu を持っている間の stepf。
func (r *runner) stepfLocked(f string, a ...any) {
	line := fmt.Sprintf(f, a...)
	r.steps = append(r.steps, line)
	r.w.logf("入力: %s", line)
}

func (r *runner) humanScene() {
	w := r.w
	w.mu.Lock()
	var names []string
	for _, n := range w.order {
		if !w.cont[n].group {
			names = append(names, n)
		}
	}
	to := names[r.rng.Intn(len(names))]
	r.stepfLocked("人がシーンを %s に切替", to)
	w.program = to
	w.fbOnHumanLocked("人のシーン切替", true)
	w.enqueueLocked(switcher.SceneChanged{At: r.clk.Now(), Scene: w.sceneNameLocked(to), SceneUUID: to}, true, ik{}, false)
	w.mu.Unlock()
	if r.rng.Intn(2) == 0 {
		w.deliverDue(true)
	}
	r.settle()
}

func (r *runner) streamToggle() {
	w := r.w
	w.mu.Lock()
	w.streaming = !w.streaming
	w.fbOnStreamLocked()
	r.stepfLocked("配信を %v に", w.streaming)
	if !w.streaming {
		w.clearMarksLocked("配信の停止")
	}
	w.enqueueLocked(switcher.StreamStateChanged{At: r.clk.Now(), Streaming: w.streaming}, false, ik{}, false)
	w.mu.Unlock()
	r.settle()
}

func (r *runner) studioToggle() {
	w := r.w
	w.mu.Lock()
	w.studio = !w.studio
	r.stepfLocked("Studio Mode を %v に", w.studio)
	w.clearMarksLocked("Studio Mode の変化")
	w.fbOnConnLocked()
	w.enqueueLocked(switcher.StudioModeChanged{At: r.clk.Now(), Enabled: w.studio}, false, ik{}, false)
	w.mu.Unlock()
	r.settle()
}

func (r *runner) disconnect() {
	w := r.w
	w.mu.Lock()
	if !w.connected {
		w.mu.Unlock()
		return
	}
	r.stepfLocked("OBS との接続が切れる")
	w.fbOnDisconnectLocked()
	w.pending = nil
	w.connected = false
	w.humanRestoredOwned = map[ik]bool{}
	w.releaseHungLocked()
	ch, gd := w.evCh, w.genDone
	w.mu.Unlock()
	w.send(ch, gd, switcher.OBSDisconnected{At: r.clk.Now()})
	r.settle()
}

func (r *runner) reconnect() {
	w := r.w
	w.mu.Lock()
	if w.connected {
		w.mu.Unlock()
		return
	}
	r.stepfLocked("OBS につなぎ直す")
	w.fbOnConnLocked()
	w.mu.Unlock()
	r.sendConnected()
	r.settle()
}

func (r *runner) showAll() {
	r.stepf("すべて表示に戻す")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := r.cur.core.Command(ctx, switcher.CmdShowAllCameras)
	var nc switcher.ErrNotConnected
	var is switcher.ErrInvalidState
	switch {
	case err == nil:
		r.w.mu.Lock()
		r.w.stats.showAll++
		r.w.mu.Unlock()
	case asErr(err, &nc), asErr(err, &is):
	default:
		r.fatalf("すべて表示に戻すの結果が想定外: %v", err)
	}
	r.settle()
}

func (r *runner) humanAddRemove() {
	w := r.w
	w.mu.Lock()
	ks := r.allItems()
	k := ks[r.rng.Intn(len(ks))]
	c := w.cont[k.c]
	if r.rng.Intn(2) == 0 && len(c.items) > 1 {
		r.stepfLocked("人が項目 %v を削除", k)
		for i, it := range c.items {
			if it.id == k.id {
				c.items = append(c.items[:i], c.items[i+1:]...)
				break
			}
		}
		delete(w.appOwned, k)
	} else {
		src := []string{"IRL", "SUB", "HAND"}[r.rng.Intn(3)]
		w.nextItem++
		c.items = append(c.items, &wItem{id: w.nextItem, source: src, kind: scenemap.InputSource, enabled: true})
		r.stepfLocked("人が %s に %s の項目 %d を追加", k.c, src, w.nextItem)
	}
	ev := switcher.SceneMapChanged{At: r.clk.Now(), Map: w.snapshotMapLocked()}
	w.enqueueLocked(ev, true, ik{}, false)
	w.fbOnHumanLocked("人の項目の追加・削除", false)
	w.fbMapMutatedLocked()
	w.mu.Unlock()
	if r.rng.Intn(2) == 0 {
		w.deliverDue(true)
	}
	r.settle()
}

func (r *runner) restart() {
	graceful := r.rng.Intn(2) == 0
	autoOn := r.rng.Intn(5) > 0
	studioHide := []string{switcher.StudioHideStandby, switcher.StudioHideIgnore, switcher.StudioHideHide}[r.rng.Intn(3)]
	r.stepf("本体を終えて起動し直す（graceful=%v, auto_enabled=%v, studio_mode_hide=%s）", graceful, autoOn, studioHide)
	w := r.w
	strict := graceful && !w.faulty
	w.mu.Lock()
	connected := w.connected
	if strict {
		w.benign = true
	}
	w.stats.restarts++
	if !graceful {
		w.stats.crashes++
	}
	w.mu.Unlock()
	r.stopGen(graceful)
	w.mu.Lock()
	w.benign = false
	if strict {
		// 通常の動作で正常に終わったときは、つながっていれば隠した項目はすべて戻り、記録は空になる。
		if connected {
			if len(w.appOwned) > 0 {
				w.violate("INV-04: Stopping のあと、本体が隠した項目が戻っていない: %v", ownedList(w.appOwned))
			}
			if es, err := readHiddenFileRaw(r.path); err != nil || len(es) != 0 {
				w.violate("INV-04: Stopping のあとの hidden.json が空でない: %+v %v", es, err)
			}
		}
	}
	w.mu.Unlock()
	r.startGen(autoOn, studioHide)
	r.settle()
}

// ownedList2 は本体が隠した項目を、決まった順で返す（乱数の列が種で決まるように）。
func ownedList2(m map[ik]bool) []ik {
	var out []ik
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].c != out[j].c {
			return out[i].c < out[j].c
		}
		return out[i].id < out[j].id
	})
	return out
}

func ownedList(m map[ik]bool) []string {
	var out []string
	for k := range m {
		out = append(out, fmt.Sprintf("%s/%d", k.c, k.id))
	}
	sort.Strings(out)
	return out
}

// ---- 回復（INV-04） ----

// recover は、入力の列のあとに、安定した回復の区間を足す（接続正常、要求成功、イベントは欠けない、Tick 継続）。
// 戻してよい理由（配信の停止、または全カメラが OK）を1つ作り、本体が隠した項目がすべて戻ることを確かめる。
func (r *runner) recoverAll() {
	w := r.w
	mode := r.rng.Intn(2)
	w.mu.Lock()
	w.benign = true
	if w.faulty {
		// 前提の外の障害（Timeout、イベントの欠け）のあとは、つなぎ直して地図を読み直させる。
		r.stepfLocked("回復: つなぎ直しで世界の状態を読み直させる")
	}
	w.mu.Unlock()
	if w.faulty {
		r.disconnect()
	}
	r.reconnect()
	w.mu.Lock()
	switch mode {
	case 0:
		w.streaming = false
		w.fbOnStreamLocked()
		w.clearMarksLocked("回復: 配信の停止")
		r.stepfLocked("回復: 配信を止める")
		w.enqueueLocked(switcher.StreamStateChanged{At: r.clk.Now(), Streaming: false}, false, ik{}, false)
	default:
		r.stepfLocked("回復: 全カメラを良い回線にする")
		for _, id := range w.cameraIDs() {
			w.kbps[id] = goodKbps
		}
	}
	w.mu.Unlock()
	const limit = 60 // 上限は 5秒×3 + 2秒 + 全対象のバッチ処理（偽の OBS は即答）+ Tick 間隔1秒 に余裕を足した秒数
	for i := 0; i < limit; i++ {
		r.second()
		w.mu.Lock()
		done := len(w.appOwned) == 0 && len(w.late) == 0 && len(w.pending) == 0
		w.mu.Unlock()
		if done {
			break
		}
	}
	w.mu.Lock()
	stuck := w.faulty && len(w.appOwned) > 0
	w.mu.Unlock()
	if stuck {
		// 前提の外の障害（Timeout のあとの遅れた適用など）で記録のない非表示が残った場合は、既知の制約。
		// 逃げ道の「すべて表示に戻す」で、1回で戻せる。
		r.stepf("回復: 記録のない非表示が残ったので、すべて表示に戻すを押す（既知の制約の逃げ道）")
		r.showAll()
		for i := 0; i < 20; i++ {
			r.second()
			w.mu.Lock()
			done := len(w.appOwned) == 0 && len(w.late) == 0
			w.mu.Unlock()
			if done {
				break
			}
		}
	}
	w.mu.Lock()
	if len(w.appOwned) > 0 || len(w.late) > 0 {
		w.violate("INV-04: 回復の区間（%d 秒、mode=%d）のあとも、本体が隠した項目が戻っていない: %v（遅れて適用される要求 %d 件）",
			limit, mode, ownedList(w.appOwned), len(w.late))
	}
	w.mu.Unlock()
	// 戻したあとの hidden.json は空になる（通常の動作）。隠しかけの掃除は決着から10秒後なので、30秒まで進める。
	if !w.faulty {
		for i := 0; ; i++ {
			es, err := readHiddenFileRaw(r.path)
			if err == nil && len(es) == 0 {
				break
			}
			if i >= 30 {
				w.mu.Lock()
				w.violate("INV-04: 回復から30秒たっても hidden.json が空でない: %+v %v", es, err)
				w.mu.Unlock()
				break
			}
			r.second()
			time.Sleep(time.Millisecond) // persister が書くのを待つ
		}
	}
}

// ---- 1つの種の実行 ----

func runSeed(t *testing.T, seed int64, faulty bool, steps int) {
	path, hw := trackedHiddenPath(t)
	clk := newClock()
	w := newWorld(seed, clk, path, faulty)
	r := &runner{t: t, w: w, clk: clk, rng: rand.New(rand.NewSource(seed ^ 0x5eed)), path: path, hw: hw}
	defer w.close()
	r.startGen(true, switcher.StudioHideStandby)
	r.settle()
	for i := 0; i < steps && len(w.violations) == 0; i++ {
		switch x := r.rng.Intn(100); {
		case x < 30:
			r.camPattern()
		case x < 42:
			r.idle()
		case x < 54:
			r.humanToggle()
		case x < 61:
			r.humanScene()
		case x < 67:
			r.streamToggle()
		case x < 74:
			if r.rng.Intn(2) == 0 {
				r.disconnect()
			} else {
				r.reconnect()
			}
		case x < 79:
			r.studioToggle()
		case x < 85:
			r.showAll()
		case x < 90:
			r.humanAddRemove()
		case x < 92:
			r.restart()
		default:
			w.deliverDue(true)
			r.settle()
		}
	}
	if len(w.violations) == 0 {
		r.recoverAll()
	}
	if len(w.violations) == 0 {
		w.mu.Lock()
		w.benign = true
		w.mu.Unlock()
		r.stopGen(true)
	} else {
		r.stopGen(false)
	}
	if len(w.violations) > 0 {
		profile := "normal"
		if faulty {
			profile = "fault"
		}
		t.Errorf("seed=%d profile=%s: %s\n--- 入力の列 ---\n%s\n--- 世界の記録（末尾） ---\n%s",
			seed, profile, strings.Join(w.violations, "\n"), strings.Join(r.steps, "\n"), joinTrace(w.trace, 150))
	}
	w.mu.Lock()
	st := w.stats
	w.mu.Unlock()
	t.Logf("seed=%d faulty=%v: false=%d true=%d timeout=%d late=%d restarts=%d(crash %d) showAll=%d humanRestores=%d marks=%d approvedBeforeMark=%d",
		seed, faulty, st.appFalse, st.appTrue, st.timeouts, st.lateApplies, st.restarts, st.crashes, st.showAll, st.humanRestores, st.marks, st.approvedBeforeMark)
}

func seedCount(short, normal int) int {
	if v := os.Getenv("IRL_INTEG_SEEDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if testing.Short() {
		return short
	}
	return normal
}

func TestIntegrationHiddenInvariants(t *testing.T) {
	base := runtime.NumGoroutine()
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for runtime.NumGoroutine() > base+2 && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if n := runtime.NumGoroutine(); n > base+2 {
			var sb strings.Builder
			_ = pprof.Lookup("goroutine").WriteTo(&sb, 1)
			t.Errorf("goroutine が残っている: %d（開始時 %d）\n%s", n, base, sb.String())
		}
	})
	steps := 90
	// 同時に走らせる数を絞る（負荷が高いと、本体と偽の OBS が静まったかの見極めが外れやすくなる）。
	sem := make(chan struct{}, 3)
	run := func(name string, faulty bool, n int, seedBase int64) {
		for i := 0; i < n; i++ {
			seed := seedBase + int64(i)
			t.Run(fmt.Sprintf("%s/seed%d", name, seed), func(t *testing.T) {
				t.Parallel()
				sem <- struct{}{}
				defer func() { <-sem }()
				runSeed(t, seed, faulty, steps)
			})
		}
	}
	run("normal", false, seedCount(12, 150), 1000)
	run("fault", true, seedCount(6, 60), 5000)
}

func asErr[T error](err error, target *T) bool { return errors.As(err, target) }

// 固定の列: 隠した項目だけが映る戻り先（Hand は手元カメラだけ）へ、戻り先の項目を表示に戻す前に切り替えない
// （waitingHidden）。手元カメラが悪い間に Hand にいる → 待機へ送り、項目を隠す → 手元が OK → 項目を戻してから Hand へ戻る。
// 「戻り先が待っている隠した項目」の待ちを外すと、AutoRecover が先に Hand へ切り替えて空の画面が映り、落ちる。
func TestIntegrationWaitingHiddenFixedSequence(t *testing.T) {
	path, hw := trackedHiddenPath(t)
	clk := newClock()
	w := newWorld(1, clk, path, false)
	w.program = "Hand"
	w.benign = true
	r := &runner{t: t, w: w, clk: clk, rng: rand.New(rand.NewSource(1)), path: path, hw: hw}
	defer w.close()
	r.startGen(true, switcher.StudioHideStandby)
	r.settle()
	w.mu.Lock()
	w.kbps[wMain], w.kbps[wSub], w.kbps[wHand] = goodKbps, goodKbps, 0
	w.mu.Unlock()
	for range 12 {
		r.second()
	}
	w.mu.Lock()
	hidden := len(w.appOwned)
	w.mu.Unlock()
	if hidden == 0 {
		t.Fatal("手元カメラの項目が隠れていない（前提の列が成り立たない）")
	}
	w.mu.Lock()
	w.kbps[wHand] = goodKbps
	w.mu.Unlock()
	for range 20 {
		r.second()
	}
	w.mu.Lock()
	prog, owned, viol := w.program, len(w.appOwned), append([]string(nil), w.violations...)
	w.mu.Unlock()
	r.stopGen(true)
	if len(viol) > 0 {
		t.Fatalf("%s\n%s", strings.Join(viol, "\n"), joinTrace(w.trace, 60))
	}
	if prog != "Hand" || owned != 0 {
		t.Errorf("戻っていない: program=%s 隠した項目=%d\n%s", prog, owned, joinTrace(w.trace, 40))
	}
}
