package sync

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/state"
)

// RefreshFeed fetches and persists the current episodes for one feed.
func RefreshFeed(
	ctx context.Context,
	repository state.Repository,
	client *http.Client,
	feedID int64,
) error {
	return RefreshFeedWithArchive(ctx, repository, client, feedID, false)
}

// RefreshFeedWithArchive retains known historical episodes for an archive
// feed when the source RSS only exposes a recent window.
func RefreshFeedWithArchive(
	ctx context.Context,
	repository state.Repository,
	client *http.Client,
	feedID int64,
	archive bool,
) error {
	current, err := repository.Load()
	if err != nil {
		return fmt.Errorf("load state for feed refresh: %w", err)
	}

	feedIndex := -1
	for i, f := range current.Feeds {
		if f.ID == feedID {
			feedIndex = i
			break
		}
	}
	if feedIndex == -1 {
		return fmt.Errorf("refresh feed %d: feed not found", feedID)
	}

	result, err := feed.FetchRSS(ctx, client, current.Feeds[feedIndex])
	if err != nil {
		return err
	}
	if result.NotModified {
		return nil
	}

	refreshedFeed, episodes, err := feed.ParseRSS(
		bytes.NewReader(result.Body),
		current.Feeds[feedIndex],
	)
	if err != nil {
		return fmt.Errorf("refresh feed %d: %w", feedID, err)
	}
	if len(episodes) == 0 {
		for _, existing := range current.Episodes {
			if existing.FeedID == feedID {
				return fmt.Errorf("refresh feed %d: refusing to replace existing episodes with an empty feed", feedID)
			}
		}
	}
	refreshedFeed.ETag = result.ETag
	refreshedFeed.LastModified = result.LastModified
	current.Feeds[feedIndex] = refreshedFeed

	retainedEpisodes := current.Episodes[:0]
	for _, existing := range current.Episodes {
		if existing.FeedID != feedID {
			retainedEpisodes = append(retainedEpisodes, existing)
		}
	}
	if archive {
		known := make(map[string]struct{}, len(episodes))
		for _, refreshed := range episodes {
			known[refreshed.IdentityKey()] = struct{}{}
		}
		for _, existing := range current.Episodes {
			if existing.FeedID == feedID {
				if _, exists := known[existing.IdentityKey()]; !exists {
					episodes = append(episodes, existing)
				}
			}
		}
	}
	current.Episodes = append(retainedEpisodes, episodes...)

	if err := repository.Save(current); err != nil {
		return fmt.Errorf("save refreshed feed %d: %w", feedID, err)
	}
	return nil
}
