package media

import (
	"strings"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func TestRelativePathForKeepsIdentityHashWhenMetadataChanges(t *testing.T) {
	first := catalog.Episode{
		FeedID:    7,
		GUID:      "episode-1",
		Title:     "Original title",
		Enclosure: catalog.Enclosure{URL: "https://example.com/episode.mp3", Type: "audio/mpeg"},
	}
	second := first
	second.Title = "Corrected title"
	resolver := Resolver{7: "podcast"}
	firstPath := resolver.RelativePathFor(first)
	secondPath := resolver.RelativePathFor(second)
	firstHash := firstPath[strings.LastIndex(firstPath, " -- "):]
	secondHash := secondPath[strings.LastIndex(secondPath, " -- "):]
	if firstHash != secondHash {
		t.Fatalf("metadata changes changed the identity hash: %q != %q", firstHash, secondHash)
	}
}

func TestRelativePathForDoesNotEscapeDeviceRoot(t *testing.T) {
	path := (Resolver{1: "podcast"}).RelativePathFor(catalog.Episode{FeedID: 1, GUID: "../../outside", Enclosure: catalog.Enclosure{URL: "https://example.com/file.mp3", Type: "audio/mpeg"}})
	if strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		t.Fatalf("path escaped device root: %q", path)
	}
}

func TestRelativePathForUsesAudioExtension(t *testing.T) {
	path := (Resolver{1: "podcast"}).RelativePathFor(catalog.Episode{FeedID: 1, GUID: "episode", Enclosure: catalog.Enclosure{Type: "audio/ogg"}})
	if !strings.HasSuffix(path, ".ogg") {
		t.Fatalf("got path %q", path)
	}
}

func TestRelativePathForUsesLogicalIDTitleDateAndHash(t *testing.T) {
	episodeValue := catalog.Episode{
		FeedID:      7,
		GUID:        "episode-1",
		Title:       "A/B: New Episode!",
		PublishedAt: time.Date(2026, 9, 20, 23, 0, 0, 0, time.FixedZone("test", 2*60*60)),
		Enclosure:   catalog.Enclosure{Type: "audio/mpeg"},
	}
	got := (Resolver{7: "welcome-to-night-vale"}).RelativePathFor(episodeValue)
	if !strings.HasPrefix(got, "Podcasts/welcome-to-night-vale/A B New Episode! - 2026-09-20 -- ") || !strings.HasSuffix(got, ".mp3") {
		t.Fatalf("got path %q", got)
	}
}
