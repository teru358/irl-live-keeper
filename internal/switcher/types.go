package switcher

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// SceneClass は現在のシーンを、その時点のシーンの設定で分けたもの。
type SceneClass int

const (
	// ClassUnknown は OBS に未接続などで現在のシーンがわからないこと。どの条件にも当てはまらない。
	ClassUnknown SceneClass = iota
	// ClassMonitored は監視対象のシーン。
	ClassMonitored
	// ClassStandby は待機シーン。
	ClassStandby
	// ClassOther は管理外のシーン。
	ClassOther
)

func (c SceneClass) String() string {
	switch c {
	case ClassMonitored:
		return "monitored"
	case ClassStandby:
		return "standby"
	case ClassOther:
		return "other"
	}
	return "unknown"
}

// Owner は待機の持ち主。誰が待機シーンに入れたか。
type Owner int

const (
	// OwnerNone は本体が待機に入れていないこと（人が手動で待機シーンを選んだ場合を含む）。
	OwnerNone Owner = iota
	// OwnerAuto は自動切替で待機シーンに入れたこと。回復したら戻り先へ戻す。
	OwnerAuto
	// OwnerManual は手動の待機（StandbyOn）で入れたこと。StandbyOff まで自動では戻さない。
	OwnerManual
)

func (o Owner) String() string {
	switch o {
	case OwnerAuto:
		return "auto"
	case OwnerManual:
		return "manual"
	}
	return "none"
}

// Purpose はバッチの目的。承認のたびにこの目的の有効条件を確かめる。
type Purpose int

const (
	PurposeAutoFail Purpose = iota + 1
	PurposeAutoRecover
	PurposeStandbyOn
	PurposeStandbyOffMonitored
	PurposeStandbyOffOther
	PurposeStartScene
	PurposeStartStream
	PurposeStopStream
)

func (p Purpose) String() string {
	switch p {
	case PurposeAutoFail:
		return "AutoFail"
	case PurposeAutoRecover:
		return "AutoRecover"
	case PurposeStandbyOn:
		return "StandbyOn"
	case PurposeStandbyOffMonitored:
		return "StandbyOffMonitored"
	case PurposeStandbyOffOther:
		return "StandbyOffOther"
	case PurposeStartScene:
		return "StartScene"
	case PurposeStartStream:
		return "StartStream"
	case PurposeStopStream:
		return "StopStream"
	}
	return "Purpose?"
}

// IsAuto は自動切替のバッチ（AutoFail / AutoRecover）かを返す。
func (p Purpose) IsAuto() bool {
	return p == PurposeAutoFail || p == PurposeAutoRecover
}

// FromCommand は Command 由来のバッチかを返す。
func (p Purpose) FromCommand() bool {
	switch p {
	case PurposeStandbyOn, PurposeStandbyOffMonitored, PurposeStandbyOffOther, PurposeStartStream, PurposeStopStream:
		return true
	}
	return false
}

// switchesScene はシーンを切り替えるバッチか（承認で SceneEpoch と ConfigEpoch を見るか）を返す。
func (p Purpose) switchesScene() bool {
	return p != PurposeStartStream && p != PurposeStopStream
}

// suppresses は、このバッチが未完了か、その期待が残っている間、自動切替を止めるかを返す。
func (p Purpose) suppresses() bool {
	switch p {
	case PurposeStartScene, PurposeStandbyOn, PurposeStandbyOffMonitored, PurposeStandbyOffOther:
		return true
	}
	return false
}

// ---- Event ----

// Event は Machine への入力。どれも時刻 At を持ち、Machine は時計を読まない。
type Event interface {
	eventAt() time.Time
}

// LinkChanged は受信状態の変化。
type LinkChanged struct {
	At    time.Time
	State ingest.LinkState
}

// OBSConnected は OBS への接続。接続した時点の配信状態、program のシーン、
// 全体のトランジションの長さ（固定長で取れなければ nil）を持つ。
type OBSConnected struct {
	At           time.Time
	Streaming    bool
	Scene        string
	TransitionMs *int
}

// OBSDisconnected は OBS との切断。
type OBSDisconnected struct {
	At time.Time
}

// SceneChanged は program のシーンの変更。
type SceneChanged struct {
	At    time.Time
	Scene string
}

// TransitionChanged は全体のトランジションまたはその長さの変更。
type TransitionChanged struct {
	At           time.Time
	TransitionMs *int
}

