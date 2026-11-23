package switcher

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"slices"
	"strings"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// Config は Machine の生成時の設定。
type Config struct {
	// RestartWait は RestartMedia.Wait に入れる待ち時間。
	RestartWait time.Duration
	// Endpoint は OBS の接続先。隠した項目の記録の同一性（接続先 + シーンコレクション）に使う。接続先の変更は新しい世代。
	Endpoint Endpoint
}

// Endpoint は OBS の接続先（資格情報は含めない）。
type Endpoint struct {
	Host string
	Port int
}

// settings は生成時に決まり、変わらない値。
type settings struct {
	RestartWait   time.Duration
	ExpectMargin  time.Duration // 期待の期限の余裕
	ExpectFloor   time.Duration // 上書きあり・長さ不明などのときの下限
	RetryInterval time.Duration // Tick の再送の間隔
}

// Machine はシーンを切り替える状態機械。I/O を持たず、時計も読まない。
// 時刻は Event.At と Approve の at だけから得る。
// シーンの設定は初期値を持たないので、最初の Event として ScenesConfigured を渡す。
type Machine struct {
	cfg settings
	st  state
	ep  Endpoint
}

// New は未接続の初期状態の Machine を返す。
func New(cfg Config) *Machine {
	return &Machine{
		ep: cfg.Endpoint,
		cfg: settings{
			RestartWait:   cfg.RestartWait,
			ExpectMargin:  3 * time.Second,
			ExpectFloor:   10 * time.Second,
			RetryInterval: 5 * time.Second,
		},
		st: state{
			CameraStates:      map[string]scenemap.CameraState{},
			Pending:           map[int]PendingBatch{},
			LastAutoSent:      map[Purpose]time.Time{},
			NextBatchID:       1,
			LastCandidateSent: map[candKey]time.Time{},
			NextReqID:         1,

			Unsent:         map[scenemap.ItemKey]int{},
			ItemReceipts:   map[uint64]ItemReceipt{},
			MAConfirm:      map[scenemap.ItemKey]bool{},
			UserShown:      map[string]bool{},
			ItemFailures:   map[string]int{},
			LastItemSent:   map[string]time.Time{},
			StudioModeHide: StudioHideStandby,
			NextItemReq:    1,
		},
	}
}

// Snapshot は status に出す状態を返す。地図や状態の map は外へ直接公開しない。
func (m *Machine) Snapshot() Snapshot {
	s := &m.st
	snap := Snapshot{
		Connected: s.Connected,
		Streaming: s.Streaming,
		Scene:     s.Scene,
		SceneName: m.nameInMap(s.Scene),
		Class:     m.classOf(s.Scene),
		Standby: StandbyStatus{Owner: s.StandbyOwner, ReturnScene: s.ReturnScene, ReturnSceneName: m.nameInMap(s.ReturnScene),
			Stack: slices.Clone(s.Stack)},
		AutoEnabled: s.AutoEnabled,
		SceneEpoch:  s.SceneEpoch,
	}
	for _, u := range s.Stack {
		snap.Standby.StackNames = append(snap.Standby.StackNames, m.nameInMap(u))
	}
	if s.Connected && s.Map != nil {
		snap.VisibleCameras = m.visibleCameras(s.Scene)
		snap.Health, snap.BadCamera, snap.HasHealth = m.healthOf(s.Scene)
	}
	if s.LastError != nil {
		e := *s.LastError
		snap.LastError = &e
	}
	if s.Connected && s.Map != nil {
		snap.FallbackWarnings = m.resolved().Warnings
	}
	snap.HiddenEntries = cloneHidden(s.Hidden)
	snap.RetainedEntries = cloneHidden(s.Retained)
	snap.HiddenRevision = s.HiddenRevision
	snap.StudioMode = s.StudioMode
	for _, c := range s.Cameras {
		snap.Cameras = append(snap.Cameras, CameraItemStatus{ID: c.ID, UserShown: s.UserShown[c.ID],
			Failures: s.ItemFailures[c.ID], Unusable: m.unusable(c.ID)})
	}
	return snap
}

// nameInMap は今の地図でのシーンの名前。地図になければ空。
func (m *Machine) nameInMap(uuid string) string {
	if m.st.Map == nil {
		return ""
	}
	return m.st.Map.Scenes[uuid].Name
}

