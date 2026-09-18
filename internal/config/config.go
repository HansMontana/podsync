package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/state"
)

type Order string

const (
	NewestFirst Order = "newest_first"
	OldestFirst Order = "oldest_first"
)

type Config struct {
	Sources   []SourceFeed  `toml:"source"`
	Feeds     []LogicalFeed `toml:"feed"`
	Briefings []Briefing    `toml:"briefing"`
}

type SourceFeed struct {
	ID  string `toml:"id"`
	URL string `toml:"url"`
}

type LogicalFeed struct {
	ID     string `toml:"id"`
	Title  string `toml:"title"`
	Source string `toml:"source"`
	Order  Order  `toml:"order"`
	Filter Filter `toml:"filter"`
}

type Filter struct {
	TitleContains       string `toml:"title_contains"`
	DescriptionContains string `toml:"description_contains"`
}

type Briefing struct {
	ID       string            `toml:"id"`
	Title    string            `toml:"title"`
	Sections []BriefingSection `toml:"section"`
}

type BriefingSection struct {
	Feed         string `toml:"feed"`
	Order        Order  `toml:"order"`
	Limit        int    `toml:"limit"`
	UnplayedOnly bool   `toml:"unplayed_only"`
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	return Parse(file)
}

func Parse(r io.Reader) (Config, error) {
	var cfg Config
	if _, err := toml.NewDecoder(r).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	sources := make(map[string]struct{}, len(c.Sources))
	urls := make(map[string]struct{}, len(c.Sources))
	for _, source := range c.Sources {
		if source.ID == "" {
			return fmt.Errorf("source ID is required")
		}
		if _, exists := sources[source.ID]; exists {
			return fmt.Errorf("duplicate source ID %q", source.ID)
		}
		normalized, err := feed.NormalizeURL(source.URL)
		if err != nil {
			return fmt.Errorf("source %q URL: %w", source.ID, err)
		}
		if _, exists := urls[normalized]; exists {
			return fmt.Errorf("duplicate source URL %q", source.URL)
		}
		sources[source.ID] = struct{}{}
		urls[normalized] = struct{}{}
	}

	logicalFeeds := make(map[string]struct{}, len(c.Feeds))
	for _, logical := range c.Feeds {
		if logical.ID == "" {
			return fmt.Errorf("logical feed ID is required")
		}
		if _, exists := logicalFeeds[logical.ID]; exists {
			return fmt.Errorf("duplicate logical feed ID %q", logical.ID)
		}
		if _, exists := sources[logical.Source]; !exists {
			return fmt.Errorf("logical feed %q references unknown source %q", logical.ID, logical.Source)
		}
		if err := validateOrder(logical.Order); err != nil {
			return fmt.Errorf("logical feed %q: %w", logical.ID, err)
		}
		logicalFeeds[logical.ID] = struct{}{}
	}

	briefings := make(map[string]struct{}, len(c.Briefings))
	for _, briefing := range c.Briefings {
		if briefing.ID == "" {
			return fmt.Errorf("briefing ID is required")
		}
		if _, exists := briefings[briefing.ID]; exists {
			return fmt.Errorf("duplicate briefing ID %q", briefing.ID)
		}
		if len(briefing.Sections) == 0 {
			return fmt.Errorf("briefing %q has no sections", briefing.ID)
		}
		for i, section := range briefing.Sections {
			if _, exists := logicalFeeds[section.Feed]; !exists {
				return fmt.Errorf("briefing %q section %d references unknown feed %q", briefing.ID, i, section.Feed)
			}
			if err := validateOrder(section.Order); err != nil {
				return fmt.Errorf("briefing %q section %d: %w", briefing.ID, i, err)
			}
			if section.Limit <= 0 {
				return fmt.Errorf("briefing %q section %d limit must be positive", briefing.ID, i)
			}
		}
		briefings[briefing.ID] = struct{}{}
	}

	return nil
}

func validateOrder(order Order) error {
	if order != NewestFirst && order != OldestFirst {
		return fmt.Errorf("order must be %q or %q", NewestFirst, OldestFirst)
	}
	return nil
}

func (f Filter) Matches(e episode.Episode) bool {
	return containsFold(e.Title, f.TitleContains) &&
		containsFold(e.Description, f.DescriptionContains)
}

// EnsureSources adds configured source feeds that are missing from durable
// state while preserving existing IDs, names, and HTTP cache metadata.
func (c Config) EnsureSources(current state.State) (state.State, error) {
	if err := c.Validate(); err != nil {
		return state.State{}, fmt.Errorf("validate config: %w", err)
	}
	result := current
	byURL := make(map[string]feed.Feed, len(result.Feeds))
	maxID := int64(0)
	for _, known := range result.Feeds {
		normalized, err := feed.NormalizeURL(known.URL)
		if err != nil {
			return state.State{}, fmt.Errorf("normalize existing feed %d: %w", known.ID, err)
		}
		byURL[normalized] = known
		if known.ID > maxID {
			maxID = known.ID
		}
	}
	for _, source := range c.Sources {
		normalized, err := feed.NormalizeURL(source.URL)
		if err != nil {
			return state.State{}, fmt.Errorf("normalize source %q: %w", source.ID, err)
		}
		if _, exists := byURL[normalized]; exists {
			continue
		}
		maxID++
		known := feed.Feed{ID: maxID, Name: source.ID, URL: normalized}
		result.Feeds = append(result.Feeds, known)
		byURL[normalized] = known
	}
	if err := result.Validate(); err != nil {
		return state.State{}, fmt.Errorf("validate reconciled state: %w", err)
	}
	return result, nil
}

func containsFold(value, query string) bool {
	return query == "" || strings.Contains(strings.ToLower(value), strings.ToLower(query))
}
