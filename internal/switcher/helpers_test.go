package switcher

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// テストの時刻はすべて t0 からのミリ秒で書く。
var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

// W は RestartMedia.Wait。テストでは 0.5秒とする。
const W = 500 * time.Millisecond

// 受信状態（カメラの状態）の短い名前。
const (
	OK   = scenemap.StateOK
	LOW  = scenemap.StateLow
	DOWN = scenemap.StateDown
)

// 例に使うシーンの UUID（表の S、S2、A、B、T、O、D）。名前は sceneNames で別に持つ（UUID と名前は違う文字列）。
const (
	standby   = "uuid-standby"
	standby2  = "uuid-standby2"
	camera    = "uuid-camera"
	camera2   = "uuid-camera2"
	talk      = "uuid-talk"
	opening   = "uuid-opening"
	deskScene = "uuid-desk"
)

// カメラの id（表の m、s、d）と入力の UUID（im、is、id）。
const (
	camMain = "main"
	camSub  = "sub"
	camDesk = "desk"
	inMain  = "in-main"
	inMain2 = "in-main2"
	inSub   = "in-sub"
	inDesk  = "in-desk"
)

const collectionX = "collection-x"

var sceneNames = map[string]string{
	standby: "Standby", standby2: "Standby2", camera: "Camera", camera2: "Camera2",
	talk: "Talk", opening: "Opening", deskScene: "Desk",
}

// nameOf は例のシーンの UUID から設定名・表示名を返す。
func nameOf(uuid string) string {
	if n, ok := sceneNames[uuid]; ok {
		return n
	}
	return uuid
}

// 単独のカメラ（main）の既定の地図。S、S2、A、B、T、O、D を登録し、visible のシーンにだけ main の入力の項目を置く。
func testMap(visible ...string) *scenemap.Map {
	m := &scenemap.Map{
		Collection: collectionX,
		Scenes:     map[string]scenemap.Container{},
		Groups:     map[string]scenemap.Container{},
		Inputs:     map[string]scenemap.Input{inMain: {Name: "main-src", CameraID: camMain}},
	}
	for uuid, name := range sceneNames {
		m.Scenes[uuid] = scenemap.Container{Name: name}
	}
	for _, v := range visible {
		c := m.Scenes[v]
		c.Items = []scenemap.Item{{ID: 1, SourceUUID: inMain, SourceKind: scenemap.InputSource, Enabled: true}}
		m.Scenes[v] = c
	}
	return m
}

// 複数のカメラの地図（別の fixture）。A は m と s、B は m、D は d が映る。S、S2、T、O はカメラなし。
func multiMap() *scenemap.Map {
	m := testMap()
	m.Inputs = map[string]scenemap.Input{
		inMain: {Name: "main-src", CameraID: camMain},
		inSub:  {Name: "sub-src", CameraID: camSub},
		inDesk: {Name: "desk-src", CameraID: camDesk},
	}
	item := func(id int, in string) scenemap.Item {
		return scenemap.Item{ID: id, SourceUUID: in, SourceKind: scenemap.InputSource, Enabled: true}
	}
	put := func(scene string, items ...scenemap.Item) {
		c := m.Scenes[scene]
		c.Items = items
		m.Scenes[scene] = c
	}
	put(camera, item(1, inMain), item(2, inSub))
	put(camera2, item(1, inMain))
	put(deskScene, item(1, inDesk))
	return m
}

// editMap は地図の複製に edit を当てて返す。
func editMap(m *scenemap.Map, edit func(m *scenemap.Map)) *scenemap.Map {
	c := cloneTestMap(m)
	edit(c)
	return c
}

// setItem は scene の項目 id の Enabled を変える。
func setItem(m *scenemap.Map, container string, id int, enabled bool) {
	c := m.Scenes[container]
	if _, ok := m.Scenes[container]; !ok {
		c = m.Groups[container]
	}
	items := append([]scenemap.Item(nil), c.Items...)
	for i := range items {
		if items[i].ID == id {
			items[i].Enabled = enabled
		}
	}
	c.Items = items
	if _, ok := m.Scenes[container]; ok {
		m.Scenes[container] = c
	} else {
		m.Groups[container] = c
	}
}

