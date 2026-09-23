package catalog

import (
	"fmt"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
)

// Catalog represents the persistent podsync state associated with a device.
type Catalog struct {
	Feeds    []feed.Feed
	Episodes []episode.Episode
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

		normalizedURL, err := feed.NormalizeURL(f.URL)
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
