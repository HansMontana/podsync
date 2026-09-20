package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	ID           string `toml:"id"`
	Title        string `toml:"title"`
	Artist       string `toml:"artist"`
	Source       string `toml:"source"`
	Archive      bool   `toml:"archive"`
	Order        Order  `toml:"order"`
	Limit        int    `toml:"limit"`
	UnplayedOnly bool   `toml:"unplayed_only"`
	Filter       Filter `toml:"filter"`
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

// Save validates and atomically writes configuration to path.
func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".podsync-config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := temporary.Write(encoded.Bytes()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install config: %w", err)
	}
	return nil
}

func Parse(r io.Reader) (Config, error) {
	var cfg Config
	metadata, err := toml.NewDecoder(r).Decode(&cfg)
	if err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("unknown configuration key %q", undecoded[0])
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
		if !validID(source.ID) {
			return fmt.Errorf("source ID %q is invalid", source.ID)
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
		if !validID(logical.ID) {
			return fmt.Errorf("logical feed ID %q is invalid", logical.ID)
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
		if logical.Limit < 0 {
			return fmt.Errorf("logical feed %q limit cannot be negative", logical.ID)
		}
		logicalFeeds[logical.ID] = struct{}{}
	}

	briefings := make(map[string]struct{}, len(c.Briefings))
	for _, briefing := range c.Briefings {
		if !validID(briefing.ID) {
			return fmt.Errorf("briefing ID %q is invalid", briefing.ID)
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
		if _, exists := logicalFeeds[briefing.ID]; exists {
			return fmt.Errorf("briefing ID %q conflicts with logical feed ID", briefing.ID)
		}
		briefings[briefing.ID] = struct{}{}
	}

	return nil
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validID(id string) bool {
	return idPattern.MatchString(id) && id != "." && id != ".."
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

// LogicalFeedIDs returns the first configured logical feed ID for each durable
// source feed. A source may have multiple logical partitions; the first one
// owns the physical media path.
func (c Config) LogicalFeedIDs(current state.State) (map[int64]string, error) {
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	sourceURLs := make(map[string]string, len(c.Sources))
	for _, source := range c.Sources {
		normalized, err := feed.NormalizeURL(source.URL)
		if err != nil {
			return nil, fmt.Errorf("normalize source %q: %w", source.ID, err)
		}
		sourceURLs[source.ID] = normalized
	}
	feedIDs := make(map[string]int64, len(current.Feeds))
	for _, known := range current.Feeds {
		normalized, err := feed.NormalizeURL(known.URL)
		if err != nil {
			return nil, fmt.Errorf("normalize existing feed %d: %w", known.ID, err)
		}
		feedIDs[normalized] = known.ID
	}
	result := make(map[int64]string)
	for _, logical := range c.Feeds {
		sourceURL := sourceURLs[logical.Source]
		feedID := feedIDs[sourceURL]
		if feedID == 0 {
			continue
		}
		if _, exists := result[feedID]; !exists {
			result[feedID] = logical.ID
		}
	}
	return result, nil
}

func containsFold(value, query string) bool {
	return query == "" || strings.Contains(strings.ToLower(value), strings.ToLower(query))
}
