package config

import (
	"strings"
	"testing"

	"github.com/HansMontana/podsync/internal/episode"
)

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
