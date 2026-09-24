package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	devicefs "github.com/HansMontana/podsync/internal/adapters/devicefs"
	"github.com/HansMontana/podsync/internal/adapters/sqlitecatalog"
	"github.com/HansMontana/podsync/internal/adapters/tomlconfig"
	applicationcatalog "github.com/HansMontana/podsync/internal/application/catalog"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/domain/curation"
)

type lockedRepository struct {
	applicationcatalog.Repository
	lock  *devicefs.Lock
	owned bool
}

type deviceVerifier func() error

func (r *lockedRepository) Close() error {
	repositoryErr := r.Repository.Close()
	var lockErr error
	if r.owned {
		lockErr = r.lock.Close()
	}
	if repositoryErr != nil {
		return repositoryErr
	}
	return lockErr
}

func openDevice(root, configPath string, readOnly, persistConfig bool) (devicefs.Layout, applicationcatalog.Repository, curation.Config, error) {
	return openDeviceWithLock(root, configPath, readOnly, persistConfig, nil)
}

func openDeviceWithLock(root, configPath string, readOnly, persistConfig bool, lock *devicefs.Lock) (devicefs.Layout, applicationcatalog.Repository, curation.Config, error) {
	var verify deviceVerifier
	if !readOnly {
		resolvedRoot, identity, err := devicefs.ResolveRootIdentity(root)
		if err != nil {
			return devicefs.Layout{}, nil, curation.Config{}, err
		}
		root = resolvedRoot
		verify = func() error { return devicefs.VerifyRootIdentity(root, identity) }
	}
	layout, repository, err := openRepositoryModeWithLock(root, readOnly, lock)
	if err != nil {
		return layout, nil, curation.Config{}, err
	}
	if configPath == "" {
		configPath = layout.ConfigPath()
	}
	if !readOnly {
		if err := recoverPendingSyncConfig(layout, repository, verify); err != nil {
			_ = repository.Close()
			return layout, nil, curation.Config{}, err
		}
	}
	if configPath == layout.ConfigPath() {
		if err := devicefs.RejectSymlink(configPath); err != nil {
			_ = repository.Close()
			return layout, nil, curation.Config{}, err
		}
	}
	cfg, err := tomlconfig.Load(configPath)
	if err != nil {
		_ = repository.Close()
		return layout, nil, curation.Config{}, err
	}
	if !readOnly {
		if err := recoverPendingFeedRemoval(layout, repository, cfg, verify); err != nil {
			_ = repository.Close()
			return layout, nil, curation.Config{}, err
		}
	}
	if !readOnly && persistConfig && configPath != layout.ConfigPath() {
		if err := verify(); err != nil {
			_ = repository.Close()
			return layout, nil, curation.Config{}, fmt.Errorf("verify device before saving config: %w", err)
		}
		if err := tomlconfig.Save(layout.ConfigPath(), cfg); err != nil {
			_ = repository.Close()
			return layout, nil, curation.Config{}, fmt.Errorf("save device config: %w", err)
		}
	}
	return layout, repository, cfg, nil
}

func openMutatingDevice(root, configPath string, persistConfig bool, lock *devicefs.Lock) (devicefs.Layout, applicationcatalog.Repository, curation.Config, deviceVerifier, error) {
	resolvedRoot, identity, err := devicefs.ResolveRootIdentity(root)
	if err != nil {
		return devicefs.Layout{}, nil, curation.Config{}, nil, err
	}
	layout, repository, cfg, err := openDeviceWithLock(resolvedRoot, configPath, false, persistConfig, lock)
	if err != nil {
		return layout, nil, curation.Config{}, nil, err
	}
	verify := func() error { return devicefs.VerifyRootIdentity(resolvedRoot, identity) }
	if err := verify(); err != nil {
		_ = repository.Close()
		return layout, nil, curation.Config{}, nil, fmt.Errorf("verify device after opening: %w", err)
	}
	return layout, repository, cfg, verify, nil
}

func recoverPendingFeedRemoval(layout devicefs.Layout, repository applicationcatalog.Repository, cfg curation.Config, verify deviceVerifier) error {
	pending, err := layout.LoadPendingFeedRemoval()
	if err != nil {
		return err
	}
	if pending.URL == "" {
		return nil
	}
	pendingURL, err := catalog.NormalizeURL(pending.URL)
	if err != nil {
		return fmt.Errorf("normalize pending feed removal URL: %w", err)
	}
	for _, source := range cfg.Sources {
		sourceURL, sourceErr := catalog.NormalizeURL(source.URL)
		if sourceErr == nil && sourceURL == pendingURL {
			return layout.ClearPendingFeedRemoval()
		}
	}
	current, err := repository.Load()
	if err != nil {
		return fmt.Errorf("load state for pending feed removal: %w", err)
	}
	remainingFeeds := current.Feeds[:0]
	removedIDs := make(map[int64]struct{})
	for _, feed := range current.Feeds {
		if feed.SameIdentity(catalog.Feed{URL: pendingURL}) {
			removedIDs[feed.ID] = struct{}{}
			continue
		}
		remainingFeeds = append(remainingFeeds, feed)
	}
	if len(removedIDs) > 0 {
		remainingEpisodes := current.Episodes[:0]
		for _, episode := range current.Episodes {
			if _, removed := removedIDs[episode.FeedID]; !removed {
				remainingEpisodes = append(remainingEpisodes, episode)
			}
		}
		current.Feeds = remainingFeeds
		current.Episodes = remainingEpisodes
		if err := verify(); err != nil {
			return fmt.Errorf("verify device before completing feed removal: %w", err)
		}
		if err := repository.Save(current); err != nil {
			return fmt.Errorf("complete pending feed removal: %w", err)
		}
	}
	return layout.ClearPendingFeedRemoval()
}

