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

// Camera は Core が扱うカメラ1台の設定。Deps.Cameras は設定の順。
type Camera struct {
	ID    string
	Name  string
	OnBad scenemap.Motion
	// Thresholds はこのカメラの Judge の閾値（共通の switch にカメラの上書きを合わせたもの）。
	Thresholds ingest.Thresholds
	// URLs はこのカメラの status に出す固定の URL（ID と Name は Core が入れる）。
	URLs CameraURLs
}

// Deps は Core が使う部品と設定。
type Deps struct {
	// Machine は New と Run の goroutine だけが触る。Run の間はほかから呼ばない。
	Machine *switcher.Machine
	OBS     obs.Client
	// StandbyScene、Cameras、AutoEnabled、StartScene はシーンとカメラの設定。New がループを始める前に
	// ScenesConfigured として Machine に1回渡す。StartScene は配信開始時の切替が無効なら nil。
	StandbyScene string
	Cameras      []Camera
	AutoEnabled  bool
	StartScene   *string
	// Samples は Poller の出力（全カメラの Sample の1組）。
	Samples <-chan ingest.SampleSet
	// MediaMTX は MediaMTX の状態を返す。nil なら status の mediamtx は空。
	MediaMTX func() supervisor.Status
	// Warnings は OBS の接続時に見つかった警告を返す（obs.WS.Warnings）。nil なら警告なし。
	Warnings func() []string
	Log      *slog.Logger
	// Tick は Tick の時刻の列。nil なら Run が1秒ごとの ticker を作る。
	Tick <-chan time.Time
	// Now は中核ループと実行役が使う時計。nil なら time.Now。
	Now func() time.Time
}

// camState は中核ループが持つカメラ1台の状態。
type camState struct {
	cfg    Camera
	judge  *ingest.Judge
	sample ingest.Sample // 最後の Sample
}

// itemKey は項目の識別（持ち主のシーンかグループの UUID と、項目の ID）。
type itemKey = scenemap.ItemKey

// Core は状態を1か所で持つ中核ループと、OBS の操作を行う実行役。
type Core struct {
	m        *switcher.Machine
	obs      obs.Client
	cams     []*camState
	samples  <-chan ingest.SampleSet
	mediamtx func() supervisor.Status
	warnings func() []string
	log      *slog.Logger
	tick     <-chan time.Time
	now      func() time.Time

	// 中核ループと実行役の間のチャネル。どれもバッファなし。
	batches   chan *switcher.Batch // 中核ループ → 実行役。列の先頭を1つだけ差し出す
	approvals chan approveReq      // 実行役 → 中核ループ
	results   chan switcher.Event  // 実行役 → 中核ループ。SceneRequestDone と BatchDone を送った順に届ける
	cmds      chan cmdReq          // Command → 中核ループ
	stopped   chan struct{}        // Run が戻ったら閉じる

	snap atomic.Pointer[Snapshot]

	// ここから下は中核ループの goroutine だけが触る。
	queue      []*switcher.Batch // 実行待ちのバッチ
	lastErr    *LastError
	prevSwErr  *switcher.ErrorInfo
	prevMtxErr string
	prevSw     *switcher.Snapshot // 前回の Snapshot を作ったときの switcher の状態。最初は nil

	// 地図の Enabled の引き継ぎ（カメラの spec 第3節）。
	connected bool          // OBS につながっているか（OBSConnected から OBSDisconnected まで）
	mapCopy   *scenemap.Map // Machine に渡した最後の地図の写し。ItemEnabledChanged も当てる。未接続なら nil
	// knownNames はシーンの UUID → 最後に分かっていた名前。切断しても消さない（切れている間の status とログに名前を出すため）。
	knownNames map[string]string
	unknown    map[itemKey]bool // 写しにない項目の ItemEnabledChanged の最後の値

	// observe はテスト用。中核ループが処理した Event と承認（approval）ごとに、中核ループの goroutine で呼ぶ。
	observe func(any)
}