// Handle は Event を1つ処理する。返すバッチは列で、非 Command ではシーン切替→ShowCamera→HideCamera の順。
// 空は nil。cmdErr は Command を受け付けられないときだけ返す。
func (m *Machine) Handle(ev Event) ([]Batch, error) {
	at := ev.eventAt()
	m.expire(at)
	one := func(b *Batch) ([]Batch, error) {
		if b == nil {
			return nil, nil
		}
		return []Batch{*b}, nil
	}
	switch e := ev.(type) {
	case ScenesConfigured:
		return m.onConfigured(e), nil
	case CamerasChanged:
		return m.onCamerasChanged(e), nil
	case SceneMapChanged:
		return m.onMapChanged(e), nil
	case ItemEnabledChanged:
		return m.onItemEnabledEvent(e), nil
	case OBSConnected:
		return m.onConnected(e), nil
	case OBSDisconnected:
		m.onDisconnected()
	case SceneChanged:
		return m.onSceneChanged(e), nil
	case TransitionChanged:
		m.st.TransitionMs = copyInt(e.TransitionMs)
	case StreamStateChanged:
		return m.onStreamState(e), nil
	case SceneRequestDone:
		m.onRequestDone(e)
	case ItemRequestDone:
		return m.onItemRequestDone(e), nil
	case StudioModeChanged:
		return m.onStudioMode(e), nil
	case BatchDone:
		return m.onBatchDone(e), nil
	case HiddenRestored:
		m.onHiddenRestored(e)
	case HiddenWriteFailed:
		m.onHiddenWriteFailed(e)
	case Stopping:
		return m.onStopping(e), nil
	case Tick:
		return m.jb(at), nil
	case Command:
		if m.st.StoppingReceived {
			return nil, ErrInvalidState{Reason: ReasonStopping}
		}
		if e.Kind == CmdShowAllCameras {
			return m.onShowAll(e)
		}
		b, err := m.onCommand(e)
		if err != nil {
			return nil, err
		}
		return one(b)
	}
	return nil, nil
}

// onStudioMode は Studio Mode のオン・オフの変化。同じ値、未接続は E のみ。
func (m *Machine) onStudioMode(e StudioModeChanged) []Batch {
	s := &m.st
	if !s.Connected || s.StudioMode == e.Enabled {
		return nil
	}
	s.StudioMode = e.Enabled
	m.refine()
	return m.jb(e.At)
}

// jb は更新後の判断（Jb）: シーン判定→ShowCamera→HideCamera の順にバッチの列を返す。停止の受信後は出さない。
func (m *Machine) jb(at time.Time) []Batch {
	if m.st.StoppingReceived {
		return nil
	}
	var out []Batch
	if b := m.judge(at); b != nil {
		out = append(out, *b)
	}
	return append(out, m.itemJudge(at)...)
}

// itemJudge は ShowCamera→HideCamera の候補。
func (m *Machine) itemJudge(at time.Time) []Batch {
	if m.st.StoppingReceived {
		return nil
	}
	out := m.genShows(at)
	return append(out, m.genHides(at)...)
}

func copyInt(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func copyString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// clearOwner は持ち主、戻り先、スタックを消す（共通の変化 C）。今いるシーンも期待も世代も変えない。
func (m *Machine) clearOwner() {
	m.st.StandbyOwner = OwnerNone
	m.st.ReturnScene = ""
	m.st.Stack = nil
}

// setAuto は auto のスタックを k にする。空なら持ち主は none。ReturnScene は k の最下段。
func (m *Machine) setAuto(k []string) {
	if len(k) == 0 {
		m.clearOwner()
		return
	}
	m.st.StandbyOwner = OwnerAuto
	m.st.Stack = slices.Clone(k)
	m.st.ReturnScene = k[0]
}

// stackNow は auto のスタックの写し。auto でなければ nil。
func (m *Machine) stackNow() []string {
	if m.st.StandbyOwner != OwnerAuto {
		return nil
	}
	return slices.Clone(m.st.Stack)
}

// releaseAll は人の切替と同じ共通の変化（コア spec v2.16 第6章）: SceneEpoch を進め、シーンの期待と再送の時刻を全部消し、
// 持ち主・戻り先・スタックを捨てる。
func (m *Machine) releaseAll() {
	s := &m.st
	s.SceneEpoch++
	s.Expectations = nil
	s.LastAutoSent = map[Purpose]time.Time{}
	s.LastCandidateSent = map[candKey]time.Time{}
	m.clearOwner()
}

// disableExpectations は、今の設定の世代で登録した残りのシーンの期待を、照合だけして作用を当てないものにする
// （古い設定の世代の期待は、世代の違いですでに作用しない）。
func (m *Machine) disableExpectations() {
	for i := range m.st.Expectations {
		if m.st.Expectations[i].ConfigEpoch == m.st.ConfigEpoch {
			m.st.Expectations[i].Plan.EffectsDisabled = true
		}
	}
}

func (m *Machine) recordError(at time.Time, msg i18n.Msg) {
	m.st.LastError = &ErrorInfo{At: at, Msg: i18n.Render(i18n.JA, msg), Localized: msg}
}

// sameConfig は e が今の設定と同じ内容かを返す。Cameras は順序つきで比べる（順の変更は設定の変更）。
func (m *Machine) sameConfig(e ScenesConfigured) bool {
	s := &m.st
	if s.ConfigEpoch == 0 || e.Standby != s.Standby || e.AutoEnabled != s.AutoEnabled {
		return false
	}
	if (e.StartScene == nil) != (s.StartScene == nil) || (e.StartScene != nil && *e.StartScene != *s.StartScene) {
		return false
	}
	if studioHideOf(e) != s.StudioModeHide {
		return false
	}
	if !slices.Equal(e.Cameras, s.Cameras) {
		return false
	}
	return sameFallbacks(e.SceneFallbacks, s.Fallbacks)
}

// normFallbacks は scene → fallback の対応（同じ scene は先の行）。行の並べ替えは同値にするための正規形。
func normFallbacks(rows []FallbackRow) map[string]string {
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		if _, dup := out[r.Scene]; !dup {
			out[r.Scene] = r.Fallback
		}
	}
	return out
}

