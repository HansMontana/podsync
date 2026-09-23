package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	devicefs "github.com/HansMontana/podsync/internal/adapters/devicefs"
	"github.com/HansMontana/podsync/internal/adapters/sqlitecatalog"
	"github.com/HansMontana/podsync/internal/adapters/tomlconfig"
	"github.com/HansMontana/podsync/internal/domain/curation"
)

func TestSaveConfigThenStateDoesNotWriteStateAfterConfigFailure(t *testing.T) {
	expected := errors.New("config failed")
	stateCalled := false
	if err := saveConfigThenState(
		func() error { return expected },
		func() error {
			stateCalled = true
			return nil
		},
	); !errors.Is(err, expected) {
		t.Fatalf("got error %v, want %v", err, expected)
	}
	if stateCalled {
		t.Fatal("state saver ran after configuration failure")
	}
}

func TestSaveConfigThenStateWritesStateAfterConfigSuccess(t *testing.T) {
	var order []string
	if err := saveConfigThenState(
		func() error {
			order = append(order, "config")
			return nil
		},
		func() error {
			order = append(order, "state")
			return errors.New("state failed")
		},
	); err == nil {
		t.Fatal("saveConfigThenState() accepted a state failure")
	}
	if strings.Join(order, ",") != "config,state" {
		t.Fatalf("call order was %v", order)
	}
}

func TestSaveStateThenConfigRestoresStateAfterConfigFailure(t *testing.T) {
	var order []string
	err := saveStateThenConfig(
		func() error {
			order = append(order, "state")
			return nil
		},
		func() error {
			order = append(order, "config")
			return errors.New("config failed")
		},
		func() error {
			order = append(order, "restore")
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "state restored") {
		t.Fatalf("got error %v", err)
	}
	if strings.Join(order, ",") != "state,config,restore" {
		t.Fatalf("call order was %v", order)
	}
}

func TestSaveStateThenConfigReportsRestoreFailure(t *testing.T) {
	err := saveStateThenConfig(
		func() error { return nil },
		func() error { return errors.New("config failed") },
		func() error { return errors.New("restore failed") },
	)
	if err == nil || !strings.Contains(err.Error(), "restore state") {
		t.Fatalf("got error %v", err)
	}
}

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
	cfg := curation.Config{
		Sources: []curation.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}},
		Feeds:   []curation.LogicalFeed{{ID: "news", Source: "news", Order: curation.NewestFirst}},
	}
	if err := tomlconfig.Save(hostConfig, cfg); err != nil {
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
	if files, err := filepath.Glob(filepath.Join(root, "Podcasts", "news", "*")); err != nil || len(files) != 0 {
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
	files, err := filepath.Glob(filepath.Join(root, "Podcasts", "news", "*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("got episode files %v, error %v", files, err)
	}
}

func TestUpdateCommandRunsDeepVerificationByDefaultAndSupportsOptOut(t *testing.T) {
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
	cfg := curation.Config{
		Sources: []curation.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}},
		Feeds:   []curation.LogicalFeed{{ID: "news", Title: "News", Source: "news", Order: curation.NewestFirst}},
	}
	if err := tomlconfig.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"update", "-device-root", root, "-config", hostConfig}); err != nil {
		t.Fatalf("default update failed: %v", err)
	}
	if err := run([]string{"update", "-device-root", root}); err != nil {
		t.Fatalf("default deep update failed: %v", err)
	}
	if err := run([]string{"update", "-device-root", root, "-skip-verify-media"}); err != nil {
		t.Fatalf("opt-out update failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Podsync", "managed-files.txt")); err != nil {
		t.Fatalf("update did not finalize the manifest: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(root, "Podcasts", "news", "*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("got episode files %v, error %v", files, err)
	}
}

func TestUpdateStopsWhenRefreshFails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	root := t.TempDir()
	hostConfig := filepath.Join(t.TempDir(), "config.toml")
	cfg := curation.Config{
		Sources: []curation.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}},
		Feeds:   []curation.LogicalFeed{{ID: "news", Title: "News", Source: "news", Order: curation.NewestFirst}},
	}
	if err := tomlconfig.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"update", "-device-root", root, "-config", hostConfig}); err == nil {
		t.Fatal("update accepted a failed refresh")
	}
	if _, err := os.Stat(filepath.Join(root, "Podcasts")); !os.IsNotExist(err) {
		t.Fatalf("failed update changed device media, error: %v", err)
	}
}

