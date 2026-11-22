package switcher

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 0b の行と列のための部品。表の記号に合わせる: m/s、x=(A,2)、y=(D,1)、ソース is。
// 状態を直接作らず、Event と Approve の流れで状態を作る（流れ自体も検査になる）。

const hbHost = "obs.local"
const hbPort = 4455

var (
	kx = scenemap.ItemKey{ContainerUUID: camera, ItemID: 2}
	ky = scenemap.ItemKey{ContainerUUID: deskScene, ItemID: 1}
	// 0b の表の鍵の名前
	hbKeyNames   = map[scenemap.ItemKey]string{kx: "x", ky: "y", {ContainerUUID: camera, ItemID: 1}: "m1", {ContainerUUID: camera2, ItemID: 1}: "m2", {ContainerUUID: camera2, ItemID: 2}: "z"}
	hbSceneNames = map[string]string{standby: "S", camera: "A", camera2: "B", deskScene: "D", talk: "T", opening: "O", standby2: "S2"}
)

func keyName(k scenemap.ItemKey) string {
	if n, ok := hbKeyNames[k]; ok {
		return n
	}
	return fmt.Sprintf("%s#%d", k.ContainerUUID, k.ItemID)
}

// hbMap は 0b の表の地図: S 待機、A は m と s、B は m、D は s だけ、T はカメラなし。
func hbMap() *scenemap.Map {
	m := testMap()
	m.Inputs = map[string]scenemap.Input{
		inMain: {Name: "main-src", CameraID: camMain},
		inSub:  {Name: "sub-src", CameraID: camSub},
	}
	it := func(id int, in string) scenemap.Item {
		return scenemap.Item{ID: id, SourceUUID: in, SourceKind: scenemap.InputSource, Enabled: true}
	}
	put := func(scene string, items ...scenemap.Item) {
		c := m.Scenes[scene]
		c.Items = items
		m.Scenes[scene] = c
	}
	put(camera, it(1, inMain), it(2, inSub))
	put(camera2, it(1, inMain))
	put(deskScene, it(1, inSub))
	return m
}

type hb struct {
	t   *testing.T
	m   *Machine
	cfg ScenesConfigured
	mp  *scenemap.Map
	now int // ms
	// start は最初に接続するときの program のシーン。
	start string
	// studio は最初の接続の Studio Mode。
	studio bool
	// lastBatch は最後に返ったバッチ。
	lastBatch Batch
	// noConn なら最初の接続をしない（設定と状態だけ）。
	noConn bool
}

type hbOpt func(h *hb)

func hbStudioHide(v string) hbOpt { return func(h *hb) { h.cfg.StudioModeHide = v } }
func hbAutoOff() hbOpt            { return func(h *hb) { h.cfg.AutoEnabled = false } }
func hbSubStandby() hbOpt         { return func(h *hb) { h.cfg.Cameras[1].OnBad = scenemap.MotionStandby } }
func hbBothHide() hbOpt           { return func(h *hb) { h.cfg.Cameras[0].OnBad = scenemap.MotionHide } }
func hbAt(uuid string) hbOpt      { return func(h *hb) { h.start = uuid } }

// hbNoY は D から y の項目を消す（表の「y は地図に存在せず対象外」）。
func hbNoY() hbOpt {
	return func(h *hb) {
		c := h.mp.Scenes[deskScene]
		c.Items = nil
		h.mp.Scenes[deskScene] = c
	}
}
func hbNoConn() hbOpt           { return func(h *hb) { h.noConn = true } }
func hbStudio() hbOpt           { return func(h *hb) { h.studio = true } }
func hbStart(uuid string) hbOpt { return func(h *hb) { h.cfg.StartScene = sp(nameOf(uuid)) } }
func hbMapEdit(f func(m *scenemap.Map)) hbOpt {
	return func(h *hb) { f(h.mp) }
}

