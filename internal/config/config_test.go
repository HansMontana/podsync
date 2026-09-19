package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/state"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := t.TempDir() + "/Podsync/podsync.toml"
	want := Config{
		Sources: []SourceFeed{{ID: "news", URL: "https://example.com/news.xml"}},
		Feeds:   []LogicalFeed{{ID: "world", Title: "World", Source: "news", Order: NewestFirst}},
		Briefings: []Briefing{{
			ID:       "morning",
			Title:    "Morning",
			Sections: []BriefingSection{{Feed: "world", Order: OldestFirst, Limit: 3, UnplayedOnly: true}},
		}},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got config %+v, want %+v", got, want)
	}
}

func TestParseConfig(t *testing.T) {
	input := `
[[source]]
id = "news"
url = "https://example.com/news.xml"

[[feed]]
id = "world"
title = "World News"
source = "news"
order = "newest_first"

[feed.filter]
title_contains = "World"

[[briefing]]
id = "morning"
title = "Morning Briefing"

[[briefing.section]]
feed = "world"
order = "oldest_first"
limit = 2
unplayed_only = true
`

	cfg, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}
	if cfg.Sources[0].ID != "news" || cfg.Feeds[0].Filter.TitleContains != "World" {
		t.Fatalf("got config %+v", cfg)
	}
	if cfg.Briefings[0].Sections[0].UnplayedOnly != true {
		t.Fatalf("got briefing %+v", cfg.Briefings[0])
	}
}

func TestEnsureSourcesAddsMissingFeedsAndPreservesExistingMetadata(t *testing.T) {
	cfg := Config{Sources: []SourceFeed{
		{ID: "existing", URL: "https://example.com/existing.xml"},
		{ID: "new", URL: "https://example.com/new.xml"},
	}}
	current := state.State{Feeds: []feed.Feed{{ID: 4, Name: "Existing name", URL: "https://EXAMPLE.com:443/existing.xml", ETag: `"etag"`}}}
	got, err := cfg.EnsureSources(current)
	if err != nil {
		t.Fatalf("EnsureSources() returned error: %v", err)
	}
	want := state.State{Feeds: []feed.Feed{
		{ID: 4, Name: "Existing name", URL: "https://EXAMPLE.com:443/existing.xml", ETag: `"etag"`},
		{ID: 5, Name: "new", URL: "https://example.com/new.xml"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got state %+v, want %+v", got, want)
	}
}

func TestConfigFilterMatchesCaseInsensitively(t *testing.T) {
	filter := Filter{TitleContains: "world", DescriptionContains: "Politics"}
	if !filter.Matches(episode.Episode{Title: "WORLD today", Description: "Politics and context"}) {
		t.Fatal("filter did not match episode")
	}
	if filter.Matches(episode.Episode{Title: "Sports", Description: "Politics and context"}) {
		t.Fatal("filter matched an unrelated title")
	}
}

func TestConfigRejectsUnknownLogicalFeedSource(t *testing.T) {
	input := `
[[feed]]
id = "world"
source = "missing"
order = "newest_first"
`

	if _, err := Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse() accepted an unknown source")
	}
}

func TestConfigRejectsInvalidBriefingLimit(t *testing.T) {
	input := `
[[source]]
id = "news"
url = "https://example.com/news.xml"

[[feed]]
id = "world"
source = "news"
order = "newest_first"

[[briefing]]
id = "morning"

[[briefing.section]]
feed = "world"
order = "newest_first"
limit = 0
`

	if _, err := Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse() accepted a non-positive briefing limit")
	}
}

func TestConfigRejectsNegativeLogicalFeedLimit(t *testing.T) {
	input := `
[[source]]
id = "news"
url = "https://example.com/news.xml"

[[feed]]
id = "news"
source = "news"
order = "newest_first"
limit = -1
`
	if _, err := Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse() accepted a negative logical feed limit")
	}
}

func TestParseRejectsUnknownKeysAndUnsafeIDs(t *testing.T) {
	for _, input := range []string{
		"unknown = true\n",
		"[[source]]\nid = \"../escape\"\nurl = \"https://example.com/feed\"\n",
		"[[source]]\nid = \"news\"\nurl = \"https://example.com/feed\"\n[[feed]]\nid = \"news\"\nsource = \"news\"\norder = \"newest_first\"\n[[briefing]]\nid = \"news\"\n",
	} {
		if _, err := Parse(strings.NewReader(input)); err == nil {
			t.Fatalf("Parse() accepted unsafe configuration %q", input)
		}
	}
}
