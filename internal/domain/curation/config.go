package curation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

type Order string

const (
	NewestFirst Order = "newest_first"
	OldestFirst Order = "oldest_first"
)

type Config struct {
	Sources      []SourceFeed
	Feeds        []LogicalFeed
	Briefings    []Briefing
	Integrations IntegrationConfig
}

type IntegrationConfig struct {
	Rockbox RockboxConfig
}

type RockboxConfig struct {
	TagCacheUpdateMarker bool
}

type SourceFeed struct {
	ID  string
	URL string
}

type LogicalFeed struct {
	ID           string
	Title        string
	Artist       string
	Source       string
	Archive      bool
	Order        Order
	Limit        int
	UnplayedOnly bool
	Filter       Filter
}

type Filter struct {
	TitleContains       string
	DescriptionContains string
	TitleExcludes       []string
}

type Briefing struct {
	ID       string
	Title    string
	Sections []BriefingSection
}

type BriefingSection struct {
	Feed         string
	Order        Order
	Limit        int
	UnplayedOnly bool
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
		normalized, err := catalog.NormalizeURL(source.URL)
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
	playlistNames := make(map[string]string, len(c.Feeds)+len(c.Briefings))
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
		if err := registerPlaylistName(playlistNames, logical.ID, logical.Title); err != nil {
			return fmt.Errorf("logical feed %q: %w", logical.ID, err)
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
		if err := registerPlaylistName(playlistNames, briefing.ID, briefing.Title); err != nil {
			return fmt.Errorf("briefing %q: %w", briefing.ID, err)
		}
		briefings[briefing.ID] = struct{}{}
	}

	return nil
}

func PlaylistFilename(title, fallback string) string {
	if strings.TrimSpace(title) == "" {
		title = fallback
	}

	var result strings.Builder
	for _, character := range strings.TrimSpace(title) {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || character == ' ' || strings.ContainsRune("._-'()&", character) {
			result.WriteRune(character)
			continue
		}
		result.WriteByte(' ')
	}

	name := strings.Trim(strings.Join(strings.Fields(result.String()), " "), " .-_'")
	if name == "" {
		return fallback
	}
	return name
}

func registerPlaylistName(names map[string]string, id, title string) error {
	filename := PlaylistFilename(title, id)
	key := strings.ToLower(filename)
	if previous, exists := names[key]; exists {
		return fmt.Errorf("playlist filename %q conflicts with %q", filename, previous)
	}
	names[key] = id
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

func (f Filter) Matches(e catalog.Episode) bool {
	if !containsFold(e.Title, f.TitleContains) || !containsFold(e.Description, f.DescriptionContains) {
		return false
	}
	for _, excluded := range f.TitleExcludes {
		if containsFold(e.Title, excluded) {
			return false
		}
	}
	return true
}

// EnsureSources adds configured source feeds that are missing from durable
// state while preserving existing IDs, names, and HTTP cache metadata.
func (c Config) EnsureSources(current catalog.Catalog) (catalog.Catalog, error) {
	if err := c.Validate(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("validate config: %w", err)
	}
	result := current
	byURL := make(map[string]catalog.Feed, len(result.Feeds))
	maxID := int64(0)
	for _, known := range result.Feeds {
		normalized, err := catalog.NormalizeURL(known.URL)
		if err != nil {
			return catalog.Catalog{}, fmt.Errorf("normalize existing feed %d: %w", known.ID, err)
		}
		byURL[normalized] = known
		if known.ID > maxID {
			maxID = known.ID
		}
	}
	for _, source := range c.Sources {
		normalized, err := catalog.NormalizeURL(source.URL)
		if err != nil {
			return catalog.Catalog{}, fmt.Errorf("normalize source %q: %w", source.ID, err)
		}
		if _, exists := byURL[normalized]; exists {
			continue
		}
		maxID++
		known := catalog.Feed{ID: maxID, Name: source.ID, URL: normalized}
		result.Feeds = append(result.Feeds, known)
		byURL[normalized] = known
	}
	if err := result.Validate(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("validate reconciled state: %w", err)
	}
	return result, nil
}

// LogicalFeedIDs returns the first configured logical feed ID for each durable
// source feed. A source may have multiple logical partitions; the first one
// owns the physical media path.
func (c Config) LogicalFeedIDs(current catalog.Catalog) (map[int64]string, error) {
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	sourceURLs := make(map[string]string, len(c.Sources))
	for _, source := range c.Sources {
		normalized, err := catalog.NormalizeURL(source.URL)
		if err != nil {
			return nil, fmt.Errorf("normalize source %q: %w", source.ID, err)
		}
		sourceURLs[source.ID] = normalized
	}
	feedIDs := make(map[string]int64, len(current.Feeds))
	for _, known := range current.Feeds {
		normalized, err := catalog.NormalizeURL(known.URL)
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
