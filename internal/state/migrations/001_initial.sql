-- +goose Up

CREATE TABLE feeds (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL
);

-- +goose Down

DROP TABLE feeds;
