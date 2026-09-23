package main

import (
	"context"
	"fmt"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/state"
	syncer "github.com/HansMontana/podsync/internal/sync"
)

func feedCommand(ctx context.Context, args []string) error {
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
		return addFeed(ctx, args[1:])
	case "remove":
		return removeFeed(ctx, args[1:])
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

func addFeed(ctx context.Context, args []string) error {
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
	if err := ctx.Err(); err != nil {
		return err
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
	if err := saveConfigThenState(
		func() error { return config.Save(layout.ConfigPath(), cfg) },
		func() error { return reconcileState(repository, cfg) },
	); err != nil {
		return err
	}
	commandLogger("feed").Info(fmt.Sprintf("Added source %s", *id))
	return nil
}

func removeFeed(ctx context.Context, args []string) error {
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
	if err := ctx.Err(); err != nil {
		return err
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
	previous := state.State{
		Feeds:    append([]feed.Feed(nil), current.Feeds...),
		Episodes: append([]episode.Episode(nil), current.Episodes...),
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
	if err := saveStateThenConfig(
		func() error { return repository.Save(current) },
		func() error { return config.Save(layout.ConfigPath(), cfg) },
		func() error { return repository.Save(previous) },
	); err != nil {
		return err
	}
	commandLogger("feed").Info(fmt.Sprintf("Removed source %s", *id))
	return nil
}

func saveConfigThenState(saveConfig func() error, saveState func() error) error {
	if err := saveConfig(); err != nil {
		return fmt.Errorf("save device config: %w", err)
	}
	if err := saveState(); err != nil {
		return fmt.Errorf("save state after config save: %w", err)
	}
	return nil
}

func saveStateThenConfig(saveState func() error, saveConfig func() error, restoreState func() error) error {
	if err := saveState(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if err := saveConfig(); err != nil {
		if restoreErr := restoreState(); restoreErr != nil {
			return fmt.Errorf("save device config: %w; restore state: %v", err, restoreErr)
		}
		return fmt.Errorf("save device config: %w; state restored", err)
	}
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

func reconcile(ctx context.Context, args []string, refresh bool) error {
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
	return reconcileDeviceWithLockContext(ctx, *deviceRoot, *configPath, refresh, nil)
}

func reconcileDevice(deviceRoot, configPath string, refresh bool) error {
	return reconcileDeviceWithLock(deviceRoot, configPath, refresh, nil)
}

func reconcileDeviceWithLock(deviceRoot, configPath string, refresh bool, lock *device.Lock) error {
	return reconcileDeviceWithLockContext(context.Background(), deviceRoot, configPath, refresh, lock)
}

func reconcileDeviceWithLockContext(ctx context.Context, deviceRoot, configPath string, refresh bool, lock *device.Lock) error {
	logger := commandLogger("feed")
	layout, repository, cfg, err := openDeviceWithLock(deviceRoot, configPath, false, true, lock)
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
	current, err := repository.Load()
	if err != nil {
		return err
	}
	requests, err := refreshRequests(current, cfg)
	if err != nil {
		return err
	}
	if err := syncer.RefreshFeeds(ctx, repository, httpClient, requests); err != nil {
		return fmt.Errorf("refresh feeds: %w", err)
	}
	for _, source := range cfg.Sources {
		logger.Info(fmt.Sprintf("Refreshed source %s", source.ID))
	}
	_ = layout
	return nil
}
