package core

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 0c（シーンごとの逃がし先）の結合の試験の「世界」の拡張（表 0c の6章）。
//
// 判定は Machine の探索の関数・CanEnter・健康の計算を使わず、偽の OBS が持つ事実
// （program・項目・入力 UUID・実際に当たった要求・hidden.json）と、カメラごとの独立の Judge から計算する。
// 読むのは core.Snapshot()（status と同じ公開の値）だけ。

const fbObsWarning = "OBS の警告（0c の試験用）"

// 0c の世界のシーン。待機 Standby、カメラが映る Camera（メイン＋サブ）、SubOnly（サブだけ）、Both（サブ＋手元）、
// Hand（手元だけ）、Nest（Camera を入れ子＋手元）、Grp（グループ G1〔サブ・メイン〕＋手元）、画像のシーン Talk（カメラなし）。
const fbSubOnly = "SubOnly"

var fbPool = []string{camera, "Nest", "Grp", "Both", "Hand", fbSubOnly, talk}

// fbCov は全種の合計（空振りの検出と報告に使う）。
var (
	fbCovMu sync.Mutex
	fbCov   = map[string]int{}
)

type fbPrev struct {
	valid bool
	owner string
	stack []string
	ev    int64
	cmd   int64
	app   int
}

type fbWorld struct {
	rows    []switcher.FallbackRow
	deleted map[string]*wCont // 表示名 → 消したシーンの写し（復活用）
	seq     int
	snap    func() Snapshot

	// 同じ要求の再送の鍵に使う世代。
	humanGen, cfgGen, mapGen, connGen int

	// FB-INV-10 の oracle: 最後の共通解除のあと、本体が最初に離れた監視対象。
	epBottom string
	epValid  bool

	appSwitches int
	consumed    int // スタックが増えた回数（適用の数を超えない: FB-INV-05）
	lastSwitch  time.Time
	cmdCount    int64
	prev        fbPrev
	keyAt       map[string]time.Time

	cov map[string]int
}

func newFBWorld() *fbWorld {
	return &fbWorld{deleted: map[string]*wCont{}, epValid: true, keyAt: map[string]time.Time{}, cov: map[string]int{}}
}

func (f *fbWorld) count(name string) { f.cov[name]++ }

// ---- 世界の拡張（シーンの追加と名前） ----

// enableFB は世界を 0c 用にする（SubOnly を足す。ほかの形は 0b のまま）。
func (w *world) enableFB(rows []switcher.FallbackRow) {
	w.fb = newFBWorld()
	w.fb.rows = slices.Clone(rows)
	w.cont[fbSubOnly] = &wCont{name: fbSubOnly, items: []*wItem{{id: 1, source: "SUB", kind: scenemap.InputSource, enabled: true}}}
	w.order = append(w.order, fbSubOnly)
}

func (w *world) sceneNameLocked(u string) string {
	if c := w.cont[u]; c != nil && c.name != "" {
		return c.name
	}
	return u
}

// byNameLocked は表示名から UUID（シーンだけ）。なければ空。
func (w *world) byNameLocked(name string) string {
	for _, u := range w.order {
		c := w.cont[u]
		if c != nil && !c.group && c.name == name {
			return u
		}
	}
	return ""
}

func (w *world) sceneUUIDsLocked() []string {
	var out []string
	for _, u := range w.order {
		if c := w.cont[u]; c != nil && !c.group {
			out = append(out, u)
		}
	}
	return out
}

// monitoredLocked は監視対象か（地図の分類: カメラが映る道があるシーン。本体が隠した項目は表示として数える）。
func (w *world) monitoredLocked(u string) bool {
	c := w.cont[u]
	return u != standby && c != nil && !c.group && len(w.visibleLocked(u, true, nil)) > 0
}

// inputsShownLocked は scene に実際に表示されている入力 UUID（本体が隠した項目は非表示）。
func (w *world) inputsShownLocked(scene string) map[string]bool {
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
			if !it.enabled {
				continue
			}
			if it.kind == scenemap.InputSource {
				out[it.source] = true
			} else {
				walk(it.source, path)
			}
		}
	}
	walk(scene, map[string]bool{})
	return out
}

