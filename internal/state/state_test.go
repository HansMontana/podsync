package state

import (
	"testing"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
)

func TestStateValidateAcceptsValidState(t *testing.T) {
	state := State{
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

	if err := state.Validate(); err != nil {
		t.Fatalf("valid state returned error: %v", err)
	}
}

func TestStateValidateRejectsDuplicateFeedIDs(t *testing.T) {
	state := State{
		Feeds: []feed.Feed{
			{ID: 1, URL: "https://example.com/one.xml"},
			{ID: 1, URL: "https://example.com/two.xml"},
		},
	}

	if err := state.Validate(); err == nil {
		t.Fatal("expected duplicate feed IDs to be rejected")
	}
}

func TestStateValidateRejectsDuplicateFeedURLs(t *testing.T) {
	state := State{
		Feeds: []feed.Feed{
			{ID: 1, URL: "https://example.com/feed.xml"},
			{ID: 2, URL: "https://EXAMPLE.com:443/feed.xml"},
		},
	}

	if err := state.Validate(); err == nil {
		t.Fatal("expected duplicate feed URLs to be rejected")
	}
}

func TestStateValidateRejectsEpisodeWithUnknownFeed(t *testing.T) {
	state := State{
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

	if err := state.Validate(); err == nil {
		t.Fatal("expected episode with unknown feed to be rejected")
	}
}

func TestStateValidateRejectsDuplicateEpisodeIdentity(t *testing.T) {
	state := State{
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

	if err := state.Validate(); err == nil {
		t.Fatal("expected duplicate episode identities to be rejected")
	}
}
