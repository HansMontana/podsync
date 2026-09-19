package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildAndApplyFilePlan(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(source, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildFilePlan(
		[]string{"Podcasts/old.mp3"},
		[]FileCopy{{Source: source, Relative: "Podcasts/new.mp3"}},
		[]PlaylistFile{{Relative: "Playlists/morning.m3u8", Content: []byte("playlist\n")}},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildFilePlan() returned error: %v", err)
	}
	if len(plan.Deletes) != 1 || plan.Deletes[0] != "Podcasts/old.mp3" {
		t.Fatalf("got deletes %+v", plan.Deletes)
	}
	if err := os.MkdirAll(filepath.Join(root, "Music"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Music/user.mp3"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Podcasts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Podcasts/old.mp3"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ApplyFilePlan(context.Background(), root, plan); err != nil {
		t.Fatalf("ApplyFilePlan() returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Podcasts/old.mp3")); !os.IsNotExist(err) {
		t.Fatalf("old managed file still exists, error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Music/user.mp3")); err != nil {
		t.Fatalf("unmanaged file was removed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Podcasts/new.mp3"))
	if err != nil || string(data) != "audio" {
		t.Fatalf("got copied data %q, error %v", data, err)
	}
}

func TestBuildFilePlanRejectsEscapingPath(t *testing.T) {
	if _, err := BuildFilePlan(nil, []FileCopy{{Source: "audio", Relative: "../outside"}}, nil, nil); err == nil {
		t.Fatal("BuildFilePlan() accepted an escaping path")
	}
}

func TestBuildFilePlanRejectsDuplicateDestination(t *testing.T) {
	if _, err := BuildFilePlan(nil, []FileCopy{{Source: "one", Relative: "Podcasts/one.mp3"}}, []PlaylistFile{{Relative: "Podcasts/one.mp3"}}, nil); err == nil {
		t.Fatal("BuildFilePlan() accepted duplicate destinations")
	}
}

func TestApplyFilePlanCancellationLeavesExistingDestinationUntouched(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "Podcasts/episode.mp3")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(source, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ApplyFilePlan(ctx, root, FilePlan{Copies: []FileCopy{{Source: source, Relative: "Podcasts/episode.mp3"}}}); err == nil {
		t.Fatal("ApplyFilePlan() accepted a canceled context")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "old" {
		t.Fatalf("existing destination changed to %q, error %v", data, err)
	}
}
