package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 配信中の Live で受信が0に落ちると、Fail への切替を送り、理由をログに残す。
func TestEndToEndAutoFail(t *testing.T) {
	h := newHarness(t)
	h.start()

	h.samplesN(5, goodKbps)
	h.waitSnap("OK", linkIs("OK"))
	h.connect(true, "Live")
	h.waitSnap("live", roleIs("live"))

	h.samplesN(3, 0) // 0 が3秒続いて LOW
	h.waitCall("scene:Fail")
	h.waitSeen("BatchDone{Completed}", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("fail", roleIs("fail"))

	want := `msg="scene switch" from=live to=fail reason=low kbps=0`
	if out := h.log.String(); !strings.Contains(out, want) {
		t.Errorf("ログに %q がない:\n%s", want, out)
	}
	if got := countPrefix(h.obs.Calls(), "scene:"); got != 1 {
		t.Errorf("SetScene の回数 = %d, want 1。calls=%v", got, h.obs.Calls())
	}
}

// Fail のまま受信が戻ると、RestartMedia のあとに SetScene(Live) を送る。
// 最後の SceneRequestDone は BatchDone より先に届く。
func TestRecoveryRestartsMediaThenLive(t *testing.T) {
	h := newHarness(t)
	h.start()

	h.connect(true, "Fail")
	h.waitSnap("fail", roleIs("fail"))
	h.samplesN(5, goodKbps) // 回復の窓を満たして OK

	seen := h.waitSeen("BatchDone{Completed}", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("live", roleIs("live"))

	want := []string{"restart:IRL", "override:Live", "scene:Live"}
	if got := h.obs.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	reqDone := slices.IndexFunc(seen, func(v any) bool {
		d, ok := v.(switcher.SceneRequestDone)
		return ok && d.Result == switcher.RequestOK
	})
	if reqDone < 0 || reqDone >= len(seen)-1 {
		t.Errorf("SceneRequestDone{OK} が BatchDone より前にない: %#v", seen)
	}
	if !strings.Contains(h.log.String(), `msg="scene switch" from=fail to=live reason=ok`) {
		t.Errorf("復帰の切替のログがない:\n%s", h.log.String())
	}
}

// RestartMedia の待ちの間に受信が悪化したら、SetScene(Live) の承認が却下され、送らない。
func TestApproveRejectsWhenLinkDropsDuringRestart(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.obs.onRestart = func(string) error { h.obs.gate(release); return nil }
	h.start()

	h.connect(true, "Fail")
	h.waitSnap("fail", roleIs("fail"))
	h.samplesN(5, goodKbps)
	h.waitCall("restart:IRL") // 実行役は RestartMedia の中で止まっている

	h.samplesN(3, 0)
	h.waitSnap("LOW", linkIs("LOW"))
	close(release)

	seen := h.waitSeen("BatchDone{Rejected}", isBatchDone(switcher.BatchRejected))
	approved := slices.ContainsFunc(seen, func(v any) bool {
		a, ok := v.(approval)
		_, rejected := a.Result.(switcher.Reject)
		return ok && rejected && a.Action == switcher.SetScene{Role: switcher.RoleLive}
	})
	if !approved {
		t.Errorf("SetScene(Live) の承認が却下されていない: %#v", seen)
	}
	if n := countPrefix(h.obs.Calls(), "scene:"); n != 0 {
		t.Errorf("SetScene が送られた: %v", h.obs.Calls())
	}
	if s := h.core.Snapshot(); s.OBS.Role != "fail" || s.LastError != nil {
		t.Errorf("Snapshot = %+v（Auto の却下は last_error に残さない）", s)
	}
}

// GoLive の待ちの間に受信が悪化したら Replace{GoFail} になり、代わりのバッチは
// 先に並んでいた StopStream より前に実行される。
func TestReplaceGoesToHeadOfQueue(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.obs.onRestart = func(string) error { h.obs.gate(release); return nil }
	h.start()

	h.connect(true, "Intro")
	h.waitSnap("intro", roleIs("intro"))
	h.samplesN(5, goodKbps)
	h.waitSnap("OK", linkIs("OK"))

	if err, _ := h.command(switcher.CmdGoLive); err != nil {
		t.Fatalf("GoLive: %v", err)
	}
	h.waitCall("restart:IRL")
	if err, _ := h.command(switcher.CmdStreamStop); err != nil {
		t.Fatalf("StreamStop: %v", err)
	}
	h.samplesN(3, 0)
	h.waitSnap("LOW", linkIs("LOW"))
	close(release)

	h.waitCall("stop")
	h.waitSeen("StopStream の BatchDone", func(v any) bool {
		bd, ok := v.(switcher.BatchDone)
		return ok && bd.BatchID == 2 // GoLive=1、StopStream=2、GoFail=3。GoFail が先に入るので最後は StopStream
	})
	want := []string{"restart:IRL", "override:Live", "override:Fail", "scene:Fail", "stop"}
	if got := h.obs.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if s := h.core.Snapshot(); s.LastError != nil {
		t.Errorf("置き換えた却下を last_error に残した: %q", *s.LastError)
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

	h.connect(true, "Live") // 受信は初期値の DOWN なので、すぐ AutoFail
	h.waitCall("scene:Fail")

	commands := []struct {
		kind    switcher.CommandKind
		invalid bool
	}{
		{switcher.CmdPauseOn, false},
		{switcher.CmdPauseOn, true}, // 同じ操作が実行待ち
		{switcher.CmdGoLive, true},  // 役割が Intro でない
		{switcher.CmdStreamStop, false},
		{switcher.CmdStreamStart, true}, // 配信中
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
			h.waitSnap(fmt.Sprintf("kbps=%v", kbps), func(s Snapshot) bool { return s.Link.Kbps == kbps })
		}
	}
	if n := countPrefix(h.obs.Calls(), "scene:"); n != 1 {
		t.Errorf("詰まっている間に別の SetScene が送られた: %v", h.obs.Calls())
	}
	close(release)
	// 詰まりが解けたら、並んでいたバッチに進む。
	h.waitCall("scene:Pause")
}

// 打ち切られたトランジション: Fail を送った直後に Pause を送り、Pause のイベントだけが届く。
// Fail への期待も消えているので、そのあとの人による Fail への切替は手動とみなす。
func TestTruncatedTransition(t *testing.T) {
	h := newHarness(t)
	h.obs.onScene = func(name string) error {
		if name == "Pause" {
			h.obs.emit(switcher.SceneChanged{At: h.clk.Now(), Scene: name})
		}
		return nil // Fail のイベントは流さない
	}
	h.start()

	h.connect(true, "Live")
	h.waitCall("scene:Fail")
	h.waitSeen("AutoFail の BatchDone", isBatchDone(switcher.BatchCompleted))
	if err, _ := h.command(switcher.CmdPauseOn); err != nil {
		t.Fatalf("PauseOn: %v", err)
	}
	h.waitSnap("pause", roleIs("pause"))
	h.waitSeen("PauseOn の BatchDone", isBatchDone(switcher.BatchCompleted))

	h.tick(h.clk.Advance(30 * time.Second)) // どの期待も期限を過ぎる
	h.obsEvent(switcher.SceneChanged{At: h.clk.Advance(time.Second), Scene: "Fail"})
	s := h.waitSnap("fail", roleIs("fail"))
	if !s.OBS.ManualHold.Active || s.OBS.ManualHold.Since == nil {
		t.Errorf("人による Fail への切替が手動とみなされていない: %+v", s.OBS.ManualHold)
	}
}

// RestartMedia が失敗しても SetScene(Live) は送り、バッチは Completed で Err を持つ。
func TestRestartFailureContinuesToSetScene(t *testing.T) {
	h := newHarness(t)
	h.obs.onRestart = func(string) error { return errors.New("input not found") }
	h.start()

	h.connect(true, "Fail")
	h.waitSnap("fail", roleIs("fail"))
	h.samplesN(5, goodKbps)

	seen := h.waitSeen("BatchDone", func(v any) bool { _, ok := v.(switcher.BatchDone); return ok })
	bd := seen[len(seen)-1].(switcher.BatchDone)
	if bd.Result != switcher.BatchCompleted || bd.Err == nil || !strings.Contains(bd.Err.Error(), "input not found") {
		t.Errorf("BatchDone = %+v", bd)
	}
	want := []string{"restart:IRL", "override:Live", "scene:Live"}
	if got := h.obs.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	s := h.waitSnap("live", roleIs("live"))
	if s.LastError == nil || !strings.Contains(*s.LastError, "input not found") {
		t.Errorf("last_error = %v", s.LastError)
	}
}

// SetScene の失敗は SceneRequestDone{Error} を返し、残りを捨てる。
func TestSetSceneFailureEndToEnd(t *testing.T) {
	h := newHarness(t)
	h.obs.onScene = func(string) error { return errors.New("scene not found") }
	h.start()

	h.connect(true, "Live")
	seen := h.waitSeen("BatchDone{Failed}", isBatchDone(switcher.BatchFailed))
	if !slices.ContainsFunc(seen, func(v any) bool {
		d, ok := v.(switcher.SceneRequestDone)
		return ok && d.Result == switcher.RequestError
	}) {
		t.Errorf("SceneRequestDone{Error} がない: %#v", seen)
	}
	s := h.waitSnap("last_error", func(s Snapshot) bool { return s.LastError != nil })
	if !strings.Contains(*s.LastError, "scene not found") {
		t.Errorf("last_error = %q", *s.LastError)
	}
}

// 実行役だけを動かし、テストが中核ループの側を受け持つ。
func TestExecutorBatchRules(t *testing.T) {
	boom := errors.New("boom")
	timeout := fmt.Errorf("%w: deadline", obs.ErrTimeout)
	setFail := switcher.SetScene{Role: switcher.RoleFail}
	restart := switcher.RestartMedia{Wait: time.Millisecond}
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
			sceneErr: boom, wantCalls: []string{"override:Fail", "scene:Fail"},
			wantReq: []switcher.RequestResult{switcher.RequestError}, wantRes: switcher.BatchFailed, wantErr: boom,
		},
		{
			name:     "SetScene のタイムアウトは Timeout で、残りを捨てる",
			actions:  []switcher.Action{setFail, switcher.StopStream{}},
			approve:  []switcher.ApproveResult{switcher.Proceed{ReqID: 3}},
			sceneErr: timeout, wantCalls: []string{"override:Fail", "scene:Fail"},
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
			restErr: boom, wantCalls: []string{"restart:IRL", "override:Fail", "scene:Fail"},
			wantReq: []switcher.RequestResult{switcher.RequestOK}, wantRes: switcher.BatchCompleted, wantErr: boom,
		},
		{
			name:      "Skip は送らずに次へ進む",
			actions:   []switcher.Action{setFail, switcher.StopStream{}},
			approve:   []switcher.ApproveResult{switcher.Skip{}, switcher.Proceed{}},
			wantCalls: []string{"override:Fail", "stop"}, wantRes: switcher.BatchCompleted,
		},
		{
			name:      "Reject で残りを捨てる",
			actions:   []switcher.Action{restart, setFail},
			approve:   []switcher.ApproveResult{switcher.Proceed{}, switcher.Reject{}},
			wantCalls: []string{"restart:IRL", "override:Fail"}, wantRes: switcher.BatchRejected,
		},
		{
			name:      "Replace で残りを捨てる",
			actions:   []switcher.Action{restart, setFail},
			approve:   []switcher.ApproveResult{switcher.Proceed{}, switcher.Replace{Batch: &switcher.Batch{BatchID: 99}}},
			wantCalls: []string{"restart:IRL", "override:Fail"}, wantRes: switcher.BatchRejected,
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
				h.core.execute(ctx, &switcher.Batch{BatchID: 7, Purpose: switcher.PurposeGoFail, Actions: tc.actions})
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
		go h.core.execute(ctx, &switcher.Batch{BatchID: 1, Actions: []switcher.Action{switcher.SetScene{Role: switcher.RoleLive}}})
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
	h.connect(false, "Live")
	h.waitSnap("connected", roleIs("live"))
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

	h.connect(true, "Live")
	h.waitCall("scene:Fail")
	h.cancel()
	select {
	case <-h.runDone:
	case <-time.After(time.Second):
		t.Fatal("Run が1秒以内に戻らない")
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := h.core.Command(ctx, switcher.CmdPauseOn); !errors.Is(err, ErrStopped) {
		t.Errorf("止まったあとの Command: err = %v", err)
	}
	close(release)
	waitInflightZero(t, h.obs)
}

// RestartMedia の待ちの間でも、ctx の取り消しで Run は戻る。
func TestCancelDuringRestartWait(t *testing.T) {
	h := newHarness(t, withRestartWait(time.Hour))
	h.start()
	h.connect(true, "Fail")
	h.waitSnap("fail", roleIs("fail"))
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

// status の JSON は 9章の形と同じキーを持つ。
const specStatusJSON = `{
  "link":    {"state": "OK", "kbps": 4200.5, "online": true, "protocol": "srt",
              "srt": {"rtt_ms": 85.2, "loss_rate": 0.01, "recv_mbps": 4.2}},
  "obs":     {"connected": true, "streaming": true, "scene": "Live", "role": "live",
              "manual_hold": {"active": false, "since": null},
              "warnings": []},
  "mediamtx": "running",
  "urls":    {"publish_srt": "srt://…", "publish_rtmp": "rtmp://…", "obs_read": "rtsp://obs:…@…"},
  "last_error": null
}`

func TestSnapshotJSONShape(t *testing.T) {
	var spec map[string]any
	if err := json.Unmarshal([]byte(specStatusJSON), &spec); err != nil {
		t.Fatal(err)
	}

	h := newHarness(t)
	// 起動前の初期値: warnings は []、since・srt・last_error は null。
	initial := marshalMap(t, h.core.Snapshot())
	if !reflect.DeepEqual(keyPaths(initial, ""), keyPaths(spec, "")) {
		t.Errorf("初期値のキー = %v, want %v", keyPaths(initial, ""), keyPaths(spec, ""))
	}
	obsPart := initial["obs"].(map[string]any)
	if w, ok := obsPart["warnings"].([]any); !ok || len(w) != 0 {
		t.Errorf("warnings = %#v, want []", obsPart["warnings"])
	}
	for _, v := range []any{obsPart["manual_hold"].(map[string]any)["since"], initial["last_error"], initial["link"].(map[string]any)["srt"]} {
		if v != nil {
			t.Errorf("null であるべき値 = %#v", v)
		}
	}
	if initial["mediamtx"] != "running" || initial["urls"].(map[string]any)["obs_read"] != "rtsp://x" {
		t.Errorf("初期値 = %#v", initial)
	}

	// SRT の Sample のあと: srt の中のキーも同じ。
	h.start()
	srt := ingest.Sample{At: h.clk.Advance(time.Second), Interval: time.Second, Online: true, Protocol: "srt", Kbps: 4200.5,
		SRT: &ingest.SRTStats{RTTMs: 85.2, LossRate: 0.01, RecvMbps: 4.2}}
	select {
	case h.samples <- srt:
	case <-time.After(wait):
		t.Fatal("Sample を受け取らない")
	}
	h.waitSnap("srt", func(s Snapshot) bool { return s.Link.SRT != nil })
	full := marshalMap(t, h.core.Snapshot())
	if !reflect.DeepEqual(keyPaths(full, ""), keyPaths(spec, "")) {
		t.Errorf("キー = %v, want %v", keyPaths(full, ""), keyPaths(spec, ""))
	}
	want := map[string]any{"state": "LOW", "kbps": 4200.5, "online": true, "protocol": "srt",
		"srt": map[string]any{"rtt_ms": 85.2, "loss_rate": 0.01, "recv_mbps": 4.2}}
	if !reflect.DeepEqual(full["link"], want) {
		t.Errorf("link = %#v, want %#v", full["link"], want)
	}
}

func marshalMap(t *testing.T, s Snapshot) map[string]any {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// keyPaths はオブジェクトのキーを "obs.manual_hold.since" の形で並べる。
// 値が null の場所は、相手がオブジェクトでもキーだけを比べる（srt は null のことがある）。
func keyPaths(m map[string]any, prefix string) []string {
	var out []string
	for k, v := range m {
		p := prefix + k
		out = append(out, p)
		if sub, ok := v.(map[string]any); ok {
			out = append(out, keyPaths(sub, p+".")...)
		} else if v == nil && p == "link.srt" {
			out = append(out, "link.srt.loss_rate", "link.srt.recv_mbps", "link.srt.rtt_ms")
		}
	}
	sort.Strings(out)
	return out
}
