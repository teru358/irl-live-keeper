package core

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// genClient は世代ごとの obs.Client。古い世代（落ちた本体）からの要求は、何も適用せずに失敗させる。
type genClient struct {
	w   *world
	gen int
}

var errStale = errors.New("この世代の本体は終わっている")

func (g *genClient) Events() <-chan switcher.Event {
	g.w.mu.Lock()
	defer g.w.mu.Unlock()
	return g.w.evCh
}

func (g *genClient) current() bool { return g.w.gen == g.gen }

func (g *genClient) StartStream() error {
	g.w.activity.Add(1)
	defer g.w.activity.Add(1)
	g.w.mu.Lock()
	if !g.current() || !g.w.connected {
		g.w.mu.Unlock()
		return errors.New("OBS に未接続")
	}
	g.w.streaming = true
	g.w.mu.Unlock()
	g.w.sendNow(switcher.StreamStateChanged{At: g.w.clk.Now(), Streaming: true})
	return nil
}

func (g *genClient) StopStream() error {
	g.w.activity.Add(1)
	defer g.w.activity.Add(1)
	g.w.mu.Lock()
	if !g.current() || !g.w.connected {
		g.w.mu.Unlock()
		return errors.New("OBS に未接続")
	}
	g.w.streaming = false
	g.w.clearMarksLocked("配信の停止")
	g.w.mu.Unlock()
	g.w.sendNow(switcher.StreamStateChanged{At: g.w.clk.Now(), Streaming: false})
	return nil
}

func (g *genClient) SceneTransitionOverrideUUID(string) (*int, bool, error) {
	g.w.mu.Lock()
	defer g.w.mu.Unlock()
	if !g.current() || !g.w.connected {
		return nil, false, errors.New("OBS に未接続")
	}
	return nil, false, nil
}

func (g *genClient) RestartMediaInputs([]string) error {
	g.w.mu.Lock()
	defer g.w.mu.Unlock()
	if !g.current() || !g.w.connected {
		return errors.New("OBS に未接続")
	}
	return nil
}

func (g *genClient) SetSceneUUID(name string) error {
	g.w.activity.Add(1)
	defer g.w.activity.Add(1)
	g.w.mu.Lock()
	if !g.current() || !g.w.connected {
		g.w.mu.Unlock()
		return errors.New("OBS に未接続")
	}
	if g.w.cont[name] == nil {
		g.w.mu.Unlock()
		return errors.New("request SetCurrentProgramScene: ResourceNotFound (600)")
	}
	g.w.program = name
	g.w.logf("本体: シーンを %s に切替", name)
	ev := switcher.SceneChanged{At: g.w.clk.Now(), Scene: name, SceneUUID: name}
	sync := g.w.benign || g.w.rng.Intn(2) == 0
	if !sync {
		g.w.enqueueLocked(ev, false, ik{}, false)
	}
	g.w.mu.Unlock()
	if sync {
		g.w.sendNow(ev)
	}
	return nil
}

// ---- SetSceneItemEnabled ----

type reqMode int

const (
	modeEventFirst reqMode = iota // 適用してイベントを流してから応答する
	modeRespFirst                 // 応答してから、遅れてイベントが流れる
	modeNotFound                  // 状態コード 600（適用しない）
	modeError                     // OBS の失敗（適用しない）
	// ここから下は前提の外（OBS が5秒以内に応えない、イベントが欠ける）。
	modeTimeoutNever // Timeout（適用しない）
	modeTimeoutLate  // Timeout のあと、遅れて適用する
	modeDropEvent    // 適用して応答するが、イベントは欠ける
	modeHang         // 切断まで応えない（切断で Timeout）
)

func (w *world) pickMode() reqMode {
	if w.benign {
		return modeEventFirst
	}
	r := w.rng.Intn(100)
	switch {
	case r < 40:
		return modeEventFirst
	case r < 82:
		return modeRespFirst
	case r < 92:
		return modeError // OBS の失敗（適用しない）。NotFound（状態コード 600）は、項目が本当にないときだけ返す
	}
	if !w.faulty {
		return modeRespFirst
	}
	switch w.rng.Intn(4) {
	case 0:
		return modeTimeoutNever
	case 1:
		return modeTimeoutLate
	case 2:
		return modeDropEvent
	}
	return modeHang
}

