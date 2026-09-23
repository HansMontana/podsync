package playback

import (
	"testing"
	"time"
)

func TestStatePlayedRequiresKnownPositivePlayCount(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{name: "unknown with count", state: State{PlayCount: 1}, want: false},
		{name: "known with no count", state: State{Known: true}, want: false},
		{name: "known with negative count", state: State{Known: true, PlayCount: -1}, want: false},
		{name: "known with count", state: State{Known: true, PlayCount: 1, LastPlayed: time.Unix(1, 0)}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.state.Played(); got != test.want {
				t.Fatalf("Played() = %t, want %t for %+v", got, test.want, test.state)
			}
		})
	}
}
