package switcher

import (
	"slices"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
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
			Link:         ingest.LinkDown,
			Pending:      map[int]PendingBatch{},
			LastAutoSent: map[Purpose]time.Time{},
			NextBatchID:  1,
			NextReqID:    1,
		},
	}
}

// Snapshot は status に出す状態を返す。
func (m *Machine) Snapshot() Snapshot {
	s := &m.st
	snap := Snapshot{
		Connected:   s.Connected,
		Streaming:   s.Streaming,
		Scene:       s.Scene,
		Class:       m.class(),
		Standby:     StandbyStatus{Owner: s.StandbyOwner, ReturnScene: s.ReturnScene},
		AutoEnabled: s.AutoEnabled,
		Link:        s.Link,
		SceneEpoch:  s.SceneEpoch,
	}
	if s.LastError != nil {
		e := *s.LastError
		snap.LastError = &e
	}
	return snap
}

// class は現在のシーンを今の設定で分ける。未接続なら不明。
func (m *Machine) class() SceneClass {
	s := &m.st
	switch {
	case !s.Connected:
		return ClassUnknown
	case m.isMonitored(s.Scene):
		return ClassMonitored
	case s.Scene == s.Standby:
		return ClassStandby
	}
	return ClassOther
}

func (m *Machine) isMonitored(scene string) bool { return slices.Contains(m.st.Monitored, scene) }

func bad(l ingest.LinkState) bool { return l == ingest.LinkLow || l == ingest.LinkDown }

