package sqlitecatalog

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"time"

	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type SQLiteRepository struct {
	db *sql.DB
}

var ErrIncompatibleSchema = errors.New("database schema is incompatible with this podsync binary")

func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
	return newSQLiteRepository(path, migrationFS)
}

func newSQLiteRepository(path string, migrationsFS fs.FS) (*SQLiteRepository, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("state database is a symlink: %q", path)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect state database: %w", err)
	}
	db, err := sql.Open("sqlite", sqliteURL(path, false))
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	if err := validateDatabaseCompatibility(path, provider.ListSources()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := provider.Up(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run state migrations: %w", err)
	}
	if err := validateDatabaseCompatibility(path, provider.ListSources()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("validate migrated state database: %w", err)
	}

	return &SQLiteRepository{db: db}, nil
}

func NewReadOnlySQLiteRepository(path string) (*SQLiteRepository, error) {
	return newReadOnlySQLiteRepository(path, migrationFS)
}

func newReadOnlySQLiteRepository(path string, migrationsFS fs.FS) (*SQLiteRepository, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("state database is a symlink: %q", path)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect state database: %w", err)
	}
	db, err := sql.Open("sqlite", sqliteURL(path, true))
	if err != nil {
		return nil, fmt.Errorf("open read-only state database: %w", err)
	}
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	fresh, err := inspectDatabaseCompatibility(path, provider.ListSources())
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if fresh {
		_ = db.Close()
		return nil, fmt.Errorf("%w: read-only repository requires an initialized database", ErrIncompatibleSchema)
	}
	if err := validateReadableSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

func validateReadableSchema(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(episodes)")
	if err != nil {
		return fmt.Errorf("%w: inspect readable episode schema: %v", ErrIncompatibleSchema, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("%w: inspect readable episode schema: %v", ErrIncompatibleSchema, err)
		}
		if name == "author" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: inspect readable episode schema: %v", ErrIncompatibleSchema, err)
	}
	return fmt.Errorf("%w: database schema is older than this podsync binary", ErrIncompatibleSchema)
}

func validateDatabaseCompatibility(path string, sources []*goose.Source) error {
	_, err := inspectDatabaseCompatibility(path, sources)
	return err
}

func inspectDatabaseCompatibility(path string, sources []*goose.Source) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("inspect state database: %w", err)
	}
	if info.Size() == 0 {
		return true, nil
	}

	supported := make(map[int64]struct{}, len(sources))
	for _, source := range sources {
		supported[source.Version] = struct{}{}
	}

	inspection, err := sql.Open("sqlite", sqliteURL(path, true))
	if err != nil {
		return false, fmt.Errorf("open state database for compatibility check: %w", err)
	}
	defer inspection.Close()

	rows, err := inspection.Query(`
		SELECT name
		FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%'
		  AND type IN ('table', 'index', 'trigger', 'view')
		ORDER BY name
	`)
	if err != nil {
		return false, fmt.Errorf("inspect state database schema: %w", err)
	}
	var objects []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return false, fmt.Errorf("scan state database schema: %w", err)
		}
		objects = append(objects, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, fmt.Errorf("read state database schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return false, fmt.Errorf("close state database schema: %w", err)
	}

	gooseTable := false
	anchors := make(map[string]struct{})
	for _, object := range objects {
		switch object {
		case goose.DefaultTablename:
			gooseTable = true
		case "feeds", "episodes":
			anchors[object] = struct{}{}
		}
	}
	if !gooseTable {
		if len(objects) > 0 {
			return false, fmt.Errorf("%w: unversioned nonempty database", ErrIncompatibleSchema)
		}
		return true, nil
	}

	versionRows, err := inspection.Query("SELECT DISTINCT version_id FROM " + goose.DefaultTablename)
	if err != nil {
		return false, fmt.Errorf("%w: read migration metadata: %v", ErrIncompatibleSchema, err)
	}
	versionCount := 0
	hasPositiveVersion := false
	for versionRows.Next() {
		var version int64
		if err := versionRows.Scan(&version); err != nil {
			_ = versionRows.Close()
			return false, fmt.Errorf("%w: read migration version: %v", ErrIncompatibleSchema, err)
		}
		versionCount++
		if version == 0 {
			continue
		}
		hasPositiveVersion = true
		if _, exists := supported[version]; !exists {
			_ = versionRows.Close()
			return false, fmt.Errorf("%w: database records unknown migration version %d", ErrIncompatibleSchema, version)
		}
	}
	if err := versionRows.Err(); err != nil {
		_ = versionRows.Close()
		return false, fmt.Errorf("%w: read migration metadata: %v", ErrIncompatibleSchema, err)
	}
	if err := versionRows.Close(); err != nil {
		return false, fmt.Errorf("%w: close migration metadata: %v", ErrIncompatibleSchema, err)
	}
	if versionCount == 0 {
		return false, fmt.Errorf("%w: migration metadata is empty", ErrIncompatibleSchema)
	}
	if hasPositiveVersion {
		if _, exists := anchors["feeds"]; !exists {
			return false, fmt.Errorf("%w: migration metadata exists without feeds table", ErrIncompatibleSchema)
		}
		if _, exists := anchors["episodes"]; !exists {
			return false, fmt.Errorf("%w: migration metadata exists without episodes table", ErrIncompatibleSchema)
		}
	}
	return false, nil
}

