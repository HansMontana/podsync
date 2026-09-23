package sqlitecatalog

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func TestSQLiteRepositoryEnablesForeignKeysForEachConnection(t *testing.T) {
	repository, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	repository.db.SetMaxOpenConns(2)
	for range 2 {
		connection, err := repository.db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		var enabled int
		if err := connection.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			t.Fatal(err)
		}
		if enabled != 1 {
			t.Fatalf("foreign keys are disabled on a pooled connection")
		}
	}
}

func TestSQLiteRepositoryRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}

	want := catalog.Catalog{
		Feeds: []catalog.Feed{
			{
				ID:           1,
				Name:         "Example Feed",
				URL:          "https://example.com/feed.xml",
				ETag:         `"feed-1"`,
				LastModified: "Wed, 02 Sep 2026 14:39:34 +0200",
			},
			{
				ID:   2,
				Name: "Another Feed",
				URL:  "https://example.org/feed.xml",
			},
		},
		Episodes: []catalog.Episode{
			{
				FeedID:      1,
				GUID:        "episode-1",
				Title:       "First episode",
				Description: "An episode description",
				Enclosure: catalog.Enclosure{
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

	want := catalog.Catalog{Feeds: []catalog.Feed{{ID: 1, URL: "https://example.com/feed.xml"}}}
	if err := repository.Save(want); err != nil {
		t.Fatalf("initial Save() returned error: %v", err)
	}

	invalid := catalog.Catalog{Episodes: []catalog.Episode{{FeedID: 999, GUID: "orphan"}}}
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

func TestSQLiteRepositoryRejectsSymlinkedDatabase(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.db")
	link := filepath.Join(root, "state.db")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := NewSQLiteRepository(link); err == nil {
		t.Fatal("NewSQLiteRepository accepted a symlinked database")
	}
}

func TestSQLiteRepositoryInitializesZeroByteDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	if err := os.WriteFile(dbPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()
	if _, err := repository.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
}

func TestSQLiteRepositoryInitializesValidEmptySQLiteDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	createSQLiteFile(t, dbPath)
	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()
	if _, err := repository.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
}

func createSQLiteFile(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func openSQLiteFile(t *testing.T, path string) *sql.DB {
	t.Helper()
	db := createSQLiteFile(t, path)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}
