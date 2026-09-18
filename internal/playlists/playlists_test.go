package playlists

import (
	"strings"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
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

func TestValidateTracksRejectsEmptyPath(t *testing.T) {
	if err := ValidateTracks([]Track{{Path: "  "}}); err == nil {
		t.Fatal("ValidateTracks() accepted an empty path")
	}
	if !strings.Contains(string(M3U(nil)), "#EXTM3U") {
		t.Fatal("M3U() did not write the playlist header")
	}
}
