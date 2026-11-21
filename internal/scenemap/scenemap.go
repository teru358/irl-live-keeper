// Package scenemap は、シーンの地図（どのシーンにどのカメラが映るか）の型と、純粋な関数を持つ。
// OBS も goobs も知らない（カメラの spec 第3節、第4節）。
package scenemap

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Map はシーンコレクション1つの地図。Scenes、Groups、Inputs の鍵は OBS の UUID。
type Map struct {
	Collection string
	Scenes     map[string]Container
	Groups     map[string]Container
	Inputs     map[string]Input
}

// Container はシーンかグループと、その項目の列（OBS の順）。
type Container struct {
	Name  string
	Items []Item
}

// Input は OBS の入力。カメラのメディアソースでなければ CameraID は空。
type Input struct {
	Name     string
	CameraID string
}

// SourceKind は項目が指すソースの種類。
type SourceKind string

const (
	InputSource SourceKind = "input"
	SceneSource SourceKind = "scene"
	GroupSource SourceKind = "group"
)

// Item はシーンかグループの項目1つ。Enabled は OBS の表示・非表示。
type Item struct {
	ID         int
	SourceUUID string
	SourceKind SourceKind
	Enabled    bool
}

// ItemKey は項目の識別（持ち主のシーンかグループの UUID と、項目の ID）。
type ItemKey struct {
	ContainerUUID string
	ItemID        int
}

// CameraState はカメラの回線の状態（健康の計算に使う。ingest に依存しないための小さな型）。
type CameraState string

const (
	StateOK   CameraState = "OK"
	StateLow  CameraState = "LOW"
	StateDown CameraState = "DOWN"
)

// Motion は回線が悪いときの動き（設定の on_bad）。
type Motion string

const (
	MotionStandby Motion = "standby"
	MotionHide    Motion = "hide"
)

// Class はシーンの分類。
type Class string

const (
	Standby   Class = "Standby"
	Monitored Class = "Monitored"
	Other     Class = "Other"
	Unknown   Class = "Unknown"
)

// MatchCamera はメディアソースの URL をカメラの id に照合する。is_local_file が false と明示されたもの
// だけを対象にし（既定は true）、スキームが rtsp、ポートが rtspPort（省略は 554）、path（前後の / を除く）が
// id と同じなら一致。host と資格情報は見ない。
func MatchCamera(rawURL string, isLocalFile *bool, rtspPort int, cameraIDs []string) (id string, ok bool) {
	if isLocalFile == nil || *isLocalFile {
		return "", false
	}
	u, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Scheme, "rtsp") || u.Opaque != "" || u.Hostname() == "" {
		return "", false
	}
	port := 554
	if p := u.Port(); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil {
			return "", false
		}
		port = parsed
	}
	if port != rtspPort {
		return "", false
	}
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return "", false
	}
	for _, cameraID := range cameraIDs {
		if path == cameraID {
			return cameraID, true
		}
	}
	return "", false
}

