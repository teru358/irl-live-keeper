package core

import (
	"testing"
	"time"
)

func TestSnapshotSince(t *testing.T) {
	clk := newClock()
	s := Snapshot{Cameras: []CameraStatus{{ID: "a", State: "DOWN"}, {ID: "b", State: "OK"}}, OBS: OBSStatus{Standby: StandbyStatus{Owner: "none"}}}
	s.recordSince(nil, clk.Now())
	initial := s.Cameras[0].StateSince
	if initial != clk.Now() || !s.OBS.Standby.Since.IsZero() {
		t.Fatal(s)
	}
	update := func(state, owner string) {
		prev := s
		s.Cameras = append([]CameraStatus(nil), s.Cameras...)
		s.Cameras[0].State = state
		s.OBS.Standby.Owner = owner
		s.recordSince(&prev, clk.Advance(time.Second))
	}
	update("DOWN", "none")
	if s.Cameras[0].StateSince != initial {
		t.Fatal("同じ状態で更新")
	}
	update("LOW", "auto")
	started := clk.Now()
	if s.Cameras[0].StateSince != started || s.OBS.Standby.Since != started || s.Cameras[1].StateSince != initial {
		t.Fatal(s)
	}
	update("LOW", "manual")
	if s.OBS.Standby.Since != started {
		t.Fatal("持ち主の交代で更新")
	}
	update("OK", "none")
	if !s.OBS.Standby.Since.IsZero() {
		t.Fatal("none の時刻")
	}
	update("OK", "manual")
	if s.OBS.Standby.Since != clk.Now() {
		t.Fatal("新しい待機")
	}
}
