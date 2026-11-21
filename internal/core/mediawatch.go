package core

import (
	"context"
	"sort"
	"sync"

	"github.com/teru358/irl-live-keeper/internal/obs"
	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// requestWatch は cam のメディアソースの見張りを頼む（中核ループから。止まらない）。
// OBS につながっていない（地図の写しがない）とき、メディアソースがないときは頼まない。
func (c *Core) requestWatch(cam *camState) {
	if c.media == nil || !c.connected || c.mapCopy == nil {
		return
	}
	c.media.request(cam.cfg.ID, mediaSources(c.mapCopy, cam.cfg.ID))
}

// mediaSources は地図の入力のうち、camera の CameraID を持つもの（名前の順）。
func mediaSources(m *scenemap.Map, camera string) []obs.MediaSource {
	var out []obs.MediaSource
	for uuid, in := range m.Inputs {
		if in.CameraID == camera {
			out = append(out, obs.MediaSource{UUID: uuid, Name: in.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// mediaDispatcher は、中核ループの頼みを MediaWatcher に渡す別の goroutine（run）。
// 中核ループは mu の中で map に書いて notify に1つ入れる（満ちていれば捨てる）だけなので止まらない。
// 頼みはカメラごとに最新の1つだけを持つ（古い頼みは上書きされる）。run が MediaWatcher を呼ぶので、
// MediaWatcher が詰まっても中核ループは進む。
type mediaDispatcher struct {
	w      MediaWatcher
	notify chan struct{} // 容量1

	mu            sync.Mutex
	pendingCancel bool                         // 次の run で、先に CancelAll を呼ぶ
	pending       map[string][]obs.MediaSource // カメラ ID → 最新の頼み
}

func newMediaDispatcher(w MediaWatcher) *mediaDispatcher {
	return &mediaDispatcher{w: w, notify: make(chan struct{}, 1), pending: map[string][]obs.MediaSource{}}
}

func (d *mediaDispatcher) wake() {
	select {
	case d.notify <- struct{}{}:
	default:
	}
}

func (d *mediaDispatcher) request(camera string, sources []obs.MediaSource) {
	d.mu.Lock()
	d.pending[camera] = sources
	d.mu.Unlock()
	d.wake()
}

// cancelAll は、溜まっている頼みを捨て、見張りを全部取り消す。
func (d *mediaDispatcher) cancelAll() {
	d.mu.Lock()
	clear(d.pending)
	d.pendingCancel = true
	d.mu.Unlock()
	d.wake()
}

func (d *mediaDispatcher) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.notify:
		}
		d.mu.Lock()
		cancel := d.pendingCancel
		d.pendingCancel = false
		todo := d.pending
		d.pending = map[string][]obs.MediaSource{}
		d.mu.Unlock()
		if cancel {
			d.w.CancelAll()
		}
		ids := make([]string, 0, len(todo))
		for id := range todo {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			d.w.Watch(id, todo[id])
		}
	}
}
