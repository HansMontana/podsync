package rockbox

import (
	"strings"
	"testing"
)

func TestParseLogRetainsPartialPlaybackAsSkipped(t *testing.T) {
	records, err := ParseLog(strings.NewReader("100:20:100:/Podcasts/news/episode.mp3\n200:100:100:/Podcasts/news/episode.mp3\n"))
	if err != nil {
		t.Fatalf("ParseLog() returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	record := records[0]
	if !record.Known || record.PlayCount != 1 || !record.Skipped {
		t.Fatalf("got record %+v", record)
	}
}

func TestParseLogIgnoresZeroElapsedPlayback(t *testing.T) {
	records, err := ParseLog(strings.NewReader("100:0:100:/Podcasts/news/episode.mp3\n"))
	if err != nil {
		t.Fatalf("ParseLog() returned error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("got records %+v", records)
	}
}
