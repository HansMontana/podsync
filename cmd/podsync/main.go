package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HansMontana/podsync/internal/briefing"
	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/logging"
	"github.com/HansMontana/podsync/internal/media"
	"github.com/HansMontana/podsync/internal/playback"
	"github.com/HansMontana/podsync/internal/playlists"
	"github.com/HansMontana/podsync/internal/selection"
	"github.com/HansMontana/podsync/internal/state"
	syncer "github.com/HansMontana/podsync/internal/sync"
)

var httpClient = &http.Client{Timeout: 10 * time.Minute}

func main() {
	logger := commandLogger("cli")
	started := time.Now()
	command := "help"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	logger.Info("Starting podsync command " + command)
	if err := run(os.Args[1:]); err != nil {
		logger.Error("Command failed: " + err.Error())
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Completed command %s in %s", command, time.Since(started).Round(time.Millisecond)))
}

func commandLogger(component string) *logging.Logger {
	return logging.New(os.Stderr).WithComponent(component)
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return nil
	}
	if args[0] == "--help" || args[0] == "-h" {
		printUsage(os.Stdout)
		return nil
	}
	if args[0] == "help" {
		if len(args) == 1 {
			printUsage(os.Stdout)
			return nil
		}
		return printCommandHelp(args[1])
	}
	switch args[0] {
	case "validate-config":
		return validateConfig(args[1:])
	case "reconcile":
		return reconcile(args[1:], false)
	case "refresh":
		return reconcile(args[1:], true)
	case "update":
		return update(args[1:])
	case "status":
		return status(args[1:])
	case "verify":
		return verify(args[1:])
	case "feed":
		return feedCommand(args[1:])
	case "playlist":
		return generatePlaylist(args[1:], false)
	case "briefing":
		return generatePlaylist(args[1:], true)
	case "sync":
		return sync(args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func usageText() string {
	return `Usage: podsync <command> [options]

Commands:
  validate-config  Validate a TOML configuration file.
  reconcile        Add configured source feeds to device state.
  refresh          Reconcile and refresh all configured source feeds.
  update           Refresh feeds, sync selected media, and verify the device.
  status           Show device state and playback summary.
  verify           Verify manifest-managed device files without changing them.
  feed             List, add, or remove source feeds.
  playlist         Generate one logical-feed playlist.
  briefing         Generate one briefing playlist.
  sync             Download selected audio and apply device files.

Use "podsync help <command>" or "podsync <command> --help" for details.`
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, usageText())
}

func printCommandHelp(command string) error {
	var text string
	switch command {
	case "validate-config":
		text = "Usage: podsync validate-config -config PATH\n\nValidate configuration without changing a device."
	case "reconcile":
		text = "Usage: podsync reconcile [-device-root PATH] [-config PATH]\n\nReconcile configured source feeds into the device database."
	case "refresh":
		text = "Usage: podsync refresh [-device-root PATH] [-config PATH]\n\nReconcile and refresh all configured source feeds."
	case "update":
		text = "Usage: podsync update [-device-root PATH] [-config PATH] [-staging PATH] [-verify-media]\n\nRefresh feeds, sync selected media, and verify the device."
	case "status":
		text = "Usage: podsync status [-device-root PATH]\n\nShow feed, episode, and playback counts."
	case "verify":
		text = "Usage: podsync verify [-device-root PATH]\n\nVerify manifest-managed device files without changing them."
	case "feed":
		text = "Usage: podsync feed <list|add|remove> [options]\n\nManage source feeds in device-local configuration."
	case "playlist":
		text = "Usage: podsync playlist [-device-root PATH] -id FEED [-config PATH]\n\nGenerate one logical-feed playlist."
	case "briefing":
		text = "Usage: podsync briefing [-device-root PATH] -id BRIEFING [-config PATH]\n\nGenerate one briefing playlist."
	case "sync":
		text = "Usage: podsync sync [-device-root PATH] [-config PATH] [-staging PATH] [-dry-run] [-verify-media]\n\nApply selected audio, playlists, and managed-file cleanup."
	default:
		return fmt.Errorf("unknown help topic %q\n\n%s", command, usageText())
	}
	fmt.Println(text)
	return nil
}

func newFlagSet(name, usage string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "%s\n\nOptions:\n", usage)
		flags.PrintDefaults()
	}
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) (bool, error) {
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return true, nil
	}
	return false, err
}