// newHB は表の既定の状態を作る: 接続・配信中、自動有効、Studio Mode オフ、現在 A、m=s=OK、一覧なし。
// 準備の時刻は 0。
func newHB(t *testing.T, opts ...hbOpt) *hb {
	t.Helper()
	h := &hb{t: t, mp: hbMap(), start: camera}
	h.m = New(Config{RestartWait: W, Endpoint: Endpoint{Host: hbHost, Port: hbPort}})
	h.cfg = ScenesConfigured{At: at(0), Standby: "Standby", AutoEnabled: true,
		Cameras: []CameraConfig{{ID: camMain, OnBad: scenemap.MotionStandby}, {ID: camSub, OnBad: scenemap.MotionHide}}}
	for _, o := range opts {
		o(h)
	}
	h.ev(h.cfg)
	h.ev(h.camsEv(0, OK, OK))
	if !h.noConn {
		h.connect(0, true, h.start, h.studio)
	}
	return h
}

func (h *hb) connect(ms int, streaming bool, scene string, studio bool) []Batch {
	return h.ev(OBSConnected{At: at(ms), Streaming: streaming, Scene: nameOf(scene), SceneUUID: scene, TransitionMs: ip(300),
		Map: cloneTestMap(h.mp), StudioMode: studio})
}

func (h *hb) camsEv(ms int, m, s scenemap.CameraState) Event {
	return CamerasChanged{At: at(ms), States: map[string]scenemap.CameraState{camMain: m, camSub: s}}
}

// ev は Event を流し、cmdErr がないことを確かめてバッチの列を返す。
func (h *hb) ev(e Event) []Batch {
	h.t.Helper()
	bs, err := h.m.Handle(e)
	if err != nil {
		h.t.Fatalf("%T: cmdErr %v", e, err)
	}
	if len(bs) > 0 {
		h.lastBatch = bs[len(bs)-1]
	}
	return bs
}

func (h *hb) evErr(e Event) ([]Batch, error) { return h.m.Handle(e) }

func (h *hb) cams(ms int, m, s scenemap.CameraState) []Batch {
	h.t.Helper()
	return h.ev(h.camsEv(ms, m, s))
}
func (h *hb) sIs(ms int, s scenemap.CameraState) []Batch { h.t.Helper(); return h.cams(ms, OK, s) }
func (h *hb) tick(ms int) []Batch                        { h.t.Helper(); return h.ev(Tick{At: at(ms)}) }
func (h *hb) scene(ms int, uuid string) []Batch          { h.t.Helper(); return h.ev(sc(ms, uuid)) }
func (h *hb) bdone(ms, id int, r BatchResult, err error) []Batch {
	h.t.Helper()
	return h.ev(BatchDone{At: at(ms), BatchID: id, Result: r, Err: err})
}
func (h *hb) itemEv(ms int, k scenemap.ItemKey, v bool) []Batch {
	h.t.Helper()
	return h.ev(ItemEnabledChanged{At: at(ms), SceneUUID: k.ContainerUUID, ItemID: k.ItemID, Enabled: v})
}
func (h *hb) itemDone(ms int, req uint64, r ItemResult) []Batch {
	h.t.Helper()
	return h.ev(ItemRequestDone{At: at(ms), ReqID: req, Result: r})
}

// ---- バッチの短い記述 ----

func letter(uuid string) string {
	if n, ok := hbSceneNames[uuid]; ok {
		return n
	}
	return uuid
}

func camLetter(c string) string {
	switch c {
	case camMain:
		return "m"
	case camSub:
		return "s"
	}
	return c
}

func inLetter(in string) string {
	switch in {
	case inMain:
		return "im"
	case inSub:
		return "is"
	}
	return in
}

