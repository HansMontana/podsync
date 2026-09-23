package selection

import (
	"fmt"
	"sort"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/playback"
)

// Feed returns episodes selected by one configured logical feed.
func Feed(cfg config.Config, current catalog.Catalog, feedID string, playbackStates map[string]playback.State, unplayedOnly bool) ([]catalog.Episode, error) {
	for _, logical := range cfg.Feeds {
		if logical.ID == feedID {
			if logical.Archive {
				return FeedOrderedLimit(cfg, current, feedID, playbackStates, false, logical.Order, 0)
			}
			return FeedOrderedLimit(cfg, current, feedID, playbackStates, unplayedOnly || logical.UnplayedOnly, logical.Order, logical.Limit)
		}
	}
	return nil, fmt.Errorf("logical feed %q not found", feedID)
}

// FeedForSync selects the newest eligible episodes for device storage. Playlist
// order is applied separately when playlist content is generated.
func FeedForSync(cfg config.Config, current catalog.Catalog, feedID string, playbackStates map[string]playback.State, unplayedOnly bool) ([]catalog.Episode, error) {
	for _, logical := range cfg.Feeds {
		if logical.ID == feedID {
			if logical.Archive {
				return FeedOrderedLimit(cfg, current, feedID, playbackStates, false, config.NewestFirst, 0)
			}
			return FeedOrderedLimit(cfg, current, feedID, playbackStates, unplayedOnly || logical.UnplayedOnly, config.NewestFirst, logical.Limit)
		}
	}
	return nil, fmt.Errorf("logical feed %q not found", feedID)
}

// FeedForPlaylist returns the same storage window as FeedForSync, reordered
// for presentation in the logical-feed playlist.
func FeedForPlaylist(cfg config.Config, current catalog.Catalog, feedID string, playbackStates map[string]playback.State) ([]catalog.Episode, error) {
	selected, err := FeedForSync(cfg, current, feedID, playbackStates, false)
	if err != nil {
		return nil, err
	}
	for _, logical := range cfg.Feeds {
		if logical.ID == feedID {
			sortEpisodes(selected, logical.Order)
			return selected, nil
		}
	}
	return nil, fmt.Errorf("logical feed %q not found", feedID)
}

func FeedOrdered(cfg config.Config, current catalog.Catalog, feedID string, playbackStates map[string]playback.State, unplayedOnly bool, order config.Order) ([]catalog.Episode, error) {
	return FeedOrderedLimit(cfg, current, feedID, playbackStates, unplayedOnly, order, 0)
}

func FeedOrderedLimit(cfg config.Config, current catalog.Catalog, feedID string, playbackStates map[string]playback.State, unplayedOnly bool, order config.Order, limit int) ([]catalog.Episode, error) {
	var logical *config.LogicalFeed
	for i := range cfg.Feeds {
		if cfg.Feeds[i].ID == feedID {
			logical = &cfg.Feeds[i]
			break
		}
	}
	if logical == nil {
		return nil, fmt.Errorf("logical feed %q not found", feedID)
	}

	var source *config.SourceFeed
	for i := range cfg.Sources {
		if cfg.Sources[i].ID == logical.Source {
			source = &cfg.Sources[i]
			break
		}
	}
	if source == nil {
		return nil, fmt.Errorf("source feed %q not found", logical.Source)
	}
	normalizedURL, err := catalog.NormalizeURL(source.URL)
	if err != nil {
		return nil, fmt.Errorf("normalize source feed %q: %w", source.ID, err)
	}

	var sourceID int64
	for _, known := range current.Feeds {
		knownURL, normalizeErr := catalog.NormalizeURL(known.URL)
		if normalizeErr == nil && knownURL == normalizedURL {
			sourceID = known.ID
			break
		}
	}
	if sourceID == 0 {
		return nil, fmt.Errorf("source feed %q is not in state", source.ID)
	}

	selected := make([]catalog.Episode, 0)
	for _, candidate := range current.Episodes {
		if candidate.FeedID != sourceID || !logical.Filter.Matches(candidate) {
			continue
		}
		selected = append(selected, candidate)
	}

	sortEpisodes(selected, order)

	result := selected[:0]
	for _, candidate := range selected {
		if unplayedOnly && playbackStates[candidate.IdentityKey()].Played() {
			continue
		}
		result = append(result, candidate)
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func sortEpisodes(episodes []catalog.Episode, order config.Order) {
	sort.SliceStable(episodes, func(i, j int) bool {
		if episodes[i].PublishedAt.Equal(episodes[j].PublishedAt) {
			return episodes[i].IdentityKey() < episodes[j].IdentityKey()
		}
		if order == config.OldestFirst {
			return episodes[i].PublishedAt.Before(episodes[j].PublishedAt)
		}
		return episodes[i].PublishedAt.After(episodes[j].PublishedAt)
	})
}
