package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/pressly/goose/v3"
)

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

func TestReadOnlySQLiteRepositoryOpensCurrentSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	createCurrentRepository(t, dbPath).Close()

	repository, err := NewReadOnlySQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("NewReadOnlySQLiteRepository() returned error: %v", err)
	}
	defer repository.Close()
	if _, err := repository.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
}

func TestReadOnlySQLiteRepositoryRejectsOlderSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository, err := newSQLiteRepository(dbPath, versionOneMigrations())
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := NewReadOnlySQLiteRepository(dbPath); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("NewReadOnlySQLiteRepository() error = %v, want schema compatibility error", err)
	}
}

func TestReadOnlySQLiteRepositoryRejectsNewerSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	createCurrentRepository(t, dbPath).Close()

	if _, err := newReadOnlySQLiteRepository(dbPath, versionOneMigrations()); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("newReadOnlySQLiteRepository() error = %v, want schema compatibility error", err)
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

func TestSQLiteRepositoryFailedMigrationRollsBackAndCanResume(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	repository, err := newSQLiteRepository(dbPath, versionOneMigrations())
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := newSQLiteRepository(dbPath, failingMigrationSet()); err == nil {
		t.Fatal("newSQLiteRepository() accepted a failed migration")
	}
	check := openSQLiteFile(t, dbPath)
	var attempted int
	if err := check.QueryRow("SELECT count(*) FROM pragma_table_info('episodes') WHERE name = 'attempted'").Scan(&attempted); err != nil {
		check.Close()
		t.Fatal(err)
	}
	if attempted != 0 {
		check.Close()
		t.Fatal("failed migration schema change was not rolled back")
	}
	var applied int
	if err := check.QueryRow("SELECT count(*) FROM goose_db_version WHERE version_id = 2").Scan(&applied); err != nil {
		check.Close()
		t.Fatal(err)
	}
	if applied != 0 {
		check.Close()
		t.Fatalf("failed migration version was recorded: %d", applied)
	}
	if err := check.Close(); err != nil {
		t.Fatal(err)
	}

	repository, err = NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("database did not resume after failed migration: %v", err)
	}
	defer repository.Close()
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

func failingMigrationSet() fstest.MapFS {
	result := versionOneMigrations()
	result["migrations/002_failing.sql"] = &fstest.MapFile{Data: []byte(`-- +goose Up

ALTER TABLE episodes ADD COLUMN attempted TEXT;
SELECT podsync_test_failure;

-- +goose Down

SELECT podsync_test_failure;
`)}
	return result
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