func sqliteURL(path string, readOnly bool) string {
	query := url.Values{}
	query.Set("_pragma", "foreign_keys(1)")
	if readOnly {
		query.Set("mode", "ro")
	}
	return (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String()
}

func (r *SQLiteRepository) Load() (catalog.Catalog, error) {
	tx, err := r.db.BeginTx(context.Background(), nil)
	if err != nil {
		return catalog.Catalog{}, fmt.Errorf("begin state read transaction: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.Query(`
		SELECT id, name, url, etag, last_modified
		FROM feeds
		ORDER BY id
	`)
	if err != nil {
		return catalog.Catalog{}, fmt.Errorf("load feeds: %w", err)
	}
	var state catalog.Catalog

	for rows.Next() {
		var f catalog.Feed

		if err := rows.Scan(&f.ID, &f.Name, &f.URL, &f.ETag, &f.LastModified); err != nil {
			_ = rows.Close()
			return catalog.Catalog{}, fmt.Errorf("scan feed: %w", err)
		}

		state.Feeds = append(state.Feeds, f)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return catalog.Catalog{}, fmt.Errorf("iterate feeds: %w", err)
	}
	if err := rows.Close(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("close feed rows: %w", err)
	}

	rows, err = tx.Query(`
		SELECT feed_id, guid, title, description, audio_url, audio_type, audio_length, published_at, duration, author
		FROM episodes
		ORDER BY feed_id, guid, audio_url, title, published_at, duration
	`)
	if err != nil {
		return catalog.Catalog{}, fmt.Errorf("load episodes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e catalog.Episode
		var publishedAt string
		var audioURL, audioType string
		var audioLength int64
		var author string

		if err := rows.Scan(
			&e.FeedID,
			&e.GUID,
			&e.Title,
			&e.Description,
			&audioURL,
			&audioType,
			&audioLength,
			&publishedAt,
			&e.Duration,
			&author,
		); err != nil {
			_ = rows.Close()
			return catalog.Catalog{}, fmt.Errorf("scan episode: %w", err)
		}
		e.Enclosure = catalog.Enclosure{URL: audioURL, Type: audioType, Length: audioLength}
		e.Author = author

		e.PublishedAt, err = time.Parse(time.RFC3339Nano, publishedAt)
		if err != nil {
			return catalog.Catalog{}, fmt.Errorf("parse episode published time %q: %w", publishedAt, err)
		}

		state.Episodes = append(state.Episodes, e)
	}

	if err := rows.Err(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("iterate episodes: %w", err)
	}
	if err := rows.Close(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("close episode rows: %w", err)
	}

	if err := state.Validate(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("validate loaded state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("commit state read transaction: %w", err)
	}
	return state, nil
}

func (r *SQLiteRepository) Save(state catalog.Catalog) error {
	if err := state.Validate(); err != nil {
		return fmt.Errorf("validate state: %w", err)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin state transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM episodes`); err != nil {
		return fmt.Errorf("clear episodes: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM feeds`); err != nil {
		return fmt.Errorf("clear feeds: %w", err)
	}

	for _, f := range state.Feeds {
		_, err := tx.Exec(`
			INSERT INTO feeds (id, name, url, etag, last_modified)
			VALUES (?, ?, ?, ?, ?)
		`, f.ID, f.Name, f.URL, f.ETag, f.LastModified)
		if err != nil {
			return fmt.Errorf("save feed %d: %w", f.ID, err)
		}
	}

	for _, e := range state.Episodes {
		_, err := tx.Exec(`
			INSERT INTO episodes (
				feed_id, guid, title, description, audio_url, audio_type, audio_length, published_at, duration, author
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			e.FeedID,
			e.GUID,
			e.Title,
			e.Description,
			e.Enclosure.URL,
			e.Enclosure.Type,
			e.Enclosure.Length,
			e.PublishedAt.UTC().Format(time.RFC3339Nano),
			e.Duration,
			e.Author,
		)
		if err != nil {
			return fmt.Errorf("save episode for feed %d: %w", e.FeedID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit state transaction: %w", err)
	}

	return nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}
