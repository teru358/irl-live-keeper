package switcher

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// Config は Machine の設定。
type Config struct {
	// Scenes は役割から OBS のシーン名への対応（Intro / Live / Fail / Pause）。
	Scenes map[Role]string
	// RestartWait は RestartMedia.Wait に入れる待ち時間。
	RestartWait time.Duration
}

// settings は生成時に決まり、変わらない値。
type settings struct {
	Scenes        map[Role]string
	RestartWait   time.Duration
	ExpectMargin  time.Duration // 期待の期限の余裕
	ExpectFloor   time.Duration // 上書きあり・長さ不明などのときの下限
	RetryInterval time.Duration // Tick の再送の間隔
}

// Machine はシーンを切り替える状態機械。I/O を持たず、時計も読まない。
// 時刻は Event.At と Approve の at だけから得る。
type Machine struct {
	cfg settings
	st  state
}

// New は未接続の初期状態の Machine を返す。
func New(cfg Config) *Machine {
	scenes := make(map[Role]string, len(cfg.Scenes))
	for r, s := range cfg.Scenes {
		scenes[r] = s
	}
	return &Machine{
		cfg: settings{
			Scenes:        scenes,
			RestartWait:   cfg.RestartWait,
			ExpectMargin:  3 * time.Second,
			ExpectFloor:   10 * time.Second,
			RetryInterval: 5 * time.Second,
		},
		st: state{
			Link:         ingest.LinkDown,
			PausePrev:    RoleUnknown,
			Pending:      map[int]PendingBatch{},
			LastAutoSent: map[Purpose]time.Time{},
			NextBatchID:  1,
			NextReqID:    1,
			NextStartID:  1,
		},
	}
}

// Snapshot は status に出す状態を返す。
func (m *Machine) Snapshot() Snapshot {
	s := &m.st
	snap := Snapshot{
		Connected: s.Connected,
		Streaming: s.Streaming,
		Scene:     s.Scene,
		Role:      s.Role,
		Link:      s.Link,
		Hold:      HoldStatus{Active: s.Hold, Since: s.HoldSince},
	}
	if s.LastError != nil {
		e := *s.LastError
		snap.LastError = &e
	}
	return snap
}

// roleOf はシーン名を役割に変換する。どれにも当たらなければ RoleOther。
func (m *Machine) roleOf(scene string) Role {
	for _, r := range []Role{RoleIntro, RoleLive, RoleFail, RolePause} {
		if name, ok := m.cfg.Scenes[r]; ok && name == scene {
			return r
		}
	}
	return RoleOther
}

func bad(l ingest.LinkState) bool { return l == ingest.LinkLow || l == ingest.LinkDown }

