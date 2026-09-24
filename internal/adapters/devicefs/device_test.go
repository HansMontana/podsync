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

func TestRequestTagCacheUpdateCreatesIdempotentMarker(t *testing.T) {
	root := t.TempDir()
	layout := Layout{Root: root}
	if err := os.Mkdir(layout.TagCacheDirectory(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := layout.RequestTagCacheUpdate(); err != nil {
		t.Fatalf("RequestTagCacheUpdate() returned error: %v", err)
	}
	marker := layout.TagCacheUpdateMarkerPath()
	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 0 {
		t.Fatalf("marker contains %q, want empty marker", content)
	}
	if err := os.WriteFile(marker, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := layout.RequestTagCacheUpdate(); err != nil {
		t.Fatalf("second RequestTagCacheUpdate() returned error: %v", err)
	}
	content, err = os.ReadFile(marker)
	if err != nil || string(content) != "existing" {
		t.Fatalf("existing marker changed to %q, error %v", content, err)
	}
}

func TestRequestTagCacheUpdateRejectsUnsafeMarkerPaths(t *testing.T) {
	root := t.TempDir()
	layout := Layout{Root: root}
	if err := os.Mkdir(layout.TagCacheDirectory(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), layout.TagCacheUpdateMarkerPath()); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := layout.RequestTagCacheUpdate(); err == nil {
		t.Fatal("RequestTagCacheUpdate() accepted a symlink marker")
	}
}

func TestPendingTagCacheUpdatePersistsUntilCleared(t *testing.T) {
	layout := Layout{Root: t.TempDir()}
	pending, err := layout.LoadPendingTagCacheUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("new layout has pending TagCache update")
	}
	if err := layout.SavePendingTagCacheUpdate(); err != nil {
		t.Fatal(err)
	}
	pending, err = layout.LoadPendingTagCacheUpdate()
	if err != nil || !pending {
		t.Fatalf("pending=%v, error=%v", pending, err)
	}
	if err := layout.ClearPendingTagCacheUpdate(); err != nil {
		t.Fatal(err)
	}
	pending, err = layout.LoadPendingTagCacheUpdate()
	if err != nil || pending {
		t.Fatalf("pending=%v after clear, error=%v", pending, err)
	}
}
