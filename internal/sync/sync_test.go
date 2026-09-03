package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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
	initial := state.State{
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

	want := state.State{
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

	want := state.State{
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
