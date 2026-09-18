package sync

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/HansMontana/podsync/internal/download"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/media"
)

// Episodes downloads selected episodes to host staging, then applies the
// complete device file plan. Device deletions happen only after all downloads
// and device writes succeed.
func Episodes(ctx context.Context, client *http.Client, stagingDir, deviceRoot string, episodes []episode.Episode, playlists []PlaylistFile, managed []string) error {
	var copies []FileCopy
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
		result, err := download.Episode(ctx, client, current, stagingDir)
		if err != nil {
			return fmt.Errorf("stage episode %q: %w", current.Title, err)
		}
		staged = append(staged, result.Path)
		copies = append(copies, FileCopy{Source: result.Path, Relative: media.RelativePath(current)})
	}

	plan, err := BuildFilePlan(managed, copies, playlists)
	if err != nil {
		return fmt.Errorf("build episode sync plan: %w", err)
	}
	if err := ApplyFilePlan(ctx, deviceRoot, plan); err != nil {
		return fmt.Errorf("apply episode sync plan: %w", err)
	}
	return nil
}
