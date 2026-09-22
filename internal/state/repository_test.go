package state

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/pressly/goose/v3"
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

	want := State{
		Feeds: []feed.Feed{
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

func TestSQLiteRepositoryUpgradesVersionOneDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository, err := newSQLiteRepository(dbPath, versionOneMigrations())
	if err != nil {
		t.Fatalf("create version-one database: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	repository, err = NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("upgrade database: %v", err)
	}
	defer repository.Close()
	var hasAuthor bool
	rows, err := repository.db.Query("PRAGMA table_info(episodes)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "author" {
			hasAuthor = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !hasAuthor {
		t.Fatal("upgraded schema has no author column")
	}
}

func TestSQLiteRepositoryRejectsNewerDatabaseBeforeWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	want := State{
		Feeds: []feed.Feed{{ID: 1, URL: "https://example.com/feed.xml"}},
		Episodes: []episode.Episode{{
			FeedID: 1,
			GUID:   "episode-1",
			Author: "Original author",
		}},
	}
	if err := repository.Save(want); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = newSQLiteRepository(dbPath, versionOneMigrations())
	if !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("newSQLiteRepository() error = %v, want schema compatibility error", err)
	}
	check := openSQLiteFile(t, dbPath)
	defer check.Close()
	var author string
	if err := check.QueryRow("SELECT author FROM episodes WHERE guid = 'episode-1'").Scan(&author); err != nil {
		t.Fatal(err)
	}
	if author != "Original author" {
		t.Fatalf("author = %q, want unchanged author", author)
	}
	var count int
	if err := check.QueryRow("SELECT count(*) FROM goose_db_version WHERE version_id = 2 AND is_applied = 1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration version 2 count = %d, want 1", count)
	}
}

func TestSQLiteRepositoryAllowsHistoricalMigrationRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository := createCurrentRepository(t, dbPath)
	if _, err := repository.db.Exec("INSERT INTO goose_db_version (version_id, is_applied) VALUES (2, 0)"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	repository, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("historical migration rows were rejected: %v", err)
	}
	defer repository.Close()
}

func TestCompatibilityGuardDoesNotRejectMissingVersionZero(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository := createCurrentRepository(t, dbPath)
	if _, err := repository.db.Exec("DELETE FROM goose_db_version WHERE version_id = 0"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	check := openSQLiteFile(t, dbPath)
	provider, err := goose.NewProvider(goose.DialectSQLite3, check, migrationFSForProvider(t))
	if err != nil {
		check.Close()
		t.Fatal(err)
	}
	if err := validateDatabaseCompatibility(dbPath, provider.ListSources()); err != nil {
		t.Fatalf("compatibility guard rejected missing version zero: %v", err)
	}
	check.Close()
}

func TestSQLiteRepositoryRejectsUnknownMigrationVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository := createCurrentRepository(t, dbPath)
	if _, err := repository.db.Exec("INSERT INTO goose_db_version (version_id, is_applied) VALUES (99, 1)"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := NewSQLiteRepository(dbPath); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("NewSQLiteRepository() error = %v, want schema compatibility error", err)
	}
	check := openSQLiteFile(t, dbPath)
	defer check.Close()
	var count int
	if err := check.QueryRow("SELECT count(*) FROM goose_db_version WHERE version_id = 99").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unknown migration row count = %d, want 1", count)
	}
}

func TestSQLiteRepositoryRejectsUnversionedNonemptyDatabase(t *testing.T) {
	for _, name := range []string{"unrelated", "feeds"} {
		t.Run(name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "state.db")
			db := createSQLiteFile(t, dbPath)
			statement := "CREATE TABLE unrelated (id INTEGER)"
			if name == "feeds" {
				statement = "CREATE TABLE feeds (id INTEGER)"
			}
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := NewSQLiteRepository(dbPath); !errors.Is(err, ErrIncompatibleSchema) {
				t.Fatalf("NewSQLiteRepository() error = %v, want schema compatibility error", err)
			}
		})
	}
}

func TestSQLiteRepositoryRejectsPositiveMigrationsWithoutAnchors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository := createCurrentRepository(t, dbPath)
	if _, err := repository.db.Exec("DROP TABLE episodes"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLiteRepository(dbPath); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("NewSQLiteRepository() error = %v, want schema compatibility error", err)
	}
}

func TestSQLiteRepositoryRejectsMalformedDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	if err := os.WriteFile(dbPath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLiteRepository(dbPath); err == nil {
		t.Fatal("NewSQLiteRepository() accepted malformed database")
	}
}

func TestSQLiteRepositoryRejectsMalformedMigrationMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository := createCurrentRepository(t, dbPath)
	if _, err := repository.db.Exec("ALTER TABLE goose_db_version RENAME TO old_goose_db_version"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.db.Exec("CREATE TABLE goose_db_version (version_id TEXT, is_applied INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.db.Exec("INSERT INTO goose_db_version VALUES ('not-a-version', 1)"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLiteRepository(dbPath); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("NewSQLiteRepository() error = %v, want schema compatibility error", err)
	}
}

func TestSQLiteRepositoryRejectsEmptyMigrationMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	db := createSQLiteFile(t, dbPath)
	if _, err := db.Exec("CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER NOT NULL, is_applied INTEGER NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLiteRepository(dbPath); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("NewSQLiteRepository() error = %v, want schema compatibility error", err)
	}
}

func versionOneMigrations() fstest.MapFS {
	data, err := fs.ReadFile(migrationFS, "migrations/001_initial.sql")
	if err != nil {
		panic(err)
	}
	return fstest.MapFS{
		"migrations/001_initial.sql": &fstest.MapFile{Data: data},
	}
}

func migrationFSForProvider(t *testing.T) fs.FS {
	t.Helper()
	result, err := fs.Sub(migrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func createCurrentRepository(t *testing.T, path string) *SQLiteRepository {
	t.Helper()
	repository, err := NewSQLiteRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	return repository
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