func cloneTestMap(m *scenemap.Map) *scenemap.Map {
	if m == nil {
		return nil
	}
	c := &scenemap.Map{Collection: m.Collection,
		Scenes: map[string]scenemap.Container{}, Groups: map[string]scenemap.Container{}, Inputs: map[string]scenemap.Input{}}
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

// resolveNames は期待の状態の StandbyUUID と StartSceneUUID を、地図の名前から引き直す。
func resolveNames(s *state) {
	find := func(name string) string {
		if s.Map == nil {
			return ""
		}
		for uuid, c := range s.Map.Scenes {
			if c.Name == name {
				return uuid
			}
		}
		return ""
	}
	s.StandbyUUID = find(s.Standby)
	s.StartSceneUUID = ""
	if s.StartScene != nil {
		s.StartSceneUUID = find(*s.StartScene)
	}
}

// handle1 は Handle を呼び、返った列が高々1本なら *Batch に戻す（形の変更だけのための包み）。
func handle1(m *Machine, ev Event) (*Batch, error) {
	bs, err := m.Handle(ev)
	switch len(bs) {
	case 0:
		return nil, err
	case 1:
		b := bs[0]
		return &b, err
	}
	panic(fmt.Sprintf("handle1: %d batches", len(bs)))
}

func newMachine() *Machine { return New(Config{RestartWait: W}) }

func ip(v int) *int       { return &v }
func sp(v string) *string { return &v }

// actsFor は Purpose と切替先ごとの Action の列（期待値）。再起動の対象は main の入力だけ。
func actsFor(p Purpose, to string) []Action { return actsForIn(p, to, []string{inMain}) }

// actsForIn は再起動の対象を指定する actsFor。
func actsForIn(p Purpose, to string, inputs []string) []Action {
	switch p {
	case PurposeAutoFail, PurposeStandbyOn, PurposeStandbyOffOther, PurposeStartScene:
		return []Action{SetScene{SceneUUID: to}}
	case PurposeAutoRecover, PurposeStandbyOffMonitored:
		if len(inputs) == 0 {
			return []Action{SetScene{SceneUUID: to}}
		}
		return []Action{RestartMedia{Inputs: inputs, Wait: W}, SetScene{SceneUUID: to}}
	case PurposeStartStream:
		return []Action{StartStream{}}
	case PurposeStopStream:
		return []Action{StopStream{}}
	}
	panic("unknown purpose")
}

func set(scene string) Action { return SetScene{SceneUUID: scene} }

var restart Action = RestartMedia{Inputs: []string{inMain}, Wait: W}

// restartOf は再起動の対象を指定した RestartMedia。
func restartOf(inputs ...string) Action { return RestartMedia{Inputs: inputs, Wait: W} }

// ---- Event の短い書き方 ----

// lc は単独のカメラ main の状態の組（全カメラの組）。
func lc(ms int, s scenemap.CameraState) Event {
	return CamerasChanged{At: at(ms), States: map[string]scenemap.CameraState{camMain: s}}
}

// cc は複数のカメラ m、s、d の状態の組。
func cc(ms int, m, s, d scenemap.CameraState) Event {
	return CamerasChanged{At: at(ms), States: map[string]scenemap.CameraState{camMain: m, camSub: s, camDesk: d}}
}
func sc(ms int, scene string) Event {
	return SceneChanged{At: at(ms), Scene: nameOf(scene), SceneUUID: scene}
}
func tick(ms int) Event               { return Tick{At: at(ms)} }
func ssc(ms int, v bool) Event        { return StreamStateChanged{At: at(ms), Streaming: v} }
func disc(ms int) Event               { return OBSDisconnected{At: at(ms)} }
func cmd(ms int, k CommandKind) Event { return Command{At: at(ms), Kind: k} }
func tc(ms int, tr *int) Event        { return TransitionChanged{At: at(ms), TransitionMs: tr} }
func conn(ms int, streaming bool, scene string, tr *int) Event {
	return OBSConnected{At: at(ms), Streaming: streaming, Scene: nameOf(scene), SceneUUID: scene, TransitionMs: tr,
		Map: testMap(camera, camera2)}
}

// connM は複数のカメラの地図 m つきの OBSConnected。
func connM(ms int, streaming bool, scene string, m *scenemap.Map) Event {
	return OBSConnected{At: at(ms), Streaming: streaming, Scene: nameOf(scene), SceneUUID: scene, TransitionMs: ip(300), Map: m}
}

// smc は地図の差し替え。visible のシーンにだけ main の項目を置いた単独のカメラの地図にする。
func smc(ms int, visible ...string) Event {
	return SceneMapChanged{At: at(ms), Map: testMap(visible...)}
}
func smcMap(ms int, m *scenemap.Map) Event { return SceneMapChanged{At: at(ms), Map: m} }
func item(ms int, scene string, id int, enabled bool) Event {
	return ItemEnabledChanged{At: at(ms), SceneUUID: scene, ItemID: id, Enabled: enabled}
}
func srd(ms, req int, r RequestResult) Event {
	return SceneRequestDone{At: at(ms), ReqID: req, Result: r}
}
func bd(ms, id int, r BatchResult) Event { return BatchDone{At: at(ms), BatchID: id, Result: r} }

// cfgMod は設定 CFG の1項目を変える。
type cfgMod func(*ScenesConfigured)

func withAuto(v bool) cfgMod { return func(c *ScenesConfigured) { c.AutoEnabled = v } }

// withStart と withStandby は UUID から設定の名前に直す。
func withStart(uuid string) cfgMod {
	return func(c *ScenesConfigured) { c.StartScene = sp(nameOf(uuid)) }
}
func withStandby(uuid string) cfgMod { return func(c *ScenesConfigured) { c.Standby = nameOf(uuid) } }

// withHide は main の on_bad を hide にする（0a の結果は standby と同じ）。設定の変更の行で使う。
func withHide() cfgMod {
	return func(c *ScenesConfigured) { c.Cameras = []CameraConfig{{ID: camMain, OnBad: scenemap.MotionHide}} }
}

// withCameras は設定のカメラの一覧を差し替える。
func withCameras(cams ...CameraConfig) cfgMod {
	return func(c *ScenesConfigured) { c.Cameras = cams }
}

func cam(id string) CameraConfig { return CameraConfig{ID: id, OnBad: scenemap.MotionStandby} }

// withMulti は複数のカメラ m、s、d の設定にする。
func withMulti() cfgMod { return withCameras(cam(camMain), cam(camSub), cam(camDesk)) }

func cfgOf(ms int, mods ...cfgMod) ScenesConfigured {
	c := ScenesConfigured{At: at(ms), Standby: "Standby", Cameras: []CameraConfig{cam(camMain)}, AutoEnabled: true}
	for _, f := range mods {
		f(&c)
	}
	return c
}
func cfg(ms int, mods ...cfgMod) Event { return cfgOf(ms, mods...) }

// ---- 状態の複製と比較 ----

func cloneState(s state) state {
	c := s
	c.Cameras = slices.Clone(s.Cameras)
	if s.StartScene != nil {
		c.StartScene = sp(*s.StartScene)
	}
	if s.TransitionMs != nil {
		c.TransitionMs = ip(*s.TransitionMs)
	}
	c.Map = cloneTestMap(s.Map)
	c.CameraStates = make(map[string]scenemap.CameraState, len(s.CameraStates))
	for k, v := range s.CameraStates {
		c.CameraStates[k] = v
	}
	c.Expectations = slices.Clone(s.Expectations)
	c.Pending = make(map[int]PendingBatch, len(s.Pending))
	for k, v := range s.Pending {
		c.Pending[k] = v
	}
	c.LastAutoSent = make(map[Purpose]time.Time, len(s.LastAutoSent))
	for k, v := range s.LastAutoSent {
		c.LastAutoSent[k] = v
	}
	return c
}
func normState(s state) state {
	c := cloneState(s)
	if len(c.Expectations) == 0 {
		c.Expectations = nil
	}
	return c
}

// diffState は2つの状態の違うフィールドを並べる。同じなら空文字列。
func diffState(got, want state) string {
	g, w := normState(got), normState(want)
	gv, wv := reflect.ValueOf(g), reflect.ValueOf(w)
	var b strings.Builder
	for i := 0; i < gv.NumField(); i++ {
		if !reflect.DeepEqual(gv.Field(i).Interface(), wv.Field(i).Interface()) {
			fmt.Fprintf(&b, "\n  %s: got %s, want %s", gv.Type().Field(i).Name,
				show(gv.Field(i).Interface()), show(wv.Field(i).Interface()))
		}
	}
	return b.String()
}

func show(v any) string {
	switch x := v.(type) {
	case *int:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("&%d", *x)
	case *string:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("&%q", *x)
	case *ErrorInfo:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("&{%s %q}", show(x.At), x.Msg)
	case *scenemap.Map:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("%+v", *x)
	case time.Time:
		if x.IsZero() {
			return "zero"
		}
		return fmt.Sprintf("t0+%v", x.Sub(t0))
	case []Expectation:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = showExp(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[Purpose]time.Time:
		parts := []string{}
		for k, t := range x {
			parts = append(parts, fmt.Sprintf("%v:%s", k, show(t)))
		}
		slices.Sort(parts)
		return "{" + strings.Join(parts, " ") + "}"
	}
	return fmt.Sprintf("%+v", v)
}

func showExp(e Expectation) string {
	return fmt.Sprintf("{r%d %q %v ret=%q cfg=%d reg=%s ov=%T%+v tr=%s resp=%v at=%s res=%v dl=%s}",
		e.ReqID, e.Scene, e.Purpose, e.ReturnScene, e.ConfigEpoch, show(e.RegisteredAt), e.Override, e.Override,
		show(e.TransitionMs), e.Responded, show(e.RespondedAt), e.Result, show(e.Deadline))
}

// ---- 状態を直接作る仕組み ----

// baseState は既定の前提の状態: 設定 CFG（カメラ main 1台）、接続、配信中、地図 testMap(A, B)、現在 A（Monitored）、
// main=OK、持ち主 none、未完了なし、期待なし。Epoch と通し番号は 0 / 1 以外にしておき、動いたかがわかるようにする。
func baseState(s *state) {
	s.Standby = "Standby"
	s.Cameras = []CameraConfig{cam(camMain)}
	s.AutoEnabled = true
	s.StartScene = nil
	s.ConfigEpoch = 5
	s.Map = testMap(camera, camera2)
	resolveNames(s)
	s.CameraStates = map[string]scenemap.CameraState{camMain: OK}
	s.Connected = true
	s.Streaming = true
	s.Scene = camera
	s.PrevConnScene = talk
	s.PrevConnStreaming = false
	s.PrevConnCollection = collectionX
	s.TransitionMs = ip(300)
	s.SceneEpoch = 3
	s.ConnEpoch = 2
	s.StreamEpoch = 4
	s.NextBatchID = 10
	s.NextReqID = 20
}

// baseMulti は複数のカメラ（m、s、d）の既定の状態: 地図 multiMap、現在 A、全カメラ OK。ほかは baseState と同じ。
func baseMulti(s *state) {
	baseState(s)
	s.Cameras = []CameraConfig{cam(camMain), cam(camSub), cam(camDesk)}
	s.Map = multiMap()
	resolveNames(s)
	s.CameraStates = map[string]scenemap.CameraState{camMain: OK, camSub: OK, camDesk: OK}
}

// inStandby は待機シーンにいて、持ち主 o、戻り先 ret の状態にする。
func inStandby(s *state, o Owner, ret string) {
	s.Scene = s.StandbyUUID
	s.StandbyOwner = o
	s.ReturnScene = ret
}

// clearOwner は持ち主と戻り先を消す。
func clearOwner(s *state) {
	s.StandbyOwner = OwnerNone
	s.ReturnScene = ""
}

// pend は今の Epoch で未完了のバッチを足す。ret はバッチが持つ戻り先。
func pend(s *state, id int, p Purpose, ret string) {
	s.Pending[id] = PendingBatch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch,
		ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch, ReturnScene: ret}
}

// waiting は今の設定の世代で登録した、応答待ちの期待を返す。
func waiting(s *state, req int, scene string, p Purpose, ret string) Expectation {
	return Expectation{ReqID: req, Scene: scene, Purpose: p, ReturnScene: ret, ConfigEpoch: s.ConfigEpoch,
		RegisteredAt: at(0), Override: OverrideNone{}, TransitionMs: ip(300)}
}

// answered は x を応答済みにする。
func answered(x Expectation, res RequestResult, respMs, deadlineMs int) Expectation {
	x.Responded = true
	x.RespondedAt = at(respMs)
	x.Result = res
	x.Deadline = at(deadlineMs)
	return x
}

// addExp は応答済み（期限は遠い先）の期待を足す。
func addExp(s *state, req int, scene string, p Purpose, ret string) {
	s.Expectations = append(s.Expectations, answered(waiting(s, req, scene, p, ret), RequestOK, 0, 60000))
}

// manualChange は手動の SceneChanged の共通の変化。
func manualChange(s *state, scene string) {
	s.Scene = scene
	s.SceneEpoch++
	s.Expectations = nil
	s.LastAutoSent = map[Purpose]time.Time{}
	clearOwner(s)
}

// wantB はバッチを出したときの共通の変化を期待の状態に当てはめ、返るべきバッチを返す。
// inputs は再起動の対象（nil なら main の入力）、bad は AutoFail の悪い代表（空なら main）。
func wantB(s *state, p Purpose, to, ret string, atT time.Time, inputs []string, bad string) *Batch {
	id := s.NextBatchID
	s.NextBatchID++
	s.Pending[id] = PendingBatch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch,
		StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch, ReturnScene: ret}
	if p.IsAuto() {
		s.LastAutoSent[p] = atT
	}
	if inputs == nil {
		inputs = []string{inMain}
	}
	if p != PurposeAutoFail {
		bad = ""
	} else if bad == "" {
		bad = camMain
	}
	return &Batch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch,
		StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch, ReturnScene: ret, BadCamera: bad, Actions: actsForIn(p, to, inputs)}
}

