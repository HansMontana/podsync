package sync

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/HansMontana/podsync/internal/download"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/media"
)

// Episodes downloads selected episodes to host staging, then applies the
// complete device file plan. Device deletions happen only after all downloads
// and device writes succeed.
func Episodes(ctx context.Context, client *http.Client, stagingDir, deviceRoot string, episodes []episode.Episode, playlists []PlaylistFile, managed []string) error {
	var copies []FileCopy
	var keep []string
	var staged []string
	defer func() {
		for _, path := range staged {
			_ = os.Remove(path)
		}
	}()

	for _, current := range episodes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		relative := media.RelativePath(current)
		exists, err := reusableFile(deviceRoot, relative, current.Enclosure.Length)
		if err != nil {
			return fmt.Errorf("inspect existing episode %q: %w", current.Title, err)
		}
		if exists {
			keep = append(keep, relative)
			continue
		}

		stagedPath, err := download.Episode(ctx, client, current, stagingDir)
		if err != nil {
			return fmt.Errorf("stage episode %q: %w", current.Title, err)
		}
		staged = append(staged, stagedPath)
		copies = append(copies, FileCopy{Source: stagedPath, Relative: relative})
	}

	plan, err := BuildFilePlan(managed, copies, playlists, keep)
	if err != nil {
		return fmt.Errorf("build episode sync plan: %w", err)
	}
	if err := ApplyFilePlan(ctx, deviceRoot, plan); err != nil {
		return fmt.Errorf("apply episode sync plan: %w", err)
	}
	return nil
}

func reusableFile(deviceRoot, relative string, expectedLength int64) (bool, error) {
	fileInfo, err := os.Lstat(filepath.Join(deviceRoot, filepath.FromSlash(relative)))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Size() == 0 {
		return false, nil
	}
	if expectedLength > 0 && fileInfo.Size() != expectedLength {
		return false, nil
	}
	return true, nil
}