// Handle は Event を1つ処理する。返すバッチは多くても1本。
// cmdErr は Command を受け付けられないときだけ返す。
func (m *Machine) Handle(ev Event) (*Batch, error) {
	at := ev.eventAt()
	m.expire(at)
	switch e := ev.(type) {
	case LinkChanged:
		return m.onLinkChanged(e), nil
	case OBSConnected:
		return m.onConnected(e), nil
	case OBSDisconnected:
		m.onDisconnected(e)
	case SceneChanged:
		return m.onSceneChanged(e), nil
	case TransitionChanged:
		m.st.TransitionMs = copyInt(e.TransitionMs)
	case StreamStateChanged:
		return m.onStreamState(e), nil
	case SceneRequestDone:
		m.onRequestDone(e)
	case BatchDone:
		return m.onBatchDone(e), nil
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

func (m *Machine) onLinkChanged(e LinkChanged) *Batch {
	s := &m.st
	if e.State == s.Link {
		return nil
	}
	old := s.Link
	s.Link = e.State
	// 保持は OK と不良の間の変化で解く。接続の有無は見ない。
	if s.Hold && bad(old) != bad(e.State) {
		m.releaseHold()
	}
	return m.judge(e.At, false)
}

func (m *Machine) onConnected(e OBSConnected) *Batch {
	s := &m.st
	sceneDiffers := e.Scene != s.PrevConnScene
	streamDiffers := e.Streaming != s.PrevConnStreaming
	s.Connected = true
	s.Streaming = e.Streaming
	s.Scene = e.Scene
	s.Role = m.roleOf(e.Scene)
	s.TransitionMs = copyInt(e.TransitionMs)
	s.PausePrev = RoleUnknown
	if streamDiffers {
		s.StreamEpoch++
	}
	// 切断中にシーンが変わっていても手動の切替とはみなさない。保持は解くだけ。
	if s.Hold && (sceneDiffers || streamDiffers) {
		m.releaseHold()
	}
	return m.judge(e.At, false)
}

func (m *Machine) onDisconnected(e OBSDisconnected) {
	s := &m.st
	s.Connected = false
	s.ConnEpoch++
	s.Expectations = nil
	s.PrevConnScene = s.Scene
	s.PrevConnStreaming = s.Streaming
	m.dropStartWait(e.At, "OBS との接続が切れたため配信を始めなかった")
}

func (m *Machine) onSceneChanged(e SceneChanged) *Batch {
	s := &m.st
	prev := s.Role
	newRole := m.roleOf(e.Scene)
	matched := -1
	for i, x := range s.Expectations {
		if x.Role == newRole {
			matched = i
			break
		}
	}
	s.Scene = e.Scene
	s.Role = newRole
	m.updatePausePrev(prev, newRole)

	if matched >= 0 {
		// 本体の切替。一致した期待とそれより前の期待を消す。
		s.Expectations = append([]Expectation(nil), s.Expectations[matched+1:]...)
		if newRole == RoleIntro && s.StartWait != nil && s.StartWait.Stage == StageWaitIntro {
			s.StartWait.Stage = StageIntroConfirmed
			return m.emit(PurposeStartStream, e.At, s.StartWait.ID)
		}
		return m.judge(e.At, false)
	}

	// 手動（外部）の切替。
	s.SceneEpoch++
	s.Expectations = nil
	if s.Streaming && (newRole == RoleLive || newRole == RoleFail) {
		if !s.Hold {
			s.Hold = true
			s.HoldSince = e.At
		}
	} else if s.Hold {
		m.releaseHold()
	}
	m.dropStartWait(e.At, "配信開始の待ちの間にシーンが手動で切り替えられたため配信を始めなかった")
	return nil
}

// updatePausePrev は役割の変化に合わせて Pause の直前の役割を決める。
func (m *Machine) updatePausePrev(prev, next Role) {
	s := &m.st
	switch {
	case next != RolePause:
		s.PausePrev = RoleUnknown
	case prev == RolePause:
		// Pause のまま。変えない。
	case prev == RoleIntro || prev == RoleLive || prev == RoleFail:
		s.PausePrev = prev
	default:
		s.PausePrev = RoleUnknown
	}
}

func (m *Machine) onStreamState(e StreamStateChanged) *Batch {
	s := &m.st
	if e.Streaming == s.Streaming {
		return nil
	}
	s.Streaming = e.Streaming
	s.StreamEpoch++
	if e.Streaming {
		// 配信が始まったので待ちは成功として消す。記録しない。
		s.StartWait = nil
		return m.judge(e.At, false)
	}
	s.Expectations = nil
	s.LastAutoSent = map[Purpose]time.Time{}
	s.PausePrev = RoleUnknown
	if s.Hold {
		m.releaseHold()
	}
	return nil
}

func (m *Machine) onRequestDone(e SceneRequestDone) {
	s := &m.st
	for i := range s.Expectations {
		x := &s.Expectations[i]
		if x.ReqID != e.ReqID {
			continue
		}
		if e.Result == RequestError {
			failed := *x
			s.Expectations = append(s.Expectations[:i:i], s.Expectations[i+1:]...)
			m.dropWaitingIntro(failed, e.At, "Intro への切替が失敗したため配信を始めなかった")
			return
		}
		x.Responded = true
		x.RespondedAt = e.At
		x.Result = e.Result
		x.Deadline = e.At.Add(m.expectWindow(*x))
		return
	}
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

// expire は期限を過ぎた期待を消す。期限ちょうどは残す。
// 期限切れそのものは手動の切替として扱わない。
func (m *Machine) expire(at time.Time) {
	s := &m.st
	kept := s.Expectations[:0:0]
	var expired []Expectation
	for _, x := range s.Expectations {
		if x.Responded && at.After(x.Deadline) {
			expired = append(expired, x)
			continue
		}
		kept = append(kept, x)
	}
	if len(expired) == 0 {
		return
	}
	s.Expectations = kept
	for _, x := range expired {
		m.dropWaitingIntro(x, at, "Intro への切替を確かめられなかったため配信を始めなかった")
	}
}

func (m *Machine) releaseHold() {
	m.st.Hold = false
	m.st.HoldSince = time.Time{}
}

func (m *Machine) recordError(at time.Time, msg string) {
	m.st.LastError = &ErrorInfo{At: at, Msg: msg}
}

// dropStartWait は配信開始の待ちを成功以外で消し、記録する。待ちがなければ何もしない。
func (m *Machine) dropStartWait(at time.Time, msg string) {
	if m.st.StartWait == nil {
		return
	}
	m.st.StartWait = nil
	m.recordError(at, msg)
}

// dropWaitingIntro は、Intro への期待 x が期限切れか失敗になったときに、x を登録した
// StreamStart の待ちが「Intro の確認待ち」の段階なら消す。前の StreamStart の期待では、
// 今の待ちを消さない。
func (m *Machine) dropWaitingIntro(x Expectation, at time.Time, msg string) {
	w := m.st.StartWait
	if x.Role != RoleIntro || w == nil || w.Stage != StageWaitIntro || x.StartID != w.ID {
		return
	}
	m.dropStartWait(at, msg)
}

// judge は自動切替の判定。すべての条件を満たすときだけ AutoFail か AutoRecover の
// バッチを返す。isTick なら再送の間隔の条件を足す。
func (m *Machine) judge(at time.Time, isTick bool) *Batch {
	s := &m.st
	if !s.Connected || !s.Streaming || s.Hold {
		return nil
	}
	var p Purpose
	var target Role
	switch {
	case s.Role == RoleLive && bad(s.Link):
		p, target = PurposeAutoFail, RoleFail
	case s.Role == RoleFail && s.Link == ingest.LinkOK:
		p, target = PurposeAutoRecover, RoleLive
	default:
		return nil
	}
	if m.hasPending(p) || m.expecting(target) || m.expectingOutsideAuto() {
		return nil
	}
	if last, ok := s.LastAutoSent[p]; isTick && ok && at.Sub(last) < m.cfg.RetryInterval {
		return nil
	}
	return m.emit(p, at, 0)
}

func (m *Machine) hasPending(ps ...Purpose) bool {
	for _, pb := range m.st.Pending {
		for _, p := range ps {
			if pb.Purpose == p {
				return true
			}
		}
	}
	return false
}

func (m *Machine) expecting(r Role) bool {
	for _, x := range m.st.Expectations {
		if x.Role == r {
			return true
		}
	}
	return false
}

// expectingOutsideAuto は Live / Fail 以外の役割（Pause、Intro）への期待が残っているかを返す。
// 残っている間は、切替の途中なので自動切替をしない。
func (m *Machine) expectingOutsideAuto() bool {
	for _, x := range m.st.Expectations {
		if x.Role != RoleLive && x.Role != RoleFail {
			return true
		}
	}
	return false
}

// actionsFor は Purpose ごとの Action の列を返す。
func (m *Machine) actionsFor(p Purpose) []Action {
	restart := RestartMedia{Wait: m.cfg.RestartWait}
	switch p {
	case PurposeAutoFail, PurposeGoFail, PurposePauseOffFail:
		return []Action{SetScene{Role: RoleFail}}
	case PurposeAutoRecover, PurposeGoLive, PurposePauseOffLive:
		return []Action{restart, SetScene{Role: RoleLive}}
	case PurposePauseOffIntro, PurposeStartIntro:
		return []Action{SetScene{Role: RoleIntro}}
	case PurposePauseOn:
		return []Action{SetScene{Role: RolePause}}
	case PurposeStartStream:
		return []Action{StartStream{}}
	case PurposeStopStream:
		return []Action{StopStream{}}
	}
	return nil
}

// emit はバッチを1本作り、未完了として持つ。startID は StartIntro / StartStream の待ちの番号。
func (m *Machine) emit(p Purpose, at time.Time, startID int) *Batch {
	s := &m.st
	id := s.NextBatchID
	s.NextBatchID++
	s.Pending[id] = PendingBatch{
		BatchID: id, Purpose: p,
		SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch,
		StartID: startID,
	}
	if p.IsAuto() {
		s.LastAutoSent[p] = at
	}
	return &Batch{
		BatchID: id, Purpose: p,
		SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch,
		Actions: m.actionsFor(p),
	}
}

func (m *Machine) onBatchDone(e BatchDone) *Batch {
	s := &m.st
	pb, ok := s.Pending[e.BatchID]
	if !ok {
		return nil
	}
	delete(s.Pending, e.BatchID)
	// 配信開始の待ちには、同じ番号のバッチだけが作用する。
	var own *StartWait
	if w := s.StartWait; w != nil && pb.StartID != 0 && w.ID == pb.StartID {
		own = w
	}
	switch e.Result {
	case BatchCompleted:
		if e.Err != nil {
			m.recordError(e.At, pb.Purpose.String()+" の一部が失敗した: "+e.Err.Error())
		}
		switch {
		case own == nil:
		case pb.Purpose == PurposeStartStream:
			s.StartWait = nil
		case pb.Purpose == PurposeStartIntro && own.Stage == StageWaitIntro && s.Role == RoleIntro:
			// SetScene が Skip されたので、期待どおりの SceneChanged は来ない。ここで確かめたことにする。
			own.Stage = StageIntroConfirmed
			return m.emit(PurposeStartStream, e.At, own.ID)
		}
	case BatchFailed:
		if own != nil && pb.Purpose == PurposeStartIntro && own.Stage == StageIntroConfirmed {
			// Intro にはなっているので、StartIntro の目的は達している。
			return nil
		}
		if own != nil {
			s.StartWait = nil
		}
		m.recordError(e.At, failMsg(pb.Purpose, e.Err))
	case BatchRejected:
		if own != nil {
			s.StartWait = nil
		}
		if !pb.Purpose.IsAuto() && !pb.Replaced {
			m.recordError(e.At, pb.Purpose.String()+" は実行の直前に状況が変わったため取りやめた")
		}
	}
	return nil
}

func failMsg(p Purpose, err error) string {
	msg := p.String() + " が失敗した"
	if err != nil {
		msg += ": " + err.Error()
	}
	return msg
}

// onCommand は手動操作を受け付ける。受け付けたら保持を解き、Command のバッチだけを返す。
// 受け付けないときは cmdErr を返し、状態を変えない。
func (m *Machine) onCommand(e Command) (*Batch, error) {
	s := &m.st
	if !s.Connected {
		return nil, ErrNotConnected{}
	}
	if reason := m.commandRefusal(e.Kind); reason != "" {
		return nil, ErrInvalidState{Reason: reason}
	}
	m.releaseHold()
	switch e.Kind {
	case CmdStreamStart:
		id := s.NextStartID
		s.NextStartID++
		if s.Role == RoleIntro {
			s.StartWait = &StartWait{ID: id, Stage: StageIntroConfirmed}
			return m.emit(PurposeStartStream, e.At, id), nil
		}
		s.StartWait = &StartWait{ID: id, Stage: StageWaitIntro}
		return m.emit(PurposeStartIntro, e.At, id), nil
	case CmdStreamStop:
		return m.emit(PurposeStopStream, e.At, 0), nil
	case CmdGoLive:
		if s.Link == ingest.LinkOK {
			return m.emit(PurposeGoLive, e.At, 0), nil
		}
		return m.emit(PurposeGoFail, e.At, 0), nil
	case CmdPauseOn:
		return m.emit(PurposePauseOn, e.At, 0), nil
	case CmdPauseOff:
		switch {
		case s.PausePrev == RoleIntro:
			return m.emit(PurposePauseOffIntro, e.At, 0), nil
		case s.Link == ingest.LinkOK:
			return m.emit(PurposePauseOffLive, e.At, 0), nil
		default:
			return m.emit(PurposePauseOffFail, e.At, 0), nil
		}
	}
	return nil, ErrInvalidState{Reason: "未知の操作"}
}

// commandRefusal は Command を受け付けない理由を返す。受け付けるなら空文字列。
// 同じ操作のバッチが未完了の間も受け付けない。
func (m *Machine) commandRefusal(k CommandKind) string {
	s := &m.st
	var same []Purpose
	switch k {
	case CmdStreamStart:
		if s.Streaming {
			return "配信中"
		}
		if s.StartWait != nil {
			return "配信開始の待ちがある"
		}
		same = []Purpose{PurposeStartIntro, PurposeStartStream}
	case CmdStreamStop:
		if !s.Streaming {
			return "配信中でない"
		}
		same = []Purpose{PurposeStopStream}
	case CmdGoLive:
		if !s.Streaming {
			return "配信中でない"
		}
		if s.Role != RoleIntro {
			return "役割が Intro でない"
		}
		same = []Purpose{PurposeGoLive, PurposeGoFail}
	case CmdPauseOn:
		if !s.Streaming {
			return "配信中でない"
		}
		if s.Role != RoleIntro && s.Role != RoleLive && s.Role != RoleFail {
			return "役割が Intro / Live / Fail でない"
		}
		same = []Purpose{PurposePauseOn}
	case CmdPauseOff:
		if !s.Streaming {
			return "配信中でない"
		}
		if s.Role != RolePause {
			return "役割が Pause でない"
		}
		same = []Purpose{PurposePauseOffLive, PurposePauseOffFail, PurposePauseOffIntro}
	default:
		return "未知の操作"
	}
	if m.hasPending(same...) {
		return "同じ操作が実行待ち"
	}
	return ""
}