func feedCommand(args []string) error {
	if len(args) == 0 {
		return printCommandHelp("feed")
	}
	if args[0] == "--help" || args[0] == "-h" {
		return printCommandHelp("feed")
	}
	switch args[0] {
	case "list":
		return listFeeds(args[1:])
	case "add":
		return addFeed(args[1:])
	case "remove":
		return removeFeed(args[1:])
	default:
		return fmt.Errorf("unknown feed command %q\n\n%s", args[0], usageText())
	}
}

func listFeeds(args []string) error {
	flags := newFlagSet("feed list", "Usage: podsync feed list -device-root PATH")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
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
	logger := commandLogger("feed")
	for _, known := range current.Feeds {
		logger.Info(fmt.Sprintf("%d\t%s\t%s", known.ID, known.Name, known.URL))
	}
	return nil
}

func addFeed(args []string) error {
	flags := newFlagSet("feed add", "Usage: podsync feed add -device-root PATH -id ID -url URL [-config PATH]")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	id := flags.String("id", "", "source feed ID")
	feedURL := flags.String("url", "", "source feed URL")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	if *id == "" || *feedURL == "" {
		return fmt.Errorf("-id and -url are required")
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath, false, true)
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
	commandLogger("feed").Info(fmt.Sprintf("Added source %s", *id))
	return nil
}

