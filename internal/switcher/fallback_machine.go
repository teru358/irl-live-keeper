package switcher

import (
	"slices"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// このファイルは、fallback.go の純粋な関数に、Machine の状態から作った値を渡す部分（0c T3）。
// 「入ってよいか」の入力（EnterFacts）はここ1か所で作り、逃がす・戻る・承認・（T4 の）OFF が同じものを使う。

// enterFunc は CanEnter を今の状態から組み立てた関数。
func (m *Machine) enterFunc() EnterFunc {
	return func(uuid string, dir Direction) EnterResult {
		return CanEnter(m.enterFacts(uuid, dir), dir)
	}
}

// enterFacts は候補 uuid の EnterFacts を今の状態から作る。存在は、地図の Scenes に UUID があることで確かめる
// （classOf は存在しない UUID にも ClassOther を返すので、先に確かめる）。
func (m *Machine) enterFacts(uuid string, dir Direction) EnterFacts {
	s := &m.st
	var f EnterFacts
	if s.Map == nil {
		return f
	}
	if _, ok := s.Map.Scenes[uuid]; !ok {
		return f
	}
	f.Exists = true
	switch m.classOf(uuid) {
	case ClassStandby:
		f.Kind = KindStandby
		return f
	case ClassMonitored:
		f.Kind = KindMonitored
	default:
		f.Kind = KindImage
		return f
	}
	f.Health, _, _ = m.healthOf(uuid)
	f.OKHiddenRecord = m.okHiddenRecord(uuid)
	f.ActualZero = len(m.actualCameras(uuid, nil)) == 0
	if dir == DirReturn {
		f.ReverseWait = m.reverseWait(uuid)
	}
	return f
}

// okHiddenRecord は「OK のカメラの非表示の記録が残っている」: 候補に映るカメラのうち、OK で使えるカメラの本体の非表示の記録が、
// 地図でまだ false の項目を指している。実表示0（A(S) が空）はここに含めない（別の列 ActualZero）。
func (m *Machine) okHiddenRecord(r string) bool {
	s := &m.st
	for _, c := range m.cameraSet(r, false, nil) {
		if s.CameraStates[c] != scenemap.StateOK || m.unusable(c) {
			continue
		}
		for _, e := range s.Hidden {
			if e.CameraID != c {
				continue
			}
			if it, ok := m.findItem(e.Key()); ok && !it.Enabled {
				return true
			}
		}
	}
	return false
}

// reverseWait は戻る先 r に逆向きの待ちの対象があるか。表示の有無は候補の実際の表示の集合から、有効な動きは motionOf から作る。
// HideValid は canHide をもとにし、「今のシーンを空にしない」制限（genHides の wouldEmpty）は含めない（FB-RW-02）。
func (m *Machine) reverseWait(r string) bool {
	s := &m.st
	actual := m.actualCameras(r, nil)
	var cams []BadCamera
	for _, c := range m.cameraSet(r, false, nil) {
		cc, ok := m.cameraCfg(c)
		if !ok {
			continue
		}
		cams = append(cams, BadCamera{
			State: s.CameraStates[c], Motion: m.motionOf(cc), Shown: slices.Contains(actual, c),
			HideValid: m.canHide(c), UserShown: s.UserShown[c], Unusable: m.unusable(c),
		})
	}
	return ReverseWait(cams)
}