// Handle は Event を1つ処理する。返すバッチは多くても1本。
// cmdErr は Command を受け付けられないときだけ返す。
func (m *Machine) Handle(ev Event) (*Batch, error) {
	at := ev.eventAt()
	m.expire(at)
	switch e := ev.(type) {
	case ScenesConfigured:
		return m.onConfigured(e), nil
	case LinkChanged:
		return m.onLinkChanged(e), nil
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
	case BatchDone:
		m.onBatchDone(e)
	case Tick:
		return m.judge(at, true), nil
	case Command:
		return m.onCommand(e)
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

// sameConfig は e が今の設定と同じ内容かを返す。監視対象は集合として比べる。
func (m *Machine) sameConfig(e ScenesConfigured) bool {
	s := &m.st
	if s.ConfigEpoch == 0 || e.Standby != s.Standby || e.AutoEnabled != s.AutoEnabled {
		return false
	}
	if (e.StartScene == nil) != (s.StartScene == nil) || (e.StartScene != nil && *e.StartScene != *s.StartScene) {
		return false
	}
	for _, name := range e.Monitored {
		if !slices.Contains(s.Monitored, name) {
			return false
		}
	}
	for _, name := range s.Monitored {
		if !slices.Contains(e.Monitored, name) {
			return false
		}
	}
	return true
}

// onConfigured はシーンの設定を差し替える。期待と未完了のバッチはそのまま残す
// （未完了のシーン切替のバッチは、設定の世代が進むので承認で却下される）。
func (m *Machine) onConfigured(e ScenesConfigured) *Batch {
	s := &m.st
	if m.sameConfig(e) {
		return nil
	}
	renamed := e.Standby != s.Standby
	s.Standby = e.Standby
	s.Monitored = slices.Clone(e.Monitored)
	s.AutoEnabled = e.AutoEnabled
	s.StartScene = copyString(e.StartScene)
	s.ConfigEpoch++
	switch {
	case s.StandbyOwner == OwnerNone:
	case renamed:
		m.clearOwner()
	case s.StandbyOwner == OwnerAuto && (!m.isMonitored(s.ReturnScene) || !s.AutoEnabled):
		m.clearOwner()
	}
	return m.judge(e.At, false)
}

func (m *Machine) onLinkChanged(e LinkChanged) *Batch {
	if e.State == m.st.Link {
		return nil
	}
	m.st.Link = e.State
	return m.judge(e.At, false)
}

// onConnected は状態を取り直す。シーンか配信状態が切断の直前と違っていたら持ち主と戻り先を消す。
// 手動の切替としては扱わず、配信開始時の切替も出さない。
func (m *Machine) onConnected(e OBSConnected) *Batch {
	s := &m.st
	sceneDiffers := e.Scene != s.PrevConnScene
	streamDiffers := e.Streaming != s.PrevConnStreaming
	s.Connected = true
	s.Streaming = e.Streaming
	s.Scene = e.Scene
	s.TransitionMs = copyInt(e.TransitionMs)
	if streamDiffers {
		s.StreamEpoch++
	}
	if sceneDiffers || streamDiffers {
		m.clearOwner()
	}
	return m.judge(e.At, false)
}

// onDisconnected は期待をすべて消す。持ち主と戻り先は再接続まで保つ。
func (m *Machine) onDisconnected() {
	s := &m.st
	s.Connected = false
	s.ConnEpoch++
	s.Expectations = nil
	s.PrevConnScene = s.Scene
	s.PrevConnStreaming = s.Streaming
}

// onSceneChanged は期待と照らし、本体の切替か手動の切替かを見分ける。
func (m *Machine) onSceneChanged(e SceneChanged) *Batch {
	s := &m.st
	s.Scene = e.Scene
	i := slices.IndexFunc(s.Expectations, func(x Expectation) bool { return x.Scene == e.Scene })
	if i < 0 {
		// 手動（外部）の切替。人がシーンを変えたので、再送の時刻も数え直す。
		s.SceneEpoch++
		s.Expectations = nil
		s.LastAutoSent = map[Purpose]time.Time{}
		m.clearOwner()
		return m.judge(e.At, false)
	}
	// 本体の切替。一致した期待とそれより前の期待、同じシーン名の期待を消す。
	hit := s.Expectations[i]
	var kept []Expectation
	for _, x := range s.Expectations[i+1:] {
		if x.Scene != e.Scene {
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
	return m.emit(PurposeStartScene, e.At, []Action{SetScene{Scene: *s.StartScene}}, "")
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
			m.recordError(at, x.Purpose.String()+" の切替（"+x.Scene+"）を確かめられなかった")
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
// Handle の判定と、Auto のバッチの承認の両方がこの1か所を使う。
func (m *Machine) autoNeed() (Purpose, string) {
	s := &m.st
	if !s.Connected || !s.Streaming || !s.AutoEnabled || m.suppressed() {
		return 0, ""
	}
	switch c := m.class(); {
	case c == ClassMonitored && bad(s.Link) && s.StandbyOwner == OwnerNone:
		return PurposeAutoFail, s.Standby
	case c == ClassStandby && s.StandbyOwner == OwnerAuto && s.Link == ingest.LinkOK:
		return PurposeAutoRecover, s.ReturnScene
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
		return m.emit(p, at, []Action{SetScene{Scene: target}}, s.Scene)
	}
	return m.emit(p, at, []Action{RestartMedia{Wait: m.cfg.RestartWait}, SetScene{Scene: target}}, "")
}

func (m *Machine) hasPending(ps ...Purpose) bool {
	for _, pb := range m.st.Pending {
		if slices.Contains(ps, pb.Purpose) {
			return true
		}
	}
	return false
}

// expecting はシーン名 scene への期待が残っているかを返す（登録した Purpose は問わない）。
func (m *Machine) expecting(scene string) bool {
	return slices.ContainsFunc(m.st.Expectations, func(x Expectation) bool { return x.Scene == scene })
}

// emit はバッチを1本作り、未完了として持つ。ret はバッチが持つ戻り先。
func (m *Machine) emit(p Purpose, at time.Time, actions []Action, ret string) *Batch {
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
		ReturnScene: ret, Actions: actions,
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
		return m.emit(PurposeStartStream, e.At, []Action{StartStream{}}, ""), nil
	case CmdStreamStop:
		return m.emit(PurposeStopStream, e.At, []Action{StopStream{}}, ""), nil
	case CmdStandbyOn:
		return m.emit(PurposeStandbyOn, e.At, []Action{SetScene{Scene: s.Standby}}, s.Scene), nil
	case CmdStandbyOff:
		ret := s.ReturnScene
		if !m.isMonitored(ret) {
			return m.emit(PurposeStandbyOffOther, e.At, []Action{SetScene{Scene: ret}}, ""), nil
		}
		if s.AutoEnabled && bad(s.Link) {
			// 回線が悪いままカメラ映像へ戻さない。回復したら AutoRecover で戻る。
			s.StandbyOwner = OwnerAuto
			return nil, nil
		}
		return m.emit(PurposeStandbyOffMonitored, e.At,
			[]Action{RestartMedia{Wait: m.cfg.RestartWait}, SetScene{Scene: ret}}, ""), nil
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
		case s.Scene == s.Standby || s.StandbyOwner != OwnerNone:
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
