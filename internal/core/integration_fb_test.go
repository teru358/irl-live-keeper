package core

import (
	"context"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"math/rand"
	"regexp"
	"runtime"
	"runtime/pprof"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 結合の試験（0c）: 偽の OBS の世界に scene_fallbacks と複数段の逃がし先を持たせ、乱数の入力の列を流して、
// 0b の HB-INV-01〜04（既存の世界の判定）に、表 0c の FB-INV（6.2）と安定区間の到達性（6.3）を足して確かめる。
// 判定は integration_fb_world_test.go の独立の計算（Machine の探索・CanEnter・健康の計算は使わない）と、
// core.Snapshot()（status と同じ公開の値）だけ。落ちたら、種、入力の列、判定に使った値、世界の記録の末尾を出す。
//
// 設定の変更は、本体（Core）に実行中の設定の経路がまだないため、ScenesConfigured を OBS のイベントの経路から流す
// （OBS のイベントの列とは独立に、順序を追い越して届くので、保存画面からの変更に近い）。

// ---- 設定の乱数 ----

func fbRandomRows(rng *rand.Rand) []switcher.FallbackRow {
	pool := slices.Clone(fbPool)
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	var rows []switcher.FallbackRow
	for i, sc := range pool {
		if rng.Intn(100) >= 70 {
			continue
		}
		var to string
		switch x := rng.Intn(100); {
		case x < 7:
			to = "OBSにない名前" // fallback の名前が OBS にない
		case x < 22 || i+1 >= len(pool):
			to = standby
		case x < 70:
			to = pool[i+1]
		default:
			to = pool[i+1+rng.Intn(len(pool)-i-1)]
		}
		rows = append(rows, switcher.FallbackRow{Scene: sc, Fallback: to})
	}
	rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	return rows
}

// fbSpecRows は spec 第3節の例: シーン1〔メインとサブ〕→ シーン2〔サブ〕→ 待機。画像のシーン Talk を終端にした列も足す。
func fbSpecRows() []switcher.FallbackRow {
	return []switcher.FallbackRow{{Scene: camera, Fallback: fbSubOnly}, {Scene: "Hand", Fallback: talk}, {Scene: "Nest", Fallback: camera}, {Scene: fbDual, Fallback: camera}, {Scene: fbAltOnly, Fallback: talk}}
}

func fbCameraConfigs() []switcher.CameraConfig {
	return []switcher.CameraConfig{
		{ID: wMain, OnBad: scenemap.MotionStandby}, {ID: wSub, OnBad: scenemap.MotionHide}, {ID: wHand, OnBad: scenemap.MotionHide}, {ID: wAlt, OnBad: scenemap.MotionStandby},
	}
}

// ---- 入力 ----

func (r *runner) fbSnap() Snapshot { return r.cur.core.Snapshot() }

// injectConfig は ScenesConfigured を本体へ流す（OBS のイベントの列を追い越して届く）。
func (r *runner) injectConfig() {
	w := r.w
	w.mu.Lock()
	ev := switcher.ScenesConfigured{At: r.clk.Now(), Standby: standby, Cameras: fbCameraConfigs(), AutoEnabled: w.autoOn,
		StudioModeHide: w.studioHid, SceneFallbacks: slices.Clone(w.fb.rows)}
	ch, gd := w.evCh, w.genDone
	w.fbOnConfigLocked()
	w.mu.Unlock()
	w.send(ch, gd, ev)
}

func (w *world) fbQuietLocked() bool {
	return w.inflight.Load() == 0 && w.connected && len(w.pending) == 0 && len(w.late) == 0 && len(w.hung) == 0
}

func stackEq(a, b []string) bool { return slices.Equal(a, b) }

func (r *runner) fbReconfig() {
	w := r.w
	rows := fbRandomRows(r.rng)
	if r.rng.Intn(4) == 0 { // 並べ替えだけ
		w.mu.Lock()
		rows = slices.Clone(w.fb.rows)
		w.mu.Unlock()
		r.rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	}
	synced := r.fbSync() // 前の本体の処理が済んでから「前」の status を読む
	w.mu.Lock()
	quiet := synced && w.fbQuietLocked()
	var before Snapshot
	app0 := w.fb.appSwitches
	if quiet {
		before = r.fbSnap()
	}
	w.fb.rows = rows
	w.fb.count("設定の変更")
	r.stepfLocked("scene_fallbacks を変更: %v", rows)
	w.mu.Unlock()
	r.injectConfig()
	r.settle()
	if !quiet {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	after := r.fbSnap()
	// FB-INV-06: 対応が変わっても、到達できないことだけでは履歴を失わない。
	if w.fb.appSwitches == app0 && w.fbQuietLocked() && (!stackEq(before.OBS.Standby.Stack, after.OBS.Standby.Stack) || before.OBS.Standby.Owner != after.OBS.Standby.Owner) {
		w.violate("FB-INV-06: scene_fallbacks の変更だけで履歴が変わった: owner %s→%s stack %v→%v", before.OBS.Standby.Owner, after.OBS.Standby.Owner,
			before.OBS.Standby.Stack, after.OBS.Standby.Stack)
	}
}

func (r *runner) fbAutoToggle() {
	w := r.w
	w.mu.Lock()
	w.autoOn = !w.autoOn
	r.stepfLocked("auto_enabled を %v に", w.autoOn)
	if !w.autoOn {
		w.fb.epBottom = ""
		w.fb.epValid = !w.hasPendingAppSceneLocked()
	}
	w.mu.Unlock()
	r.injectConfig()
	r.settle()
}

func (r *runner) fbSetAuto(on bool) {
	w := r.w
	w.mu.Lock()
	cur := w.autoOn
	w.mu.Unlock()
	if cur != on {
		r.fbAutoToggle()
	}
}

func (r *runner) fbHumanScene() {
	w := r.w
	w.mu.Lock()
	us := w.sceneUUIDsLocked()
	to := us[r.rng.Intn(len(us))]
	if r.rng.Intn(100) < 60 { // 監視対象のシーン（逃がす列の元になる）を多めに選ぶ
		var mon []string
		for _, u := range us {
			if w.monitoredLocked(u) {
				mon = append(mon, u)
			}
		}
		if len(mon) > 0 {
			to = mon[r.rng.Intn(len(mon))]
		}
	}
	if r.rng.Intn(100) < 20 { // 入力 IRL を共有する Dual と Camera の組（共有入力の再起動の除外）を多めにする
		if u := w.byNameLocked(fbDual); u != "" {
			to = u
		}
	}
	if r.rng.Intn(3) == 0 { // 積んだスタックの中のシーン、今のシーンの逃がし先、を選ぶ
		snap := r.fbSnap()
		names := slices.Clone(snap.OBS.Standby.Stack)
		for _, u := range w.chainLocked(w.program) {
			names = append(names, w.sceneNameLocked(u))
		}
		if len(names) > 0 {
			if u := w.byNameLocked(names[r.rng.Intn(len(names))]); u != "" {
				to = u
			}
		}
	}
	r.stepfLocked("人がシーンを %s に切替", w.sceneNameLocked(to))
	w.program = to
	w.fbOnHumanLocked("人のシーン切替", true)
	w.enqueueLocked(switcher.SceneChanged{At: r.clk.Now(), Scene: w.sceneNameLocked(to), SceneUUID: to}, true, ik{}, false)
	w.mu.Unlock()
	if r.rng.Intn(2) == 0 {
		w.deliverDue(true)
	}
	r.settle()
}

func (r *runner) fbRename() {
	w := r.w
	synced := r.fbSync()
	w.mu.Lock()
	var us []string
	for _, u := range w.sceneUUIDsLocked() {
		if u != standby {
			us = append(us, u)
		}
	}
	if len(us) == 0 {
		w.mu.Unlock()
		return
	}
	u := us[r.rng.Intn(len(us))]
	old := w.sceneNameLocked(u)
	base := old
	if i := strings.Index(base, "_r"); i > 0 {
		base = base[:i]
	}
	w.fb.seq++
	nn := fmt.Sprintf("%s_r%d", base, w.fb.seq)
	quiet := synced && w.fbQuietLocked()
	var before Snapshot
	app0 := w.fb.appSwitches
	if quiet {
		before = r.fbSnap()
	}
	w.cont[u].name = nn
	w.fb.count("改名")
	r.stepfLocked("シーン %s を %s に改名（UUID %s は同じ）", old, nn, u)
	w.enqueueLocked(switcher.SceneMapChanged{At: r.clk.Now(), Map: w.snapshotMapLocked()}, true, ik{}, false)
	w.fbOnHumanLocked("改名", false)
	w.fbMapMutatedLocked()
	w.mu.Unlock()
	if quiet {
		w.deliverDue(true)
	}
	r.settle()
	if !quiet {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	after := r.fbSnap()
	want := make([]string, len(before.OBS.Standby.Stack))
	for i, n := range before.OBS.Standby.Stack {
		want[i] = n
		if n == old {
			want[i] = nn
		}
	}
	// FB-INV-06: 改名は UUID の履歴を保つ。
	if w.fb.appSwitches == app0 && w.fbQuietLocked() && (!stackEq(want, after.OBS.Standby.Stack) || before.OBS.Standby.Owner != after.OBS.Standby.Owner) {
		w.violate("FB-INV-06: 改名（%s→%s）で履歴が変わった: owner %s→%s stack %v→%v（期待 %v）", old, nn, before.OBS.Standby.Owner, after.OBS.Standby.Owner,
			before.OBS.Standby.Stack, after.OBS.Standby.Stack, want)
	}
}

func (r *runner) fbDelete() {
	w := r.w
	w.mu.Lock()
	var us []string
	for _, u := range w.sceneUUIDsLocked() {
		if u != standby {
			us = append(us, u)
		}
	}
	if len(us) <= 3 {
		w.mu.Unlock()
		return
	}
	u := us[r.rng.Intn(len(us))]
	c := w.cont[u]
	name := c.name
	tmpl := &wCont{name: name}
	for _, it := range c.items {
		tmpl.items = append(tmpl.items, &wItem{id: it.id, source: it.source, kind: it.kind, enabled: true})
	}
	w.fb.deleted[name] = tmpl
	delete(w.cont, u)
	w.order = slices.DeleteFunc(w.order, func(x string) bool { return x == u })
	for _, oc := range w.cont { // 消したシーンを指す項目も消える
		oc.items = slices.DeleteFunc(oc.items, func(it *wItem) bool { return it.kind == scenemap.SceneSource && it.source == u })
	}
	for k := range w.appOwned {
		if k.c == u {
			delete(w.appOwned, k)
		}
	}
	w.fb.count("削除")
	r.stepfLocked("シーン %s（UUID %s）を削除", name, u)
	w.enqueueLocked(switcher.SceneMapChanged{At: r.clk.Now(), Map: w.snapshotMapLocked()}, true, ik{}, false)
	if w.program == u { // OBS が別のシーンへ移す（外部のシーン切替）
		w.program = standby
		w.fbOnHumanLocked("削除で program が移る", true)
		w.enqueueLocked(switcher.SceneChanged{At: r.clk.Now(), Scene: standby, SceneUUID: standby}, true, ik{}, false)
	}
	w.fbOnHumanLocked("削除", false)
	w.fbMapMutatedLocked()
	w.mu.Unlock()
	if r.rng.Intn(2) == 0 {
		w.deliverDue(true)
	}
	r.settle()
}

func (r *runner) fbRevive() {
	w := r.w
	w.mu.Lock()
	var names []string
	for n := range w.fb.deleted {
		names = append(names, n)
	}
	if len(names) == 0 {
		w.mu.Unlock()
		return
	}
	sort.Strings(names)
	name := names[r.rng.Intn(len(names))]
	tmpl := w.fb.deleted[name]
	delete(w.fb.deleted, name)
	if w.byNameLocked(name) != "" {
		w.mu.Unlock()
		return
	}
	w.fb.seq++
	u := fmt.Sprintf("%s#%d", name, w.fb.seq)
	c := &wCont{name: name}
	for _, it := range tmpl.items {
		c.items = append(c.items, &wItem{id: it.id, source: it.source, kind: it.kind, enabled: true})
	}
	w.cont[u] = c
	w.order = append(w.order, u)
	w.fb.count("復活")
	r.stepfLocked("シーン %s を作り直す（新しい UUID %s）", name, u)
	w.enqueueLocked(switcher.SceneMapChanged{At: r.clk.Now(), Map: w.snapshotMapLocked()}, true, ik{}, false)
	w.fbOnHumanLocked("復活", false)
	w.fbMapMutatedLocked()
	w.mu.Unlock()
	if r.rng.Intn(2) == 0 {
		w.deliverDue(true)
	}
	r.settle()
}

// fbCamSingle は1台のカメラの状態を変える（Judge が状態を変える長さ〔3〜5秒〕を超えて保つことが多い）。
func (r *runner) fbCamSingle() {
	ids := r.w.cameraIDs()
	id := ids[r.rng.Intn(len(ids))]
	var k float64
	switch x := r.rng.Intn(100); {
	case x < 45:
		k = goodKbps
	case x < 85:
		k = 0
	default:
		k = 500
	}
	n := 3 + r.rng.Intn(8)
	r.stepf("カメラ %s を %.0f kbps で %d 秒", id, k, n)
	r.w.mu.Lock()
	r.w.kbps[id] = k
	r.w.mu.Unlock()
	for range n {
		r.second()
	}
}

// fbCamMulti は複数のカメラの状態を同時に変える（同時の悪化・回復）。
func (r *runner) fbCamMulti() {
	w := r.w
	n := 2 + r.rng.Intn(3)
	ids := slices.Clone(w.cameraIDs())
	r.rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	w.mu.Lock()
	var parts []string
	for _, id := range ids[:n] {
		var k float64
		switch x := r.rng.Intn(100); {
		case x < 50:
			k = goodKbps
		case x < 85:
			k = 0
		default:
			k = 500
		}
		w.kbps[id] = k
		parts = append(parts, fmt.Sprintf("%s=%.0f", id, k))
	}
	r.stepfLocked("カメラを同時に変える: %s", strings.Join(parts, " "))
	w.mu.Unlock()
	for range 3 + r.rng.Intn(8) {
		r.second()
	}
}

// fbCommand は手動の待機の ON・OFF。ON の特例（auto の待機、戻る切替の期待中、R の承認後など）は、
// 乱数の列の中で、遅れて届くシーンの切替の最中に押されることで起きる（fbOnDuringReturn が狙って作る）。
func (r *runner) fbCommand() {
	kind := switcher.CmdStandbyOff
	onPct := 40
	if r.fbSnap().OBS.Standby.Owner == "manual" {
		onPct = 10
	}
	if r.rng.Intn(100) < onPct {
		kind = switcher.CmdStandbyOn
	}
	r.fbCommandKind(kind)
}

func (r *runner) fbCommandKind(kind switcher.CommandKind) (pre Snapshot, err error) {
	w := r.w
	r.fbSync()
	pre = r.fbSnap()
	w.mu.Lock()
	cfg0 := w.fb.cfgGen
	pendApp := w.hasPendingAppSceneLocked()
	w.mu.Unlock()
	r.stepf("コマンド %v（直前: owner=%s scene=%s stack=%v 遅れて届く切替=%v）", kind, pre.OBS.Standby.Owner, pre.OBS.Scene, pre.OBS.Standby.Stack, pendApp)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = r.cur.core.Command(ctx, kind)
	var nc switcher.ErrNotConnected
	var is switcher.ErrInvalidState
	switch {
	case err == nil:
	case asErr(err, &nc), asErr(err, &is):
	default:
		r.fatalf("コマンドの結果が想定外: %v", err)
	}
	w.mu.Lock()
	w.fb.cmdCount++
	if err == nil {
		switch {
		case kind == switcher.CmdStandbyOn && pre.OBS.Standby.Owner == "auto" && pre.OBS.Scene == standby && pendApp:
			w.fb.count("ON特例(待機のauto・切替の期待中)")
		case kind == switcher.CmdStandbyOn && pre.OBS.Standby.Owner == "auto" && pre.OBS.Scene == standby:
			w.fb.count("ON特例(待機のauto)")
		case kind == switcher.CmdStandbyOn && pre.OBS.Standby.Owner == "auto":
			w.fb.count("ON(個別の退避先のauto)")
		case kind == switcher.CmdStandbyOn:
			w.fb.count("ON(none)")
		case pre.OBS.Standby.Owner == "manual":
			w.fb.count("OFF(manual)")
		}
	}
	w.mu.Unlock()
	r.settle()
	if kind == switcher.CmdStandbyOff && err == nil && pre.OBS.Standby.Owner == "manual" && pre.OBS.Standby.ReturnScene != nil {
		r.fbCheckOff(pre, cfg0)
	}
	return pre, err
}

// fbOnDuringReturn は、全カメラを良くして戻る切替が飛ぶのを待ち、そのイベントが遅れて届く間に ON を押す。
// 間に設定の変更（R の承認 → 設定の変更 → ON → 古い R の遅着）を挟むこともある。
// 待機の auto の ON は期待中でも切替なしで manual になり、あとから届く戻りの切替は手動の待機を解く（持ち主 none）。
func (r *runner) fbOnDuringReturn() {
	w := r.w
	w.mu.Lock()
	vec := map[string]float64{}
	for _, id := range w.cameraIDs() {
		vec[id] = goodKbps
		if r.rng.Intn(5) == 0 {
			vec[id] = 0
		}
		w.kbps[id] = vec[id]
	}
	r.stepfLocked("戻る切替の期待中の ON を狙う: カメラを良くする %v", vec)
	w.mu.Unlock()
	for i := 0; i < 14; i++ {
		r.second()
		w.mu.Lock()
		var dest string
		for _, p := range w.pending {
			if sc, ok := p.ev.(switcher.SceneChanged); ok && p.appScene {
				dest = sc.SceneUUID
			}
		}
		w.mu.Unlock()
		if dest == "" {
			continue
		}
		reconf := r.rng.Intn(2) == 0
		if reconf {
			r.fbReconfig()
		}
		pre, err := r.fbCommandKind(switcher.CmdStandbyOn)
		w.deliverDue(true)
		r.settle()
		w.mu.Lock()
		defer w.mu.Unlock()
		snap := r.fbSnap()
		special := err == nil && pre.OBS.Standby.Owner == "auto" && pre.OBS.Scene == standby && dest != standby
		if !special || !w.fbQuietLocked() || len(w.violations) > 0 || snap.OBS.Scene != w.sceneNameLocked(dest) {
			return
		}
		w.fb.count("ON特例の後に、古い戻りの切替が届いた")
		if snap.OBS.Standby.Owner == "manual" {
			w.violate("ON 特例: 待機 ON の前に送った戻りの切替が %s へ届いたのに、手動の待機（manual）のまま残っている", w.sceneNameLocked(dest))
		}
		if !reconf && snap.LastError != nil && strings.Contains(i18n.Render(i18n.JA, snap.LastError.Message), "もう一度 ON を押してください") {
			w.fb.count("ON特例の後の遅着: last_error の通知")
		}
		return
	}
}

// fbCheckOff は FB-INV-07: manual の戻り先は単独で、OFF が ON 前の中間の履歴を復元しない。
// OFF のあとのスタックは、none（空）か、[戻り先, …戻り先の逃がし先の列の飛ばした監視対象] だけ。
func (r *runner) fbCheckOff(pre Snapshot, cfg0 int) {
	w := r.w
	w.mu.Lock()
	defer w.mu.Unlock()
	ret := *pre.OBS.Standby.ReturnScene
	u := w.byNameLocked(ret)
	if len(w.violations) > 0 || w.fb.cfgGen != cfg0 || u == "" || !w.fbQuietLocked() {
		return
	}
	post := r.fbSnap()
	if post.OBS.Standby.Owner != "auto" {
		return
	}
	st := post.OBS.Standby.Stack
	allowed := map[string]bool{ret: true}
	for _, c := range w.chainLocked(u) {
		allowed[w.sceneNameLocked(c)] = true
	}
	if len(st) == 0 || st[0] != ret {
		w.violate("FB-INV-07: OFF のあとの auto のスタックの底が、manual の戻り先 %s でない: %v", ret, st)
		return
	}
	for _, n := range st {
		if !allowed[n] {
			w.violate("FB-INV-07: OFF のあとのスタック %v に、戻り先 %s の逃がし先の列にない %s がある（ON 前の中間の履歴の復元）", st, ret, n)
		}
	}
}

// ---- 判定（status と世界の事実） ----

var fbWarnRe = regexp.MustCompile(`^scene_fallbacks の (\d+) 行目: (scene|fallback)「(.*)」が`)

// fbSync は、実行中の OBS の要求が終わるのを待ち（上限あり）、core がそれまでに受けたイベントの処理と publish を
// 終えたことを Tick の受理で確かめる（Tick は core の1回のループが終わってからしか受け取られない）。
func (r *runner) fbSync() bool {
	deadline := time.Now().Add(5 * time.Second)
	for r.w.inflight.Load() > 0 {
		if time.Now().After(deadline) {
			return false // 要求が終わらない（hung など）。静かな時点の判定は控える
		}
		time.Sleep(100 * time.Microsecond)
	}
	r.sendTick()
	return r.w.inflight.Load() == 0
}

func (r *runner) fbCheck(where string) {
	w := r.w
	if w.fb == nil || r.cur == nil {
		return
	}
	synced := r.fbSync()
	// send は非バッファチャネルへの受け渡しで戻るが、handle と publish は
	// まだ終わっていないことがある。送信を止めて Tick を受理させれば、
	// それ以前に渡した全イベントの publish が終わる（core.loop）。
	// 比較が終わるまで保持し、新しいイベントがこの障壁を追い越すのも防ぐ。
	w.sendMu.Lock()
	defer w.sendMu.Unlock()
	r.sendTick()
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.violations) > 0 {
		return
	}
	f := w.fb
	quiet0, seq0 := synced && w.fbQuietLocked(), w.reqSeq.Load()
	snap := r.fbSnap()
	st := snap.OBS.Standby
	owner := st.Owner
	vf := func(format string, a ...any) {
		w.violate("%s: %s（status: owner=%s scene=%s stack=%v return=%v streaming=%v connected=%v）", where, fmt.Sprintf(format, a...), owner, snap.OBS.Scene,
			st.Stack, st.ReturnScene, snap.OBS.Streaming, snap.OBS.Connected)
	}
	// FB-INV-01: 重複なし。現在のシーンと待機を含まない。
	seen := map[string]bool{}
	for _, n := range st.Stack {
		if seen[n] {
			vf("FB-INV-01: スタックに同じシーン %s が2回ある", n)
			return
		}
		seen[n] = true
	}
	if snap.OBS.Connected {
		if seen[snap.OBS.Scene] || seen[standby] {
			vf("FB-INV-01: スタックに今のシーンか待機シーンがある")
			return
		}
		// FB-INV-02: manual / none ではスタックは空。配信していない間は持ち主 none でスタックは空。
		switch owner {
		case "manual":
			if len(st.Stack) != 0 || st.ReturnScene == nil {
				vf("FB-INV-02: manual なのにスタックが空でない、または戻り先がない")
				return
			}
		case "none":
			if len(st.Stack) != 0 || st.ReturnScene != nil {
				vf("FB-INV-02: none なのにスタックか戻り先がある")
				return
			}
		case "auto":
			if len(st.Stack) == 0 || st.ReturnScene == nil || *st.ReturnScene != st.Stack[0] {
				vf("FB-INV-03: auto なのにスタックが空か、return_scene がスタックの底でない")
				return
			}
			if !w.autoOn {
				vf("auto_enabled=false なのに持ち主が auto")
				return
			}
		}
		if !snap.OBS.Streaming && (owner != "none" || len(st.Stack) != 0) {
			vf("FB-INV-02: 配信していないのに持ち主・スタックが残っている")
			return
		}
	}
	if len(st.Stack) >= 2 {
		f.count("深さ2以上")
	} else if len(st.Stack) == 1 && snap.OBS.Scene != standby {
		f.count("個別の退避先にいる")
	}
	if owner == "manual" {
		f.count("manual")
	}
	// 履歴は期待どおりのイベント・コマンドのあとにだけ変わる（FB-INV-04 / 05）。
	if !synced {
		return // 要求が終わらない間は、履歴の変化とイベントの数の対応を見ない（prev も更新しない）
	}
	cur := fbPrev{valid: true, owner: owner, stack: slices.Clone(st.Stack), ev: w.evCount.Load(), cmd: f.cmdCount, app: f.appSwitches}
	if f.prev.valid && !stackEq(f.prev.stack, cur.stack) {
		if cur.ev == f.prev.ev && cur.cmd == f.prev.cmd {
			vf("FB-INV-04: イベントもコマンドもないのにスタックが変わった: %v → %v", f.prev.stack, cur.stack)
			return
		}
		if len(cur.stack) > len(f.prev.stack) {
			// 増えるのは、本体の SetScene の適用（遅れて届く SceneChanged）か Command の OFF だけ。増えた回数が、適用の数を超えない。
			if f.appSwitches-f.consumed > 0 {
				f.consumed++
				f.count("履歴が増えた到着")
			} else if cur.cmd == f.prev.cmd {
				vf("FB-INV-05: 本体の SetScene の適用（未消化 %d）も Command もないのにスタックが増えた: %v → %v", f.appSwitches-f.consumed, f.prev.stack, cur.stack)
				return
			}
		}
	}
	if f.prev.valid && len(cur.stack) < len(f.prev.stack) && cur.app != f.prev.app {
		f.count("履歴が減った到着")
	}
	f.prev = cur
	// 静かな時点: 前後で OBS の要求が1つも始まっていない（status を読む間に世界が変わっていない）。
	if !(quiet0 && w.reqSeq.Load() == seq0 && w.fbQuietLocked() && snap.OBS.Connected) {
		return
	}
	// ここから静かな時点（流れていないイベントなし、本体と世界が静まった）。
	if snap.OBS.Scene != w.sceneNameLocked(w.program) || snap.OBS.Streaming != w.streaming {
		vf("status が偽の OBS と違う: 世界の program=%s streaming=%v（inflight=%d reqSeq=%d→%d evCount=%d pending=%d）", w.sceneNameLocked(w.program), w.streaming, w.inflight.Load(), seq0, w.reqSeq.Load(), w.evCount.Load(), len(w.pending))
		return
	}
	// FB-INV-03: スタックの全要素は存在し、監視対象。
	for _, n := range st.Stack {
		u := w.byNameLocked(n)
		if u == "" || !w.monitoredLocked(u) {
			vf("FB-INV-03: スタックの %s が OBS にないか、監視対象でない", n)
			return
		}
	}
	// status の警告: scene_fallbacks の名前が OBS にない行と、OBS の警告が同時にあっても、どちらも出る。
	wantWarn, wantIdx := map[string]bool{}, map[string]bool{}
	for i, row := range f.rows {
		if w.byNameLocked(row.Scene) == "" {
			wantWarn["scene/"+row.Scene] = true
			wantIdx[fmt.Sprintf("%d/scene/%s", i+1, row.Scene)] = true
		}
		if w.byNameLocked(row.Fallback) == "" {
			wantWarn["fallback/"+row.Fallback] = true
			wantIdx[fmt.Sprintf("%d/fallback/%s", i+1, row.Fallback)] = true
		}
	}
	gotWarn, gotIdx, obsWarn := map[string]bool{}, map[string]bool{}, false
	for _, m := range i18n.Strings(i18n.JA, snap.OBS.Warnings) {
		if m == fbObsWarning {
			obsWarn = true
		}
		if g := fbWarnRe.FindStringSubmatch(m); g != nil {
			gotWarn[g[2]+"/"+g[3]] = true
			gotIdx[g[1]+"/"+g[2]+"/"+g[3]] = true
		}
	}
	if !obsWarn {
		vf("OBS の既存の警告が status から消えている: %v", i18n.Strings(i18n.JA, snap.OBS.Warnings))
		return
	}
	if !maps_equal(wantWarn, gotWarn) {
		vf("scene_fallbacks の警告が違う: 期待 %v、実際 %v", sortedMapKeys(wantWarn), sortedMapKeys(gotWarn))
		return
	}
	if len(wantWarn) > 0 {
		f.count("名前がないfallbackの警告とOBSの警告の同時")
		// 行番号も照合する（並べ替えだけの変更でも、新しい設定の並びになる）。
		if !maps_equal(wantIdx, gotIdx) {
			vf("scene_fallbacks の警告の行番号が設定の並びと違う: 期待 %v、実際 %v", sortedMapKeys(wantIdx), sortedMapKeys(gotIdx))
			return
		}
	}
	// FB-INV-10: スタックの底は、最後の共通解除のあと、本体が最初に離れた監視対象（独立に計算）。
	switch owner {
	case "none":
		f.epBottom, f.epValid = "", true
	case "auto":
		if f.epValid && f.epBottom != "" && w.monitoredLocked(f.epBottom) {
			if want := w.sceneNameLocked(f.epBottom); st.Stack[0] != want {
				vf("FB-INV-10: スタックの底が %s のはずが %s（本体が最初に離れた監視対象）", want, st.Stack[0])
				return
			}
			f.count("底の照合(FB-INV-10)")
		}
	}
}

func maps_equal(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func sortedMapKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- 安定区間の到達性（6.3） ----

// fbRepair は、人が項目を戻し、消したシーンを作り直す（全カメラ OK の区間で、底へ戻れる世界にする）。
func (r *runner) fbRepair() {
	w := r.w
	r.stepf("人が非表示の項目を戻し、消したシーンを作り直す")
	for {
		w.mu.Lock()
		n := len(w.fb.deleted)
		w.mu.Unlock()
		if n == 0 {
			break
		}
		r.fbRevive()
	}
	w.mu.Lock()
	for _, name := range w.order {
		for _, it := range w.cont[name].items {
			k := ik{name, it.id}
			if !it.enabled && !w.appOwned[k] {
				it.enabled = true
				w.enqueueLocked(switcher.ItemEnabledChanged{At: r.clk.Now(), SceneUUID: name, ItemID: it.id, Enabled: true}, true, k, true)
			}
		}
	}
	w.fbOnHumanLocked("修復", false)
	w.fbMapMutatedLocked()
	w.mu.Unlock()
	w.deliverDue(true)
	r.settle()
}

// fbStable は、入力が止まった（カメラの状態を固定した）安定区間を作り、規則で決まる行き先にいること、
// 往復し続けないこと（切替が 2n 回以下、終わりの20秒は0回）を確かめる。
func (r *runner) fbStable(label string, vec map[string]float64, repair bool) {
	w := r.w
	w.mu.Lock()
	w.benign = true
	w.mu.Unlock()
	r.stepf("安定区間 %s（%v, repair=%v）", label, vec, repair)
	r.reconnect()
	w.mu.Lock()
	needStream, needStudioOff := !w.streaming, w.studio
	w.mu.Unlock()
	if needStream {
		r.streamToggle()
	}
	if needStudioOff {
		r.studioToggle()
	}
	r.fbSetAuto(true)
	for i := 0; i < 3; i++ {
		if r.fbSnap().OBS.Standby.Owner != "manual" {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = r.cur.core.Command(ctx, switcher.CmdStandbyOff)
		cancel()
		w.mu.Lock()
		w.fb.cmdCount++
		w.mu.Unlock()
		r.settle()
		r.second()
	}
	if repair {
		r.fbRepair()
	}
	w.mu.Lock()
	wantBottom := ""
	if st0 := r.fbSnap(); w.fbQuietLocked() && st0.OBS.Standby.Owner == "auto" && w.fb.epValid && w.fb.epBottom != "" && w.monitoredLocked(w.fb.epBottom) &&
		len(st0.OBS.Standby.Stack) > 0 && st0.OBS.Standby.Stack[0] == w.sceneNameLocked(w.fb.epBottom) {
		wantBottom = w.fb.epBottom // 区間の初めの底（独立の計算で、status の底と一致している）
	}
	for _, id := range w.cameraIDs() {
		w.kbps[id] = vec[id]
	}
	app0 := w.fb.appSwitches
	n := 0
	for _, u := range w.sceneUUIDsLocked() {
		if w.monitoredLocked(u) {
			n++
		}
	}
	w.mu.Unlock()
	const hold = 60
	for range hold {
		r.second()
	}
	r.settle()
	w.mu.Lock()
	defer w.mu.Unlock()
	excl := func(why string) {
		if repair {
			w.fb.count("安定区間(全復旧): 除外 " + why)
		}
	}
	if len(w.violations) > 0 {
		return
	}
	if !w.fbQuietLocked() {
		excl("静かでない")
		return
	}
	snap := r.fbSnap()
	owner, st := snap.OBS.Standby.Owner, snap.OBS.Standby.Stack
	vf := func(format string, a ...any) {
		w.violate("6.3 安定区間 %s: %s（status: owner=%s scene=%s stack=%v、カメラの状態 %v、監視対象のシーン数 n=%d）", label, fmt.Sprintf(format, a...), owner, snap.OBS.Scene, st, w.fbStatesLocked(), n)
	}
	w.fb.count("安定区間")
	if sw := w.fb.appSwitches - app0; sw > 2*n {
		vf("安定区間の切替が %d 回で、2n=%d を超えた（往復している）", sw, 2*n)
		return
	}
	if age := w.clk.Now().Sub(w.fb.lastSwitch); w.fb.appSwitches > app0 && age < 20*time.Second {
		vf("終わりの20秒以内（%v 前）にまだ切替がある（振動）", age)
		return
	}
	for _, cs := range snap.Cameras {
		if cs.Unusable || cs.UserShown {
			excl("使えない・人の印") // 独立の計算が及ばない（待ちの例外）
			return
		}
	}
	if owner == "manual" || !snap.OBS.Streaming {
		excl("manual か配信なし")
		return
	}
	if w.monitoredLocked(w.program) && w.badLocked(w.program) {
		vf("今のシーン %s は監視対象で悪いのに、逃がしていない（待機は必ず入れる）", w.sceneNameLocked(w.program))
		return
	}
	enterable := func(n string) bool {
		u := w.byNameLocked(n)
		return u != "" && w.monitoredLocked(u) && !w.badLocked(u) && len(w.exceptUnusable(w.okHiddenRecordLocked(u), snap)) == 0 &&
			len(w.visibleLocked(u, false, nil)) > 0 && len(w.reverseWaitLocked(u, snap)) == 0
	}
	if owner == "auto" {
		for _, n := range st {
			if enterable(n) {
				vf("履歴の %s は入ってよい（健康 OK、Show 済み、実表示あり、逆向きの待ちなし）のに、戻っていない", n)
				return
			}
		}
	}
	if !repair {
		return
	}
	// 全カメラ OK・全 Show 成功（benign）の区間: none と空のスタックへの到達は必須。入れない構成（人が消した項目など）は別に数える。
	if owner != "none" || len(st) != 0 {
		for _, n := range st {
			if !enterable(n) {
				excl("戻り先が入れない構成")
				return
			}
		}
		vf("全カメラ OK・全 Show 成功なのに、持ち主 none・空のスタックに到達していない")
		return
	}
	w.fb.count("安定区間(全復旧): 検証した(none・スタック空)")
	if wantBottom != "" && w.monitoredLocked(wantBottom) {
		if w.program != wantBottom {
			vf("全カメラ OK で、最後は本体が最初に離れたシーン %s にいるはずが %s", w.sceneNameLocked(wantBottom), w.sceneNameLocked(w.program))
			return
		}
		w.fb.count("安定区間(全復旧): 底へ戻った")
	}
}

func (w *world) fbStatesLocked() string {
	var parts []string
	for _, id := range w.cameraIDs() {
		parts = append(parts, fmt.Sprintf("%s=%s", id, w.judges[id].State()))
	}
	return strings.Join(parts, " ")
}

// ---- 1つの種の実行 ----

// 列のプロファイル:
//
//	normal  通常の保証の範囲（応答は5秒以内、本体は落ちない、イベントは欠けない。遅延・重複・地図の読み直しの順の入れ替えは含む）。
//	        確かめる性質: 全 FB-INV、HB-INV-01〜04、独立の入場判定、静かな時点の status、安定区間の到達（全復旧で none・空）。
//	fault   OBS の応答の Timeout・イベントの欠落（保証外）。確かめる性質: 落ちない、構造の不変条件（FB-INV-01〜03）、
//	        回復の区間で隠した項目がすべて戻る（記録回収の到達性）。
//	late    12〜15秒のシーンの遅着（期待の期限切れ）と本体の再起動（保証外）。確かめる性質: 構造の不変条件（FB-INV-01〜05・07・09 相当）、
//	        独立の入場判定、回復の区間で none・空のスタックへ到達すること（履歴の底の照合は、遅着・再起動を含む区間では無効にする）。
func runSeedFB(t *testing.T, seed int64, profile string, steps int) {
	faulty := profile == "fault"
	path, hw := trackedHiddenPath(t)
	clk := newClock()
	w := newWorld(seed, clk, path, faulty)
	rrng := rand.New(rand.NewSource(seed ^ 0xfb0c))
	rows := fbRandomRows(rrng)
	if seed%3 == 0 {
		rows = fbSpecRows()
	}
	w.enableFB(rows)
	w.fb.late = profile == "late"
	r := &runner{t: t, w: w, clk: clk, rng: rand.New(rand.NewSource(seed ^ 0x5eed)), path: path, hw: hw, logBuf: &syncBuf{}}
	defer w.close()
	r.startGen(true, switcher.StudioHideStandby)
	r.settle()
	r.stepf("設定 scene_fallbacks=%v", rows)
	for i := 0; i < steps && len(w.violations) == 0; i++ {
		switch x := r.rng.Intn(116); {
		case x < 22:
			r.fbCamSingle()
		case x < 32:
			r.fbCamMulti()
		case x < 40:
			r.idle()
		case x < 48:
			r.humanToggle()
		case x < 56:
			r.fbHumanScene()
		case x < 66:
			r.fbCommand()
		case x < 70:
			r.streamToggle()
		case x < 75:
			if r.rng.Intn(2) == 0 {
				r.disconnect()
			} else {
				r.reconnect()
			}
		case x < 78:
			r.studioToggle()
		case x < 81:
			r.showAll()
		case x < 85:
			r.humanAddRemove()
		case x < 91:
			r.fbReconfig()
		case x < 94:
			r.fbAutoToggle()
		case x < 97:
			r.fbRename()
		case x < 100:
			r.fbDelete()
		case x < 103:
			r.fbRevive()
		case x < 105:
			if profile == "late" {
				r.restart()
			} else {
				r.idle()
			}
		case x < 111:
			r.fbOnDuringReturn()
		default:
			w.deliverDue(true)
			r.settle()
		}
		r.fbCheck(fmt.Sprintf("入力 %d のあと", i))
	}
	if len(w.violations) == 0 {
		r.recoverAll()
	}
	if len(w.violations) == 0 {
		good := map[string]float64{wMain: goodKbps, wSub: goodKbps, wHand: goodKbps, wAlt: goodKbps}
		r.fbStable("全カメラ OK", good, true)
	}
	if len(w.violations) == 0 {
		vec := map[string]float64{}
		for _, id := range w.cameraIDs() {
			switch x := r.rng.Intn(100); {
			case x < 50:
				vec[id] = goodKbps
			case x < 85:
				vec[id] = 0
			default:
				vec[id] = 500
			}
		}
		r.fbStable("固定の悪い状態", vec, false)
	}
	if len(w.violations) == 0 {
		w.mu.Lock()
		w.benign = true
		w.mu.Unlock()
		r.stopGen(true)
	} else {
		r.stopGen(false)
	}
	r.fbCheckLog(faulty)
	if len(w.violations) > 0 {
		t.Errorf("seed=%d profile=%s: %s\n--- 入力の列 ---\n%s\n--- 世界の記録（末尾） ---\n%s",
			seed, profile, strings.Join(w.violations, "\n"), strings.Join(r.steps, "\n"), joinTrace(w.trace, 150))
	}
	w.mu.Lock()
	cov := w.fb.cov
	st := w.stats
	w.mu.Unlock()
	fbCovMu.Lock()
	for k, v := range cov {
		fbCov[k] += v
	}
	fbCov["seed"]++
	fbCovMu.Unlock()
	t.Logf("seed=%d faulty=%v: false=%d true=%d timeout=%d restarts=%d appSwitches=%d", seed, faulty, st.appFalse, st.appTrue, st.timeouts, st.restarts, w.fb.appSwitches)
}

// fbCheckLog は「scene switch」のログを見る。退避の reason は 0b のまま low / down（行き先は to）、復帰は ok。
func (r *runner) fbCheckLog(faulty bool) {
	w := r.w
	if r.logBuf == nil {
		return
	}
	re := regexp.MustCompile(`msg="scene switch" from=(\S+) to=(\S+) reason=(\S+)`)
	for _, line := range strings.Split(r.logBuf.String(), "\n") {
		g := re.FindStringSubmatch(line)
		if g == nil {
			continue
		}
		switch g[3] {
		case "low", "down", "ok", "command", "stream_start":
		default:
			w.mu.Lock()
			w.violate("ログの reason が想定外: %s", line)
			w.mu.Unlock()
		}
		if g[3] == "command" && strings.Contains(line, " camera=") {
			w.mu.Lock()
			w.violate("手動の切替のログに架空の camera が付いている: %s", line)
			w.mu.Unlock()
		}
	}
}

func fbSeedCount(short, normal int) int { return seedCount(short, normal) }

func TestIntegrationFallbackInvariants(t *testing.T) {
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
	fbCovMu.Lock()
	clear(fbCov) // 呼び出しごとに初期化（-count で持ち越さない）
	fbCovMu.Unlock()
	t.Cleanup(func() {
		fbCovMu.Lock()
		defer fbCovMu.Unlock()
		var keys []string
		for k := range fbCov {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", k, fbCov[k]))
		}
		t.Logf("0c の乱数の列の到達（全種の合計）: %s", strings.Join(parts, ", "))
		if fbCov["seed"] < 50 || t.Failed() {
			return
		}
		for _, k := range []string{"戻る切替", "監視対象への逃がす切替", "待機への切替", "深さ2以上", "個別の退避先にいる", "RestartMedia", "ON特例(待機のauto)",
			"OFF(manual)", "設定の変更", "改名", "削除", "復活", "名前がないfallbackの警告とOBSの警告の同時", "底の照合(FB-INV-10)", "安定区間",
			"履歴が増えた到着", "履歴が減った到着", "ON特例の後に、古い戻りの切替が届いた", "ON特例の後の遅着: last_error の通知",
			"安定区間(全復旧): 検証した(none・スタック空)", "安定区間(全復旧): 底へ戻った", "遅着のシーン切替", "地図の読み直しが先のイベントを追い越した"} {
			if fbCov[k] == 0 {
				t.Errorf("乱数の列が %q に一度も届いていない（入力の混ぜ方の見直しが要る）", k)
			}
		}
	})
	steps := 100
	sem := make(chan struct{}, 3)
	run := func(name string, n int, seedBase int64) {
		for i := 0; i < n; i++ {
			seed := seedBase + int64(i)
			t.Run(fmt.Sprintf("%s/seed%d", name, seed), func(t *testing.T) {
				t.Parallel()
				sem <- struct{}{}
				defer func() { <-sem }()
				runSeedFB(t, seed, name, steps)
			})
		}
	}
	run("normal", fbSeedCount(6, 24), 20000)
	run("fault", fbSeedCount(3, 8), 25000)
	run("late", fbSeedCount(4, 16), 30000)
}

// ---- 固定の列（spec 第3節の例、ログ、共有入力の再起動） ----

type fbFixed struct {
	r *runner
	w *world
}

func newFBFixed(t *testing.T, start string, rows []switcher.FallbackRow) *fbFixed {
	path, hw := trackedHiddenPath(t)
	clk := newClock()
	w := newWorld(1, clk, path, false)
	w.enableFB(rows)
	w.program = start
	w.benign = true
	r := &runner{t: t, w: w, clk: clk, rng: rand.New(rand.NewSource(1)), path: path, hw: hw, logBuf: &syncBuf{}}
	t.Cleanup(w.close)
	r.startGen(true, switcher.StudioHideStandby)
	r.settle()
	return &fbFixed{r: r, w: w}
}

func (f *fbFixed) set(good map[string]bool, secs int) {
	f.w.mu.Lock()
	for _, id := range f.w.cameraIDs() {
		if g, ok := good[id]; ok {
			f.w.kbps[id] = 0
			if g {
				f.w.kbps[id] = goodKbps
			}
		}
	}
	f.w.mu.Unlock()
	for range secs {
		f.r.second()
	}
}

func (f *fbFixed) want(t *testing.T, label, scene, owner string, stack ...string) {
	t.Helper()
	snap := f.r.fbSnap()
	if stack == nil {
		stack = []string{}
	}
	if snap.OBS.Scene != scene || snap.OBS.Standby.Owner != owner || !slices.Equal(snap.OBS.Standby.Stack, stack) {
		f.w.mu.Lock()
		viol := strings.Join(f.w.violations, "\n")
		f.w.mu.Unlock()
		t.Fatalf("%s: scene=%s owner=%s stack=%v、期待 scene=%s owner=%s stack=%v\n%s\n%s", label, snap.OBS.Scene, snap.OBS.Standby.Owner, snap.OBS.Standby.Stack,
			scene, owner, stack, viol, joinTrace(f.w.trace, 60))
	}
}

func (f *fbFixed) noViolation(t *testing.T) {
	t.Helper()
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	if len(f.w.violations) > 0 {
		t.Fatalf("%s\n%s", strings.Join(f.w.violations, "\n"), joinTrace(f.w.trace, 80))
	}
}

// 表 0c の例 1〜4・7 と同じ列を、Hide / Show・応答・イベントを省かない偽の OBS で通し、各段の status とログ（reason は 0b のまま low / down / ok、
// 行き先は to、画像・手動に架空の camera を付けない）を確かめる。
func TestIntegrationFallbackSpecExample(t *testing.T) {
	f := newFBFixed(t, camera, []switcher.FallbackRow{{Scene: camera, Fallback: fbSubOnly}, {Scene: fbDual, Fallback: camera}, {Scene: "Hand", Fallback: talk}})
	all := map[string]bool{wMain: true, wSub: true, wHand: true, wAlt: true}
	f.set(all, 10)
	f.want(t, "全カメラ OK", camera, "none")
	f.set(map[string]bool{wMain: false}, 7)
	f.want(t, "メインが悪化", fbSubOnly, "auto", camera)
	f.set(map[string]bool{wSub: false}, 7)
	f.want(t, "サブも悪化", standby, "auto", camera, fbSubOnly)
	f.set(map[string]bool{wSub: true}, 8)
	f.want(t, "サブが回復（メインはまだ悪い）", fbSubOnly, "auto", camera)
	f.set(map[string]bool{wMain: true}, 8)
	f.want(t, "メインが回復", camera, "none")
	f.noViolation(t)
	// 別の列: 待機にいる間に、メインとサブが同時に回復する（SubOnly を飛ばして直接戻る）
	f.set(map[string]bool{wMain: false}, 7)
	f.set(map[string]bool{wSub: false}, 7)
	f.want(t, "待機へ", standby, "auto", camera, fbSubOnly)
	f.set(map[string]bool{wMain: true, wSub: true}, 9)
	f.want(t, "同時に回復して底へ戻る", camera, "none")
	// 手動の待機 ON と OFF（ログに camera を付けない）
	if _, err := f.r.fbCommandKind(switcher.CmdStandbyOn); err != nil {
		t.Fatalf("ON: %v", err)
	}
	f.r.settle()
	f.r.second()
	f.want(t, "手動の待機", standby, "manual")
	if _, err := f.r.fbCommandKind(switcher.CmdStandbyOff); err != nil {
		t.Fatalf("OFF: %v", err)
	}
	f.r.settle()
	f.r.second()
	f.want(t, "OFF で戻る", camera, "none")
	f.noViolation(t)

	logs := f.r.logBuf.String()
	var seq []string
	re := regexp.MustCompile(`msg="scene switch" from=(\S+) to=(\S+) reason=(\S+)(?: camera=(\S+) kbps=(\S+))?`)
	for _, line := range strings.Split(logs, "\n") {
		if g := re.FindStringSubmatch(line); g != nil {
			seq = append(seq, strings.Join(g[1:], " "))
		}
	}
	// 起動直後（全カメラ DOWN から回復するまで）の切替は列の頭に付く。例の列は、最後の「Camera → SubOnly low cam1」から。
	want := []string{
		"Camera SubOnly low cam1 0",
		"SubOnly Standby low cam2 0",
		"Standby SubOnly ok cam2 3000",
		"SubOnly Camera ok cam1 3000",
		"Camera SubOnly low cam1 0",
		"SubOnly Standby low cam2 0",
		"Standby Camera ok cam2 3000", // 同時に OK になったときは、あとに処理したカメラ（設定順で後）
		"Camera Standby command  ",
		"Standby Camera command  ",
	}
	i := slices.Index(seq, want[0])
	if i < 0 || !slices.Equal(seq[i:], want) {
		t.Errorf("scene switch のログの列が違う:\n実際 %q\n期待（末尾）%q", seq, want)
	}
	for _, l := range strings.Split(logs, "\n") {
		if strings.Contains(l, "reason=fallback") {
			t.Errorf("reason は 0b のまま low / down（行き先は to）: %s", l)
		}
	}
}

// 入力 IRL を共有する Dual（メイン＋alt）と Camera（メイン＋サブ）: alt が悪化して Dual → Camera へ逃がし、alt が回復して戻るとき、
// Camera に実際に表示されている IRL は再起動せず、ALT だけを再起動する（0c の共有入力の除外。独立の計算）。
func TestIntegrationFallbackSharedInputRestart(t *testing.T) {
	f := newFBFixed(t, fbDual, []switcher.FallbackRow{{Scene: fbDual, Fallback: camera}})
	all := map[string]bool{wMain: true, wSub: true, wHand: true, wAlt: true}
	f.set(all, 10)
	f.want(t, "Dual で全カメラ OK", fbDual, "none")
	f.w.mu.Lock()
	n0 := len(f.w.fb.restarts)
	f.w.mu.Unlock()
	f.set(map[string]bool{wAlt: false}, 7)
	f.want(t, "alt が悪化して Camera へ逃がす", camera, "auto", fbDual)
	f.set(map[string]bool{wAlt: true}, 8)
	f.want(t, "alt が回復して Dual へ戻る", fbDual, "none")
	f.noViolation(t)
	f.w.mu.Lock()
	rs := slices.Clone(f.w.fb.restarts[n0:])
	f.w.mu.Unlock()
	if len(rs) != 1 || !slices.Equal(rs[0], []string{"ALT"}) {
		t.Errorf("戻るときの RestartMedia は [ALT] の1回だけのはず（Camera が表示している IRL は除く）: %v", rs)
	}
}
