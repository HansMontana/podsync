package media

import (
	"strings"
	"testing"

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
