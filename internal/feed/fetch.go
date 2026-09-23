package feed

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

const maxRSSSize = 64 << 20

// FetchResult contains an RSS response and its HTTP cache metadata.
type FetchResult struct {
	Body         []byte
	ETag         string
	LastModified string
	NotModified  bool
}

// FetchRSS fetches a feed, using its cached HTTP metadata when available.
func FetchRSS(ctx context.Context, client *http.Client, f catalog.Feed) (FetchResult, error) {
	url, err := catalog.NormalizeURL(f.URL)
	if err != nil {
		return FetchResult{}, fmt.Errorf("normalize feed URL: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("create RSS request: %w", err)
	}
	if f.ETag != "" {
		req.Header.Set("If-None-Match", f.ETag)
	}
	if f.LastModified != "" {
		req.Header.Set("If-Modified-Since", f.LastModified)
	}

	response, err := client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch RSS: %w", err)
	}
	defer response.Body.Close()

	result := FetchResult{
		ETag:         response.Header.Get("ETag"),
		LastModified: response.Header.Get("Last-Modified"),
	}
	if response.StatusCode == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}
	if response.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("fetch RSS: unexpected HTTP status %s", response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxRSSSize+1))
	if err != nil {
		return FetchResult{}, fmt.Errorf("read RSS response: %w", err)
	}
	if len(body) > maxRSSSize {
		return FetchResult{}, fmt.Errorf("RSS response exceeds %d bytes", maxRSSSize)
	}
	result.Body = body

	return result, nil
}