// defaultTarget は、行が切替先と戻り先を書かないときの既定値。
func defaultTarget(s *state, p Purpose, to, ret string) (string, string) {
	switch p {
	case PurposeAutoFail, PurposeStandbyOn:
		if to == "" {
			to = s.StandbyUUID
		}
		if ret == "" {
			ret = s.Scene
		}
	case PurposeAutoRecover, PurposeStandbyOffMonitored, PurposeStandbyOffOther:
		if to == "" {
			to = s.ReturnScene
		}
	case PurposeStartScene:
		if to == "" {
			to = s.StartSceneUUID
		}
	}
	return to, ret
}

// ---- 入口ごとに、1つの Event の前後の状態を比べる仕組み ----

type evRow struct {
	name   string
	base   func(s *state) // 空なら baseState
	setup  func(s *state)
	ev     Event
	err    error    // 返るべき cmdErr（型だけ比べる）
	reason string   // 返るべき理由の文（空なら比べない）
	out    Purpose  // 0 ならバッチなし
	to     string   // バッチの SetScene の切替先（空なら既定値）
	ret    string   // バッチの戻り先（空なら既定値）
	in     []string // バッチの RestartMedia の対象（nil なら main の入力）
	bad    string   // AutoFail の悪い代表（空なら main）
	change func(s *state)
	recErr bool                           // LastError を記録する
	check  func(t *testing.T, m *Machine) // Event のあとの Machine を追加で確かめる
}

