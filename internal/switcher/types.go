package switcher

import (
	"slices"
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
	// 0b: 項目の表示・非表示。承認のたびに有効条件を確かめる。
	PurposeHideCamera
	PurposeShowCamera
	PurposeRestoreAll
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
	case PurposeHideCamera:
		return "HideCamera"
	case PurposeShowCamera:
		return "ShowCamera"
	case PurposeRestoreAll:
		return "RestoreAll"
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
	return p != PurposeStartStream && p != PurposeStopStream && !p.isItem()
}

// isItem は項目の表示・非表示のバッチ（Hide / Show / RestoreAll）かを返す。
func (p Purpose) isItem() bool {
	return p == PurposeHideCamera || p == PurposeShowCamera || p == PurposeRestoreAll
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
	// StudioMode は接続した時点の Studio Mode（GetStudioModeEnabled）。読めなければ OBSConnected を流さず、接続の失敗としてつなぎ直す。
	StudioMode bool
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
	// StudioModeHide は Studio Mode のとき on_bad=hide のカメラをどう扱うか（standby / ignore / hide）。空は standby。
	StudioModeHide string
	// SceneFallbacks は 0c のシーンごとの逃がし先（名前のまま）。空なら共通の待機シーンだけ。
	// 行の並べ替えだけは同値（scene → fallback の対応として比べる）。
	SceneFallbacks []FallbackRow
}

// Studio Mode のときの hide のカメラの扱い（obs.studio_mode_hide）。
const (
	StudioHideStandby = "standby"
	StudioHideIgnore  = "ignore"
	StudioHideHide    = "hide"
)

// CommandKind は手動操作の種類。
type CommandKind int