// snapOK は status（本体の見ているカメラの状態）が OK か。独立の Judge との間には、Sample を渡してから本体が処理するまでの差がある。
func snapOK(snap Snapshot, id string) bool {
	for _, cs := range snap.Cameras {
		if cs.ID == id {
			return cs.State == "OK"
		}
	}
	return false
}

func (w *world) stateOKLocked(cam string) bool { return w.judges[cam].State() == ingest.LinkOK }

// badLocked は監視対象 u の論理健康が悪いか（カメラの spec 第4節。Studio off の独立の計算）。
// standby のカメラ（メイン）が OK でない、または映るカメラが hide のものだけで、すべてが OK でない。
func (w *world) badLocked(u string) bool { return w.badWithLocked(u, w.stateOKLocked) }

func (w *world) badWithLocked(u string, okf func(string) bool) bool {
	cams := w.visibleLocked(u, true, nil)
	if len(cams) == 0 {
		return false
	}
	standbyBad, allHideNotOK := false, true
	for id := range cams {
		ok := okf(id)
		if id == wMain {
			allHideNotOK = false
			if !ok {
				standbyBad = true
			}
		} else if ok {
			allHideNotOK = false
		}
	}
	return standbyBad || allHideNotOK
}

// okHiddenRecordLocked は u に、本体の非表示のせいで映っていない OK のカメラがあるか（戻り先が待つ Show）。
func (w *world) okHiddenRecordLocked(u string) []string { return w.okHiddenRecordWithLocked(u, w.stateOKLocked) }

