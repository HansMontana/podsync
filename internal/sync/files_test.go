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
		[]string{"Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"},
		[]FileCopy{{Source: source, Relative: "Podcasts/podcast-1/bbbbbbbbbbbbbbbb.mp3"}},
		[]PlaylistFile{{Relative: "Playlists/morning.m3u8", Content: []byte("playlist\n")}},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildFilePlan() returned error: %v", err)
	}
	if len(plan.Deletes) != 1 || plan.Deletes[0] != "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3" {
		t.Fatalf("got deletes %+v", plan.Deletes)
	}
	if err := os.MkdirAll(filepath.Join(root, "Music"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Music/user.mp3"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Podcasts/podcast-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ApplyFilePlan(context.Background(), root, plan); err != nil {
		t.Fatalf("ApplyFilePlan() returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3")); !os.IsNotExist(err) {
		t.Fatalf("old managed file still exists, error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Music/user.mp3")); err != nil {
		t.Fatalf("unmanaged file was removed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Podcasts/podcast-1/bbbbbbbbbbbbbbbb.mp3"))
	if err != nil || string(data) != "audio" {
		t.Fatalf("got copied data %q, error %v", data, err)
	}
}

func TestBuildFilePlanAcceptsReadablePlaylistPath(t *testing.T) {
	if _, err := BuildFilePlan(nil, nil, []PlaylistFile{{Relative: "Playlists/Süddeutsche Zeitung.m3u8"}}, nil); err != nil {
		t.Fatalf("BuildFilePlan() rejected readable playlist path: %v", err)
	}
}

func TestBuildFilePlanRejectsEscapingPath(t *testing.T) {
	if _, err := BuildFilePlan(nil, []FileCopy{{Source: "audio", Relative: "../outside"}}, nil, nil); err == nil {
		t.Fatal("BuildFilePlan() accepted an escaping path")
	}
}

func TestApplyFilePlanReportsCopyAndPlaylistProgress(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(source, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := FilePlan{
		Copies:    []FileCopy{{Source: source, Relative: "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}},
		Playlists: []PlaylistFile{{Relative: "Playlists/Daily Briefing.m3u8", Content: []byte("#EXTM3U\n")}},
	}
	var progress []FileProgress
	if err := ApplyFilePlanWithProgress(context.Background(), root, plan, func(current FileProgress) {
		progress = append(progress, current)
	}); err != nil {
		t.Fatal(err)
	}
	if len(progress) != 2 {
		t.Fatalf("got progress %+v", progress)
	}
	if progress[0].Phase != "copy" || progress[0].Completed != 1 || progress[0].Total != 1 {
		t.Fatalf("got copy progress %+v", progress[0])
	}
	if progress[1].Phase != "playlist" || progress[1].Completed != 1 || progress[1].Total != 1 {
		t.Fatalf("got playlist progress %+v", progress[1])
	}
}

func TestBuildFilePlanRejectsUnownedManifestPaths(t *testing.T) {
	for _, current := range []string{"iPod_Control/iTunes/iTunesDB", "Podsync/podsync.db", "Music/manual.mp3", "Podcasts/feed-1/aaaaaaaaaaaaaaaa.mp3"} {
		if _, err := BuildFilePlan([]string{current}, nil, nil, nil); err == nil {
			t.Fatalf("BuildFilePlan() accepted %q", current)
		}
	}
}

func TestVerifyManagedFiles(t *testing.T) {
	root := t.TempDir()
	path := "Podcasts/podcast/episode.mp3"
	destination := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyManagedFiles(root, []string{path}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyManagedFiles(root, []string{"Podcasts/podcast/missing.mp3"}); err == nil {
		t.Fatal("VerifyManagedFiles() accepted a missing file")
	}
}

func TestApplyFilePlanRejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "Podcasts")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := ApplyFilePlan(context.Background(), root, FilePlan{Playlists: []PlaylistFile{{Relative: "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}}}); err == nil {
		t.Fatal("ApplyFilePlan() followed a symlink")
	}
}

func TestBuildFilePlanRejectsDuplicateDestination(t *testing.T) {
	if _, err := BuildFilePlan(nil, []FileCopy{{Source: "one", Relative: "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}}, []PlaylistFile{{Relative: "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}}, nil); err == nil {
		t.Fatal("BuildFilePlan() accepted duplicate destinations")
	}
}

func TestApplyFilePlanCancellationLeavesExistingDestinationUntouched(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3")
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
	if err := ApplyFilePlan(ctx, root, FilePlan{Copies: []FileCopy{{Source: source, Relative: "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}}}); err == nil {
		t.Fatal("ApplyFilePlan() accepted a canceled context")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "old" {
		t.Fatalf("existing destination changed to %q, error %v", data, err)
	}
}