func removeFeed(args []string) error {
	flags := newFlagSet("feed remove", "Usage: podsync feed remove -device-root PATH -id ID [-config PATH]")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	id := flags.String("id", "", "source feed ID")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	if *id == "" {
		return fmt.Errorf("-id is required")
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath, false, true)
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
	removedURL := ""
	for _, source := range cfg.Sources {
		if source.ID == *id {
			found = true
			removedURL = source.URL
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
	current, err := repository.Load()
	if err != nil {
		return err
	}
	remaining := current.Feeds[:0]
	removedIDs := make(map[int64]struct{})
	for _, known := range current.Feeds {
		if known.SameIdentity(feed.Feed{URL: removedURL}) {
			removedIDs[known.ID] = struct{}{}
			continue
		}
		remaining = append(remaining, known)
	}
	current.Feeds = remaining
	episodes := current.Episodes[:0]
	for _, currentEpisode := range current.Episodes {
		if _, removed := removedIDs[currentEpisode.FeedID]; !removed {
			episodes = append(episodes, currentEpisode)
		}
	}
	current.Episodes = episodes
	if err := config.Save(layout.ConfigPath(), cfg); err != nil {
		return fmt.Errorf("save device config: %w", err)
	}
	if err := repository.Save(current); err != nil {
		return fmt.Errorf("remove source state after config save: %w", err)
	}
	commandLogger("feed").Info(fmt.Sprintf("Removed source %s", *id))
	return nil
}

func validateConfig(args []string) error {
	flags := newFlagSet("validate-config", "Usage: podsync validate-config -config PATH")
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	if *configPath == "" {
		return fmt.Errorf("-config is required")
	}
	if _, err := config.Load(*configPath); err != nil {
		return err
	}
	commandLogger("config").Info("Configuration is valid")
	return nil
}

func reconcile(args []string, refresh bool) error {
	command := "reconcile"
	usage := "Usage: podsync reconcile -device-root PATH [-config PATH]"
	if refresh {
		command = "refresh"
		usage = "Usage: podsync refresh -device-root PATH [-config PATH]"
	}
	flags := newFlagSet(command, usage)
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration (defaults to device config)")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	return reconcileDevice(*deviceRoot, *configPath, refresh)
}

func reconcileDevice(deviceRoot, configPath string, refresh bool) error {
	logger := commandLogger("feed")
	layout, repository, cfg, err := openDevice(deviceRoot, configPath, false, true)
	if err != nil {
		return err
	}
	defer repository.Close()
	if err := reconcileState(repository, cfg); err != nil {
		return err
	}
	if !refresh {
		logger.Info(fmt.Sprintf("Reconciled %d source feeds", len(cfg.Sources)))
		return nil
	}
	for _, source := range cfg.Sources {
		feedID, err := sourceID(repository, source)
		if err != nil {
			return err
		}
		archive := false
		for _, logical := range cfg.Feeds {
			if logical.Source == source.ID && logical.Archive {
				archive = true
				break
			}
		}
		if err := syncer.RefreshFeedWithArchive(context.Background(), repository, httpClient, feedID, archive); err != nil {
			return fmt.Errorf("refresh source %q: %w", source.ID, err)
		}
		logger.Info(fmt.Sprintf("Refreshed source %s", source.ID))
	}
	_ = layout
	return nil
}

func update(args []string) error {
	flags := newFlagSet("update", "Usage: podsync update [-device-root PATH] [-config PATH] [-staging PATH] [-verify-media]")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration (defaults to device config)")
	stagingDir := flags.String("staging", "", "host-side staging directory (defaults to a temporary directory)")
	verifyMedia := flags.Bool("verify-media", false, "inspect existing MP3 metadata and repair missing fields")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}

	logger := logging.New(os.Stderr).WithComponent("update")
	started := time.Now()
	logger.Info("Starting update")
	resolvedRoot, identity, err := device.ResolveRootIdentity(*deviceRoot)
	if err != nil {
		return err
	}
	if err := reconcileDevice(resolvedRoot, *configPath, true); err != nil {
		return fmt.Errorf("update refresh: %w", err)
	}
	logger.Info("Refresh complete")
	if err := device.VerifyRootIdentity(resolvedRoot, identity); err != nil {
		return fmt.Errorf("before update sync: %w", err)
	}
	if err := syncDevice(syncOptions{deviceRoot: resolvedRoot, configPath: *configPath, stagingDir: *stagingDir, verifyMedia: *verifyMedia}); err != nil {
		return fmt.Errorf("update sync: %w", err)
	}
	logger.Info("Sync complete")
	if err := device.VerifyRootIdentity(resolvedRoot, identity); err != nil {
		return fmt.Errorf("before update verification: %w", err)
	}
	verified, err := verifyDevice(resolvedRoot)
	if err != nil {
		return fmt.Errorf("update verification: %w", err)
	}
	logger.Info(fmt.Sprintf("Device verification complete: %d managed files", verified))
	logger.Info(fmt.Sprintf("Update complete in %s", time.Since(started).Round(time.Millisecond)))
	return nil
}

func status(args []string) error {
	flags := newFlagSet("status", "Usage: podsync status -device-root PATH")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, "", true, false)
	if err != nil {
		return err
	}
	defer repository.Close()
	logger := commandLogger("status")
	current, err := repository.Load()
	if err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Feeds: %d\tEpisodes: %d", len(current.Feeds), len(current.Episodes)))
	records, err := loadPlaybackRecords(layout)
	if err != nil {
		return fmt.Errorf("load playback state: %w", err)
	}
	logger.Info(fmt.Sprintf("Playback records: %d", len(records)))
	logicalIDs, err := cfg.LogicalFeedIDs(current)
	if err != nil {
		return err
	}
	states := playback.ForEpisodesWithResolver(current.Episodes, records, media.Resolver(logicalIDs))
	played := 0
	for _, state := range states {
		if state.Played() {
			played++
		}
	}
	logger.Info(fmt.Sprintf("Played: %d", played))
	return nil
}

func verify(args []string) error {
	flags := newFlagSet("verify", "Usage: podsync verify [-device-root PATH]")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	verified, err := verifyDevice(*deviceRoot)
	if err != nil {
		return err
	}
	commandLogger("verify").Info(fmt.Sprintf("Verified %d managed files", verified))
	return nil
}

