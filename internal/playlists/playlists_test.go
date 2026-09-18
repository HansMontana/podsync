package playlists

import (
	"strings"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/playback"
	"github.com/HansMontana/podsync/internal/state"
)

func TestM3UProducesExtendedPlaylist(t *testing.T) {
	got := string(M3U([]Track{{
		Episode: episode.Episode{Title: "Morning news", Duration: 2*time.Minute + 3*time.Second},
		Path:    "Podcasts/news.mp3",
	}}))
	want := "#EXTM3U\n#EXTINF:123,Morning news\nPodcasts/news.mp3\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLogicalFeedUsesStableDevicePathsAndConfiguredOrder(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "world", Source: "news", Order: config.OldestFirst}},
	}
	current := state.State{
		Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}},
		Episodes: []episode.Episode{
			{FeedID: 1, GUID: "new", Title: "New", PublishedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Enclosure: episode.Enclosure{URL: "https://example.com/new.mp3", Type: "audio/mpeg"}},
			{FeedID: 1, GUID: "old", Title: "Old", PublishedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Enclosure: episode.Enclosure{URL: "https://example.com/old.mp3", Type: "audio/mpeg"}},
		},
	}
	playlist, err := LogicalFeed(cfg, current, "world", map[string]playback.State{})
	if err != nil {
		t.Fatalf("LogicalFeed() returned error: %v", err)
	}
	text := string(playlist)
	if strings.Index(text, "Old") > strings.Index(text, "New") {
		t.Fatalf("playlist is not oldest-first: %q", text)
	}
	if !strings.Contains(text, "../Podcasts/feed-1/") {
		t.Fatalf("playlist does not use device-relative stable path: %q", text)
	}
}

func TestValidateTracksRejectsEmptyPath(t *testing.T) {
	if err := ValidateTracks([]Track{{Path: "  "}}); err == nil {
		t.Fatal("ValidateTracks() accepted an empty path")
	}
	if !strings.Contains(string(M3U(nil)), "#EXTM3U") {
		t.Fatal("M3U() did not write the playlist header")
	}
}
