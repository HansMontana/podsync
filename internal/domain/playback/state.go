package playback

import "time"

// State contains the playback information used by domain decisions.
// Unknown state is intentionally treated as unplayed.
type State struct {
	Known      bool
	PlayCount  int
	LastPlayed time.Time
	Skipped    bool
}

func (s State) Played() bool {
	return s.Known && s.PlayCount > 0
}

func (s State) Consumed() bool {
	return s.Known && (s.PlayCount > 0 || s.Skipped)
}