func verifyDevice(deviceRoot string) (int, error) {
	layout, repository, err := openRepositoryMode(deviceRoot, true)
	if err != nil {
		return 0, err
	}
	defer repository.Close()
	managed, err := layout.LoadManagedPaths()
	if err != nil {
		return 0, err
	}
	if err := syncer.VerifyManagedFiles(layout.Root, managed); err != nil {
		return 0, err
	}
	return len(managed), nil
}

func generatePlaylist(args []string, briefingMode bool) error {
	name := "playlist"
	if briefingMode {
		name = "briefing"
	}
	flags := newFlagSet(name, fmt.Sprintf("Usage: podsync %s -device-root PATH -id ID [-config PATH]", name))
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	id := flags.String("id", "", "logical feed or briefing ID")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	if *id == "" {
		return fmt.Errorf("-id is required")
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath, false, true)
	if err != nil {
		return err
	}
	defer repository.Close()
	current, err := repository.Load()
	if err != nil {
		return err
	}
	logicalIDs, err := cfg.LogicalFeedIDs(current)
	if err != nil {
		return err
	}
	resolver := media.Resolver(logicalIDs)
	records, err := loadPlaybackRecords(layout)
	if err != nil {
		return err
	}
	playbackStates := playback.ForEpisodesWithResolver(current.Episodes, records, resolver)
	var content []byte
	if briefingMode {
		content, err = playlists.BriefingWithResolver(cfg, current, *id, playbackStates, resolver)
	} else {
		content, err = playlists.LogicalFeedWithResolver(cfg, current, *id, playbackStates, resolver)
	}
	if err != nil {
		return err
	}
	title := *id
	if briefingMode {
		for _, configured := range cfg.Briefings {
			if configured.ID == *id {
				title = configured.Title
				break
			}
		}
	} else {
		for _, configured := range cfg.Feeds {
			if configured.ID == *id {
				title = configured.Title
				break
			}
		}
	}
	relative := path.Join("Playlists", playlists.Filename(title, *id)+".m3u8")
	managed, err := layout.LoadManagedPaths()
	if err != nil {
		return err
	}
	managed = appendUnique(managed, relative)
	manifest := syncer.PlaylistFile{Relative: layout.ManifestRelativePath(), Content: manifestContent(managed)}
	if err := syncer.ApplyFilePlan(context.Background(), layout.Root, syncer.FilePlan{Playlists: []syncer.PlaylistFile{{Relative: relative, Content: content}, manifest}}); err != nil {
		return err
	}
	commandLogger("playlist").Info(fmt.Sprintf("Wrote %s", relative))
	return nil
}

type syncOptions struct {
	deviceRoot  string
	configPath  string
	stagingDir  string
	dryRun      bool
	verifyMedia bool
}

func sync(args []string) error {
	flags := newFlagSet("sync", "Usage: podsync sync -device-root PATH [-config PATH] [-staging PATH] [-dry-run] [-verify-media]")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration (defaults to device config)")
	stagingDir := flags.String("staging", "", "host-side staging directory (defaults to a temporary directory)")
	dryRun := flags.Bool("dry-run", false, "show the sync plan without downloading or changing the device")
	verifyMedia := flags.Bool("verify-media", false, "inspect existing MP3 metadata and repair missing fields")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	return syncDevice(syncOptions{deviceRoot: *deviceRoot, configPath: *configPath, stagingDir: *stagingDir, dryRun: *dryRun, verifyMedia: *verifyMedia})
}

