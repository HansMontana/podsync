package catalog

import (
	"testing"
	"time"
)

func TestNormalizeURLTrimsWhitespace(t *testing.T) {
	got, err := NormalizeURL("  https://example.com/feed.xml  ")
	if err != nil {
		t.Fatal(err)
	}

	if got != "https://example.com/feed.xml" {
		t.Fatalf("NormalizeURL() = %q", got)
	}
}

func TestNormalizeURLLowercasesSchemeAndHost(t *testing.T) {
	got, err := NormalizeURL("HTTPS://EXAMPLE.COM/feed.xml")
	if err != nil {
		t.Fatal(err)
	}

	if got != "https://example.com/feed.xml" {
		t.Fatalf("NormalizeURL() = %q", got)
	}
}

func TestNormalizeURLRemovesDefaultPorts(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{raw: "http://example.com:80/feed.xml", want: "http://example.com/feed.xml"},
		{raw: "https://example.com:443/feed.xml", want: "https://example.com/feed.xml"},
	} {
		got, err := NormalizeURL(test.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("NormalizeURL(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestNormalizeURLPreservesQuery(t *testing.T) {
	got, err := NormalizeURL("https://example.com/feed.xml?format=rss")
	if err != nil {
		t.Fatal(err)
	}

	if got != "https://example.com/feed.xml?format=rss" {
		t.Fatalf("NormalizeURL() = %q", got)
	}
}

func TestNormalizeURLRejectsInvalidURLs(t *testing.T) {
	for _, raw := range []string{
		"://not-a-url",
		"example.com/feed.xml",
		"https://user:secret@example.com/feed.xml",
	} {
		if _, err := NormalizeURL(raw); err == nil {
			t.Fatalf("NormalizeURL(%q) accepted an invalid URL", raw)
		}
	}
}

func TestFeedIdentityUsesNormalizedURL(t *testing.T) {
	first := Feed{URL: " HTTPS://EXAMPLE.COM:443/feed.xml "}
	second := Feed{URL: "https://example.com/feed.xml"}

	if !first.SameIdentity(second) {
		t.Fatal("feeds with equivalent normalized URLs should have the same identity")
	}
}

func TestFeedIdentityIgnoresName(t *testing.T) {
	first := Feed{Name: "ATP", URL: "https://example.com/feed.xml"}
	second := Feed{Name: "Accidental Tech Podcast", URL: "https://example.com/feed.xml"}

	if !first.SameIdentity(second) {
		t.Fatal("changing the feed name should not change feed identity")
	}
}

func TestFeedIdentityRejectsDifferentURLs(t *testing.T) {
	first := Feed{URL: "https://example.com/feed-a.xml"}
	second := Feed{URL: "https://example.com/feed-b.xml"}

	if first.SameIdentity(second) {
		t.Fatal("feeds with different URLs should not have the same identity")
	}
}

func TestEpisodeIdentityUsesGUID(t *testing.T) {
	first := Episode{
		FeedID:    1,
		GUID:      "episode-123",
		Enclosure: Enclosure{URL: "https://example.com/old.mp3"},
		Title:     "Original title",
	}
	second := Episode{
		FeedID:    1,
		GUID:      "episode-123",
		Enclosure: Enclosure{URL: "https://example.com/new.mp3"},
		Title:     "Corrected title",
	}

	if !first.SameIdentity(second) {
		t.Fatal("episodes with the same GUID should have the same identity")
	}
}

func TestEpisodeIdentityFallsBackToAudioURL(t *testing.T) {
	first := Episode{FeedID: 1, Enclosure: Enclosure{URL: "https://example.com/episode.mp3"}}
	second := Episode{FeedID: 1, Enclosure: Enclosure{URL: "https://example.com/episode.mp3"}}

	if !first.SameIdentity(second) {
		t.Fatal("episodes without GUIDs should use the audio URL as fallback identity")
	}
}

func TestEpisodeIdentityRejectsDifferentAudioURLs(t *testing.T) {
	first := Episode{FeedID: 1, Enclosure: Enclosure{URL: "https://example.com/episode-a.mp3"}}
	second := Episode{FeedID: 1, Enclosure: Enclosure{URL: "https://example.com/episode-b.mp3"}}

	if first.SameIdentity(second) {
		t.Fatal("episodes with different audio URLs should not have the same identity")
	}
}

func TestEpisodeIdentityFallsBackToFingerprint(t *testing.T) {
	published := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	first := Episode{FeedID: 1, Title: "A Test Episode", PublishedAt: published, Duration: 45 * time.Minute}
	second := Episode{FeedID: 1, Title: "A Test Episode", PublishedAt: published, Duration: 45 * time.Minute}

	if !first.SameIdentity(second) {
		t.Fatal("episodes without GUIDs or audio URLs should use the same fingerprint")
	}
}

func TestEpisodeDifferentFingerprintIsDifferent(t *testing.T) {
	published := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	first := Episode{FeedID: 1, Title: "Episode One", PublishedAt: published, Duration: 45 * time.Minute}
	second := Episode{FeedID: 1, Title: "Episode Two", PublishedAt: published, Duration: 45 * time.Minute}

	if first.SameIdentity(second) {
		t.Fatal("episodes with different fingerprint data should not have the same identity")
	}
}

func TestEpisodeDescriptionDoesNotAffectIdentity(t *testing.T) {
	published := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	first := Episode{FeedID: 1, Title: "A Test Episode", Description: "Original", PublishedAt: published, Duration: 45 * time.Minute}
	second := Episode{FeedID: 1, Title: "A Test Episode", Description: "Corrected", PublishedAt: published, Duration: 45 * time.Minute}

	if !first.SameIdentity(second) {
		t.Fatal("changing the description should not change episode identity")
	}
}

func TestEpisodeIdentityIncludesFeed(t *testing.T) {
	first := Episode{FeedID: 1, GUID: "episode-123"}
	second := Episode{FeedID: 2, GUID: "episode-123"}

	if first.SameIdentity(second) {
		t.Fatal("the same GUID in different feeds should not have the same identity")
	}
}
