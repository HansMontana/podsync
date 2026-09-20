package sync

import (
	"context"
	"errors"
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
type WarningFunc func(message string)

// Episodes downloads selected episodes to host staging, then applies the
// complete device file plan. Device deletions happen only after all downloads
// and device writes succeed.
func Episodes(ctx context.Context, client *http.Client, stagingDir, deviceRoot string, episodes []episode.Episode, playlists []PlaylistFile, managed []string) error {
	return EpisodesWithProgress(ctx, client, stagingDir, deviceRoot, episodes, playlists, managed, nil, nil, nil)
}

func EpisodesWithProgress(ctx context.Context, client *http.Client, stagingDir, deviceRoot string, episodes []episode.Episode, playlists []PlaylistFile, managed []string, feedNames map[int64]string, progress ProgressFunc, fileProgress FileProgressFunc) error {
	return episodesWithProgress(ctx, client, stagingDir, deviceRoot, episodes, playlists, managed, feedNames, progress, fileProgress, nil)
}

// EpisodesWithProgressAndWarnings continues past unreadable existing device
// files, warning the caller when it must retain or replace one.
func EpisodesWithProgressAndWarnings(ctx context.Context, client *http.Client, stagingDir, deviceRoot string, episodes []episode.Episode, playlists []PlaylistFile, managed []string, feedNames map[int64]string, progress ProgressFunc, fileProgress FileProgressFunc, warning WarningFunc) error {
	return episodesWithProgress(ctx, client, stagingDir, deviceRoot, episodes, playlists, managed, feedNames, progress, fileProgress, warning)
}

func episodesWithProgress(ctx context.Context, client *http.Client, stagingDir, deviceRoot string, episodes []episode.Episode, playlists []PlaylistFile, managed []string, feedNames map[int64]string, progress ProgressFunc, fileProgress FileProgressFunc, warning WarningFunc) error {
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
					var readErr *existingReadError
					if !errors.As(stageErr, &readErr) {
						return fmt.Errorf("stage existing episode %q: %w", current.Title, stageErr)
					}
					warn(warning, fmt.Sprintf("unreadable existing episode %q: %v; redownloading", current.Title, readErr))
					stagedPath, err = download.Episode(ctx, client, current, stagingDir)
					if err != nil {
						keep = append(keep, relative)
						warn(warning, fmt.Sprintf("could not redownload %q: %v; keeping existing file", current.Title, err))
						continue
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
					continue
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
		return "", &existingReadError{err: err}
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
	reader := &existingReader{reader: input}
	if _, err := io.Copy(output, reader); err != nil {
		if reader.err != nil {
			_ = output.Close()
			_ = os.Remove(path)
			return "", &existingReadError{err: reader.err}
		}
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

type existingReadError struct{ err error }

func (e *existingReadError) Error() string { return e.err.Error() }
func (e *existingReadError) Unwrap() error { return e.err }

type existingReader struct {
	reader io.Reader
	err    error
}

func (r *existingReader) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return count, err
}

func warn(warning WarningFunc, message string) {
	if warning != nil {
		warning(message)
	}
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