func runEvRows(t *testing.T, rows []evRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			m := newMachine()
			if r.base != nil {
				r.base(&m.st)
			} else {
				baseState(&m.st)
			}
			if r.setup != nil {
				r.setup(&m.st)
			}
			before := cloneState(m.st)
			got, err := handle1(m, r.ev)
			checkErr(t, err, r.err, r.reason)
			want := cloneState(before)
			if r.change != nil {
				r.change(&want)
			}
			if r.out != 0 {
				to, ret := defaultTarget(&want, r.out, r.to, r.ret)
				wb := wantB(&want, r.out, to, ret, r.ev.eventAt(), r.in, r.bad)
				if !reflect.DeepEqual(got, wb) {
					t.Errorf("batch: got %+v, want %+v", got, wb)
				}
			} else if got != nil {
				t.Errorf("batch: got %+v, want none", got)
			}
			checkRecErr(t, m, &want, before.LastError, r.recErr, r.ev.eventAt())
			if d := diffState(m.st, want); d != "" {
				t.Errorf("state differs:%s", d)
			}
			if r.check != nil {
				r.check(t, m)
			}
		})
	}
}

func checkErr(t *testing.T, got, want error, reason string) {
	t.Helper()
	switch want.(type) {
	case nil:
		if got != nil {
			t.Fatalf("cmdErr: got %v, want nil", got)
		}
	case ErrNotConnected:
		var e ErrNotConnected
		if !errors.As(got, &e) {
			t.Fatalf("cmdErr: got %v, want ErrNotConnected", got)
		}
	case ErrInvalidState:
		var e ErrInvalidState
		if !errors.As(got, &e) || e.Reason == "" {
			t.Fatalf("cmdErr: got %v, want ErrInvalidState with reason", got)
		}
		if reason != "" && e.Reason != reason {
			t.Fatalf("reason: got %q, want %q", e.Reason, reason)
		}
	}
}

