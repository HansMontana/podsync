package state

import (
	"path/filepath"
	"testing"

	"github.com/HansMontana/podsync/internal/feed"
)

func TestSQLiteRepositoryRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()

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
	}

	if err := repository.Save(want); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	got, err := repository.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if len(got.Feeds) != len(want.Feeds) {
		t.Fatalf("got %d feeds, want %d", len(got.Feeds), len(want.Feeds))
	}

	if len(got.Feeds) != len(want.Feeds) {
		t.Fatalf("got %d feeds, want %d", len(got.Feeds), len(want.Feeds))
	}

	for i := range want.Feeds {
		if got.Feeds[i] != want.Feeds[i] {
			t.Fatalf("got feed %+v, want %+v at index %d", got.Feeds[i], want.Feeds[i], i)
		}
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
