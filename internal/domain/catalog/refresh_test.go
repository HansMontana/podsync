package catalog

import "testing"

func TestApplyRefreshReplacesTargetFeedAndPreservesOtherFeeds(t *testing.T) {
	current := Catalog{
		Feeds: []Feed{
			{ID: 1, Name: "Old", URL: "https://example.com/old.xml"},
			{ID: 2, Name: "Other", URL: "https://example.com/other.xml"},
		},
		Episodes: []Episode{
			{FeedID: 1, GUID: "old"},
			{FeedID: 2, GUID: "other"},
		},
	}

	updated, err := ApplyRefresh(current, RefreshResult{
		Feed:     Feed{ID: 1, Name: "Updated", URL: "https://example.com/old.xml"},
		Episodes: []Episode{{FeedID: 1, GUID: "new"}},
	})
	if err != nil {
		t.Fatalf("ApplyRefresh() returned error: %v", err)
	}
	if updated.Feeds[0].Name != "Updated" {
		t.Fatalf("got feeds %+v", updated.Feeds)
	}
	if len(updated.Episodes) != 2 || updated.Episodes[0].GUID != "other" || updated.Episodes[1].GUID != "new" {
		t.Fatalf("got episodes %+v", updated.Episodes)
	}
}

func TestApplyRefreshRetainsArchiveHistoryByIdentity(t *testing.T) {
	current := Catalog{
		Feeds: []Feed{{ID: 1, URL: "https://example.com/feed.xml"}},
		Episodes: []Episode{
			{FeedID: 1, GUID: "old"},
			{FeedID: 1, GUID: "current"},
		},
	}

	updated, err := ApplyRefresh(current, RefreshResult{
		Feed:     current.Feeds[0],
		Episodes: []Episode{{FeedID: 1, GUID: "current"}},
		Archive:  true,
	})
	if err != nil {
		t.Fatalf("ApplyRefresh() returned error: %v", err)
	}
	if len(updated.Episodes) != 2 || updated.Episodes[0].GUID != "current" || updated.Episodes[1].GUID != "old" {
		t.Fatalf("got episodes %+v", updated.Episodes)
	}
}

func TestApplyRefreshRejectsEmptyRefreshForFeedWithEpisodes(t *testing.T) {
	current := Catalog{
		Feeds:    []Feed{{ID: 1, URL: "https://example.com/feed.xml"}},
		Episodes: []Episode{{FeedID: 1, GUID: "existing"}},
	}

	if _, err := ApplyRefresh(current, RefreshResult{Feed: current.Feeds[0]}); err == nil {
		t.Fatal("ApplyRefresh() accepted an empty refresh")
	}
}

func TestApplyRefreshValidatesResultingCatalog(t *testing.T) {
	current := Catalog{
		Feeds:    []Feed{{ID: 1, URL: "https://example.com/feed.xml"}},
		Episodes: []Episode{{FeedID: 1, GUID: "existing"}},
	}

	if _, err := ApplyRefresh(current, RefreshResult{
		Feed:     Feed{ID: 1, URL: "not-a-feed-url"},
		Episodes: []Episode{{FeedID: 1, GUID: "new"}},
	}); err == nil {
		t.Fatal("ApplyRefresh() accepted an invalid resulting catalog")
	}
}
