package core

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 隠す・戻す（0b）の中核ループ側の仕事: バッチの列の入れ方、hidden.json の保存、Stopping、ログ。

// isItemPurpose は項目の表示・非表示のバッチ（隠す・戻す・すべて戻す）か。
func isItemPurpose(p switcher.Purpose) bool {
	return p == switcher.PurposeHideCamera || p == switcher.PurposeShowCamera || p == switcher.PurposeRestoreAll
}

// enqueue は Handle が返したバッチを実行待ちの列に入れる。実行中のバッチは抜かない（列に入っているのは未着手のものだけ）。
//   - 停止用の RestoreAll は列の先頭（実行中のバッチの次）。
//   - シーンの切替のバッチは、実行待ちの隠す・戻すのバッチより前。
//   - 隠す・戻す、手動の RestoreAll は末尾。
func (c *Core) enqueue(b *switcher.Batch) {
	switch {
	case b.Purpose == switcher.PurposeRestoreAll && b.RestoreScope == switcher.RestoreTrackedItems:
		c.queue = append([]*switcher.Batch{b}, c.queue...)
	case isItemPurpose(b.Purpose):
		c.queue = append(c.queue, b)
	default:
		i := slices.IndexFunc(c.queue, func(q *switcher.Batch) bool { return isItemPurpose(q.Purpose) })
		if i < 0 {
			i = len(c.queue)
		}
		c.queue = slices.Insert(c.queue, i, b)
	}
}

// fixItemResult は、実行役が返した SetItemEnabled の結果を直す。要求を認めたあとに接続が切れた（または
// つなぎ直した）のに Error なら、適用されたかわからないので Timeout にする（spec 第7節「項目の期待」。
// obs は応答前に切れれば ErrTimeout を返すが、書き込みのエラーが先に返る競合があるため core でも直す）。
func (c *Core) fixItemResult(d switcher.ItemRequestDone) switcher.ItemRequestDone {
	seq, ok := c.itemSeq[d.ReqID]
	if !ok {
		return d
	}
	delete(c.itemSeq, d.ReqID)
	if d.Result == switcher.ItemError && seq != c.connSeq {
		d.Result = switcher.ItemTimeout
	}
	return d
}

// ---- hidden.json の保存 ----

// allEntries は保存する中身（今の Z の記録と、残した記録）。同じ項目の記録は今の Z の方を使う。
func allEntries(ms switcher.Snapshot) []switcher.HiddenEntry {
	out := make([]switcher.HiddenEntry, 0, len(ms.HiddenEntries)+len(ms.RetainedEntries))
	out = append(out, ms.HiddenEntries...)
	for _, r := range ms.RetainedEntries {
		if !slices.ContainsFunc(out, func(e switcher.HiddenEntry) bool { return hkey(e) == hkey(r) }) {
			out = append(out, r)
		}
	}
	return out
}

// hiddenKey は記録の識別（接続先、コレクション、項目）。
type hiddenKey struct {
	Host       string
	Port       int
	Collection string
	Item       itemKey
}

func indexEntries(es []switcher.HiddenEntry) map[hiddenKey]switcher.HiddenEntry {
	m := make(map[hiddenKey]switcher.HiddenEntry, len(es))
	for _, e := range es {
		m[hkey(e)] = e
	}
	return m
}

func hkey(e switcher.HiddenEntry) hiddenKey {
	return hiddenKey{e.Host, e.Port, e.Collection, itemKey{ContainerUUID: e.ContainerUUID, ItemID: e.ItemID}}
}

// persistSync は隠した項目の一覧を hidden.json に同期で書く。上限は1秒（超えたら失敗）。
// 最初の false（Proceed.MustPersist）の前と、Stopping の最後の保存に使う。書く場所がなければ何もしない。
func (c *Core) persistSync() error {
	if c.store == nil {
		return nil
	}
	ms := c.m.Snapshot()
	rev, data := ms.HiddenRevision, encodeHidden(allEntries(ms), c.now())
	done := make(chan error, 1)
	c.syncWG.Add(1)
	go func() {
		defer c.syncWG.Done()
		done <- c.store.put(rev, data)
	}()
	t := time.NewTimer(hiddenSyncLimit)
	defer t.Stop()
	select {
	case err := <-done:
		if err == nil {
			c.persistRev = rev
		}
		return err
	case <-t.C:
		c.store.failing.Store(true)
		return errors.New("hidden.json の書き込みが1秒以内に終わらない")
	}
}

