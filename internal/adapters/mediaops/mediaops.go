package mediaops

import (
	"context"
	"errors"
	"net/http"

	"github.com/HansMontana/podsync/internal/adapters/download"
	"github.com/HansMontana/podsync/internal/adapters/metadata"
	"github.com/HansMontana/podsync/internal/domain/catalog"
)

// Operations composes the technical media adapters used by device sync.
type Operations struct{}

func New() Operations {
	return Operations{}
}

func (Operations) DownloadEpisode(ctx context.Context, client *http.Client, episode catalog.Episode, stagingDir string) (string, bool, error) {
	path, err := download.Episode(ctx, client, episode, stagingDir)
	return path, errors.Is(err, download.ErrUnsupportedMedia), err
}

func (Operations) NeedsNormalization(path string, episode catalog.Episode) (bool, error) {
	return metadata.NeedsNormalization(path, episode)
}

func (Operations) NormalizeMP3(path, feedName string, episode catalog.Episode) (bool, error) {
	return metadata.NormalizeMP3(path, feedName, episode)
}
