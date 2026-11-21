package core

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// wait はテストの待ちの上限。正常なら一瞬で満たされる条件にだけ使う。
const wait = 2 * time.Second

// ---- 時計 ----

// fakeClock はテストが進める時計。中核ループ、Sample、偽の OBS のイベントの時刻を1つにそろえる。
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
	return c.t
}

// ---- ログ ----

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// ---- 偽の obs.Client ----

// fakeOBS は obs.Client の偽物。呼ばれた操作を "restart:IRL"、"override:Camera"、"scene:Camera"、
// "start"、"stop" の形で記録する。各操作の振る舞いは関数で差し替える。
type fakeOBS struct {
	clk    *fakeClock
	events chan switcher.Event // バッファなし。送った順に中核ループが受ける
	done   chan struct{}       // テストの終わりで閉じる。偽物の中の待ちを解く

	mu    sync.Mutex
	calls []string
	// 呼ばれた直後（振る舞いを実行する前）に知らせる。
	entered chan string

	inflight atomic.Int32 // 実行中の操作の数

	onScene    func(name string) error
	onRestart  func(input string) error
	onStart    func() error
	onStop     func() error
	onOverride func(scene string) (*int, bool, error)
}

func newFakeOBS(clk *fakeClock) *fakeOBS {
	f := &fakeOBS{
		clk:     clk,
		events:  make(chan switcher.Event),
		done:    make(chan struct{}),
		entered: make(chan string, 1000),
	}
	// 既定では、SetScene は OBS と同じく program のシーンの変更を流してから成功を返す。
	f.onScene = func(name string) error {
		f.emit(switcher.SceneChanged{At: clk.Now(), Scene: name, SceneUUID: name})
		return nil
	}
	return f
}

// emit は偽の OBS からイベントを流す。テストが終われば諦める。
func (f *fakeOBS) emit(ev switcher.Event) {
	select {
	case f.events <- ev:
	case <-f.done:
	}
}

// gate は release を閉じるまで（テストが終わるまで）止まる。
func (f *fakeOBS) gate(release <-chan struct{}) {
	select {
	case <-release:
	case <-f.done:
	}
}

func (f *fakeOBS) record(c string) func() {
	f.inflight.Add(1)
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	f.entered <- c
	return func() { f.inflight.Add(-1) }
}

func (f *fakeOBS) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeOBS) Events() <-chan switcher.Event { return f.events }

func (f *fakeOBS) SceneTransitionOverrideUUID(scene string) (*int, bool, error) {
	defer f.record("override:" + scene)()
	if f.onOverride != nil {
		return f.onOverride(scene)
	}
	return nil, false, nil
}

func (f *fakeOBS) SetSceneUUID(name string) error {
	defer f.record("scene:" + name)()
	return f.onScene(name)
}

// RestartMediaInputs は呼びごとに "restart:" と入力の UUID を "," でつないで記録する。
func (f *fakeOBS) RestartMediaInputs(inputs []string) error {
	in := strings.Join(inputs, ",")
	defer f.record("restart:" + in)()
	if f.onRestart != nil {
		return f.onRestart(in)
	}
	return nil
}

// SetSceneItemEnabled は M-12 まで core が呼ばない。呼ばれたら記録して成功にする。
func (f *fakeOBS) SetSceneItemEnabled(sceneUUID string, itemID int, enabled bool) error {
	defer f.record(fmt.Sprintf("item:%s:%d:%v", sceneUUID, itemID, enabled))()
	return nil
}

func (f *fakeOBS) StartStream() error {
	defer f.record("start")()
	if f.onStart != nil {
		return f.onStart()
	}
	return nil
}

func (f *fakeOBS) StopStream() error {
	defer f.record("stop")()
	if f.onStop != nil {
		return f.onStop()
	}
	return nil
}

// ---- 組み立て ----

// mainCam と subCam は例に使うカメラの id。
const (
	mainCam = "main"
	subCam  = "sub"
)

// testCamera は例のカメラの設定。閾値は testThresholds。
func testCamera(id, name string, onBad scenemap.Motion) Camera {
	return Camera{ID: id, Name: name, OnBad: onBad, Thresholds: testThresholds,
		URLs: CameraURLs{PublishSRT: "srt://x/" + id, PublishRTMP: "rtmp://x/" + id, OBSRead: "rtsp://x/" + id}}
}

// testMap は例の地図。UUID は名前と同じにする。待機シーンと管理外のシーン（Talk、Opening）はカメラなし、
// Camera と Camera2 には、カメラ main のメディアソース IRL を映す。
func testMap() *scenemap.Map {
	cam := func(name string) scenemap.Container {
		return scenemap.Container{Name: name, Items: []scenemap.Item{{ID: 1, SourceUUID: "IRL", SourceKind: scenemap.InputSource, Enabled: true}}}
	}
	return &scenemap.Map{
		Scenes: map[string]scenemap.Container{
			standby: {Name: standby}, camera: cam(camera), camera2: cam(camera2), talk: {Name: talk}, opening: {Name: opening},
		},
		Groups: map[string]scenemap.Container{},
		Inputs: map[string]scenemap.Input{"IRL": {Name: "IRL", CameraID: mainCam}},
	}
}

