package feed

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
)

// ParseRSS parses an RSS 2.0 podcast and returns only items with audio
// enclosures. The supplied feed provides the durable identity and URL.
func ParseRSS(r io.Reader, f Feed) (Feed, []episode.Episode, error) {
	normalizedURL, err := NormalizeURL(f.URL)
	if err != nil {
		return Feed{}, nil, fmt.Errorf("normalize feed URL: %w", err)
	}

	var document rssDocument
	if err := xml.NewDecoder(r).Decode(&document); err != nil {
		return Feed{}, nil, fmt.Errorf("decode RSS: %w", err)
	}
	if document.Channel.Title == "" {
		return Feed{}, nil, fmt.Errorf("RSS channel has no title")
	}

	f.URL = normalizedURL
	f.Name = strings.TrimSpace(document.Channel.Title)

	episodes := make([]episode.Episode, 0, len(document.Channel.Items))
	for i, item := range document.Channel.Items {
		if !isAudioEnclosure(item.Enclosure) {
			continue
		}

		publishedAt, err := parsePublishedAt(item.PubDate)
		if err != nil {
			return Feed{}, nil, fmt.Errorf("parse item %d publication time: %w", i, err)
		}
		duration, err := parseDuration(item.Duration)
		if err != nil {
			return Feed{}, nil, fmt.Errorf("parse item %d duration: %w", i, err)
		}

		episodes = append(episodes, episode.Episode{
			FeedID:      f.ID,
			GUID:        strings.TrimSpace(item.GUID),
			Title:       strings.TrimSpace(item.Title),
			Description: strings.TrimSpace(item.Description),
			Enclosure: episode.Enclosure{
				URL:    strings.TrimSpace(item.Enclosure.URL),
				Type:   strings.TrimSpace(item.Enclosure.Type),
				Length: item.Enclosure.Length,
			},
			PublishedAt: publishedAt,
			Duration:    duration,
		})
	}

	return f, episodes, nil
}

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	GUID        string       `xml:"guid"`
	Title       string       `xml:"title"`
	Description string       `xml:"description"`
	PubDate     string       `xml:"pubDate"`
	Duration    string       `xml:"duration"`
	Enclosure   rssEnclosure `xml:"enclosure"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length int64  `xml:"length,attr"`
}

func isAudioEnclosure(enclosure rssEnclosure) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(enclosure.Type)), "audio/") &&
		strings.TrimSpace(enclosure.URL) != ""
}

func parsePublishedAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}

	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported date %q", raw)
}

func parseDuration(raw string) (time.Duration, error) {
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
