package sync

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/HansMontana/podsync/internal/download"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/media"
	"github.com/HansMontana/podsync/internal/metadata"
)

type ProgressFunc func(completed, total int, current episode.Episode, reused bool)

// Episodes downloads selected episodes to host staging, then applies the
// complete device file plan. Device deletions happen only after all downloads
// and device writes succeed.
func Episodes(ctx context.Context, client *http.Client, stagingDir, deviceRoot string, episodes []episode.Episode, playlists []PlaylistFile, managed []string) error {
	return EpisodesWithProgress(ctx, client, stagingDir, deviceRoot, episodes, playlists, managed, nil, nil, nil)
}

func EpisodesWithProgress(ctx context.Context, client *http.Client, stagingDir, deviceRoot string, episodes []episode.Episode, playlists []PlaylistFile, managed []string, feedNames map[int64]string, progress ProgressFunc, fileProgress FileProgressFunc) error {
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
			changed := false
			if isMP3(current, relative) {
				stagedPath, stageErr := stageExisting(deviceRoot, relative, stagingDir)
				if stageErr != nil {
					return fmt.Errorf("stage existing episode %q: %w", current.Title, stageErr)
				}
				staged = append(staged, stagedPath)
				var metadataErr error
				changed, metadataErr = metadata.NormalizeMP3(stagedPath, feedNames[current.FeedID], current)
				if metadataErr != nil {
					return fmt.Errorf("normalize metadata for %q: %w", current.Title, metadataErr)
				}
				if changed {
					copies = append(copies, FileCopy{Source: stagedPath, Relative: relative})
				} else {
					keep = append(keep, relative)
				}
			} else {
				keep = append(keep, relative)
			}
			if progress != nil {
				progress(len(keep)+len(copies), len(episodes), current, !changed)
			}
			continue
		}

		stagedPath, err := download.Episode(ctx, client, current, stagingDir)
		if err != nil {
			return fmt.Errorf("stage episode %q: %w", current.Title, err)
		}
		staged = append(staged, stagedPath)
		if isMP3(current, relative) {
			if _, metadataErr := metadata.NormalizeMP3(stagedPath, feedNames[current.FeedID], current); metadataErr != nil {
				return fmt.Errorf("normalize metadata for %q: %w", current.Title, metadataErr)
			}
		}
		copies = append(copies, FileCopy{Source: stagedPath, Relative: relative})
		if progress != nil {
			progress(len(keep)+len(copies), len(episodes), current, false)
		}
	}

	plan, err := BuildFilePlan(managed, copies, playlists, keep)
	if err != nil {
		return fmt.Errorf("build episode sync plan: %w", err)
	}
	if err := ApplyFilePlanWithProgress(ctx, deviceRoot, plan, fileProgress); err != nil {
		return fmt.Errorf("apply episode sync plan: %w", err)
	}
	return nil
}

func isMP3(current episode.Episode, relative string) bool {
	return strings.EqualFold(strings.TrimSpace(current.Enclosure.Type), "audio/mpeg") || strings.EqualFold(filepath.Ext(relative), ".mp3")
}

func stageExisting(deviceRoot, relative, stagingDir string) (string, error) {
	sourcePath, err := safeDevicePath(deviceRoot, relative, false)
	if err != nil {
		return "", err
	}
	input, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer input.Close()
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", err
	}
	output, err := os.CreateTemp(stagingDir, "podsync-existing-*")
	if err != nil {
		return "", err
	}
	path := output.Name()
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := output.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func reusableFile(deviceRoot, relative string, expectedLength int64) (bool, error) {
	path, err := safeDevicePath(deviceRoot, relative, false)
	if err != nil {
		return false, err
	}
	fileInfo, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Size() == 0 {
		return false, nil
	}
	if expectedLength > 0 && !strings.EqualFold(filepath.Ext(relative), ".mp3") && fileInfo.Size() != expectedLength {
		return false, nil
	}
	return true, nil
}