// StreamStateChanged は配信状態の変化。
type StreamStateChanged struct {
	At        time.Time
	Streaming bool
}

// ScenesConfigured はシーンの設定。StartScene は配信開始時の切替が無効なら nil。
// 検証は送る側が済ませる。
type ScenesConfigured struct {
	At          time.Time
	Standby     string
	Monitored   []string
	AutoEnabled bool
	StartScene  *string
}

// CommandKind は手動操作の種類。
type CommandKind int

const (
	CmdStreamStart CommandKind = iota + 1
	CmdStreamStop
	CmdStandbyOn
	CmdStandbyOff
)

func (k CommandKind) String() string {
	switch k {
	case CmdStreamStart:
		return "StreamStart"
	case CmdStreamStop:
		return "StreamStop"
	case CmdStandbyOn:
		return "StandbyOn"
	case CmdStandbyOff:
		return "StandbyOff"
	}
	return "Command?"
}

// Command は手動操作。
type Command struct {
	At   time.Time
	Kind CommandKind
}

// RequestResult は送った SetScene の応答の結果。
type RequestResult int

const (
	RequestOK RequestResult = iota + 1
	RequestTimeout
	RequestError
)

// SceneRequestDone は送った SetScene の応答。期待の期限の起点になる。
type SceneRequestDone struct {
	At     time.Time
	ReqID  int
	Result RequestResult
}

// BatchResult はバッチの終わり方。
type BatchResult int

const (
	BatchCompleted BatchResult = iota + 1
	BatchFailed
	BatchRejected
)

// BatchDone はバッチの終わり。成功、失敗、承認の却下のどれでも1回だけ届く。
type BatchDone struct {
	At      time.Time
	BatchID int
	Result  BatchResult
	Err     error
}

// Tick は1秒ごとの時刻の知らせ。再送と期待の期限の判定に使う。
type Tick struct {
	At time.Time
}

func (e LinkChanged) eventAt() time.Time        { return e.At }
func (e OBSConnected) eventAt() time.Time       { return e.At }
func (e OBSDisconnected) eventAt() time.Time    { return e.At }
func (e SceneChanged) eventAt() time.Time       { return e.At }
func (e TransitionChanged) eventAt() time.Time  { return e.At }
func (e StreamStateChanged) eventAt() time.Time { return e.At }
func (e ScenesConfigured) eventAt() time.Time   { return e.At }
func (e Command) eventAt() time.Time            { return e.At }
func (e SceneRequestDone) eventAt() time.Time   { return e.At }
func (e BatchDone) eventAt() time.Time          { return e.At }
func (e Tick) eventAt() time.Time               { return e.At }

// ---- Action ----

// Action は OBS への操作。実行役がバッチの順に1つずつ実行する。
type Action interface {
	isAction()
}

// SetScene は program のシーンを切り替える。Scene は OBS のシーン名そのもの。
type SetScene struct {
	Scene string
}

// RestartMedia はメディアソースに RESTART を送り、Wait だけ待ってから次の Action に進む。
type RestartMedia struct {
	Wait time.Duration
}

// StartStream は配信を始める。
type StartStream struct{}

// StopStream は配信を止める。
type StopStream struct{}

func (SetScene) isAction()     {}
func (RestartMedia) isAction() {}
func (StartStream) isAction()  {}
func (StopStream) isAction()   {}

// Batch は Handle が返す Action の列。作った時点の4つの Epoch を持つ。
// 待機シーンへ切り替えるバッチ（AutoFail、StandbyOn）は、戻り先のシーン名 ReturnScene も持つ。
type Batch struct {
	BatchID     int
	Purpose     Purpose
	SceneEpoch  int
	ConnEpoch   int
	StreamEpoch int
	ConfigEpoch int
	ReturnScene string
	Actions     []Action
}

// ---- Override ----

// Override は切替先シーンのトランジションの上書きの状態。
type Override interface {
	isOverride()
}

// OverrideNone は上書きなし。nil の Override も同じに扱う。
type OverrideNone struct{}

// OverrideKnown は上書きあり・長さがわかる。
type OverrideKnown struct {
	Ms int
}

// OverrideUnknown は上書きあり・長さ不明（読み取りの失敗を含む）。
type OverrideUnknown struct{}

func (OverrideNone) isOverride()    {}
func (OverrideKnown) isOverride()   {}
func (OverrideUnknown) isOverride() {}

// ---- ApproveResult ----

