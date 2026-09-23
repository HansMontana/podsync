package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
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
	if parsed.User != nil {
		return "", fmt.Errorf("feed URL must not contain credentials")
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported feed URL scheme %q", parsed.Scheme)
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	if parsed.Port() == "443" {
		parsed.Host = parsed.Hostname()
	}

	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.Fragment = ""

	return parsed.String(), nil
}

// ValidateRemoteURL restricts network fetches to authenticated HTTPS URLs.
func ValidateRemoteURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse remote URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("remote URL must use HTTPS")
	}
	if parsed.Host == "" {
		return fmt.Errorf("remote URL has no host")
	}
	if parsed.User != nil {
		return fmt.Errorf("remote URL must not contain credentials")
	}
	return nil
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

// Episode represents a podcast episode known to podsync.
//
// The GUID is the stable identity supplied by the podcast feed.
// Filenames and local paths deliberately do not form part of the identity.
type Episode struct {
	FeedID      int64
	GUID        string
	Title       string
	Author      string
	Description string
	Enclosure   Enclosure
	PublishedAt time.Time
	Duration    time.Duration
}

// Enclosure identifies the downloadable media attached to an episode.
type Enclosure struct {
	URL    string
	Type   string
	Length int64
}

func (e Episode) SameIdentity(other Episode) bool {
	if e.FeedID != other.FeedID {
		return false
	}

	if e.GUID != "" || other.GUID != "" {
		return e.GUID != "" &&
			other.GUID != "" &&
			e.GUID == other.GUID
	}

	if e.Enclosure.URL != "" || other.Enclosure.URL != "" {
		return e.Enclosure.URL != "" &&
			other.Enclosure.URL != "" &&
			e.Enclosure.URL == other.Enclosure.URL
	}

	return e.fingerprint() == other.fingerprint()
}

// IdentityKey returns a stable key for matching an episode across refreshes.
func (e Episode) IdentityKey() string {
	if e.GUID != "" {
		return fmt.Sprintf("guid:%d:%s", e.FeedID, e.GUID)
	}
	if e.Enclosure.URL != "" {
		return fmt.Sprintf("audio:%d:%s", e.FeedID, e.Enclosure.URL)
	}
	return "fingerprint:" + e.fingerprint()
}

func (e Episode) fingerprint() string {
	data := fmt.Sprintf(
		"%d\x00%s\x00%s\x00%d",
		e.FeedID,
		e.Title,
		e.PublishedAt.UTC().Format(time.RFC3339Nano),
		e.Duration,
	)

	sum := sha256.Sum256([]byte(data))

	return hex.EncodeToString(sum[:])
}

// Catalog represents the persistent podsync state associated with a device.
type Catalog struct {
	Feeds    []Feed
	Episodes []Episode
}

// RefreshResult describes one fetched and parsed source feed without
// persisting it.
type RefreshResult struct {
	Feed     Feed
	Episodes []Episode
	Archive  bool
}

// ApplyRefresh replaces one source feed in memory while retaining archive
// history.
func ApplyRefresh(current Catalog, result RefreshResult) (Catalog, error) {
	feedIndex := -1
	for i, known := range current.Feeds {
		if known.ID == result.Feed.ID {
			feedIndex = i
			break
		}
	}
	if feedIndex == -1 {
		return Catalog{}, fmt.Errorf("refresh feed %d: feed not found", result.Feed.ID)
	}

	previousEpisodes := current.Episodes
	retainedEpisodes := make([]Episode, 0, len(previousEpisodes))
	for _, existing := range previousEpisodes {
		if existing.FeedID != result.Feed.ID {
			retainedEpisodes = append(retainedEpisodes, existing)
		}
	}
	if len(result.Episodes) == 0 {
		for _, existing := range previousEpisodes {
			if existing.FeedID == result.Feed.ID {
				return Catalog{}, fmt.Errorf("refresh feed %d: refusing to replace existing episodes with an empty feed", result.Feed.ID)
			}
		}
	}
	if result.Archive {
		known := make(map[string]struct{}, len(result.Episodes))
		for _, refreshed := range result.Episodes {
			known[refreshed.IdentityKey()] = struct{}{}
		}
		for _, existing := range previousEpisodes {
			if existing.FeedID == result.Feed.ID {
				if _, exists := known[existing.IdentityKey()]; !exists {
					result.Episodes = append(result.Episodes, existing)
				}
			}
		}
	}
	current.Feeds[feedIndex] = result.Feed
	current.Episodes = append(retainedEpisodes, result.Episodes...)
	if err := current.Validate(); err != nil {
		return Catalog{}, fmt.Errorf("validate refreshed state: %w", err)
	}
	return current, nil
}

// Validate checks the invariants of the persistent catalog.
func (c Catalog) Validate() error {
	feedIDs := make(map[int64]struct{})
	feedURLs := make(map[string]struct{})
	episodeIDs := make(map[string]struct{}, len(c.Episodes))

	for _, f := range c.Feeds {
		if f.ID <= 0 {
			return fmt.Errorf("feed ID must be positive: %d", f.ID)
		}
		if _, exists := feedIDs[f.ID]; exists {
			return fmt.Errorf("duplicate feed ID: %d", f.ID)
		}

		feedIDs[f.ID] = struct{}{}

		normalizedURL, err := NormalizeURL(f.URL)
		if err != nil {
			return fmt.Errorf("invalid feed URL %q: %w", f.URL, err)
		}

		if _, exists := feedURLs[normalizedURL]; exists {
			return fmt.Errorf("duplicate feed URL: %q", f.URL)
		}

		feedURLs[normalizedURL] = struct{}{}
	}

	for _, e := range c.Episodes {
		if _, exists := feedIDs[e.FeedID]; !exists {
			return fmt.Errorf(
				"episode references unknown feed ID: %d",
				e.FeedID,
			)
		}

		identity := e.IdentityKey()
		if _, exists := episodeIDs[identity]; exists {
			return fmt.Errorf("duplicate episode identity for feed ID: %d", e.FeedID)
		}
		episodeIDs[identity] = struct{}{}
	}

	return nil
}
