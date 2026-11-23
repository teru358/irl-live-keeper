package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 配信中の監視対象のシーンで受信が0に落ちると、待機シーンへの切替を送り、理由をログに残す。
// 待機シーンへ届いたら、持ち主が auto、戻り先が切替前のシーンになる。
func TestEndToEndAutoFail(t *testing.T) {
	h := newHarness(t)
	h.start()

	h.samplesN(5, goodKbps)
	h.waitSnap("OK", linkIs("OK"))
	h.connect(true, camera)
	h.waitSnap("camera", sceneIs(camera))

	h.samplesN(3, 0) // 0 が3秒続いて LOW
	h.waitCall("scene:" + standby)
	h.waitSeen("BatchDone{Completed}", isBatchDone(switcher.BatchCompleted))
	s := h.waitSnap("自動の待機", both(sceneIs(standby), standbyIs("auto", camera)))
	if s.OBS.SceneClass != "standby" {
		t.Errorf("scene_class = %q, want standby", s.OBS.SceneClass)
	}

	want := `msg="scene switch" from=Camera to=Standby reason=low camera=main kbps=0`
	if out := h.log.String(); !strings.Contains(out, want) {
		t.Errorf("ログに %q がない:\n%s", want, out)
	}
	if got := countPrefix(h.obs.Calls(), "scene:"); got != 1 {
		t.Errorf("SetScene の回数 = %d, want 1。calls=%v", got, h.obs.Calls())
	}
}

