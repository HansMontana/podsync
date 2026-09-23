package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/HansMontana/podsync/internal/adapters/sqlitecatalog"
	"github.com/HansMontana/podsync/internal/adapters/tomlconfig"
	"github.com/HansMontana/podsync/internal/domain/curation"
)

func TestRealFeedCLIWorkflow(t *testing.T) {
	if os.Getenv("PODSYNC_REAL_FEEDS") != "1" {
		t.Skip("set PODSYNC_REAL_FEEDS=1 to run against live RSS feeds")
	}

	root := t.TempDir()
	configPath := filepath.Join("..", "..", "examples", "daily-briefing.toml")
	expected, err := tomlconfig.Load(configPath)
	if err != nil {
		t.Fatalf("load daily briefing example: %v", err)
	}

	if err := run([]string{"validate-config", "-config", configPath}); err != nil {
		t.Fatalf("validate-config command failed: %v", err)
	}
	if err := run([]string{"reconcile", "-device-root", root, "-config", configPath}); err != nil {
		t.Fatalf("reconcile command failed: %v", err)
	}
	assertDeviceConfig(t, root, expected)

	if err := run([]string{"feed", "list", "-device-root", root}); err != nil {
		t.Fatalf("feed list command failed: %v", err)
	}
	if err := run([]string{"refresh", "-device-root", root}); err != nil {
		t.Fatalf("refresh command failed: %v", err)
	}

	repository, err := sqlitecatalog.NewReadOnlySQLiteRepository(filepath.Join(root, "Podsync", "podsync.db"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := repository.Load()
	if closeErr := repository.Close(); err != nil {
		t.Fatal(err)
	} else if closeErr != nil {
		t.Fatal(closeErr)
	}
	if len(current.Episodes) == 0 {
		t.Fatal("refresh imported no episodes from live feeds")
	}

	if err := run([]string{"status", "-device-root", root}); err != nil {
		t.Fatalf("status command failed: %v", err)
	}
	if err := run([]string{"playlist", "-device-root", root, "-id", "dlf-german-newspapers"}); err != nil {
		t.Fatalf("playlist command failed: %v", err)
	}
	if err := run([]string{"briefing", "-device-root", root, "-id", "daily"}); err != nil {
		t.Fatalf("briefing command failed: %v", err)
	}
	if err := run([]string{"sync", "-device-root", root, "-dry-run"}); err != nil {
		t.Fatalf("sync dry-run command failed: %v", err)
	}
	for _, relative := range []string{
		filepath.Join("Playlists", "German Newspaper Presseschau.m3u8"),
		filepath.Join("Playlists", "Daily Briefing.m3u8"),
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("expected generated playlist %s: %v", relative, err)
		}
	}

	added := expected
	added.Sources = append(added.Sources, curation.SourceFeed{
		ID:  "nightvale",
		URL: "https://feeds.megaphone.fm/SBP4591212513",
	})
	if err := run([]string{
		"feed", "add", "-device-root", root,
		"-id", "nightvale", "-url", "https://feeds.megaphone.fm/SBP4591212513",
	}); err != nil {
		t.Fatalf("feed add command failed: %v", err)
	}
	assertDeviceConfig(t, root, added)
	if err := run([]string{"refresh", "-device-root", root}); err != nil {
		t.Fatalf("refresh after feed add failed: %v", err)
	}
	if err := run([]string{"feed", "remove", "-device-root", root, "-id", "nightvale"}); err != nil {
		t.Fatalf("feed remove command failed: %v", err)
	}
	assertDeviceConfig(t, root, expected)

	podcastRoot := t.TempDir()
	podcastConfigPath := filepath.Join("..", "..", "examples", "podcasts.toml")
	podcastExpected, err := tomlconfig.Load(podcastConfigPath)
	if err != nil {
		t.Fatalf("load podcasts example: %v", err)
	}
	if err := run([]string{"validate-config", "-config", podcastConfigPath}); err != nil {
		t.Fatalf("validate-config podcasts command failed: %v", err)
	}
	if err := run([]string{"reconcile", "-device-root", podcastRoot, "-config", podcastConfigPath}); err != nil {
		t.Fatalf("reconcile podcasts command failed: %v", err)
	}
	assertDeviceConfig(t, podcastRoot, podcastExpected)
	if err := run([]string{"refresh", "-device-root", podcastRoot}); err != nil {
		t.Fatalf("refresh podcasts command failed: %v", err)
	}
	if err := run([]string{"status", "-device-root", podcastRoot}); err != nil {
		t.Fatalf("status podcasts command failed: %v", err)
	}
	if err := run([]string{"playlist", "-device-root", podcastRoot, "-id", "sgu"}); err != nil {
		t.Fatalf("playlist podcasts command failed: %v", err)
	}
	if err := run([]string{"sync", "-device-root", podcastRoot, "-dry-run"}); err != nil {
		t.Fatalf("sync podcasts dry-run command failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(podcastRoot, "Playlists", "The Skeptics' Guide to the Universe.m3u8")); err != nil {
		t.Fatalf("expected generated podcasts playlist: %v", err)
	}
}

func assertDeviceConfig(t *testing.T, root string, want curation.Config) {
	t.Helper()

	got, err := tomlconfig.Load(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("device config = %+v, want %+v", got, want)
	}
}
