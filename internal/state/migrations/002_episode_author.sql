-- +goose Up

ALTER TABLE episodes ADD COLUMN author TEXT NOT NULL DEFAULT '';

-- +goose Down

-- Down migrations are intentionally unsupported. Restore a device backup to
-- downgrade podsync instead of changing a live database in place.
SELECT podsync_down_migrations_are_not_supported;
