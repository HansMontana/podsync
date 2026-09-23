package rss

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/mmcdole/gofeed"
)

// ParseRSS parses an RSS 2.0 podcast and returns only items with audio
// enclosures. The supplied feed provides the durable identity and URL.
func ParseRSS(r io.Reader, f catalog.Feed) (catalog.Feed, []catalog.Episode, error) {
	normalizedURL, err := catalog.NormalizeURL(f.URL)
	if err != nil {
		return catalog.Feed{}, nil, fmt.Errorf("normalize feed URL: %w", err)
	}

	parser := gofeed.NewParser()
	parsed, err := parser.Parse(r)
	if err != nil {
		return catalog.Feed{}, nil, fmt.Errorf("decode RSS: %w", err)
	}
	if parsed.FeedType != "rss" {
		return catalog.Feed{}, nil, fmt.Errorf("unsupported feed type %q: only RSS is supported", parsed.FeedType)
	}
	if strings.TrimSpace(parsed.Title) == "" {
		return catalog.Feed{}, nil, fmt.Errorf("RSS channel has no title")
	}

	f.URL = normalizedURL
	f.Name = strings.TrimSpace(parsed.Title)
	feedAuthor := authorName(parsed.Author, parsed.Authors)
	if parsed.ITunesExt != nil && strings.TrimSpace(parsed.ITunesExt.Author) != "" {
		feedAuthor = strings.TrimSpace(parsed.ITunesExt.Author)
	}

	episodes := make([]catalog.Episode, 0, len(parsed.Items))
	for i, item := range parsed.Items {
		enclosure, ok := audioEnclosure(item)
		if !ok {
			continue
		}
		if err := validateEnclosureURL(enclosure.URL); err != nil {
			return catalog.Feed{}, nil, fmt.Errorf("parse item %d enclosure: %w", i, err)
		}

		duration, err := parseDuration(item)
		if err != nil {
			return catalog.Feed{}, nil, fmt.Errorf("parse item %d duration: %w", i, err)
		}

		var publishedAt time.Time
		if item.PublishedParsed != nil {
			publishedAt = item.PublishedParsed.UTC()
		}

		itemAuthor := feedAuthor
		if item.ITunesExt != nil && strings.TrimSpace(item.ITunesExt.Author) != "" {
			itemAuthor = strings.TrimSpace(item.ITunesExt.Author)
		} else if author := authorName(item.Author, item.Authors); author != "" {
			itemAuthor = author
		}
		episodes = append(episodes, catalog.Episode{
			FeedID:      f.ID,
			GUID:        strings.TrimSpace(item.GUID),
			Title:       strings.TrimSpace(item.Title),
			Author:      itemAuthor,
			Description: strings.TrimSpace(item.Description),
			Enclosure: catalog.Enclosure{
				URL:    strings.TrimSpace(enclosure.URL),
				Type:   strings.TrimSpace(enclosure.Type),
				Length: enclosureLength(enclosure.Length),
			},
			PublishedAt: publishedAt,
			Duration:    duration,
		})
	}

	return f, episodes, nil
}

func authorName(primary *gofeed.Person, authors []*gofeed.Person) string {
	if primary != nil && strings.TrimSpace(primary.Name) != "" {
		return strings.TrimSpace(primary.Name)
	}
	for _, author := range authors {
		if author != nil && strings.TrimSpace(author.Name) != "" {
			return strings.TrimSpace(author.Name)
		}
	}
	return ""
}

func audioEnclosure(item *gofeed.Item) (*gofeed.Enclosure, bool) {
	for _, enclosure := range item.Enclosures {
		if enclosure == nil || strings.TrimSpace(enclosure.URL) == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(enclosure.Type)), "audio/") {
			return enclosure, true
		}
	}
	return nil, false
}

func validateEnclosureURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse audio URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("audio URL must use HTTP or HTTPS")
	}
	if parsed.Host == "" {
		return fmt.Errorf("audio URL has no host")
	}
	return nil
}

func enclosureLength(raw string) int64 {
	length, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || length < 0 {
		return 0
	}
	return length
}

func parseDuration(item *gofeed.Item) (time.Duration, error) {
	if item.ITunesExt == nil {
		return 0, nil
	}
	return parseDurationString(item.ITunesExt.Duration)
}

func parseDurationString(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	parts := strings.Split(raw, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("unsupported duration %q", raw)
	}

	values := make([]int64, len(parts))
	for i, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil || value < 0 {
			return 0, fmt.Errorf("invalid duration %q", raw)
		}
		values[i] = value
	}

	var seconds int64
	switch len(values) {
	case 1:
		seconds = values[0]
	case 2:
		seconds = values[0]*60 + values[1]
	case 3:
		seconds = values[0]*3600 + values[1]*60 + values[2]
	}

	return time.Duration(seconds) * time.Second, nil
}