// 例に使うシーン名。待機シーン、監視対象2つ、管理外2つ。
const (
	standby = "Standby"
	camera  = "Camera"
	camera2 = "Camera2"
	talk    = "Talk"
	opening = "Opening"
)

// 閾値。DOWN から good を5回で OK、OK から 0 を3回で LOW、5回で DOWN。
var testThresholds = ingest.Thresholds{
	FailKbps:    1000,
	RecoverKbps: 2000,
	LowSec:      3 * time.Second,
	RecoverSec:  5 * time.Second,
	DownSec:     5 * time.Second,
}

const goodKbps = 3000

type harness struct {
	t       *testing.T
	clk     *fakeClock
	obs     *fakeOBS
	core    *Core
	log     *syncBuf
	samples chan ingest.SampleSet
	ticks   chan time.Time
	seen    chan any // 中核ループが処理した Event と承認の記録

	mtxMu sync.Mutex
	mtx   supervisor.Status // MediaMTX の状態。setMediaMTX で変える

	ctx     context.Context
	cancel  context.CancelFunc
	started bool
	runDone chan struct{}
}

type option func(*switcher.Config, *Deps)

func withRestartWait(d time.Duration) option {
	return func(c *switcher.Config, _ *Deps) { c.RestartWait = d }
}

func withStartScene(name string) option {
	return func(_ *switcher.Config, d *Deps) { d.StartScene = &name }
}

func newHarness(t *testing.T, opts ...option) *harness {
	t.Helper()
	clk := newClock()
	f := newFakeOBS(clk)
	buf := &syncBuf{}
	mcfg := switcher.Config{RestartWait: 10 * time.Millisecond}
	h := &harness{
		t: t, clk: clk, obs: f, log: buf,
		samples: make(chan ingest.SampleSet),
		ticks:   make(chan time.Time),
		seen:    make(chan any, 10000),
		runDone: make(chan struct{}),
		mtx:     supervisor.Status{State: supervisor.StateRunning},
	}
	deps := Deps{
		OBS:          f,
		StandbyScene: standby,
		Cameras:      []Camera{testCamera(mainCam, "メイン", scenemap.MotionStandby)},
		AutoEnabled:  true,
		Samples:      h.samples,
		MediaMTX: func() supervisor.Status {
			h.mtxMu.Lock()
			defer h.mtxMu.Unlock()
			return h.mtx
		},
		Log:  slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Tick: h.ticks,
		Now:  clk.Now,
	}
	for _, o := range opts {
		o(&mcfg, &deps)
	}
	deps.Machine = switcher.New(mcfg)
	h.core = New(deps)
	h.core.observe = func(v any) {
		select {
		case h.seen <- v:
		default:
			t.Errorf("seen があふれた")
		}
	}
	h.ctx, h.cancel = context.WithCancel(context.Background())
	t.Cleanup(func() {
		h.cancel()
		close(f.done)
		if !h.started {
			return
		}
		select {
		case <-h.runDone:
		case <-time.After(wait):
			t.Errorf("Run が戻らない")
		}
	})
	return h
}

func (h *harness) start() {
	h.started = true
	go func() {
		defer close(h.runDone)
		h.core.Run(h.ctx)
	}()
}

// sample は1秒ぶんの Sample を、先頭のカメラに kbps、残りのカメラに 0 で、1組にして送る。中核ループが受け取るまで待つ。
func (h *harness) sample(kbps float64) {
	h.t.Helper()
	ks := make([]float64, len(h.core.cams))
	ks[0] = kbps
	h.sampleSet(ks...)
}

// sampleSet は1秒ぶんの Sample の組を、カメラの順の kbps で作って送る。
func (h *harness) sampleSet(kbps ...float64) {
	h.t.Helper()
	at := h.clk.Advance(time.Second)
	set := ingest.SampleSet{At: at}
	for i, k := range kbps {
		set.Cameras = append(set.Cameras, ingest.Sample{
			Camera: h.core.cams[i].cfg.ID, At: at, Interval: time.Second, Online: k > 0, Protocol: "srt", Kbps: k})
	}
	h.sendSet(set)
}

func (h *harness) sendSet(set ingest.SampleSet) {
	h.t.Helper()
	select {
	case h.samples <- set:
	case <-time.After(wait):
		h.t.Fatalf("Sample の組を受け取らない: %+v", set)
	}
}

func (h *harness) samplesN(n int, kbps float64) {
	h.t.Helper()
	for range n {
		h.sample(kbps)
	}
}

// obsEvent は偽の OBS からイベントを流し、中核ループが受け取るまで待つ。
func (h *harness) obsEvent(ev switcher.Event) {
	h.t.Helper()
	select {
	case h.obs.events <- ev:
	case <-time.After(wait):
		h.t.Fatalf("OBS のイベントを受け取らない: %#v", ev)
	}
}

