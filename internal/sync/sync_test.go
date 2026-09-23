package sync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/state"
)

func TestRefreshFeedReplacesOnlyTargetFeedAndPersistsMetadata(t *testing.T) {
	rss := `<rss><channel><title>Updated feed</title><item><guid>new-episode</guid><title>New episode</title><enclosure url="https://example.com/new.mp3" type="audio/mpeg" length="42"/></item></channel></rss>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"updated"`)
		w.Header().Set("Last-Modified", "Wed, 02 Sep 2026 14:39:34 +0200")
		_, _ = w.Write([]byte(rss))
	}))
	defer server.Close()

	repository, err := state.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

	wantOther := episode.Episode{
		FeedID:    2,
		GUID:      "other-episode",
		Enclosure: episode.Enclosure{URL: "https://example.com/other.mp3", Type: "audio/mpeg"},
	}
	initial := catalog.Catalog{
		Feeds: []feed.Feed{
			{ID: 1, Name: "Old feed", URL: server.URL},
			{ID: 2, Name: "Other feed", URL: "https://example.org/feed.xml"},
		},
		Episodes: []episode.Episode{
			{FeedID: 1, GUID: "old-episode", Enclosure: episode.Enclosure{URL: "https://example.com/old.mp3"}},
			wantOther,
		},
	}
	if err := repository.Save(initial); err != nil {
		t.Fatalf("initial Save() returned error: %v", err)
	}

	if err := RefreshFeed(context.Background(), repository, server.Client(), 1); err != nil {
		t.Fatalf("RefreshFeed() returned error: %v", err)
	}

	got, err := repository.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if got.Feeds[0].Name != "Updated feed" || got.Feeds[0].ETag != `"updated"` {
		t.Fatalf("got refreshed feed %+v", got.Feeds[0])
	}
	if len(got.Episodes) != 2 || got.Episodes[0].GUID != "new-episode" || got.Episodes[1].GUID != "other-episode" {
		t.Fatalf("got episodes %+v", got.Episodes)
	}
	if got.Episodes[0].Enclosure.Length != 42 {
		t.Fatalf("got refreshed enclosure %+v", got.Episodes[0].Enclosure)
	}
}

func TestRefreshFeedWithArchiveRetainsEpisodesOutsideRSSWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<rss><channel><title>Archive</title><item><guid>new</guid><title>New</title><enclosure url="https://example.com/new.mp3" type="audio/mpeg"/></item></channel></rss>`))
	}))
	defer server.Close()
	repository, err := state.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	initial := catalog.Catalog{
		Feeds: []feed.Feed{
			{ID: 1, URL: server.URL},
			{ID: 2, URL: "https://example.org/other.xml"},
		},
		Episodes: []episode.Episode{
			{FeedID: 1, GUID: "old", Enclosure: episode.Enclosure{URL: "https://example.com/old.mp3"}},
			{FeedID: 2, GUID: "other", Enclosure: episode.Enclosure{URL: "https://example.org/other.mp3"}},
		},
	}
	if err := repository.Save(initial); err != nil {
		t.Fatal(err)
	}
	if err := RefreshFeedWithArchive(context.Background(), repository, server.Client(), 1, true); err != nil {
		t.Fatal(err)
	}
	got, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 3 {
		t.Fatalf("got episodes %+v", got.Episodes)
	}
	identities := make(map[string]struct{}, len(got.Episodes))
	for _, current := range got.Episodes {
		identities[current.IdentityKey()] = struct{}{}
	}
	for _, guid := range []string{"new", "old", "other"} {
		if _, exists := identities[(episode.Episode{FeedID: map[string]int64{"new": 1, "old": 1, "other": 2}[guid], GUID: guid}).IdentityKey()]; !exists {
			t.Fatalf("missing retained episode %q: %+v", guid, got.Episodes)
		}
	}
}

func TestRefreshFeed304LeavesStateUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `"current"` {
			t.Errorf("missing conditional ETag header")
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	repository, err := state.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

	want := catalog.Catalog{
		Feeds:    []feed.Feed{{ID: 1, Name: "Current", URL: server.URL, ETag: `"current"`}},
		Episodes: []episode.Episode{{FeedID: 1, GUID: "existing"}},
	}
	if err := repository.Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	if err := RefreshFeed(context.Background(), repository, server.Client(), 1); err != nil {
		t.Fatalf("RefreshFeed() returned error: %v", err)
	}
	got, err := repository.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if len(got.Episodes) != 1 || got.Episodes[0].GUID != "existing" {
		t.Fatalf("got changed state %+v", got)
	}
}

func TestRefreshFeedMalformedRSSLeavesStateUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<rss><channel><title>broken`))
	}))
	defer server.Close()

	repository, err := state.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

	want := catalog.Catalog{
		Feeds:    []feed.Feed{{ID: 1, Name: "Current", URL: server.URL}},
		Episodes: []episode.Episode{{FeedID: 1, GUID: "existing"}},
	}
	if err := repository.Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	if err := RefreshFeed(context.Background(), repository, server.Client(), 1); err == nil {
		t.Fatal("RefreshFeed() accepted malformed RSS")
	}
	got, err := repository.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if len(got.Episodes) != 1 || got.Episodes[0].GUID != "existing" {
		t.Fatalf("got changed state %+v", got)
	}
}

func TestRefreshFeedsPersistsMultipleFeedsOnce(t *testing.T) {
	servers := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guid := "one"
		if r.URL.Path == "/two" {
			guid = "two"
		}
		_, _ = fmt.Fprintf(w, `<rss><channel><title>%s</title><item><guid>%s</guid><title>Episode</title><enclosure url="https://example.com/%s.mp3" type="audio/mpeg"/></item></channel></rss>`, guid, guid, guid)
	}))
	defer servers.Close()

	repository, err := state.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Save(catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: servers.URL + "/one"}, {ID: 2, URL: servers.URL + "/two"}}}); err != nil {
		t.Fatal(err)
	}
	if err := RefreshFeeds(context.Background(), repository, servers.Client(), []RefreshRequest{{FeedID: 1}, {FeedID: 2}}); err != nil {
		t.Fatal(err)
	}
	got, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 2 {
		t.Fatalf("got episodes %+v", got.Episodes)
	}
}

func TestRefreshFeedsCancellationDoesNotPersistPartialResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, `<rss><channel><title>Updated</title><item><guid>new</guid><title>Episode</title><enclosure url="https://example.com/new.mp3" type="audio/mpeg"/></item></channel></rss>`)
	}))
	defer server.Close()
	repository, err := state.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	want := catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: server.URL + "/ok"}, {ID: 2, URL: server.URL + "/fail"}}, Episodes: []episode.Episode{{FeedID: 1, GUID: "old"}}}
	if err := repository.Save(want); err != nil {
		t.Fatal(err)
	}
	if err := RefreshFeeds(context.Background(), repository, server.Client(), []RefreshRequest{{FeedID: 1}, {FeedID: 2}}); err == nil {
		t.Fatal("RefreshFeeds accepted a failed worker")
	}
	got, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 1 || got.Episodes[0].GUID != "old" {
		t.Fatalf("failed batch changed state: %+v", got)
	}
}

func TestRefreshFeedsReturnsParentCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("canceled refresh made an HTTP request")
	}))
	defer server.Close()
	repository, err := state.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Save(catalog.Catalog{Feeds: []feed.Feed{{ID: 1, URL: server.URL}}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RefreshFeeds(ctx, repository, server.Client(), []RefreshRequest{{FeedID: 1}}); err == nil {
		t.Fatal("canceled RefreshFeeds returned nil")
	}
}