func sameFallbacks(a, b []FallbackRow) bool {
	na, nb := normFallbacks(a), normFallbacks(b)
	if len(na) != len(nb) {
		return false
	}
	for k, v := range na {
		if w, ok := nb[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func studioHideOf(e ScenesConfigured) string {
	if e.StudioModeHide == "" {
		return StudioHideStandby
	}
	return e.StudioModeHide
}

// onConfigured はシーンとカメラの設定を差し替える。期待と未完了のバッチはそのまま残す
// （未完了のシーン切替のバッチは、設定の世代が進むので承認で却下される）。
func (m *Machine) onConfigured(e ScenesConfigured) []Batch {
	s := &m.st
	if m.sameConfig(e) {
		// 行の並べ替えだけ: 世代は進めないが、並びは新しい設定のものに差し替える（status の警告の行番号を設定に合わせる）。
		m.st.Fallbacks = slices.Clone(e.SceneFallbacks)
		return nil
	}
	renamed := e.Standby != s.Standby
	s.StudioModeHide = studioHideOf(e)
	s.Standby = e.Standby
	s.Cameras = slices.Clone(e.Cameras)
	// 残るカメラの状態は保ち、増えたカメラは DOWN、減ったカメラは除く。
	states := make(map[string]scenemap.CameraState, len(e.Cameras))
	for _, c := range e.Cameras {
		if v, ok := s.CameraStates[c.ID]; ok {
			states[c.ID] = v
		} else {
			states[c.ID] = scenemap.StateDown
		}
	}
	s.CameraStates = states
	for id := range s.UserShown {
		if _, ok := states[id]; !ok {
			delete(s.UserShown, id)
		}
	}
	for id := range s.ItemFailures {
		if _, ok := states[id]; !ok {
			delete(s.ItemFailures, id)
		}
	}
	for id := range s.LastItemSent {
		if _, ok := states[id]; !ok {
			delete(s.LastItemSent, id)
		}
	}
	s.AutoEnabled = e.AutoEnabled
	s.StartScene = copyString(e.StartScene)
	s.Fallbacks = slices.Clone(e.SceneFallbacks)
	s.ConfigEpoch++
	switch {
	case s.StandbyOwner == OwnerNone:
	case renamed:
		m.clearOwner() // 旧い設定の世代の期待は、到着しても作用しない（世代で無効）
	case s.StandbyOwner == OwnerAuto && !s.AutoEnabled:
		m.clearOwner()
	}
	m.refine()
	return m.jb(e.At)
}

// refine は「整える」共通の手順。ScenesConfigured、SceneMapChanged、ItemEnabledChanged、OBSConnected、
// SceneChanged の更新のあと、判断の前に行う（表 0c 2）。
//  1. 待機と開始時の名前を今の地図で引き直す。待機の解決結果が変わったら（名前・UUID の変更や消失）、auto も manual も C にし、
//     残っているシーンの期待は照合だけにする（作用を無効化）。
//  2. auto の履歴を整える: 重複（下を残す）、現在のシーンとそれより上、削除（下が消えたら手動共通解除、途中は外すだけ）、
//     管理外化（外す）。空になれば none。
//  3. manual の戻り先が削除されたら手動共通解除。manual は現在のシーンが待機であることを要する。
func (m *Machine) refine() {
	s := &m.st
	old := s.StandbyUUID
	s.StandbyUUID = m.resolve(s.Standby)
	s.StartSceneUUID = ""
	if s.StartScene != nil {
		s.StartSceneUUID = m.resolve(*s.StartScene)
	}
	if !s.Connected {
		return
	}
	if old != "" && old != s.StandbyUUID {
		m.disableExpectations()
		m.clearOwner()
	}
	m.tidyHistory()
	if s.StandbyOwner == OwnerManual && (s.StandbyUUID == "" || s.Scene != s.StandbyUUID) {
		m.clearOwner()
	}
}

// tidyHistory は auto のスタックと manual の戻り先を、今の地図で整える。空の地図（シーンの一覧がない）では存在を見ない。
func (m *Machine) tidyHistory() {
	s := &m.st
	switch s.StandbyOwner {
	case OwnerAuto:
		var k []string
		for _, u := range s.Stack {
			if !slices.Contains(k, u) { // 不正な重複は下側を残す
				k = append(k, u)
			}
		}
		if i := slices.Index(k, s.Scene); i >= 0 {
			k = k[:i] // 現在のシーンとそれより上は履歴でない
		}
		if len(k) == 0 {
			m.clearOwner()
			return
		}
		if !mapIsEmpty(s.Map) {
			if _, ok := s.Map.Scenes[k[0]]; !ok {
				m.releaseAll() // 戻り先の削除は人の操作と同じ（削除を管理外化より先に判定）
				return
			}
			var kept []string
			for _, u := range k {
				if _, ok := s.Map.Scenes[u]; ok && m.isMonitored(u) {
					kept = append(kept, u)
				}
			}
			k = kept
		}
		m.setAuto(k)
	case OwnerManual:
		if s.ReturnScene != "" && !mapIsEmpty(s.Map) {
			if _, ok := s.Map.Scenes[s.ReturnScene]; !ok {
				m.releaseAll()
			}
		}
	}
}

// onCamerasChanged は全カメラの組を一括で置き換える。未知のキーは無視し、足りないキーは DOWN に整える。
// 整えた組が今と同じなら何もしない。途中のカメラ状態では判断しない。
func (m *Machine) onCamerasChanged(e CamerasChanged) []Batch {
	s := &m.st
	next := make(map[string]scenemap.CameraState, len(s.Cameras))
	for _, c := range s.Cameras {
		if v, ok := e.States[c.ID]; ok {
			next[c.ID] = v
		} else {
			next[c.ID] = scenemap.StateDown
		}
	}
	if sameStates(next, s.CameraStates) {
		return nil
	}
	for id, v := range next {
		if s.CameraStates[id] != v {
			delete(s.ItemFailures, id)
			if v == scenemap.StateOK {
				delete(s.UserShown, id)
			}
		}
	}
	s.CameraStates = next
	return m.jb(e.At)
}

func sameStates(a, b map[string]scenemap.CameraState) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// onMapChanged は地図を差し替える。未接続なら何もしない。同じ地図なら何もしない。
// 地図の更新だけでは現在の program UUID も SceneEpoch も ConfigEpoch も変えない。
func (m *Machine) onMapChanged(e SceneMapChanged) []Batch {
	s := &m.st
	if !s.Connected || e.Map == nil {
		return nil
	}
	// 構造の照合は、地図の同値判定だけで省略せず、未照合の記録がある場合を考える（H-02）。
	if sameMap(s.Map, e.Map) && !(len(s.Retained) > 0 && !mapIsEmpty(e.Map)) {
		return nil
	}
	s.Map = copyMap(e.Map)
	m.resplit()
	m.reconcile(e.At)
	m.refine()
	return m.jb(e.At)
}

// onConnected は地図と状態を取り直す。シーン、配信状態、コレクションのどれかが切断の直前と違っていたら
// 持ち主と戻り先を消す（手動の切替としては扱わず、配信開始時の切替も出さない）。そのあと「整える」を行う。
func (m *Machine) onConnected(e OBSConnected) []Batch {
	s := &m.st
	sceneDiffers := e.SceneUUID != s.PrevConnScene
	streamDiffers := e.Streaming != s.PrevConnStreaming
	mp := copyMap(e.Map)
	if mp == nil {
		mp = copyMap(&scenemap.Map{})
	}
	collectionDiffers := mp.Collection != s.PrevConnCollection
	s.Connected = true
	s.Streaming = e.Streaming
	s.Scene = e.SceneUUID
	s.Map = mp
	s.TransitionMs = copyInt(e.TransitionMs)
	s.StudioMode = e.StudioMode
	s.HiddenRestoredDone = true
	if streamDiffers {
		s.StreamEpoch++
		if !e.Streaming {
			clear(s.UserShown)
		}
	}
	if sceneDiffers || streamDiffers || collectionDiffers {
		m.clearOwner()
	}
	s.StandbyUUID = s.PrevConnStandby // 新しい地図の解決結果を、切断の直前と比べる
	m.resplit()
	m.reconcile(e.At)
	m.refine()
	return m.jb(e.At)
}

// onDisconnected は期待をすべて消し、地図と解決した UUID を消す。持ち主と戻り先は再接続まで保つ。
func (m *Machine) onDisconnected() {
	s := &m.st
	s.Connected = false
	s.ConnEpoch++
	s.Expectations = nil
	s.ItemExpectations = nil
	s.PrevConnScene = s.Scene
	s.PrevConnStreaming = s.Streaming
	if s.Map != nil {
		s.PrevConnCollection = s.Map.Collection
	}
	s.PrevConnStandby = s.StandbyUUID // 空は「情報なし」（待機が引けていなかった）。古い値を残さない
	s.Map = nil
	s.StandbyUUID = ""
	s.StartSceneUUID = ""
}

// onSceneChanged は期待と照らし、本体の切替か手動の切替かを見分ける。期待と一致したら、その期待の計画を1回だけ当てる。
func (m *Machine) onSceneChanged(e SceneChanged) []Batch {
	s := &m.st
	s.Scene = e.SceneUUID
	i := slices.IndexFunc(s.Expectations, func(x Expectation) bool { return x.Scene == e.SceneUUID })
	if i < 0 {
		// 手動（外部）の切替。人がシーンを変えたので、共通の変化を当てる。
		m.releaseAll()
		m.refine()
		return m.jb(e.At)
	}
	// 本体の切替。一致した期待とそれより前の期待、同じシーンの期待を消す。当てるのは一致した1件の計画だけ。
	hit := s.Expectations[i]
	var kept []Expectation
	for _, x := range s.Expectations[i+1:] {
		if x.Scene != e.SceneUUID {
			kept = append(kept, x)
		}
	}
	s.Expectations = kept
	fresh := hit.ConfigEpoch == s.ConfigEpoch && !hit.Plan.EffectsDisabled
	switch hit.Purpose {
	case PurposeAutoFail:
		// 設定が変わる前の期待、作用を無効にした期待では、履歴を追加せず持ち主を入れない。
		if fresh {
			m.setAuto(ApplyPlan(m.stackNow(), hit.Plan.Plan, e.SceneUUID))
		}
	case PurposeStandbyOn:
		if fresh {
			s.StandbyOwner = OwnerManual
			s.ReturnScene = hit.ReturnScene
			s.Stack = nil
		}
	case PurposeAutoRecover:
		if fresh {
			m.setAuto(ApplyPlan(m.stackNow(), hit.Plan.Plan, e.SceneUUID))
		} else {
			// 古い復帰は部分的に当てず、全解除する（V-07）。
			// 待機の ON の特例のあとに先の戻りの切替が届いたときは、戻り先に残って手動の待機を解き、知らせる（FB-SC-11）。
			afterOn := hit.Plan.DisabledByOn && s.StandbyOwner == OwnerManual && e.SceneUUID != s.StandbyUUID
			m.clearOwner()
			if afterOn {
				m.recordError(e.At, i18n.New("switch.return", "scene", m.sceneLabel(e.SceneUUID)))
			}
		}
	case PurposeStandbyOffMonitored, PurposeStandbyOffOther:
		// 戻り先へ届けば解除。途中の逃がし先へ届けば、OFF 専用の計画（戻り先と飛ばした監視対象）で auto にする。
		// 古い設定の世代・無効にした期待は部分的に当てず、全解除する。
		if fresh && hit.Plan.Plan.Kind == PlanEscape {
			m.setAuto(ApplyPlan(nil, hit.Plan.Plan, e.SceneUUID))
		} else {
			m.clearOwner()
		}
	}
	// 期待の照合のあとに整える。
	m.refine()
	return m.jb(e.At)
}

func (m *Machine) onStreamState(e StreamStateChanged) []Batch {
	s := &m.st
	if e.Streaming == s.Streaming {
		return nil
	}
	s.Streaming = e.Streaming
	s.StreamEpoch++
	if !e.Streaming {
		m.clearOwner()
		s.Expectations = nil
		s.LastAutoSent = map[Purpose]time.Time{}
		s.LastCandidateSent = map[candKey]time.Time{}
		clear(s.UserShown)
		return m.jb(e.At)
	}
	if s.StartScene == nil {
		return m.jb(e.At)
	}
	// 配信開始時の切替。受信状態は見ない。前の配信の StartScene が残っていれば重ねない。
	if m.hasPending(PurposeStartScene) {
		return nil
	}
	if s.StartSceneUUID == "" {
		// 開始シーンが地図で引けない。バッチは出さず（再送もしない）警告を記録し、ほかの判断は続ける。
		m.recordError(e.At, i18n.New("switch.start_missing", "scene", *s.StartScene))
		return m.jb(e.At)
	}
	b := m.emit(PurposeStartScene, e.At, []Action{SetScene{SceneUUID: s.StartSceneUUID}}, "", "", ArrivalPlan{})
	return append([]Batch{*b}, m.itemJudge(e.At)...)
}

func (m *Machine) onRequestDone(e SceneRequestDone) {
	s := &m.st
	i := slices.IndexFunc(s.Expectations, func(x Expectation) bool { return x.ReqID == e.ReqID })
	if i < 0 {
		return
	}
	if e.Result == RequestError {
		// 記録は、続いて届く BatchDone{Failed} で行う。
		s.Expectations = slices.Delete(s.Expectations, i, i+1)
		return
	}
	x := &s.Expectations[i]
	x.Responded = true
	x.RespondedAt = e.At
	x.Result = e.Result
	x.Deadline = e.At.Add(m.expectWindow(*x))
}

// expectWindow は応答の時刻から期限までの長さを返す。
func (m *Machine) expectWindow(x Expectation) time.Duration {
	ms := func(v int) time.Duration { return time.Duration(v) * time.Millisecond }
	floored := func(d time.Duration) time.Duration { return max(d+m.cfg.ExpectMargin, m.cfg.ExpectFloor) }
	switch o := x.Override.(type) {
	case OverrideKnown:
		return floored(ms(o.Ms))
	case OverrideUnknown:
		if x.TransitionMs == nil {
			return m.cfg.ExpectFloor
		}
		return floored(ms(*x.TransitionMs))
	default:
		if x.TransitionMs == nil {
			return m.cfg.ExpectFloor
		}
		return ms(*x.TransitionMs) + m.cfg.ExpectMargin
	}
}

// expire は期限を過ぎた期待を消す。期限ちょうどは残す。期限切れそのものは手動の切替として
// 扱わない。StartScene と Command 由来の切替の期限切れは記録する（Auto は Tick で再送する）。
func (m *Machine) expire(at time.Time) {
	s := &m.st
	var kept []Expectation
	for _, x := range s.Expectations {
		if !x.Responded || !at.After(x.Deadline) {
			kept = append(kept, x)
			continue
		}
		if x.Purpose == PurposeStartScene || x.Purpose.FromCommand() {
			m.recordError(at, i18n.New("switch.unconfirmed", "purpose", x.Purpose.String(), "scene", m.sceneLabel(x.Scene)))
		}
	}
	s.Expectations = kept
	m.expireItems(at)
}

func (m *Machine) onBatchDone(e BatchDone) []Batch {
	s := &m.st
	pb, ok := s.Pending[e.BatchID]
	if !ok {
		return nil
	}
	delete(s.Pending, e.BatchID)
	if pb.Purpose.isItem() {
		return m.onItemBatchDone(e, pb)
	}
	switch e.Result {
	case BatchCompleted:
		if e.Err != nil {
			m.recordError(e.At, i18n.New("switch.partial", "purpose", pb.Purpose.String(), "error", e.Err.Error()))
		}
	case BatchFailed:
		msg := i18n.New("switch.failed", "purpose", pb.Purpose.String())
		if e.Err != nil {
			msg = i18n.New("switch.failed_detail", "purpose", pb.Purpose.String(), "error", e.Err.Error())
		}
		m.recordError(e.At, msg)
	case BatchRejected:
		if (pb.Purpose.FromCommand() || pb.Purpose == PurposeStartScene) && !pb.ManualRejected {
			m.recordError(e.At, i18n.New("switch.rejected", "purpose", pb.Purpose.String()))
		}
	}
	// 未完了が解消したので判断し直す（StartScene・AutoRecover・OFF の未完了の抑止、承認で却下した候補の再判断）。
	m.refine()
	return m.jb(e.At)
}

// onItemBatchDone は項目のバッチの終わり: 未送信の予約を戻し、Hide / Show は失敗の回数を数える。
func (m *Machine) onItemBatchDone(e BatchDone, pb PendingBatch) []Batch {
	s := &m.st
	m.rollbackBatch(&pb, false)
	// 結果を受けた台帳は、バッチが終わったら捨てる（結果がまだのものは、遅れた応答を解くために保つ）。
	for _, pi := range pb.Items {
		if r, ok := s.ItemReceipts[pi.ReqID]; ok && pi.ReqID != 0 && r.Result != 0 {
			delete(s.ItemReceipts, pi.ReqID)
		}
	}
	failed := e.Result == BatchCompleted && (e.Err != nil || pb.Failures > 0)
	switch pb.Purpose {
	case PurposeHideCamera, PurposeShowCamera:
		c := pb.CameraID
		switch {
		case failed:
			s.ItemFailures[c]++
			if s.ItemFailures[c] == itemFailLimit {
				m.recordError(e.At, i18n.New("switch.item_failed", "camera", c))
			}
		case e.Result == BatchCompleted && pb.Sent > 0:
			delete(s.ItemFailures, c)
		}
	case PurposeRestoreAll:
		if pb.Scope == RestoreAllCameraItems && failed {
			m.recordError(e.At, i18n.New("switch.restore_failed"))
		}
	}
	m.refine()
	return m.jb(e.At)
}

// ---- 自動切替 ----

// suppressed は切替中の抑止に当たるかを返す。抑止の Purpose のバッチが未完了か、
// その期待（設定の世代が古いものも含む）が残っている間。
func (m *Machine) suppressed() bool {
	for _, pb := range m.st.Pending {
		if pb.Purpose.suppresses() {
			return true
		}
	}
	for _, x := range m.st.Expectations {
		if x.Purpose.suppresses() {
			return true
		}
	}
	return false
}

// recoverInFlight は AutoRecover の未完了か期待が残っているか。残る間は AutoFail を出さない（到着してから通常判断する）。
// 手動の待機の OFF の未完了・期待も同じに扱う（FB-TK-08）。
func (m *Machine) recoverInFlight() bool {
	in := func(p Purpose) bool {
		return p == PurposeAutoRecover || p == PurposeStandbyOffMonitored || p == PurposeStandbyOffOther
	}
	for _, pb := range m.st.Pending {
		if in(pb.Purpose) {
			return true
		}
	}
	return slices.ContainsFunc(m.st.Expectations, func(x Expectation) bool { return in(x.Purpose) })
}

// autoDecision は自動切替の判断の結果: 目的、切替先、届いたときに当てる計画、悪い代表のカメラ（AutoFail）。
type autoDecision struct {
	Purpose Purpose
	Target  string
	Plan    ArrivalPlan
	Bad     string
}

// decide は今の状態で自動切替が要るかを決める（表 0c 3.2）。要るなら ok。Handle の判定と、Auto のバッチの承認の
// 両方がこの1か所を使う（Pending や再送の時刻は見ない）。健康はいつも今の材料から計算する。
//  1. auto なら、スタックを下から見て、戻る方向で最初に入れる要素へ戻る（AutoRecover）。
//  2. 戻れず、現在が不良の監視対象で、持ち主が none か auto なら、逃がす（AutoFail）。
func (m *Machine) decide() (autoDecision, bool) {
	s := &m.st
	if !s.Connected || !s.Streaming || !s.AutoEnabled || m.suppressed() || s.StandbyOwner == OwnerManual {
		return autoDecision{}, false
	}
	enter := m.enterFunc()
	stack := m.stackNow()
	if i := slices.Index(stack, s.Scene); i >= 0 {
		stack = stack[:i] // 現在のシーンとそれより上は履歴でない（整理の前でも同じに扱う）
	}
	if len(stack) > 0 {
		if pl := FindReturn(stack, enter); pl.Kind == PlanReturn {
			return autoDecision{Purpose: PurposeAutoRecover, Target: pl.Target, Plan: ArrivalPlan{Plan: pl}}, true
		}
	}
	if m.classOf(s.Scene) != ClassMonitored {
		return autoDecision{}, false
	}
	h, bad, has := m.healthOf(s.Scene)
	if !isBad(h, has) || m.recoverInFlight() {
		return autoDecision{}, false
	}
	pl := FindEscape(EscapeInput{Current: s.Scene, Stack: stack, Chain: m.resolved(), Enter: enter})
	switch pl.Kind {
	case PlanEscape:
		return autoDecision{Purpose: PurposeAutoFail, Target: pl.Target, Plan: ArrivalPlan{Plan: pl}, Bad: bad}, true
	case PlanReturn:
		// 手順1で先に選ぶので通常は届かない（防御）。
		return autoDecision{Purpose: PurposeAutoRecover, Target: pl.Target, Plan: ArrivalPlan{Plan: pl}}, true
	}
	return autoDecision{}, false
}

// resolved は設定の名前を今の地図で引き直した結果。
func (m *Machine) resolved() Resolved {
	return ResolveFallbacks(m.st.Map, m.st.Standby, m.st.Fallbacks)
}

// judge は自動切替の判定。要るときで、同じ Purpose の未完了のバッチも切替先への期待もなく、同じ候補の前回の送出から
// 再送の間隔がたっているときだけ、バッチを返す。同じ候補の歯止めは全入口（Tick、BatchDone、Event）に当て、
// 別の候補（行き先・計画が違う）は直ちに判断する。
func (m *Machine) judge(at time.Time) *Batch {
	s := &m.st
	d, ok := m.decide()
	if !ok || m.hasPending(d.Purpose) || m.expecting(d.Target) {
		return nil
	}
	key := m.candidateKey(d.Purpose, d.Target, d.Plan)
	if last, ok := s.LastCandidateSent[key]; ok && at.Sub(last) < m.cfg.RetryInterval {
		return nil
	}
	s.LastCandidateSent[key] = at
	if d.Purpose == PurposeAutoFail {
		return m.emit(d.Purpose, at, []Action{SetScene{SceneUUID: d.Target}}, s.Scene, d.Bad, d.Plan)
	}
	return m.emit(d.Purpose, at, m.restoreActions(d.Target), "", "", d.Plan)
}

// candidateKey は候補の識別（再送の歯止めの鍵）。
func (m *Machine) candidateKey(p Purpose, target string, plan ArrivalPlan) candKey {
	return makeCandKey(&m.st, p, target, plan)
}

func makeCandKey(s *state, p Purpose, target string, plan ArrivalPlan) candKey {
	return candKey{Purpose: p, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, SceneEpoch: s.SceneEpoch, ConfigEpoch: s.ConfigEpoch,
		Target: target, Plan: fmt.Sprintf("%d|%s|%s", plan.Plan.Kind, plan.Plan.Target, strings.Join(plan.Plan.Append, ","))}
}

func (m *Machine) hasPending(ps ...Purpose) bool {
	for _, pb := range m.st.Pending {
		if slices.Contains(ps, pb.Purpose) {
			return true
		}
	}
	return false
}

// expecting はシーン scene（UUID）への期待が残っているかを返す（登録した Purpose は問わない）。
func (m *Machine) expecting(scene string) bool {
	return slices.ContainsFunc(m.st.Expectations, func(x Expectation) bool { return x.Scene == scene })
}

// emit はバッチを1本作り、未完了として持つ。ret はバッチが持つ戻り先、bad は AutoFail の悪い代表のカメラ、
// plan は届いたときに当てる計画。
func (m *Machine) emit(p Purpose, at time.Time, actions []Action, ret, bad string, plan ArrivalPlan) *Batch {
	s := &m.st
	id := s.NextBatchID
	s.NextBatchID++
	s.Pending[id] = PendingBatch{
		BatchID: id, Purpose: p,
		SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch,
		ReturnScene: ret, Plan: plan.clone(),
	}
	if p.IsAuto() {
		s.LastAutoSent[p] = at
		// 古い世代の候補の時刻は捨てる。
		for k := range s.LastCandidateSent {
			if k.ConnEpoch != s.ConnEpoch || k.StreamEpoch != s.StreamEpoch || k.SceneEpoch != s.SceneEpoch || k.ConfigEpoch != s.ConfigEpoch {
				delete(s.LastCandidateSent, k)
			}
		}
	}
	return &Batch{
		BatchID: id, Purpose: p,
		SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch,
		ReturnScene: ret, BadCamera: bad, Actions: actions, Plan: plan.clone(),
	}
}

// ---- 手動操作 ----

// onCommand は手動操作を受け付ける。受け付けたら Command のバッチだけを返す（自動切替の判定は
// 次の Event で行う）。受け付けないときは cmdErr を返し、状態を変えない。
func (m *Machine) onCommand(e Command) (*Batch, error) {
	s := &m.st
	if !s.Connected {
		return nil, ErrNotConnected{}
	}
	if reason := m.commandRefusal(e.Kind); reason != "" {
		return nil, ErrInvalidState{Reason: reason}
	}
	switch e.Kind {
	case CmdStreamStart:
		return m.emit(PurposeStartStream, e.At, []Action{StartStream{}}, "", "", ArrivalPlan{}), nil
	case CmdStreamStop:
		return m.emit(PurposeStopStream, e.At, []Action{StopStream{}}, "", "", ArrivalPlan{}), nil
	case CmdStandbyOn:
		if s.Scene == s.StandbyUUID && s.StandbyOwner == OwnerAuto {
			// 今すでに待機で auto の ON は、切替を送らず、Handle の中で manual に固定する（裁定 9）。戻り先はスタックの最下段（ReturnScene）。
			// 残っている自動の期待は照合だけにする（持ち主・スタック・戻り先へ作用しない）。
			s.StandbyOwner = OwnerManual
			s.Stack = nil
			m.disableExpectations()
			for i := range s.Expectations {
				if s.Expectations[i].Purpose.IsAuto() {
					s.Expectations[i].Plan.DisabledByOn = true // 古い設定の世代の期待にも付ける
				}
			}
			return nil, nil
		}
		return m.emit(PurposeStandbyOn, e.At, []Action{SetScene{SceneUUID: s.StandbyUUID}}, m.onReturnTarget(), "", ArrivalPlan{}), nil
	case CmdStandbyOff:
		op, reason := m.offPlan()
		if reason != "" {
			if reason == ReasonOffNoTarget {
				m.recordError(e.At, i18n.New("switch.no_target"))
			}
			return nil, ErrInvalidState{Reason: reason}
		}
		if op.Immediate {
			// 候補が今の待機。切替を送らず、その場で auto と計画した履歴を確定する（明示的な所有権変更）。
			m.setAuto(ApplyPlan(nil, op.Plan.Plan, s.Scene))
			return nil, nil
		}
		return m.emit(op.Purpose, e.At, op.Actions, "", "", op.Plan), nil
	}
	return nil, ErrInvalidState{Reason: "未知の操作"}
}

// commandRefusal は Command を受け付けない理由を返す。受け付けるなら空文字列。
// 確かめる順は、配信の条件、シーンと持ち主の条件、同じ操作の実行待ち、切替の実行中。
func (m *Machine) commandRefusal(k CommandKind) string {
	s := &m.st
	switch k {
	case CmdStreamStart:
		switch {
		case s.Streaming:
			return ReasonStreaming
		case m.hasPending(PurposeStartStream):
			return ReasonSamePending
		}
	case CmdStreamStop:
		switch {
		case !s.Streaming:
			return ReasonNotStreaming
		case m.hasPending(PurposeStopStream):
			return ReasonSamePending
		}
	case CmdStandbyOn:
		inStandby := s.StandbyUUID != "" && s.Scene == s.StandbyUUID
		switch {
		case !s.Streaming:
			return ReasonNotStreaming
		case s.StandbyUUID == "":
			return ReasonStandbyNotFound
		case s.StandbyOwner == OwnerManual || (inStandby && s.StandbyOwner == OwnerNone):
			return ReasonAlreadyStandby
		case m.hasPending(PurposeStandbyOn):
			return ReasonSamePending
		case inStandby:
			// 待機で auto の ON は、切替の未完了・期待が残っていても受け付ける（裁定 9。Handle の中で manual に固定する）。
		case m.hasPending(PurposeStartScene, PurposeAutoFail, PurposeAutoRecover) || len(s.Expectations) > 0:
			// タイムアウトのあとも、期待が残る間は OBS が後から切り替えうる。
			return ReasonSwitchInProcess
		}
	case CmdStandbyOff:
		switch {
		case !s.Streaming:
			return ReasonNotStreaming
		case s.StandbyOwner != OwnerManual:
			return ReasonNotManual
		case m.hasPending(PurposeStandbyOffMonitored, PurposeStandbyOffOther):
			return ReasonSamePending
		}
	default:
		return "未知の操作"
	}
	return ""
}
