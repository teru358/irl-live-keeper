package obs

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// ErrTimeout は OBS からの応答が待ち時間（5 秒）内に返らなかったこと。
// SetScene などの失敗と区別するために使う。
var ErrTimeout = errors.New("OBS の応答がタイムアウト")

// goobs は応答待ちの超過を、番兵のエラーを持たない文で返す
// （api.Client.SendRequest の "timeout waiting for response from server"）。
const goobsTimeoutText = "timeout waiting for response from server"

// classifyError は goobs のエラーを、応答待ちの超過なら ErrTimeout を包んだ形にして返す。
// それ以外はそのまま返す。
func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), goobsTimeoutText) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	return err
}

// outputState は StreamStateChanged の outputState を、流すか（emit）と配信中か（streaming）に写す。
// STARTED と STOPPED だけ流す。STARTING / STOPPING / RECONNECTING / RECONNECTED と
// 未知の値は流さない（OBS から配信サイトへの再接続を、配信の停止として扱わないため）。
// obs-websocket は "OBS_WEBSOCKET_OUTPUT_" の接頭辞つきで送るので、それも同じに扱う。
func outputState(state string) (emit, streaming bool) {
	switch strings.TrimPrefix(state, "OBS_WEBSOCKET_OUTPUT_") {
	case "STARTED":
		return true, true
	case "STOPPED":
		return true, false
	}
	return false, false
}

// streamingFromStatus は GetStreamStatus から、接続時点の配信状態を求める。
// 再接続中は outputActive が false になるので、outputReconnecting も配信中とみなす。
func streamingFromStatus(outputActive, outputReconnecting bool) bool {
	return outputActive || outputReconnecting
}

// overrideFromResponse は GetSceneSceneTransitionOverride の応答を (ms, has) に写す。
// 名前がなければ上書きなし。名前があって長さが正なら Known。名前があって長さがなければ
// 長さ不明（ms は nil）。goobs の応答型は null と 0 を区別できないので、0 は長さなしとして扱う。
func overrideFromResponse(name string, durationMs float64) (ms *int, has bool) {
	if name == "" {
		return nil, false
	}
	if durationMs <= 0 {
		return nil, true
	}
	v := int(math.Round(durationMs))
	return &v, true
}

// ToOverride は SceneTransitionOverride の戻り値を switcher の Override に写す。
// エラーのときは、上書きあり・長さ不明にする。
func ToOverride(ms *int, has bool, err error) switcher.Override {
	switch {
	case err != nil:
		return switcher.OverrideUnknown{}
	case !has:
		return switcher.OverrideNone{}
	case ms == nil:
		return switcher.OverrideUnknown{}
	}
	return switcher.OverrideKnown{Ms: *ms}
}

// transitionMs は全体のトランジションの長さを *int にする。固定長と負の値は nil。
func transitionMs(fixed bool, durationMs float64) *int {
	if fixed || durationMs < 0 {
		return nil
	}
	v := int(math.Round(durationMs))
	return &v
}
