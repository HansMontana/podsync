package state

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Repository interface {
	Load() (State, error)
	Save(State) error
	Close() error
}

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", sqliteURL(path, false))
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	migrations, err := fs.Sub(migrationFS, "migrations")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run state migrations: %w", err)
	}

	return &SQLiteRepository{db: db}, nil
}

func NewReadOnlySQLiteRepository(path string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", sqliteURL(path, true))
	if err != nil {
		return nil, fmt.Errorf("open read-only state database: %w", err)
	}
	return &SQLiteRepository{db: db}, nil
}

func sqliteURL(path string, readOnly bool) string {
	query := url.Values{}
	query.Set("_pragma", "foreign_keys(1)")
	if readOnly {
		query.Set("mode", "ro")
	}
	return (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String()
}

func (r *SQLiteRepository) Load() (State, error) {
	rows, err := r.db.Query(`
		SELECT id, name, url, etag, last_modified
		FROM feeds
		ORDER BY id
	`)
	if err != nil {
		return State{}, fmt.Errorf("load feeds: %w", err)
	}
	var state State

	for rows.Next() {
		var f feed.Feed

		if err := rows.Scan(&f.ID, &f.Name, &f.URL, &f.ETag, &f.LastModified); err != nil {
			return State{}, fmt.Errorf("scan feed: %w", err)
		}

		state.Feeds = append(state.Feeds, f)
	}

	if err := rows.Err(); err != nil {
		return State{}, fmt.Errorf("iterate feeds: %w", err)
	}
	if err := rows.Close(); err != nil {
		return State{}, fmt.Errorf("close feed rows: %w", err)
	}

	rows, err = r.db.Query(`
		SELECT feed_id, guid, title, description, audio_url, audio_type, audio_length, published_at, duration, author
		FROM episodes
		ORDER BY feed_id, guid, audio_url, title, published_at, duration
	`)
	if err != nil {
		return State{}, fmt.Errorf("load episodes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e episode.Episode
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
			return State{}, fmt.Errorf("scan episode: %w", err)
		}
		e.Enclosure = episode.Enclosure{URL: audioURL, Type: audioType, Length: audioLength}
		e.Author = author

		e.PublishedAt, err = time.Parse(time.RFC3339Nano, publishedAt)
		if err != nil {
			return State{}, fmt.Errorf("parse episode published time %q: %w", publishedAt, err)
		}

		state.Episodes = append(state.Episodes, e)
	}

	if err := rows.Err(); err != nil {
		return State{}, fmt.Errorf("iterate episodes: %w", err)
	}

	if err := state.Validate(); err != nil {
		return State{}, fmt.Errorf("validate loaded state: %w", err)
	}
	return state, nil
}

func (r *SQLiteRepository) Save(state State) error {
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
