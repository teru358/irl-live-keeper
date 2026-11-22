package switcher

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"
)

// 0c の性質のテスト（表 0c の6章の FB-INV のうち、Machine の範囲で決まるもの）。
// 0b の pfake（偽の OBS の事実、独立の期待の台帳、実行役の模型）に scene_fallbacks を加え、乱数の列の途中で設定を替える。
// 判定は Machine の探索・CanEnter・健康の計算を使わず、Snapshot（公開の値）と、Event を渡す前後の比較だけでする。
//   FB-INV-01  スタックに同じ UUID が2回ない。今のシーンと待機シーンを含まない。
//   FB-INV-02  manual / none ではスタックは空。配信していない間は持ち主 none でスタックは空。
//   FB-INV-03  接続中の auto ではスタックは非空、return_scene は底、全要素は地図にある。
//   FB-INV-04  履歴は、期待どおりの SceneChanged・Command・地図/接続/配信の Event だけで変わる。
//              CamerasChanged / Tick / SceneRequestDone / BatchDone / ItemRequestDone / 承認 / 同値の設定では変わらない。
//   FB-INV-06  scene_fallbacks の変更（行き先の変更・並べ替え）だけでは、履歴も持ち主も変わらない。
//   FB-INV-09  Snapshot の slice・ポインタの中身を呼び出し側が変えても、内部は変わらない。

type fbStk struct {
	owner  Owner
	ret    string
	stack  []string
	filled bool
}

func (f *pfake) fbStack() fbStk {
	if !f.fbOn {
		return fbStk{}
	}
	ms := f.m.Snapshot()
	return fbStk{owner: ms.Standby.Owner, ret: ms.Standby.ReturnScene, stack: slices.Clone(ms.Standby.Stack), filled: true}
}

func (a fbStk) equal(b fbStk) bool {
	return a.owner == b.owner && a.ret == b.ret && slices.Equal(a.stack, b.stack)
}

func (f *pfake) fbSame(before fbStk, what string) {
	f.t.Helper()
	if !f.fbOn || !before.filled {
		return
	}
	if now := f.fbStack(); !before.equal(now) {
		f.fail("FB-INV-04: %s だけで履歴が変わった: owner %v→%v return %q→%q stack %v→%v", what, before.owner, now.owner, before.ret, now.ret, before.stack, now.stack)
	}
}

// fbNoGrow は、履歴を足さない・切り替えない Event のあとの比較。記録（Hide の予約）の変化で分類が変わると、整理で要素を外すことはある
// （列挙済みの整理）が、新しい要素の追加も、順序の入れ替えも、持ち主の変更（auto → none の解除を除く）もない。
func (f *pfake) fbNoGrow(before fbStk, what string) {
	f.t.Helper()
	if !f.fbOn || !before.filled {
		return
	}
	now := f.fbStack()
	if before.equal(now) {
		return
	}
	i, ok := 0, true
	for _, u := range now.stack {
		for i < len(before.stack) && before.stack[i] != u {
			i++
		}
		if i >= len(before.stack) {
			ok = false
			break
		}
		i++
	}
	if !ok || (now.owner != before.owner && !(before.owner == OwnerAuto && now.owner == OwnerNone && len(now.stack) == 0)) {
		f.fail("FB-INV-04: %s だけで履歴が増えたか、持ち主が変わった: owner %v→%v return %q→%q stack %v→%v", what, before.owner, now.owner, before.ret, now.ret, before.stack, now.stack)
	}
}

// fbAfterEvent は Event を渡したあとの比較。履歴を変えてよい Event は SceneChanged・Command・地図・接続・配信・Studio・項目の Event（整理）だけ。
func (f *pfake) fbAfterEvent(ev Event, before fbStk) {
	f.t.Helper()
	if !f.fbOn {
		return
	}
	switch ev.(type) {
	case CamerasChanged, Tick:
		f.fbSame(before, fmt.Sprintf("%T", ev))
	case SceneRequestDone, BatchDone, ItemRequestDone:
		f.fbNoGrow(before, fmt.Sprintf("%T", ev))
	}
}

// fbReconfig は scene_fallbacks を替えて、ScenesConfigured を渡す（FB-INV-06: 履歴は変わらない）。
func (f *pfake) fbReconfig() {
	f.t.Helper()
	before := f.fbStack()
	rows := f.fbRandomRows()
	if f.rng.Intn(4) == 0 && len(f.fbRows) > 1 { // 並べ替えだけ
		rows = slices.Clone(f.fbRows)
		f.rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	}
	f.fbRows = rows
	f.logf("設定の変更 scene_fallbacks=%v", rows)
	f.handle(f.cfgEv())
	f.fbSame(before, "scene_fallbacks の変更")
}

func (f *pfake) fbSceneNames() []string {
	var names []string
	for _, c := range f.base.Scenes {
		if c.Name != "" && c.Name != "Standby" && c.Name != "Standby2" {
			names = append(names, c.Name)
		}
	}
	slices.Sort(names)
	return names
}

