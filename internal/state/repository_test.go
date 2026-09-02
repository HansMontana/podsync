package state

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
)

func TestSQLiteRepositoryRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}

	want := State{
		Feeds: []feed.Feed{
			{
				ID:   1,
				Name: "Example Feed",
				URL:  "https://example.com/feed.xml",
			},
			{
				ID:   2,
				Name: "Another Feed",
				URL:  "https://example.org/feed.xml",
			},
		},
		Episodes: []episode.Episode{
			{
				FeedID:      1,
				GUID:        "episode-1",
				Title:       "First episode",
				Description: "An episode description",
				Enclosure: episode.Enclosure{
					URL:    "https://example.com/episode-1.mp3",
					Type:   "audio/mpeg",
					Length: 1234,
				},
				PublishedAt: time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC),
				Duration:    45 * time.Minute,
			},
		},
	}

	if err := repository.Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	repository, err = NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}
	defer repository.Close()

	got, err := repository.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got state %+v, want %+v", got, want)
	}
}

func TestSQLiteRepositoryRejectsInvalidStateWithoutChangingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

	want := State{Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/feed.xml"}}}
	if err := repository.Save(want); err != nil {
		t.Fatalf("initial Save() returned error: %v", err)
	}

	invalid := State{Episodes: []episode.Episode{{FeedID: 999, GUID: "orphan"}}}
	if err := repository.Save(invalid); err == nil {
		t.Fatal("Save() accepted an episode for an unknown feed")
	}

	got, err := repository.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got state %+v after rejected save, want %+v", got, want)
	}
}

func TestSQLiteRepositoryCreatesSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

	state, err := repository.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if len(state.Feeds) != 0 {
		t.Fatalf("got %d feeds from new database, want 0", len(state.Feeds))
	}
}
