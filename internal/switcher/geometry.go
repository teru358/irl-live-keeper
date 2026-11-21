package switcher

import (
	"reflect"
	"slices"
	"sort"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// この Machine は地図を自分で所有する。Event の map と slice は取り込むときに複製し、あとから外で変えられても
// 状態は変わらない。

// copyMap は地図の複製を返す（nil は nil）。辞書は空でも nil にしない。
func copyMap(m *scenemap.Map) *scenemap.Map {
	if m == nil {
		return nil
	}
	c := &scenemap.Map{
		Collection: m.Collection,
		Scenes:     make(map[string]scenemap.Container, len(m.Scenes)),
		Groups:     make(map[string]scenemap.Container, len(m.Groups)),
		Inputs:     make(map[string]scenemap.Input, len(m.Inputs)),
	}
	for k, v := range m.Scenes {
		v.Items = slices.Clone(v.Items)
		c.Scenes[k] = v
	}
	for k, v := range m.Groups {
		v.Items = slices.Clone(v.Items)
		c.Groups[k] = v
	}
	for k, v := range m.Inputs {
		c.Inputs[k] = v
	}
	return c
}

// sameMap は2つの地図が同じ内容かを返す。Collection、鍵と値、各 Item の全フィールドを比べ、
// nil と空の辞書は同じとみなす。Items の順の違いは違う地図とする。
func sameMap(a, b *scenemap.Map) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Collection != b.Collection || len(a.Inputs) != len(b.Inputs) {
		return false
	}
	return sameContainers(a.Scenes, b.Scenes) && sameContainers(a.Groups, b.Groups) && reflect.DeepEqual(
		nilIfEmpty(a.Inputs), nilIfEmpty(b.Inputs))
}

func nilIfEmpty(m map[string]scenemap.Input) map[string]scenemap.Input {
	if len(m) == 0 {
		return nil
	}
	return m
}

func sameContainers(a, b map[string]scenemap.Container) bool {
	if len(a) != len(b) {
		return false
	}
	for k, x := range a {
		y, ok := b[k]
		if !ok || x.Name != y.Name || !slices.Equal(x.Items, y.Items) {
			return false
		}
	}
	return true
}

// calcMap は、設定にないカメラ id の入力を「カメラなし」にした計算用の地図を返す。
// 分類・健康・再起動対象はすべてこの地図で計算する。地図がなければ空の地図。
func (m *Machine) calcMap() scenemap.Map {
	s := &m.st
	if s.Map == nil {
		return scenemap.Map{}
	}
	configured := make(map[string]struct{}, len(s.Cameras))
	for _, c := range s.Cameras {
		configured[c.ID] = struct{}{}
	}
	c := *s.Map
	c.Inputs = make(map[string]scenemap.Input, len(s.Map.Inputs))
	for uuid, in := range s.Map.Inputs {
		if _, ok := configured[in.CameraID]; !ok {
			in.CameraID = ""
		}
		c.Inputs[uuid] = in
	}
	return c
}

// resolve は設定のシーン名を今の地図で引いた UUID を返す。引けなければ空。
// 同じ名前が複数あれば UUID の辞書順で先のもの。
func (m *Machine) resolve(name string) string {
	s := &m.st
	if s.Map == nil || name == "" {
		return ""
	}
	found := ""
	for uuid, c := range s.Map.Scenes {
		if c.Name == name && (found == "" || uuid < found) {
			found = uuid
		}
	}
	return found
}

// sceneLabel はログや LastError に使うシーンの表示。今の地図の名前があればそれ、なければ UUID。
func (m *Machine) sceneLabel(uuid string) string {
	if m.st.Map != nil {
		if c, ok := m.st.Map.Scenes[uuid]; ok && c.Name != "" {
			return c.Name
		}
	}
	return uuid
}

// classOf はシーン uuid の分類。未接続・地図なしなら不明。
func (m *Machine) classOf(uuid string) SceneClass {
	s := &m.st
	if !s.Connected || s.Map == nil {
		return ClassUnknown
	}
	switch scenemap.ClassifyWithMap(true, m.calcMap(), uuid, s.StandbyUUID) {
	case scenemap.Standby:
		return ClassStandby
	case scenemap.Monitored:
		return ClassMonitored
	}
	return ClassOther
}

func (m *Machine) isMonitored(uuid string) bool { return m.classOf(uuid) == ClassMonitored }

// cameraIDs は設定のカメラの id を設定の順で返す。
func (m *Machine) cameraIDs() []string {
	ids := make([]string, len(m.st.Cameras))
	for i, c := range m.st.Cameras {
		ids[i] = c.ID
	}
	return ids
}

// visibleCameras はシーン uuid に映るカメラの id（辞書順）。地図がなければ nil。
func (m *Machine) visibleCameras(uuid string) []string {
	if m.st.Map == nil {
		return nil
	}
	ids := scenemap.CamerasIn(m.calcMap(), uuid, nil, true)
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// healthOf は監視対象のシーン uuid の健康と、悪い代表のカメラ。監視対象でなければ has は false
// （待機は、カメラが映っていても健康を返さない）。
// 0a では設定の hide も standby と同じに扱う。Health には設定の全カメラの鍵を入れる。
func (m *Machine) healthOf(uuid string) (health scenemap.CameraState, bad string, has bool) {
	s := &m.st
	if m.classOf(uuid) != ClassMonitored {
		return "", "", false
	}
	motions := make(map[string]scenemap.Motion, len(s.Cameras))
	for _, c := range s.Cameras {
		motions[c.ID] = scenemap.MotionStandby
	}
	return scenemap.Health(m.visibleCameras(uuid), s.CameraStates, motions, false, nil, m.cameraIDs())
}

// isBad は健康が不良（LOW か DOWN）かを返す。健康なしは不良に含めない。
func isBad(h scenemap.CameraState, has bool) bool { return has && h != scenemap.StateOK }

// restartInputs は戻り先 uuid に映るカメラの、再起動の対象の入力 UUID を返す。
// 映るカメラを設定順に並べ、同じカメラの入力は UUID の辞書順にする。重複は除く。
func (m *Machine) restartInputs(uuid string) []string {
	visible := m.visibleCameras(uuid)
	if len(visible) == 0 {
		return nil
	}
	calc := m.calcMap()
	var out []string
	for _, id := range m.cameraIDs() {
		if !slices.Contains(visible, id) {
			continue
		}
		var ins []string
		for inUUID, in := range calc.Inputs {
			if in.CameraID == id {
				ins = append(ins, inUUID)
			}
		}
		sort.Strings(ins)
		for _, in := range ins {
			if !slices.Contains(out, in) {
				out = append(out, in)
			}
		}
	}
	return out
}

// restoreActions は戻り先 target へ戻す Action の列。再起動の対象があれば RestartMedia を1つ（待ちは最後に1回）、
// そのあとに SetScene。対象が空なら RestartMedia 自体を作らない。
func (m *Machine) restoreActions(target string) []Action {
	var acts []Action
	if in := m.restartInputs(target); len(in) > 0 {
		acts = append(acts, RestartMedia{Inputs: in, Wait: m.cfg.RestartWait})
	}
	return append(acts, SetScene{SceneUUID: target})
}