// checkRecErr は LastError の記録を確かめ、記録したならその値を期待の状態に写す。
func checkRecErr(t *testing.T, m *Machine, want *state, before *ErrorInfo, rec bool, atT time.Time) {
	t.Helper()
	if !rec {
		return
	}
	got := m.st.LastError
	if got == nil || got == before || !got.At.Equal(atT) || got.Msg == "" {
		t.Errorf("LastError: got %s, want a new record at %s", show(got), show(atT))
		return
	}
	want.LastError = got
}

// ---- Approve の前後の状態を比べる仕組み ----

type apRow struct {
	name     string
	base     func(s *state) // 空なら baseState
	setup    func(s *state) // 未完了のバッチは id=1 で足す
	action   Action
	override Override
	atMs     int
	res      ApproveResult // Proceed（ReqID は見ない）/ Skip / Reject / Replace
	register bool          // Proceed で期待を登録する
	change   func(s *state)
	recErr   bool
	check    func(t *testing.T, m *Machine) // Approve のあとの Machine を追加で確かめる
}

func runApRows(t *testing.T, rows []apRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			m := newMachine()
			if r.base != nil {
				r.base(&m.st)
			} else {
				baseState(&m.st)
			}
			r.setup(&m.st)
			before := cloneState(m.st)
			atT := at(r.atMs)
			got := m.Approve(1, r.action, r.override, atT)
			want := cloneState(before)
			if r.change != nil {
				r.change(&want)
			}
			switch r.res.(type) {
			case Proceed:
				p, ok := got.(Proceed)
				if !ok {
					t.Fatalf("result: got %#v, want Proceed", got)
				}
				if r.register {
					if p.ReqID != before.NextReqID {
						t.Errorf("ReqID: got %d, want %d", p.ReqID, before.NextReqID)
					}
					ov := r.override
					if ov == nil {
						ov = OverrideNone{}
					}
					pb := before.Pending[1]
					want.Expectations = append(want.Expectations, Expectation{
						ReqID: before.NextReqID, Scene: r.action.(SetScene).SceneUUID, Purpose: pb.Purpose,
						ReturnScene: pb.ReturnScene, ConfigEpoch: before.ConfigEpoch, RegisteredAt: atT,
						Override: ov, TransitionMs: before.TransitionMs})
					want.NextReqID++
				} else if p.ReqID != 0 {
					t.Errorf("ReqID: got %d, want 0", p.ReqID)
				}
			default:
				if !reflect.DeepEqual(got, r.res) {
					t.Fatalf("result: got %#v, want %#v", got, r.res)
				}
			}
			checkRecErr(t, m, &want, before.LastError, r.recErr, atT)
			if d := diffState(m.st, want); d != "" {
				t.Errorf("state differs:%s", d)
			}
			if r.check != nil {
				r.check(t, m)
			}
		})
	}
}

