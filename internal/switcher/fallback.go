package switcher

import (
	"slices"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// このファイルは、シーンごとの逃がし先（区切り 0c）の純粋な関数を持つ。Machine の状態・時計・OBS を知らない。
// Machine（T3）は、状態から入力の値を組み立ててここを呼ぶ。表 0c v0.2 の 1.2・2・3、spec 第3節が正。

// ---- 名前の解決（表 1.2、FB-NM） ----

// FallbackRow は設定の scene_fallbacks の1行（名前のまま）。
type FallbackRow struct {
	Scene    string
	Fallback string
}

// NameWarning は名前を地図で引けなかった行。Index は scene_fallbacks の 0 始まりの添字。
// Field は "scene"、"fallback"、または共通の逃がし先が引けないときの "standby"（このとき Index は -1）。
type NameWarning struct {
	Index int
	Field string
	Name  string
}

// Resolved は名前を UUID に引いた結果。
type Resolved struct {
	// Known は地図を取得済みか。false のとき何も引けず、呼び側は「画像のシーン」と推定せず逃がさない（FB-NM-01）。
	// 取得済みで一覧が空でも true。
	Known bool
	// Standby は共通の逃がし先の UUID。引けなければ空。
	Standby string
	// Next は元のシーン UUID → 逃がし先 UUID。引ける行だけ。fallback が引けない行は入れない（共通の逃がし先に置き換わる）。
	Next     map[string]string
	Warnings []NameWarning
}

// NextOf は uuid の逃がし先（列の次）。行がなければ共通の逃がし先。それも引けなければ空（そこで列は終わる）。
func (r Resolved) NextOf(uuid string) string {
	if n, ok := r.Next[uuid]; ok {
		return n
	}
	return r.Standby
}

// ResolveFallbacks は、設定の名前を今の地図で引き直す（地図が変わるたびに全行を呼ぶ）。
// 同名が複数あれば UUID の辞書順で先のもの。名前の前後の空白は trim しない。
// scene が引けない行は適用元がなく、待機シーンを元にした行は使わない（待機は終端）。設定の行そのものは変えない。
func ResolveFallbacks(m *scenemap.Map, standbyName string, rows []FallbackRow) Resolved {
	r := Resolved{Next: map[string]string{}}
	if m == nil {
		return r
	}
	r.Known = true
	byName := func(name string) string {
		found := ""
		if name == "" {
			return ""
		}
		for uuid, c := range m.Scenes {
			if c.Name == name && (found == "" || uuid < found) {
				found = uuid
			}
		}
		return found
	}
	r.Standby = byName(standbyName)
	for i, row := range rows {
		from := byName(row.Scene)
		if from == "" {
			r.Warnings = append(r.Warnings, NameWarning{Index: i, Field: "scene", Name: row.Scene})
		}
		to := byName(row.Fallback)
		if to == "" {
			r.Warnings = append(r.Warnings, NameWarning{Index: i, Field: "fallback", Name: row.Fallback})
		}
		if from == "" || to == "" || from == r.Standby {
			continue
		}
		if _, dup := r.Next[from]; !dup {
			r.Next[from] = to
		}
	}
	if r.Standby == "" {
		r.Warnings = append(r.Warnings, NameWarning{Index: -1, Field: "standby", Name: standbyName})
	}
	return r
}

// ---- 入ってよいか（表 3.1、FB-EN・FB-RW） ----

// Direction は入る方向。逃がす方向は逆向きの待ちを課さない。
type Direction int

const (
	DirEscape Direction = iota
	DirReturn
)

// SceneKind は候補の種類（地図で存在するものについて）。
type SceneKind int

const (
	KindMonitored SceneKind = iota // カメラが映る監視対象（本体が隠した記録も映ると数える）
	KindImage                      // カメラなしの管理外シーン（画像の存在は検査しない）
	KindStandby                    // 待機シーン（健康を持たない終端）
)

// EnterReason は入れない理由。表 3.1 の上の行を優先して最初の1つ。
type EnterReason string

const (
	ReasonNone           EnterReason = ""
	ReasonMissing        EnterReason = "Missing"
	ReasonBadHealth      EnterReason = "BadHealth"
	ReasonWaitingShow    EnterReason = "WaitingShow"
	ReasonNoActualCamera EnterReason = "NoActualCamera"
	ReasonWaitingHide    EnterReason = "WaitingHide"
)

// EnterFacts は CanEnter が要る値だけ。Machine の状態は渡さない。
type EnterFacts struct {
	Exists bool // 地図で存在を確認できた（地図なし・削除は false）
	Kind   SceneKind
	Health scenemap.CameraState // 監視対象だけ
	// OKHiddenRecord は「OK のカメラの非表示の記録が残っている」（本体の記録、使えないカメラを除く）。
	OKHiddenRecord bool
	// ActualZero は「実際の表示が0」（A(S) が空）。
	ActualZero bool
	// ReverseWait は逆向きの待ちの対象があるか（ReverseWait 関数）。戻る方向だけで使う。
	ReverseWait bool
}

// EnterResult は CanEnter の結果。Terminal は画像・待機（入れたらそこで列が終わる）。
type EnterResult struct {
	Allowed  bool
	Reason   EnterReason
	Terminal bool
}

// CanEnter は候補に入ってよいかを返す。逃がす・戻る・承認・OFF で共有する。
func CanEnter(f EnterFacts, dir Direction) EnterResult {
	if !f.Exists {
		return EnterResult{Reason: ReasonMissing}
	}
	switch f.Kind {
	case KindImage, KindStandby:
		return EnterResult{Allowed: true, Terminal: true}
	}
	switch {
	case f.Health != scenemap.StateOK:
		return EnterResult{Reason: ReasonBadHealth}
	case f.OKHiddenRecord:
		return EnterResult{Reason: ReasonWaitingShow}
	case f.ActualZero:
		return EnterResult{Reason: ReasonNoActualCamera}
	case dir == DirReturn && f.ReverseWait:
		return EnterResult{Reason: ReasonWaitingHide}
	}
	return EnterResult{Allowed: true}
}

// BadCamera は戻る先に映るカメラ1つの、逆向きの待ちの判定に要る値。
type BadCamera struct {
	State  scenemap.CameraState
	Motion scenemap.Motion // Studio Mode を当てた有効な動き
	Shown  bool            // 戻る先に実際に表示されている
	// HideValid は HideCamera の有効条件を満たすか。ただし「今のシーンを空にしない」ために隠せないことだけでは
	// false にしない（true のまま。FB-RW-02）。
	HideValid bool
	UserShown bool // 「人が戻した」印
	Unusable  bool // 3回の失敗で「使えない」
}

// ReverseWait は逆向きの待ちの対象があるか。実際に表示されている悪い hide のカメラが、隠れるまで待つ。
func ReverseWait(cams []BadCamera) bool {
	for _, c := range cams {
		if c.State != scenemap.StateOK && c.Motion == scenemap.MotionHide && c.Shown && c.HideValid && !c.UserShown && !c.Unusable {
			return true
		}
	}
	return false
}

// ---- 探索と計画（表 3.2・2、FB-EX） ----

// PlanKind は計画の種類。
type PlanKind int

const (
	PlanNone PlanKind = iota
	PlanEscape
	PlanReturn
)

// Plan は選んだ切替と、到着したときにスタックへ当てる変更。
// Escape は Target へ逃がし、Append を（順に）積む。Return は Target（スタックの要素）へ戻り、Target 自身と上を捨てる。
type Plan struct {
	Kind   PlanKind
	Target string
	Append []string
}

// EnterFunc は候補 uuid に、方向 dir で入れるかを返す（CanEnter を状態から組み立てた関数）。
type EnterFunc func(uuid string, dir Direction) EnterResult

// FindReturn は、スタックを下から見て、戻る方向で最初に入れる要素へ戻る計画を返す。なければ PlanNone。
// 下が OK でも準備待ち・表示0・逆向きの待ちなら入れないので、上を調べる。
func FindReturn(stack []string, enter EnterFunc) Plan {
	for _, u := range stack {
		if enter(u, DirReturn).Allowed {
			return Plan{Kind: PlanReturn, Target: u}
		}
	}
	return Plan{}
}

// EscapeInput は FindEscape の入力。
type EscapeInput struct {
	Current string
	// CurrentTerminal は現在が画像・待機（そこから先へ自動では退避しない）。
	CurrentTerminal bool
	Stack           []string
	Chain           Resolved
	Enter           EnterFunc
}

// FindEscape は、今のシーンの列（自身を含まず、行の相手から順に、なければ共通の逃がし先）を前からたどり、逃がす計画を返す。
//   - スタックの中のシーンに出会ったら、戻る方向で入れるならそこへ戻る（PlanReturn）。入れなければ飛ばして続ける（積み直さない）。
//   - スタックにない候補は、逃がす方向で入れるならそこへ（PlanEscape）。入れなければ飛ばし、存在するものは積む。
//   - 積むのは、今のシーンと飛ばした候補を、たどった順に。既存のスタックにあるものは積まない。到着先と共通の逃がし先は積まない。
//   - 訪問済みで止まる（輪でも有限）。入れる候補がなければ PlanNone。現在が終端なら PlanNone。
//
// 現在が悪いか、戻れるものがあるかの判断（手順1〜4）は呼び側。
func FindEscape(in EscapeInput) Plan {
	if in.CurrentTerminal {
		return Plan{}
	}
	visited := map[string]bool{in.Current: true}
	skipped := []string{} // スタックにない、存在する、入れなかった候補
	cur := in.Current
	for {
		next := in.Chain.NextOf(cur)
		if next == "" || visited[next] {
			return Plan{}
		}
		visited[next] = true
		cur = next
		if slices.Contains(in.Stack, next) {
			if in.Enter(next, DirReturn).Allowed {
				return Plan{Kind: PlanReturn, Target: next}
			}
			continue
		}
		r := in.Enter(next, DirEscape)
		if r.Allowed {
			app := []string{}
			for _, u := range append([]string{in.Current}, skipped...) {
				if u != next && !slices.Contains(in.Stack, u) && !slices.Contains(app, u) {
					app = append(app, u)
				}
			}
			return Plan{Kind: PlanEscape, Target: next, Append: app}
		}
		if r.Reason != ReasonMissing {
			skipped = append(skipped, next)
		}
	}
}

// ApplyPlan は、期待どおりの到着（現在 current）に1回だけ当てる、スタックの新しい値を返す（入力は書き換えない）。
//  1. 不正な重複は下側を残して除く（C-02）。
//  2. Escape: Append を順に積む。ただし既存のスタックの UUID と到着後の現在は積まない。
//     Return: Target がスタックにあれば、その位置から上（自身を含む）を捨てる。なければ切り詰めない（FB-SC-15）。
//  3. 現在がスタックにあれば、自身と上を捨てる（FB-ST-16）。
//
// 戻り値が空なら持ち主は none、残れば auto。PlanNone は整理（1 と 3）だけ。
func ApplyPlan(stack []string, p Plan, current string) []string {
	out := make([]string, 0, len(stack)+len(p.Append))
	for _, u := range stack {
		if !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	switch p.Kind {
	case PlanEscape:
		for _, u := range p.Append {
			if u != current && !slices.Contains(out, u) {
				out = append(out, u)
			}
		}
	case PlanReturn:
		if i := slices.Index(out, p.Target); i >= 0 {
			out = out[:i]
		}
	}
	if i := slices.Index(out, current); i >= 0 {
		out = out[:i]
	}
	return out
}