func TestUpdateSyncsPartialRefreshAndReturnsFailure(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/broken.xml" {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/episode.mp3" {
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("audio"))
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>News</title><item><title>Episode one</title><guid>one</guid><enclosure url="%s/episode.mp3" type="audio/mpeg" length="5"/></item></channel></rss>`, server.URL)
	}))
	defer server.Close()

	root := t.TempDir()
	hostConfig := filepath.Join(t.TempDir(), "config.toml")
	cfg := curation.Config{
		Sources: []curation.SourceFeed{
			{ID: "news", URL: server.URL + "/news.xml"},
			{ID: "broken", URL: server.URL + "/broken.xml"},
		},
		Feeds: []curation.LogicalFeed{{ID: "news", Title: "News", Source: "news", Order: curation.NewestFirst}},
	}
	if err := tomlconfig.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}

	err := run([]string{"update", "-device-root", root, "-config", hostConfig})
	if err == nil {
		t.Fatal("partial update returned success")
	}
	if !strings.Contains(err.Error(), "refresh feed") {
		t.Fatalf("partial update returned unrelated error: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(root, "Podcasts", "news", "*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("partial update did not sync successful feed: %v, error %v", files, err)
	}
	playlists, err := filepath.Glob(filepath.Join(root, "Playlists", "*.m3u8"))
	if err != nil || len(playlists) != 1 {
		t.Fatalf("partial update did not write playlist: %v, error %v", playlists, err)
	}
}

func TestFeedCommandsManageDeviceConfiguration(t *testing.T) {
	root := t.TempDir()
	hostConfig := filepath.Join(t.TempDir(), "config.toml")
	cfg := curation.Config{Sources: []curation.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}}}
	if err := tomlconfig.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"reconcile", "-device-root", root, "-config", hostConfig}); err != nil {
		t.Fatalf("reconcile command failed: %v", err)
	}
	if err := run([]string{"feed", "add", "-device-root", root, "-id", "sports", "-url", "https://example.com/sports.xml"}); err != nil {
		t.Fatalf("feed add command failed: %v", err)
	}
	updated, err := tomlconfig.Load(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil || len(updated.Sources) != 2 {
		t.Fatalf("got updated config %+v, error %v", updated, err)
	}
	if err := run([]string{"feed", "remove", "-device-root", root, "-id", "sports"}); err != nil {
		t.Fatalf("feed remove command failed: %v", err)
	}
	updated, err = tomlconfig.Load(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil || len(updated.Sources) != 1 {
		t.Fatalf("got final config %+v, error %v", updated, err)
	}
}

func TestStandalonePlaylistDoesNotPromoteUnrelatedPendingOwnership(t *testing.T) {
	root := t.TempDir()
	hostConfig := filepath.Join(t.TempDir(), "config.toml")
	cfg := curation.Config{
		Sources: []curation.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []curation.LogicalFeed{{ID: "news", Source: "news", Order: curation.NewestFirst}},
	}
	if err := tomlconfig.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"reconcile", "-device-root", root, "-config", hostConfig}); err != nil {
		t.Fatalf("reconcile command failed: %v", err)
	}
	layout := devicefs.Layout{Root: root}
	if err := layout.SavePendingManagedPaths([]string{"Podcasts/old/episode.mp3"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"playlist", "-device-root", root, "-id", "news"}); err != nil {
		t.Fatalf("playlist command failed: %v", err)
	}
	pending, err := layout.LoadPendingManagedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0] != "Podcasts/old/episode.mp3" {
		t.Fatalf("got pending ownership %v, want unrelated path preserved", pending)
	}
	managed, err := layout.LoadCommittedManagedPaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range managed {
		if path == "Podcasts/old/episode.mp3" {
			t.Fatal("standalone playlist promoted unrelated pending path")
		}
	}
}

func TestSyncFailureDoesNotPersistSuppliedConfiguration(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/episode.mp3" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>News</title><link>https://example.com</link><description>News</description><item><title>Episode one</title><guid>one</guid><pubDate>Fri, 02 Jan 2026 00:00:00 GMT</pubDate><enclosure url="%s/episode.mp3" type="audio/mpeg" length="5"/></item></channel></rss>`, server.URL)
	}))
	defer server.Close()

	root := t.TempDir()
	original := curation.Config{Sources: []curation.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}}, Feeds: []curation.LogicalFeed{{ID: "news", Source: "news", Order: curation.NewestFirst}}}
	originalPath := filepath.Join(t.TempDir(), "original.toml")
	if err := tomlconfig.Save(originalPath, original); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"refresh", "-device-root", root, "-config", originalPath}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil {
		t.Fatal(err)
	}
	updated := original
	updated.Sources = append(updated.Sources, curation.SourceFeed{ID: "sports", URL: "https://example.com/sports.xml"})
	updatedPath := filepath.Join(t.TempDir(), "updated.toml")
	if err := tomlconfig.Save(updatedPath, updated); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sync", "-device-root", root, "-config", updatedPath}); err == nil {
		t.Fatal("sync accepted a failed download")
	}
	after, err := os.ReadFile(filepath.Join(root, "Podsync", "podsync.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed sync persisted the supplied configuration")
	}
	repository, err := sqlitecatalog.NewReadOnlySQLiteRepository(filepath.Join(root, "Podsync", "podsync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	current, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Feeds) != 1 {
		t.Fatalf("failed sync persisted reconciled state: %+v", current.Feeds)
	}
}
