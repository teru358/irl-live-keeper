package switcher

import (
	"fmt"
	"slices"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// ---- 1.2 名前の解決（FB-NM） ----

func nmMap(scenes map[string]string) *scenemap.Map {
	m := &scenemap.Map{Scenes: map[string]scenemap.Container{}}
	for uuid, name := range scenes {
		m.Scenes[uuid] = scenemap.Container{Name: name}
	}
	return m
}

func TestFBNM名前の解決(t *testing.T) {
	rows := []FallbackRow{{Scene: "シーン1", Fallback: "シーン2"}}
	type want struct {
		known    bool
		standby  string
		next     map[string]string
		warnings []NameWarning
	}
	tests := []struct {
		name  string
		m     *scenemap.Map
		rows  []FallbackRow
		want  want
		chain map[string]string // NextOf の確認（uuid → 期待）
	}{
		{"FB-NM-01 地図なしは不明", nil, rows, want{known: false, next: map[string]string{}}, map[string]string{"A": ""}},
		{"FB-NM-02 両方ある", nmMap(map[string]string{"A": "シーン1", "D": "シーン2", "S": "Standby"}), rows,
			want{known: true, standby: "S", next: map[string]string{"A": "D"}}, map[string]string{"A": "D", "D": "S"}},
		{"FB-NM-03 fallback 名がない は共通へ", nmMap(map[string]string{"A": "シーン1", "S": "Standby"}), rows,
			want{known: true, standby: "S", next: map[string]string{}, warnings: []NameWarning{{Index: 0, Field: "fallback", Name: "シーン2"}}},
			map[string]string{"A": "S"}},
		{"FB-NM-04 元の scene 名がない は適用元なし", nmMap(map[string]string{"D": "シーン2", "S": "Standby"}), rows,
			want{known: true, standby: "S", next: map[string]string{}, warnings: []NameWarning{{Index: 0, Field: "scene", Name: "シーン1"}}},
			map[string]string{"D": "S"}},
		{"FB-NM-05 復活すれば元に戻る", nmMap(map[string]string{"A": "シーン1", "D": "シーン2", "S": "Standby"}), rows,
			want{known: true, standby: "S", next: map[string]string{"A": "D"}}, map[string]string{"A": "D"}},
		{"FB-NM-06 Sなし・Dは引ける", nmMap(map[string]string{"A": "シーン1", "D": "シーン2"}), rows,
			want{known: true, standby: "", next: map[string]string{"A": "D"}, warnings: []NameWarning{{Index: -1, Field: "standby", Name: "Standby"}}},
			map[string]string{"A": "D", "D": ""}},
		{"FB-NM-07 Sなし・Dも引けない", nmMap(map[string]string{"A": "シーン1"}), rows,
			want{known: true, standby: "", next: map[string]string{}, warnings: []NameWarning{
				{Index: 0, Field: "fallback", Name: "シーン2"}, {Index: -1, Field: "standby", Name: "Standby"}}},
			map[string]string{"A": ""}},
		{"FB-NM-08 Dを改名・設定は旧名で未解決", nmMap(map[string]string{"A": "シーン1", "D": "シーン2改", "S": "Standby"}), rows,
			want{known: true, standby: "S", next: map[string]string{}, warnings: []NameWarning{{Index: 0, Field: "fallback", Name: "シーン2"}}},
			map[string]string{"A": "S", "D": "S"}},
		{"FB-NM-09 同名が別UUIDで復活は新UUID", nmMap(map[string]string{"A": "シーン1", "D2": "シーン2", "S": "Standby"}), rows,
			want{known: true, standby: "S", next: map[string]string{"A": "D2"}}, map[string]string{"A": "D2"}},
		{"FB-NM-10 行なしは今の次が S", nmMap(map[string]string{"A": "シーン1", "S": "Standby"}), nil,
			want{known: true, standby: "S", next: map[string]string{}}, map[string]string{"A": "S"}},
		{"取得済みの空の一覧は不明ではない", nmMap(nil), nil,
			want{known: true, standby: "", next: map[string]string{}, warnings: []NameWarning{{Index: -1, Field: "standby", Name: "Standby"}}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveFallbacks(tc.m, "Standby", tc.rows)
			if got.Known != tc.want.known || got.Standby != tc.want.standby {
				t.Fatalf("known/standby = %v/%q, want %v/%q", got.Known, got.Standby, tc.want.known, tc.want.standby)
			}
			if len(got.Next) != len(tc.want.next) {
				t.Fatalf("Next = %v, want %v", got.Next, tc.want.next)
			}
			for k, v := range tc.want.next {
				if got.Next[k] != v {
					t.Fatalf("Next = %v, want %v", got.Next, tc.want.next)
				}
			}
			if !slices.Equal(got.Warnings, tc.want.warnings) {
				t.Fatalf("warnings = %+v, want %+v", got.Warnings, tc.want.warnings)
			}
			for u, w := range tc.chain {
				if n := got.NextOf(u); n != w {
					t.Errorf("NextOf(%s) = %q, want %q", u, n, w)
				}
			}
		})
	}
}

func TestFBNM同名が複数あればUUIDの辞書順で先(t *testing.T) {
	m := nmMap(map[string]string{"A": "シーン1", "D9": "シーン2", "D1": "シーン2", "S": "Standby"})
	r := ResolveFallbacks(m, "Standby", []FallbackRow{{Scene: "シーン1", Fallback: "シーン2"}})
	if r.Next["A"] != "D1" {
		t.Fatalf("Next = %v", r.Next)
	}
}

func TestFBNM重複したscene行は先の行が優先(t *testing.T) {
	m := nmMap(map[string]string{"A": "シーン1", "D": "シーン2", "S": "Standby"})
	// 先の行だけ未解決: 後の行は使わず、共通の逃がし先へ
	r := ResolveFallbacks(m, "Standby", []FallbackRow{{Scene: "シーン1", Fallback: "Missing"}, {Scene: "シーン1", Fallback: "シーン2"}})
	if len(r.Next) != 0 || r.NextOf("A") != "S" {
		t.Fatalf("先の行だけ未解決: Next=%v", r.Next)
	}
	// 両方解決済み: 先の行
	m.Scenes["E"] = scenemap.Container{Name: "シーン3"}
	r = ResolveFallbacks(m, "Standby", []FallbackRow{{Scene: "シーン1", Fallback: "シーン3"}, {Scene: "シーン1", Fallback: "シーン2"}})
	if r.Next["A"] != "E" || len(r.Next) != 1 {
		t.Fatalf("両方解決: Next=%v", r.Next)
	}
}

func TestFBNM待機シーンを元にした行は使わない(t *testing.T) {
	m := nmMap(map[string]string{"A": "シーン1", "S": "Standby"})
	r := ResolveFallbacks(m, "Standby", []FallbackRow{{Scene: "Standby", Fallback: "シーン1"}})
	if len(r.Next) != 0 {
		t.Fatalf("Next = %v", r.Next)
	}
}

// ---- 3.1 CanEnter（FB-EN） ----

func TestFBEN入ってよいか(t *testing.T) {
	mon := func(h scenemap.CameraState, okRec, zero, rw bool) EnterFacts {
		return EnterFacts{Exists: true, Kind: KindMonitored, Health: h, OKHiddenRecord: okRec, ActualZero: zero, ReverseWait: rw}
	}
	tests := []struct {
		name string
		f    EnterFacts
		dir  Direction
		want EnterResult
	}{
		{"FB-EN-01 存在しない(監視)", EnterFacts{Exists: false, Kind: KindMonitored, Health: scenemap.StateOK}, DirEscape, EnterResult{false, ReasonMissing, false}},
		{"FB-EN-01 存在しない(待機)", EnterFacts{Exists: false, Kind: KindStandby}, DirReturn, EnterResult{false, ReasonMissing, false}},
		{"FB-EN-02 LOW", mon(scenemap.StateLow, false, false, false), DirEscape, EnterResult{false, ReasonBadHealth, false}},
		{"FB-EN-02 DOWN は他の条件より先", mon(scenemap.StateDown, true, true, true), DirReturn, EnterResult{false, ReasonBadHealth, false}},
		{"FB-EN-03 OK記録あり・表示あり", mon(scenemap.StateOK, true, false, false), DirEscape, EnterResult{false, ReasonWaitingShow, false}},
		{"FB-EN-03 OK記録あり・表示0 は WaitingShow が先", mon(scenemap.StateOK, true, true, true), DirReturn, EnterResult{false, ReasonWaitingShow, false}},
		{"FB-EN-04 表示0", mon(scenemap.StateOK, false, true, false), DirEscape, EnterResult{false, ReasonNoActualCamera, false}},
		{"FB-EN-05 逃がす方向は逆待を課さない", mon(scenemap.StateOK, false, false, true), DirEscape, EnterResult{true, ReasonNone, false}},
		{"FB-EN-06 戻る方向・逆待あり", mon(scenemap.StateOK, false, false, true), DirReturn, EnterResult{false, ReasonWaitingHide, false}},
		{"FB-EN-07 戻る方向・逆待なし", mon(scenemap.StateOK, false, false, false), DirReturn, EnterResult{true, ReasonNone, false}},
		{"FB-EN-08 画像は健康を見ず終端", EnterFacts{Exists: true, Kind: KindImage, Health: scenemap.StateDown, ActualZero: true, OKHiddenRecord: true, ReverseWait: true}, DirReturn, EnterResult{true, ReasonNone, true}},
		{"FB-EN-08 画像・逃がす方向", EnterFacts{Exists: true, Kind: KindImage}, DirEscape, EnterResult{true, ReasonNone, true}},
		{"FB-EN-09 待機は健康を見ず終端", EnterFacts{Exists: true, Kind: KindStandby, Health: scenemap.StateDown, ReverseWait: true}, DirReturn, EnterResult{true, ReasonNone, true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanEnter(tc.f, tc.dir); got != tc.want {
				t.Fatalf("CanEnter = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// ---- 3.1 逆向きの待ち（FB-RW） ----

func TestFBRW逆向きの待ちの対象(t *testing.T) {
	base := BadCamera{State: scenemap.StateDown, Motion: scenemap.MotionHide, Shown: true, HideValid: true}
	with := func(f func(c *BadCamera)) BadCamera { c := base; f(&c); return c }
	tests := []struct {
		name string
		cams []BadCamera
		want bool
	}{
		{"FB-RW-01 hide・実表示あり・Hide有効", []BadCamera{base}, true},
		{"FB-RW-01 LOW も対象", []BadCamera{with(func(c *BadCamera) { c.State = scenemap.StateLow })}, true},
		{"FB-RW-02 今を空にするためだけ隠せない は呼び側が HideValid=true にして対象", []BadCamera{base}, true},
		{"FB-RW-03 人が戻した印", []BadCamera{with(func(c *BadCamera) { c.UserShown = true })}, false},
		{"FB-RW-04 3回失敗", []BadCamera{with(func(c *BadCamera) { c.Unusable = true })}, false},
		{"FB-RW-05 standby", []BadCamera{with(func(c *BadCamera) { c.Motion = scenemap.MotionStandby })}, false},
		{"FB-RW-05 ignore", []BadCamera{with(func(c *BadCamera) { c.Motion = scenemap.MotionIgnore })}, false},
		{"FB-RW-06 実表示なし", []BadCamera{with(func(c *BadCamera) { c.Shown = false })}, false},
		{"FB-RW-07 OK", []BadCamera{with(func(c *BadCamera) { c.State = scenemap.StateOK })}, false},
		{"Hide の有効条件を満たさない(自動無効など)", []BadCamera{with(func(c *BadCamera) { c.HideValid = false })}, false},
		{"1つでも対象なら待つ", []BadCamera{with(func(c *BadCamera) { c.Shown = false }), base}, true},
		{"空", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReverseWait(tc.cams); got != tc.want {
				t.Fatalf("ReverseWait = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- 3.2 探索（FB-EX） ----

// world は探索の試験用の世界。scene ごとに「逃がす方向で入れるか」「戻る方向で入れるか」を与える。
type world struct {
	next   map[string]string
	std    string
	escape map[string]EnterResult
	ret    map[string]EnterResult
}

func ok() EnterResult         { return EnterResult{Allowed: true} }
func okTerm() EnterResult     { return EnterResult{Allowed: true, Terminal: true} }
func bad() EnterResult        { return EnterResult{Reason: ReasonBadHealth} }
func waitShow() EnterResult   { return EnterResult{Reason: ReasonWaitingShow} }
func waitHide() EnterResult   { return EnterResult{Reason: ReasonWaitingHide} }
func missing() EnterResult    { return EnterResult{Reason: ReasonMissing} }
func (w world) res() Resolved { return Resolved{Known: true, Standby: w.std, Next: w.next} }
func (w world) enter(u string, d Direction) EnterResult {
	if d == DirReturn {
		if r, ok := w.ret[u]; ok {
			return r
		}
	}
	if r, ok := w.escape[u]; ok {
		return r
	}
	return missing()
}

func (w world) escapeFrom(cur string, stack []string, terminal bool) Plan {
	return FindEscape(EscapeInput{Current: cur, CurrentTerminal: terminal, Stack: stack, Chain: w.res(), Enter: w.enter})
}

func TestFBEX探索(t *testing.T) {
	std := "S"
	tests := []struct {
		name  string
		w     world
		cur   string
		stack []string
		term  bool
		want  Plan
		after []string // 到着後のスタック
	}{
		{"FB-EX-01 A不良 D入れる", world{next: map[string]string{"A": "D"}, std: std,
			escape: map[string]EnterResult{"D": ok(), "S": okTerm()}}, "A", nil, false,
			Plan{Kind: PlanEscape, Target: "D", Append: []string{"A"}}, []string{"A"}},
		{"FB-EX-02 A不良 D不良 S入れる", world{next: map[string]string{"A": "D"}, std: std,
			escape: map[string]EnterResult{"D": bad(), "S": okTerm()}}, "A", nil, false,
			Plan{Kind: PlanEscape, Target: "S", Append: []string{"A", "D"}}, []string{"A", "D"}},
		{"FB-EX-03 D は健康OKだがOK記録あり/表示0 は飛ばして積む", world{next: map[string]string{"A": "D"}, std: std,
			escape: map[string]EnterResult{"D": waitShow(), "S": okTerm()}}, "A", nil, false,
			Plan{Kind: PlanEscape, Target: "S", Append: []string{"A", "D"}}, []string{"A", "D"}},
		{"FB-EX-03 D は実際の表示が0 は飛ばして積む", world{next: map[string]string{"A": "D"}, std: std,
			escape: map[string]EnterResult{"D": {Reason: ReasonNoActualCamera}, "S": okTerm()}}, "A", nil, false,
			Plan{Kind: PlanEscape, Target: "S", Append: []string{"A", "D"}}, []string{"A", "D"}},
		{"FB-EX-06 戻り待ちでも退避 B→T", world{next: map[string]string{"B": "T"}, std: std,
			escape: map[string]EnterResult{"A": ok(), "D": ok(), "T": okTerm(), "S": okTerm()},
			ret:    map[string]EnterResult{"A": waitHide(), "D": bad()}}, "B", []string{"A", "D"}, false,
			Plan{Kind: PlanEscape, Target: "T", Append: []string{"B"}}, []string{"A", "D", "B"}},
		{"FB-EX-07 履歴内 B に出会っても止めない", world{next: map[string]string{"C": "B", "B": "D"}, std: std,
			escape: map[string]EnterResult{"A": ok(), "B": ok(), "D": ok(), "S": okTerm()},
			ret:    map[string]EnterResult{"A": bad(), "B": bad()}}, "C", []string{"A", "B"}, false,
			Plan{Kind: PlanEscape, Target: "D", Append: []string{"C"}}, []string{"A", "B", "C"}},
		{"FB-EX-08 B が戻る方向で可なら復帰", world{next: map[string]string{"C": "B", "B": "D"}, std: std,
			escape: map[string]EnterResult{"A": ok(), "B": ok(), "D": ok(), "S": okTerm()},
			ret:    map[string]EnterResult{"A": bad(), "B": ok()}}, "C", []string{"A", "B"}, false,
			Plan{Kind: PlanReturn, Target: "B"}, []string{"A"}},
		{"FB-EX-09 画像は終端で先へ進まない", world{next: map[string]string{"A": "T", "T": "D"}, std: std,
			escape: map[string]EnterResult{"T": okTerm(), "D": ok(), "S": okTerm()}}, "A", nil, false,
			Plan{Kind: PlanEscape, Target: "T", Append: []string{"A"}}, []string{"A"}},
		{"FB-NM-06 Sなし A→D", world{next: map[string]string{"A": "D"}, std: "",
			escape: map[string]EnterResult{"D": ok()}}, "A", nil, false,
			Plan{Kind: PlanEscape, Target: "D", Append: []string{"A"}}, []string{"A"}},
		{"FB-NM-07 Sなし D も入れない は探索終了", world{next: map[string]string{"A": "D"}, std: "",
			escape: map[string]EnterResult{"D": bad()}}, "A", nil, false, Plan{}, nil},
		{"FB-NM-10 行なしは次が S", world{next: map[string]string{}, std: std,
			escape: map[string]EnterResult{"S": okTerm()}}, "A", nil, false,
			Plan{Kind: PlanEscape, Target: "S", Append: []string{"A"}}, []string{"A"}},
		{"手順9 現在が待機/画像なら退避しない", world{next: map[string]string{}, std: std,
			escape: map[string]EnterResult{"S": okTerm()}}, "S", []string{"A"}, true, Plan{}, nil},
		{"存在しない中間は飛ばすが積まない", world{next: map[string]string{"A": "D", "D": "E"}, std: std,
			escape: map[string]EnterResult{"E": bad(), "S": okTerm()}}, "A", nil, false,
			Plan{Kind: PlanEscape, Target: "S", Append: []string{"A", "E"}}, []string{"A", "E"}},
		{"飛ばした既存履歴は積み直さない(積む順は辿った順)", world{next: map[string]string{"X": "A", "A": "D", "D": "E"}, std: std,
			escape: map[string]EnterResult{"A": bad(), "D": bad(), "E": bad(), "S": okTerm()},
			ret:    map[string]EnterResult{"A": bad()}}, "X", []string{"A"}, false,
			Plan{Kind: PlanEscape, Target: "S", Append: []string{"X", "D", "E"}}, []string{"A", "X", "D", "E"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.w.escapeFrom(tc.cur, tc.stack, tc.term)
			if got.Kind != tc.want.Kind || got.Target != tc.want.Target || !slices.Equal(got.Append, tc.want.Append) {
				t.Fatalf("plan = %+v, want %+v", got, tc.want)
			}
			if tc.want.Kind != PlanNone {
				if st := ApplyPlan(tc.stack, got, got.Target); !slices.Equal(st, tc.after) {
					t.Fatalf("到着後 = %v, want %v", st, tc.after)
				}
			}
		})
	}
}

func TestFBEX循環する列でも止まる(t *testing.T) {
	w := world{next: map[string]string{"A": "B", "B": "A"}, std: "S",
		escape: map[string]EnterResult{"B": bad(), "S": okTerm()}}
	if p := w.escapeFrom("A", nil, false); p.Kind != PlanNone {
		t.Fatalf("A→B→A: %+v", p)
	}
	// 自己参照
	w.next = map[string]string{"A": "A"}
	if p := w.escapeFrom("A", nil, false); p.Kind != PlanNone {
		t.Fatalf("A→A: %+v", p)
	}
	// 履歴の中の輪
	w.next = map[string]string{"C": "A", "A": "B", "B": "A"}
	w.escape = map[string]EnterResult{"A": bad(), "B": bad()}
	w.ret = map[string]EnterResult{"A": bad(), "B": bad()}
	if p := w.escapeFrom("C", []string{"A", "B"}, false); p.Kind != PlanNone {
		t.Fatalf("履歴内の輪: %+v", p)
	}
}

func TestFBEX現在がスタックにあっても積まない(t *testing.T) {
	w := world{next: map[string]string{"A": "D"}, std: "S", escape: map[string]EnterResult{"D": ok()}}
	p := w.escapeFrom("A", []string{"A"}, false)
	if p.Kind != PlanEscape || len(p.Append) != 0 {
		t.Fatalf("plan = %+v", p)
	}
}

// ---- 3.2 戻る（FB-EX-04/05 ほか） ----

func TestFBEX戻る(t *testing.T) {
	tests := []struct {
		name  string
		stack []string
		ret   map[string]EnterResult
		want  Plan
		after []string
	}{
		{"FB-EX-04 A不可 D可", []string{"A", "D"}, map[string]EnterResult{"A": bad(), "D": ok()}, Plan{Kind: PlanReturn, Target: "D"}, []string{"A"}},
		{"FB-EX-05 A可 D可 は最上段優先にしない", []string{"A", "D"}, map[string]EnterResult{"A": ok(), "D": ok()}, Plan{Kind: PlanReturn, Target: "A"}, []string{}},
		{"下がOKでも逆待なら上を調べる", []string{"A", "D"}, map[string]EnterResult{"A": waitHide(), "D": ok()}, Plan{Kind: PlanReturn, Target: "D"}, []string{"A"}},
		{"下がOKでも表示待ちなら上を調べる", []string{"A", "D"}, map[string]EnterResult{"A": waitShow(), "D": ok()}, Plan{Kind: PlanReturn, Target: "D"}, []string{"A"}},
		{"入れるものがない", []string{"A", "D"}, map[string]EnterResult{"A": bad(), "D": bad()}, Plan{}, nil},
		{"空のスタック", nil, nil, Plan{}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := world{ret: tc.ret}
			got := FindReturn(tc.stack, w.enter)
			if got.Kind != tc.want.Kind || got.Target != tc.want.Target {
				t.Fatalf("plan = %+v, want %+v", got, tc.want)
			}
			if tc.want.Kind != PlanNone {
				if st := ApplyPlan(tc.stack, got, got.Target); !slices.Equal(st, tc.after) {
					t.Fatalf("到着後 = %v, want %v", st, tc.after)
				}
			}
		})
	}
}

// ---- 2 計画を当てる（FB-SC-02/14/15、FB-ST-16、C-02） ----

func TestFB計画を当てる(t *testing.T) {
	tests := []struct {
		name    string
		stack   []string
		plan    Plan
		current string
		want    []string
	}{
		{"FB-SC-01 退避を追加", nil, Plan{Kind: PlanEscape, Target: "D", Append: []string{"A"}}, "D", []string{"A"}},
		{"FB-SC-02 R(D;[A]) は D 自身を残さない", []string{"A", "D"}, Plan{Kind: PlanReturn, Target: "D"}, "D", []string{"A"}},
		{"FB-SC-03 R(A;[]) は空", []string{"A", "D"}, Plan{Kind: PlanReturn, Target: "A"}, "A", []string{}},
		{"FB-SC-14 既存 A を追加しない", []string{"A"}, Plan{Kind: PlanEscape, Target: "S", Append: []string{"A", "D"}}, "S", []string{"A", "D"}},
		{"FB-SC-14 1つ目(F(D;+[A]))", nil, Plan{Kind: PlanEscape, Target: "D", Append: []string{"A"}}, "D", []string{"A"}},
		{"FB-SC-15 ReturnUUID が K にない は切り詰めない", []string{"A"}, Plan{Kind: PlanReturn, Target: "D"}, "D", []string{"A"}},
		{"到着後の現在 UUID は積まない", nil, Plan{Kind: PlanEscape, Target: "D", Append: []string{"A", "D"}}, "D", []string{"A"}},
		{"追加の中の重複は1回", nil, Plan{Kind: PlanEscape, Target: "S", Append: []string{"A", "D", "A"}}, "S", []string{"A", "D"}},
		{"FB-ST-16 現在が混入していれば自身と上を捨てる", []string{"A", "D"}, Plan{}, "A", []string{}},
		{"FB-ST-16 中段が現在", []string{"A", "B", "D"}, Plan{}, "B", []string{"A"}},
		{"C-02 不正な重複は下側を残す", []string{"A", "D", "A"}, Plan{}, "S", []string{"A", "D"}},
		{"スタックにある要素は ReturnUUID の位置から上を捨てる", []string{"A", "B", "D"}, Plan{Kind: PlanReturn, Target: "B"}, "B", []string{"A"}},
		{"戻る・計画なし(None)は整理だけ", []string{"A", "D"}, Plan{}, "S", []string{"A", "D"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := slices.Clone(tc.stack)
			got := ApplyPlan(tc.stack, tc.plan, tc.current)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("stack = %v, want %v", got, tc.want)
			}
			if !slices.Equal(tc.stack, orig) {
				t.Fatalf("入力のスタックを書き換えた: %v → %v", orig, tc.stack)
			}
		})
	}
}

func TestFB計画を当てる_SC14の2回の到着(t *testing.T) {
	// F(D;+[A]) と F(S;+[A,D]) の期待。SC(D) のあと SC(S)
	st := ApplyPlan(nil, Plan{Kind: PlanEscape, Target: "D", Append: []string{"A"}}, "D")
	if !slices.Equal(st, []string{"A"}) {
		t.Fatal(st)
	}
	st = ApplyPlan(st, Plan{Kind: PlanEscape, Target: "S", Append: []string{"A", "D"}}, "S")
	if !slices.Equal(st, []string{"A", "D"}) {
		t.Fatal(st)
	}
}

// ---- spec 第3節の例（スタックの変化）を関数の組み合わせで追う ----

// specWorld は シーン1(A)=メイン+サブ、シーン2(D)=サブの全画面、S=待機。
// 健康と「戻る方向の待ち」を、メイン・サブの状態から組み立てる。
type specWorld struct {
	mainOK, subOK bool
	subHidden     bool // シーン1 のサブの項目が隠れたか（戻る方向の逆待の解消）
}

func (s specWorld) enter(u string, d Direction) EnterResult {
	switch u {
	case "S":
		return EnterResult{Allowed: true, Terminal: true}
	case "A":
		if !s.mainOK {
			return EnterResult{Reason: ReasonBadHealth}
		}
		// メインOKでサブが悪いとき、シーン1の悪いサブは hide → 隠れるまで待つ（戻る方向）
		if !s.subOK && d == DirReturn {
			if !s.subHidden {
				return EnterResult{Reason: ReasonWaitingHide}
			}
			return EnterResult{Allowed: true}
		}
		return EnterResult{Allowed: true}
	case "D":
		if !s.subOK {
			return EnterResult{Reason: ReasonBadHealth}
		}
		return EnterResult{Allowed: true}
	}
	return EnterResult{Reason: ReasonMissing}
}

// step は今の状況で 戻る→(なければ)逃がす の順に判断し、到着まで進める（現在が悪い監視対象のときだけ逃がす）。
func (s specWorld) step(cur string, stack []string) (string, []string) {
	chain := Resolved{Known: true, Standby: "S", Next: map[string]string{"A": "D"}}
	if p := FindReturn(stack, s.enter); p.Kind == PlanReturn {
		return p.Target, ApplyPlan(stack, p, p.Target)
	}
	curBad := cur != "S" && !s.enter(cur, DirEscape).Allowed
	if !curBad {
		return cur, stack
	}
	p := FindEscape(EscapeInput{Current: cur, CurrentTerminal: cur == "S", Stack: stack, Chain: chain, Enter: s.enter})
	if p.Kind == PlanNone {
		return cur, stack
	}
	return p.Target, ApplyPlan(stack, p, p.Target)
}

func TestFBSpec第3節の例(t *testing.T) {
	type row struct {
		name  string
		w     specWorld
		cur   string
		stack []string
	}
	cur, stack := "A", []string(nil)
	steps := []row{
		{"シーン1でメインが悪くなる", specWorld{mainOK: false, subOK: true}, "D", []string{"A"}},
		{"サブも悪くなる", specWorld{mainOK: false, subOK: false}, "S", []string{"A", "D"}},
		{"サブが良くなる(メインはまだ悪い)", specWorld{mainOK: false, subOK: true}, "D", []string{"A"}},
		{"メインが良くなる", specWorld{mainOK: true, subOK: true}, "A", []string{}},
	}
	for _, r := range steps {
		cur, stack = r.w.step(cur, stack)
		if cur != r.cur || !slices.Equal(stack, r.stack) {
			t.Fatalf("%s: 今=%s スタック=%v, want %s %v", r.name, cur, stack, r.cur, r.stack)
		}
	}
	// 別の列: 待機にいる間に、メインとサブが同時に良くなる → A へ直接
	cur, stack = specWorld{mainOK: true, subOK: true}.step("S", []string{"A", "D"})
	if cur != "A" || len(stack) != 0 {
		t.Fatalf("同時回復: %s %v", cur, stack)
	}
	// 別の列: シーン1でメインとサブが同時に悪くなる → 待機。D は飛ばして積む
	cur, stack = specWorld{}.step("A", nil)
	if cur != "S" || !slices.Equal(stack, []string{"A", "D"}) {
		t.Fatalf("同時悪化: %s %v", cur, stack)
	}
	// 別の列: 待機にいて [A,D]、メインだけ良い。サブは悪い。隠れるまで戻らない
	w := specWorld{mainOK: true, subOK: false}
	cur, stack = w.step("S", []string{"A", "D"})
	if cur != "S" || !slices.Equal(stack, []string{"A", "D"}) {
		t.Fatalf("逆待: %s %v", cur, stack)
	}
	w.subHidden = true
	cur, stack = w.step("S", []string{"A", "D"})
	if cur != "A" || len(stack) != 0 {
		t.Fatalf("隠れたあと: %s %v", cur, stack)
	}
}

func TestFBSpec設定変更で履歴と列が重なる例(t *testing.T) {
	// スタック [A,B]、今 C が悪い。C→B→D→待機。A と B は戻る方向で入れず、D は入れる。
	w := world{next: map[string]string{"C": "B", "B": "D"}, std: "S",
		escape: map[string]EnterResult{"B": ok(), "D": ok(), "S": okTerm()},
		ret:    map[string]EnterResult{"A": bad(), "B": bad()}}
	p := w.escapeFrom("C", []string{"A", "B"}, false)
	if p.Kind != PlanEscape || p.Target != "D" || fmt.Sprint(p.Append) != "[C]" {
		t.Fatalf("plan = %+v", p)
	}
	if st := ApplyPlan([]string{"A", "B"}, p, "D"); !slices.Equal(st, []string{"A", "B", "C"}) {
		t.Fatalf("到着後 %v", st)
	}
	// B に戻れる別の場合は復帰優先で B へ、到着後は [A]
	w.ret["B"] = ok()
	p = FindReturn([]string{"A", "B"}, w.enter)
	if p.Kind != PlanReturn || p.Target != "B" {
		t.Fatalf("plan = %+v", p)
	}
	if st := ApplyPlan([]string{"A", "B"}, p, "B"); !slices.Equal(st, []string{"A"}) {
		t.Fatalf("到着後 %v", st)
	}
}
