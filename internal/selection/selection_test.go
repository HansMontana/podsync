package selection

import (
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/playback"
	"github.com/HansMontana/podsync/internal/state"
)

func TestFeedSelectsNewestUnplayedEpisodesAndFilters(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "world", Source: "news", Order: config.NewestFirst, Filter: config.Filter{TitleContains: "World"}}},
	}
	newest := episode.Episode{FeedID: 1, GUID: "new", Title: "World latest", PublishedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)}
	oldest := episode.Episode{FeedID: 1, GUID: "old", Title: "World earlier", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	other := episode.Episode{FeedID: 1, GUID: "other", Title: "Sports latest", PublishedAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)}

	got, err := Feed(cfg, state.State{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}}, Episodes: []episode.Episode{oldest, newest, other}}, "world", map[string]playback.State{
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
	got, err := Feed(cfg, state.State{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}}, Episodes: []episode.Episode{episodeValue}}, "news", nil, true)
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
	got, err := Feed(cfg, state.State{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/news.xml"}}, Episodes: []episode.Episode{later, earlier}}, "news", nil, false)
	if err != nil {
		t.Fatalf("Feed() returned error: %v", err)
	}
	if len(got) != 2 || got[0].GUID != "earlier" {
		t.Fatalf("got episodes %+v", got)
	}
}
