package playlists

import (
	"strings"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/domain/curation"
	"github.com/HansMontana/podsync/internal/domain/playback"
)

func TestM3UProducesExtendedPlaylist(t *testing.T) {
	got := string(M3U([]Track{{
		Episode: catalog.Episode{Title: "Morning news", Duration: 2*time.Minute + 3*time.Second},
		Path:    "Podcasts/news.mp3",
	}}))
	want := "#EXTM3U\n#EXTINF:123,Morning news\nPodcasts/news.mp3\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestM3USanitizesControlCharactersInTitles(t *testing.T) {
	got := string(M3U([]Track{{Episode: catalog.Episode{Title: "A\rB\nC\x00D"}, Path: "episode.mp3"}}))
	if strings.ContainsAny(got, "\r\x00") || !strings.Contains(got, "A B C D") {
		t.Fatalf("M3U() = %q", got)
	}
}

func TestFilenameUsesReadableSafeTitle(t *testing.T) {
	if got := Filename("Süddeutsche Zeitung: Auf den Punkt", "sz"); got != "Süddeutsche Zeitung Auf den Punkt" {
		t.Fatalf("Filename() = %q", got)
	}
	if got := Filename("", "fallback"); got != "fallback" {
		t.Fatalf("Filename() with empty title = %q", got)
	}
}

func TestLogicalFeedUsesStableDevicePathsAndConfiguredOrder(t *testing.T) {
	cfg := curation.Config{
		Sources: []curation.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []curation.LogicalFeed{{ID: "world", Source: "news", Order: curation.OldestFirst, Limit: 1}},
	}
	current := catalog.Catalog{
		Feeds: []catalog.Feed{{ID: 1, URL: "https://example.com/news.xml"}},
		Episodes: []catalog.Episode{
			{FeedID: 1, GUID: "new", Title: "New", PublishedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Enclosure: catalog.Enclosure{URL: "https://example.com/new.mp3", Type: "audio/mpeg"}},
			{FeedID: 1, GUID: "old", Title: "Old", PublishedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Enclosure: catalog.Enclosure{URL: "https://example.com/old.mp3", Type: "audio/mpeg"}},
		},
	}
	playlist, err := LogicalFeed(cfg, current, "world", map[string]playback.State{})
	if err != nil {
		t.Fatalf("LogicalFeed() returned error: %v", err)
	}
	text := string(playlist)
	if !strings.Contains(text, "New") || strings.Contains(text, "Old") {
		t.Fatalf("playlist does not use the synced newest storage window: %q", text)
	}
	if !strings.Contains(text, "../Podcasts/world/") {
		t.Fatalf("playlist does not use device-relative stable path: %q", text)
	}
}
