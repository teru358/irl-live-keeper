package obs

import (
	"math"
	"time"

	"github.com/andreykaipov/goobs/api/events"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// mapEvent は goobs のイベントを switcher の Event に写す。at は受信時刻。
// 流さないイベント（STARTING などの配信状態、関係のないイベント）は ok=false。
// CurrentSceneTransitionChanged は長さを取り直す必要があるので、ここでは扱わない。
func mapEvent(ev any, at time.Time) (out switcher.Event, ok bool) {
	switch e := ev.(type) {
	case *events.CurrentProgramSceneChanged:
		return switcher.SceneChanged{At: at, Scene: e.SceneName, SceneUUID: e.SceneUuid}, true
	case *events.CurrentSceneTransitionDurationChanged:
		ms := int(math.Round(e.TransitionDuration))
		return switcher.TransitionChanged{At: at, TransitionMs: &ms}, true
	case *events.StreamStateChanged:
		emit, streaming := outputState(e.OutputState)
		if !emit {
			return nil, false
		}
		return switcher.StreamStateChanged{At: at, Streaming: streaming}, true
	}
	return nil, false
}
