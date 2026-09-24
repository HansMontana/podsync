package catalog

import (
	"context"
	"errors"
	"fmt"
	"sync"

	domaincatalog "github.com/HansMontana/podsync/internal/domain/catalog"
)

// FeedReader fetches and parses a catalog feed without exposing its transport
// or feed format to the application layer.
type FeedReader interface {
	Fetch(context.Context, domaincatalog.Feed) (FetchResult, error)
	Parse(FetchResult, domaincatalog.Feed) (domaincatalog.Feed, []domaincatalog.Episode, error)
}

// FetchResult contains a fetched feed and its cache metadata.
type FetchResult struct {
	Body         []byte
	ETag         string
	LastModified string
	NotModified  bool
}

// RefreshFeed fetches and persists the current episodes for one feed.
func RefreshFeed(ctx context.Context, repository Repository, reader FeedReader, feedID int64) error {
	return RefreshFeedWithArchive(ctx, repository, reader, feedID, false)
}

// RefreshFeedWithArchive retains known historical episodes for an archive
// feed when the source only exposes a recent window.
func RefreshFeedWithArchive(ctx context.Context, repository Repository, reader FeedReader, feedID int64, archive bool) error {
	current, err := repository.Load()
	if err != nil {
		return fmt.Errorf("load state for feed refresh: %w", err)
	}

	feedIndex := -1
	for i, f := range current.Feeds {
		if f.ID == feedID {
			feedIndex = i
			break
		}
	}
	if feedIndex == -1 {
		return fmt.Errorf("refresh feed %d: feed not found", feedID)
	}

	result, err := reader.Fetch(ctx, current.Feeds[feedIndex])
	if err != nil {
		return err
	}
	if result.NotModified {
		return nil
	}

	refreshedFeed, episodes, err := reader.Parse(result, current.Feeds[feedIndex])
	if err != nil {
		return fmt.Errorf("refresh feed %d: %w", feedID, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	refreshedFeed.ETag = result.ETag
	refreshedFeed.LastModified = result.LastModified
	updated, err := domaincatalog.ApplyRefresh(current, domaincatalog.RefreshResult{Feed: refreshedFeed, Episodes: episodes, Archive: archive})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := repository.Save(updated); err != nil {
		return fmt.Errorf("save refreshed feed %d: %w", feedID, err)
	}
	return nil
}

// RefreshFeeds fetches and persists multiple feeds with one state load and save.
// Feed-level failures are returned in the report after successful results are
// persisted. Fatal application errors are returned separately.
func RefreshFeeds(ctx context.Context, repository Repository, reader FeedReader, requests []RefreshRequest) (RefreshReport, error) {
	current, err := repository.Load()
	if err != nil {
		return RefreshReport{}, fmt.Errorf("load state for feed refresh: %w", err)
	}
	results, failures, err := fetchRefreshResults(ctx, reader, current, requests)
	if err != nil {
		return RefreshReport{}, err
	}
	for _, result := range results {
		if err := ctx.Err(); err != nil {
			return RefreshReport{}, err
		}
		current, err = domaincatalog.ApplyRefresh(current, result)
		if err != nil {
			return RefreshReport{}, err
		}
	}
	if len(results) > 0 {
		if err := ctx.Err(); err != nil {
			return RefreshReport{}, err
		}
		if err := repository.Save(current); err != nil {
			return RefreshReport{}, fmt.Errorf("save refreshed feeds: %w", err)
		}
	}
	return RefreshReport{Refreshed: len(requests) - len(failures), Failures: failures}, nil
}

const refreshWorkers = 4

func fetchRefreshResults(ctx context.Context, reader FeedReader, current domaincatalog.Catalog, requests []RefreshRequest) ([]domaincatalog.RefreshResult, []RefreshFailure, error) {
	if len(requests) == 0 {
		return nil, nil, nil
	}
	parentCtx := ctx
	workerCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	results := make([]domaincatalog.RefreshResult, len(requests))
	valid := make([]bool, len(requests))
	completed := make([]bool, len(requests))
	failures := make([]RefreshFailure, len(requests))
	jobs := make(chan int)
	var workers sync.WaitGroup
	workerCount := refreshWorkers
	if len(requests) < workerCount {
		workerCount = len(requests)
	}
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if workerCtx.Err() != nil {
					continue
				}
				request := requests[index]
				known, err := findFeed(current, request.FeedID)
				if err == nil {
					response, fetchErr := reader.Fetch(workerCtx, known)
					if fetchErr != nil {
						err = fetchErr
					} else if !response.NotModified {
						var episodes []domaincatalog.Episode
						var refreshed domaincatalog.Feed
						refreshed, episodes, err = reader.Parse(response, known)
						if err == nil {
							refreshed.ETag = response.ETag
							refreshed.LastModified = response.LastModified
							results[index] = domaincatalog.RefreshResult{Feed: refreshed, Episodes: episodes, Archive: request.Archive}
							// Each job owns a distinct result slot; application remains ordered below.
							valid[index] = true
						}
					} else {
						completed[index] = true
					}
				}
				if err != nil {
					if parentCtx.Err() == nil {
						failures[index] = RefreshFailure{FeedID: request.FeedID, Err: err}
						completed[index] = true
					}
				}
			}
		}()
	}
	for index := range requests {
		select {
		case jobs <- index:
		case <-workerCtx.Done():
			break
		}
		if workerCtx.Err() != nil {
			break
		}
	}
	close(jobs)
	workers.Wait()
	if err := parentCtx.Err(); err != nil {
		return nil, nil, err
	}
	ordered := make([]domaincatalog.RefreshResult, 0, len(requests))
	orderedFailures := make([]RefreshFailure, 0)
	for index := range results {
		if valid[index] {
			ordered = append(ordered, results[index])
		}
		if completed[index] && failures[index].Err != nil {
			orderedFailures = append(orderedFailures, failures[index])
		}
	}
	return ordered, orderedFailures, nil
}

type RefreshReport struct {
	Refreshed int
	Failures  []RefreshFailure
}

func (r RefreshReport) FailureError() error {
	if len(r.Failures) == 0 {
		return nil
	}
	errs := make([]error, len(r.Failures))
	for i, failure := range r.Failures {
		errs[i] = failure
	}
	return fmt.Errorf("refresh incomplete: %w", errors.Join(errs...))
}

type RefreshFailure struct {
	FeedID int64
	Err    error
}

func (f RefreshFailure) Error() string {
	return fmt.Sprintf("refresh feed %d: %v", f.FeedID, f.Err)
}

func (f RefreshFailure) Unwrap() error {
	return f.Err
}

type RefreshRequest struct {
	FeedID  int64
	Archive bool
}

func findFeed(current domaincatalog.Catalog, feedID int64) (domaincatalog.Feed, error) {
	for _, known := range current.Feeds {
		if known.ID == feedID {
			return known, nil
		}
	}
	return domaincatalog.Feed{}, fmt.Errorf("refresh feed %d: feed not found", feedID)
}