// fbRandomRows は輪にならない対応（並びの後ろへだけ向ける。待機・OBS にない名前も使う）。
func (f *pfake) fbRandomRows() []FallbackRow {
	pool := f.fbSceneNames()
	f.rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	var rows []FallbackRow
	for i, sc := range pool {
		if f.rng.Intn(100) >= 65 {
			continue
		}
		to := "Standby"
		switch x := f.rng.Intn(100); {
		case x < 8:
			to = "OBSにない名前"
		case x < 25 || i+1 >= len(pool):
		default:
			to = pool[i+1+f.rng.Intn(len(pool)-i-1)]
		}
		rows = append(rows, FallbackRow{Scene: sc, Fallback: to})
	}
	f.rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	return rows
}

func (f *pfake) fbCheck() {
	f.t.Helper()
	if !f.fbOn {
		return
	}
	ms := f.m.Snapshot()
	st := ms.Standby
	seen := map[string]bool{}
	for _, u := range st.Stack {
		if seen[u] {
			f.fail("FB-INV-01: スタック %v に同じ UUID %s が2回ある", st.Stack, u)
		}
		seen[u] = true
	}
	if ms.Connected {
		if seen[ms.Scene] || seen[standby] {
			f.fail("FB-INV-01: スタック %v に今のシーン %s か待機シーンがある", st.Stack, ms.Scene)
		}
		switch st.Owner {
		case OwnerManual, OwnerNone:
			if len(st.Stack) != 0 {
				f.fail("FB-INV-02: 持ち主 %v なのにスタックが空でない: %v", st.Owner, st.Stack)
			}
		case OwnerAuto:
			if len(st.Stack) == 0 || st.ReturnScene != st.Stack[0] {
				f.fail("FB-INV-03: auto なのにスタックが空か、戻り先 %q がスタックの底でない: %v", st.ReturnScene, st.Stack)
			}
			for _, u := range st.Stack {
				if _, ok := f.base.Scenes[u]; !ok {
					f.fail("FB-INV-03: スタックの %s が地図にない", u)
				}
			}
		}
		if !ms.Streaming && (st.Owner != OwnerNone || len(st.Stack) != 0) {
			f.fail("FB-INV-02: 配信していないのに持ち主 %v・スタック %v が残っている", st.Owner, st.Stack)
		}
	}
	fbReach[fmt.Sprintf("owner=%v", st.Owner)]++
	if len(st.Stack) >= 1 {
		fbReach["stack>=1"]++
	}
	if len(st.Stack) >= 2 {
		fbReach["stack>=2"]++
	}
	if len(ms.HiddenEntries) > 0 {
		fbReach["hidden記録あり"]++
	}
	f.fbIsolation()
}

// fbReach は乱数の列が届いた状態の数（空振りの確認用）。
var fbReach = map[string]int{}

// fbIsolation は FB-INV-09: Snapshot の中身を呼び出し側が変えても、次の Snapshot は変わらない。
func (f *pfake) fbIsolation() {
	f.t.Helper()
	a := f.m.Snapshot()
	b := f.m.Snapshot()
	for i := range b.Standby.Stack {
		b.Standby.Stack[i] = "!変えた"
	}
	for i := range b.Standby.StackNames {
		b.Standby.StackNames[i] = "!変えた"
	}
	for i := range b.VisibleCameras {
		b.VisibleCameras[i] = "!変えた"
	}
	for _, es := range [][]HiddenEntry{b.HiddenEntries, b.RetainedEntries} {
		for i := range es {
			es[i].CameraID, es[i].ContainerUUID, es[i].ItemID = "!変えた", "!変えた", -1
			if es[i].SettledAt != nil {
				*es[i].SettledAt = time.Unix(1, 0)
			}
		}
	}
	for i := range b.Cameras {
		b.Cameras[i].ID, b.Cameras[i].Unusable = "!変えた", !b.Cameras[i].Unusable
	}
	for i := range b.FallbackWarnings {
		b.FallbackWarnings[i].Name = "!変えた"
	}
	if c := f.m.Snapshot(); !reflect.DeepEqual(a, c) {
		f.fail("FB-INV-09: Snapshot の中身を呼び出し側が変えたら、内部が変わった:\n前 %+v\n後 %+v", a, c)
	}
}

func TestFB性質_履歴の不変条件(t *testing.T) {
	runs := 3000
	if testing.Short() {
		runs = 150
	}
	if v := os.Getenv("FB_RUNS"); v != "" {
		n, _ := strconv.Atoi(v)
		runs = n
	}
	first := int64(1)
	if v := os.Getenv("FB_SEED"); v != "" { // 落ちた種だけを再現する
		n, _ := strconv.ParseInt(v, 10, 64)
		first, runs = n, int(n)
	}
	for seed := first; seed <= int64(runs); seed++ {
		f := newPfake(t, seed)
		f.fbOn = true
		f.rng = rand.New(rand.NewSource(seed ^ 0x0c0c))
		f.fbRows = f.fbRandomRows()
		f.handle(f.cfgEv())
		f.mode = int(seed % 4)
		steps := 60 + f.rng.Intn(120)
		for i := 0; i < steps; i++ {
			f.randomStep(true)
		}
		f.recoverPhase()
	}
	if runs >= 1000 && !testing.Short() {
		t.Logf("到達: %v", fbReach)
		for _, k := range []string{"stack>=1", "stack>=2", "owner=manual", "hidden記録あり"} {
			if fbReach[k] == 0 {
				t.Errorf("乱数の列が %q に一度も届いていない", k)
			}
		}
	}
}