// waitSyncWrites は Run の終わりに、同期の保存の goroutine を待つ（上限1秒。ディスクが固まっていれば残る）。
func (c *Core) waitSyncWrites() {
	ch := make(chan struct{})
	go func() { c.syncWG.Wait(); close(ch) }()
	select {
	case <-ch:
	case <-time.After(hiddenSyncLimit):
	}
}

// onHiddenChanged は記録が変わったとき（Snapshot を作るたびに HiddenRevision を見る）、保存を求め、
// 消えた記録をログに書く。中核ループは止まらない（書くのは persister）。
func (c *Core) onHiddenChanged(ms switcher.Snapshot) {
	if ms.HiddenRevision == c.persistRev {
		return
	}
	c.persistRev = ms.HiddenRevision
	es := allEntries(ms)
	c.logHiddenDrops(es)
	if c.pers != nil {
		c.pers.submit(ms.HiddenRevision, encodeHidden(es, c.now()))
	}
}

// logHiddenDrops は、前回の記録から消えたものを `hidden item dropped` に書く。本体が戻した項目（true を送った）と、
// false を送る前に予約を巻き戻しただけの記録は書かない。
func (c *Core) logHiddenDrops(es []switcher.HiddenEntry) {
	cur := make(map[hiddenKey]switcher.HiddenEntry, len(es))
	for _, e := range es {
		cur[hkey(e)] = e
	}
	for k, e := range c.prevHidden {
		if _, ok := cur[k]; ok {
			continue
		}
		if c.shownKeys[k.Item] {
			delete(c.shownKeys, k.Item)
			continue
		}
		if e.State == switcher.HiddenP && !c.falseSent[k.Item] {
			continue
		}
		c.log.Info("hidden item dropped", "camera", e.CameraID, "scene", c.sceneName(e.ContainerUUID), "item", e.ItemID, "state", e.State.String())
	}
	c.prevHidden = cur
}

// ---- ログ: camera hidden / camera shown ----

// itemLog はバッチごとの、送った項目の数（カメラごと）と理由。BatchDone で `camera hidden` / `camera shown` を書く。
type itemLog struct {
	hide   bool
	reason string
	byCam  map[string]int
}

// noteItemProceed は SetItemEnabled の承認を記録する（送信の数、ログ、中核ループ側の ReqID の接続の世代）。
func (c *Core) noteItemProceed(req approveReq, set switcher.SetItemEnabled, r switcher.Proceed, ms switcher.Snapshot) {
	k := itemKey{ContainerUUID: set.SceneUUID, ItemID: set.ItemID}
	c.itemSeq[r.ItemReqID] = c.connSeq
	if set.Enabled {
		c.shownKeys[k] = true
	} else {
		c.falseSent[k] = true
		delete(c.shownKeys, k)
	}
	l := c.itemLogs[req.batchID]
	if l == nil {
		l = &itemLog{hide: !set.Enabled, reason: c.itemReason(req, set.Enabled, ms), byCam: map[string]int{}}
		c.itemLogs[req.batchID] = l
	}
	cam := req.cameraID
	if cam == "" {
		cam = c.cameraOfItem(k, ms)
	}
	l.byCam[cam]++
}

// cameraOfItem は項目のカメラの id。記録にあればそれ、なければ地図の項目のソースから引く（なければ空）。
func (c *Core) cameraOfItem(k itemKey, ms switcher.Snapshot) string {
	for _, list := range [][]switcher.HiddenEntry{ms.HiddenEntries, ms.RetainedEntries} {
		for _, e := range list {
			if e.Key() == k {
				return e.CameraID
			}
		}
	}
	if c.mapCopy != nil {
		for _, set := range []map[string]scenemap.Container{c.mapCopy.Scenes, c.mapCopy.Groups} {
			if ct, ok := set[k.ContainerUUID]; ok {
				for _, it := range ct.Items {
					if it.ID == k.ItemID {
						return c.mapCopy.Inputs[it.SourceUUID].CameraID
					}
				}
			}
		}
	}
	return ""
}

// itemReason は `camera hidden` / `camera shown` の理由。
// hidden: low / down（カメラの受信状態）。shown: manual_all / shutdown / stream_stop / auto_off / config /
// studio_mode / ok / restore（設定にないカメラの記録を戻すなど）。
func (c *Core) itemReason(req approveReq, enabled bool, ms switcher.Snapshot) string {
	cam := c.camByID(req.cameraID)
	if !enabled {
		if cam != nil && cam.judge.State() == ingest.LinkLow {
			return "low"
		}
		return "down"
	}
	switch {
	case req.scope == switcher.RestoreAllCameraItems:
		return "manual_all"
	case req.purpose == switcher.PurposeRestoreAll:
		return "shutdown"
	case cam == nil:
		return "restore"
	case !ms.Streaming:
		return "stream_stop"
	case !ms.AutoEnabled:
		return "auto_off"
	case cam.cfg.OnBad != scenemap.MotionHide:
		return "config"
	case ms.StudioMode && c.studioHide != switcher.StudioHideHide:
		return "studio_mode"
	case cam.judge.State() == ingest.LinkOK:
		return "ok"
	}
	return "restore"
}

