package switcher

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// Role はシーンの役割。OBS のシーン名を config の割り当てで変換したもの。
type Role int

const (
	// RoleUnknown は Pause の直前の役割が「不明」であることを表す。
	// シーンの役割としては使わない。
	RoleUnknown Role = iota
	RoleIntro
	RoleLive
	RoleFail
	RolePause
	// RoleOther は管理外のシーン。
	RoleOther
)

func (r Role) String() string {
	switch r {
	case RoleIntro:
		return "intro"
	case RoleLive:
		return "live"
	case RoleFail:
		return "fail"
	case RolePause:
		return "pause"
	case RoleOther:
		return "other"
	}
	return "unknown"
}

// Purpose はバッチの目的。承認のたびにこの目的の有効条件を確かめる。
type Purpose int

const (
	PurposeAutoFail Purpose = iota + 1
	PurposeAutoRecover
	PurposeGoLive
	PurposeGoFail
	PurposePauseOffLive
	PurposePauseOffFail
	PurposePauseOffIntro
	PurposePauseOn
	PurposeStartIntro
	PurposeStartStream
	PurposeStopStream
)

func (p Purpose) String() string {
	switch p {
	case PurposeAutoFail:
		return "AutoFail"
	case PurposeAutoRecover:
		return "AutoRecover"
	case PurposeGoLive:
		return "GoLive"
	case PurposeGoFail:
		return "GoFail"
	case PurposePauseOffLive:
		return "PauseOffLive"
	case PurposePauseOffFail:
		return "PauseOffFail"
	case PurposePauseOffIntro:
		return "PauseOffIntro"
	case PurposePauseOn:
		return "PauseOn"
	case PurposeStartIntro:
		return "StartIntro"
	case PurposeStartStream:
		return "StartStream"
	case PurposeStopStream:
		return "StopStream"
	}
	return "Purpose?"
}

// IsAuto は自動切替のバッチ（AutoFail / AutoRecover）かを返す。
// それ以外はすべて Command 由来である。
func (p Purpose) IsAuto() bool {
	return p == PurposeAutoFail || p == PurposeAutoRecover
}

// switchesScene はシーンを切り替えるバッチか（承認で SceneEpoch を見るか）を返す。
func (p Purpose) switchesScene() bool {
	return p != PurposeStartStream && p != PurposeStopStream
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

// CommandKind は手動操作の種類。
type CommandKind int

const (
	CmdStreamStart CommandKind = iota + 1
	CmdStreamStop
	CmdGoLive
	CmdPauseOn
	CmdPauseOff
)

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
func (e Command) eventAt() time.Time            { return e.At }
func (e SceneRequestDone) eventAt() time.Time   { return e.At }
func (e BatchDone) eventAt() time.Time          { return e.At }
func (e Tick) eventAt() time.Time               { return e.At }

// ---- Action ----

// Action は OBS への操作。実行役がバッチの順に1つずつ実行する。
type Action interface {
	isAction()
}

// SetScene は program のシーンを役割のシーンに切り替える。
type SetScene struct {
	Role Role
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

// Batch は Handle や Approve が返す Action の列。作った時点の3つの Epoch を持つ。
type Batch struct {
	BatchID     int
	Purpose     Purpose
	SceneEpoch  int
	ConnEpoch   int
	StreamEpoch int
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

// Skip は送らずに成功とみなすこと（すでに目的の役割になっている）。
type Skip struct{}

// Reject はバッチの残りを捨てること。
type Reject struct{}

// Replace はバッチの残りを捨て、代わりの Batch を実行すること。
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

// ErrorInfo は記録したエラー（時刻と文）。status の last_error になる。
type ErrorInfo struct {
	At  time.Time
	Msg string
}

// ---- 内部状態 ----

// Expectation は送った SetScene の期待シーン。
type Expectation struct {
	ReqID        int
	Role         Role
	RegisteredAt time.Time
	Override     Override
	TransitionMs *int // Approve の時点の全体の長さ
	Responded    bool
	RespondedAt  time.Time
	Result       RequestResult // OK か Timeout。Error の期待は消すので残らない
	Deadline     time.Time     // Responded のときだけ有効
	StartID      int           // StartIntro が登録した期待なら、その配信開始の待ちの番号。ほかは 0
}

// StartStage は配信開始の待ちの段階。
type StartStage int

const (
	// StageWaitIntro は Intro の確認待ち。
	StageWaitIntro StartStage = iota + 1
	// StageIntroConfirmed は Intro 確認済み。
	StageIntroConfirmed
)

// StartWait は配信開始の待ち。ID は StreamStart ごとの通し番号。
type StartWait struct {
	ID    int
	Stage StartStage
}

// PendingBatch は BatchDone が来るまで持つ未完了のバッチ。
type PendingBatch struct {
	BatchID     int
	Purpose     Purpose
	SceneEpoch  int
	ConnEpoch   int
	StreamEpoch int
	StartID     int  // StartIntro / StartStream だけ。どの待ちのためのバッチか
	Replaced    bool // Replace で置き換えた元のバッチ。却下を記録しない
}

// state は Machine の状態。
type state struct {
	Connected         bool
	Streaming         bool
	Scene             string
	Role              Role
	PrevConnScene     string
	PrevConnStreaming bool
	Link              ingest.LinkState
	Hold              bool
	HoldSince         time.Time
	Expectations      []Expectation
	PausePrev         Role
	StartWait         *StartWait
	Pending           map[int]PendingBatch
	LastAutoSent      map[Purpose]time.Time
	SceneEpoch        int
	ConnEpoch         int
	StreamEpoch       int
	TransitionMs      *int
	LastError         *ErrorInfo
	NextBatchID       int
	NextReqID         int
	NextStartID       int
}

// Snapshot は status に出す読み取り専用の状態。
type Snapshot struct {
	Connected bool
	Streaming bool
	Scene     string
	Role      Role
	Link      ingest.LinkState
	Hold      HoldStatus
	LastError *ErrorInfo
}

// HoldStatus は手動保持の状態。Since は Active のときだけ有効。
type HoldStatus struct {
	Active bool
	Since  time.Time
}
