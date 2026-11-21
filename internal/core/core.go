package core

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/supervisor"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// ErrStopped は中核ループが止まっているため Command を受け付けられないこと。
var ErrStopped = errors.New("中核ループが止まっている")

// Deps は Core が使う部品と設定。
type Deps struct {
	// Machine と Judge は New と Run の goroutine だけが触る。Run の間はほかから呼ばない。
	Machine *switcher.Machine
	Judge   *ingest.Judge
	OBS     obs.Client
	// StandbyScene、MonitoredScenes、AutoEnabled、StartScene はシーンの設定。New がループを始める前に
	// ScenesConfigured として Machine に1回渡す。StartScene は配信開始時の切替が無効なら nil。
	StandbyScene    string
	MonitoredScenes []string
	AutoEnabled     bool
	StartScene      *string
	// MediaSource は RestartMedia を送るメディアソースの入力名。
	MediaSource string
	// Samples は Poller の出力。
	Samples <-chan ingest.Sample
	// MediaMTX は MediaMTX の状態を返す。nil なら status の mediamtx は空。
	MediaMTX func() supervisor.Status
	// Warnings は OBS の接続時に見つかった警告を返す（obs.WS.Warnings）。nil なら警告なし。
	Warnings func() []string
	// URLs は status に出す固定の URL。
	URLs URLs
	Log  *slog.Logger
	// Tick は Tick の時刻の列。nil なら Run が1秒ごとの ticker を作る。
	Tick <-chan time.Time
	// Now は中核ループと実行役が使う時計。nil なら time.Now。
	Now func() time.Time
}

// Core は状態を1か所で持つ中核ループと、OBS の操作を行う実行役。
type Core struct {
	m           *switcher.Machine
	judge       *ingest.Judge
	obs         obs.Client
	mediaSource string
	standby     string
	monitored   []string
	startScene  *string
	samples     <-chan ingest.Sample
	mediamtx    func() supervisor.Status
	warnings    func() []string
	urls        URLs
	log         *slog.Logger
	tick        <-chan time.Time
	now         func() time.Time

	// 中核ループと実行役の間のチャネル。どれもバッファなし。
	batches   chan *switcher.Batch // 中核ループ → 実行役。列の先頭を1つだけ差し出す
	approvals chan approveReq      // 実行役 → 中核ループ
	results   chan switcher.Event  // 実行役 → 中核ループ。SceneRequestDone と BatchDone を送った順に届ける
	cmds      chan cmdReq          // Command → 中核ループ
	stopped   chan struct{}        // Run が戻ったら閉じる

	snap atomic.Pointer[Snapshot]

	// ここから下は中核ループの goroutine だけが触る。
	queue      []*switcher.Batch // 実行待ちのバッチ
	sample     ingest.Sample     // 最後の Sample
	lastErr    *LastError
	prevSwErr  *switcher.ErrorInfo
	prevMtxErr string
	prevSw     *switcher.Snapshot // 前回の Snapshot を作ったときの switcher の状態。最初は nil

	// observe はテスト用。中核ループが処理した Event と承認（approval）ごとに、中核ループの goroutine で呼ぶ。
	observe func(any)
}

// approveReq は実行役からの承認要求。reply は容量1。
type approveReq struct {
	batchID  int
	purpose  switcher.Purpose
	action   switcher.Action
	override switcher.Override
	reply    chan switcher.ApproveResult
}

// cmdReq は Command の要求。reply は容量1。
type cmdReq struct {
	kind  switcher.CommandKind
	reply chan error
}

// approval は observe に渡す承認の記録。
type approval struct {
	BatchID int
	Action  switcher.Action
	Result  switcher.ApproveResult
}

