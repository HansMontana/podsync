package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	devicefs "github.com/HansMontana/podsync/internal/adapters/devicefs"
	"github.com/HansMontana/podsync/internal/adapters/media"
	rockboxplayback "github.com/HansMontana/podsync/internal/adapters/rockbox"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/domain/playback"
)

func appendUnique(paths []string, value string) []string {
	for _, current := range paths {
		if current == value {
			return paths
		}
	}
	return append(paths, value)
}

func removePath(paths []string, value string) []string {
	result := paths[:0]
	for _, path := range paths {
		if path != value {
			result = append(result, path)
		}
	}
	return result
}

func manifestContent(paths []string) []byte {
	var content bytes.Buffer
	for _, current := range paths {
		_, _ = content.WriteString(current)
		_, _ = content.WriteString("\n")
	}
	return content.Bytes()
}

func loadPlaybackRecords(layout devicefs.Layout) ([]rockboxplayback.Record, []error, error) {
	paths, err := layout.PlaybackLogPaths()
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(paths)
	var records []rockboxplayback.Record
	var warnings []error
	for _, path := range paths {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			warnings = append(warnings, fmt.Errorf("inspect playback log %q: %w", path, statErr))
			continue
		}
		if !info.Mode().IsRegular() {
			warnings = append(warnings, fmt.Errorf("playback log %q is not a regular file", path))
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("open playback log %q: %w", path, err))
			continue
		}
		parsed, parseErr := rockboxplayback.ParseLog(file)
		closeErr := file.Close()
		if parseErr != nil {
			warnings = append(warnings, fmt.Errorf("parse playback log %q: %w", path, parseErr))
			continue
		}
		if closeErr != nil {
			warnings = append(warnings, fmt.Errorf("close playback log %q: %w", path, closeErr))
			continue
		}
		records = append(records, parsed...)
	}
	masterPath := filepath.Join(layout.TagCacheDirectory(), "database_idx.tcd")
	filenamePath := filepath.Join(layout.TagCacheDirectory(), "database_4.tcd")
	masterExists, err := fileExists(masterPath)
	if err != nil {
		warnings = append(warnings, fmt.Errorf("inspect TagCache master: %w", err))
		return rockboxplayback.MergeRecords(records), warnings, nil
	}
	filenameExists, err := fileExists(filenamePath)
	if err != nil {
		warnings = append(warnings, fmt.Errorf("inspect TagCache filename index: %w", err))
		return rockboxplayback.MergeRecords(records), warnings, nil
	}
	if masterExists != filenameExists {
		warnings = append(warnings, fmt.Errorf("incomplete TagCache: master=%t filename-index=%t", masterExists, filenameExists))
		return rockboxplayback.MergeRecords(records), warnings, nil
	}
	if masterExists {
		parsed, err := rockboxplayback.ParseTagCache(layout.TagCacheDirectory())
		if err != nil {
			warnings = append(warnings, fmt.Errorf("parse TagCache: %w", err))
			return rockboxplayback.MergeRecords(records), warnings, nil
		}
		return rockboxplayback.PreferRecords(parsed, rockboxplayback.MergeRecords(records)), warnings, nil
	}
	return rockboxplayback.MergeRecords(records), warnings, nil
}

func loadPlaybackRecordsAndStates(layout devicefs.Layout, episodes []catalog.Episode, resolver media.Resolver) ([]rockboxplayback.Record, map[string]playback.State, []error, error) {
	records, warnings, err := loadPlaybackRecords(layout)
	if err != nil {
		return nil, nil, nil, err
	}
	return records, rockboxplayback.ForEpisodesWithResolver(episodes, records, resolver), warnings, nil
}

func fileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("path is not a regular file: %q", path)
		}
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