// CamerasIn は、sceneUUID から項目をたどって映るカメラの id を、重複なしで辞書順に返す。
// 入れ子のシーンとグループの中まで、表示の道だけをたどる（1本でも途中がすべて表示なら映る。循環はたどらない）。
// countToolHiddenAsShown が true なら「映る」: 本体が隠した項目（hiddenByTool が true）を表示として数える。
// false なら「実際に表示」: 本体が隠した項目を、地図でまだ表示でも（送ったがイベントがまだの「隠す」）非表示として数える。
// 人が非表示にした項目は、どちらでも数えない。
func CamerasIn(m Map, sceneUUID string, hiddenByTool func(ItemKey) bool, countToolHiddenAsShown bool) []string {
	found := make(map[string]struct{})
	active := make(map[string]bool)
	var walk func(uuid string, kind SourceKind)
	walk = func(uuid string, kind SourceKind) {
		pathKey := string(kind) + "\x00" + uuid
		if active[pathKey] {
			return
		}
		var container Container
		var exists bool
		switch kind {
		case SceneSource:
			container, exists = m.Scenes[uuid]
		case GroupSource:
			container, exists = m.Groups[uuid]
		}
		if !exists {
			return
		}
		active[pathKey] = true
		defer delete(active, pathKey)
		for _, item := range container.Items {
			enabled := item.Enabled
			if hiddenByTool != nil && hiddenByTool(ItemKey{ContainerUUID: uuid, ItemID: item.ID}) {
				enabled = countToolHiddenAsShown
			}
			if !enabled {
				continue
			}
			switch item.SourceKind {
			case InputSource:
				if input, ok := m.Inputs[item.SourceUUID]; ok && input.CameraID != "" {
					found[input.CameraID] = struct{}{}
				}
			case SceneSource, GroupSource:
				walk(item.SourceUUID, item.SourceKind)
			}
		}
	}
	walk(sceneUUID, SceneSource)
	ids := make([]string, 0, len(found))
	for id := range found {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Classify は地図があるときのシーンの分類。地図にないシーンは Other。地図がないときは ClassifyWithMap を使う。
func Classify(m Map, sceneUUID, standbyUUID string) Class {
	return ClassifyWithMap(true, m, sceneUUID, standbyUUID)
}

// ClassifyWithMap は、地図がない（mapKnown が false）ときに Unknown を返す。待機シーンは、カメラが映っていても Standby。
func ClassifyWithMap(mapKnown bool, m Map, sceneUUID, standbyUUID string) Class {
	if !mapKnown {
		return Unknown
	}
	if standbyUUID != "" && sceneUUID == standbyUUID {
		return Standby
	}
	if _, exists := m.Scenes[sceneUUID]; !exists {
		return Other
	}
	if len(CamerasIn(m, sceneUUID, nil, true)) > 0 {
		return Monitored
	}
	return Other
}

// Health はシーンの健康を計算する。cameras は CamerasIn の「映る」の結果。カメラがなければ hasHealth は false。
// badCamera は悪いと判断したカメラのうち、いちばん悪いもの（同じなら cameraOrder の順で先）で、ログの kbps に使う。
// Studio Mode では hide のカメラを standby として数える。unusable（表示に戻すのに3回失敗）は DOWN として数える。
func Health(cameras []string, states map[string]CameraState, motions map[string]Motion, studioMode bool, unusable map[string]bool, cameraOrder []string) (health CameraState, badCamera string, hasHealth bool) {
	if len(cameras) == 0 {
		return "", "", false
	}
	rank := make(map[string]int, len(cameraOrder))
	for i, id := range cameraOrder {
		rank[id] = i
	}
	isBad := func(id string) bool {
		motion := motions[id]
		if studioMode && motion == MotionHide {
			motion = MotionStandby
		}
		if motion == MotionStandby {
			return states[id] != StateOK
		}
		return false
	}
	bad := make([]string, 0, len(cameras))
	for _, id := range cameras {
		if isBad(id) {
			bad = append(bad, id)
		}
	}
	if len(bad) == 0 {
		allHideAndUnusable := true
		for _, id := range cameras {
			motion := motions[id]
			if studioMode && motion == MotionHide {
				motion = MotionStandby
			}
			if motion != MotionHide || (states[id] == StateOK && !unusable[id]) {
				allHideAndUnusable = false
				break
			}
		}
		if !allHideAndUnusable {
			return StateOK, "", true
		}
		bad = append(bad, cameras...)
	}
	// いちばん悪い状態を選ぶ。同じなら設定の順で先のもの。
	selected := ""
	selectedRank := int(^uint(0) >> 1)
	selectedSeverity := 0
	for _, id := range bad {
		severity := 1
		if states[id] == StateDown || unusable[id] {
			severity = 2
		}
		if states[id] == StateOK && !unusable[id] {
			severity = 0
		}
		position, exists := rank[id]
		if !exists {
			position = len(rank) + 1
		}
		if selected == "" || severity > selectedSeverity || (severity == selectedSeverity && position < selectedRank) {
			selected, selectedSeverity, selectedRank = id, severity, position
		}
	}
	if selectedSeverity >= 2 {
		return StateDown, selected, true
	}
	return StateLow, selected, true
}
