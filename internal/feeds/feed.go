package feeds

import (
	"fmt"
	"net/url"
	"strings"
)

// Feed represents a podcast feed known to podsync.
type Feed struct {
	ID           int64
	Name         string
	URL          string
	ETag         string
	LastModified string
}

// NormalizeURL returns the canonical form of a feed URL.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse feed URL: %w", err)
	}

	if parsed.Scheme == "" {
		return "", fmt.Errorf("feed URL has no scheme")
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("feed URL has no host")
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	switch parsed.Scheme {
	case "http":
		if parsed.Port() == "80" {
			parsed.Host = parsed.Hostname()
		}
	case "https":
		if parsed.Port() == "443" {
			parsed.Host = parsed.Hostname()
		}
	}

	if parsed.Path == "" {
		parsed.Path = "/"
	}

	return parsed.String(), nil
}

func (f Feed) SameIdentity(other Feed) bool {
	first, err := NormalizeURL(f.URL)
	if err != nil {
		return false
	}

	second, err := NormalizeURL(other.URL)
	if err != nil {
		return false
	}

	return first == second
}
