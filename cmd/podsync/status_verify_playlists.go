package main

import (
	"context"
	"fmt"
	"path"

	"github.com/HansMontana/podsync/internal/adapters/media"
	"github.com/HansMontana/podsync/internal/adapters/playlists"
	"github.com/HansMontana/podsync/internal/playback"
	syncer "github.com/HansMontana/podsync/internal/sync"
)

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

func generatePlaylist(ctx context.Context, args []string, briefingMode bool) error {
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
	if err := ctx.Err(); err != nil {
		return err
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
	_, playbackStates, err := loadPlaybackRecordsAndStates(layout, current.Episodes, resolver)
	if err != nil {
		return err
	}
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
	managed, err := layout.LoadCommittedManagedPaths()
	if err != nil {
		return err
	}
	pending, err := layout.LoadPendingManagedPaths()
	if err != nil {
		return err
	}
	pending = appendUnique(pending, relative)
	if err := layout.SavePendingManagedPaths(pending); err != nil {
		return fmt.Errorf("save pending playlist ownership: %w", err)
	}
	managed = appendUnique(managed, relative)
	manifest := syncer.PlaylistFile{Relative: layout.ManifestRelativePath(), Content: manifestContent(managed)}
	if err := syncer.ApplyFilePlan(ctx, layout.Root, syncer.FilePlan{Playlists: []syncer.PlaylistFile{{Relative: relative, Content: content}, manifest}}); err != nil {
		return err
	}
	pending = removePath(pending, relative)
	if len(pending) == 0 {
		if err := layout.ClearPendingManagedPaths(); err != nil {
			return fmt.Errorf("clear pending playlist ownership: %w", err)
		}
	} else if err := layout.SavePendingManagedPaths(pending); err != nil {
		return fmt.Errorf("update pending playlist ownership: %w", err)
	}
	commandLogger("playlist").Info(fmt.Sprintf("Wrote %s", relative))
	return nil
}
