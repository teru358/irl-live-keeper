package obs

import (
	"errors"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 本来の接続の SetScene が応答待ちで Timeout したあとに、遅れて応答が届いたら、
// 接続を捨ててつなぎ直す（OBSDisconnected のあとに OBSConnected）。
func TestMainTimeoutDiscardsClientAndReconnects(t *testing.T) {
	h := newWSHarness(t, nil)
	var n int
	h.f.setHook(func(r auxReq) (auxReply, bool) {
		if r.Type == "SetCurrentProgramScene" {
			n++
			if n == 1 {
				return auxReply{Delay: 700 * time.Millisecond}, true // w.timeout（300ms）より遅い
			}
		}
		return auxReply{}, false
	})
	h.awaitConnected(1)

	start := time.Now()
	err := h.w.SetSceneUUID("A")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("ErrTimeout のはずが %v", err)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("Timeout で戻るのが遅い: %v", el)
	}
	h.awaitDisconnected(1)
	h.awaitConnected(2)
	if got := h.s.accepted(); got < 2 {
		t.Fatalf("つなぎ直していない: 接続は %d 本", got)
	}
	// つなぎ直したあとは、要求がふつうに通る。
	if err := h.w.SetSceneUUID("B"); err != nil {
		t.Fatalf("つなぎ直したあとの SetScene が失敗: %v", err)
	}
	if lastDisc, ok := lastOf[switcher.OBSDisconnected](h.events()); !ok || lastDisc.At.IsZero() {
		t.Fatal("OBSDisconnected に時刻がない")
	}
}

// goobs の制約の確かめ（spec 第3節「接続の扱い」、第8節）: 要求が Timeout したあとに遅れて応答が
// 届くと、受け手のいない応答で goobs の handleOpcodes が止まる。イベントは届かなくなり、次の要求は
// 送る段階（Opcodes への送信）で止まって、応答待ちの期限（Timeout）も効かないまま戻らない。
// 本体はこの Client を使い続けてはならない、という根拠のテスト。
// 止まった goobs の goroutine は外から戻せないので、このテストの Client は Disconnect せず残す。
func TestGoobsLateReplyStallsClient(t *testing.T) {
	s := newAuxServer(t)
	first := true
	s.setHandler(func(r auxReq) auxReply {
		if r.Type == "SetCurrentProgramScene" && first {
			first = false
			return auxReply{Delay: 600 * time.Millisecond}
		}
		return auxReply{}
	})
	cfg := s.config(t)
	cli, rec, err := dialOBS(t.Context(), "127.0.0.1:"+itoa(cfg.Port), "", 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rec.close)

	_, err = cli.Scenes.SetCurrentProgramScene(newSetScene("A"))
	if err == nil || classifyError(err) == err {
		t.Fatalf("Timeout になるはずが %v", err)
	}
	time.Sleep(700 * time.Millisecond) // 遅れた応答が届く（受け手がいない）

	s.sendEvent(1, "CurrentProgramSceneChanged", map[string]any{"sceneName": "X", "sceneUuid": "ux"})
	select {
	case ev := <-cli.IncomingEvents:
		t.Fatalf("goobs は止まるはずが、イベントが届いた: %T", ev)
	case <-time.After(500 * time.Millisecond):
	}
	next := make(chan error, 1)
	go func() { _, err := cli.Scenes.GetSceneList(); next <- err }()
	select {
	case err := <-next:
		t.Fatalf("次の要求は戻らないはずが、戻った: %v", err)
	case <-time.After(1500 * time.Millisecond): // 応答待ちの期限（300ms）の5倍
	}
}
