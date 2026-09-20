package playback

import (
	"strings"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/media"
)

func TestForEpisodesMatchesNormalizedDevicePaths(t *testing.T) {
	current := episode.Episode{FeedID: 2, GUID: "one", Enclosure: episode.Enclosure{URL: "https://example.com/one.mp3", Type: "audio/mpeg"}}
	playedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	resolver := media.Resolver{2: "podcast"}
	states := ForEpisodesWithResolver([]episode.Episode{current}, []Record{{Path: "/" + resolver.RelativePathFor(current), Known: true, PlayCount: 3, LastPlayed: playedAt}}, resolver)
	state := states[current.IdentityKey()]
	if !state.Known || !state.Played() || state.PlayCount != 3 || !state.LastPlayed.Equal(playedAt) {
		t.Fatalf("got playback state %+v", state)
	}
}

func TestForEpisodesMatchesRockboxVolumePrefixedPaths(t *testing.T) {
	current := episode.Episode{FeedID: 2, GUID: "one", Enclosure: episode.Enclosure{URL: "https://example.com/one.mp3", Type: "audio/mpeg"}}
	resolver := media.Resolver{2: "podcast"}
	states := ForEpisodesWithResolver([]episode.Episode{current}, []Record{{Path: "/<HDD0>/" + resolver.RelativePathFor(current), Known: true, PlayCount: 1}}, resolver)
	state := states[current.IdentityKey()]
	if !state.Known || !state.Played() || state.PlayCount != 1 {
		t.Fatalf("got playback state %+v", state)
	}
}

func TestForEpisodesTreatsMissingRecordsAsUnplayed(t *testing.T) {
	current := episode.Episode{FeedID: 2, GUID: "one", Enclosure: episode.Enclosure{URL: "https://example.com/one.mp3", Type: "audio/mpeg"}}
	state := ForEpisodesWithResolver([]episode.Episode{current}, nil, media.Resolver{2: "podcast"})[current.IdentityKey()]
	if state.Known || state.Played() {
		t.Fatalf("got playback state %+v", state)
	}
}

func TestForEpisodesDoesNotTrustUnknownRecords(t *testing.T) {
	current := episode.Episode{FeedID: 2, GUID: "one", Enclosure: episode.Enclosure{URL: "https://example.com/one.mp3", Type: "audio/mpeg"}}
	resolver := media.Resolver{2: "podcast"}
	state := ForEpisodesWithResolver([]episode.Episode{current}, []Record{{Path: "/" + resolver.RelativePathFor(current), PlayCount: 4}}, resolver)[current.IdentityKey()]
	if state.Known || state.Played() {
		t.Fatalf("got playback state %+v", state)
	}
}

func TestMergeRecordsIgnoresUnknownCounts(t *testing.T) {
	records := MergeRecords([]Record{
		{Path: "/Podcasts/episode.mp3", PlayCount: 7},
		{Path: "/Podcasts/episode.mp3", Known: true, PlayCount: 2},
	})
	if len(records) != 1 || records[0].PlayCount != 2 {
		t.Fatalf("got records %+v", records)
	}
}

func TestPreferRecordsUsesFallbackForMissingPaths(t *testing.T) {
	primary := []Record{{Path: "/Podcasts/one.mp3", Known: true, PlayCount: 2}}
	fallback := []Record{
		{Path: "/Podcasts/one.mp3", Known: true, PlayCount: 9},
		{Path: "/Podcasts/two.mp3", Known: true, PlayCount: 1},
	}
	got := PreferRecords(primary, fallback)
	if len(got) != 2 {
		t.Fatalf("got records %+v", got)
	}
	for _, record := range got {
		if normalizePath(record.Path) == "Podcasts/one.mp3" && record.PlayCount != 2 {
			t.Fatalf("fallback replaced primary record: %+v", got)
		}
	}
	if normalizePath(got[1].Path) != "Podcasts/two.mp3" {
		t.Fatalf("got records %+v", got)
	}
}

func TestParseLogAggregatesRockboxPlaybackEntries(t *testing.T) {
	log := `# Started Ver. 4.x
	1700000000:15000:20000:/Podcasts/podcast/episode.mp3
	1700000100:15000:20000:/Podcasts/podcast/episode.mp3
malformed
`
	records, err := ParseLog(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseLog() returned error: %v", err)
	}
	if len(records) != 1 || records[0].PlayCount != 2 {
		t.Fatalf("got records %+v", records)
	}
	if records[0].LastPlayed.Unix() != 1700000100 {
		t.Fatalf("got last played %v", records[0].LastPlayed)
	}
}

func TestParseLogAcceptsRockboxVolumePrefixedPaths(t *testing.T) {
	log := "1700000000:15000:20000:/<HDD0>/Podcasts/podcast/episode.mp3\n"
	records, err := ParseLog(strings.NewReader(log))
	if err != nil {
		t.Fatalf("ParseLog() returned error: %v", err)
	}
	if len(records) != 1 || records[0].PlayCount != 1 {
		t.Fatalf("got records %+v", records)
	}
}

func TestParseLogRejectsBriefOrMalformedPlayback(t *testing.T) {
	log := "1700000000:14999:20000:/Podcasts/podcast/episode.mp3\n1700000000:20001:20000:/Podcasts/podcast/episode.mp3\n"
	records, err := ParseLog(strings.NewReader(log))
	if err != nil || len(records) != 0 {
		t.Fatalf("got records %+v, error %v", records, err)
	}
}

func TestPreferRecordsKeepsPlayedLogEvidence(t *testing.T) {
	got := PreferRecords([]Record{{Path: "/Podcasts/one.mp3", Known: true}}, []Record{{Path: "/Podcasts/one.mp3", Known: true, PlayCount: 1}})
	if len(got) != 1 || got[0].PlayCount != 1 {
		t.Fatalf("got records %+v", got)
	}
}
