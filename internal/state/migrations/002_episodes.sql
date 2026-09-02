-- +goose Up

CREATE TABLE episodes (
    feed_id INTEGER NOT NULL,
    guid TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    audio_url TEXT NOT NULL,
    published_at TEXT NOT NULL,
    duration INTEGER NOT NULL,
    FOREIGN KEY (feed_id) REFERENCES feeds(id)
);

CREATE INDEX episodes_feed_id_idx ON episodes(feed_id);

-- +goose Down

DROP TABLE episodes;