// ---- Event の列を流す仕組み ----

type seq struct {
	t *testing.T
	m *Machine
}

func newSeq(t *testing.T) *seq { return &seq{t: t, m: newMachine()} }

// prep は準備 P(シーン, 配信, 受信状態) を、全体の長さ tr で行う。
func (q *seq) prep(scene string, streaming bool, link scenemap.CameraState, tr *int, mods ...cfgMod) *seq {
	q.t.Helper()
	q.none(cfg(0, mods...))
	q.none(lc(0, link))
	q.none(conn(0, streaming, scene, tr))
	return q
}

// multi は複数のカメラ m、s、d の準備: 設定（standby の3台）、全カメラの状態 st、複数のカメラの地図で接続する。
func (q *seq) multi(scene string, streaming bool, st [3]scenemap.CameraState, mods ...cfgMod) *seq {
	q.t.Helper()
	q.none(cfg(0, append([]cfgMod{withMulti()}, mods...)...))
	q.none(cc(0, st[0], st[1], st[2]))
	q.none(connM(0, streaming, scene, multiMap()))
	return q
}

// P は準備 P(シーン, 配信, 受信状態)。
func (q *seq) P(scene string, streaming bool, link scenemap.CameraState, mods ...cfgMod) *seq {
	q.t.Helper()
	return q.prep(scene, streaming, link, ip(300), mods...)
}

// PA は準備 PA(R): 自動の待機。結果は待機シーン、持ち主 auto、戻り先 R、LOW。
func (q *seq) PA(ret string) *seq {
	q.t.Helper()
	q.P(ret, true, OK)
	q.batch(lc(100, LOW), 1, PurposeAutoFail, standby, ret)
	q.approve(1, set(standby), 100, Proceed{ReqID: 1})
	q.none(srd(100, 1, RequestOK))
	q.none(bd(100, 1, BatchCompleted))
	q.none(sc(500, standby))
	q.owner(OwnerAuto, ret)
	return q
}

// PM は準備 PM(R, 受信状態): 手動の待機。結果は持ち主 manual、戻り先 R。
func (q *seq) PM(ret string, link scenemap.CameraState, mods ...cfgMod) *seq {
	q.t.Helper()
	q.P(ret, true, OK, mods...)
	q.standbyOnFrom(ret)
	if link != OK {
		q.none(lc(600, link))
	}
	return q
}

// standbyOnFrom は t=0.1 に StandbyOn#1 を受け付けて通し、t=0.5 に待機シーンへ届ける。
func (q *seq) standbyOnFrom(ret string) {
	q.t.Helper()
	q.batch(cmd(100, CmdStandbyOn), 1, PurposeStandbyOn, standby, ret)
	q.approve(1, set(standby), 100, Proceed{ReqID: 1})
	q.none(srd(100, 1, RequestOK))
	q.none(bd(100, 1, BatchCompleted))
	q.none(sc(500, standby))
	q.owner(OwnerManual, ret)
}