func (w *world) okHiddenRecordWithLocked(u string, okf func(string) bool) []string {
	if w.faulty {
		return nil
	}
	app := w.visibleLocked(u, true, nil)
	act := w.visibleLocked(u, false, nil)
	var out []string
	for id := range app {
		if !act[id] && okf(id) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// exceptUnusable は status で「使えない」（表示に戻すのに3回失敗した）カメラを除く（入場の条件の例外）。
func (w *world) exceptUnusable(ids []string, snap Snapshot) []string {
	var out []string
	for _, id := range ids {
		un := false
		for _, cs := range snap.Cameras {
			if cs.ID == id && cs.Unusable {
				un = true
			}
		}
		if !un {
			out = append(out, id)
		}
	}
	return out
}

// reverseWaitLocked は戻る方向の逆向きの待ち: 戻り先に実際に表示されている悪い hide のカメラ。
// 人が戻した印・使えないものは待たない（status の値で見る）。Studio off だけ。
func (w *world) reverseWaitLocked(u string, snap Snapshot) []string {
	return w.reverseWaitWithLocked(u, snap, w.stateOKLocked)
}

func (w *world) reverseWaitWithLocked(u string, snap Snapshot, okf func(string) bool) []string {
	if w.studio {
		return nil
	}
	var out []string
	for id := range w.visibleLocked(u, false, nil) {
		if id == wMain || okf(id) || w.mark[id] {
			continue
		}
		exempt := false
		for _, cs := range snap.Cameras {
			if cs.ID == id && (cs.UserShown || cs.Unusable) {
				exempt = true
			}
		}
		if !exempt {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// nextOfLocked は設定の対応と OBS の名前から、u の逃がし先（列の次）の UUID（なければ空）。待機は終端。
func (w *world) nextOfLocked(u string) string {
	sb := w.byNameLocked(standby)
	if u == sb {
		return ""
	}
	name := w.sceneNameLocked(u)
	for _, row := range w.fb.rows {
		if row.Scene == name {
			if to := w.byNameLocked(row.Fallback); to != "" {
				return to
			}
			return sb
		}
	}
	return sb
}

// chainLocked は u の逃がし先の列（u 自身を含まない。待機か監視対象でないシーンで終わる）。
func (w *world) chainLocked(u string) []string {
	var out []string
	seen := map[string]bool{u: true}
	for cur := w.nextOfLocked(u); cur != "" && !seen[cur]; cur = w.nextOfLocked(cur) {
		seen[cur] = true
		out = append(out, cur)
		if !w.monitoredLocked(cur) {
			break
		}
	}
	return out
}

func (w *world) hasPendingAppSceneLocked() bool {
	return slices.ContainsFunc(w.pending, func(p pendEv) bool { return p.appScene })
}

// ---- 世界のできごとの通知（0b の世界から呼ぶ。fb が nil なら何もしない） ----

// fbOnHumanLocked は人の操作。prog なら人のシーン切替（共通の解除）。
func (w *world) fbOnHumanLocked(why string, prog bool) {
	f := w.fb
	if f == nil {
		return
	}
	w.humanWin = true
	f.humanGen++
	if prog {
		f.epBottom = ""
		f.epValid = !w.hasPendingAppSceneLocked()
	}
}

func (w *world) fbOnStreamLocked() {
	f := w.fb
	if f == nil {
		return
	}
	f.connGen++
	f.epBottom = ""
	f.epValid = !w.hasPendingAppSceneLocked()
}

func (w *world) fbOnConnLocked() {
	f := w.fb
	if f == nil {
		return
	}
	f.connGen++
	if w.hasPendingAppSceneLocked() {
		f.epValid = false
	}
}

// fbOnDisconnectLocked は切断の直前（流れていないイベントはここで消える）。
func (w *world) fbOnDisconnectLocked() {
	f := w.fb
	if f == nil {
		return
	}
	f.connGen++
	if w.hasPendingAppSceneLocked() {
		f.epBottom, f.epValid = "", false
	}
}

func (w *world) fbOnNewGenLocked() {
	f := w.fb
	if f == nil {
		return
	}
	f.epBottom, f.epValid = "", true
	f.prev = fbPrev{}
	f.consumed = f.appSwitches
	f.keyAt = map[string]time.Time{}
	f.connGen++
	f.snap = nil
}

func (w *world) fbOnConfigLocked() {
	f := w.fb
	if f == nil {
		return
	}
	f.cfgGen++
	f.humanGen++
	w.humanWin = true
	if w.hasPendingAppSceneLocked() {
		f.epValid = false // 古い設定世代の退避の到着は、履歴を足さない
	}
}

// fbMapMutatedLocked は地図が変わったあと（人の操作）。底が消えたか監視対象でなくなったら、その区間は判定しない。
func (w *world) fbMapMutatedLocked() {
	f := w.fb
	if f == nil {
		return
	}
	f.mapGen++
	if f.epBottom != "" && !w.monitoredLocked(f.epBottom) {
		f.epBottom, f.epValid = "", false
	}
}

// fbLateSceneLocked は、次の SceneChanged を期待の期限のあとに届かせることがある（遅着。人の切替として扱われる）。
func (w *world) fbLateSceneLocked() {
	f := w.fb
	if f == nil || w.benign || w.rng.Intn(30) != 0 {
		return
	}
	w.forceDelay = time.Duration(12+w.rng.Intn(4)) * time.Second
	f.epValid = false
	f.count("遅着のシーン切替")
}

// fbBeforeAppSceneLocked は本体の SetScene を適用する直前の判定（入場の独立の判定、同じ要求の5秒、RestartMedia の共有）。
func (w *world) fbBeforeAppSceneLocked(from, to string) {
	f := w.fb
	if f == nil {
		return
	}
	var snap Snapshot
	if f.snap != nil {
		snap = f.snap()
	}
	toName := w.sceneNameLocked(to)
	inStack := slices.Contains(snap.OBS.Standby.Stack, toName)
	key := fmt.Sprintf("%s>%s|%s|h%d c%d m%d n%d", from, to, strings.Join(snap.OBS.Standby.Stack, ","), f.humanGen, f.cfgGen, f.mapGen, f.connGen)
	now := w.clk.Now()
	if t, ok := f.keyAt[key]; ok && now.Sub(t) < 5*time.Second && !w.faulty {
		w.violate("FB-INV-08: 同じ要求（%s → %s）を %v 後にまた送った（最短5秒のはず）", w.sceneNameLocked(from), toName, now.Sub(t))
	}
	f.keyAt[key] = now
	if inStack {
		f.count("戻る切替")
	} else if w.monitoredLocked(to) {
		f.count("監視対象への逃がす切替")
	} else if to == standby {
		f.count("待機への切替")
	} else {
		f.count("画像シーンへの切替")
	}
	if inStack && w.monitoredLocked(to) {
		if shown := w.inputsShownLocked(w.program); len(shown) > 0 && w.visibleLocked(w.program, false, nil)[wMain] {
			if tshown := w.inputsShownLocked(to); tshown["IRL"] && shown["IRL"] {
				f.count("共有入力のまま戻る")
			}
		}
	}
	if !w.autoOn || !w.streaming || !w.connected || w.studio || w.humanPendingLocked() || w.humanWin {
		return
	}
	if !w.monitoredLocked(to) {
		return
	}
	// 独立の Judge と status の状態が食い違うときは、違反にしない（どちらの見方でも違反のときだけ）。
	either := func(id string) bool { return w.stateOKLocked(id) || snapOK(snap, id) }
	both := func(id string) bool { return w.stateOKLocked(id) && snapOK(snap, id) }
	if w.badWithLocked(to, either) {
		w.violate("入場: 監視対象 %s は健康が悪い（独立の Judge でも status でも）のに、本体が SetScene を送った（状態 %s）", toName, w.fbStatesLocked())
	}
	if ids := w.exceptUnusable(w.okHiddenRecordWithLocked(to, both), snap); len(ids) > 0 {
		w.violate("入場: 監視対象 %s は OK のカメラ %v が本体の非表示のせいで映っていないのに、本体が SetScene を送った（状態 %s）", toName, ids, w.fbStatesLocked())
	}
	if len(w.visibleLocked(to, false, nil)) == 0 {
		w.violate("入場: 監視対象 %s は実際の表示が0なのに、本体が SetScene を送った", toName)
	}
	if inStack && snap.OBS.Standby.Owner == "auto" && !w.hasPendingAppSceneLocked() {
		if ids := w.reverseWaitWithLocked(to, snap, either); len(ids) > 0 {
			w.violate("入場: 戻り先 %s に悪い hide のカメラ %v が表示されたままなのに、本体が戻した（逆向きの待ち）", toName, ids)
		}
	}
}

// fbAfterAppSceneLocked は本体の SetScene の適用直後。FB-INV-10 の底を更新する。
func (w *world) fbAfterAppSceneLocked(from, to string) {
	f := w.fb
	if f == nil {
		return
	}
	f.appSwitches++
	f.lastSwitch = w.clk.Now()
	if f.epValid && f.epBottom == "" && w.monitoredLocked(from) && from != to {
		f.epBottom = from
	}
}

// fbOnRestartLocked は RestartMediaInputs の要求。固定した入力から、そのとき program に実際に表示されている入力を
// 除いたものを送るはず（0a W-03 / 0c の共有入力）。独立に、standby のカメラの入力であること、重複なし、実表示でないことを見る。
func (w *world) fbOnRestartLocked(inputs []string) {
	f := w.fb
	if f == nil {
		return
	}
	f.count("RestartMedia")
	seen := map[string]bool{}
	shown := w.inputsShownLocked(w.program)
	for _, u := range inputs {
		if seen[u] {
			w.violate("RestartMedia: 入力 %s が重複している: %v", u, inputs)
		}
		seen[u] = true
		cam := w.inputs[u].CameraID
		if cam != wMain && !w.studio && !w.humanWin && !w.humanPendingLocked() {
			// Studio Mode では hide のカメラも standby として再起動の対象になる。hide のカメラの入力は、Show の再起動だけ（カメラが OK のとき。対象は生成時に固定するので、今の表示は見ない）。
			var snap Snapshot
			if f.snap != nil {
				snap = f.snap()
			}
			if !w.stateOKLocked(cam) && !snapOK(snap, cam) {
				w.violate("RestartMedia: standby でないカメラの入力 %s を、カメラ %s が OK でないのに再起動した（状態 %s）: %v", u, cam, w.judges[cam].State(), inputs)
			}
		}
		// 本体の切替のイベントがまだ届いていない間は、本体が見ている今のシーンが古い（遅着の切替）。
		if cam == wMain && shown[u] && !w.humanWin && !w.humanPendingLocked() && !w.hasPendingAppSceneLocked() {
			w.violate("RestartMedia: 今 program %s に実際に表示されている入力 %s を再起動した: %v", w.sceneNameLocked(w.program), u, inputs)
		}
	}
}
