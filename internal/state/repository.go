package state

import (
	"database/sql"
	"embed"
	"fmt"

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
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}

	goose.SetBaseFS(migrationFS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set migration dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run state migrations: %w", err)
	}

	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Load() (State, error) {
	rows, err := r.db.Query(`
		SELECT id, name, url
		FROM feeds
		ORDER BY id
	`)
	if err != nil {
		return State{}, fmt.Errorf("load feeds: %w", err)
	}
	defer rows.Close()

	var state State

	for rows.Next() {
		var f feed.Feed

		if err := rows.Scan(&f.ID, &f.Name, &f.URL); err != nil {
			return State{}, fmt.Errorf("scan feed: %w", err)
		}

		state.Feeds = append(state.Feeds, f)
	}

	if err := rows.Err(); err != nil {
		return State{}, fmt.Errorf("iterate feeds: %w", err)
	}

	return state, nil
}

func (r *SQLiteRepository) Save(state State) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin state transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM feeds`); err != nil {
		return fmt.Errorf("clear feeds: %w", err)
	}

	for _, f := range state.Feeds {
		_, err := tx.Exec(`
			INSERT INTO feeds (id, name, url)
			VALUES (?, ?, ?)
		`, f.ID, f.Name, f.URL)
		if err != nil {
			return fmt.Errorf("save feed %d: %w", f.ID, err)
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
