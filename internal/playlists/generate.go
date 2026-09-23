package playlists

import (
	"fmt"
	"path"

	"github.com/HansMontana/podsync/internal/briefing"
	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/media"
	"github.com/HansMontana/podsync/internal/playback"
	"github.com/HansMontana/podsync/internal/selection"
)

func LogicalFeed(cfg config.Config, current catalog.Catalog, feedID string, playbackStates map[string]playback.State) ([]byte, error) {
	logicalIDs, err := cfg.LogicalFeedIDs(current)
	if err != nil {
		return nil, err
	}
	return LogicalFeedWithResolver(cfg, current, feedID, playbackStates, media.Resolver(logicalIDs))
}

func LogicalFeedWithResolver(cfg config.Config, current catalog.Catalog, feedID string, playbackStates map[string]playback.State, resolver media.Resolver) ([]byte, error) {
	episodes, err := selection.FeedForPlaylist(cfg, current, feedID, playbackStates)
	if err != nil {
		return nil, fmt.Errorf("select logical feed %q: %w", feedID, err)
	}
	return M3U(tracksWithResolver(episodes, resolver)), nil
}

func Briefing(cfg config.Config, current catalog.Catalog, briefingID string, playbackStates map[string]playback.State) ([]byte, error) {
	logicalIDs, err := cfg.LogicalFeedIDs(current)
	if err != nil {
		return nil, err
	}
	return BriefingWithResolver(cfg, current, briefingID, playbackStates, media.Resolver(logicalIDs))
}

func BriefingWithResolver(cfg config.Config, current catalog.Catalog, briefingID string, playbackStates map[string]playback.State, resolver media.Resolver) ([]byte, error) {
	plan, err := briefing.Build(cfg, current, briefingID, playbackStates)
	if err != nil {
		return nil, fmt.Errorf("build briefing %q: %w", briefingID, err)
	}
	return M3U(tracksWithResolver(plan.Episodes, resolver)), nil
}

func tracks(episodes []episode.Episode) []Track {
	return tracksWithResolver(episodes, nil)
}

func tracksWithResolver(episodes []episode.Episode, resolver media.Resolver) []Track {
	result := make([]Track, len(episodes))
	for i, current := range episodes {
		result[i] = Track{Episode: current, Path: path.Join("..", resolver.RelativePathFor(current))}
	}
	return result
}
