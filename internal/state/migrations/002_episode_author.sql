-- +goose Up

ALTER TABLE episodes ADD COLUMN author TEXT NOT NULL DEFAULT '';

-- +goose Down

-- SQLite cannot safely remove this column without rebuilding the table.