// 自動の待機のまま受信が戻ると、RestartMedia のあとに SetScene(戻り先) を送る。
// 最後の SceneRequestDone は BatchDone より先に届く。戻り先へ届いたら持ち主と戻り先が消える。
func TestRecoveryRestartsMediaThenReturnScene(t *testing.T) {
	h := newHarness(t)
	h.start()
	n := h.autoStandby(camera)

	h.samplesN(5, goodKbps) // 回復の窓を満たして OK
	seen := h.waitSeen("BatchDone{Completed}", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("戻り先", both(sceneIs(camera), standbyIs("none", "")))

	want := []string{"restart:IRL", "override:Camera", "scene:Camera"}
	if got := callsSince(h.obs, n); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	reqDone := slices.IndexFunc(seen, func(v any) bool {
		d, ok := v.(switcher.SceneRequestDone)
		return ok && d.Result == switcher.RequestOK
	})
	if reqDone < 0 || reqDone >= len(seen)-1 {
		t.Errorf("SceneRequestDone{OK} が BatchDone より前にない: %#v", seen)
	}
	if !strings.Contains(h.log.String(), `msg="scene switch" from=Standby to=Camera reason=ok camera=main kbps=3000`) {
		t.Errorf("復帰の切替のログがない:\n%s", h.log.String())
	}
}

// IRL Link で、回線が悪いまま別の監視対象のシーンを選ぶと、すぐ待機シーンへ戻され、
// 回復したら最後に選んだシーンへ戻る。
func TestExternalSceneChoiceBecomesReturnScene(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.autoStandby(camera)

	h.obsEvent(switcher.SceneChanged{At: h.clk.Now(), Scene: camera2, SceneUUID: camera2})
	h.waitSeen("2本目の AutoFail の BatchDone", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("Camera2 を戻り先にした待機", both(sceneIs(standby), standbyIs("auto", camera2)))
	n := len(h.obs.Calls())

	h.samplesN(5, goodKbps)
	h.waitSeen("AutoRecover の BatchDone", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("Camera2", both(sceneIs(camera2), standbyIs("none", "")))
	want := []string{"restart:IRL", "override:Camera2", "scene:Camera2"}
	if got := callsSince(h.obs, n); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

// 回線が悪い間に、管理外のシーンから監視対象のシーンへ人が切り替えると、待機シーンへ切り替え、
// そのシーンが戻り先になる。
func TestManualEntryIntoMonitoredWhileBad(t *testing.T) {
	h := newHarness(t)
	h.start()

	h.connect(true, talk) // 受信は初期値の DOWN
	h.waitSnap("talk", sceneIs(talk))
	if n := len(h.obs.Calls()); n != 0 {
		t.Fatalf("管理外のシーンで操作した: %v", h.obs.Calls())
	}
	h.obsEvent(switcher.SceneChanged{At: h.clk.Now(), Scene: camera, SceneUUID: camera})
	h.waitCall("scene:" + standby)
	h.waitSeen("BatchDone{Completed}", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("Camera を戻り先にした待機", both(sceneIs(standby), standbyIs("auto", camera)))
}

// 配信開始時の切替が有効なら、配信が始まったときに受信状態によらず開始時のシーンへ切り替える。
func TestOnStreamStartSwitchesToStartScene(t *testing.T) {
	h := newHarness(t, withStartScene(opening))
	h.start()

	h.connect(false, camera) // 受信は初期値の DOWN
	h.waitSnap("camera", sceneIs(camera))
	h.obsEvent(switcher.StreamStateChanged{At: h.clk.Now(), Streaming: true})
	h.waitSeen("StartScene の BatchDone", isBatchDone(switcher.BatchCompleted))
	s := h.waitSnap("opening", sceneIs(opening))
	if s.OBS.SceneClass != "other" || !s.OBS.Streaming {
		t.Errorf("Snapshot = %+v", s.OBS)
	}
	want := []string{"override:Opening", "scene:Opening"}
	if got := h.obs.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if !strings.Contains(h.log.String(), `msg="scene switch" from=Camera to=Opening reason=stream_start`) {
		t.Errorf("開始時の切替のログがない:\n%s", h.log.String())
	}
}

// RestartMedia の待ちの間に受信が悪化したら、SetScene(戻り先) の承認が却下され、送らない。
func TestApproveRejectsWhenLinkDropsDuringRestart(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.obs.onRestart = func(string) error { h.obs.gate(release); return nil } // 復帰で初めて呼ばれる
	h.start()
	n := h.autoStandby(camera)

	h.samplesN(5, goodKbps)
	h.waitCall("restart:IRL") // 実行役は RestartMedia の中で止まっている

	h.samplesN(3, 0)
	h.waitSnap("LOW", linkIs("LOW"))
	close(release)

	seen := h.waitSeen("BatchDone{Rejected}", isBatchDone(switcher.BatchRejected))
	rejected := slices.ContainsFunc(seen, func(v any) bool {
		a, ok := v.(approval)
		_, rej := a.Result.(switcher.Reject)
		return ok && rej && a.Action == switcher.SetScene{SceneUUID: camera}
	})
	if !rejected {
		t.Errorf("SetScene(Camera) の承認が却下されていない: %#v", seen)
	}
	if n := countPrefix(callsSince(h.obs, n), "scene:"); n != 0 {
		t.Errorf("SetScene が送られた: %v", h.obs.Calls())
	}
	if s := h.core.Snapshot(); s.OBS.Scene != standby || s.OBS.Standby.Owner != "auto" || s.LastError != nil {
		t.Errorf("Snapshot = %+v（Auto の却下は last_error に残さない）", s)
	}
}

// 手動の待機を戻す StandbyOffMonitored の待ちの間に受信が悪化したら、OFF の計画が変わるので承認で Reject になる
// （0c: auto 化へ置き換えない）。持ち主は manual のまま、last_error に取りやめを残す。先に並んでいた StopStream がそのまま次に実行される。
func TestOffRejectedWhenPlanChanges(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.obs.onRestart = func(string) error { h.obs.gate(release); return nil } // StandbyOff で初めて呼ばれる
	h.start()
	n := h.manualStandby(camera)

	if err, _ := h.command(switcher.CmdStandbyOff); err != nil {
		t.Fatalf("StandbyOff: %v", err)
	}
	h.waitCall("restart:IRL")
	if err, _ := h.command(switcher.CmdStreamStop); err != nil {
		t.Fatalf("StreamStop: %v", err)
	}
	h.samplesN(3, 0)
	h.waitSnap("LOW", linkIs("LOW"))
	close(release)

	seen := h.waitSeen("StopStream の BatchDone", func(v any) bool {
		bd, ok := v.(switcher.BatchDone)
		return ok && bd.BatchID == 3 // StandbyOn=1、StandbyOff=2、StopStream=3
	})
	if !slices.ContainsFunc(seen, func(v any) bool {
		a, ok := v.(approval)
		_, isSet := a.Action.(switcher.SetScene)
		return ok && isSet && reflect.DeepEqual(a.Result, switcher.Reject{})
	}) {
		t.Errorf("OFF の SetScene の Reject がない: %#v", seen)
	}
	want := []string{"restart:IRL", "override:Camera", "stop"}
	if got := callsSince(h.obs, n); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	s := h.core.Snapshot()
	if s.LastError == nil || !strings.Contains(i18n.Render(i18n.JA, s.LastError.Message), "取りやめた") {
		t.Errorf("手動の OFF の却下を last_error に残す: %+v", s.LastError)
	}
	if !standbyIs("manual", camera)(s) || s.OBS.Scene != standby {
		t.Errorf("Snapshot = %+v", s.OBS)
	}
}

// 実行役が SetScene で詰まっている間も、中核ループは Sample と Command を受け続ける。
func TestNoDeadlockWhileExecutorBlocked(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.obs.onScene = func(string) error {
		select {
		case <-release:
		case <-time.After(3 * time.Second):
		case <-h.obs.done:
		}
		return nil
	}
	h.start()

	h.connect(true, camera) // 受信は初期値の DOWN なので、すぐ AutoFail
	h.waitCall("scene:" + standby)

	commands := []struct {
		kind    switcher.CommandKind
		invalid bool
	}{
		{switcher.CmdStandbyOn, true}, // 切替の実行中
		{switcher.CmdStreamStop, false},
		{switcher.CmdStreamStop, true},  // 同じ操作が実行待ち
		{switcher.CmdStreamStart, true}, // 配信中
		{switcher.CmdStandbyOff, true},  // 手動の待機中でない
	}
	for i := range 50 {
		h.sample(float64(100 * (i + 1)))
		if i%10 == 9 {
			c := commands[i/10]
			err, took := h.command(c.kind)
			if took > 100*time.Millisecond {
				t.Errorf("Command %v に %v かかった", c.kind, took)
			}
			var inv switcher.ErrInvalidState
			if c.invalid != errors.As(err, &inv) || (!c.invalid && err != nil) {
				t.Errorf("Command %v: err = %v, want invalid=%v", c.kind, err, c.invalid)
			}
			kbps := float64(100 * (i + 1))
			h.waitSnap(fmt.Sprintf("kbps=%v", kbps), func(s Snapshot) bool { return s.Cameras[0].Kbps == kbps })
		}
	}
	if n := countPrefix(h.obs.Calls(), "scene:"); n != 1 {
		t.Errorf("詰まっている間に別の SetScene が送られた: %v", h.obs.Calls())
	}
	close(release)
	// 詰まりが解けたら、並んでいたバッチに進む。
	h.waitCall("stop")
}

// 送った切替のシーン変更のイベントが来ない（打ち切られたトランジションなど）。期待が残る間は
// 再送せず、期限を過ぎたら Tick で再送する。再送の切替が届けば自動の待機になる。
func TestMissingSceneEventResendsAfterDeadline(t *testing.T) {
	h := newHarness(t)
	var first atomic.Bool
	first.Store(true)
	h.obs.onScene = func(name string) error {
		if !first.Swap(false) {
			h.obs.emit(switcher.SceneChanged{At: h.clk.Now(), Scene: name, SceneUUID: name})
		}
		return nil
	}
	h.start()

	h.connect(true, camera) // 受信は初期値の DOWN なので、すぐ AutoFail
	h.waitCall("scene:" + standby)
	h.waitSeen("1本目の BatchDone", isBatchDone(switcher.BatchCompleted))
	if s := h.core.Snapshot(); s.OBS.Scene != camera || s.OBS.Standby.Owner != "none" {
		t.Fatalf("イベントが来ていないのに状態が変わった: %+v", s.OBS)
	}

	// 長さ不明のトランジションなので期限は応答から10秒。5秒の間隔は過ぎたが、期待が残る。
	h.tick(h.clk.Advance(6 * time.Second))
	h.waitSeen("Tick", func(v any) bool { _, ok := v.(switcher.Tick); return ok })
	if n := countPrefix(h.obs.Calls(), "scene:"); n != 1 {
		t.Fatalf("期待が残る間に再送した: %v", h.obs.Calls())
	}

	h.tick(h.clk.Advance(6 * time.Second))
	h.waitSeen("2本目の BatchDone", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("自動の待機", both(sceneIs(standby), standbyIs("auto", camera)))
	if n := countPrefix(h.obs.Calls(), "scene:"); n != 2 {
		t.Errorf("SetScene の回数 = %d, want 2。calls=%v", n, h.obs.Calls())
	}
}

// RestartMedia が失敗しても SetScene(戻り先) は送り、バッチは Completed で Err を持つ。
func TestRestartFailureContinuesToSetScene(t *testing.T) {
	h := newHarness(t)
	h.obs.onRestart = func(string) error { return errors.New("input not found") }
	h.start()
	n := h.autoStandby(camera)

	h.samplesN(5, goodKbps)

	seen := h.waitSeen("BatchDone", func(v any) bool { _, ok := v.(switcher.BatchDone); return ok })
	bd := seen[len(seen)-1].(switcher.BatchDone)
	if bd.Result != switcher.BatchCompleted || bd.Err == nil || !strings.Contains(bd.Err.Error(), "input not found") {
		t.Errorf("BatchDone = %+v", bd)
	}
	want := []string{"restart:IRL", "override:Camera", "scene:Camera"}
	if got := callsSince(h.obs, n); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	s := h.waitSnap("camera と last_error", func(s Snapshot) bool { return sceneIs(camera)(s) && s.LastError != nil })
	if e := s.LastError; e == nil || e.Source != SourceSwitcher || !strings.Contains(i18n.Render(i18n.JA, e.Message), "input not found") {
		t.Errorf("last_error = %+v", e)
	}
}

// SetScene の失敗は SceneRequestDone{Error} を返し、残りを捨てる。
func TestSetSceneFailureEndToEnd(t *testing.T) {
	h := newHarness(t)
	h.obs.onScene = func(string) error { return errors.New("scene not found") }
	h.start()

	h.connect(true, camera)
	seen := h.waitSeen("BatchDone{Failed}", isBatchDone(switcher.BatchFailed))
	if !slices.ContainsFunc(seen, func(v any) bool {
		d, ok := v.(switcher.SceneRequestDone)
		return ok && d.Result == switcher.RequestError
	}) {
		t.Errorf("SceneRequestDone{Error} がない: %#v", seen)
	}
	s := h.waitSnap("last_error", func(s Snapshot) bool { return s.LastError != nil })
	if e := s.LastError; e.Source != SourceSwitcher || !strings.Contains(i18n.Render(i18n.JA, e.Message), "scene not found") || !e.At.Equal(h.clk.Now()) {
		t.Errorf("last_error = %+v", e)
	}
}

// 実行役だけを動かし、テストが中核ループの側を受け持つ。
func TestExecutorBatchRules(t *testing.T) {
	boom := errors.New("boom")
	timeout := fmt.Errorf("%w: deadline", obs.ErrTimeout)
	setFail := switcher.SetScene{SceneUUID: standby}
	restart := switcher.RestartMedia{Inputs: []string{"IRL"}, Wait: time.Millisecond}
	cases := []struct {
		name      string
		actions   []switcher.Action
		approve   []switcher.ApproveResult // 承認要求ごとの返事
		sceneErr  error
		restErr   error
		wantCalls []string
		wantReq   []switcher.RequestResult
		wantRes   switcher.BatchResult
		wantErr   error
	}{
		{
			name:     "SetScene の失敗で残りを捨てる",
			actions:  []switcher.Action{setFail, switcher.StopStream{}},
			approve:  []switcher.ApproveResult{switcher.Proceed{ReqID: 3}},
			sceneErr: boom, wantCalls: []string{"override:Standby", "scene:Standby"},
			wantReq: []switcher.RequestResult{switcher.RequestError}, wantRes: switcher.BatchFailed, wantErr: boom,
		},
		{
			name:     "SetScene のタイムアウトは Timeout で、残りを捨てる",
			actions:  []switcher.Action{setFail, switcher.StopStream{}},
			approve:  []switcher.ApproveResult{switcher.Proceed{ReqID: 3}},
			sceneErr: timeout, wantCalls: []string{"override:Standby", "scene:Standby"},
			wantReq: []switcher.RequestResult{switcher.RequestTimeout}, wantRes: switcher.BatchFailed, wantErr: obs.ErrTimeout,
		},
		{
			name:    "StopStream の失敗で残りを捨てる",
			actions: []switcher.Action{switcher.StopStream{}, setFail},
			approve: []switcher.ApproveResult{switcher.Proceed{}},
			restErr: nil, wantCalls: []string{"stop"},
			wantRes: switcher.BatchFailed, wantErr: boom,
		},
		{
			name:    "RestartMedia の失敗では続け、Completed で Err を持つ",
			actions: []switcher.Action{restart, setFail},
			approve: []switcher.ApproveResult{switcher.Proceed{}, switcher.Proceed{ReqID: 9}},
			restErr: boom, wantCalls: []string{"restart:IRL", "override:Standby", "scene:Standby"},
			wantReq: []switcher.RequestResult{switcher.RequestOK}, wantRes: switcher.BatchCompleted, wantErr: boom,
		},
		{
			name:      "Skip は送らずに次へ進む",
			actions:   []switcher.Action{setFail, switcher.StopStream{}},
			approve:   []switcher.ApproveResult{switcher.Skip{}, switcher.Proceed{}},
			wantCalls: []string{"override:Standby", "stop"}, wantRes: switcher.BatchCompleted,
		},
		{
			name:      "Reject で残りを捨てる",
			actions:   []switcher.Action{restart, setFail},
			approve:   []switcher.ApproveResult{switcher.Proceed{}, switcher.Reject{}},
			wantCalls: []string{"restart:IRL", "override:Standby"}, wantRes: switcher.BatchRejected,
		},
		{
			name:      "Replace で残りを捨てる",
			actions:   []switcher.Action{restart, setFail},
			approve:   []switcher.ApproveResult{switcher.Proceed{}, switcher.Replace{Batch: &switcher.Batch{BatchID: 99}}},
			wantCalls: []string{"restart:IRL", "override:Standby"}, wantRes: switcher.BatchRejected,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.obs.onScene = func(string) error { return tc.sceneErr }
			h.obs.onRestart = func(string) error { return tc.restErr }
			h.obs.onStop = func() error {
				if tc.name == "StopStream の失敗で残りを捨てる" {
					return boom
				}
				return nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				h.core.execute(ctx, &switcher.Batch{BatchID: 7, Purpose: switcher.PurposeStandbyOn, Actions: tc.actions})
			}()

			var reqs []switcher.RequestResult
			var bd *switcher.BatchDone
			n := 0
			for bd == nil {
				select {
				case req := <-h.core.approvals:
					if req.batchID != 7 {
						t.Fatalf("batchID = %d", req.batchID)
					}
					if n >= len(tc.approve) {
						t.Fatalf("承認要求が多すぎる: %#v", req.action)
					}
					req.reply <- tc.approve[n]
					n++
				case ev := <-h.core.results:
					switch e := ev.(type) {
					case switcher.SceneRequestDone:
						if e.ReqID == 0 {
							t.Errorf("ReqID が 0")
						}
						reqs = append(reqs, e.Result)
					case switcher.BatchDone:
						bd = &e
					}
				case <-time.After(wait):
					t.Fatalf("実行役が止まった。calls=%v", h.obs.Calls())
				}
			}
			<-done
			if got := h.obs.Calls(); !reflect.DeepEqual(got, tc.wantCalls) {
				t.Errorf("calls = %v, want %v", got, tc.wantCalls)
			}
			if !reflect.DeepEqual(reqs, tc.wantReq) {
				t.Errorf("SceneRequestDone = %v, want %v", reqs, tc.wantReq)
			}
			if bd.BatchID != 7 || bd.Result != tc.wantRes {
				t.Errorf("BatchDone = %+v, want Result %v", *bd, tc.wantRes)
			}
			if (tc.wantErr == nil) != (bd.Err == nil) || (tc.wantErr != nil && !errors.Is(bd.Err, tc.wantErr)) {
				t.Errorf("BatchDone.Err = %v, want %v", bd.Err, tc.wantErr)
			}
		})
	}
}

// 承認の前に、切替先シーンのトランジションの上書きを読み、obs.ToOverride の形で渡す。
func TestExecutorPassesOverride(t *testing.T) {
	ms := 1500
	cases := []struct {
		ms   *int
		has  bool
		err  error
		want switcher.Override
	}{
		{nil, false, nil, switcher.OverrideNone{}},
		{&ms, true, nil, switcher.OverrideKnown{Ms: 1500}},
		{nil, true, nil, switcher.OverrideUnknown{}},
		{nil, false, errors.New("x"), switcher.OverrideUnknown{}},
	}
	for _, tc := range cases {
		h := newHarness(t)
		h.obs.onOverride = func(string) (*int, bool, error) { return tc.ms, tc.has, tc.err }
		ctx, cancel := context.WithCancel(context.Background())
		go h.core.execute(ctx, &switcher.Batch{BatchID: 1, Actions: []switcher.Action{switcher.SetScene{SceneUUID: camera}}})
		select {
		case req := <-h.core.approvals:
			if !reflect.DeepEqual(req.override, tc.want) {
				t.Errorf("override = %#v, want %#v", req.override, tc.want)
			}
			req.reply <- switcher.Reject{}
		case <-time.After(wait):
			t.Fatal("承認要求が来ない")
		}
		cancel()
	}
}

// Command の応答は cmdErr の型をそのまま返す。
func TestCommandReturnsCmdErr(t *testing.T) {
	h := newHarness(t)
	h.start()
	err, _ := h.command(switcher.CmdStreamStart)
	var nc switcher.ErrNotConnected
	if !errors.As(err, &nc) {
		t.Errorf("未接続の Command: err = %v", err)
	}
	h.connect(false, camera)
	h.waitSnap("connected", sceneIs(camera))
	err, _ = h.command(switcher.CmdStreamStop)
	var inv switcher.ErrInvalidState
	if !errors.As(err, &inv) {
		t.Errorf("配信していない StreamStop: err = %v", err)
	}
	if err, _ := h.command(switcher.CmdStreamStart); err != nil {
		t.Errorf("StreamStart: err = %v", err)
	}
}

// 実行役が OBS の操作の中で止まっていても、ctx の取り消しで Run は1秒以内に戻る。
// 止まっていた操作が戻れば、残った goroutine もなくなる。
func TestCancelWhileExecutorBlockedInOBS(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.obs.onScene = func(string) error { h.obs.gate(release); return nil }
	h.start()

	h.connect(true, camera)
	h.waitCall("scene:" + standby)
	h.cancel()
	select {
	case <-h.runDone:
	case <-time.After(time.Second):
		t.Fatal("Run が1秒以内に戻らない")
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := h.core.Command(ctx, switcher.CmdStandbyOn); !errors.Is(err, ErrStopped) {
		t.Errorf("止まったあとの Command: err = %v", err)
	}
	close(release)
	waitInflightZero(t, h.obs)
}

// RestartMedia の待ちの間でも、ctx の取り消しで Run は戻る。
func TestCancelDuringRestartWait(t *testing.T) {
	h := newHarness(t, withRestartWait(time.Hour))
	h.start()
	h.autoStandby(camera)
	h.samplesN(5, goodKbps)
	h.waitCall("restart:IRL")
	waitInflightZero(t, h.obs)
	h.cancel()
	select {
	case <-h.runDone:
	case <-time.After(time.Second):
		t.Fatal("Run が1秒以内に戻らない")
	}
}

// 実行役が中核ループへの送信で待っている間に止めても、Run は戻る。
func TestCancelWhileExecutorWaitsForCore(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.core.execute(ctx, &switcher.Batch{BatchID: 1, Actions: []switcher.Action{switcher.StopStream{}}})
	}()
	// 承認要求には誰も答えない。
	time.AfterFunc(10*time.Millisecond, cancel)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("実行役が抜けない")
	}
	if n := len(h.obs.Calls()); n != 0 {
		t.Errorf("承認なしで送った: %v", h.obs.Calls())
	}
}

func waitInflightZero(t *testing.T, f *fakeOBS) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for f.inflight.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("偽の OBS の操作が戻らない（inflight=%d）", f.inflight.Load())
		}
		time.Sleep(time.Millisecond)
	}
}

// status の JSON の形（カメラの spec 第9節の 0a の分）。sources、hidden、user_shown、unusable、studio_mode を含む（0b）。
const specStatusJSON = `{
  "cameras": [
    {"id": "main", "name": "メイン", "on_bad": "standby", "state": "LOW", "kbps": 4200.5, "online": true, "protocol": "srt",
     "srt": {"rtt_ms": 85.2, "loss_rate": 0.01, "recv_mbps": 4.2},
     "sources": ["IRL"], "hidden": 0, "user_shown": false, "unusable": false},
    {"id": "sub", "name": "サブ", "on_bad": "hide", "state": "DOWN", "kbps": 0, "online": false, "protocol": "", "srt": null,
     "sources": [], "hidden": 0, "user_shown": false, "unusable": false}
  ],
  "obs":     {"connected": true, "streaming": false, "scene": "Camera", "scene_uuid": "Camera", "scene_class": "monitored",
              "scene_cameras": ["main"], "scene_health": "LOW", "studio_mode": false,
              "standby": {"owner": "none", "return_scene": null, "stack": []},
              "auto_enabled": true,
              "warnings": []},
  "mediamtx": "running",
  "urls":    [
    {"id": "main", "name": "メイン", "publish_srt": "srt://x/main", "publish_rtmp": "rtmp://x/main", "obs_read": "rtsp://x/main"},
    {"id": "sub", "name": "サブ", "publish_srt": "srt://x/sub", "publish_rtmp": "rtmp://x/sub", "obs_read": "rtsp://x/sub"}
  ],
  "last_error": null
}`

func twoCameras(_ *switcher.Config, d *Deps) {
	d.Cameras = []Camera{testCamera(mainCam, "メイン", scenemap.MotionStandby), testCamera(subCam, "サブ", scenemap.MotionHide)}
}

func TestSnapshotJSONShape(t *testing.T) {
	var spec map[string]any
	if err := json.Unmarshal([]byte(specStatusJSON), &spec); err != nil {
		t.Fatal(err)
	}

	h := newHarness(t, twoCameras)
	// 起動前の初期値: カメラは DOWN で kbps 0、srt は null。OBS に一度も接続していないので scene_class は unknown、
	// scene_cameras は []、scene_health は null。auto_enabled は Deps の設定から入る。
	initial := marshalMap(t, h.core.Snapshot())
	if !reflect.DeepEqual(keyPaths(initial, ""), keyPaths(spec, "")) {
		t.Errorf("初期値のキー = %v, want %v", keyPaths(initial, ""), keyPaths(spec, ""))
	}
	obsPart := initial["obs"].(map[string]any)
	if w, ok := obsPart["warnings"].([]any); !ok || len(w) != 0 {
		t.Errorf("warnings = %#v, want []", obsPart["warnings"])
	}
	if sc, ok := obsPart["scene_cameras"].([]any); !ok || len(sc) != 0 {
		t.Errorf("scene_cameras = %#v, want []", obsPart["scene_cameras"])
	}
	if v, ok := obsPart["scene_health"]; !ok || v != nil {
		t.Errorf("scene_health = %#v, want null", v)
	}
	wantStandby := map[string]any{"owner": "none", "return_scene": nil, "stack": []any{}}
	if !reflect.DeepEqual(obsPart["standby"], wantStandby) {
		t.Errorf("standby = %#v, want %#v", obsPart["standby"], wantStandby)
	}
	if obsPart["scene_class"] != "unknown" || obsPart["auto_enabled"] != true || obsPart["connected"] != false {
		t.Errorf("obs = %#v", obsPart)
	}
	if initial["last_error"] != nil {
		t.Errorf("last_error = %#v", initial["last_error"])
	}
	cams, _ := initial["cameras"].([]any)
	if len(cams) != 2 {
		t.Fatalf("cameras = %#v", initial["cameras"])
	}
	wantInit := []any{
		map[string]any{"id": "main", "name": "メイン", "on_bad": "standby", "state": "DOWN", "kbps": 0.0, "online": false, "protocol": "", "srt": nil,
			"sources": []any{}, "hidden": 0.0, "user_shown": false, "unusable": false},
		map[string]any{"id": "sub", "name": "サブ", "on_bad": "hide", "state": "DOWN", "kbps": 0.0, "online": false, "protocol": "", "srt": nil,
			"sources": []any{}, "hidden": 0.0, "user_shown": false, "unusable": false},
	}
	if !reflect.DeepEqual(cams, wantInit) {
		t.Errorf("cameras = %#v, want %#v", cams, wantInit)
	}
	if initial["mediamtx"] != "running" || !reflect.DeepEqual(initial["urls"], spec["urls"]) {
		t.Errorf("初期値 = %#v", initial)
	}

	// SRT の Sample のあと: 1組の中のカメラごとの値。sub は送っていない（0 kbps）。
	h.start()
	h.connect(false, camera)
	at := h.clk.Advance(time.Second)
	h.sendSet(ingest.SampleSet{At: at, Cameras: []ingest.Sample{
		{Camera: mainCam, At: at, Interval: time.Second, Online: true, Protocol: "srt", Kbps: 4200.5,
			SRT: &ingest.SRTStats{RTTMs: 85.2, LossRate: 0.01, RecvMbps: 4.2}},
		{Camera: subCam, At: at, Interval: time.Second},
	}})
	h.waitSnap("srt", func(s Snapshot) bool { return s.Cameras[0].SRT != nil && s.OBS.SceneHealth != nil })
	full := marshalMap(t, h.core.Snapshot())
	// 受信は DOWN から good 1回なので LOW（Judge の規則）。
	if !reflect.DeepEqual(keyPaths(full, ""), keyPaths(spec, "")) {
		t.Errorf("キー = %v, want %v", keyPaths(full, ""), keyPaths(spec, ""))
	}
	if !reflect.DeepEqual(full["cameras"], spec["cameras"]) {
		t.Errorf("cameras = %#v, want %#v", full["cameras"], spec["cameras"])
	}
	fo := full["obs"].(map[string]any)
	if !reflect.DeepEqual(fo["scene_cameras"], []any{"main"}) || fo["scene_health"] != "LOW" || fo["scene"] != "Camera" || fo["scene_class"] != "monitored" {
		t.Errorf("obs = %#v", fo)
	}

	// 接続すると分類が入る。
	h.connect(false, talk)
	h.waitSnap("talk", sceneIs(talk))
	fo = marshalMap(t, h.core.Snapshot())["obs"].(map[string]any)
	if fo["scene_class"] != "other" || fo["scene_health"] != nil || len(fo["scene_cameras"].([]any)) != 0 {
		t.Errorf("obs = %#v", fo)
	}

	// last_error はオブジェクトになる。
	h.setMediaMTX(supervisor.Status{State: supervisor.StateRunning, LastMessage: i18n.Text("listen tcp :1935: bind: address already in use"), LastError: "listen tcp :1935: bind: address already in use"})
	h.tick(h.clk.Advance(time.Second))
	h.waitSnap("last_error", func(s Snapshot) bool { return s.LastError != nil })
	le := marshalMap(t, h.core.Snapshot())["last_error"].(map[string]any)
	if le["source"] != "mediamtx" || le["message"] != "listen tcp :1935: bind: address already in use" {
		t.Errorf("last_error = %#v", le)
	}
	if at, err := time.Parse(time.RFC3339, le["at"].(string)); err != nil || !at.Equal(h.clk.Now()) {
		t.Errorf("last_error.at = %#v (%v), want %s", le["at"], err, h.clk.Now())
	}
}

// 戻り先があるときの return_scene はシーン名になる。
func TestSnapshotReturnScene(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.autoStandby(camera)
	obsPart := marshalMap(t, h.core.Snapshot())["obs"].(map[string]any)
	want := map[string]any{"owner": "auto", "return_scene": "Camera", "stack": []any{"Camera"}}
	if !reflect.DeepEqual(obsPart["standby"], want) || obsPart["scene_class"] != "standby" {
		t.Errorf("obs = %#v", obsPart)
	}
}

// last_error は switcher と MediaMTX のうち時刻の新しい方を出す。時刻が同じなら後から気づいた方。
// MediaMTX の記録が空に戻っても消さない。
func TestLastErrorNewestWins(t *testing.T) {
	h := newHarness(t)
	c := h.core
	t1 := h.clk.Now()
	sw := func(at time.Time, msg string) {
		c.noteSwitcherError(&switcher.ErrorInfo{At: at, Msg: msg, Localized: i18n.Text(msg)})
	}
	check := func(step, source, msg string, at time.Time) {
		t.Helper()
		e := c.lastErr
		if e == nil || e.Source != source || i18n.Render(i18n.JA, e.Message) != msg || !e.At.Equal(at) {
			t.Fatalf("%s: last_error = %+v, want %s %q at %s", step, e, source, msg, at)
		}
	}

	c.noteMediaMTXError("bind failed", i18n.Text("bind failed"))
	check("MediaMTX の最初の記録", SourceMediaMTX, "bind failed", t1)

	sw(t1.Add(-time.Second), "older")
	check("それより古い switcher の記録", SourceMediaMTX, "bind failed", t1)

	t2 := h.clk.Advance(time.Second)
	sw(t2, "newer")
	check("新しい switcher の記録", SourceSwitcher, "newer", t2)

	sw(t2, "newer")
	check("同じ switcher の記録", SourceSwitcher, "newer", t2)

	c.noteMediaMTXError("bind failed", i18n.Text("bind failed"))
	check("変わらない MediaMTX の記録", SourceSwitcher, "newer", t2)

	c.noteMediaMTXError("bind failed again", i18n.Text("bind failed again"))
	check("同じ時刻の MediaMTX の記録", SourceMediaMTX, "bind failed again", t2)

	c.noteMediaMTXError("", i18n.Text(""))
	check("空に戻った MediaMTX", SourceMediaMTX, "bind failed again", t2)

	t3 := h.clk.Advance(time.Second)
	c.noteMediaMTXError("bind failed again", i18n.Text("bind failed again"))
	check("空から同じ文に戻った MediaMTX", SourceMediaMTX, "bind failed again", t3)

	sw(t3, "tie")
	check("同じ時刻の switcher の記録", SourceSwitcher, "tie", t3)
}

// 中核ループを通しても、あとから変わった方が出る。
func TestLastErrorThroughLoop(t *testing.T) {
	h := newHarness(t)
	h.obs.onScene = func(string) error { return errors.New("scene not found") }
	h.start()

	h.setMediaMTX(supervisor.Status{State: supervisor.StateRunning, LastMessage: i18n.Text("bind failed"), LastError: "bind failed"})
	h.tick(h.clk.Advance(time.Second))
	h.waitSnap("mediamtx", func(s Snapshot) bool { return s.LastError != nil && s.LastError.Source == SourceMediaMTX })

	h.clk.Advance(time.Second)
	h.connect(true, camera) // 受信は初期値の DOWN なので、すぐ AutoFail。SetScene が失敗する
	h.waitSnap("switcher", func(s Snapshot) bool { return s.LastError != nil && s.LastError.Source == SourceSwitcher })

	h.setMediaMTX(supervisor.Status{State: supervisor.StateRunning, LastMessage: i18n.Text("bind failed 2"), LastError: "bind failed 2"})
	h.tick(h.clk.Advance(time.Second))
	s := h.waitSnap("mediamtx 2", func(s Snapshot) bool {
		return s.LastError != nil && i18n.Render(i18n.JA, s.LastError.Message) == "bind failed 2"
	})
	if !s.LastError.At.Equal(h.clk.Now()) {
		t.Errorf("last_error.at = %s, want %s", s.LastError.At, h.clk.Now())
	}
}

func marshalMap(t *testing.T, s Snapshot) map[string]any {
	t.Helper()
	b, err := json.Marshal(s.Render(i18n.JA))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// nullObjects は null のことがあるオブジェクトの場所と、その中のキー。
var nullObjects = map[string][]string{
	"last_error": {"at", "message", "source"},
}

// keyPaths はオブジェクトのキーを "obs.standby.owner" の形で並べる。配列の中のオブジェクトは、先頭の要素で "cameras[]." の形にする。
// 値が null の場所は、相手がオブジェクトでもキーだけを比べる（last_error は null のことがある）。
// cameras[].srt の中のキーは SRT のときだけなので比べない。
func keyPaths(m map[string]any, prefix string) []string {
	var out []string
	for k, v := range m {
		p := prefix + k
		out = append(out, p)
		if strings.HasSuffix(p, "[].srt") {
			continue
		}
		switch v := v.(type) {
		case map[string]any:
			out = append(out, keyPaths(v, p+".")...)
		case []any:
			if len(v) > 0 {
				if first, ok := v[0].(map[string]any); ok {
					out = append(out, keyPaths(first, p+"[].")...)
				}
			}
		default:
			if keys, ok := nullObjects[p]; ok && v == nil {
				for _, key := range keys {
					out = append(out, p+"."+key)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// C-03: 承認が返した部分集合（Proceed.RestartInputs）だけを再起動する。Skip なら再起動も待ちもしない。
func TestExecuteRestartSubset(t *testing.T) {
	run := func(t *testing.T, restartRes switcher.ApproveResult, wait time.Duration) (calls []string, elapsed time.Duration) {
		h := newHarness(t) // Run は始めない。実行役だけを、承認を台本で返す相手につなぐ
		c := h.core
		h.obs.onScene = func(string) error { return nil } // イベントは流さない
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			for {
				select {
				case req := <-c.approvals:
					switch req.action.(type) {
					case switcher.RestartMedia:
						req.reply <- restartRes
					default:
						req.reply <- switcher.Proceed{ReqID: 1}
					}
				case <-c.results:
				case <-h.obs.entered:
				case <-ctx.Done():
					return
				}
			}
		}()
		b := &switcher.Batch{BatchID: 7, Purpose: switcher.PurposeAutoRecover, Actions: []switcher.Action{
			switcher.RestartMedia{Inputs: []string{"in-a", "in-b"}, Wait: wait},
			switcher.SetScene{SceneUUID: camera},
		}}
		n := len(h.obs.Calls())
		start := time.Now()
		c.execute(ctx, b)
		return callsSince(h.obs, n), time.Since(start)
	}
	t.Run("subset", func(t *testing.T) {
		calls, _ := run(t, switcher.Proceed{RestartInputs: []string{"in-b"}}, time.Millisecond)
		if got := countPrefix(calls, "restart:"); got != 1 || calls[0] != "restart:in-b" && calls[1] != "restart:in-b" {
			t.Fatalf("calls = %v, want restart:in-b だけ", calls)
		}
		for _, c := range calls {
			if strings.Contains(c, "in-a") {
				t.Fatalf("外した入力を再起動した: %v", calls)
			}
		}
	})
	t.Run("fixed_when_no_subset", func(t *testing.T) {
		// 項目の Show の Restart など、RestartInputs を持たない Proceed は固定した入力すべて。
		calls, _ := run(t, switcher.Proceed{}, time.Millisecond)
		if !slices.Contains(calls, "restart:in-a,in-b") {
			t.Fatalf("calls = %v", calls)
		}
	})
	t.Run("skip_no_restart_no_wait", func(t *testing.T) {
		calls, elapsed := run(t, switcher.Skip{}, 5*time.Second)
		if countPrefix(calls, "restart:") != 0 {
			t.Fatalf("Skip なのに再起動した: %v", calls)
		}
		if elapsed > time.Second {
			t.Fatalf("Skip なのに待った: %v", elapsed)
		}
		if countPrefix(calls, "scene:") != 1 {
			t.Fatalf("SetScene へ進まない: %v", calls)
		}
	})
}

// status の warnings: 名前が OBS で引けない scene_fallbacks の行を、行番号（1 始まり）と名前つきで出す（0c T4）。
func TestSnapshotFallbackWarnings(t *testing.T) {
	h := newHarness(t, func(_ *switcher.Config, d *Deps) {
		d.WarningMessages = func() []i18n.Msg { return []i18n.Msg{i18n.Text("existing warning")} }
		d.SceneFallbacks = []switcher.FallbackRow{
			{Scene: "NoSuchScene", Fallback: camera},
			{Scene: camera, Fallback: "GoneScene"},
			{Scene: camera2, Fallback: camera},
		}
	})
	h.start()
	h.connect(true, camera)
	s := h.waitSnap("警告", func(s Snapshot) bool {
		return slices.ContainsFunc(i18n.Strings(i18n.JA, s.OBS.Warnings), func(w string) bool { return strings.Contains(w, "scene_fallbacks の 1 行目") })
	})
	var got []string
	for _, w := range i18n.Strings(i18n.JA, s.OBS.Warnings) {
		if strings.Contains(w, "scene_fallbacks") {
			got = append(got, w)
		}
	}
	if !slices.Contains(i18n.Strings(i18n.JA, s.OBS.Warnings), "existing warning") {
		t.Fatalf("既存の警告が消えた: %v", i18n.Strings(i18n.JA, s.OBS.Warnings))
	}
	if len(got) != 2 || !strings.Contains(got[0], "NoSuchScene") || !strings.Contains(got[1], "scene_fallbacks の 2 行目") || !strings.Contains(got[1], "GoneScene") {
		t.Fatalf("warnings = %v", got)
	}
}
