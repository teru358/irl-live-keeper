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
	clk  *fakeClock
	log  *lockedBuf
	base int // 接続した時点で置かれている待ちの数（Aux の待ちなど）
}

// newBounceHarness は program が Live（uuid-live）の接続を作る。studio が true なら、読み取りで Studio Mode オン。
func newBounceHarness(t *testing.T, studio bool) *bounceH {
	t.Helper()
	s := newAuxServer(t)
	f := newFakeOBS()
	f.studio = studio
	s.setHandler(f.handle)
	clk := newFakeClock()
	lb := &lockedBuf{}
	h := startWSHarness(t, s, f, func(w *WS) {
		w.clock = clk
		w.log = slog.New(slog.NewJSONHandler(lb, nil))
	})
	h.awaitConnected(1)
	time.Sleep(100 * time.Millisecond) // 接続の直後の待ちが置かれ終わるのを待つ
	return &bounceH{wsHarness: h, clk: clk, log: lb, base: clk.pending()}
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

// armed は、取っておく窓の待ちが n 個置かれるまで待つ（取っておく側がイベントを処理した印）。
func (h *bounceH) armed(n int) {
	h.t.Helper()
	waitFor(h.t, "窓の待ちが置かれる", func() bool { return h.clk.pending() >= h.base+n })
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
		}
	}
	return fmt.Sprint(out)
}

func (h *bounceH) awaitKinds(want string) {
	h.t.Helper()
	waitFor(h.t, "流れたイベント "+want, func() bool { return h.kinds() == want })
}

// 何も増えないことを、少し待って確かめる。
func (h *bounceH) stays(want string) {
	h.t.Helper()
	time.Sleep(120 * time.Millisecond)
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
	h.clk.Advance(time.Second) // 捨てた窓の待ちが起きても、何も流れない
	h.stays("[studio:false]")
}

// T2: 同じシーンへの切替が2回（A → A → false）。
func TestStudioBounceSameSceneTwice(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("Live")
	h.prog("Live")
	h.studioEvent(false)
	h.awaitKinds("[studio:false]")
	h.stays("[studio:false]")
	if !strings.Contains(h.log.String(), `"scenes":["Live","Live"]`) {
		t.Fatalf("ログ: %s", h.log.String())
	}
}

// T3: X だけ。窓の終わりまで流れず、終わったら届いた時刻のまま流れる。
func TestStudioProgramChangeFlowsAfterWindow(t *testing.T) {
	h := newBounceHarness(t, true)
	before := time.Now()
	h.prog("X")
	after := time.Now()
	h.armed(1)
	h.clk.Advance(249 * time.Millisecond)
	h.stays("[]")
	h.clk.Advance(1 * time.Millisecond)
	h.awaitKinds("[scene:X]")
	sc, _ := lastOf[switcher.SceneChanged](h.events())
	// 届いた時刻。窓の終わりまで待った時間（偽の時計なので実時間はほぼ進まない）より前。
	if sc.At.Before(before) || sc.At.After(after.Add(2*time.Second)) {
		t.Fatalf("At = %v（%v〜%v）", sc.At, before, after)
	}
	if time.Since(sc.At) < 100*time.Millisecond {
		t.Fatalf("At が流した時刻になっている（stays の待ちより新しい）: %v", sc.At)
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
	if h.clk.pending() != h.base {
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
	h.stays("[studio:true studio:false]")
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
	h2.stays("[]")
	h2.clk.Advance(250 * time.Millisecond)
	h2.awaitKinds("[scene:X item:1=false item:2=true]")
}

// T7: 取っておく間に切断したら、何も流れない（古い SceneChanged が OBSDisconnected のあとに出ない）。
func TestStudioBounceDroppedOnDisconnect(t *testing.T) {
	h := newBounceHarness(t, true)
	h.prog("X")
	h.item(1, false)
	h.armed(1)
	h.s.kill(1)
	h.awaitDisconnected(1)
	h.clk.Advance(time.Second)
	time.Sleep(100 * time.Millisecond)
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
	h.armed(1)
	h.s.sendEvent(1, "CurrentSceneCollectionChanging", map[string]any{"sceneCollectionName": "Old"})
	h.awaitDisconnected(1)
	h.clk.Advance(250 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
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
	h.stays("[]")
}

// T9: 窓の終わりと false の境。窓の終わり（250ms）になった時点で流す側が先になり、そのあとに届く false は
// 捨てる対象がない（SceneChanged はすでに流れた）。窓の中（249ms）に届く false は捨てる。
func TestStudioBounceWindowBoundary(t *testing.T) {
	// 窓の終わりのあとに false: 流れたものは取り消さない。
	h := newBounceHarness(t, true)
	h.prog("H")
	h.prog("Live")
	h.armed(1)
	h.clk.Advance(250 * time.Millisecond)
	h.awaitKinds("[scene:H scene:Live]")
	h.studioEvent(false)
	h.awaitKinds("[scene:H scene:Live studio:false]")

	// 窓の終わりの直前（249ms）に false: 捨てる。
	h2 := newBounceHarness(t, true)
	h2.prog("H")
	h2.prog("Live")
	h2.armed(1)
	h2.clk.Advance(249 * time.Millisecond)
	h2.studioEvent(false)
	h2.awaitKinds("[studio:false]")
	h2.clk.Advance(time.Second)
	h2.stays("[studio:false]")
}
