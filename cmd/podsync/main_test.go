package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/HansMontana/podsync/internal/config"
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
	if err := run([]string{"sync", "-device-root", root, "-dry-run"}); err != nil {
		t.Fatalf("dry-run command failed: %v", err)
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