func (g *genClient) SetSceneItemEnabled(scene string, id int, en bool) error {
	w := g.w
	w.activity.Add(1)
	defer w.activity.Add(1)
	w.mu.Lock()
	if !g.current() || !w.connected {
		w.mu.Unlock()
		return errors.New("OBS に未接続（送っていない）")
	}
	k := ik{scene, id}
	c := w.cont[scene]
	var it *wItem
	if c != nil {
		it = c.find(id)
	}
	if it == nil {
		w.logf("本体: 項目 %v を %v にする要求 → 項目がない（600）", k, en)
		w.mu.Unlock()
		return errors.New("request SetSceneItemEnabled: ResourceNotFound (600): no such item")
	}
	mode := w.pickMode()
	w.logf("本体: 項目 %v を %v にする要求（現在 %v, mode=%d）", k, en, it.enabled, mode)
	if !en {
		w.checkFalseIssueLocked(k, it)
	} else {
		w.trueIssued[k] = true
		if u := w.uncertain[k]; u != nil && (mode == modeEventFirst || mode == modeRespFirst || mode == modeDropEvent) {
			*u = true // 本体が true を送って OK を得た: 未決着の記録は消えうる
		}
	}
	switch mode {
	case modeNotFound:
		w.mu.Unlock()
		return errors.New("request SetSceneItemEnabled: ResourceNotFound (600): item removed")
	case modeError:
		w.mu.Unlock()
		return errors.New("request SetSceneItemEnabled: unexpected failure")
	case modeTimeoutNever:
		w.stats.timeouts++
		w.markUncertainLocked(k, en)
		w.mu.Unlock()
		return fmt.Errorf("%w: SetSceneItemEnabled", obs.ErrTimeout)
	case modeTimeoutLate:
		w.stats.timeouts++
		w.markUncertainLocked(k, en)
		w.late = append(w.late, lateApply{due: w.clk.Now().Add(time.Duration(1+w.rng.Intn(8)) * time.Second), key: k, val: en})
		w.mu.Unlock()
		return fmt.Errorf("%w: SetSceneItemEnabled（遅れて適用される）", obs.ErrTimeout)
	case modeHang:
		w.stats.timeouts++
		w.markUncertainLocked(k, en)
		ch := make(chan struct{})
		w.hung = append(w.hung, ch)
		apply := w.rng.Intn(2) == 0
		w.mu.Unlock()
		select {
		case <-ch:
		case <-w.done:
		}
		if apply {
			w.mu.Lock()
			w.applyLocked(k, en, "本体（切断の直前に適用）", false)
			w.mu.Unlock()
		}
		return fmt.Errorf("%w: SetSceneItemEnabled（切断）", obs.ErrTimeout)
	}
	ev, changed := w.applyLocked(k, en, "本体", mode == modeDropEvent)
	if changed && mode == modeRespFirst {
		w.enqueueLocked(ev, false, ik{}, false)
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()
	if changed && mode == modeEventFirst {
		w.sendNow(ev)
	}
	return nil
}

// applyLocked は項目の値を変える。変わったら ItemEnabledChanged を返す（ドロップなら nil で、変わったことだけ返す）。
func (w *world) applyLocked(k ik, v bool, by string, dropEvent bool) (switcher.Event, bool) {
	it := w.cont[k.c].find(k.id)
	if it == nil || it.enabled == v {
		return nil, false
	}
	if !v {
		if u := w.uncertain[k]; u != nil && *u && (strings.Contains(by, "遅れ") || strings.Contains(by, "切断")) {
			w.logf("既知の制約: Timeout のあと、本体の true が OK になってから遅れて false が適用された（記録は消えている）")
		} else {
			w.checkFalseApplyLocked(k, it, by)
		}
		w.stats.appFalse++
	} else {
		w.stats.appTrue++
	}
	it.enabled = v
	if v {
		delete(w.appOwned, k)
	} else {
		w.appOwned[k] = true
	}
	w.logf("%s: 項目 %v を %v に（適用）", by, k, v)
	if dropEvent {
		return nil, true
	}
	return switcher.ItemEnabledChanged{At: w.clk.Now(), SceneUUID: k.c, ItemID: k.id, Enabled: v}, true
}

func (w *world) markUncertainLocked(k ik, en bool) {
	if !en {
		cleared := false
		w.uncertain[k] = &cleared
	}
}

func (w *world) clearMarksLocked(why string) {
	if len(w.mark) > 0 {
		w.logf("印を消した: %s", why)
	}
	w.mark = map[string]bool{}
	w.humanRestoredOwned = map[ik]bool{}
}

// ---- 事実の判定 ----

// checkFalseIssueLocked は、本体が false の要求を出した（応答を待つ）時点の判定（INV-02、INV-03）。
func (w *world) checkFalseIssueLocked(k ik, it *wItem) {
	cam := w.cameraOfItemLocked(k)
	// INV-03: 人が戻した印が本体に届いたあとの、同じカメラへの false は、同じ不調の間は出さない。
	if cam != "" && w.mark[cam] && !w.canRestoreOracleLocked(cam) {
		w.stats.issuesWithMark++
		if !w.faulty {
			w.violate("INV-03: 人が戻した印があるのに、カメラ %s の項目 %v に false を出した", cam, k)
		}
	}
	// INV-02: 配信中の今のシーン（本体から見て監視対象）の、実際に表示されているカメラを0にしない。
	if w.faulty || !w.streaming || w.humanPendingLocked() {
		return
	}
	if len(w.visibleLocked(w.program, true, nil)) == 0 {
		return // 監視対象でない（待機シーンなど）
	}
	before := w.visibleLocked(w.program, false, nil)
	if len(before) == 0 {
		return // すでに空（人の操作）
	}
	after := w.visibleLocked(w.program, false, map[ik]bool{k: false})
	if len(after) == 0 {
		w.violate("INV-02: 配信中の今のシーン %s の表示されているカメラを、項目 %v の false で0にした（前 %v）", w.program, k, sortedKeys(before))
	}
}

// checkFalseApplyLocked は、本体の false が適用される（true→false）瞬間の判定（INV-01）。
// 偽の OBS が記録した事実と、hidden.json のファイルの中身だけを使う。
func (w *world) checkFalseApplyLocked(k ik, it *wItem, by string) {
	es, err := readHiddenFileRaw(w.hiddenPath)
	if err != nil {
		w.violate("INV-01: 本体の false を適用する瞬間に hidden.json を読めない（%s, %v）: %v", by, k, err)
		return
	}
	for _, e := range es {
		if e.Container == k.c && e.Item == k.id && e.Source == it.source && e.Collection == testCollection &&
			e.Host == testEndpoint.Host && e.Port == testEndpoint.Port {
			return
		}
	}
	w.violate("INV-01: 本体の false（%s）で項目 %v（ソース %s）が非表示になったが、その瞬間の hidden.json に記録がない: %+v", by, k, it.source, es)
}

// persistentCheckLocked は、静かな時点の判定: 本体が隠した項目（人が触っていない）は、hidden.json にある（INV-01）。
func (w *world) persistentCheckLocked() {
	if w.faulty {
		return
	}
	es, err := readHiddenFileRaw(w.hiddenPath)
	if err != nil {
		w.violate("INV-01: hidden.json を読めない: %v", err)
		return
	}
	for k := range w.appOwned {
		it := w.cont[k.c].find(k.id)
		if it == nil || it.enabled {
			continue
		}
		if !slices.ContainsFunc(es, func(e fileEntry) bool {
			return e.Container == k.c && e.Item == k.id && e.Collection == testCollection
		}) {
			w.violate("INV-01: 本体が隠した項目 %v が、静かな時点の hidden.json にない: %+v", k, es)
		}
	}
}

func readHiddenFileRaw(path string) ([]fileEntry, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseHiddenFile(b)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func joinTrace(lines []string, n int) string {
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

var _ = scenemap.StateOK
