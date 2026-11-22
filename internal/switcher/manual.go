package switcher

// このファイルは手動の待機（0c T4）の、ON の戻り先と OFF 専用の探索を持つ。通常の auto の履歴探索（decide）とは別。

// onReturnTarget は待機の ON が保存する戻り先。auto ならスタックの最下段、none なら今のシーン。manual は持たない（空）。
func (m *Machine) onReturnTarget() string {
	s := &m.st
	switch s.StandbyOwner {
	case OwnerAuto:
		if len(s.Stack) > 0 {
			return s.Stack[0]
		}
	case OwnerNone:
		return s.Scene
	}
	return ""
}

// offPlanResult は手動の OFF の専用の計画（Handle で作って固定し、承認で最新の状態から計算し直して比べる）。
type offPlanResult struct {
	Purpose Purpose
	Target  string
	Actions []Action
	Plan    ArrivalPlan
	// Immediate は候補が今のシーン（待機）で、切替を送らずその場で auto にすること。
	Immediate bool
}

// offPlan は manual の戻り先そのもの（戻る方向で検査）から始め、その逃がし先の列（逃がす方向で検査）を調べて、最初に入れる候補を選ぶ。
//   - 戻り先が管理外（監視対象でない）なら、そのまま StandbyOffOther（再起動なし、到着で解除）。
//   - auto_enabled が false なら、健康・表示待ち・実表示0・逆向きの待ちを見ず戻り先そのものへ（存在だけ要る）。
//   - 戻り先へ入れれば再起動つきの復帰（到着で解除）。入れなければ列の次から。到着で auto（戻り先と飛ばした監視対象）。
//
// reason が空でなければ受け付けない。名前がどこにも引けないときは ReasonOffNoTarget（C-01）。
func (m *Machine) offPlan() (offPlanResult, string) {
	s := &m.st
	origin := s.ReturnScene
	if origin == "" || (!mapIsEmpty(s.Map) && !m.sceneExists(origin)) {
		return offPlanResult{}, ReasonNotManual // 戻り先の削除は整理で全解除済み。存在しない元へ強制しない
	}
	if !m.isMonitored(origin) {
		return offPlanResult{Purpose: PurposeStandbyOffOther, Target: origin, Actions: []Action{SetScene{SceneUUID: origin}}}, ""
	}
	back := func() (offPlanResult, string) {
		return offPlanResult{Purpose: PurposeStandbyOffMonitored, Target: origin, Actions: m.restoreActions(origin),
			Plan: ArrivalPlan{}}, "" // 戻り先そのものへ届けば解除（計画なし）
	}
	enter := m.enterFunc()
	if !s.AutoEnabled || enter(origin, DirReturn).Allowed {
		return back()
	}
	pl := FindEscape(EscapeInput{Current: origin, Chain: m.resolved(), Enter: enter})
	if pl.Kind != PlanEscape {
		return offPlanResult{}, ReasonOffNoTarget
	}
	return offPlanResult{Purpose: PurposeStandbyOffMonitored, Target: pl.Target, Actions: []Action{SetScene{SceneUUID: pl.Target}},
		Plan: ArrivalPlan{Plan: pl}, Immediate: pl.Target == s.Scene}, ""
}

// sceneExists は地図にシーン uuid があるか。
func (m *Machine) sceneExists(uuid string) bool {
	if m.st.Map == nil {
		return false
	}
	_, ok := m.st.Map.Scenes[uuid]
	return ok
}