// desc はバッチを表の書き方で表す。F(S)、R(A)、Hc(s)[x,y]、Sc(s)[R(is),x,y]、RA[x]、MA[x,y]。
func desc(b Batch) string {
	var parts []string
	for _, a := range b.Actions {
		switch a := a.(type) {
		case SetItemEnabled:
			parts = append(parts, keyName(scenemap.ItemKey{ContainerUUID: a.SceneUUID, ItemID: a.ItemID}))
		case RestartMedia:
			var ins []string
			for _, in := range a.Inputs {
				ins = append(ins, inLetter(in))
			}
			parts = append(parts, "R("+strings.Join(ins, "+")+")")
		case SetScene:
			parts = append(parts, letter(a.SceneUUID))
		case StartStream:
			parts = append(parts, "start")
		case StopStream:
			parts = append(parts, "stop")
		}
	}
	body := strings.Join(parts, ",")
	switch b.Purpose {
	case PurposeAutoFail:
		return "F(" + letter(b.ReturnScene) + ")"
	case PurposeAutoRecover:
		return "R(" + letter(b.Actions[len(b.Actions)-1].(SetScene).SceneUUID) + ")"
	case PurposeStandbyOffMonitored:
		return "M(" + letter(b.Actions[len(b.Actions)-1].(SetScene).SceneUUID) + ")"
	case PurposeStartScene:
		return "St(" + body + ")"
	case PurposeHideCamera:
		return "Hc(" + camLetter(b.CameraID) + ")[" + body + "]"
	case PurposeShowCamera:
		return "Sc(" + camLetter(b.CameraID) + ")[" + body + "]"
	case PurposeRestoreAll:
		if b.RestoreScope == RestoreAllCameraItems {
			return "MA[" + body + "]"
		}
		return "RA[" + body + "]"
	}
	return b.Purpose.String() + "[" + body + "]"
}

func descs(bs []Batch) string {
	var out []string
	for _, b := range bs {
		out = append(out, desc(b))
	}
	return strings.Join(out, " ")
}

// out は出力の列が want（空白区切り。空は出力なし）であることを確かめ、列を返す。
func (h *hb) out(bs []Batch, want string) []Batch {
	h.t.Helper()
	if got := descs(bs); got != want {
		h.t.Fatalf("出力: got %q, want %q", got, want)
	}
	return bs
}

// ---- 項目の承認 ----

func (h *hb) itemAct(k scenemap.ItemKey, v bool) Action {
	return SetItemEnabled{SceneUUID: k.ContainerUUID, ItemID: k.ItemID, Enabled: v}
}

// ap は SetItemEnabled(k, v) を承認する。
func (h *hb) ap(b Batch, ms int, k scenemap.ItemKey, v bool) ApproveResult {
	h.t.Helper()
	return h.m.Approve(b.BatchID, h.itemAct(k, v), OverrideNone{}, at(ms))
}

// proceed は承認が Proceed のとき、その結果を返す。
func (h *hb) proceed(b Batch, ms int, k scenemap.ItemKey, v bool) Proceed {
	h.t.Helper()
	r := h.ap(b, ms, k, v)
	p, ok := r.(Proceed)
	if !ok {
		h.t.Fatalf("Approve(%s, %s=%v) = %#v, want Proceed", desc(b), keyName(k), v, r)
	}
	if p.ItemReqID == 0 {
		h.t.Fatalf("Proceed.ItemReqID = 0")
	}
	return p
}

func (h *hb) want(r ApproveResult, k scenemap.ItemKey, b Batch, ms int, v bool, want ApproveResult) {
	h.t.Helper()
	if r != want {
		h.t.Fatalf("Approve(%s, %s=%v) at %d = %#v, want %#v", desc(b), keyName(k), v, ms, r, want)
	}
}

// hide は t=ms に s を DOWN にして、Hc(s)[x,y] を受け取る（m は OK のまま）。
func (h *hb) hide(ms int) Batch {
	h.t.Helper()
	bs := h.sIs(ms, DOWN)
	h.out(bs, "Hc(s)[x,y]")
	return bs[0]
}

// hideX は Hc を受け取り x を Proceed・OK・イベントまで進め、y は未送信 P のまま返す。
func (h *hb) xDone(ms int, b Batch) (req uint64) {
	h.t.Helper()
	p := h.proceed(b, ms, kx, false)
	return p.ItemReqID
}

// ---- 記録と期待の読み取り ----

