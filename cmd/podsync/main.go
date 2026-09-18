package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"

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
		return fmt.Errorf("usage: podsync <validate-config|reconcile|refresh|status|playlist|briefing|sync> [options]")
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
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath)
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
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	_, repository, _, err := openDevice(*deviceRoot, *configPath)
	if err != nil {
		return err
	}
	defer repository.Close()
	current, err := repository.Load()
	if err != nil {
		return err
	}
	fmt.Printf("feeds: %d\nepisodes: %d\n", len(current.Feeds), len(current.Episodes))
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
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath)
	if err != nil {
		return err
	}
	defer repository.Close()
	current, err := repository.Load()
	if err != nil {
		return err
	}
	var content []byte
	if briefingMode {
		content, err = playlists.Briefing(cfg, current, *id, map[string]playback.State{})
	} else {
		content, err = playlists.LogicalFeed(cfg, current, *id, map[string]playback.State{})
	}
	if err != nil {
		return err
	}
	relative := path.Join("Playlists", *id+".m3u8")
	if err := syncer.ApplyFilePlan(context.Background(), layout.Root, syncer.FilePlan{Playlists: []syncer.PlaylistFile{{Relative: relative, Content: content}}}); err != nil {
		return err
	}
	managed, err := layout.LoadManagedPaths()
	if err != nil {
		return err
	}
	managed = appendUnique(managed, relative)
	if err := layout.SaveManagedPaths(managed); err != nil {
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
	if err := flags.Parse(args); err != nil {
		return err
	}
	layout, repository, cfg, err := openDevice(*deviceRoot, *configPath)
	if err != nil {
		return err
	}
	defer repository.Close()
	if err := reconcileState(repository, cfg); err != nil {
		return err
	}
	current, err := repository.Load()
	if err != nil {
		return err
	}
	states := map[string]playback.State{}
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
		newManaged = append(newManaged, mediaPath(currentEpisode))
	}
	for _, playlist := range playlistFiles {
		newManaged = append(newManaged, playlist.Relative)
	}
	if err := syncer.Episodes(context.Background(), http.DefaultClient, *stagingDir, layout.Root, episodes, playlistFiles, managed); err != nil {
		return err
	}
	if err := layout.SaveManagedPaths(newManaged); err != nil {
		return err
	}
	fmt.Printf("synced %d episodes and %d playlists\n", len(episodes), len(playlistFiles))
	return nil
}

func openDevice(root, configPath string) (device.Layout, state.Repository, config.Config, error) {
	if root == "" {
		return device.Layout{}, nil, config.Config{}, fmt.Errorf("-device-root is required")
	}
	layout := device.Layout{Root: root}
	if err := os.MkdirAll(layout.StateDirectory(), 0o755); err != nil {
		return layout, nil, config.Config{}, fmt.Errorf("create device state directory: %w", err)
	}
	if configPath == "" {
		configPath = layout.ConfigPath()
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return layout, nil, config.Config{}, err
	}
	if configPath != layout.ConfigPath() {
		if err := config.Save(layout.ConfigPath(), cfg); err != nil {
			return layout, nil, config.Config{}, fmt.Errorf("save device config: %w", err)
		}
	}
	repository, err := state.NewSQLiteRepository(layout.DatabasePath())
	if err != nil {
		return layout, nil, config.Config{}, err
	}
	return layout, repository, cfg, nil
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

func mediaPath(current episode.Episode) string {
	return media.RelativePath(current)
}
