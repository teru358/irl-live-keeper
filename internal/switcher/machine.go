package switcher

import (
	"slices"
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// Config は Machine の生成時の設定。
type Config struct {
	// RestartWait は RestartMedia.Wait に入れる待ち時間。
	RestartWait time.Duration
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
}

// New は未接続の初期状態の Machine を返す。
func New(cfg Config) *Machine {
	return &Machine{
		cfg: settings{
			RestartWait:   cfg.RestartWait,
			ExpectMargin:  3 * time.Second,
			ExpectFloor:   10 * time.Second,
			RetryInterval: 5 * time.Second,
		},
		st: state{
			CameraStates: map[string]scenemap.CameraState{},
			Pending:      map[int]PendingBatch{},
			LastAutoSent: map[Purpose]time.Time{},
			NextBatchID:  1,
			NextReqID:    1,
		},
	}
}

// Snapshot は status に出す状態を返す。地図や状態の map は外へ直接公開しない。
func (m *Machine) Snapshot() Snapshot {
	s := &m.st
	snap := Snapshot{
		Connected:   s.Connected,
		Streaming:   s.Streaming,
		Scene:       s.Scene,
		SceneName:   m.nameInMap(s.Scene),
		Class:       m.classOf(s.Scene),
		Standby:     StandbyStatus{Owner: s.StandbyOwner, ReturnScene: s.ReturnScene, ReturnSceneName: m.nameInMap(s.ReturnScene)},
		AutoEnabled: s.AutoEnabled,
		SceneEpoch:  s.SceneEpoch,
	}
	if s.Connected && s.Map != nil {
		snap.VisibleCameras = m.visibleCameras(s.Scene)
		snap.Health, snap.BadCamera, snap.HasHealth = m.healthOf(s.Scene)
	}
	if s.LastError != nil {
		e := *s.LastError
		snap.LastError = &e
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
		return one(m.onConfigured(e))
	case CamerasChanged:
		return one(m.onCamerasChanged(e))
	case SceneMapChanged:
		return one(m.onMapChanged(e))
	case ItemEnabledChanged:
		return one(m.onItemEnabled(e))
	case OBSConnected:
		return one(m.onConnected(e))
	case OBSDisconnected:
		m.onDisconnected()
	case SceneChanged:
		return one(m.onSceneChanged(e))
	case TransitionChanged:
		m.st.TransitionMs = copyInt(e.TransitionMs)
	case StreamStateChanged:
		return one(m.onStreamState(e))
	case SceneRequestDone:
		m.onRequestDone(e)
	case BatchDone:
		m.onBatchDone(e)
	case Tick:
		return one(m.judge(at, true))
	case Command:
		b, err := m.onCommand(e)
		if err != nil {
			return nil, err
		}
		return one(b)
	}
	return nil, nil
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

// clearOwner は持ち主と戻り先を消す。今いるシーンは変えない。
func (m *Machine) clearOwner() {
	m.st.StandbyOwner = OwnerNone
	m.st.ReturnScene = ""
}

func (m *Machine) recordError(at time.Time, msg string) {
	m.st.LastError = &ErrorInfo{At: at, Msg: msg}
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
	return slices.Equal(e.Cameras, s.Cameras)
}

// onConfigured はシーンとカメラの設定を差し替える。期待と未完了のバッチはそのまま残す
// （未完了のシーン切替のバッチは、設定の世代が進むので承認で却下される）。
func (m *Machine) onConfigured(e ScenesConfigured) *Batch {
	s := &m.st
	if m.sameConfig(e) {
		return nil
	}
	renamed := e.Standby != s.Standby
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
	s.AutoEnabled = e.AutoEnabled
	s.StartScene = copyString(e.StartScene)
	s.ConfigEpoch++
	switch {
	case s.StandbyOwner == OwnerNone:
	case renamed:
		m.clearOwner()
	case s.StandbyOwner == OwnerAuto && !s.AutoEnabled:
		m.clearOwner()
	}
	m.refine()
	return m.judge(e.At, false)
}

// refine は「整える」共通の手順。ScenesConfigured、SceneMapChanged、ItemEnabledChanged、OBSConnected、
// SceneChanged の更新のあと、判断の前に行う。
//  1. 待機と開始時の名前を今の地図で引き直す。
//  2. 接続中で、持ち主が none 以外なのに現在のシーンが待機 UUID でなければ C。
//  3. 接続中で、持ち主が auto なのに戻り先が監視対象でなければ C。
func (m *Machine) refine() {
	s := &m.st
	s.StandbyUUID = m.resolve(s.Standby)
	s.StartSceneUUID = ""
	if s.StartScene != nil {
		s.StartSceneUUID = m.resolve(*s.StartScene)
	}
	if !s.Connected {
		return
	}
	if s.StandbyOwner != OwnerNone && (s.StandbyUUID == "" || s.Scene != s.StandbyUUID) {
		m.clearOwner()
	}
	if s.StandbyOwner == OwnerAuto && !m.isMonitored(s.ReturnScene) {
		m.clearOwner()
	}
}

// onCamerasChanged は全カメラの組を一括で置き換える。未知のキーは無視し、足りないキーは DOWN に整える。
// 整えた組が今と同じなら何もしない。途中のカメラ状態では判断しない。
func (m *Machine) onCamerasChanged(e CamerasChanged) *Batch {
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
	s.CameraStates = next
	return m.judge(e.At, false)
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
func (m *Machine) onMapChanged(e SceneMapChanged) *Batch {
	s := &m.st
	if !s.Connected || e.Map == nil || sameMap(s.Map, e.Map) {
		return nil
	}
	s.Map = copyMap(e.Map)
	m.refine()
	return m.judge(e.At, false)
}

// onItemEnabled は既知のシーンかグループの項目の表示・非表示を書き換える。未接続、未知の項目、同じ値なら何もしない。
func (m *Machine) onItemEnabled(e ItemEnabledChanged) *Batch {
	s := &m.st
	if !s.Connected || s.Map == nil {
		return nil
	}
	containers := s.Map.Scenes
	c, ok := containers[e.SceneUUID]
	if !ok {
		containers = s.Map.Groups
		c, ok = containers[e.SceneUUID]
	}
	if !ok {
		return nil
	}
	i := slices.IndexFunc(c.Items, func(it scenemap.Item) bool { return it.ID == e.ItemID })
	if i < 0 || c.Items[i].Enabled == e.Enabled {
		return nil
	}
	items := slices.Clone(c.Items)
	items[i].Enabled = e.Enabled
	c.Items = items
	containers[e.SceneUUID] = c
	m.refine()
	return m.judge(e.At, false)
}

// onConnected は地図と状態を取り直す。シーン、配信状態、コレクションのどれかが切断の直前と違っていたら
// 持ち主と戻り先を消す（手動の切替としては扱わず、配信開始時の切替も出さない）。そのあと「整える」を行う。
func (m *Machine) onConnected(e OBSConnected) *Batch {
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
	if streamDiffers {
		s.StreamEpoch++
	}
	if sceneDiffers || streamDiffers || collectionDiffers {
		m.clearOwner()
	}
	m.refine()
	return m.judge(e.At, false)
}

// onDisconnected は期待をすべて消し、地図と解決した UUID を消す。持ち主と戻り先は再接続まで保つ。
func (m *Machine) onDisconnected() {
	s := &m.st
	s.Connected = false
	s.ConnEpoch++
	s.Expectations = nil
	s.PrevConnScene = s.Scene
	s.PrevConnStreaming = s.Streaming
	if s.Map != nil {
		s.PrevConnCollection = s.Map.Collection
	}
	s.Map = nil
	s.StandbyUUID = ""
	s.StartSceneUUID = ""
}

// onSceneChanged は期待と照らし、本体の切替か手動の切替かを見分ける。
func (m *Machine) onSceneChanged(e SceneChanged) *Batch {
	s := &m.st
	s.Scene = e.SceneUUID
	i := slices.IndexFunc(s.Expectations, func(x Expectation) bool { return x.Scene == e.SceneUUID })
	if i < 0 {
		// 手動（外部）の切替。人がシーンを変えたので、再送の時刻も数え直す。
		s.SceneEpoch++
		s.Expectations = nil
		s.LastAutoSent = map[Purpose]time.Time{}
		m.clearOwner()
		m.refine()
		return m.judge(e.At, false)
	}
	// 本体の切替。一致した期待とそれより前の期待、同じシーンの期待を消す。
	hit := s.Expectations[i]
	var kept []Expectation
	for _, x := range s.Expectations[i+1:] {
		if x.Scene != e.SceneUUID {
			kept = append(kept, x)
		}
	}
	s.Expectations = kept
	switch hit.Purpose {
	case PurposeAutoFail, PurposeStandbyOn:
		// 設定が変わる前の期待では、持ち主と戻り先を入れない。
		if hit.ConfigEpoch == s.ConfigEpoch {
			s.StandbyOwner = OwnerAuto
			if hit.Purpose == PurposeStandbyOn {
				s.StandbyOwner = OwnerManual
			}
			s.ReturnScene = hit.ReturnScene
		}
	case PurposeAutoRecover, PurposeStandbyOffMonitored, PurposeStandbyOffOther:
		// 戻り先へ届いたので、設定の世代によらず消す。
		m.clearOwner()
	}
	// 期待の照合のあとに整える（待機でなくなった期待や、戻り先が監視対象外の期待は持ち主を残さない）。
	m.refine()
	return m.judge(e.At, false)
}

func (m *Machine) onStreamState(e StreamStateChanged) *Batch {
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
		return nil
	}
	if s.StartScene == nil {
		return m.judge(e.At, false)
	}
	// 配信開始時の切替。受信状態は見ない。前の配信の StartScene が残っていれば重ねない。
	if m.hasPending(PurposeStartScene) {
		return nil
	}
	if s.StartSceneUUID == "" {
		// 開始シーンが地図で引けない。バッチは出さず（再送もしない）警告を記録し、ほかの判断は続ける。
		m.recordError(e.At, "配信開始時の切替先（"+*s.StartScene+"）が OBS にない")
		return m.judge(e.At, false)
	}
	return m.emit(PurposeStartScene, e.At, []Action{SetScene{SceneUUID: s.StartSceneUUID}}, "", "")
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
			m.recordError(at, x.Purpose.String()+" の切替（"+m.sceneLabel(x.Scene)+"）を確かめられなかった")
		}
	}
	s.Expectations = kept
}