// none は Event を流し、バッチも cmdErr も返らないことを確かめる。
func (q *seq) none(ev Event) {
	q.t.Helper()
	b, err := handle1(q.m, ev)
	if err != nil || b != nil {
		q.t.Fatalf("%T at %s: got batch %s err %v, want none", ev, show(ev.eventAt()), showBatch(b), err)
	}
}

func showBatch(b *Batch) string {
	if b == nil {
		return "nil"
	}
	return fmt.Sprintf("%v#%d%+v ret=%q", b.Purpose, b.BatchID, b.Actions, b.ReturnScene)
}

// batch は Event を流し、id と Purpose、切替先 to、戻り先 ret のバッチが返ることを確かめる。
func (q *seq) batch(ev Event, id int, p Purpose, to, ret string) *Batch {
	q.t.Helper()
	b, err := handle1(q.m, ev)
	if err != nil {
		q.t.Fatalf("%T at %s: cmdErr %v", ev, show(ev.eventAt()), err)
	}
	q.checkBatch(b, id, p, to, ret)
	return b
}

func (q *seq) checkBatch(b *Batch, id int, p Purpose, to, ret string) {
	q.t.Helper()
	q.checkBatchIn(b, id, p, to, ret, []string{inMain}, camMain)
}

// checkBatchIn は再起動の対象 inputs と AutoFail の悪い代表 bad も確かめる。
func (q *seq) checkBatchIn(b *Batch, id int, p Purpose, to, ret string, inputs []string, bad string) {
	q.t.Helper()
	if b == nil {
		q.t.Fatalf("got no batch, want %v#%d", p, id)
	}
	want := actsForIn(p, to, inputs)
	if p == PurposeAutoFail && b.BadCamera != bad {
		q.t.Fatalf("BadCamera: got %q, want %q", b.BadCamera, bad)
	}
	if p != PurposeAutoFail && b.BadCamera != "" {
		q.t.Fatalf("BadCamera: got %q, want empty", b.BadCamera)
	}
	if b.BatchID != id || b.Purpose != p || !reflect.DeepEqual(b.Actions, want) || b.ReturnScene != ret {
		q.t.Fatalf("got %s, want %v#%d%+v ret=%q", showBatch(b), p, id, want, ret)
	}
	s := &q.m.st
	if b.SceneEpoch != s.SceneEpoch || b.ConnEpoch != s.ConnEpoch || b.StreamEpoch != s.StreamEpoch || b.ConfigEpoch != s.ConfigEpoch {
		q.t.Fatalf("batch epochs %d/%d/%d/%d, state %d/%d/%d/%d", b.SceneEpoch, b.ConnEpoch, b.StreamEpoch, b.ConfigEpoch,
			s.SceneEpoch, s.ConnEpoch, s.StreamEpoch, s.ConfigEpoch)
	}
}

// batchIn は batch と同じだが、再起動の対象と AutoFail の悪い代表も確かめる（複数のカメラの列）。
func (q *seq) batchIn(ev Event, id int, p Purpose, to, ret string, inputs []string, bad string) *Batch {
	q.t.Helper()
	b, err := handle1(q.m, ev)
	if err != nil {
		q.t.Fatalf("%T at %s: cmdErr %v", ev, show(ev.eventAt()), err)
	}
	q.checkBatchIn(b, id, p, to, ret, inputs, bad)
	return b
}

// cmdErr は Command を流し、その型の cmdErr が返り状態が変わらないことを確かめる。
func (q *seq) cmdErr(ev Event, want error) {
	q.t.Helper()
	before := cloneState(q.m.st)
	b, err := handle1(q.m, ev)
	if b != nil {
		q.t.Fatalf("got batch %s, want none", showBatch(b))
	}
	checkErr(q.t, err, want, "")
	if d := diffState(q.m.st, before); d != "" {
		q.t.Fatalf("state changed on cmdErr:%s", d)
	}
}

// approve は Approve を呼び、結果が want と一致することを確かめる。
func (q *seq) approve(id int, a Action, ms int, want ApproveResult) {
	q.t.Helper()
	q.approveOv(id, a, OverrideNone{}, ms, want)
}

