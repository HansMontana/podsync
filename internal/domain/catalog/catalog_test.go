package catalog

import (
	"fmt"
	"testing"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
)

func TestCatalogValidateAcceptsValidCatalog(t *testing.T) {
	catalog := Catalog{
		Feeds: []feed.Feed{
			{
				ID:  1,
				URL: "https://example.com/feed.xml",
			},
		},
		Episodes: []episode.Episode{
			{
				FeedID: 1,
				GUID:   "episode-1",
			},
		},
	}

	if err := catalog.Validate(); err != nil {
		t.Fatalf("valid catalog returned error: %v", err)
	}
}

func TestCatalogValidateRejectsDuplicateFeedIDs(t *testing.T) {
	catalog := Catalog{
		Feeds: []feed.Feed{
			{ID: 1, URL: "https://example.com/one.xml"},
			{ID: 1, URL: "https://example.com/two.xml"},
		},
	}

	if err := catalog.Validate(); err == nil {
		t.Fatal("expected duplicate feed IDs to be rejected")
	}
}

func TestCatalogValidateRejectsDuplicateFeedURLs(t *testing.T) {
	catalog := Catalog{
		Feeds: []feed.Feed{
			{ID: 1, URL: "https://example.com/feed.xml"},
			{ID: 2, URL: "https://EXAMPLE.com:443/feed.xml"},
		},
	}

	if err := catalog.Validate(); err == nil {
		t.Fatal("expected duplicate feed URLs to be rejected")
	}
}

func TestCatalogValidateRejectsEpisodeWithUnknownFeed(t *testing.T) {
	catalog := Catalog{
		Feeds: []feed.Feed{
			{ID: 1, URL: "https://example.com/feed.xml"},
		},
		Episodes: []episode.Episode{
			{
				FeedID: 999,
				GUID:   "episode-1",
			},
		},
	}

	if err := catalog.Validate(); err == nil {
		t.Fatal("expected episode with unknown feed to be rejected")
	}
}

func TestCatalogValidateRejectsDuplicateEpisodeIdentity(t *testing.T) {
	catalog := Catalog{
		Feeds: []feed.Feed{
			{ID: 1, URL: "https://example.com/feed.xml"},
		},
		Episodes: []episode.Episode{
			{
				FeedID: 1,
				GUID:   "episode-1",
				Title:  "First version",
			},
			{
				FeedID: 1,
				GUID:   "episode-1",
				Title:  "Updated version",
			},
		},
	}

	if err := catalog.Validate(); err == nil {
		t.Fatal("expected duplicate episode identities to be rejected")
	}
}

func BenchmarkCatalogValidateEpisodes(b *testing.B) {
	catalog := Catalog{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/feed.xml"}}}
	for i := 0; i < 20000; i++ {
		catalog.Episodes = append(catalog.Episodes, episode.Episode{FeedID: 1, GUID: fmt.Sprintf("episode-%d", i)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := catalog.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}
