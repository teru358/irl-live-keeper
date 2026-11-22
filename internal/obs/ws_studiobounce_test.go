package obs

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// F-9: Studio Mode を切るときの一瞬の切替を、obs の層で捨てる。

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type bounceH struct {
	*wsHarness
	clk *fakeClock
	log *lockedBuf
}

// newBounceHarness は program が Live（uuid-live）の接続を作る。studio が true なら、読み取りで Studio Mode オン。
func newBounceHarness(t *testing.T, studio bool) *bounceH {
	return newBounceHarnessOpt(t, studio, false)
}

// withMap が true なら、地図の読み取りと読み直し（補助の接続）が動く世界にする。
func newBounceHarnessOpt(t *testing.T, studio, withMap bool) *bounceH {
	t.Helper()
	s := newAuxServer(t)
	f := newFakeOBS()
	if withMap {
		mapWorld(f)
	}
	f.studio = studio
	s.setHandler(f.handle)
	clk := newFakeClock()
	lb := &lockedBuf{}
	h := startWSHarness(t, s, f, func(w *WS) {
		if withMap {
			mapTune(w)
		}
		w.clock = clk
		w.log = slog.New(slog.NewJSONHandler(lb, nil))
	})
	h.awaitConnected(1)
	if withMap {
		waitFor(t, "補助の接続が開く", func() bool { return s.accepted() == 2 })
		waitFor(t, "30秒の待ちが置かれる", func() bool { return clk.pending() >= 1 })
	}
	return &bounceH{wsHarness: h, clk: clk, log: lb}
}

func (h *bounceH) prog(name string) {
	h.s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": name, "sceneUuid": "uuid-" + strings.ToLower(name)})
}

func (h *bounceH) studioEvent(on bool) {
	h.s.sendEvent(1, "StudioModeStateChanged", map[string]any{"studioModeEnabled": on})
}

func (h *bounceH) item(id int, on bool) {
	h.s.sendEvent(1, "SceneItemEnableStateChanged", map[string]any{"sceneUuid": "uuid-live", "sceneItemId": id, "sceneItemEnabled": on})
}

// armed は、取っておく窓の待ち（bounce の窓だけ。ほかのタイマーは数えない）が n 回置かれるまで待つ。
func (h *bounceH) armed(n int) {
	h.t.Helper()
	waitFor(h.t, "窓の待ちが置かれる", func() bool { return h.w.bounceArmed.Load() >= int64(n) })
}

// buffered は、取っておいたイベントが n 個になるまで待つ（session がそこまで処理した印）。
func (h *bounceH) buffered(n int) {
	h.t.Helper()
	waitFor(h.t, "イベントが取っておかれる", func() bool { return h.w.bounceBuffered.Load() >= int64(n) })
}

// kinds は、接続のあとに流れたイベントを短い文字列にしたもの。
func (h *bounceH) kinds() string {
	var out []string
	for _, e := range h.events() {
		switch v := e.(type) {
		case switcher.SceneChanged:
			out = append(out, "scene:"+v.Scene)
		case switcher.StudioModeChanged:
			out = append(out, fmt.Sprint("studio:", v.Enabled))
		case switcher.ItemEnabledChanged:
			out = append(out, fmt.Sprintf("item:%d=%v", v.ItemID, v.Enabled))
		case switcher.TransitionChanged:
			if v.TransitionMs != nil {
				out = append(out, fmt.Sprintf("trans:%d", *v.TransitionMs))
			}
		case switcher.SceneMapChanged:
			out = append(out, "map")
		}
	}
	return fmt.Sprint(out)
}

func (h *bounceH) awaitKinds(want string) {
	h.t.Helper()
	waitFor(h.t, "流れたイベント "+want, func() bool { return h.kinds() == want })
}

// 今までに流れたものが want のままであることを確かめる（待たない。呼ぶ前に buffered などで、
// session が処理し終わったことを確かめておく）。
func (h *bounceH) stays(want string) {
	h.t.Helper()
	if got := h.kinds(); got != want {
		h.t.Fatalf("流れたイベント = %s, want %s", got, want)
	}
}