func (m *Machine) onBatchDone(e BatchDone) {
	s := &m.st
	pb, ok := s.Pending[e.BatchID]
	if !ok {
		return
	}
	delete(s.Pending, e.BatchID)
	switch e.Result {
	case BatchCompleted:
		if e.Err != nil {
			m.recordError(e.At, pb.Purpose.String()+" の一部が失敗した: "+e.Err.Error())
		}
	case BatchFailed:
		msg := pb.Purpose.String() + " が失敗した"
		if e.Err != nil {
			msg += ": " + e.Err.Error()
		}
		m.recordError(e.At, msg)
	case BatchRejected:
		if (pb.Purpose.FromCommand() || pb.Purpose == PurposeStartScene) && !pb.Replaced && !pb.ManualRejected {
			m.recordError(e.At, pb.Purpose.String()+" は実行の直前に状況が変わったため取りやめた")
		}
	}
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

// autoNeed は今の状態で自動切替が要るかを決める。要るなら Purpose と切替先を返し、要らなければ 0。
// Handle の判定と、Auto のバッチの承認の両方がこの1か所を使う。健康はいつも今の材料から計算する。
//   - AutoFail: 現在が監視対象で、健康が不良、持ち主 none。切替先は待機 UUID（引けなければ出さない）。
//   - AutoRecover: 現在が待機、持ち主 auto、戻り先が監視対象で、戻り先の健康が OK。切替先は戻り先。
func (m *Machine) autoNeed() (Purpose, string) {
	s := &m.st
	if !s.Connected || !s.Streaming || !s.AutoEnabled || m.suppressed() {
		return 0, ""
	}
	switch c := m.classOf(s.Scene); {
	case c == ClassMonitored && s.StandbyOwner == OwnerNone:
		h, _, has := m.healthOf(s.Scene)
		if isBad(h, has) && s.StandbyUUID != "" {
			return PurposeAutoFail, s.StandbyUUID
		}
	case c == ClassStandby && s.StandbyOwner == OwnerAuto:
		if h, _, has := m.healthOf(s.ReturnScene); has && h == scenemap.StateOK {
			return PurposeAutoRecover, s.ReturnScene
		}
	}
	return 0, ""
}

// judge は自動切替の判定。要るときで、同じ Purpose の未完了のバッチも切替先への期待もなく、
// Tick なら前回の送出から再送の間隔がたっているときだけ、バッチを返す。
func (m *Machine) judge(at time.Time, isTick bool) *Batch {
	s := &m.st
	p, target := m.autoNeed()
	if p == 0 || m.hasPending(p) || m.expecting(target) {
		return nil
	}
	if last, ok := s.LastAutoSent[p]; isTick && ok && at.Sub(last) < m.cfg.RetryInterval {
		return nil
	}
	if p == PurposeAutoFail {
		_, bad, _ := m.healthOf(s.Scene)
		return m.emit(p, at, []Action{SetScene{SceneUUID: target}}, s.Scene, bad)
	}
	return m.emit(p, at, m.restoreActions(target), "", "")
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

// emit はバッチを1本作り、未完了として持つ。ret はバッチが持つ戻り先、bad は AutoFail の悪い代表のカメラ。
func (m *Machine) emit(p Purpose, at time.Time, actions []Action, ret, bad string) *Batch {
	s := &m.st
	id := s.NextBatchID
	s.NextBatchID++
	s.Pending[id] = PendingBatch{
		BatchID: id, Purpose: p,
		SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch,
		ReturnScene: ret,
	}
	if p.IsAuto() {
		s.LastAutoSent[p] = at
	}
	return &Batch{
		BatchID: id, Purpose: p,
		SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch,
		ReturnScene: ret, BadCamera: bad, Actions: actions,
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
		return m.emit(PurposeStartStream, e.At, []Action{StartStream{}}, "", ""), nil
	case CmdStreamStop:
		return m.emit(PurposeStopStream, e.At, []Action{StopStream{}}, "", ""), nil
	case CmdStandbyOn:
		return m.emit(PurposeStandbyOn, e.At, []Action{SetScene{SceneUUID: s.StandbyUUID}}, s.Scene, ""), nil
	case CmdStandbyOff:
		ret := s.ReturnScene
		if !m.isMonitored(ret) {
			// 戻り先が管理外（地図から消えた場合を含む）。再起動なしで切り替える。
			return m.emit(PurposeStandbyOffOther, e.At, []Action{SetScene{SceneUUID: ret}}, "", ""), nil
		}
		if h, _, has := m.healthOf(ret); s.AutoEnabled && isBad(h, has) {
			// 回線が悪いままカメラ映像へ戻さない。回復したら AutoRecover で戻る。
			s.StandbyOwner = OwnerAuto
			return nil, nil
		}
		return m.emit(PurposeStandbyOffMonitored, e.At, m.restoreActions(ret), "", ""), nil
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
		switch {
		case !s.Streaming:
			return ReasonNotStreaming
		case s.StandbyUUID == "":
			return ReasonStandbyNotFound
		case s.Scene == s.StandbyUUID || s.StandbyOwner != OwnerNone:
			return ReasonAlreadyStandby
		case m.hasPending(PurposeStandbyOn):
			return ReasonSamePending
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
