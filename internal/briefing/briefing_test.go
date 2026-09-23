package briefing

import (
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/playback"
)

func TestBuildOrdersBriefingSectionsAndLimitsEpisodes(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{
			{ID: "one", URL: "https://example.com/one.xml"},
			{ID: "two", URL: "https://example.com/two.xml"},
		},
		Feeds: []config.LogicalFeed{
			{ID: "first", Source: "one", Order: config.NewestFirst},
			{ID: "second", Source: "two", Order: config.NewestFirst},
		},
		Briefings: []config.Briefing{
			{ID: "morning", Sections: []config.BriefingSection{
				{Feed: "first", Order: config.OldestFirst, Limit: 1, UnplayedOnly: true},
				{Feed: "second", Order: config.NewestFirst, Limit: 2},
			}},
		},
	}
	firstOld := episode.Episode{FeedID: 1, GUID: "first-old", PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	firstNew := episode.Episode{FeedID: 1, GUID: "first-new", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	secondOld := episode.Episode{FeedID: 2, GUID: "second-old", PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	secondNew := episode.Episode{FeedID: 2, GUID: "second-new", PublishedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)}

	plan, err := Build(cfg, catalog.Catalog{
		Feeds:    []feed.Feed{{ID: 1, URL: "https://example.com/one.xml"}, {ID: 2, URL: "https://example.com/two.xml"}},
		Episodes: []episode.Episode{firstNew, firstOld, secondOld, secondNew},
	}, "morning", map[string]playback.State{firstOld.IdentityKey(): {Known: true, PlayCount: 1}})
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if len(plan.Episodes) != 2 || plan.Episodes[0].GUID != "second-new" || plan.Episodes[1].GUID != "second-old" {
		t.Fatalf("got plan episodes %+v", plan.Episodes)
	}
}

func TestBuildSkipsOnlyPlayedBriefingSection(t *testing.T) {
	cfg := config.Config{
		Sources: []config.SourceFeed{{ID: "one", URL: "https://example.com/one.xml"}, {ID: "two", URL: "https://example.com/two.xml"}},
		Feeds:   []config.LogicalFeed{{ID: "first", Source: "one", Order: config.NewestFirst}, {ID: "second", Source: "two", Order: config.NewestFirst}},
		Briefings: []config.Briefing{{ID: "morning", Sections: []config.BriefingSection{
			{Feed: "first", Order: config.NewestFirst, Limit: 1, UnplayedOnly: true},
			{Feed: "second", Order: config.NewestFirst, Limit: 1, UnplayedOnly: true},
		}}},
	}
	first := episode.Episode{FeedID: 1, GUID: "first", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	second := episode.Episode{FeedID: 2, GUID: "second", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	plan, err := Build(cfg, catalog.Catalog{
		Feeds:    []feed.Feed{{ID: 1, URL: "https://example.com/one.xml"}, {ID: 2, URL: "https://example.com/two.xml"}},
		Episodes: []episode.Episode{first, second},
	}, "morning", map[string]playback.State{first.IdentityKey(): {Known: true, PlayCount: 1}})
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if len(plan.Episodes) != 1 || plan.Episodes[0].GUID != "second" {
		t.Fatalf("got plan episodes %+v", plan.Episodes)
	}
}
