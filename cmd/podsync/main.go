package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/media"
	"github.com/HansMontana/podsync/internal/playback"
	"github.com/HansMontana/podsync/internal/playlists"
	"github.com/HansMontana/podsync/internal/selection"
	"github.com/HansMontana/podsync/internal/state"
	syncer "github.com/HansMontana/podsync/internal/sync"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: podsync <validate-config|reconcile|refresh|status|feed|playlist|briefing|sync> [options]")
	}
	switch args[0] {
	case "validate-config":
		return validateConfig(args[1:])
	case "reconcile":
		return reconcile(args[1:], false)
	case "refresh":
		return reconcile(args[1:], true)
	case "status":
		return status(args[1:])
	case "feed":
		return feedCommand(args[1:])
	case "playlist":
		return generatePlaylist(args[1:], false)
	case "briefing":
		return generatePlaylist(args[1:], true)
	case "sync":
		return sync(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func feedCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: podsync feed <list|add|remove> [options]")
	}
	switch args[0] {
	case "list":
		return listFeeds(args[1:])
	case "add":
		return addFeed(args[1:])
	case "remove":
		return removeFeed(args[1:])
	default:
		return fmt.Errorf("unknown feed command %q", args[0])
	}
}

func listFeeds(args []string) error {
	flags := flag.NewFlagSet("feed list", flag.ContinueOnError)
	deviceRoot := flags.String("device-root", "", "mounted iPod root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	_, repository, err := openRepositoryMode(*deviceRoot, true)
	if err != nil {
		return err
	}
	defer repository.Close()
	current, err := repository.Load()
	if err != nil {
		return err
	}
	for _, known := range current.Feeds {
		fmt.Printf("%d\t%s\t%s\n", known.ID, known.Name, known.URL)
	}
	return nil
}

func addFeed(args []string) error {
	flags := flag.NewFlagSet("feed add", flag.ContinueOnError)
	deviceRoot := flags.String("device-root", "", "mounted iPod root")
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	id := flags.String("id", "", "source feed ID")
	feedURL := flags.String("url", "", "source feed URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == "" || *feedURL == "" {
		return fmt.Errorf("-id and -url are required")
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath, false)
	if err != nil {
		return err
	}
	defer repository.Close()
	cfg.Sources = append(cfg.Sources, config.SourceFeed{ID: *id, URL: *feedURL})
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := config.Save(layout.ConfigPath(), cfg); err != nil {
		return fmt.Errorf("save device config: %w", err)
	}
	if err := reconcileState(repository, cfg); err != nil {
		return err
	}
	fmt.Printf("added source %s\n", *id)
	return nil
}

func removeFeed(args []string) error {
	flags := flag.NewFlagSet("feed remove", flag.ContinueOnError)
	deviceRoot := flags.String("device-root", "", "mounted iPod root")
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	id := flags.String("id", "", "source feed ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("-id is required")
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath, false)
	if err != nil {
		return err
	}
	defer repository.Close()
	for _, logical := range cfg.Feeds {
		if logical.Source == *id {
			return fmt.Errorf("source %q is used by logical feed %q", *id, logical.ID)
		}
	}
	updated := cfg.Sources[:0]
	found := false
	for _, source := range cfg.Sources {
		if source.ID == *id {
			found = true
			continue
		}
		updated = append(updated, source)
	}
	if !found {
		return fmt.Errorf("source feed %q not found", *id)
	}
	cfg.Sources = updated
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := config.Save(layout.ConfigPath(), cfg); err != nil {
		return fmt.Errorf("save device config: %w", err)
	}
	fmt.Printf("removed source %s\n", *id)
	return nil
}

func validateConfig(args []string) error {
	flags := flag.NewFlagSet("validate-config", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("-config is required")
	}
	if _, err := config.Load(*configPath); err != nil {
		return err
	}
	fmt.Println("configuration is valid")
	return nil
}

func reconcile(args []string, refresh bool) error {
	flags := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	deviceRoot := flags.String("device-root", "", "mounted iPod root")
	configPath := flags.String("config", "", "path to podsync TOML configuration (defaults to device config)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath, false)
	if err != nil {
		return err
	}
	defer repository.Close()
	if err := reconcileState(repository, cfg); err != nil {
		return err
	}
	if !refresh {
		fmt.Printf("reconciled %d source feeds\n", len(cfg.Sources))
		return nil
	}
	for _, source := range cfg.Sources {
		feedID, err := sourceID(repository, source)
		if err != nil {
			return err
		}
		if err := syncer.RefreshFeed(context.Background(), repository, http.DefaultClient, feedID); err != nil {
			return fmt.Errorf("refresh source %q: %w", source.ID, err)
		}
		fmt.Printf("refreshed %s\n", source.ID)
	}
	_ = layout
	return nil
}

func status(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	deviceRoot := flags.String("device-root", "", "mounted iPod root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	layout, repository, err := openRepositoryMode(*deviceRoot, false)
	if err != nil {
		return err
	}
	defer repository.Close()
	current, err := repository.Load()
	if err != nil {
		return err
	}
	fmt.Printf("feeds: %d\nepisodes: %d\n", len(current.Feeds), len(current.Episodes))
	records, err := loadPlaybackRecords(layout)
	if err != nil {
		return fmt.Errorf("load playback state: %w", err)
	}
	fmt.Printf("playback records: %d\n", len(records))
	states := playback.ForEpisodes(current.Episodes, records)
	played := 0
	for _, state := range states {
		if state.Played() {
			played++
		}
	}
	fmt.Printf("played: %d\n", played)
	return nil
}

func generatePlaylist(args []string, briefingMode bool) error {
	name := "playlist"
	if briefingMode {
		name = "briefing"
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	deviceRoot := flags.String("device-root", "", "mounted iPod root")
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	id := flags.String("id", "", "logical feed or briefing ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("-id is required")
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath, false)
	if err != nil {
		return err
	}
	defer repository.Close()
	current, err := repository.Load()
	if err != nil {
		return err
	}
	records, err := loadPlaybackRecords(layout)
	if err != nil {
		return err
	}
	playbackStates := playback.ForEpisodes(current.Episodes, records)
	var content []byte
	if briefingMode {
		content, err = playlists.Briefing(cfg, current, *id, playbackStates)
	} else {
		content, err = playlists.LogicalFeed(cfg, current, *id, playbackStates)
	}
	if err != nil {
		return err
	}
	relative := path.Join("Playlists", *id+".m3u8")
	managed, err := layout.LoadManagedPaths()
	if err != nil {
		return err
	}
	managed = appendUnique(managed, relative)
	manifest := syncer.PlaylistFile{Relative: layout.ManifestRelativePath(), Content: manifestContent(managed)}
	if err := syncer.ApplyFilePlan(context.Background(), layout.Root, syncer.FilePlan{Playlists: []syncer.PlaylistFile{{Relative: relative, Content: content}, manifest}}); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", relative)
	return nil
}

func sync(args []string) error {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	deviceRoot := flags.String("device-root", "", "mounted iPod root")
	configPath := flags.String("config", "", "path to podsync TOML configuration (defaults to device config)")
	stagingDir := flags.String("staging", "", "host-side staging directory (defaults to a temporary directory)")
	dryRun := flags.Bool("dry-run", false, "show the sync plan without downloading or changing the device")
	if err := flags.Parse(args); err != nil {
		return err
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath, *dryRun)
	if err != nil {
		return err
	}
	defer repository.Close()
	var current state.State
	if *dryRun {
		current, err = repository.Load()
	} else {
		if err := reconcileState(repository, cfg); err != nil {
			return err
		}
		current, err = repository.Load()
	}
	if err != nil {
		return err
	}
	records, err := loadPlaybackRecords(layout)
	if err != nil {
		return err
	}
	states := playback.ForEpisodes(current.Episodes, records)
	selected := make(map[string]episode.Episode)
	var playlistFiles []syncer.PlaylistFile
	var managed []string
	for _, logical := range cfg.Feeds {
		episodes, selectErr := selection.Feed(cfg, current, logical.ID, states, false)
		if selectErr != nil {
			return selectErr
		}
		for _, currentEpisode := range episodes {
			selected[currentEpisode.IdentityKey()] = currentEpisode
		}
		content, generateErr := playlists.LogicalFeed(cfg, current, logical.ID, states)
		if generateErr != nil {
			return generateErr
		}
		playlistFiles = append(playlistFiles, syncer.PlaylistFile{Relative: path.Join("Playlists", logical.ID+".m3u8"), Content: content})
	}
	for _, configured := range cfg.Briefings {
		content, generateErr := playlists.Briefing(cfg, current, configured.ID, states)
		if generateErr != nil {
			return generateErr
		}
		playlistFiles = append(playlistFiles, syncer.PlaylistFile{Relative: path.Join("Playlists", configured.ID+".m3u8"), Content: content})
	}
	managed, err = layout.LoadManagedPaths()
	if err != nil {
		return err
	}
	if *stagingDir == "" {
		*stagingDir, err = os.MkdirTemp("", "podsync-staging-")
		if err != nil {
			return fmt.Errorf("create staging directory: %w", err)
		}
		defer os.RemoveAll(*stagingDir)
	}
	episodes := make([]episode.Episode, 0, len(selected))
	for _, currentEpisode := range selected {
		episodes = append(episodes, currentEpisode)
	}
	newManaged := make([]string, 0, len(episodes)+len(playlistFiles))
	for _, currentEpisode := range episodes {
		newManaged = append(newManaged, media.RelativePath(currentEpisode))
	}
	for _, playlist := range playlistFiles {
		newManaged = append(newManaged, playlist.Relative)
	}
	manifest := syncer.PlaylistFile{Relative: layout.ManifestRelativePath(), Content: manifestContent(newManaged)}
	playlistFiles = append(playlistFiles, manifest)
	if *dryRun {
		copies := make([]syncer.FileCopy, 0, len(episodes))
		for _, currentEpisode := range episodes {
			copies = append(copies, syncer.FileCopy{Relative: media.RelativePath(currentEpisode)})
		}
		plan, err := syncer.BuildFilePlan(managed, copies, playlistFiles, nil)
		if err != nil {
			return err
		}
		fmt.Printf("would sync %d episodes and %d playlists\n", len(episodes), len(playlistFiles)-1)
		fmt.Printf("would delete %d managed files\n", len(plan.Deletes))
		return nil
	}
	if err := syncer.Episodes(context.Background(), http.DefaultClient, *stagingDir, layout.Root, episodes, playlistFiles, managed); err != nil {
		return err
	}
	fmt.Printf("synced %d episodes and %d playlists\n", len(episodes), len(playlistFiles)-1)
	return nil
}

func openDevice(root, configPath string, readOnly bool) (device.Layout, state.Repository, config.Config, error) {
	layout, repository, err := openRepositoryMode(root, readOnly)
	if err != nil {
		return layout, nil, config.Config{}, err
	}
	if configPath == "" {
		configPath = layout.ConfigPath()
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		_ = repository.Close()
		return layout, nil, config.Config{}, err
	}
	if !readOnly && configPath != layout.ConfigPath() {
		if err := config.Save(layout.ConfigPath(), cfg); err != nil {
			_ = repository.Close()
			return layout, nil, config.Config{}, fmt.Errorf("save device config: %w", err)
		}
	}
	return layout, repository, cfg, nil
}

func openRepositoryMode(root string, readOnly bool) (device.Layout, state.Repository, error) {
	if root == "" {
		return device.Layout{}, nil, fmt.Errorf("-device-root is required")
	}
	layout := device.Layout{Root: root}
	if !readOnly {
		if err := os.MkdirAll(layout.StateDirectory(), 0o755); err != nil {
			return layout, nil, fmt.Errorf("create device state directory: %w", err)
		}
	}
	var repository state.Repository
	var err error
	if readOnly {
		repository, err = state.NewReadOnlySQLiteRepository(layout.DatabasePath())
	} else {
		repository, err = state.NewSQLiteRepository(layout.DatabasePath())
	}
	if err != nil {
		return layout, nil, err
	}
	return layout, repository, nil
}

func reconcileState(repository state.Repository, cfg config.Config) error {
	current, err := repository.Load()
	if err != nil {
		return err
	}
	reconciled, err := cfg.EnsureSources(current)
	if err != nil {
		return err
	}
	if err := repository.Save(reconciled); err != nil {
		return fmt.Errorf("save configured sources: %w", err)
	}
	return nil
}

func sourceID(repository state.Repository, source config.SourceFeed) (int64, error) {
	current, err := repository.Load()
	if err != nil {
		return 0, err
	}
	normalized, err := feed.NormalizeURL(source.URL)
	if err != nil {
		return 0, fmt.Errorf("normalize source %q: %w", source.ID, err)
	}
	for _, known := range current.Feeds {
		knownURL, normalizeErr := feed.NormalizeURL(known.URL)
		if normalizeErr == nil && knownURL == normalized {
			return known.ID, nil
		}
	}
	return 0, fmt.Errorf("source %q was not reconciled", source.ID)
}

func appendUnique(paths []string, value string) []string {
	for _, current := range paths {
		if current == value {
			return paths
		}
	}
	return append(paths, value)
}

func manifestContent(paths []string) []byte {
	var content bytes.Buffer
	for _, current := range paths {
		_, _ = content.WriteString(current)
		_, _ = content.WriteString("\n")
	}
	return content.Bytes()
}

func loadPlaybackRecords(layout device.Layout) ([]playback.Record, error) {
	paths, err := layout.PlaybackLogPaths()
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var records []playback.Record
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open playback log %q: %w", path, err)
		}
		parsed, parseErr := playback.ParseLog(file)
		closeErr := file.Close()
		if parseErr != nil {
			return nil, fmt.Errorf("parse playback log %q: %w", path, parseErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close playback log %q: %w", path, closeErr)
		}
		records = append(records, parsed...)
	}
	masterPath := filepath.Join(layout.TagCacheDirectory(), "database_idx.tcd")
	filenamePath := filepath.Join(layout.TagCacheDirectory(), "database_4.tcd")
	masterExists, err := fileExists(masterPath)
	if err != nil {
		return nil, fmt.Errorf("inspect TagCache master: %w", err)
	}
	filenameExists, err := fileExists(filenamePath)
	if err != nil {
		return nil, fmt.Errorf("inspect TagCache filename index: %w", err)
	}
	if masterExists != filenameExists {
		return nil, fmt.Errorf("incomplete TagCache: master=%t filename-index=%t", masterExists, filenameExists)
	}
	if masterExists {
		parsed, err := playback.ParseTagCache(layout.TagCacheDirectory())
		if err != nil {
			return nil, fmt.Errorf("parse TagCache: %w", err)
		}
		return playback.PreferRecords(parsed, playback.MergeRecords(records)), nil
	}
	return playback.MergeRecords(records), nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
