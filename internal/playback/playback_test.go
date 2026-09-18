package playback

import (
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/media"
)

func TestForEpisodesMatchesNormalizedDevicePaths(t *testing.T) {
	current := episode.Episode{FeedID: 2, GUID: "one", Enclosure: episode.Enclosure{URL: "https://example.com/one.mp3", Type: "audio/mpeg"}}
	playedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	states := ForEpisodes([]episode.Episode{current}, []Record{{Path: "/" + media.RelativePath(current), PlayCount: 3, LastPlayed: playedAt}})
	state := states[current.IdentityKey()]
	if !state.Known || !state.Played() || state.PlayCount != 3 || !state.LastPlayed.Equal(playedAt) {
		t.Fatalf("got playback state %+v", state)
	}
}

func TestForEpisodesTreatsMissingRecordsAsUnplayed(t *testing.T) {
	current := episode.Episode{FeedID: 2, GUID: "one", Enclosure: episode.Enclosure{URL: "https://example.com/one.mp3", Type: "audio/mpeg"}}
	state := ForEpisodes([]episode.Episode{current}, nil)[current.IdentityKey()]
	if state.Known || state.Played() {
		t.Fatalf("got playback state %+v", state)
	}
}
