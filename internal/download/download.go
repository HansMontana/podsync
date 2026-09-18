package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/HansMontana/podsync/internal/episode"
)

const maxDownloadSize int64 = 1 << 30

type Result struct {
	Path  string
	Bytes int64
}

// Episode downloads audio into host-side staging storage.
func Episode(ctx context.Context, client *http.Client, e episode.Episode, stagingDir string) (result Result, err error) {
	if e.Enclosure.URL == "" {
		return Result{}, fmt.Errorf("episode has no audio URL")
	}
	if e.Enclosure.Length > maxDownloadSize {
		return Result{}, fmt.Errorf("episode enclosure exceeds %d bytes", maxDownloadSize)
	}
	if client == nil {
		client = http.DefaultClient
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create staging directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.Enclosure.URL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create audio request: %w", err)
	}
	response, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("download episode: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Result{}, fmt.Errorf("download episode: unexpected HTTP status %s", response.Status)
	}
	if response.ContentLength > maxDownloadSize {
		return Result{}, fmt.Errorf("download response exceeds %d bytes", maxDownloadSize)
	}

	file, err := os.CreateTemp(stagingDir, "podsync-download-*")
	if err != nil {
		return Result{}, fmt.Errorf("create staging file: %w", err)
	}
	path := file.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()

	bytes, err := io.Copy(file, io.LimitReader(response.Body, maxDownloadSize+1))
	if err != nil {
		_ = file.Close()
		return Result{}, fmt.Errorf("write staged episode: %w", err)
	}
	if bytes > maxDownloadSize {
		_ = file.Close()
		return Result{}, fmt.Errorf("download exceeds %d bytes", maxDownloadSize)
	}
	if err := file.Close(); err != nil {
		return Result{}, fmt.Errorf("close staged episode: %w", err)
	}

	result.Path = filepath.Clean(path)
	result.Bytes = bytes
	return result, nil
}
