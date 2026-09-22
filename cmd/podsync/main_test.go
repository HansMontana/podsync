package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/state"
)

func TestDaemonRunsOncePerDeviceSession(t *testing.T) {
	root := t.TempDir()
	identity, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	current := daemonDevice{root: root, identity: identity}
	checks := 0
	runs := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = daemonLoop(ctx, daemonOptions{pollInterval: time.Millisecond}, func() (daemonDevice, bool, error) {
		checks++
		switch checks {
		case 1, 2, 4:
			return current, true, nil
		case 3:
			return daemonDevice{}, false, nil
		default:
			return daemonDevice{}, false, nil
		}
	}, func(context.Context, updateOptions) error {
		runs++
		if runs == 2 {
			cancel()
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("daemonLoop() returned error: %v", err)
	}
	if runs != 2 {
		t.Fatalf("daemon ran %d times, want one run per device session", runs)
	}
}

func TestDaemonRetriesFailedUpdateBeforeCompletingSession(t *testing.T) {
	root := t.TempDir()
	identity, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	current := daemonDevice{root: root, identity: identity}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0

	err = daemonLoop(ctx, daemonOptions{pollInterval: time.Millisecond}, func() (daemonDevice, bool, error) {
		return current, true, nil
	}, func(context.Context, updateOptions) error {
		runs++
		if runs == 1 {
			return errors.New("transient update failure")
		}
		cancel()
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("daemonLoop() returned error: %v", err)
	}
	if runs != 2 {
		t.Fatalf("daemon ran %d times, want retry followed by success", runs)
	}
}

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
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "news", Title: "News", Source: "news", Order: config.NewestFirst}},
	}
	if err := config.Save(hostConfig, cfg); err != nil {
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
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "news", Title: "News", Source: "news", Order: config.NewestFirst}},
	}
	if err := config.Save(hostConfig, cfg); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"update", "-device-root", root, "-config", hostConfig}); err == nil {
		t.Fatal("update accepted a failed refresh")
	}
	if _, err := os.Stat(filepath.Join(root, "Podcasts")); !os.IsNotExist(err) {
		t.Fatalf("failed update changed device media, error: %v", err)
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
	original := config.Config{Sources: []config.SourceFeed{{ID: "news", URL: server.URL + "/feed.xml"}}, Feeds: []config.LogicalFeed{{ID: "news", Source: "news", Order: config.NewestFirst}}}
	originalPath := filepath.Join(t.TempDir(), "original.toml")
	if err := config.Save(originalPath, original); err != nil {
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
	updated.Sources = append(updated.Sources, config.SourceFeed{ID: "sports", URL: "https://example.com/sports.xml"})
	updatedPath := filepath.Join(t.TempDir(), "updated.toml")
	if err := config.Save(updatedPath, updated); err != nil {
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
	repository, err := state.NewReadOnlySQLiteRepository(filepath.Join(root, "Podsync", "podsync.db"))
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

func TestValidateStagingDirectoryRejectsDevicePaths(t *testing.T) {
	root := t.TempDir()
	if err := validateStagingDirectory(root, filepath.Join(root, "staging")); err == nil {
		t.Fatal("validateStagingDirectory() accepted a device path")
	}
	if err := validateStagingDirectory(root, t.TempDir()); err != nil {
		t.Fatalf("validateStagingDirectory() rejected a host path: %v", err)
	}
}

func TestPrepareStagingDirectoryCleansOwnedFilesAndUsesRunDirectory(t *testing.T) {
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
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale staging file remains, error: %v", err)
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
	records, err := loadPlaybackRecords(device.Layout{Root: root})
	if err != nil {
		t.Fatalf("loadPlaybackRecords() returned error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("got records %+v", records)
	}
}

func TestStatusReportsPlaybackImportErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".rockbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".rockbox", "database_idx.tcd"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"status", "-device-root", root}); err == nil {
		t.Fatal("status accepted incomplete TagCache data")
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