// New は Core を作り、シーンの設定を Machine に渡す。最初の Snapshot もここで公開する。
func New(d Deps) *Core {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	c := &Core{
		m: d.Machine, judge: d.Judge, obs: d.OBS, mediaSource: d.MediaSource,
		standby: d.StandbyScene, monitored: slices.Clone(d.MonitoredScenes),
		samples: d.Samples, mediamtx: d.MediaMTX, warnings: d.Warnings,
		urls: d.URLs, log: d.Log, tick: d.Tick, now: d.Now,
		batches:   make(chan *switcher.Batch),
		approvals: make(chan approveReq),
		results:   make(chan switcher.Event),
		cmds:      make(chan cmdReq),
		stopped:   make(chan struct{}),
	}
	var start *string
	if d.StartScene != nil {
		v := *d.StartScene
		start = &v
	}
	c.startScene = start
	// ループを始める前の最初の Event。チャネルは通さない。
	// 第1段階の設定（監視対象のシーンの名前と1本のメディアソース）を、カメラ1台 legacyCamera の設定に読み替える。
	// 本格的な配線（複数のカメラの設定と OBS の地図）は M-8 で行う。
	c.handle(switcher.ScenesConfigured{
		At:          c.now(),
		Standby:     d.StandbyScene,
		Cameras:     []switcher.CameraConfig{{ID: legacyCamera, OnBad: scenemap.MotionStandby}},
		AutoEnabled: d.AutoEnabled,
		StartScene:  start,
	})
	c.publish()
	return c
}

// Snapshot は最後に公開した状態を返す。どの goroutine から呼んでもよい。
func (c *Core) Snapshot() Snapshot {
	return *c.snap.Load()
}

