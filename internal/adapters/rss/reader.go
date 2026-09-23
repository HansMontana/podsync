package rss

import (
	"bytes"
	"context"
	"net/http"

	applicationcatalog "github.com/HansMontana/podsync/internal/application/catalog"
	"github.com/HansMontana/podsync/internal/domain/catalog"
)

// Reader adapts RSS fetching and parsing to the catalog application port.
type Reader struct {
	Client *http.Client
}

func NewReader(client *http.Client) Reader {
	return Reader{Client: client}
}

func (r Reader) Fetch(ctx context.Context, feed catalog.Feed) (applicationcatalog.FetchResult, error) {
	result, err := FetchRSS(ctx, r.Client, feed)
	if err != nil {
		return applicationcatalog.FetchResult{}, err
	}
	return applicationcatalog.FetchResult{
		Body:         result.Body,
		ETag:         result.ETag,
		LastModified: result.LastModified,
		NotModified:  result.NotModified,
	}, nil
}

func (Reader) Parse(result applicationcatalog.FetchResult, feed catalog.Feed) (catalog.Feed, []catalog.Episode, error) {
	return ParseRSS(bytes.NewReader(result.Body), feed)
}
