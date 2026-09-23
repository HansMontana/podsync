package device

import (
	"context"
	"net/http"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

// MediaOperations provides the media work required by device synchronization.
// Implementations own the technical details of downloading and tagging files.
type MediaOperations interface {
	DownloadEpisode(context.Context, *http.Client, catalog.Episode, string) (string, bool, error)
	NeedsNormalization(string, catalog.Episode) (bool, error)
	NormalizeMP3(string, string, catalog.Episode) (bool, error)
}