func syncDevice(options syncOptions) error {
	deviceRoot := options.deviceRoot
	configPath := options.configPath
	stagingDir := options.stagingDir
	dryRun := options.dryRun
	verifyMedia := options.verifyMedia
	configProvided := configPath != ""
	logger := logging.New(os.Stderr).WithComponent("sync")
	logger.Info("Starting sync")
	layout, repository, cfg, err := openDevice(deviceRoot, configPath, dryRun, false)
	if err != nil {
		return err
	}
	defer repository.Close()
	current, err := repository.Load()
	if err != nil {
		return err
	}
	current, err = cfg.EnsureSources(current)
	if err != nil {
		return err
	}
	logicalIDs, err := cfg.LogicalFeedIDs(current)
	if err != nil {
		return err
	}
	resolver := media.Resolver(logicalIDs)
	records, err := loadPlaybackRecords(layout)
	if err != nil {
		return err
	}
	states := playback.ForEpisodesWithResolver(current.Episodes, records, resolver)
	selected := make(map[string]episode.Episode)
	var playlistFiles []syncer.PlaylistFile
	var managed []string
	feedNames := make(map[int64]string, len(current.Feeds))
	for _, known := range current.Feeds {
		feedNames[known.ID] = known.Name
	}
	for _, logical := range cfg.Feeds {
		episodes, selectErr := selection.FeedForSync(cfg, current, logical.ID, states, false)
		if selectErr != nil {
			return selectErr
		}
		for _, currentEpisode := range episodes {
			selected[currentEpisode.IdentityKey()] = currentEpisode
		}
		content, generateErr := playlists.LogicalFeedWithResolver(cfg, current, logical.ID, states, resolver)
		if generateErr != nil {
			return generateErr
		}
		playlistFiles = append(playlistFiles, syncer.PlaylistFile{Relative: path.Join("Playlists", playlists.Filename(logical.Title, logical.ID)+".m3u8"), Content: content})
	}
	for _, configured := range cfg.Briefings {
		plan, generateErr := briefing.Build(cfg, current, configured.ID, states)
		if generateErr != nil {
			return generateErr
		}
		for _, currentEpisode := range plan.Episodes {
			selected[currentEpisode.IdentityKey()] = currentEpisode
		}
		tracks := make([]playlists.Track, len(plan.Episodes))
		for i, currentEpisode := range plan.Episodes {
			tracks[i] = playlists.Track{Episode: currentEpisode, Path: path.Join("..", resolver.RelativePathFor(currentEpisode))}
		}
		playlistFiles = append(playlistFiles, syncer.PlaylistFile{Relative: path.Join("Playlists", playlists.Filename(configured.Title, configured.ID)+".m3u8"), Content: playlists.M3U(tracks)})
	}
	artistOverrides := make(map[int64]string)
	for _, logical := range cfg.Feeds {
		if logical.Artist == "" {
			continue
		}
		for feedID, logicalID := range logicalIDs {
			if logicalID == logical.ID {
				artistOverrides[feedID] = logical.Artist
			}
		}
	}
	for identity, currentEpisode := range selected {
		if artist := artistOverrides[currentEpisode.FeedID]; artist != "" {
			currentEpisode.Author = artist
			selected[identity] = currentEpisode
		}
	}
	managed, err = layout.LoadManagedPaths()
	if err != nil {
		return err
	}
	if stagingDir != "" {
		if err := validateStagingDirectory(layout.Root, stagingDir); err != nil {
			return err
		}
	} else if !dryRun {
		stagingDir, err = os.MkdirTemp("", "podsync-staging-")
		if err != nil {
			return fmt.Errorf("create staging directory: %w", err)
		}
		defer os.RemoveAll(stagingDir)
	}
	episodes := make([]episode.Episode, 0, len(selected))
	for _, currentEpisode := range selected {
		episodes = append(episodes, currentEpisode)
	}
	sort.Slice(episodes, func(i, j int) bool {
		return resolver.RelativePathFor(episodes[i]) < resolver.RelativePathFor(episodes[j])
	})
	newManaged := make([]string, 0, len(episodes)+len(playlistFiles))
	for _, currentEpisode := range episodes {
		newManaged = append(newManaged, resolver.RelativePathFor(currentEpisode))
	}
	for _, playlist := range playlistFiles {
		newManaged = append(newManaged, playlist.Relative)
	}
	manifest := syncer.PlaylistFile{Relative: layout.ManifestRelativePath(), Content: manifestContent(newManaged)}
	playlistFiles = append(playlistFiles, manifest)
	if dryRun {
		copies := make([]syncer.FileCopy, 0, len(episodes))
		for _, currentEpisode := range episodes {
			copies = append(copies, syncer.FileCopy{Relative: resolver.RelativePathFor(currentEpisode)})
		}
		plan, err := syncer.BuildFilePlan(managed, copies, playlistFiles, nil)
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("Dry run selected %d episodes, writes %d playlists, and deletes %d managed files", len(episodes), len(playlistFiles)-1, len(plan.Deletes)))
		return nil
	}
	if err := syncer.EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), httpClient, stagingDir, layout.Root, episodes, playlistFiles, managed, feedNames, resolver, syncer.EpisodeSyncOptions{VerifyMedia: verifyMedia}, func(completed, total int, current episode.Episode, reused bool) {
		if logProgress(completed, total) {
			logger.Info(fmt.Sprintf("Prepared episodes: %d/%d", completed, total))
		}
	}, func(progress syncer.FileProgress) {
		switch progress.Phase {
		case "copy":
			if logProgress(progress.Completed, progress.Total) {
				logger.Info(fmt.Sprintf("Copied files: %d/%d", progress.Completed, progress.Total))
			}
		case "playlist":
			if logProgress(progress.Completed, progress.Total) {
				logger.Info(fmt.Sprintf("Wrote playlists: %d/%d", progress.Completed, progress.Total))
			}
		case "delete":
			if logProgress(progress.Completed, progress.Total) {
				logger.Info(fmt.Sprintf("Removed managed files: %d/%d", progress.Completed, progress.Total))
			}
		}
	}, func(message string) {
		logger.Warn(message)
	}); err != nil {
		return err
	}
	if configProvided {
		if err := config.Save(layout.ConfigPath(), cfg); err != nil {
			return fmt.Errorf("save device config: %w", err)
		}
	}
	if err := repository.Save(current); err != nil {
		return fmt.Errorf("save configured sources: %w", err)
	}
	logger.Info(fmt.Sprintf("Sync complete: %d episodes, %d playlists", len(episodes), len(playlistFiles)-1))
	return nil
}