const (
	CmdStreamStart CommandKind = iota + 1
	CmdStreamStop
	CmdStandbyOn
	CmdStandbyOff
	// CmdShowAllCameras は手動の「すべて表示に戻す」。
	CmdShowAllCameras
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
	case CmdShowAllCameras:
		return "ShowAllCameras"
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

// StudioModeChanged は OBS の Studio Mode のオン・オフの変化（StudioModeStateChanged）。
// 接続した時点の値は OBSConnected.StudioMode にある。
type StudioModeChanged struct {
	At      time.Time
	Enabled bool
}

// ItemResult は送った SetItemEnabled の応答の結果。
type ItemResult int

const (
	// ItemOK は OBS が成功を返した。
	ItemOK ItemResult = iota + 1
	// ItemTimeout は応答待ちの超過（ID の合わない応答を含む）。適用されたかわからない。
	ItemTimeout
	// ItemError は OBS が失敗を返した（NotFound 以外）、または送れなかった。
	ItemError
	// ItemNotFound は OBS が状態コード 600（ResourceNotFound）を返した。項目かシーンがもうない。
	ItemNotFound
)

func (r ItemResult) String() string {
	switch r {
	case ItemOK:
		return "OK"
	case ItemTimeout:
		return "Timeout"
	case ItemError:
		return "Error"
	case ItemNotFound:
		return "NotFound"
	}
	return "ItemResult?"
}

// ItemRequestDone は送った SetItemEnabled の応答。ReqID の型と、番号を誰が振るかは core の実行役（M-12）で決める。
type ItemRequestDone struct {
	At     time.Time
	ReqID  uint64
	Result ItemResult
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

// HiddenState は隠した項目の記録の状態。
type HiddenState int

const (
	// HiddenP は隠しかけ（記録を先に書いた。false を送る前か、送ったが結果がまだ）。
	HiddenP HiddenState = iota + 1
	// HiddenH は隠した（false が本来の接続のイベントで確かめられた）。
	HiddenH
	// HiddenU は未決着（false の応答が Timeout など、適用されたかわからない）。
	HiddenU
)

func (h HiddenState) String() string {
	switch h {
	case HiddenP:
		return "P"
	case HiddenH:
		return "H"
	case HiddenU:
		return "U"
	}
	return "HiddenState?"
}

// HiddenEntry は本体が隠した項目の記録。接続先とシーンコレクションと項目の鍵で識別する（資格情報は含めない）。
type HiddenEntry struct {
	CameraID      string
	Host          string
	Port          int
	Collection    string
	ContainerUUID string
	ItemID        int
	SourceUUID    string
	State         HiddenState
	// SettledAt は応答が決着した時刻。未応答なら nil。
	SettledAt *time.Time
	// WrittenAt は記録を作った（ディスクへ書いた）時刻。
	WrittenAt time.Time
}

// Key は項目の鍵。
func (e HiddenEntry) Key() scenemap.ItemKey {
	return scenemap.ItemKey{ContainerUUID: e.ContainerUUID, ItemID: e.ItemID}
}

func (e HiddenEntry) clone() HiddenEntry {
	if e.SettledAt != nil {
		t := *e.SettledAt
		e.SettledAt = &t
	}
	return e
}

// HiddenRestored は起動時に hidden.json から読んだ記録。設定の後、最初の接続より前に1回だけ渡す。
// Entries は Machine が複製する。
type HiddenRestored struct {
	At      time.Time
	Entries []HiddenEntry
}

// HiddenWriteFailed は最初の false の保存（Proceed.MustPersist）に失敗したこと。BatchID が作った予約と期待を巻き戻す。
type HiddenWriteFailed struct {
	At      time.Time
	BatchID int
}

// Stopping は本体の停止の知らせ。世代ごとに高々1回の意味で、2回目以降は無作用。
type Stopping struct {
	At time.Time
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
func (e StudioModeChanged) eventAt() time.Time  { return e.At }
func (e ItemRequestDone) eventAt() time.Time    { return e.At }
func (e ScenesConfigured) eventAt() time.Time   { return e.At }
func (e Command) eventAt() time.Time            { return e.At }
func (e SceneRequestDone) eventAt() time.Time   { return e.At }
func (e BatchDone) eventAt() time.Time          { return e.At }
func (e Tick) eventAt() time.Time               { return e.At }
func (e HiddenRestored) eventAt() time.Time     { return e.At }
func (e HiddenWriteFailed) eventAt() time.Time  { return e.At }
func (e Stopping) eventAt() time.Time           { return e.At }

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

// SetItemEnabled はシーン項目の表示・非表示を切り替える（0b）。SceneUUID はシーンかグループの UUID
// （古い OBS では名前）。グループの中の項目は、グループの UUID で送る。今は Machine は出さない。
type SetItemEnabled struct {
	SceneUUID string
	ItemID    int
	Enabled   bool
}

// StartStream は配信を始める。
type StartStream struct{}

// StopStream は配信を止める。
type StopStream struct{}

func (SetScene) isAction()       {}
func (RestartMedia) isAction()   {}
func (SetItemEnabled) isAction() {}
func (StartStream) isAction()    {}
func (StopStream) isAction()     {}

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
	// CameraID は Hide / Show のバッチの対象のカメラ。ほかは空。
	CameraID string
	// Items は項目のバッチ（Hide / Show / RestoreAll）が作った時点で固定した対象。SetItemEnabled の Action はこの中の1つに当たる。
	Items []BatchItem
	// RestoreScope は RestoreAll の範囲。ほかは RestoreNone。
	RestoreScope RestoreScope
	// FromCommand は手動の Command 由来のバッチか（手動の「すべて表示に戻す」）。
	FromCommand bool
	// Plan は届いたときにスタックと持ち主へ当てる変更（0c）。生成の時点で固定し、期待どおりの SceneChanged に1回だけ当てる。
	Plan ArrivalPlan
}

// ArrivalPlan は、切替が期待どおりに届いたときに当てる変更（表 0c 1.1）。Batch、PendingBatch、Expectation が持ち、
// 承認で Expectation に写す（後から Batch 側を変えても届かない）。
//   - AutoFail: Plan.Kind=PlanEscape、Plan.Append を既存のスタックに積む。
//   - AutoRecover: Plan.Kind=PlanReturn、Plan.Target 自身と上を捨てる。
//
// StandbyOn と手動の OFF の到着後の持ち主は、Purpose と ReturnScene（Expectation / PendingBatch / Batch）で決まる。
type ArrivalPlan struct {
	Plan Plan
	// EffectsDisabled は照合だけして作用（持ち主・スタック・戻り先）を当てない印。
	EffectsDisabled bool
}

// samePlan は2つの計画の意味が同じか（承認で最新の探索結果と比べる）。EffectsDisabled は比べない。
func samePlan(a, b ArrivalPlan) bool {
	return a.Plan.Kind == b.Plan.Kind && a.Plan.Target == b.Plan.Target &&
		slices.Equal(a.Plan.Append, b.Plan.Append)
}

// BatchItem は項目のバッチの対象の項目と、固定した入力の UUID。
type BatchItem struct {
	SceneUUID  string
	ItemID     int
	SourceUUID string
}

// RestoreScope は RestoreAll の範囲。
type RestoreScope int

const (
	RestoreNone RestoreScope = iota
	// RestoreTrackedItems は停止用。現在の接続先の一覧の項目だけ。
	RestoreTrackedItems
	// RestoreAllCameraItems は手動の「すべて表示に戻す」。非表示のカメラの項目と一覧と false の期待の項目。
	RestoreAllCameraItems
)

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
	// ItemReqID は SetItemEnabled の要求の番号。Machine が振る（項目用の別の領域で、世代内では再利用しない）。
	// 実行役は結果の ItemRequestDone にそのまま載せる。SetItemEnabled 以外では 0。
	ItemReqID uint64
	// MustPersist は、この承認の Proceed を返す前に、隠した項目の一覧を同期で保存しなければならないこと
	// （最初の false の承認。保存に失敗したら HiddenWriteFailed を渡し、実行役には Reject を返す）。
	MustPersist bool
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
	// ReasonStopping は本体の停止中で Command を受け付けないこと。
	ReasonStopping = "終了中"
)

// ErrorInfo は記録したエラー（時刻と文）。status の last_error になる。
type ErrorInfo struct {
	At  time.Time
	Msg string
}

// ---- 内部状態 ----

// Expectation は送った SetScene の期待。
type Expectation struct {
	ReqID       int
	Scene       string // 切替先の UUID
	Purpose     Purpose
	ReturnScene string // 戻り先の UUID。AutoFail（退避元）、StandbyOn だけ。ほかは空
	// Plan は届いたときに当てる変更（Proceed でバッチの Plan を写す）。
	Plan         ArrivalPlan
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
	BatchID     int
	Purpose     Purpose
	SceneEpoch  int
	ConnEpoch   int
	StreamEpoch int
	ConfigEpoch int
	ReturnScene string
	// Plan は届いたときに当てる変更（0c）。
	Plan ArrivalPlan
	// 0b: 項目のバッチ。
	CameraID string
	// Z はバッチを作った時の接続先とコレクション。巻き戻しは、この Z の記録だけに及ぶ。
	Z              zone
	Scope          RestoreScope
	FromCommand    bool
	Items          []pendItem
	Reserved       bool // 最初の false の承認で全対象を予約した
	WriteFailed    bool // 最初の false の保存に失敗した（以後の承認は Reject）
	Sent           int  // 送った要求の数
	Failures       int  // 失敗した要求の数（Error / Timeout）
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
	// Fallbacks は設定の scene_fallbacks（名前のまま）。解決結果は地図から必要なときに計算する。
	Fallbacks []FallbackRow

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
	// PrevConnStandby は切断の直前に解決していた待機の UUID（再接続の新しい地図で変化を見る。空は情報なし）。
	PrevConnStandby string
	StandbyOwner    Owner
	// ReturnScene は manual の単独の戻り先、auto なら Stack[0]（派生値。auto で none なら空）。UUID。
	ReturnScene string
	// Stack は auto の未復帰の退避履歴（下→上）。現在の退避先を含めない。auto 以外は空。プロセス内だけ（0c）。
	Stack        []string
	Expectations []Expectation
	Pending      map[int]PendingBatch
	// LastAutoSent は目的別の最後の自動切替の送出時刻（診断用。再送の判断には使わない）。
	LastAutoSent map[Purpose]time.Time
	// LastCandidateSent は候補ごとの送出時刻（0c）。同じ候補の再送は最短5秒、新しい候補は即時。
	LastCandidateSent map[candKey]time.Time
	SceneEpoch        int
	ConnEpoch         int
	StreamEpoch       int
	TransitionMs      *int
	LastError         *ErrorInfo
	NextBatchID       int
	NextReqID         int

	// ---- 0b ----

	// Z は今の接続先とシーンコレクション（地図があるとき）。
	Z zone
	// Hidden は今の Z の隠した項目の記録。Retained は別の Z や未照合の記録（全 Z を含む）。
	Hidden   []HiddenEntry
	Retained []HiddenEntry
	// Unsent は予約しただけで、まだ false を Proceed していない項目（値は予約したバッチ）。
	Unsent map[scenemap.ItemKey]int
	// ItemExpectations は送った SetItemEnabled の期待（登録順）。ItemReceipts は切断をまたぐ送信台帳。
	ItemExpectations []ItemExpectation
	ItemReceipts     map[uint64]ItemReceipt
	// MAConfirm は手動の「すべて表示に戻す」で true を送った項目。期待どおりの true イベントか読み直しの表示確認まで記録を保つ。
	MAConfirm map[scenemap.ItemKey]bool
	// UserShown は人が戻した印。ItemFailures は続けて失敗した回数。LastItemSent は Hide / Show 共通の送出時刻。
	UserShown        map[string]bool
	ItemFailures     map[string]int
	LastItemSent     map[string]time.Time
	StudioMode       bool
	StudioModeHide   string
	StoppingReceived bool
	RestoreIssued    bool
	// HiddenRestoredDone は HiddenRestored を受け付けた（または受け付けられなくなった）こと。
	HiddenRestoredDone bool
	NextItemReq        uint64
	// MAFloor は手動の「すべて表示に戻す」を受け付けた時点の次の BatchID。それより前の Hide は却下する。
	MAFloor        int
	HiddenRevision int
}

// candKey は自動切替の候補の識別（再送の5秒の歯止めの鍵）。目的、4つの世代、行き先、履歴計画の意味的な内容を含む。
// Append の順も含める。同じ UUID でも履歴計画が違えば別の要求。
type candKey struct {
	Purpose                                         Purpose
	ConnEpoch, StreamEpoch, SceneEpoch, ConfigEpoch int
	Target                                          string
	Plan                                            string
}

// zone は接続先とシーンコレクション。
type zone struct {
	Host       string
	Port       int
	Collection string
}

// ItemExpectation は送った SetItemEnabled の期待。
type ItemExpectation struct {
	Key         scenemap.ItemKey
	Value       bool
	ReqID       uint64
	BatchID     int
	Purpose     Purpose
	Scope       RestoreScope
	Responded   bool
	RespondedAt time.Time
	Result      ItemResult
	Deadline    time.Time
	// SawEvent は期待どおりのイベントを見たこと。Contra は反対の値のイベントを見たこと（無イベントの補正をしない）。
	SawEvent bool
	Contra   bool
}

// ItemReceipt は送信台帳の1件。結果を受けた後も、バッチが終わるまで保つ。
type ItemReceipt struct {
	ReqID    uint64
	Key      scenemap.ItemKey
	Value    bool
	BatchID  int
	CameraID string
	// Z と Source は送った時の接続先・コレクションと入力 UUID。結果は、この Z の記録だけに当てる。
	Z        zone
	Source   string
	Purpose  Purpose
	Scope    RestoreScope
	Result   ItemResult // 0 なら結果をまだ受けていない
	ResultAt time.Time
}

// pendItem は項目のバッチの対象1つの扱い。
type pendItem struct {
	Key     scenemap.ItemKey
	Source  string
	Value   bool
	Created bool // このバッチが予約で記録を作った
	Sent    bool
	ReqID   uint64
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
	// 0b: 隠した項目の記録の写し（外から変えても内部は変わらない）と、変更のたびに進む番号。
	HiddenEntries   []HiddenEntry
	RetainedEntries []HiddenEntry
	HiddenRevision  int
	StudioMode      bool
	// Cameras は設定の順のカメラ別の状態。
	Cameras []CameraItemStatus
}

// CameraItemStatus はカメラ別の項目の表示・非表示の状態。
type CameraItemStatus struct {
	ID        string
	UserShown bool
	Failures  int
	// Unusable は続けて3回以上失敗して、隠すことも戻すことも確かでないこと。
	Unusable bool
}

// StandbyStatus は待機の持ち主と戻り先。戻り先がなければ ReturnScene は空。
type StandbyStatus struct {
	Owner       Owner
	ReturnScene string
	// ReturnSceneName は戻り先の今の地図での名前（地図になければ空）。
	ReturnSceneName string
	// Stack は auto のスタック（UUID、下→上）。写しで、外から変えても内部は変わらない。auto 以外は空。
	Stack []string
	// StackNames は Stack の今の地図での名前（地図になければ空文字列）。
	StackNames []string
}
