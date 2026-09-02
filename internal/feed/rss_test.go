package feed

import (
	_ "embed"
	"strings"
	"testing"
	"time"
)

//go:embed testdata/tagesschau.xml
var tagesschauRSS string

func TestParseRSSParsesCapturedAudioFeed(t *testing.T) {
	gotFeed, gotEpisodes, err := ParseRSS(
		strings.NewReader(tagesschauRSS),
		Feed{ID: 7, URL: " HTTPS://EXAMPLE.COM/feed.xml "},
	)
	if err != nil {
		t.Fatalf("ParseRSS() returned error: %v", err)
	}

	wantFeed := Feed{
		ID:   7,
		Name: "tagesschau in 100 Sekunden: Das aktuelle News-Update (Audio)",
		URL:  "https://example.com/feed.xml",
	}
	if gotFeed != wantFeed {
		t.Fatalf("got feed %+v, want %+v", gotFeed, wantFeed)
	}

	if len(gotEpisodes) != 1 {
		t.Fatalf("got %d episodes, want 1", len(gotEpisodes))
	}

	got := gotEpisodes[0]
	if got.FeedID != 7 || got.GUID != "c365228a-a39e-426c-ad95-8a5f0af8eb05" || got.Title != "tagesschau in 100 Sekunden" {
		t.Fatalf("got episode identity and title %+v", got)
	}
	if got.Description != "tagesschau in 100 Sekunden" || got.Enclosure.URL != "https://tagesschau-podcast.ard-mcdn.de/audio/2026/0902/AU-20260902-1439-2400.mp3" {
		t.Fatalf("got episode metadata %+v", got)
	}
	if got.Enclosure.Type != "audio/mpeg" || got.Enclosure.Length != 2252299 {
		t.Fatalf("got enclosure %+v", got.Enclosure)
	}
	if !got.PublishedAt.Equal(time.Date(2026, 9, 2, 12, 39, 0, 0, time.UTC)) {
		t.Fatalf("got publication time %v", got.PublishedAt)
	}
	if got.Duration != 140*time.Second {
		t.Fatalf("got duration %v, want 2m20s", got.Duration)
	}
}

func TestParseRSSSkipsNonAudioItems(t *testing.T) {
	input := `<rss><channel><title>Example</title><item><enclosure url="https://example.com/video.mp4" type="video/mp4"/></item></channel></rss>`

	_, episodes, err := ParseRSS(strings.NewReader(input), Feed{ID: 1, URL: "https://example.com/feed.xml"})
	if err != nil {
		t.Fatalf("ParseRSS() returned error: %v", err)
	}
	if len(episodes) != 0 {
		t.Fatalf("got %d episodes, want 0", len(episodes))
	}
}

func TestParseRSSSupportsDurationFormats(t *testing.T) {
	for _, test := range []struct {
		input string
		want  time.Duration
	}{
		{input: "42", want: 42 * time.Second},
		{input: "1:42", want: 102 * time.Second},
		{input: "1:02:03", want: 1*time.Hour + 2*time.Minute + 3*time.Second},
	} {
		got, err := parseDuration(test.input)
		if err != nil {
			t.Fatalf("parseDuration(%q) returned error: %v", test.input, err)
		}
		if got != test.want {
			t.Errorf("parseDuration(%q) = %v, want %v", test.input, got, test.want)
		}
	}
}

func TestParseRSSRejectsInvalidAudioItemDuration(t *testing.T) {
	input := `<rss><channel><title>Example</title><item><itunes:duration>bad</itunes:duration><enclosure url="https://example.com/audio.mp3" type="audio/mpeg"/></item></channel></rss>`

	_, _, err := ParseRSS(strings.NewReader(input), Feed{ID: 1, URL: "https://example.com/feed.xml"})
	if err == nil {
		t.Fatal("ParseRSS() accepted an invalid audio duration")
	}
}

func TestParseRSSRejectsFeedWithoutTitle(t *testing.T) {
	_, _, err := ParseRSS(
		strings.NewReader(`<rss><channel><item/></channel></rss>`),
		Feed{ID: 1, URL: "https://example.com/feed.xml"},
	)
	if err == nil {
		t.Fatal("ParseRSS() accepted a feed without a title")
	}
}
