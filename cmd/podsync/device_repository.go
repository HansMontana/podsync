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
	layout, repository, err := openRepositoryModeWithLock(root, readOnly, lock)
	if err != nil {
		return layout, nil, curation.Config{}, err
	}
	if configPath == "" {
		configPath = layout.ConfigPath()
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
		if err := os.MkdirAll(layout.StateDirectory(), 0o755); err != nil {
			return layout, nil, fmt.Errorf("create device state directory: %w", err)
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