func (q *seq) approveOv(id int, a Action, ov Override, ms int, want ApproveResult) {
	q.t.Helper()
	got := q.m.Approve(id, a, ov, at(ms))
	if !reflect.DeepEqual(got, want) {
		q.t.Fatalf("Approve(%d, %+v) at %dms: got %#v, want %#v", id, a, ms, got, want)
	}
}

// exps は期待の列のシーン名と ReqID を、"Standby", 1, ... の形で確かめる。
func (q *seq) exps(want ...any) {
	q.t.Helper()
	var got []any
	for _, e := range q.m.st.Expectations {
		got = append(got, e.Scene, e.ReqID)
	}
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		q.t.Fatalf("expectations: got %v, want %v", got, want)
	}
}

func (q *seq) deadline(req, ms int) {
	q.t.Helper()
	for _, e := range q.m.st.Expectations {
		if e.ReqID == req {
			if !e.Responded || !e.Deadline.Equal(at(ms)) {
				q.t.Fatalf("r%d deadline: got %s (responded=%v), want %s", req, show(e.Deadline), e.Responded, show(at(ms)))
			}
			return
		}
	}
	q.t.Fatalf("r%d not found", req)
}

// owner は Snapshot の持ち主と戻り先を確かめる。
func (q *seq) owner(o Owner, ret string) {
	q.t.Helper()
	s := q.m.Snapshot().Standby
	if s.Owner != o || s.ReturnScene != ret {
		q.t.Fatalf("standby: got %v %q, want %v %q", s.Owner, s.ReturnScene, o, ret)
	}
}

func (q *seq) scene(name string) {
	q.t.Helper()
	if got := q.m.Snapshot().Scene; got != name {
		q.t.Fatalf("scene: got %q, want %q", got, name)
	}
}

func (q *seq) pending(ids ...int) {
	q.t.Helper()
	if len(q.m.st.Pending) != len(ids) {
		q.t.Fatalf("pending: got %v, want %v", q.m.st.Pending, ids)
	}
	for _, id := range ids {
		if _, ok := q.m.st.Pending[id]; !ok {
			q.t.Fatalf("pending: %d missing in %v", id, q.m.st.Pending)
		}
	}
}

// err は今の LastError を返す。
func (q *seq) err() *ErrorInfo { return q.m.st.LastError }

// errAt は LastError が時刻 ms の記録であることを確かめる。
func (q *seq) errAt(ms int) {
	q.t.Helper()
	got := q.m.Snapshot().LastError
	if got == nil || !got.At.Equal(at(ms)) || got.Msg == "" {
		q.t.Fatalf("LastError: got %s, want a record at %s", show(got), show(at(ms)))
	}
}

// noErr は LastError が一度も記録されていないことを確かめる。
func (q *seq) noErr() {
	q.t.Helper()
	if got := q.m.Snapshot().LastError; got != nil {
		q.t.Fatalf("LastError: got %s, want none", show(got))
	}
}

// sameErr は LastError が prev から変わっていないことを確かめる。
func (q *seq) sameErr(prev *ErrorInfo) {
	q.t.Helper()
	if got := q.m.st.LastError; got != prev {
		q.t.Fatalf("LastError: got %s, want unchanged %s", show(got), show(prev))
	}
}

// epochs は4つの Epoch を返す。
func (q *seq) epochs() [4]int {
	s := &q.m.st
	return [4]int{s.SceneEpoch, s.ConnEpoch, s.StreamEpoch, s.ConfigEpoch}
}

// epochDelta は4つの Epoch が before から scene、conn、stream、config だけ進んだことを確かめる。
func (q *seq) epochDelta(before [4]int, scene, conn, stream, config int) {
	q.t.Helper()
	now := q.epochs()
	want := [4]int{before[0] + scene, before[1] + conn, before[2] + stream, before[3] + config}
	if now != want {
		q.t.Fatalf("epochs (scene/conn/stream/config): got %v, want %v", now, want)
	}
}

func (q *seq) lastAuto(p Purpose, ms int) {
	q.t.Helper()
	got, ok := q.m.st.LastAutoSent[p]
	if !ok || !got.Equal(at(ms)) {
		q.t.Fatalf("LastAutoSent[%v]: got %s (ok=%v), want %s", p, show(got), ok, show(at(ms)))
	}
}

func (q *seq) noLastAuto() {
	q.t.Helper()
	if len(q.m.st.LastAutoSent) != 0 {
		q.t.Fatalf("LastAutoSent: got %s, want empty", show(q.m.st.LastAutoSent))
	}
}

func (q *seq) ticks(fromMs, toMs, stepMs int) {
	q.t.Helper()
	for ms := fromMs; ms <= toMs; ms += stepMs {
		q.none(tick(ms))
	}
}