func hbEntryString(es []HiddenEntry) string {
	var out []string
	for _, e := range es {
		out = append(out, keyName(e.Key())+":"+e.State.String())
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func (h *hb) entries() string  { return hbEntryString(h.m.st.Hidden) }
func (h *hb) retained() string { return hbEntryString(h.m.st.Retained) }

func (h *hb) wantEntries(want string) {
	h.t.Helper()
	if got := h.entries(); got != want {
		h.t.Fatalf("Hidden: got %q, want %q", got, want)
	}
}

func (h *hb) wantRetained(want string) {
	h.t.Helper()
	if got := h.retained(); got != want {
		h.t.Fatalf("Retained: got %q, want %q", got, want)
	}
}

func (h *hb) qs() string {
	var out []string
	for _, q := range h.m.st.ItemExpectations {
		out = append(out, fmt.Sprintf("%s:%v", keyName(q.Key), q.Value))
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func (h *hb) wantQ(want string) {
	h.t.Helper()
	if got := h.qs(); got != want {
		h.t.Fatalf("Q: got %q, want %q", got, want)
	}
}

func (h *hb) q(k scenemap.ItemKey, v bool) *ItemExpectation {
	for i := range h.m.st.ItemExpectations {
		q := &h.m.st.ItemExpectations[i]
		if q.Key == k && q.Value == v {
			return q
		}
	}
	return nil
}

func (h *hb) entry(k scenemap.ItemKey) *HiddenEntry {
	for i := range h.m.st.Hidden {
		if h.m.st.Hidden[i].Key() == k {
			return &h.m.st.Hidden[i]
		}
	}
	return nil
}

// mapEnabled は今の地図の項目の Enabled。項目がなければ ok=false。
func (h *hb) mapEnabled(k scenemap.ItemKey) (v, ok bool) {
	if h.m.st.Map == nil {
		return false, false
	}
	c, found := h.m.st.Map.Scenes[k.ContainerUUID]
	if !found {
		c, found = h.m.st.Map.Groups[k.ContainerUUID]
	}
	if !found {
		return false, false
	}
	for _, it := range c.Items {
		if it.ID == k.ItemID {
			return it.Enabled, true
		}
	}
	return false, false
}

func (h *hb) wantMap(k scenemap.ItemKey, want bool) {
	h.t.Helper()
	if v, ok := h.mapEnabled(k); !ok || v != want {
		h.t.Fatalf("地図 %s: got %v (ok=%v), want %v", keyName(k), v, ok, want)
	}
}

func (h *hb) wantU(c string, want bool) {
	h.t.Helper()
	if got := h.m.st.UserShown[c]; got != want {
		h.t.Fatalf("u(%s): got %v, want %v", c, got, want)
	}
}

func (h *hb) wantF(c string, want int) {
	h.t.Helper()
	if got := h.m.st.ItemFailures[c]; got != want {
		h.t.Fatalf("f(%s): got %d, want %d", c, got, want)
	}
}

func (h *hb) wantSent(c string, ms int) {
	h.t.Helper()
	got, ok := h.m.st.LastItemSent[c]
	if !ok || !got.Equal(at(ms)) {
		h.t.Fatalf("送出時刻(%s): got %s (ok=%v), want %s", c, show(got), ok, show(at(ms)))
	}
}

func (h *hb) wantNoSent(c string) {
	h.t.Helper()
	if got, ok := h.m.st.LastItemSent[c]; ok {
		h.t.Fatalf("送出時刻(%s): got %s, want none", c, show(got))
	}
}

func (h *hb) wantErrContains(sub string) {
	h.t.Helper()
	e := h.m.st.LastError
	if e == nil || !strings.Contains(e.Msg, sub) {
		h.t.Fatalf("LastError: got %s, want containing %q", show(e), sub)
	}
}

func (h *hb) wantNoErr() {
	h.t.Helper()
	if e := h.m.st.LastError; e != nil {
		h.t.Fatalf("LastError: got %s, want none", show(e))
	}
}

func (h *hb) wantRev(changed bool, before int) {
	h.t.Helper()
	if (h.m.st.HiddenRevision != before) != changed {
		h.t.Fatalf("HiddenRevision: before %d now %d, changed want %v", before, h.m.st.HiddenRevision, changed)
	}
}

// wantCams は現在のシーンの「映る」V と「実際に表示」A を確かめる。
func (h *hb) wantV(scene string, want ...string) {
	h.t.Helper()
	if got := h.m.visibleCameras(scene); !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
		h.t.Fatalf("V(%s): got %v, want %v", letter(scene), got, want)
	}
}

func (h *hb) wantA(scene string, want ...string) {
	h.t.Helper()
	if got := h.m.actualCameras(scene, nil); !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
		h.t.Fatalf("A(%s): got %v, want %v", letter(scene), got, want)
	}
}

func (h *hb) wantHealth(scene string, want scenemap.CameraState) {
	h.t.Helper()
	got, _, has := h.m.healthOf(scene)
	if !has || got != want {
		h.t.Fatalf("健康(%s): got %q (has=%v), want %q", letter(scene), got, has, want)
	}
}

func (h *hb) wantClass(scene string, want SceneClass) {
	h.t.Helper()
	if got := h.m.classOf(scene); got != want {
		h.t.Fatalf("分類(%s): got %v, want %v", letter(scene), got, want)
	}
}

func (h *hb) wantPendingNone() {
	h.t.Helper()
	if len(h.m.st.Pending) != 0 {
		h.t.Fatalf("Pending: got %v, want none", h.m.st.Pending)
	}
}

// ---- 流れの部品 ----

// sendFalse は b の項目 k の false を承認し、OK を返し、false のイベントを流す（x が H になる）。
func (h *hb) sendFalse(b Batch, ms int, k scenemap.ItemKey) uint64 {
	h.t.Helper()
	p := h.proceed(b, ms, k, false)
	h.itemDone(ms, p.ItemReqID, ItemOK)
	h.itemEv(ms, k, false)
	return p.ItemReqID
}

// sendTrue は true を承認し、OK とイベントまで流す。
func (h *hb) sendTrue(b Batch, ms int, k scenemap.ItemKey) uint64 {
	h.t.Helper()
	p := h.proceed(b, ms, k, true)
	h.itemDone(ms, p.ItemReqID, ItemOK)
	h.itemEv(ms, k, true)
	return p.ItemReqID
}

// hiddenX は s DOWN → Hc を x だけ H にして、y は予約のままにする前の段階までは進めない。
// x と y の両方を H にして Hc を完了させ、Hide が終わった状態にする（t0 から数ミリ秒で進める）。
func (h *hb) hiddenBoth(ms int) {
	h.t.Helper()
	b := h.hide(ms)
	h.sendFalse(b, ms+100, kx)
	h.sendFalse(b, ms+100, ky)
	h.bdone(ms+100, b.BatchID, BatchCompleted, nil)
	h.wantEntries("x:H,y:H")
}

func hbErrIs(err error, target any) bool {
	switch target.(type) {
	case ErrNotConnected:
		var e ErrNotConnected
		return errors.As(err, &e)
	case ErrInvalidState:
		var e ErrInvalidState
		return errors.As(err, &e)
	}
	return false
}

func (h *hb) wantReason(err error, reason string) {
	h.t.Helper()
	var e ErrInvalidState
	if !errors.As(err, &e) || e.Reason != reason {
		h.t.Fatalf("cmdErr: got %v, want ErrInvalidState{%s}", err, reason)
	}
}

var _ = time.Second

// yOff は y を地図 false の一覧外項目にする（人が隠した。表の「y は地図 false の一覧外項目」）。
func (h *hb) yOff() {
	h.t.Helper()
	setItem(h.mp, ky.ContainerUUID, ky.ItemID, false)
	h.out(h.itemEv(5, ky, false), "")
}

// seed は一覧に項目の記録を直接入れる（再起動で読み込んだ記録と同じ形）。map が true のままのときは map は変えない。
func (h *hb) seed(state HiddenState, mapFalse bool, keys ...scenemap.ItemKey) {
	h.t.Helper()
	// 先に地図の値を全部変え、そのあとで記録を足す（途中に判断が入って Show が出ないように）。
	if mapFalse {
		for _, k := range keys {
			setItem(h.mp, k.ContainerUUID, k.ItemID, false)
			h.itemEv(5, k, false)
		}
	}
	for _, k := range keys {
		h.m.st.Hidden = append(h.m.st.Hidden, h.entryOf(k, state))
	}
	h.m.sortHidden()
}

func (h *hb) seedH(keys ...scenemap.ItemKey) { h.t.Helper(); h.seed(HiddenH, true, keys...) }
func (h *hb) seedU(keys ...scenemap.ItemKey) { h.t.Helper(); h.seed(HiddenU, true, keys...) }

func (h *hb) entryOf(k scenemap.ItemKey, st HiddenState) HiddenEntry {
	return HiddenEntry{CameraID: camSub, Host: hbHost, Port: hbPort, Collection: collectionX,
		ContainerUUID: k.ContainerUUID, ItemID: k.ItemID, SourceUUID: inSub, State: st, WrittenAt: at(5)}
}

// setMap は今の地図の項目の Enabled を直接書く（core が引き継ぐ値を作る）。
func (h *hb) setMap(k scenemap.ItemKey, v bool) {
	h.t.Helper()
	c := h.m.st.Map.Scenes[k.ContainerUUID]
	items := slices.Clone(c.Items)
	for i := range items {
		if items[i].ID == k.ItemID {
			items[i].Enabled = v
		}
	}
	c.Items = items
	h.m.st.Map.Scenes[k.ContainerUUID] = c
}

// hcX は y を地図 false にして s DOWN にし、Hc(s)[x] を受け取る。
func (h *hb) hcX(ms int) Batch {
	h.t.Helper()
	h.yOff()
	bs := h.sIs(ms, DOWN)
	h.out(bs, "Hc(s)[x]")
	return bs[0]
}

// pX は hcX に続けて x の false を Proceed する（P(x)、Q(x,false)）。返すのは Hc と ReqID。
func (h *hb) pX(ms int) (Batch, uint64) {
	h.t.Helper()
	b := h.hcX(ms)
	p := h.proceed(b, ms+100, kx, false)
	return b, p.ItemReqID
}

// disconnectReconnect は切断して、同じ Z に再接続する（地図は fixture のまま。h.mp は変えない）。
func (h *hb) reconnect(ms int, streaming bool) []Batch {
	h.t.Helper()
	h.ev(OBSDisconnected{At: at(ms)})
	return h.connect(ms+1, streaming, camera, false)
}

// 短い書き方（scenemap の型）。
type scenemapMap = scenemap.Map

func scenemapKey(c string, id int) scenemap.ItemKey {
	return scenemap.ItemKey{ContainerUUID: c, ItemID: id}
}
func scenemapContainer(name string, items ...scenemap.Item) scenemap.Container {
	return scenemap.Container{Name: name, Items: items}
}
func scenemapItem(id int, src string, enabled bool) scenemap.Item {
	return scenemap.Item{ID: id, SourceUUID: src, SourceKind: scenemap.InputSource, Enabled: enabled}
}
func scenemapGroupItem(id int, src string, enabled bool) scenemap.Item {
	return scenemap.Item{ID: id, SourceUUID: src, SourceKind: scenemap.GroupSource, Enabled: enabled}
}

// pXKeep は yOff なしで（y も対象）Hc[x,y] を受け取り、x を Proceed する。
func (h *hb) pXKeep() (Batch, uint64) {
	h.t.Helper()
	b := h.hide(1000)
	return b, h.proceed(b, 1100, kx, false).ItemReqID
}

func (h *hb) smc(ms int, m *scenemap.Map) []Batch {
	h.t.Helper()
	return h.ev(SceneMapChanged{At: at(ms), Map: m})
}

func (h *hb) noLastAuto() {
	h.t.Helper()
	if len(h.m.st.LastAutoSent) != 0 {
		h.t.Fatalf("LastAutoSent: %v, want empty", h.m.st.LastAutoSent)
	}
}

// sendTrueOut は true の承認・OK・イベントを流し、イベントで出た列が want であることを確かめる。
func (h *hb) sendTrueOut(b Batch, ms int, k scenemap.ItemKey, want string) []Batch {
	h.t.Helper()
	p := h.proceed(b, ms, k, true)
	h.itemDone(ms, p.ItemReqID, ItemOK)
	bs := h.itemEv(ms, k, true)
	h.out(bs, want)
	return bs
}

// sendFalseOut は false の承認・OK・イベントを流し、応答とイベントで出た列（合わせて）が want であることを確かめる。
func (h *hb) sendFalseOut(b Batch, ms int, k scenemap.ItemKey, want string) []Batch {
	h.t.Helper()
	p := h.proceed(b, ms, k, false)
	bs := h.itemDone(ms, p.ItemReqID, ItemOK)
	bs = append(bs, h.itemEv(ms, k, false)...)
	h.out(bs, want)
	return bs
}
