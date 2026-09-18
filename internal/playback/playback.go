package playback

import (
	"path"
	"strings"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/media"
)

// State contains playback information imported from a device.
// Unknown state is intentionally treated as unplayed.
type State struct {
	Known      bool
	PlayCount  int
	LastPlayed time.Time
}

func (s State) Played() bool {
	return s.Known && s.PlayCount > 0
}

type Record struct {
	Path       string
	PlayCount  int
	LastPlayed time.Time
}

// ForEpisodes matches device records by stable device-relative media path.
// Episodes without a trusted matching record remain unknown and therefore
// unplayed.
func ForEpisodes(episodes []episode.Episode, records []Record) map[string]State {
	byPath := make(map[string]State, len(records))
	for _, record := range records {
		key := normalizePath(record.Path)
		if key == "" {
			continue
		}
		current := byPath[key]
		if !current.Known || record.PlayCount > current.PlayCount || record.LastPlayed.After(current.LastPlayed) {
			byPath[key] = State{Known: true, PlayCount: record.PlayCount, LastPlayed: record.LastPlayed}
		}
	}

	result := make(map[string]State, len(episodes))
	for _, current := range episodes {
		state, exists := byPath[normalizePath(media.RelativePath(current))]
		if exists {
			result[current.IdentityKey()] = state
		} else {
			result[current.IdentityKey()] = State{}
		}
	}
	return result
}

func normalizePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.TrimPrefix(value, "/")
	clean := path.Clean(value)
	if clean == "." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}
