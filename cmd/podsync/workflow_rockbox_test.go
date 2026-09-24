package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	devicefs "github.com/HansMontana/podsync/internal/adapters/devicefs"
	"github.com/HansMontana/podsync/internal/adapters/tomlconfig"
	"github.com/HansMontana/podsync/internal/domain/curation"
)

func TestSyncRetriesUnavailableRockboxMarkerWithoutFailingSync(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/episode.mp3" {
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("audio"))
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>News</title><link>https://example.com</link><description>News</description><item><title>Episode one</title><guid>one</guid><pubDate>Fri, 02 Jan 2026 00:00:00 GMT</pubDate><enclosure url="%s/episode.mp3" type="audio/mpeg" length="5"/></item></channel></rss>`, server.URL)
	}))
	defer server.Close()
	run := func(args []string) error { return runWithHTTPClient(args, server.Client()) }

	root := t.TempDir()
	hostConfig := filepath.Join(t.TempDir(), "config.toml")
	cfg := curation.Config{
		Sources: []curation.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}},
		Feeds:   []curation.LogicalFeed{{ID: "news", Source: "news", Order: curation.NewestFirst}},
		Integrations: curation.IntegrationConfig{Rockbox: curation.RockboxConfig{
			TagCacheUpdateMarker: true,
		}},
	}
	if err := tomlconfig.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"refresh", "-device-root", root, "-config", hostConfig}); err != nil {
		t.Fatalf("refresh command failed: %v", err)
	}
	if err := run([]string{"sync", "-device-root", root}); err != nil {
		t.Fatalf("sync failed despite unavailable marker directory: %v", err)
	}
	layout := devicefs.Layout{Root: root}
	pending, err := layout.LoadPendingTagCacheUpdate()
	if err != nil || !pending {
		t.Fatalf("pending=%v after unavailable marker, error=%v", pending, err)
	}
	if _, err := tomlconfig.Load(layout.ConfigPath()); err != nil {
		t.Fatalf("device config was not persisted after marker failure: %v", err)
	}
	if err := os.Mkdir(layout.TagCacheDirectory(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sync", "-device-root", root}); err != nil {
		t.Fatalf("sync retry failed: %v", err)
	}
	if _, err := os.Stat(layout.TagCacheUpdateMarkerPath()); err != nil {
		t.Fatalf("TagCache marker was not created on retry: %v", err)
	}
	pending, err = layout.LoadPendingTagCacheUpdate()
	if err != nil || pending {
		t.Fatalf("pending=%v after retry, error=%v", pending, err)
	}
}
