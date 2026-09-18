package playback

// State contains playback information imported from a device.
// Unknown state is intentionally treated as unplayed.
type State struct {
	Known     bool
	PlayCount int
}

func (s State) Played() bool {
	return s.Known && s.PlayCount > 0
}
