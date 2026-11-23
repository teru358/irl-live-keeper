package obs

import (
	"errors"
	"testing"
)

func TestAuxSceneRejectedStatus(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want bool
	}{
		{"request SetCurrentProgramScene: ResourceNotFound (600): missing", true},
		{"request SetCurrentProgramScene: timeout waiting for response from server", false},
		{"request SetCurrentProgramScene: mismatched ID: expected x got y", false},
		{"write: broken pipe", false},
		{"request GetSceneList: ResourceNotFound (600)", false},
		{"request SetCurrentProgramScene: Success (100)", false},
		{"request SetCurrentProgramScene: broken pipe (600)", false},
	} {
		if got := auxRejected("SetCurrentProgramScene", errors.New(tc.s)); got != tc.want {
			t.Errorf("%s: %v", tc.s, got)
		}
	}
}