// ApproveResult は承認の結果。
type ApproveResult interface {
	isApproveResult()
}

// Proceed は送ってよいこと。SetScene なら期待を登録し、その ReqID を持つ。
// SetScene 以外では ReqID は 0。
type Proceed struct {
	ReqID int
}

// Skip は送らずに成功とみなすこと（現在のシーンがすでに切替先のシーン）。
type Skip struct{}

// Reject はバッチの残りを捨てること。
type Reject struct{}

// Replace はバッチの残りを捨て、決め直した扱いに置き換えること。Batch が nil なら、
// 代わりに実行するものはない（Machine が Approve の中で自分の状態を変えて済ませた）。
type Replace struct {
	Batch *Batch
}

func (Proceed) isApproveResult() {}
func (Skip) isApproveResult()    {}
func (Reject) isApproveResult()  {}
func (Replace) isApproveResult() {}

// ---- cmdErr ----

// ErrNotConnected は OBS に未接続のため Command を受け付けられないこと。
type ErrNotConnected struct{}

func (ErrNotConnected) Error() string { return "OBS に未接続" }

// ErrInvalidState は今の状態では Command を受け付けられないこと。Reason に理由を持つ。
type ErrInvalidState struct {
	Reason string
}

func (e ErrInvalidState) Error() string { return "受け付けられない: " + e.Reason }

// Command を受け付けない理由の文。
const (
	ReasonStreaming       = "配信中"
	ReasonNotStreaming    = "配信中でない"
	ReasonAlreadyStandby  = "すでに待機シーン"
	ReasonNotManual       = "手動の待機中でない"
	ReasonSamePending     = "同じ操作が実行待ち"
	ReasonSwitchInProcess = "切替の実行中"
)

// ErrorInfo は記録したエラー（時刻と文）。status の last_error になる。
type ErrorInfo struct {
	At  time.Time
	Msg string
}

// ---- 内部状態 ----

// Expectation は送った SetScene の期待。
type Expectation struct {
	ReqID        int
	Scene        string
	Purpose      Purpose
	ReturnScene  string // AutoFail、StandbyOn だけ。ほかは空
	ConfigEpoch  int
	RegisteredAt time.Time
	Override     Override
	TransitionMs *int // Approve の時点の全体の長さ
	Responded    bool
	RespondedAt  time.Time
	Result       RequestResult // OK か Timeout。Error の期待は消すので残らない
	Deadline     time.Time     // Responded のときだけ有効
}

// PendingBatch は BatchDone が来るまで持つ未完了のバッチ。
type PendingBatch struct {
	BatchID        int
	Purpose        Purpose
	SceneEpoch     int
	ConnEpoch      int
	StreamEpoch    int
	ConfigEpoch    int
	ReturnScene    string
	Replaced       bool // Replace で置き換えた元のバッチ。却下を記録しない
	ManualRejected bool // StartScene を手動の切替のために却下した。却下を記録しない
}

// state は Machine の状態。シーンの設定（Standby から ConfigEpoch まで）も含む。
// ConfigEpoch が 0 の間は、シーンの設定をまだ受けていない。
type state struct {
	Standby     string
	Monitored   []string
	AutoEnabled bool
	StartScene  *string
	ConfigEpoch int

	Connected         bool
	Streaming         bool
	Scene             string
	PrevConnScene     string
	PrevConnStreaming bool
	Link              ingest.LinkState
	StandbyOwner      Owner
	ReturnScene       string
	Expectations      []Expectation
	Pending           map[int]PendingBatch
	LastAutoSent      map[Purpose]time.Time
	SceneEpoch        int
	ConnEpoch         int
	StreamEpoch       int
	TransitionMs      *int
	LastError         *ErrorInfo
	NextBatchID       int
	NextReqID         int
}

// Snapshot は status に出す読み取り専用の状態。
type Snapshot struct {
	Connected   bool
	Streaming   bool
	Scene       string
	Class       SceneClass
	Standby     StandbyStatus
	AutoEnabled bool
	Link        ingest.LinkState
	LastError   *ErrorInfo
	// SceneEpoch は人の切替（本体の期待に当たらないシーンの変化）とみなした回数。
	// 増えたことで、core は人の切替があったと知る。
	SceneEpoch int
}

// StandbyStatus は待機の持ち主と戻り先。戻り先がなければ ReturnScene は空。
type StandbyStatus struct {
	Owner       Owner
	ReturnScene string
}
