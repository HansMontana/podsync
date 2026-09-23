package catalog_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HansMontana/podsync/internal/adapters/rss"
	"github.com/HansMontana/podsync/internal/adapters/sqlitecatalog"
	applicationcatalog "github.com/HansMontana/podsync/internal/application/catalog"
	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func TestRefreshFeedReplacesOnlyTargetFeedAndPersistsMetadata(t *testing.T) {
	rssBody := `<rss><channel><title>Updated feed</title><item><guid>new-episode</guid><title>New episode</title><enclosure url="https://example.com/new.mp3" type="audio/mpeg" length="42"/></item></channel></rss>`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"updated"`)
		w.Header().Set("Last-Modified", "Wed, 02 Sep 2026 14:39:34 +0200")
		_, _ = w.Write([]byte(rssBody))
	}))
	defer server.Close()

	repository, err := sqlitecatalog.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

	wantOther := catalog.Episode{
		FeedID:    2,
		GUID:      "other-episode",
		Enclosure: catalog.Enclosure{URL: "https://example.com/other.mp3", Type: "audio/mpeg"},
	}
	initial := catalog.Catalog{
		Feeds: []catalog.Feed{
			{ID: 1, Name: "Old feed", URL: server.URL},
			{ID: 2, Name: "Other feed", URL: "https://example.org/feed.xml"},
		},
		Episodes: []catalog.Episode{
			{FeedID: 1, GUID: "old-episode", Enclosure: catalog.Enclosure{URL: "https://example.com/old.mp3"}},
			wantOther,
		},
	}
	if err := repository.Save(initial); err != nil {
		t.Fatalf("initial Save() returned error: %v", err)
	}

	if err := applicationcatalog.RefreshFeed(context.Background(), repository, rss.NewReader(server.Client()), 1); err != nil {
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<rss><channel><title>Archive</title><item><guid>new</guid><title>New</title><enclosure url="https://example.com/new.mp3" type="audio/mpeg"/></item></channel></rss>`))
	}))
	defer server.Close()
	repository, err := sqlitecatalog.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	initial := catalog.Catalog{
		Feeds: []catalog.Feed{
			{ID: 1, URL: server.URL},
			{ID: 2, URL: "https://example.org/other.xml"},
		},
		Episodes: []catalog.Episode{
			{FeedID: 1, GUID: "old", Enclosure: catalog.Enclosure{URL: "https://example.com/old.mp3"}},
			{FeedID: 2, GUID: "other", Enclosure: catalog.Enclosure{URL: "https://example.org/other.mp3"}},
		},
	}
	if err := repository.Save(initial); err != nil {
		t.Fatal(err)
	}
	if err := applicationcatalog.RefreshFeedWithArchive(context.Background(), repository, rss.NewReader(server.Client()), 1, true); err != nil {
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
		if _, exists := identities[(catalog.Episode{FeedID: map[string]int64{"new": 1, "old": 1, "other": 2}[guid], GUID: guid}).IdentityKey()]; !exists {
			t.Fatalf("missing retained episode %q: %+v", guid, got.Episodes)
		}
	}
}

func TestRefreshFeed304LeavesStateUnchanged(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `"current"` {
			t.Errorf("missing conditional ETag header")
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	repository, err := sqlitecatalog.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

	want := catalog.Catalog{
		Feeds:    []catalog.Feed{{ID: 1, Name: "Current", URL: server.URL, ETag: `"current"`}},
		Episodes: []catalog.Episode{{FeedID: 1, GUID: "existing"}},
	}
	if err := repository.Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	if err := applicationcatalog.RefreshFeed(context.Background(), repository, rss.NewReader(server.Client()), 1); err != nil {
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<rss><channel><title>broken`))
	}))
	defer server.Close()

	repository, err := sqlitecatalog.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

	want := catalog.Catalog{
		Feeds:    []catalog.Feed{{ID: 1, Name: "Current", URL: server.URL}},
		Episodes: []catalog.Episode{{FeedID: 1, GUID: "existing"}},
	}
	if err := repository.Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	if err := applicationcatalog.RefreshFeed(context.Background(), repository, rss.NewReader(server.Client()), 1); err == nil {
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
	servers := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guid := "one"
		if r.URL.Path == "/two" {
			guid = "two"
		}
		_, _ = fmt.Fprintf(w, `<rss><channel><title>%s</title><item><guid>%s</guid><title>Episode</title><enclosure url="https://example.com/%s.mp3" type="audio/mpeg"/></item></channel></rss>`, guid, guid, guid)
	}))
	defer servers.Close()

	repository, err := sqlitecatalog.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Save(catalog.Catalog{Feeds: []catalog.Feed{{ID: 1, URL: servers.URL + "/one"}, {ID: 2, URL: servers.URL + "/two"}}}); err != nil {
		t.Fatal(err)
	}
	report, err := applicationcatalog.RefreshFeeds(context.Background(), repository, rss.NewReader(servers.Client()), []applicationcatalog.RefreshRequest{{FeedID: 1}, {FeedID: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Refreshed != 2 || len(report.Failures) != 0 {
		t.Fatalf("got refresh report %+v", report)
	}
	got, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 2 {
		t.Fatalf("got episodes %+v", got.Episodes)
	}
}

func TestRefreshFeedsPersistsSuccessfulResultsAndReportsFailures(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, `<rss><channel><title>Updated</title><item><guid>new</guid><title>Episode</title><enclosure url="https://example.com/new.mp3" type="audio/mpeg"/></item></channel></rss>`)
	}))
	defer server.Close()
	repository, err := sqlitecatalog.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	want := catalog.Catalog{Feeds: []catalog.Feed{{ID: 1, URL: server.URL + "/ok"}, {ID: 2, URL: server.URL + "/fail"}}, Episodes: []catalog.Episode{{FeedID: 1, GUID: "old"}}}
	if err := repository.Save(want); err != nil {
		t.Fatal(err)
	}
	report, err := applicationcatalog.RefreshFeeds(context.Background(), repository, rss.NewReader(server.Client()), []applicationcatalog.RefreshRequest{{FeedID: 1}, {FeedID: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Refreshed != 1 || len(report.Failures) != 1 || report.Failures[0].FeedID != 2 {
		t.Fatalf("got refresh report %+v", report)
	}
	if failureErr := report.FailureError(); failureErr == nil || !strings.Contains(failureErr.Error(), "refresh feed 2") {
		t.Fatalf("got failure error %v", failureErr)
	}
	got, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 1 || got.Episodes[0].GUID != "new" {
		t.Fatalf("successful partial refresh was not persisted: %+v", got)
	}
}

func TestRefreshFeedsReturnsParentCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("canceled refresh made an HTTP request")
	}))
	defer server.Close()
	repository, err := sqlitecatalog.NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	want := catalog.Catalog{Feeds: []catalog.Feed{{ID: 1, URL: server.URL}}, Episodes: []catalog.Episode{{FeedID: 1, GUID: "old"}}}
	if err := repository.Save(want); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := applicationcatalog.RefreshFeeds(ctx, repository, rss.NewReader(server.Client()), []applicationcatalog.RefreshRequest{{FeedID: 1}}); err == nil {
		t.Fatal("canceled RefreshFeeds returned nil")
	}
	got, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 1 || got.Episodes[0].GUID != "old" {
		t.Fatalf("canceled refresh changed state: %+v", got)
	}
}
