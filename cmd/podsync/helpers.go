package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/playback"
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