// Command は手動操作を中核ループに渡し、switcher の cmdErr を返す（nil、ErrNotConnected、
// ErrInvalidState）。Action の実行は待たない。Run が止まっていれば ErrStopped。
func (c *Core) Command(ctx context.Context, kind switcher.CommandKind) error {
	req := cmdReq{kind: kind, reply: make(chan error, 1)}
	select {
	case c.cmds <- req:
	case <-c.stopped:
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run は ctx が終わるまで中核ループと実行役を動かす。1回だけ呼ぶ。
// 実行役が OBS の操作の応答を待っている間に止めた場合、その操作は Run が戻ったあとも
// 応答（obs の応答待ちの上限）まで残るが、結果は捨てる。
func (c *Core) Run(ctx context.Context) {
	defer close(c.stopped)
	tick := c.tick
	if tick == nil {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		tick = t.C
	}
	var wg sync.WaitGroup
	wg.Go(func() { c.runExecutor(ctx) })
	defer wg.Wait()
	c.loop(ctx, tick)
}

// loop は中核ループ。どのチャネルへの送信でも止まらない。
func (c *Core) loop(ctx context.Context, tick <-chan time.Time) {
	samples := c.samples
	events := c.obs.Events()
	for {
		// 実行待ちの先頭だけを差し出す。列が空なら nil チャネルでこの case を無効にする。
		var out chan<- *switcher.Batch
		var head *switcher.Batch
		if len(c.queue) > 0 {
			out, head = c.batches, c.queue[0]
		}
		select {
		case <-ctx.Done():
			return
		case s, ok := <-samples:
			if !ok {
				samples = nil
				continue
			}
			c.onSample(s)
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			c.handle(ev)
		case at := <-tick:
			c.handle(switcher.Tick{At: at})
		case req := <-c.cmds:
			// reply は容量1なので止まらない。
			req.reply <- c.handle(switcher.Command{At: c.now(), Kind: req.kind})
		case req := <-c.approvals:
			req.reply <- c.approve(req)
		case ev := <-c.results:
			c.handle(ev)
		case out <- head:
			c.queue = c.queue[1:]
		}
		c.publish()
	}
}

func (c *Core) onSample(s ingest.Sample) {
	c.sample = s
	before := c.judge.State()
	after := c.judge.Update(s)
	if after == before {
		return
	}
	c.log.Info("link changed", "from", before.String(), "to", after.String(), "kbps", s.Kbps)
	c.handle(switcher.CamerasChanged{At: s.At, States: map[string]scenemap.CameraState{legacyCamera: cameraState(after)}})
}

// legacyCamera は、第1段階の設定（メディアソース1本）を読み替えたカメラ1台の id。
const legacyCamera = "main"

func cameraState(l ingest.LinkState) scenemap.CameraState {
	switch l {
	case ingest.LinkOK:
		return scenemap.StateOK
	case ingest.LinkLow:
		return scenemap.StateLow
	}
	return scenemap.StateDown
}

// bridge は obs から届いた Event を、地図と UUID を持つ形に読み替える（M-8 までのつなぎ）。
// obs がまだ UUID と地図を流さない間は、シーン名を UUID の代わりに使い（古い OBS と同じ）、設定のシーンとメディアソースから
// 地図を作る: 待機・開始時・監視対象のシーンを登録し、監視対象のシーンにだけメディアソース（カメラ legacyCamera）を置く。
func (c *Core) bridge(ev switcher.Event) switcher.Event {
	switch e := ev.(type) {
	case switcher.OBSConnected:
		if e.SceneUUID == "" {
			e.SceneUUID = e.Scene
		}
		if e.Map == nil {
			e.Map = c.legacyMap()
		}
		return e
	case switcher.SceneChanged:
		if e.SceneUUID == "" {
			e.SceneUUID = e.Scene
		}
		return e
	}
	return ev
}

// legacyMap は設定から作る地図。UUID はシーン名・入力名そのもの。
func (c *Core) legacyMap() *scenemap.Map {
	m := &scenemap.Map{
		Scenes: map[string]scenemap.Container{},
		Groups: map[string]scenemap.Container{},
		Inputs: map[string]scenemap.Input{c.mediaSource: {Name: c.mediaSource, CameraID: legacyCamera}},
	}
	m.Scenes[c.standby] = scenemap.Container{Name: c.standby}
	if c.startScene != nil {
		m.Scenes[*c.startScene] = scenemap.Container{Name: *c.startScene}
	}
	for _, name := range c.monitored {
		m.Scenes[name] = scenemap.Container{Name: name, Items: []scenemap.Item{
			{ID: 1, SourceUUID: c.mediaSource, SourceKind: scenemap.InputSource, Enabled: true}}}
	}
	return m
}

// handle は Event を Machine に渡し、返ったバッチを実行待ちの列の後ろに足す。
func (c *Core) handle(ev switcher.Event) error {
	ev = c.bridge(ev)
	b, cmdErr := c.m.Handle(ev)
	if b != nil {
		c.queue = append(c.queue, b)
	}
	if bd, ok := ev.(switcher.BatchDone); ok {
		c.logBatchDone(bd)
	}
	if c.observe != nil {
		c.observe(ev)
	}
	return cmdErr
}

// approve は承認要求に答える。Replace の代わりのバッチは列の先頭に入れる。
func (c *Core) approve(req approveReq) switcher.ApproveResult {
	from := c.m.Snapshot().Scene
	res := c.m.Approve(req.batchID, req.action, req.override, c.now())
	switch r := res.(type) {
	case switcher.Proceed:
		if set, ok := req.action.(switcher.SetScene); ok {
			c.log.Info("scene switch",
				"from", from, "to", set.SceneUUID,
				"reason", c.reason(req.purpose), "kbps", c.sample.Kbps, "purpose", req.purpose.String())
		}
	case switcher.Replace:
		if r.Batch != nil {
			c.queue = append([]*switcher.Batch{r.Batch}, c.queue...)
			c.log.Info("batch replaced", "batch", req.batchID, "purpose", req.purpose.String(),
				"by", r.Batch.Purpose.String(), "link", c.judge.State().String())
		}
	case switcher.Reject:
		c.log.Info("batch rejected", "batch", req.batchID, "purpose", req.purpose.String())
	}
	if c.observe != nil {
		c.observe(approval{BatchID: req.batchID, Action: req.action, Result: res})
	}
	return res
}

// reason は切替の理由。自動切替は受信状態（low / down / ok）、配信開始時の切替は stream_start、
// それ以外は command。
func (c *Core) reason(p switcher.Purpose) string {
	switch {
	case p.IsAuto():
		return strings.ToLower(c.judge.State().String())
	case p == switcher.PurposeStartScene:
		return "stream_start"
	}
	return "command"
}

func (c *Core) logBatchDone(bd switcher.BatchDone) {
	switch {
	case bd.Result == switcher.BatchFailed:
		c.log.Warn("batch failed", "batch", bd.BatchID, "error", bd.Err)
	case bd.Err != nil:
		c.log.Warn("batch partly failed", "batch", bd.BatchID, "error", bd.Err)
	default:
		c.log.Debug("batch done", "batch", bd.BatchID, "result", int(bd.Result))
	}
}