// logItemBatch はバッチの終わりに、送った項目の数をカメラごとに1行で書く。
func (c *Core) logItemBatch(bd switcher.BatchDone) {
	l := c.itemLogs[bd.BatchID]
	if l == nil {
		return
	}
	delete(c.itemLogs, bd.BatchID)
	msg := "camera shown"
	if l.hide {
		msg = "camera hidden"
	}
	cams := make([]string, 0, len(l.byCam))
	for id := range l.byCam {
		cams = append(cams, id)
	}
	slices.Sort(cams)
	for _, id := range cams {
		c.log.Info(msg, "camera", id, "items", l.byCam[id], "reason", l.reason)
	}
}

// ---- Stopping ----

// stopReq は Stopping の要求。done は終わりを知らせるために閉じる。
type stopReq struct{ done chan struct{} }

// stopState は Stopping の進み。
type stopState struct {
	started  bool
	waiting  bool // 停止用の RestoreAll の BatchDone を待っている
	finished bool
	batchID  int
	dones    []chan struct{}
}

// Stopping は本体の終了のとき、HTTP サーバーと MediaMTX を止める前に呼ぶ。switcher に Stopping を渡し、
// 隠している項目を表示に戻す RestoreAll の BatchDone（すべての要求に応答があったとき）か StopTimeout（既定2秒）まで
// 待つ。BatchDone のときは hidden.json を書いてから戻る。OBS につながっていなければすぐ戻る。
// 何度呼んでもよい（2回目以降は最初の終わりを待つだけ）。Run が止まっていれば何もせず戻る。
func (c *Core) Stopping(ctx context.Context) {
	req := stopReq{done: make(chan struct{})}
	t := time.NewTimer(c.stopTimeout)
	defer t.Stop()
	select {
	case c.stopCh <- req:
	case <-c.stopped:
		return
	case <-ctx.Done():
		return
	case <-t.C:
		return
	}
	select {
	case <-req.done:
	case <-c.stopped:
	case <-ctx.Done():
	case <-t.C:
	}
}

func (c *Core) beginStop(req stopReq) {
	if c.stop.finished {
		close(req.done)
		return
	}
	c.stop.dones = append(c.stop.dones, req.done)
	if c.stop.started {
		return
	}
	c.stop.started = true
	c.handle(switcher.Stopping{At: c.now()})
	for _, q := range c.queue {
		if q.Purpose == switcher.PurposeRestoreAll && q.RestoreScope == switcher.RestoreTrackedItems {
			c.stop.waiting, c.stop.batchID = true, q.BatchID
			return
		}
	}
	c.finishStop()
}

// onStopBatchDone は停止用の RestoreAll の BatchDone（完了でも却下でも）で待ちを終える。
func (c *Core) onStopBatchDone(bd switcher.BatchDone) {
	if c.stop.waiting && bd.BatchID == c.stop.batchID {
		c.finishStop()
	}
}

// finishStop は最後の保存（まだ書けていなければ）をして、待っている Stopping を戻す。
func (c *Core) finishStop() {
	c.stop.waiting, c.stop.finished = false, true
	if c.store != nil && int64(c.m.Snapshot().HiddenRevision) != c.store.writtenRev.Load() {
		if err := c.persistSync(); err != nil {
			c.log.Warn("終了前の hidden.json の保存に失敗しました", "error", err)
		}
	}
	for _, d := range c.stop.dones {
		close(d)
	}
	c.stop.dones = nil
}

// warnStudioMode は Studio Mode の間の隠す動きの文（spec 第1節）。on_bad=hide のカメラがなければ空。
func (c *Core) warnStudioMode(ms switcher.Snapshot) string {
	if !ms.Connected || !ms.StudioMode || !c.anyHide {
		return ""
	}
	switch strings.TrimSpace(c.studioHide) {
	case switcher.StudioHideIgnore:
		return "屋外配信では Studio Mode を勧めません。Studio Mode のため、隠す動きを止めています"
	case switcher.StudioHideHide:
		return "屋外配信では Studio Mode を勧めません。Studio Mode でも隠しています（OBS の『シーンを複製』を切っていないと配信に効きません）"
	}
	return "屋外配信では Studio Mode を勧めません。Studio Mode のため、隠す動きの代わりに待機シーンへ送っています"
}
