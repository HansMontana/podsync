package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	devicefs "github.com/HansMontana/podsync/internal/adapters/devicefs"
)

func TestValidateStagingDirectoryRejectsDevicePaths(t *testing.T) {
	root := t.TempDir()
	if err := validateStagingDirectory(root, filepath.Join(root, "staging")); err == nil {
		t.Fatal("validateStagingDirectory() accepted a device path")
	}
	if err := validateStagingDirectory(root, t.TempDir()); err != nil {
		t.Fatalf("validateStagingDirectory() rejected a host path: %v", err)
	}
	link := filepath.Join(t.TempDir(), "staging-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := validateStagingDirectory(root, link); err == nil {
		t.Fatal("validateStagingDirectory() accepted a symlink into the device")
	}
}

func TestPrepareStagingDirectoryPreservesParentFilesAndUsesRunDirectory(t *testing.T) {
	parent := t.TempDir()
	stale := filepath.Join(parent, "podsync-download-stale")
	if err := os.WriteFile(stale, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	runDir, err := prepareStagingDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(runDir) != parent {
		t.Fatalf("run directory %q is not under %q", runDir, parent)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("stale staging file was removed, error: %v", err)
	}
	if err := os.RemoveAll(runDir); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPlaybackRecordsAllowsMissingTagCache(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".rockbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	records, warnings, err := loadPlaybackRecords(devicefs.Layout{Root: root})
	if err != nil {
		t.Fatalf("loadPlaybackRecords() returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("got warnings %+v", warnings)
	}
	if len(records) != 0 {
		t.Fatalf("got records %+v", records)
	}
}

func TestStatusTreatsIncompleteTagCacheAsUnknownPlayback(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".rockbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".rockbox", "database_idx.tcd"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, warnings, err := loadPlaybackRecords(devicefs.Layout{Root: root})
	if err != nil {
		t.Fatalf("loadPlaybackRecords() rejected incomplete TagCache data: %v", err)
	}
	if len(records) != 0 || len(warnings) != 1 {
		t.Fatalf("got records=%+v warnings=%+v", records, warnings)
	}
}

func TestCLIHelpAndUnknownCommandUsage(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Fatalf("top-level help returned error: %v", err)
	}
	if err := run([]string{"help", "sync"}); err != nil {
		t.Fatalf("command help returned error: %v", err)
	}
	err := run([]string{"not-a-command"})
	if err == nil || !strings.Contains(err.Error(), "Usage: podsync") {
		t.Fatalf("unknown command error did not include usage: %v", err)
	}
}

func TestCLIRejectsUnexpectedArguments(t *testing.T) {
	if err := run([]string{"status", "unexpected"}); err == nil {
		t.Fatal("status accepted an unexpected argument")
	}
	if err := run([]string{"help", "sync", "unexpected"}); err == nil {
		t.Fatal("help accepted an unexpected argument")
	}
}

func TestUsageTextUsesConsistentSpaceIndentation(t *testing.T) {
	text := usageText()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "\t") {
			t.Fatalf("usage line starts with a tab: %q", line)
		}
	}
	for _, command := range []string{"validate-config", "reconcile", "refresh", "update", "status", "verify", "feed", "playlist", "briefing", "sync"} {
		if !strings.Contains(text, "  "+command) {
			t.Fatalf("usage text does not contain consistently indented command %q", command)
		}
	}
}
