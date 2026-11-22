package obs

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// ErrTimeout は OBS からの応答が待ち時間（5 秒）内に返らなかったこと。
// SetScene などの失敗と区別するために使う。
var ErrTimeout = errors.New("OBS の応答がタイムアウト")

// goobs は応答待ちの超過を、番兵のエラーを持たない文で返す
// （api.Client.SendRequest の "timeout waiting for response from server"）。
const goobsTimeoutText = "timeout waiting for response from server"

// goobs は応答の ID が要求と合わないときも、番兵のエラーを持たない文で返す。応答待ちの超過と同じく
// 「適用されたかわからない」ので、同じ扱いにする。
const goobsMismatchText = "mismatched ID"

// 接続が切れたことによる失敗の文。切断のイベントより先に、要求の失敗として返ることがある（順は決まらない）。
// 送ったかどうか、OBS が適用したかどうかがわからないので、応答待ちの超過と同じ「未決着」として扱う。
// goobs が「送らなかった」と返す "client already disconnected" は含めない。
var connLostRe = regexp.MustCompile(`use of closed network connection|broken pipe|connection reset|websocket: close|unexpected EOF|: EOF$|^EOF$`)

// classifyError は goobs のエラーを、応答待ちの超過か ID のずれ、接続が切れたことによる失敗なら
// ErrTimeout を包んだ形にして返す。それ以外はそのまま返す。
func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if msg := err.Error(); strings.Contains(msg, goobsTimeoutText) || strings.Contains(msg, goobsMismatchText) || connLostRe.MatchString(msg) {
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

// goobs は OBS の失敗の応答を、型を持たない文 "request <名前>: <状態の名前> (<コード>)[: <コメント>]" で返す
// （goobs v1.11.1 の api.Client.SendRequest。状態コードを取り出す型や関数はない）。
// ResourceNotFound は状態コード 600。
var notFoundRe = regexp.MustCompile(`request \w+: ResourceNotFound \(600\)`)

// ClassifyItemResult は SetSceneItemEnabled のエラーを switcher.ItemResult に分ける。
// nil は OK、応答待ちの超過と ID のずれ（ErrTimeout）は Timeout、OBS の状態コード 600（ResourceNotFound）は
// NotFound、それ以外（OBS の失敗、送れなかった）は Error。状態コードは goobs のエラーの文から読む。
func ClassifyItemResult(err error) switcher.ItemResult {
	switch {
	case err == nil:
		return switcher.ItemOK
	case errors.Is(err, ErrTimeout):
		return switcher.ItemTimeout
	case connLostRe.MatchString(err.Error()):
		return switcher.ItemTimeout
	case notFoundRe.MatchString(err.Error()):
		return switcher.ItemNotFound
	}
	return switcher.ItemError
}