func (h *harness) tick(at time.Time) {
	h.t.Helper()
	select {
	case h.ticks <- at:
	case <-time.After(wait):
		h.t.Fatalf("Tick を受け取らない")
	}
}

// setMediaMTX は MediaMTX の状態を変える。中核ループは次の入力の処理のあとで気づく。
func (h *harness) setMediaMTX(st supervisor.Status) {
	h.mtxMu.Lock()
	defer h.mtxMu.Unlock()
	h.mtx = st
}

func (h *harness) connect(streaming bool, scene string) {
	h.t.Helper()
	h.obsEvent(switcher.OBSConnected{At: h.clk.Now(), Streaming: streaming, Scene: scene, SceneUUID: scene, Map: testMap()})
}

// command は Command を送り、かかった時間も返す。
func (h *harness) command(k switcher.CommandKind) (error, time.Duration) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	start := time.Now()
	err := h.core.Command(ctx, k)
	return err, time.Since(start)
}

// waitCall は偽の OBS が c を呼ぶまで待つ。それまでに呼ばれた操作は読み捨てる。
func (h *harness) waitCall(c string) {
	h.t.Helper()
	deadline := time.After(wait)
	for {
		select {
		case got := <-h.obs.entered:
			if got == c {
				return
			}
		case <-deadline:
			h.t.Fatalf("%q が呼ばれない。calls=%v", c, h.obs.Calls())
		}
	}
}

// waitSeen は pred を満たす記録が来るまで待ち、それまでに読んだ記録（満たしたものを含む）を返す。
func (h *harness) waitSeen(what string, pred func(any) bool) []any {
	h.t.Helper()
	var got []any
	deadline := time.After(wait)
	for {
		select {
		case v := <-h.seen:
			got = append(got, v)
			if pred(v) {
				return got
			}
		case <-deadline:
			h.t.Fatalf("%s が来ない。来たもの=%#v", what, got)
			return nil
		}
	}
}

func isBatchDone(result switcher.BatchResult) func(any) bool {
	return func(v any) bool {
		bd, ok := v.(switcher.BatchDone)
		return ok && bd.Result == result
	}
}

// waitSnap は Snapshot が pred を満たすまで待つ（上限つきの見回り）。
func (h *harness) waitSnap(what string, pred func(Snapshot) bool) Snapshot {
	h.t.Helper()
	deadline := time.Now().Add(wait)
	for {
		s := h.core.Snapshot()
		if pred(s) {
			return s
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("Snapshot が %s にならない: %+v", what, s)
		}
		time.Sleep(time.Millisecond)
	}
}

// linkIs は先頭のカメラの受信状態を見る。
func linkIs(state string) func(Snapshot) bool {
	return func(s Snapshot) bool { return len(s.Cameras) > 0 && s.Cameras[0].State == state }
}

func sceneIs(scene string) func(Snapshot) bool {
	return func(s Snapshot) bool { return s.OBS.Connected && s.OBS.Scene == scene }
}

// standbyIs は待機の持ち主と戻り先（空なら null）を見る。
func standbyIs(owner, ret string) func(Snapshot) bool {
	return func(s Snapshot) bool {
		r := s.OBS.Standby.ReturnScene
		return s.OBS.Standby.Owner == owner && ((ret == "" && r == nil) || (r != nil && *r == ret))
	}
}

func both(a, b func(Snapshot) bool) func(Snapshot) bool {
	return func(s Snapshot) bool { return a(s) && b(s) }
}

// autoStandby は、受信を OK にして配信中の ret に接続し、受信を落として自動の待機に入れる。
// 戻り値は、そこまでに偽の OBS が呼ばれた回数。
func (h *harness) autoStandby(ret string) int {
	h.t.Helper()
	h.samplesN(5, goodKbps)
	h.waitSnap("OK", linkIs("OK"))
	h.connect(true, ret)
	h.waitSnap(ret, sceneIs(ret))
	h.samplesN(3, 0) // 0 が3秒続いて LOW
	h.waitCall("scene:" + standby)
	h.waitSeen("AutoFail の BatchDone", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("自動の待機", both(sceneIs(standby), standbyIs("auto", ret)))
	return len(h.obs.Calls())
}

// manualStandby は、受信を OK にして配信中の ret に接続し、StandbyOn で手動の待機に入れる。
func (h *harness) manualStandby(ret string) int {
	h.t.Helper()
	h.samplesN(5, goodKbps)
	h.waitSnap("OK", linkIs("OK"))
	h.connect(true, ret)
	h.waitSnap(ret, sceneIs(ret))
	if err, _ := h.command(switcher.CmdStandbyOn); err != nil {
		h.t.Fatalf("StandbyOn: %v", err)
	}
	h.waitSeen("StandbyOn の BatchDone", isBatchDone(switcher.BatchCompleted))
	h.waitSnap("手動の待機", both(sceneIs(standby), standbyIs("manual", ret)))
	return len(h.obs.Calls())
}

func callsSince(f *fakeOBS, n int) []string { return f.Calls()[n:] }

func countPrefix(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}