func logProgress(completed, total int) bool {
	return completed == 1 || completed == total || completed%50 == 0
}

func openDevice(root, configPath string, readOnly, persistConfig bool) (device.Layout, state.Repository, config.Config, error) {
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
	if !readOnly && persistConfig && configPath != layout.ConfigPath() {
		if err := config.Save(layout.ConfigPath(), cfg); err != nil {
			_ = repository.Close()
			return layout, nil, config.Config{}, fmt.Errorf("save device config: %w", err)
		}
	}
	return layout, repository, cfg, nil
}

func validateStagingDirectory(deviceRoot, stagingDir string) error {
	deviceRoot, err := filepath.Abs(deviceRoot)
	if err != nil {
		return fmt.Errorf("resolve device root: %w", err)
	}
	stagingDir, err = filepath.Abs(stagingDir)
	if err != nil {
		return fmt.Errorf("resolve staging directory: %w", err)
	}
	relative, err := filepath.Rel(deviceRoot, stagingDir)
	if err != nil {
		return fmt.Errorf("compare staging directory: %w", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return fmt.Errorf("staging directory must be outside the device root")
	}
	return nil
}

func openRepositoryMode(root string, readOnly bool) (device.Layout, state.Repository, error) {
	if root == "" {
		var err error
		root, err = device.ResolveRoot("")
		if err != nil {
			return device.Layout{}, nil, err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return device.Layout{}, nil, fmt.Errorf("resolve device root: %w", err)
	}
	layout := device.Layout{Root: root}
	info, err := os.Lstat(root)
	if err != nil {
		return layout, nil, fmt.Errorf("inspect device root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return layout, nil, fmt.Errorf("device root is not a real directory")
	}
	if stateInfo, stateErr := os.Lstat(layout.StateDirectory()); stateErr == nil && stateInfo.Mode()&os.ModeSymlink != 0 {
		return layout, nil, fmt.Errorf("device state directory is a symlink")
	} else if stateErr != nil && !os.IsNotExist(stateErr) {
		return layout, nil, fmt.Errorf("inspect device state directory: %w", stateErr)
	}
	if !readOnly {
		if err := os.MkdirAll(layout.StateDirectory(), 0o755); err != nil {
			return layout, nil, fmt.Errorf("create device state directory: %w", err)
		}
	}
	var repository state.Repository
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