func recoverPendingSyncConfig(layout devicefs.Layout, repository applicationcatalog.Repository, verify deviceVerifier) error {
	pendingPath := layout.PendingSyncConfigPath()
	if err := devicefs.RejectSymlink(pendingPath); err != nil {
		return err
	}
	if _, err := os.Stat(pendingPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect pending sync configuration: %w", err)
	}
	pending, err := tomlconfig.Load(pendingPath)
	if err != nil {
		return fmt.Errorf("load pending sync configuration: %w", err)
	}
	current, err := repository.Load()
	if err != nil {
		return fmt.Errorf("load state for pending sync: %w", err)
	}
	reconciled, err := pending.EnsureSources(current)
	if err != nil {
		return fmt.Errorf("reconcile pending sync configuration: %w", err)
	}
	if verify != nil {
		if err := verify(); err != nil {
			return fmt.Errorf("verify device before recovering sync state: %w", err)
		}
	}
	if err := repository.Save(reconciled); err != nil {
		return fmt.Errorf("recover pending sync state: %w", err)
	}
	if verify != nil {
		if err := verify(); err != nil {
			return fmt.Errorf("verify device before recovering sync configuration: %w", err)
		}
	}
	if err := tomlconfig.Save(layout.ConfigPath(), pending); err != nil {
		return fmt.Errorf("recover pending sync configuration: %w", err)
	}
	if err := layout.ClearPendingSyncConfig(); err != nil {
		return fmt.Errorf("clear pending sync configuration: %w", err)
	}
	return nil
}

func validateStagingDirectory(deviceRoot, stagingDir string) error {
	deviceRoot, err := filepath.EvalSymlinks(deviceRoot)
	if err != nil {
		return fmt.Errorf("resolve device root: %w", err)
	}
	stagingDir, err = resolveExistingPath(stagingDir)
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

func resolveExistingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var suffix []string
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing parent for %q", path)
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func openRepositoryMode(root string, readOnly bool) (devicefs.Layout, applicationcatalog.Repository, error) {
	return openRepositoryModeWithLock(root, readOnly, nil)
}

func openRepositoryModeWithLock(root string, readOnly bool, heldLock *devicefs.Lock) (devicefs.Layout, applicationcatalog.Repository, error) {
	if root == "" {
		var err error
		root, err = devicefs.ResolveRoot("")
		if err != nil {
			return devicefs.Layout{}, nil, err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return devicefs.Layout{}, nil, fmt.Errorf("resolve device root: %w", err)
	}
	layout := devicefs.Layout{Root: root}
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
	if err := devicefs.RejectSymlink(layout.DatabasePath()); err != nil {
		return layout, nil, err
	}
	if !readOnly {
		if err := devicefs.EnsureStateDirectory(root); err != nil {
			return layout, nil, err
		}
	}
	var lock *devicefs.Lock
	if !readOnly && heldLock == nil {
		lock, err = devicefs.AcquireLock(root)
		if err != nil {
			return layout, nil, err
		}
	}
	var repository applicationcatalog.Repository
	if readOnly {
		repository, err = sqlitecatalog.NewReadOnlySQLiteRepository(layout.DatabasePath())
	} else {
		repository, err = sqlitecatalog.NewSQLiteRepository(layout.DatabasePath())
	}
	if err != nil {
		if lock != nil {
			_ = lock.Close()
		}
		return layout, nil, err
	}
	if lock != nil {
		repository = &lockedRepository{Repository: repository, lock: lock, owned: true}
	} else if heldLock != nil {
		repository = &lockedRepository{Repository: repository, lock: heldLock}
	}
	return layout, repository, nil
}

func reconcileState(repository applicationcatalog.Repository, cfg curation.Config) error {
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

func refreshRequests(current catalog.Catalog, cfg curation.Config) ([]applicationcatalog.RefreshRequest, error) {
	byURL := make(map[string]int64, len(current.Feeds))
	for _, known := range current.Feeds {
		normalized, err := catalog.NormalizeURL(known.URL)
		if err != nil {
			return nil, fmt.Errorf("normalize existing feed %d: %w", known.ID, err)
		}
		byURL[normalized] = known.ID
	}
	archiveBySource := make(map[string]bool)
	for _, logical := range cfg.Feeds {
		archiveBySource[logical.Source] = archiveBySource[logical.Source] || logical.Archive
	}
	requests := make([]applicationcatalog.RefreshRequest, 0, len(cfg.Sources))
	for _, source := range cfg.Sources {
		normalized, err := catalog.NormalizeURL(source.URL)
		if err != nil {
			return nil, fmt.Errorf("normalize source %q: %w", source.ID, err)
		}
		feedID, exists := byURL[normalized]
		if !exists {
			return nil, fmt.Errorf("source %q was not reconciled", source.ID)
		}
		requests = append(requests, applicationcatalog.RefreshRequest{FeedID: feedID, Archive: archiveBySource[source.ID]})
	}
	return requests, nil
}