// T1: A → H → A → false。SceneChanged は流れず、StudioModeChanged{false} だけ流れる。
func TestStudioBounceDiscardedWhenBackToOriginal(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("H")
	h.prog("Live")
	h.studioEvent(false)
	h.awaitKinds("[studio:false]")
	h.stays("[studio:false]")
	if !strings.Contains(h.log.String(), `"msg":"studio mode off bounce ignored"`) || !strings.Contains(h.log.String(), `"scenes":["H","Live"]`) {
		t.Fatalf("ログ: %s", h.log.String())
	}
	// 捨てた窓の待ちが起きても、何も流れない。次の切替（Studio Mode はもうオフ）がすぐ流れる印で、
	// session が待ちを処理し終えたあとの列を見る。
	h.clk.Advance(time.Second)
	h.prog("Z")
	h.awaitKinds("[studio:false scene:Z]")
}

// T2: 同じシーンへの切替が2回（A → A → false）。
func TestStudioBounceSameSceneTwice(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("Live")
	h.prog("Live")
	h.studioEvent(false)
	h.awaitKinds("[studio:false]")
	if !strings.Contains(h.log.String(), `"scenes":["Live","Live"]`) {
		t.Fatalf("ログ: %s", h.log.String())
	}
}

// T3: X だけ。窓の終わりまで流れず、終わったら届いた時刻のまま流れる。
func TestStudioProgramChangeFlowsAfterWindow(t *testing.T) {
	h := newBounceHarness(t, true)
	before := time.Now()
	h.prog("X")
	h.armed(1)
	h.buffered(1)
	h.clk.Advance(249 * time.Millisecond)
	h.stays("[]")
	flushAt := time.Now()
	h.clk.Advance(1 * time.Millisecond)
	h.awaitKinds("[scene:X]")
	sc, _ := lastOf[switcher.SceneChanged](h.events())
	// 届いた時刻（送ったあとから、時計を進める前まで）。流した時刻ではない。
	if sc.At.Before(before) || sc.At.After(flushAt) {
		t.Fatalf("At = %v（%v〜%v）", sc.At, before, flushAt)
	}
}

// T4: X → false。最後が前と違うので、SceneChanged X を1つと StudioModeChanged{false}。
func TestStudioBounceEndingElsewhereFlowsLastOnly(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("H")
	h.prog("X")
	h.studioEvent(false)
	h.awaitKinds("[scene:X studio:false]")
	if strings.Contains(h.log.String(), "bounce ignored") {
		t.Fatalf("捨てていないのにログが出た: %s", h.log.String())
	}
}

// T5: Studio Mode オフでは、すぐ流れる。
func TestProgramChangeImmediateWhenStudioOff(t *testing.T) {
	h := newBounceHarness(t, false)
	h.prog("X")
	h.awaitKinds("[scene:X]")
	if n := h.w.bounceArmed.Load(); n != 0 {
		t.Fatal("取っておく窓が置かれた")
	}
}

// Studio Mode のオンを StudioModeStateChanged で知ったときも取っておく（T1 の形）。
func TestStudioBounceAfterStudioOnEvent(t *testing.T) {
	h := newBounceHarness(t, false)
	h.studioEvent(true)
	h.awaitKinds("[studio:true]")
	h.prog("H")
	h.prog("Live")
	h.studioEvent(false)
	h.awaitKinds("[studio:true studio:false]")
}

// T6: 取っておく間の SceneItemEnableStateChanged は、順を保って流れる（捨てる形と、窓が終わる形）。
func TestStudioBounceKeepsItemOrder(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("H")
	h.item(1, false)
	h.prog("Live")
	h.item(2, true)
	h.studioEvent(false)
	h.awaitKinds("[item:1=false item:2=true studio:false]")

	h2 := newBounceHarness(t, true)
	h2.prog("X")
	h2.item(1, false)
	h2.item(2, true)
	h2.armed(1)
	h2.buffered(3)
	h2.stays("[]")
	h2.clk.Advance(250 * time.Millisecond)
	h2.awaitKinds("[scene:X item:1=false item:2=true]")
}

// T7: 取っておく間に切断したら、何も流れない（古い SceneChanged が OBSDisconnected のあとに出ない）。
func TestStudioBounceDroppedOnDisconnect(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("X")
	h.item(1, false)
	h.buffered(2)
	h.s.kill(1)
	h.awaitDisconnected(1)
	h.clk.Advance(time.Second)
	if got := h.kinds(); got != "[]" {
		t.Fatalf("切断のあとに流れた: %s", got)
	}
	last := h.events()[len(h.events())-1]
	if _, ok := last.(switcher.OBSDisconnected); !ok {
		t.Fatalf("最後は OBSDisconnected のはず: %T", last)
	}
}

