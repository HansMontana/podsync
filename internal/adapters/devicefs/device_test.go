package devicefs

import (
	"os"
	"path/filepath"
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

func TestPendingManagedPathsRecoverOwnership(t *testing.T) {
	layout := Layout{Root: t.TempDir()}
	if err := os.MkdirAll(layout.StateDirectory(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.ManifestPath(), []byte("Podcasts/current/episode.mp3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := layout.SavePendingManagedPaths([]string{"Podcasts/current/episode.mp3", "Podcasts/old/episode.mp3"}); err != nil {
		t.Fatal(err)
	}
	managed, err := layout.LoadManagedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(managed) != 2 || managed[1] != "Podcasts/old/episode.mp3" {
		t.Fatalf("got managed paths %+v", managed)
	}
	if err := layout.ClearPendingManagedPaths(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.PendingManifestPath()); !os.IsNotExist(err) {
		t.Fatalf("pending manifest remains, error: %v", err)
	}
}
