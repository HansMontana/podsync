-- +goose Up

ALTER TABLE feeds ADD COLUMN etag TEXT NOT NULL DEFAULT '';
ALTER TABLE feeds ADD COLUMN last_modified TEXT NOT NULL DEFAULT '';

-- +goose Down

-- SQLite does not support dropping columns on all supported versions.
