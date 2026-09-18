package device

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLayoutUsesDeviceRelativePaths(t *testing.T) {
	layout := Layout{Root: "/media/ipod"}
	if layout.DatabasePath() != filepath.Join("/media/ipod", "Podsync", "podsync.db") {
		t.Fatalf("unexpected database path %q", layout.DatabasePath())
	}
	if layout.ConfigPath() != filepath.Join("/media/ipod", "Podsync", "podsync.toml") {
		t.Fatalf("unexpected config path %q", layout.ConfigPath())
	}
}

func TestManagedPathsRoundTrip(t *testing.T) {
	layout := Layout{Root: t.TempDir()}
	want := []string{"Podcasts/feed-1/episode.mp3", "Playlists/morning.m3u8"}
	if err := layout.SaveManagedPaths(want); err != nil {
		t.Fatalf("SaveManagedPaths() returned error: %v", err)
	}
	got, err := layout.LoadManagedPaths()
	if err != nil {
		t.Fatalf("LoadManagedPaths() returned error: %v", err)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got paths %v, want %v", got, want)
	}
}
