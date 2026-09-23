package tomlconfig

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/HansMontana/podsync/internal/domain/curation"
)

type configDTO struct {
	Sources   []sourceFeedDTO  `toml:"source"`
	Feeds     []logicalFeedDTO `toml:"feed"`
	Briefings []briefingDTO    `toml:"briefing"`
}

type sourceFeedDTO struct {
	ID  string `toml:"id"`
	URL string `toml:"url"`
}

type logicalFeedDTO struct {
	ID           string    `toml:"id"`
	Title        string    `toml:"title"`
	Artist       string    `toml:"artist"`
	Source       string    `toml:"source"`
	Archive      bool      `toml:"archive"`
	Order        string    `toml:"order"`
	Limit        int       `toml:"limit"`
	UnplayedOnly bool      `toml:"unplayed_only"`
	Filter       filterDTO `toml:"filter"`
}

type filterDTO struct {
	TitleContains       string   `toml:"title_contains"`
	DescriptionContains string   `toml:"description_contains"`
	TitleExcludes       []string `toml:"title_excludes"`
}

type briefingDTO struct {
	ID       string               `toml:"id"`
	Title    string               `toml:"title"`
	Sections []briefingSectionDTO `toml:"section"`
}

type briefingSectionDTO struct {
	Feed         string `toml:"feed"`
	Order        string `toml:"order"`
	Limit        int    `toml:"limit"`
	UnplayedOnly bool   `toml:"unplayed_only"`
}

func Load(path string) (curation.Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return curation.Config{}, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	return Parse(file)
}

// Save validates and atomically writes configuration to path.
func Save(path string, cfg curation.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(toDTO(cfg)); err != nil {
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

func Parse(r io.Reader) (curation.Config, error) {
	var decoded configDTO
	metadata, err := toml.NewDecoder(r).Decode(&decoded)
	if err != nil {
		return curation.Config{}, fmt.Errorf("decode config: %w", err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return curation.Config{}, fmt.Errorf("unknown configuration key %q", undecoded[0])
	}
	cfg := fromDTO(decoded)
	if err := cfg.Validate(); err != nil {
		return curation.Config{}, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

func toDTO(cfg curation.Config) configDTO {
	result := configDTO{
		Sources:   make([]sourceFeedDTO, len(cfg.Sources)),
		Feeds:     make([]logicalFeedDTO, len(cfg.Feeds)),
		Briefings: make([]briefingDTO, len(cfg.Briefings)),
	}
	for i, source := range cfg.Sources {
		result.Sources[i] = sourceFeedDTO{ID: source.ID, URL: source.URL}
	}
	for i, feed := range cfg.Feeds {
		result.Feeds[i] = logicalFeedDTO{
			ID: feed.ID, Title: feed.Title, Artist: feed.Artist, Source: feed.Source,
			Archive: feed.Archive, Order: string(feed.Order), Limit: feed.Limit,
			UnplayedOnly: feed.UnplayedOnly,
			Filter: filterDTO{
				TitleContains: feed.Filter.TitleContains, DescriptionContains: feed.Filter.DescriptionContains,
				TitleExcludes: feed.Filter.TitleExcludes,
			},
		}
	}
	for i, briefing := range cfg.Briefings {
		result.Briefings[i] = briefingDTO{ID: briefing.ID, Title: briefing.Title, Sections: make([]briefingSectionDTO, len(briefing.Sections))}
		for j, section := range briefing.Sections {
			result.Briefings[i].Sections[j] = briefingSectionDTO{
				Feed: section.Feed, Order: string(section.Order), Limit: section.Limit, UnplayedOnly: section.UnplayedOnly,
			}
		}
	}
	return result
}

func fromDTO(dto configDTO) curation.Config {
	result := curation.Config{
		Sources:   make([]curation.SourceFeed, len(dto.Sources)),
		Feeds:     make([]curation.LogicalFeed, len(dto.Feeds)),
		Briefings: make([]curation.Briefing, len(dto.Briefings)),
	}
	for i, source := range dto.Sources {
		result.Sources[i] = curation.SourceFeed{ID: source.ID, URL: source.URL}
	}
	for i, feed := range dto.Feeds {
		result.Feeds[i] = curation.LogicalFeed{
			ID: feed.ID, Title: feed.Title, Artist: feed.Artist, Source: feed.Source,
			Archive: feed.Archive, Order: curation.Order(feed.Order), Limit: feed.Limit,
			UnplayedOnly: feed.UnplayedOnly,
			Filter: curation.Filter{
				TitleContains: feed.Filter.TitleContains, DescriptionContains: feed.Filter.DescriptionContains,
				TitleExcludes: feed.Filter.TitleExcludes,
			},
		}
	}
	for i, briefing := range dto.Briefings {
		result.Briefings[i] = curation.Briefing{ID: briefing.ID, Title: briefing.Title, Sections: make([]curation.BriefingSection, len(briefing.Sections))}
		for j, section := range briefing.Sections {
			result.Briefings[i].Sections[j] = curation.BriefingSection{
				Feed: section.Feed, Order: curation.Order(section.Order), Limit: section.Limit, UnplayedOnly: section.UnplayedOnly,
			}
		}
	}
	return result
}
