package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/HansMontana/podsync/internal/episode"
)

const maxDownloadSize int64 = 1 << 30

// Episode downloads audio into host-side staging storage.
func Episode(ctx context.Context, client *http.Client, e episode.Episode, stagingDir string) (path string, err error) {
	if e.Enclosure.URL == "" {
		return "", fmt.Errorf("episode has no audio URL")
	}
	if e.Enclosure.Length > maxDownloadSize {
		return "", fmt.Errorf("episode enclosure exceeds %d bytes", maxDownloadSize)
	}
	if client == nil {
		client = http.DefaultClient
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", fmt.Errorf("create staging directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.Enclosure.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create audio request: %w", err)
	}
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download episode: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download episode: unexpected HTTP status %s", response.Status)
	}
	if response.ContentLength > maxDownloadSize {
		return "", fmt.Errorf("download response exceeds %d bytes", maxDownloadSize)
	}

	file, err := os.CreateTemp(stagingDir, "podsync-download-*")
	if err != nil {
		return "", fmt.Errorf("create staging file: %w", err)
	}
	path = file.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()

	bytes, err := io.Copy(file, io.LimitReader(response.Body, maxDownloadSize+1))
	if err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write staged episode: %w", err)
	}
	if bytes > maxDownloadSize {
		_ = file.Close()
		return "", fmt.Errorf("download exceeds %d bytes", maxDownloadSize)
	}
	if bytes == 0 {
		_ = file.Close()
		return "", fmt.Errorf("download is empty")
	}
	if e.Enclosure.Length > 0 && bytes != e.Enclosure.Length {
		_ = file.Close()
		return "", fmt.Errorf("download size %d does not match enclosure size %d", bytes, e.Enclosure.Length)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close staged episode: %w", err)
	}
	return path, nil
}
