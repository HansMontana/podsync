package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HansMontana/podsync/internal/adapters/playlists"
	"github.com/HansMontana/podsync/internal/adapters/tomlconfig"
	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/domain/curation"
	"github.com/HansMontana/podsync/internal/logging"
	"github.com/HansMontana/podsync/internal/media"
	syncer "github.com/HansMontana/podsync/internal/sync"
)

type syncOptions struct {
	deviceRoot      string
	configPath      string
	stagingDir      string
	dryRun          bool
	skipVerifyMedia bool
	lock            *device.Lock
	identity        fs.FileInfo
}

func sync(ctx context.Context, args []string) error {
	flags := newFlagSet("sync", "Usage: podsync sync -device-root PATH [-config PATH] [-staging PATH] [-dry-run] [-skip-verify-media]")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration (defaults to device config)")
	stagingDir := flags.String("staging", "", "host-side staging directory (defaults to a temporary directory)")
	dryRun := flags.Bool("dry-run", false, "show the sync plan without downloading or changing the device")
	skipVerifyMedia := flags.Bool("skip-verify-media", false, "skip inspection and repair of existing MP3 metadata")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	return syncDeviceContext(ctx, syncOptions{deviceRoot: *deviceRoot, configPath: *configPath, stagingDir: *stagingDir, dryRun: *dryRun, skipVerifyMedia: *skipVerifyMedia})
}

func syncDevice(options syncOptions) error {
	return syncDeviceContext(context.Background(), options)
}

func syncDeviceContext(ctx context.Context, options syncOptions) error {
	identity := options.identity
	if identity == nil {
		resolvedRoot, captured, err := device.ResolveRootIdentity(options.deviceRoot)
		if err != nil {
			return err
		}
		options.deviceRoot = resolvedRoot
		identity = captured
	}
	deviceRoot := options.deviceRoot
	configPath := options.configPath
	stagingDir := options.stagingDir
	dryRun := options.dryRun
	verifyMedia := !options.skipVerifyMedia
	configProvided := configPath != ""
	logger := logging.New(os.Stderr).WithComponent("sync")
	logger.Info("Starting sync")
	layout, repository, cfg, err := openDeviceWithLock(deviceRoot, configPath, dryRun, false, options.lock)
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
	_, states, err := loadPlaybackRecordsAndStates(layout, current.Episodes, resolver)
	if err != nil {
		return err
	}
	selected := make(map[string]catalog.Episode)
	var playlistFiles []syncer.PlaylistFile
	var managed []string
	feedNames := make(map[int64]string, len(current.Feeds))
	for _, known := range current.Feeds {
		feedNames[known.ID] = known.Name
	}
	for _, logical := range cfg.Feeds {
		episodes, selectErr := curation.FeedForSync(cfg, current, logical.ID, states, false)
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
		plan, generateErr := curation.Build(cfg, current, configured.ID, states)
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
		stagingDir, err = prepareStagingDirectory(stagingDir)
		if err != nil {
			return err
		}
		defer os.RemoveAll(stagingDir)
	} else if !dryRun {
		stagingDir, err = os.MkdirTemp("", "podsync-staging-")
		if err != nil {
			return fmt.Errorf("create staging directory: %w", err)
		}
		defer os.RemoveAll(stagingDir)
	}
	episodes := make([]catalog.Episode, 0, len(selected))
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
	if err := syncer.EpisodesWithResolverAndProgressAndWarningsWithOptions(ctx, httpClient, stagingDir, layout.Root, episodes, playlistFiles, managed, feedNames, resolver, syncer.EpisodeSyncOptions{VerifyMedia: verifyMedia, VerifyDevice: func() error {
		return device.VerifyRootIdentity(layout.Root, identity)
	}}, func(completed, total int, current catalog.Episode, reused bool) {
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
	if err := device.VerifyRootIdentity(layout.Root, identity); err != nil {
		return fmt.Errorf("before persisting sync state: %w", err)
	}
	if configProvided {
		if err := tomlconfig.Save(layout.ConfigPath(), cfg); err != nil {
			return fmt.Errorf("save device config: %w", err)
		}
	}
	if err := repository.Save(current); err != nil {
		return fmt.Errorf("save configured sources: %w", err)
	}
	logger.Info(fmt.Sprintf("Sync complete: %d episodes, %d playlists", len(episodes), len(playlistFiles)-1))
	return nil
}

func prepareStagingDirectory(parent string) (string, error) {
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create staging parent: %w", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", fmt.Errorf("read staging parent: %w", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "podsync-download-") {
			if err := os.Remove(filepath.Join(parent, entry.Name())); err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("remove stale staging file %q: %w", entry.Name(), err)
			}
		}
	}
	runDir, err := os.MkdirTemp(parent, "podsync-run-")
	if err != nil {
		return "", fmt.Errorf("create run staging directory: %w", err)
	}
	return runDir, nil
}

func logProgress(completed, total int) bool {
	return completed == 1 || completed == total || completed%50 == 0
}
