package obs

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

const (
	// refreshDebounce は、地図のイベントを受けてから、まとめて読み直すまでの待ち。
	refreshDebounce = 300 * time.Millisecond
	// refreshRetry は、読み直しが失敗したときの、次までの待ち。
	refreshRetry = time.Second
	// refreshEvery は、イベントがなくても読み直す間隔（グループの中への項目の追加は通知されない。M-0）。
	refreshEvery = 30 * time.Second
)

// inputSettingsEv は InputSettingsChanged の、照合に使う部分。
type inputSettingsEv struct {
	uuid     string
	settings map[string]any
}

// refresher は地図の読み直しを担うエポックの goroutine 1本。補助の接続（Aux）で読む。
//
// 持つもの: cur（今の地図。この goroutine だけが読み書きする。流した地図は変更しない）。
// session の goroutine からは kick と inputSettings だけが呼ばれ、どちらも待たない。
// 結果は emitMap で流す。エポックは OBSConnected から OBSDisconnected までなので、「接続の世代」の
// 役目も果たす（切断のあとに、前の世代で始めた読み直しの結果は流れない）。
type refresher struct {
	w      *WS
	ep     *epoch
	req    mapRequester
	legacy bool
	cur    *scenemap.Map

	kickCh chan struct{} // 全部を読み直すきっかけ（容量1。重なったら1つ）
	sigCh  chan struct{} // InputSettingsChanged が届いた
	mu     sync.Mutex
	pend   []inputSettingsEv
}

func newRefresher(w *WS, ep *epoch, cur *scenemap.Map, legacy bool) *refresher {
	return &refresher{
		w: w, ep: ep, req: w.aux, legacy: legacy, cur: cur,
		kickCh: make(chan struct{}, 1), sigCh: make(chan struct{}, 1),
	}
}

// kick は、0.3秒まとめて全部を読み直すきっかけを置く。
func (r *refresher) kick() {
	select {
	case r.kickCh <- struct{}{}:
	default:
	}
}

// inputSettings は InputSettingsChanged を渡す。読み直しの最中でなければ、その入力の URL の照合だけを
// やり直す。読み直しの最中に届いたものは、終わったあとにもう一度全部を読む。
func (r *refresher) inputSettings(uuid string, settings map[string]any) {
	r.mu.Lock()
	r.pend = append(r.pend, inputSettingsEv{uuid, settings})
	r.mu.Unlock()
	select {
	case r.sigCh <- struct{}{}:
	default:
	}
}

func (r *refresher) takePending() []inputSettingsEv {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.pend
	r.pend = nil
	return p
}

func (r *refresher) run() {
	w, ctx := r.w, r.ep.ctx
	periodic := w.clock.After(refreshEvery)
	retry := false
	for {
		full := false
		if retry {
			retry = false
			select {
			case <-ctx.Done():
				return
			case <-w.clock.After(refreshRetry):
			}
			full = true
		} else {
			select {
			case <-ctx.Done():
				return
			case <-r.kickCh:
				select {
				case <-ctx.Done():
					return
				case <-w.clock.After(refreshDebounce):
				}
				full = true
			case <-r.sigCh:
				r.applySettings()
			case <-periodic:
				full = true
			}
		}
		if !full {
			continue
		}
		// ここまでに届いたきっかけは、これから読む内容に含まれる。
		select {
		case <-r.kickCh:
		default:
		}
		r.takePending()

		m, err := r.read(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.log.Warn("シーンの地図を読み直せない。1秒あけて読み直す", "err", err)
			retry = true
			continue
		}
		r.cur = m
		if !w.emitMap(r.ep, m) {
			return
		}
		periodic = w.clock.After(refreshEvery)
		// 読み直しの最中に InputSettingsChanged が届いたなら、読み取りの値が古いかもしれない。
		if len(r.takePending()) > 0 {
			r.kick()
		}
	}
}

func (r *refresher) read(ctx context.Context) (*scenemap.Map, error) {
	if r.legacy {
		return readSceneListOnly(ctx, r.req, true)
	}
	return readSceneMap(ctx, r.req, r.w.cameraPaths(), r.w.cfg.RTSPPort)
}

// applySettings は、届いた InputSettingsChanged のうち、地図で ffmpeg_source とわかっている入力のものだけを、
// イベントの inputSettings で URL の照合だけやり直す。照合が変わったら SceneMapChanged を1回流す。
func (r *refresher) applySettings() {
	var next *scenemap.Map
	for _, e := range r.takePending() {
		cur := r.cur
		if next != nil {
			cur = next
		}
		in, ok := cur.Inputs[e.uuid]
		if !ok {
			continue
		}
		id, legacy := matchInputSettings(e.settings, r.w.cameraPaths(), r.w.cfg.RTSPPort)
		if id == in.CameraID && legacy == in.LegacyCameraID {
			continue
		}
		if next == nil {
			cp := *r.cur // 流した地図は変更しない。変えるのは Inputs だけなので、そこだけ複製する
			cp.Inputs = maps.Clone(r.cur.Inputs)
			next = &cp
		}
		next.Inputs[e.uuid] = scenemap.Input{Name: in.Name, CameraID: id, LegacyCameraID: legacy}
	}
	if next == nil {
		return
	}
	r.cur = next
	r.w.emitMap(r.ep, next)
}

func (w *WS) cameraPaths() []scenemap.CameraPath {
	cams := make([]scenemap.CameraPath, len(w.cfg.Cameras))
	for i, c := range w.cfg.Cameras {
		cams[i] = scenemap.CameraPath{ID: c.ID, Path: c.cameraPath()}
	}
	return cams
}

// emitMap は読み直しの結果を流し、警告に反映する。切断のあとなら流さない。
func (w *WS) emitMap(ep *epoch, m *scenemap.Map) bool {
	if !w.emitEpoch(ep, switcher.SceneMapChanged{At: w.clock.Now(), Map: m}) {
		return false
	}
	w.setMapWarnings(m, "")
	return true
}

// setMapWarnings は、地図を読むたびに警告を作り直す。emptyReason があれば、読めずに空の地図で
// つないだこと。古い OBS と空の地図では、カメラごとの警告は出さない。
func (w *WS) setMapWarnings(m *scenemap.Map, emptyReason string) {
	var cams []string
	if emptyReason == "" && !w.isLegacy() {
		cams = cameraWarnings(w.cfg.Cameras, m)
	}
	w.mu.Lock()
	w.warnMap, w.warnCams = emptyReason, cams
	w.mu.Unlock()
}

func (w *WS) isLegacy() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.legacy
}
