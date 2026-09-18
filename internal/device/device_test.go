package device

import (
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
