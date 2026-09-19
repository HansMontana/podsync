package selection

import (
	"fmt"
	"sort"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/playback"
	"github.com/HansMontana/podsync/internal/state"
)

// Feed returns episodes selected by one configured logical feed.
func Feed(cfg config.Config, current state.State, feedID string, playbackStates map[string]playback.State, unplayedOnly bool) ([]episode.Episode, error) {
	for _, logical := range cfg.Feeds {
		if logical.ID == feedID {
			return FeedOrderedLimit(cfg, current, feedID, playbackStates, unplayedOnly || logical.UnplayedOnly, logical.Order, logical.Limit)
		}
	}
	return nil, fmt.Errorf("logical feed %q not found", feedID)
}

// FeedForSync selects the newest eligible episodes for device storage. Playlist
// order is applied separately when playlist content is generated.
func FeedForSync(cfg config.Config, current state.State, feedID string, playbackStates map[string]playback.State, unplayedOnly bool) ([]episode.Episode, error) {
	for _, logical := range cfg.Feeds {
		if logical.ID == feedID {
			return FeedOrderedLimit(cfg, current, feedID, playbackStates, unplayedOnly || logical.UnplayedOnly, config.NewestFirst, logical.Limit)
		}
	}
	return nil, fmt.Errorf("logical feed %q not found", feedID)
}

func FeedOrdered(cfg config.Config, current state.State, feedID string, playbackStates map[string]playback.State, unplayedOnly bool, order config.Order) ([]episode.Episode, error) {
	return FeedOrderedLimit(cfg, current, feedID, playbackStates, unplayedOnly, order, 0)
}

func FeedOrderedLimit(cfg config.Config, current state.State, feedID string, playbackStates map[string]playback.State, unplayedOnly bool, order config.Order, limit int) ([]episode.Episode, error) {
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
	normalizedURL, err := feed.NormalizeURL(source.URL)
	if err != nil {
		return nil, fmt.Errorf("normalize source feed %q: %w", source.ID, err)
	}

	var sourceID int64
	for _, known := range current.Feeds {
		knownURL, normalizeErr := feed.NormalizeURL(known.URL)
		if normalizeErr == nil && knownURL == normalizedURL {
			sourceID = known.ID
			break
		}
	}
	if sourceID == 0 {
		return nil, fmt.Errorf("source feed %q is not in state", source.ID)
	}

	selected := make([]episode.Episode, 0)
	for _, candidate := range current.Episodes {
		if candidate.FeedID != sourceID || !logical.Filter.Matches(candidate) {
			continue
		}
		selected = append(selected, candidate)
	}

	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].PublishedAt.Equal(selected[j].PublishedAt) {
			return selected[i].IdentityKey() < selected[j].IdentityKey()
		}
		if order == config.OldestFirst {
			return selected[i].PublishedAt.Before(selected[j].PublishedAt)
		}
		return selected[i].PublishedAt.After(selected[j].PublishedAt)
	})

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
