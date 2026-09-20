package media

import (
	"strings"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
)

func TestRelativePathIsStableWhenMetadataChanges(t *testing.T) {
	first := episode.Episode{
		FeedID:    7,
		GUID:      "episode-1",
		Title:     "Original title",
		Enclosure: episode.Enclosure{URL: "https://example.com/episode.mp3", Type: "audio/mpeg"},
	}
	second := first
	second.Title = "Corrected title"
	if RelativePath(first) != RelativePath(second) {
		t.Fatal("metadata changes changed the stable media path")
	}
}

func TestRelativePathDoesNotEscapeDeviceRoot(t *testing.T) {
	path := RelativePath(episode.Episode{FeedID: 1, GUID: "../../outside", Enclosure: episode.Enclosure{URL: "https://example.com/file.mp3", Type: "audio/mpeg"}})
	if strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		t.Fatalf("path escaped device root: %q", path)
	}
}

func TestRelativePathUsesAudioExtension(t *testing.T) {
	path := RelativePath(episode.Episode{FeedID: 1, GUID: "episode", Enclosure: episode.Enclosure{Type: "audio/ogg"}})
	if !strings.HasSuffix(path, ".ogg") {
		t.Fatalf("got path %q", path)
	}
}

func TestRelativePathForUsesLogicalIDTitleDateAndHash(t *testing.T) {
	episodeValue := episode.Episode{
		FeedID:      7,
		GUID:        "episode-1",
		Title:       "A/B: New Episode!",
		PublishedAt: time.Date(2026, 9, 20, 23, 0, 0, 0, time.FixedZone("test", 2*60*60)),
		Enclosure:   episode.Enclosure{Type: "audio/mpeg"},
	}
	got := (Resolver{7: "welcome-to-night-vale"}).RelativePathFor(episodeValue)
	if !strings.HasPrefix(got, "Podcasts/welcome-to-night-vale/A B New Episode! - 2026-09-20 -- ") || !strings.HasSuffix(got, ".mp3") {
		t.Fatalf("got path %q", got)
	}
}
