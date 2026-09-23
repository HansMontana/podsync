package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/HansMontana/podsync/internal/adapters/tomlconfig"
	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/domain/curation"
	"github.com/HansMontana/podsync/internal/state"
	syncer "github.com/HansMontana/podsync/internal/sync"
)

type lockedRepository struct {
	state.Repository
	lock  *device.Lock
	owned bool
}

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

func openDevice(root, configPath string, readOnly, persistConfig bool) (device.Layout, state.Repository, curation.Config, error) {
	return openDeviceWithLock(root, configPath, readOnly, persistConfig, nil)
}

func openDeviceWithLock(root, configPath string, readOnly, persistConfig bool, lock *device.Lock) (device.Layout, state.Repository, curation.Config, error) {
	layout, repository, err := openRepositoryModeWithLock(root, readOnly, lock)
	if err != nil {
		return layout, nil, curation.Config{}, err
	}
	if configPath == "" {
		configPath = layout.ConfigPath()
	}
	if configPath == layout.ConfigPath() {
		if err := device.RejectSymlink(configPath); err != nil {
			_ = repository.Close()
			return layout, nil, curation.Config{}, err
		}
	}
	cfg, err := tomlconfig.Load(configPath)
	if err != nil {
		_ = repository.Close()
		return layout, nil, curation.Config{}, err
	}
	if !readOnly && persistConfig && configPath != layout.ConfigPath() {
		if err := tomlconfig.Save(layout.ConfigPath(), cfg); err != nil {
			_ = repository.Close()
			return layout, nil, curation.Config{}, fmt.Errorf("save device config: %w", err)
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
	return openRepositoryModeWithLock(root, readOnly, nil)
}

func openRepositoryModeWithLock(root string, readOnly bool, heldLock *device.Lock) (device.Layout, state.Repository, error) {
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
	if err := device.RejectSymlink(layout.DatabasePath()); err != nil {
		return layout, nil, err
	}
	if !readOnly {
		if err := os.MkdirAll(layout.StateDirectory(), 0o755); err != nil {
			return layout, nil, fmt.Errorf("create device state directory: %w", err)
		}
	}
	var lock *device.Lock
	if !readOnly && heldLock == nil {
		lock, err = device.AcquireLock(root)
		if err != nil {
			return layout, nil, err
		}
	}
	var repository state.Repository
	if readOnly {
		repository, err = state.NewReadOnlySQLiteRepository(layout.DatabasePath())
	} else {
		repository, err = state.NewSQLiteRepository(layout.DatabasePath())
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

func reconcileState(repository state.Repository, cfg curation.Config) error {
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

func refreshRequests(current catalog.Catalog, cfg curation.Config) ([]syncer.RefreshRequest, error) {
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
	requests := make([]syncer.RefreshRequest, 0, len(cfg.Sources))
	for _, source := range cfg.Sources {
		normalized, err := catalog.NormalizeURL(source.URL)
		if err != nil {
			return nil, fmt.Errorf("normalize source %q: %w", source.ID, err)
		}
		feedID, exists := byURL[normalized]
		if !exists {
			return nil, fmt.Errorf("source %q was not reconciled", source.ID)
		}
		requests = append(requests, syncer.RefreshRequest{FeedID: feedID, Archive: archiveBySource[source.ID]})
	}
	return requests, nil
}
