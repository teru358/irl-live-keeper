package config

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"strings"
)

// SceneFallback は逃がし先の1行。Scene（元のシーン）が使えないとき Fallback へ逃がす。
// 名前は OBS の実名のまま持ち、前後の空白も trim しない。
type SceneFallback struct {
	Scene    string `yaml:"scene"`
	Fallback string `yaml:"fallback"`
}

// validateSceneFallbacks は obs.scene_fallbacks を検証する（OBS への接続は不要）。
// 行番号は人向けに 1 始まり、項目名は obs.scene_fallbacks[i].scene / .fallback。
func (c *Config) validateSceneFallbacks(add func(field string, reason i18n.Msg)) {
	rows := c.OBS.SceneFallbacks
	field := func(i int, item string) string {
		return fmt.Sprintf("obs.scene_fallbacks[%d].%s", i, item)
	}

	// first[scene] = 輪の判定に使う、その scene の最初の有効な行
	first := map[string]int{}
	for i, r := range rows {
		if r.Scene == "" {
			add(field(i, "scene"), i18n.New("config.validation_28"))
		}
		if r.Fallback == "" {
			add(field(i, "fallback"), i18n.New("config.validation_29"))
		}
		if r.Scene == "" {
			continue
		}
		if r.Scene == c.OBS.StandbyScene {
			add(field(i, "scene"), i18n.New("config.validation_30", "a", fmt.Sprintf("%d", i+1), "b", fmt.Sprintf("%q", r.Scene)))
			continue
		}
		if j, dup := first[r.Scene]; dup {
			add(field(i, "scene"), i18n.New("config.validation_31", "a", fmt.Sprintf("%d", i+1), "b", fmt.Sprintf("%q", r.Scene), "c", fmt.Sprintf("%d", j+1)))
			continue
		}
		first[r.Scene] = i
		if r.Fallback == r.Scene {
			add(field(i, "fallback"), i18n.New("config.validation_32", "a", fmt.Sprintf("%d", i+1), "b", fmt.Sprintf("%s", r.Scene), "c", fmt.Sprintf("%s", r.Fallback)))
		}
	}

	// 輪（長さ2以上）。各 scene の出る辺は最初の有効な行の1本だけなので、たどって見つける。
	const (
		unseen = iota
		walking
		done
	)
	state := map[string]int{}
	for start := range rows {
		s := rows[start].Scene
		if i, ok := first[s]; !ok || i != start || s == rows[start].Fallback {
			continue
		}
		var path []string
		cur := s
		for {
			i, ok := first[cur]
			if !ok || state[cur] == done {
				break
			}
			if state[cur] == walking {
				// path 内の cur から後ろが輪
				k := 0
				for path[k] != cur {
					k++
				}
				reportCycle(path[k:], first, add, field)
				break
			}
			state[cur] = walking
			path = append(path, cur)
			next := rows[i].Fallback
			if next == cur {
				break
			}
			cur = next
		}
		for _, p := range path {
			state[p] = done
		}
	}
}

// reportCycle は輪を1回だけ報告する。箇所は輪のなかで最も上の行の fallback。
func reportCycle(cycle []string, first map[string]int, add func(field string, reason i18n.Msg), field func(int, string) string) {
	if len(cycle) < 2 {
		return
	}
	top := 0
	for k, s := range cycle {
		if first[s] < first[cycle[top]] {
			top = k
		}
	}
	rot := append(append([]string{}, cycle[top:]...), cycle[:top]...)
	rot = append(rot, rot[0])
	row := first[rot[0]]
	add(field(row, "fallback"), i18n.New("config.validation_33", "a", fmt.Sprintf("%d", row+1), "b", fmt.Sprintf("%s", strings.Join(rot, "→"))))
}

// StandbyRenameWarnings は、保存のときに旧設定 prev と、保存に成功した新設定 next を比べて、
// 待機シーン（obs.standby_scene）を変えたのに、前の待機シーンの名前が next の scene_fallbacks の
// scene または fallback に残っていれば、該当の行と項目ごとの警告の文を返す。
// 保存の API の成功応答と保存画面のための警告で、起動時や status には出さない（前の設定を知れる保存時だけ）。
// prev か next が nil、待機シーンの名前が同じなら nil。
func StandbyRenameWarnings(prev, next *Config) []string {
	if prev == nil || next == nil || prev.OBS.StandbyScene == next.OBS.StandbyScene {
		return nil
	}
	old := prev.OBS.StandbyScene
	var ws []string
	for i, r := range next.OBS.SceneFallbacks {
		for _, it := range []struct{ item, v string }{{"scene", r.Scene}, {"fallback", r.Fallback}} {
			if it.v == old {
				ws = append(ws, fmt.Sprintf("obs.scene_fallbacks[%d].%s（%d 行目）に、前の待機シーンの名前 %q が残っています。この名前は普通のシーンとして残り、新しい待機シーンにはなりません", i, it.item, i+1, old))
			}
		}
	}
	return ws
}
