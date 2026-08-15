package episodes

import (
	"testing"
	"time"
)

func TestEpisodeIdentityUsesGUID(t *testing.T) {
	first := Episode{
		FeedID:   1,
		GUID:     "episode-123",
		AudioURL: "https://example.com/old.mp3",
		Title:    "Original title",
	}

	second := Episode{
		FeedID:   1,
		GUID:     "episode-123",
		AudioURL: "https://example.com/new.mp3",
		Title:    "Corrected title",
	}

	if !first.SameIdentity(second) {
		t.Fatal("episodes with the same GUID should have the same identity")
	}
}

func TestEpisodeIdentityFallsBackToAudioURL(t *testing.T) {
	first := Episode{
		FeedID:   1,
		AudioURL: "https://example.com/episode.mp3",
	}

	second := Episode{
		FeedID:   1,
		AudioURL: "https://example.com/episode.mp3",
	}

	if !first.SameIdentity(second) {
		t.Fatal("episodes without GUIDs should use the audio URL as fallback identity")
	}
}

func TestEpisodeIdentityDifferentAudioURLsAreDifferent(t *testing.T) {
	first := Episode{
		FeedID:   1,
		AudioURL: "https://example.com/episode-a.mp3",
	}

	second := Episode{
		FeedID:   1,
		AudioURL: "https://example.com/episode-b.mp3",
	}

	if first.SameIdentity(second) {
		t.Fatal("episodes without GUIDs and different audio URLs should not have the same identity")
	}
}

func TestEpisodeIdentityFallsBackToFingerprint(t *testing.T) {
	published := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

	first := Episode{
		FeedID:      1,
		Title:       "A Test Episode",
		PublishedAt: published,
		Duration:    45 * time.Minute,
	}

	second := Episode{
		FeedID:      1,
		Title:       "A Test Episode",
		PublishedAt: published,
		Duration:    45 * time.Minute,
	}

	if !first.SameIdentity(second) {
		t.Fatal("episodes without GUIDs or audio URLs should use the same fingerprint")
	}
}

func TestEpisodeDifferentFingerprintIsDifferent(t *testing.T) {
	published := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

	first := Episode{
		FeedID:      1,
		Title:       "Episode One",
		PublishedAt: published,
		Duration:    45 * time.Minute,
	}

	second := Episode{
		FeedID:      1,
		Title:       "Episode Two",
		PublishedAt: published,
		Duration:    45 * time.Minute,
	}

	if first.SameIdentity(second) {
		t.Fatal("episodes with different fingerprint data should not have the same identity")
	}
}

func TestEpisodeDescriptionDoesNotAffectFingerprint(t *testing.T) {
	published := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

	first := Episode{
		FeedID:      1,
		Title:       "A Test Episode",
		Description: "Original description",
		PublishedAt: published,
		Duration:    45 * time.Minute,
	}

	second := Episode{
		FeedID:      1,
		Title:       "A Test Episode",
		Description: "Corrected description",
		PublishedAt: published,
		Duration:    45 * time.Minute,
	}

	if !first.SameIdentity(second) {
		t.Fatal("changing the description should not change episode identity")
	}
}

func TestEpisodeIdentityIncludesFeed(t *testing.T) {
	first := Episode{
		FeedID: 1,
		GUID:   "episode-123",
	}

	second := Episode{
		FeedID: 2,
		GUID:   "episode-123",
	}

	if first.SameIdentity(second) {
		t.Fatal("the same GUID in different feeds should not have the same identity")
	}
}
