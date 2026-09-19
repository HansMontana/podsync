package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/device"
)

func TestRefreshAndSyncCommandsUseDeviceStateAndStaging(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/episode.mp3" {
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("audio"))
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>News</title><link>https://example.com</link><description>News</description><item><title>Episode one</title><guid>one</guid><pubDate>Fri, 02 Jan 2026 00:00:00 GMT</pubDate><enclosure url="%s/episode.mp3" type="audio/mpeg" length="5"/></item></channel></rss>`, server.URL)
	}))
	defer server.Close()

	root := t.TempDir()
	hostConfig := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "news", Source: "news", Order: config.NewestFirst}},
	}
	if err := config.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"refresh", "-device-root", root, "-config", hostConfig}); err != nil {
		t.Fatalf("refresh command failed: %v", err)
	}
	databaseBefore, err := os.ReadFile(filepath.Join(root, "Podsync", "podsync.db"))
	if err != nil {
		t.Fatal(err)
	}
	configBefore, err := os.ReadFile(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sync", "-device-root", root, "-dry-run"}); err != nil {
		t.Fatalf("dry-run command failed: %v", err)
	}
	databaseAfter, err := os.ReadFile(filepath.Join(root, "Podsync", "podsync.db"))
	if err != nil {
		t.Fatal(err)
	}
	configAfter, err := os.ReadFile(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(databaseBefore, databaseAfter) || !bytes.Equal(configBefore, configAfter) {
		t.Fatal("dry-run modified device state")
	}
	if files, err := filepath.Glob(filepath.Join(root, "Podcasts", "feed-1", "*")); err != nil || len(files) != 0 {
		t.Fatalf("dry-run changed device audio: %v, error %v", files, err)
	}
	if err := run([]string{"sync", "-device-root", root}); err != nil {
		t.Fatalf("sync command failed: %v", err)
	}
	if err := run([]string{"status", "-device-root", root}); err != nil {
		t.Fatalf("status command failed without config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Podsync", "podsync.toml")); err != nil {
		t.Fatalf("device config was not persisted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Playlists", "news.m3u8")); err != nil {
		t.Fatalf("playlist was not generated: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(root, "Podcasts", "feed-1", "*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("got episode files %v, error %v", files, err)
	}
}

func TestFeedCommandsManageDeviceConfiguration(t *testing.T) {
	root := t.TempDir()
	hostConfig := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Config{Sources: []config.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}}}
	if err := config.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"reconcile", "-device-root", root, "-config", hostConfig}); err != nil {
		t.Fatalf("reconcile command failed: %v", err)
	}
	if err := run([]string{"feed", "add", "-device-root", root, "-id", "sports", "-url", "https://example.com/sports.xml"}); err != nil {
		t.Fatalf("feed add command failed: %v", err)
	}
	updated, err := config.Load(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil || len(updated.Sources) != 2 {
		t.Fatalf("got updated config %+v, error %v", updated, err)
	}
	if err := run([]string{"feed", "remove", "-device-root", root, "-id", "sports"}); err != nil {
		t.Fatalf("feed remove command failed: %v", err)
	}
	updated, err = config.Load(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil || len(updated.Sources) != 1 {
		t.Fatalf("got final config %+v, error %v", updated, err)
	}
}

func TestLoadPlaybackRecordsAllowsMissingTagCache(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".rockbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	records, err := loadPlaybackRecords(device.Layout{Root: root})
	if err != nil {
		t.Fatalf("loadPlaybackRecords() returned error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("got records %+v", records)
	}
}
