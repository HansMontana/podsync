package curation

import (
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/domain/playback"
)

func TestBuildOrdersBriefingSectionsAndLimitsEpisodes(t *testing.T) {
	cfg := Config{
		Sources: []SourceFeed{
			{ID: "one", URL: "https://example.com/one.xml"},
			{ID: "two", URL: "https://example.com/two.xml"},
		},
		Feeds: []LogicalFeed{
			{ID: "first", Source: "one", Order: NewestFirst},
			{ID: "second", Source: "two", Order: NewestFirst},
		},
		Briefings: []Briefing{
			{ID: "morning", Sections: []BriefingSection{
				{Feed: "first", Order: OldestFirst, Limit: 1, UnplayedOnly: true},
				{Feed: "second", Order: NewestFirst, Limit: 2},
			}},
		},
	}
	firstOld := catalog.Episode{FeedID: 1, GUID: "first-old", PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	firstNew := catalog.Episode{FeedID: 1, GUID: "first-new", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	secondOld := catalog.Episode{FeedID: 2, GUID: "second-old", PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	secondNew := catalog.Episode{FeedID: 2, GUID: "second-new", PublishedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)}

	plan, err := Build(cfg, catalog.Catalog{
		Feeds:    []catalog.Feed{{ID: 1, URL: "https://example.com/one.xml"}, {ID: 2, URL: "https://example.com/two.xml"}},
		Episodes: []catalog.Episode{firstNew, firstOld, secondOld, secondNew},
	}, "morning", map[string]playback.State{firstOld.IdentityKey(): {Known: true, PlayCount: 1}})
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if len(plan.Episodes) != 2 || plan.Episodes[0].GUID != "second-new" || plan.Episodes[1].GUID != "second-old" {
		t.Fatalf("got plan episodes %+v", plan.Episodes)
	}
}

func TestBuildSkipsOnlyPlayedBriefingSection(t *testing.T) {
	cfg := Config{
		Sources: []SourceFeed{{ID: "one", URL: "https://example.com/one.xml"}, {ID: "two", URL: "https://example.com/two.xml"}},
		Feeds:   []LogicalFeed{{ID: "first", Source: "one", Order: NewestFirst}, {ID: "second", Source: "two", Order: NewestFirst}},
		Briefings: []Briefing{{ID: "morning", Sections: []BriefingSection{
			{Feed: "first", Order: NewestFirst, Limit: 1, UnplayedOnly: true},
			{Feed: "second", Order: NewestFirst, Limit: 1, UnplayedOnly: true},
		}}},
	}
	first := catalog.Episode{FeedID: 1, GUID: "first", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	second := catalog.Episode{FeedID: 2, GUID: "second", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	plan, err := Build(cfg, catalog.Catalog{
		Feeds:    []catalog.Feed{{ID: 1, URL: "https://example.com/one.xml"}, {ID: 2, URL: "https://example.com/two.xml"}},
		Episodes: []catalog.Episode{first, second},
	}, "morning", map[string]playback.State{first.IdentityKey(): {Known: true, PlayCount: 1}})
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if len(plan.Episodes) != 1 || plan.Episodes[0].GUID != "second" {
		t.Fatalf("got plan episodes %+v", plan.Episodes)
	}
}

func TestBuildDoesNotBackfillSkippedBriefingSection(t *testing.T) {
	cfg := Config{
		Sources: []SourceFeed{{ID: "one", URL: "https://example.com/one.xml"}},
		Feeds:   []LogicalFeed{{ID: "first", Source: "one", Order: NewestFirst}},
		Briefings: []Briefing{{ID: "morning", Sections: []BriefingSection{
			{Feed: "first", Order: NewestFirst, Limit: 1, UnplayedOnly: true},
		}}},
	}
	newest := catalog.Episode{FeedID: 1, GUID: "new", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	older := catalog.Episode{FeedID: 1, GUID: "old", PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	plan, err := Build(cfg, catalog.Catalog{
		Feeds:    []catalog.Feed{{ID: 1, URL: "https://example.com/one.xml"}},
		Episodes: []catalog.Episode{newest, older},
	}, "morning", map[string]playback.State{newest.IdentityKey(): {Known: true, Skipped: true}})
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if len(plan.Episodes) != 0 {
		t.Fatalf("got plan episodes %+v", plan.Episodes)
	}
}

func TestBuildFiltersConsumedEpisodesWithinBriefingWindowWithoutBackfill(t *testing.T) {
	cfg := Config{
		Sources: []SourceFeed{{ID: "one", URL: "https://example.com/one.xml"}},
		Feeds:   []LogicalFeed{{ID: "first", Source: "one", Order: NewestFirst}},
		Briefings: []Briefing{{ID: "morning", Sections: []BriefingSection{
			{Feed: "first", Order: NewestFirst, Limit: 3, UnplayedOnly: true},
		}}},
	}
	newest := catalog.Episode{FeedID: 1, GUID: "newest", PublishedAt: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)}
	consumed := catalog.Episode{FeedID: 1, GUID: "consumed", PublishedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)}
	third := catalog.Episode{FeedID: 1, GUID: "third", PublishedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}
	older := catalog.Episode{FeedID: 1, GUID: "older", PublishedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}

	plan, err := Build(cfg, catalog.Catalog{
		Feeds:    []catalog.Feed{{ID: 1, URL: "https://example.com/one.xml"}},
		Episodes: []catalog.Episode{newest, consumed, third, older},
	}, "morning", map[string]playback.State{consumed.IdentityKey(): {Known: true, Skipped: true}})
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if len(plan.Episodes) != 2 || plan.Episodes[0].GUID != "newest" || plan.Episodes[1].GUID != "third" {
		t.Fatalf("got plan episodes %+v", plan.Episodes)
	}
}