// 取っておく間にコレクションが切り替わったら、捨てる。
func TestStudioBounceDroppedOnCollectionSwitch(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("X")
	h.buffered(1)
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	h.clk.Advance(250 * time.Millisecond)
	time.Sleep(100 * time.Millisecond) // 何も流れないことの確認なので、待つしかない
	if got := h.kinds(); got != "[]" {
		t.Fatalf("切替のあとに流れた: %s", got)
	}
}

// T8: 読み取りで Studio Mode が true のときから取っておく（T1 の形は newBounceHarness(true) で済んでいる）。
// 接続の読み取りが true で、その後の StudioModeStateChanged がなくても、取っておく。
func TestStudioFromSnapshotHoldsFirstProgramChange(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("X")
	h.armed(1)
	h.buffered(1)
	h.stays("[]")
}

// T9: 窓の終わりと false の境。窓の終わり（250ms）になった時点で流す側が先になり、そのあとに届く false は
// 捨てる対象がない（SceneChanged はすでに流れた）。窓の中（249ms）に届く false は捨てる。
func TestStudioBounceWindowBoundary(t *testing.T) {
	// 窓の終わりのあとに false: 流れたものは取り消さない。
	h := newBounceHarness(t, true)
	h.prog("H")
	h.prog("Live")
	h.buffered(2)
	h.clk.Advance(250 * time.Millisecond)
	h.awaitKinds("[scene:H scene:Live]")
	h.studioEvent(false)
	h.awaitKinds("[scene:H scene:Live studio:false]")

	// 窓の終わりの直前（249ms）に false: 捨てる。
	h2 := newBounceHarness(t, true)
	h2.prog("H")
	h2.prog("Live")
	h2.buffered(2)
	h2.clk.Advance(249 * time.Millisecond)
	h2.studioEvent(false)
	h2.awaitKinds("[studio:false]")
	h2.clk.Advance(time.Second)
	h2.prog("Z")
	h2.awaitKinds("[studio:false scene:Z]")
}

// I-1(a): 取っておく間に TransitionDurationChanged と CurrentSceneTransitionChanged が届く。
// 取っておいたものが先に流れ、取り直しの結果（エポックの goroutine）がそのあとに流れる。
func TestStudioBounceFlushedBeforeTransitionRefetch(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("X")
	h.s.sendEvent(1, "CurrentSceneTransitionDurationChanged", map[string]any{"transitionDuration": 100})
	h.buffered(2)
	h.s.sendEvent(1, "CurrentSceneTransitionChanged", map[string]any{"transitionName": "Fade"})
	// 取り直しの結果は偽の OBS の 300ms
	h.awaitKinds("[scene:X trans:100 trans:300]")
}

// I-1(b): 取っておいた ItemEnabledChanged が、SceneItemCreated による SceneMapChanged より先に流れる。
func TestStudioBounceFlushedBeforeMapRefresh(t *testing.T) {
	h := newBounceHarnessOpt(t, true, true)
	h.prog("X")
	h.item(1, false)
	h.buffered(2)
	p := h.clk.pending()
	h.s.sendEvent(1, "SceneItemCreated", map[string]any{"sceneName": "Live", "sceneUuid": "uuid-live", "sceneItemId": 9, "sourceName": "x"})
	h.awaitKinds("[scene:X item:1=false]")
	waitFor(t, "まとめる待ちが置かれる", func() bool { return h.clk.pending() > p })
	h.clk.Advance(300 * time.Millisecond)
	h.awaitKinds("[scene:X item:1=false map]")
}

// 取っておく間の program でない SceneRemoved も、先に流してから扱う。
func TestStudioBounceFlushedBeforeSceneRemoved(t *testing.T) {
	h := newBounceHarnessOpt(t, true, true)
	h.prog("X")
	h.buffered(1)
	h.s.sendEvent(1, "SceneRemoved", map[string]any{"sceneName": "Wait", "sceneUuid": "uuid-wait", "isGroup": false})
	h.awaitKinds("[scene:X]")
}