// approveReq は実行役からの承認要求。reply は容量1。
type approveReq struct {
	batchID int
	purpose switcher.Purpose
	// badCamera は AutoFail のバッチの悪い代表のカメラ（ほかは空）。
	badCamera string
	action    switcher.Action
	override  switcher.Override
	reply     chan switcher.ApproveResult
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

// New は Core を作り、シーンとカメラの設定を Machine に渡す。最初の Snapshot もここで公開する。
func New(d Deps) *Core {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	c := &Core{
		m: d.Machine, obs: d.OBS,
		samples: d.Samples, mediamtx: d.MediaMTX, warnings: d.Warnings,
		log: d.Log, tick: d.Tick, now: d.Now,
		batches:   make(chan *switcher.Batch),
		approvals: make(chan approveReq),
		results:   make(chan switcher.Event),
		cmds:      make(chan cmdReq),
		stopped:   make(chan struct{}),
		unknown:   map[itemKey]bool{},
	}
	cfgs := make([]switcher.CameraConfig, len(d.Cameras))
	for i, cam := range d.Cameras {
		c.cams = append(c.cams, &camState{cfg: cam, judge: ingest.NewJudge(cam.Thresholds)})
		cfgs[i] = switcher.CameraConfig{ID: cam.ID, OnBad: cam.OnBad}
	}
	var start *string
	if d.StartScene != nil {
		v := *d.StartScene
		start = &v
	}
	// ループを始める前の最初の Event。チャネルは通さない。
	c.handle(switcher.ScenesConfigured{
		At:          c.now(),
		Standby:     d.StandbyScene,
		Cameras:     cfgs,
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
		case set, ok := <-samples:
			if !ok {
				samples = nil
				continue
			}
			c.onSamples(set)
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

// onSamples は全カメラの Sample の1組を取り込む。カメラごとの Judge を更新し、変わったカメラがあれば
// 全カメラの今の状態を CamerasChanged として1回だけ Machine に渡す（1台ずつ渡すと、途中の組み合わせで
// 誤って待機へ送るため）。Snapshot は呼び出し側がそのあと1回作る。
func (c *Core) onSamples(set ingest.SampleSet) {
	byID := make(map[string]ingest.Sample, len(set.Cameras))
	for _, s := range set.Cameras {
		byID[s.Camera] = s
	}
	changed := false
	for _, cam := range c.cams {
		s, ok := byID[cam.cfg.ID]
		if !ok {
			continue
		}
		cam.sample = s
		before := cam.judge.State()
		after := cam.judge.Update(s)
		if after == before {
			continue
		}
		changed = true
		c.log.Info("camera changed", "camera", cam.cfg.ID, "from", before.String(), "to", after.String(), "kbps", s.Kbps)
	}
	if !changed {
		return
	}
	states := make(map[string]scenemap.CameraState, len(c.cams))
	for _, cam := range c.cams {
		states[cam.cfg.ID] = cameraState(cam.judge.State())
	}
	c.handle(switcher.CamerasChanged{At: set.At, States: states})
}

// camByID は id のカメラ（なければ nil）。
func (c *Core) camByID(id string) *camState {
	for _, cam := range c.cams {
		if cam.cfg.ID == id {
			return cam
		}
	}
	return nil
}

func cameraState(l ingest.LinkState) scenemap.CameraState {
	switch l {
	case ingest.LinkOK:
		return scenemap.StateOK
	case ingest.LinkLow:
		return scenemap.StateLow
	}
	return scenemap.StateDown
}

// handle は Event を Machine に渡し、返ったバッチを実行待ちの列の後ろに足す。
// 地図に関わる Event（OBSConnected、OBSDisconnected、SceneMapChanged、ItemEnabledChanged）は、
// Machine に渡す前に prepareMap が写しと突き合わせる。切断中の地図と項目の Event はここで捨てる。
func (c *Core) handle(ev switcher.Event) error {
	ev, pass := c.prepareMap(ev)
	if !pass {
		return nil
	}
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

// prepareMap は、表示・非表示を本来の接続のイベントで正とするために、地図の Event を整える（カメラの spec 第3節）。
//   - OBSConnected: 新しい接続の最初の地図なので、そのまま使い、写しにする。覚えた値は空にする。
//   - OBSDisconnected: 写しと覚えた値を空にする。
//   - SceneMapChanged: 写しにある項目（同じシーンかグループの UUID と項目の ID）の Enabled は写しの値にし、
//     写しにない新しい項目には、覚えた値があれば重ねてから渡す。写しを渡した地図に替え、覚えた値は空にする。
//   - ItemEnabledChanged: 写しの項目に当てる。写しにない項目は、最後の値を覚える。どちらも Machine に1回渡す。
//
// 切断中の SceneMapChanged と ItemEnabledChanged（古い読み直しの結果と、旧い接続の項目のイベント）は、
// Machine に渡さず捨てる（表 MC-MAP-15、MC-ITEM-11）。
func (c *Core) prepareMap(ev switcher.Event) (switcher.Event, bool) {
	switch e := ev.(type) {
	case switcher.OBSConnected:
		c.connected = true
		c.mapCopy = cloneMap(e.Map)
		c.rememberNames(e.Map)
		clear(c.unknown)
	case switcher.OBSDisconnected:
		c.connected = false
		c.mapCopy = nil
		clear(c.unknown)
	case switcher.SceneMapChanged:
		if !c.connected || e.Map == nil {
			return ev, false
		}
		m := cloneMap(e.Map)
		overlayEnabled(m, c.mapCopy, c.unknown)
		c.mapCopy = cloneMap(m)
		c.rememberNames(m)
		clear(c.unknown)
		e.Map = m
		return e, true
	case switcher.ItemEnabledChanged:
		if !c.connected {
			return ev, false
		}
		if !setEnabled(c.mapCopy, e.SceneUUID, e.ItemID, e.Enabled) {
			c.unknown[itemKey{ContainerUUID: e.SceneUUID, ItemID: e.ItemID}] = e.Enabled
		}
	}
	return ev, true
}

// cloneMap は地図の複製（nil は nil）。項目の列も複製する。
func cloneMap(m *scenemap.Map) *scenemap.Map {
	if m == nil {
		return nil
	}
	c := &scenemap.Map{
		Collection: m.Collection,
		Scenes:     make(map[string]scenemap.Container, len(m.Scenes)),
		Groups:     make(map[string]scenemap.Container, len(m.Groups)),
		Inputs:     make(map[string]scenemap.Input, len(m.Inputs)),
	}
	for k, v := range m.Scenes {
		v.Items = slices.Clone(v.Items)
		c.Scenes[k] = v
	}
	for k, v := range m.Groups {
		v.Items = slices.Clone(v.Items)
		c.Groups[k] = v
	}
	for k, v := range m.Inputs {
		c.Inputs[k] = v
	}
	return c
}

// overlayEnabled は m の項目の Enabled を重ねる。old（写し）にある項目は old の値、ないものは unknown の値（あれば）。
func overlayEnabled(m, old *scenemap.Map, unknown map[itemKey]bool) {
	known := func(uuid string, id int) (bool, bool) {
		if old != nil {
			for _, set := range []map[string]scenemap.Container{old.Scenes, old.Groups} {
				if c, ok := set[uuid]; ok {
					for _, it := range c.Items {
						if it.ID == id {
							return it.Enabled, true
						}
					}
				}
			}
		}
		return false, false
	}
	for _, set := range []map[string]scenemap.Container{m.Scenes, m.Groups} {
		for uuid, c := range set {
			for i, it := range c.Items {
				if v, ok := known(uuid, it.ID); ok {
					c.Items[i].Enabled = v
				} else if v, ok := unknown[itemKey{ContainerUUID: uuid, ItemID: it.ID}]; ok {
					c.Items[i].Enabled = v
				}
			}
		}
	}
}

// setEnabled は写しの項目の Enabled を書き換える。写しにその項目があれば true。
func setEnabled(m *scenemap.Map, uuid string, id int, enabled bool) bool {
	if m == nil {
		return false
	}
	for _, set := range []map[string]scenemap.Container{m.Scenes, m.Groups} {
		if c, ok := set[uuid]; ok {
			for i, it := range c.Items {
				if it.ID == id {
					c.Items[i].Enabled = enabled
					return true
				}
			}
		}
	}
	return false
}

// sceneName は地図（写し）での UUID の名前。地図になければ最後に分かっていた名前、それもなければ UUID そのもの
// （古い OBS は名前を UUID の代わりに使う）。
func (c *Core) sceneName(uuid string) string {
	if c.mapCopy != nil {
		if sc, ok := c.mapCopy.Scenes[uuid]; ok && sc.Name != "" {
			return sc.Name
		}
	}
	if n, ok := c.knownNames[uuid]; ok {
		return n
	}
	return uuid
}

// sceneLabel は表示とログに使うシーンの名前。Machine が地図から引いた名前があればそれ、なければ sceneName。
func (c *Core) sceneLabel(uuid, name string) string {
	if name != "" {
		return name
	}
	return c.sceneName(uuid)
}

// rememberNames は地図のシーンの名前を knownNames に足す（上書きする。消えたシーンの名前は残す）。
func (c *Core) rememberNames(m *scenemap.Map) {
	if m == nil {
		return
	}
	if c.knownNames == nil {
		c.knownNames = make(map[string]string)
	}
	for uuid, sc := range m.Scenes {
		if sc.Name != "" {
			c.knownNames[uuid] = sc.Name
		}
	}
}

// approve は承認要求に答える。Replace の代わりのバッチは列の先頭に入れる。
func (c *Core) approve(req approveReq) switcher.ApproveResult {
	from := c.m.Snapshot().Scene
	res := c.m.Approve(req.batchID, req.action, req.override, c.now())
	switch r := res.(type) {
	case switcher.Proceed:
		if set, ok := req.action.(switcher.SetScene); ok {
			attrs := []any{"from", c.sceneName(from), "to", c.sceneName(set.SceneUUID), "reason", c.reason(req.purpose, req.badCamera)}
			// camera と kbps の代表: AutoFail は悪い代表のカメラ。AutoRecover は戻り先に映るカメラのうち kbps がいちばん低いもの
			// （同じなら設定の順で先。承認の時点の地図で）。
			rep := req.badCamera
			if req.purpose == switcher.PurposeAutoRecover {
				rep = c.lowestVisibleCamera(set.SceneUUID)
			}
			if cam := c.camByID(rep); cam != nil {
				attrs = append(attrs, "camera", cam.cfg.ID, "kbps", cam.sample.Kbps)
			}
			c.log.Info("scene switch", append(attrs, "purpose", req.purpose.String())...)
		}
	case switcher.Replace:
		if r.Batch != nil {
			c.queue = append([]*switcher.Batch{r.Batch}, c.queue...)
			c.log.Info("batch replaced", "batch", req.batchID, "purpose", req.purpose.String(),
				"by", r.Batch.Purpose.String(), "camera", r.Batch.BadCamera)
		}
	case switcher.Reject:
		c.log.Info("batch rejected", "batch", req.batchID, "purpose", req.purpose.String())
	}
	if c.observe != nil {
		c.observe(approval{BatchID: req.batchID, Action: req.action, Result: res})
	}
	return res
}

// lowestVisibleCamera は、sceneUUID に映るカメラのうち kbps がいちばん低いものの id（なければ空）。同じなら設定の順で先。
func (c *Core) lowestVisibleCamera(sceneUUID string) string {
	if c.mapCopy == nil {
		return ""
	}
	visible := scenemap.CamerasIn(*c.mapCopy, sceneUUID, nil, true)
	var best *camState
	for _, cam := range c.cams {
		if !slices.Contains(visible, cam.cfg.ID) {
			continue
		}
		if best == nil || cam.sample.Kbps < best.sample.Kbps {
			best = cam
		}
	}
	if best == nil {
		return ""
	}
	return best.cfg.ID
}

// reason は切替の理由。AutoFail は悪い代表のカメラの受信状態（low / down）、AutoRecover は ok、
// 配信開始時の切替は stream_start、それ以外は command。
func (c *Core) reason(p switcher.Purpose, badCamera string) string {
	switch {
	case p == switcher.PurposeAutoFail:
		if cam := c.camByID(badCamera); cam != nil {
			return strings.ToLower(cam.judge.State().String())
		}
		return "low"
	case p.IsAuto():
		return strings.ToLower(ingest.LinkOK.String())
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
