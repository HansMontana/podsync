package selection

import (
	"fmt"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/playback"
)

func TestFeedSelectsNewestUnplayedEpisodesAndFilters(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "world", Source: "news", Order: config.NewestFirst, Filter: config.Filter{TitleContains: "World"}}},
	}
	newest := episode.Episode{FeedID: 1, GUID: "new", Title: "World latest", PublishedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)}
	oldest := episode.Episode{FeedID: 1, GUID: "old", Title: "World earlier", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	other := episode.Episode{FeedID: 1, GUID: "other", Title: "Sports latest", PublishedAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)}

	got, err := Feed(cfg, catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}}, Episodes: []episode.Episode{oldest, newest, other}}, "world", map[string]playback.State{
		newest.IdentityKey(): {Known: true, PlayCount: 1},
	}, true)
	if err != nil {
		t.Fatalf("Feed() returned error: %v", err)
	}
	if len(got) != 1 || got[0].GUID != "old" {
		t.Fatalf("got episodes %+v", got)
	}
}

func TestFeedTreatsUnknownPlaybackAsUnplayed(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "news", Source: "news", Order: config.OldestFirst}},
	}
	episodeValue := episode.Episode{FeedID: 1, GUID: "episode-1", PublishedAt: time.Now()}
	got, err := Feed(cfg, catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}}, Episodes: []episode.Episode{episodeValue}}, "news", nil, true)
	if err != nil {
		t.Fatalf("Feed() returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatal("unknown playback state was incorrectly treated as played")
	}
}

func TestFeedSupportsOldestFirst(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "news", Source: "news", Order: config.OldestFirst}},
	}
	earlier := episode.Episode{FeedID: 1, GUID: "earlier", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	later := episode.Episode{FeedID: 1, GUID: "later", PublishedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)}
	got, err := Feed(cfg, catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}}, Episodes: []episode.Episode{later, earlier}}, "news", nil, false)
	if err != nil {
		t.Fatalf("Feed() returned error: %v", err)
	}
	if len(got) != 2 || got[0].GUID != "earlier" {
		t.Fatalf("got episodes %+v", got)
	}
}

func TestFeedForSyncSelectsNewestEpisodesRegardlessOfPlaylistOrder(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "news", Source: "news", Order: config.OldestFirst, Limit: 2}},
	}
	episodes := []episode.Episode{
		{FeedID: 1, GUID: "oldest", PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{FeedID: 1, GUID: "middle", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)},
		{FeedID: 1, GUID: "newest", PublishedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)},
	}
	got, err := FeedForSync(cfg, catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}}, Episodes: episodes}, "news", nil, false)
	if err != nil {
		t.Fatalf("FeedForSync() returned error: %v", err)
	}
	if len(got) != 2 || got[0].GUID != "newest" || got[1].GUID != "middle" {
		t.Fatalf("got episodes %+v", got)
	}
}

func TestArchiveFeedIgnoresLimitAndPlaybackFilters(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "archive", URL: "https://example.com/archive.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "archive", Source: "archive", Archive: true, Limit: 1, UnplayedOnly: true}},
	}
	episodes := []episode.Episode{
		{FeedID: 1, GUID: "old", PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{FeedID: 1, GUID: "new", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)},
	}
	got, err := FeedForSync(cfg, catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/archive.xml"}}, Episodes: episodes}, "archive", map[string]playback.State{
		"guid:1:new": {Known: true, PlayCount: 1},
	}, false)
	if err != nil {
		t.Fatalf("FeedForSync() returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d episodes, want 2", len(got))
	}
}

func TestFeedUnplayedLimitBackfillsOlderEpisodes(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "news", Source: "news", Order: config.NewestFirst, Limit: 5, UnplayedOnly: true}},
	}
	episodes := make([]episode.Episode, 0, 6)
	playbackStates := make(map[string]playback.State)
	for i := 0; i < 6; i++ {
		current := episode.Episode{FeedID: 1, GUID: fmt.Sprintf("episode-%d", i), PublishedAt: time.Date(2026, 9, i+1, 0, 0, 0, 0, time.UTC)}
		episodes = append(episodes, current)
		if i == 5 {
			playbackStates[current.IdentityKey()] = playback.State{Known: true, PlayCount: 1}
		}
	}
	got, err := Feed(cfg, catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}}, Episodes: episodes}, "news", playbackStates, false)
	if err != nil {
		t.Fatalf("Feed() returned error: %v", err)
	}
	if len(got) != 5 || got[0].GUID != "episode-4" || got[4].GUID != "episode-0" {
		t.Fatalf("got episodes %+v", got)
	}
}
