package switcher

import (
	"time"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
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

// CamerasChanged は全カメラの受信状態の組（Judge 済みの OK / LOW / DOWN）。差分ではなく全カメラの組を持つ。
// 設定にない id は無視し、足りない id は DOWN に整えてから、今の組と比べる。通常の core は設定済みの全 id を渡す。
type CamerasChanged struct {
	At     time.Time
	States map[string]scenemap.CameraState
}

// OBSConnected は OBS への接続。接続した時点の配信状態、program のシーン、
// 全体のトランジションの長さ（固定長で取れなければ nil）を持つ。
type OBSConnected struct {
	At           time.Time
	Streaming    bool
	Scene        string
	TransitionMs *int
	// SceneUUID は program のシーンの UUID。古い OBS（obs-websocket 5.4.0 未満）では名前を入れる。
	SceneUUID string
	// Map は最初のシーンの地図。読み終えてから流す。流したあとは変更しない。
	Map *scenemap.Map
}

// OBSDisconnected は OBS との切断。
type OBSDisconnected struct {
	At time.Time
}

// SceneChanged は program のシーンの変更。
type SceneChanged struct {
	At    time.Time
	Scene string
	// SceneUUID は切り替わったシーンの UUID。古い OBS（obs-websocket 5.4.0 未満）では名前を入れる。
	SceneUUID string
}

// SceneMapChanged はシーンの地図の読み直しの結果（obs は補助の接続で読む）。Map は流したあと変更しない。
// 今の地図にある項目の表示・非表示は、読み取りの時刻の値で、本来の接続のイベントより古いことがある。
// core が、今の地図にすでにある項目の Enabled を今の値で引き継いでから Machine に渡す（Machine は受け取った地図をそのまま使う。カメラの spec 第3節、M-7 の表 5.2）。
type SceneMapChanged struct {
	At  time.Time
	Map *scenemap.Map
}

// ItemEnabledChanged はシーン項目の表示・非表示の変化（SceneItemEnableStateChanged）。
// グループの中の項目は、グループの UUID で届く。古い OBS では SceneUUID に名前を入れる。
type ItemEnabledChanged struct {
	At        time.Time
	SceneUUID string
	ItemID    int
	Enabled   bool
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

// CameraConfig はカメラ1台の設定。OnBad は回線が悪いときの動き（0a では hide も standby と同じに扱う）。
type CameraConfig struct {
	ID    string
	OnBad scenemap.Motion
}

// ScenesConfigured はシーンとカメラの設定。Standby と StartScene は OBS のシーン名で、現在の地図から UUID を引く。
// StartScene は配信開始時の切替が無効なら nil。Cameras は設定の順（同じ悪さのときの代表の選択に使う）。
// 検証は送る側が済ませる。
type ScenesConfigured struct {
	At          time.Time
	Standby     string
	Cameras     []CameraConfig
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

func (e CamerasChanged) eventAt() time.Time     { return e.At }
func (e OBSConnected) eventAt() time.Time       { return e.At }
func (e OBSDisconnected) eventAt() time.Time    { return e.At }
func (e SceneChanged) eventAt() time.Time       { return e.At }
func (e TransitionChanged) eventAt() time.Time  { return e.At }
func (e SceneMapChanged) eventAt() time.Time    { return e.At }
func (e ItemEnabledChanged) eventAt() time.Time { return e.At }
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

// SetScene は program のシーンを切り替える。SceneUUID は切替先の UUID（バッチを作った時点で固定する。
// 古い OBS では名前が入る）。
type SetScene struct {
	SceneUUID string
}

// RestartMedia は Inputs の入力（UUID）に順に RESTART を送り、最後に1回だけ Wait 待ってから次の Action に進む。
// Inputs は作った時点で固定する。
type RestartMedia struct {
	Inputs []string
	Wait   time.Duration
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
// 待機シーンへ切り替えるバッチ（AutoFail、StandbyOn）は、戻り先のシーンの UUID ReturnScene も持つ。
// AutoFail は、作ったときの悪い代表のカメラ BadCamera も固定して持つ（ログの kbps と camera に使う）。
type Batch struct {
	BatchID     int
	Purpose     Purpose
	SceneEpoch  int
	ConnEpoch   int
	StreamEpoch int
	ConfigEpoch int
	ReturnScene string
	BadCamera   string
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
	// ReasonStandbyNotFound は待機シーンの設定名が OBS の地図で引けないこと。
	ReasonStandbyNotFound = "待機シーンが OBS にない"
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
	Scene        string // 切替先の UUID
	Purpose      Purpose
	ReturnScene  string // 戻り先の UUID。AutoFail、StandbyOn だけ。ほかは空
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

// state は Machine の状態。シーンとカメラの設定（Standby から ConfigEpoch まで）も含む。
// ConfigEpoch が 0 の間は、設定をまだ受けていない。
// 分類と健康は保存せず、Map と CameraStates から必要なときに計算する。
type state struct {
	Standby     string // 設定の名前
	Cameras     []CameraConfig
	AutoEnabled bool
	StartScene  *string
	ConfigEpoch int

	// StandbyUUID と StartSceneUUID は、設定の名前を今の地図で引いた UUID。引けなければ空。
	StandbyUUID    string
	StartSceneUUID string
	// Map は今の地図（所有する複製）。未接続なら nil。
	Map *scenemap.Map
	// CameraStates は設定の全カメラの id → 状態。接続の有無によらず保つ。
	CameraStates map[string]scenemap.CameraState

	Connected          bool
	Streaming          bool
	Scene              string // program の UUID
	PrevConnScene      string
	PrevConnStreaming  bool
	PrevConnCollection string
	StandbyOwner       Owner
	ReturnScene        string // 戻り先の UUID
	Expectations       []Expectation
	Pending            map[int]PendingBatch
	LastAutoSent       map[Purpose]time.Time
	SceneEpoch         int
	ConnEpoch          int
	StreamEpoch        int
	TransitionMs       *int
	LastError          *ErrorInfo
	NextBatchID        int
	NextReqID          int
}

// Snapshot は status に出す読み取り専用の状態。シーンと戻り先は UUID（名前は SceneName などに持つ）。
type Snapshot struct {
	Connected bool
	Streaming bool
	Scene     string
	// SceneName は現在のシーンの今の地図での名前（地図になければ空）。
	SceneName   string
	Class       SceneClass
	Standby     StandbyStatus
	AutoEnabled bool
	// Health は現在のシーンの健康。HasHealth が false（監視対象でない）なら意味を持たない。
	Health    scenemap.CameraState
	HasHealth bool
	// VisibleCameras は現在のシーンに映るカメラの id（辞書順）。地図がなければ空。
	VisibleCameras []string
	// BadCamera は現在のシーンの悪い代表のカメラ（なければ空）。
	BadCamera string
	LastError *ErrorInfo
	// SceneEpoch は人の切替（本体の期待に当たらないシーンの変化）とみなした回数。
	// 増えたことで、core は人の切替があったと知る。
	SceneEpoch int
}

// StandbyStatus は待機の持ち主と戻り先。戻り先がなければ ReturnScene は空。
type StandbyStatus struct {
	Owner       Owner
	ReturnScene string
	// ReturnSceneName は戻り先の今の地図での名前（地図になければ空）。
	ReturnSceneName string
}
