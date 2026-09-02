-- +goose Up

ALTER TABLE episodes ADD COLUMN audio_type TEXT NOT NULL DEFAULT '';
ALTER TABLE episodes ADD COLUMN audio_length INTEGER NOT NULL DEFAULT 0;

-- +goose Down

-- SQLite does not support dropping columns on all supported versions.
