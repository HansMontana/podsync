package mediaops

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/HansMontana/podsync/internal/adapters/download"
	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func TestDownloadEpisodeReportsUnsupportedMedia(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video"))
	}))
	defer server.Close()

	path, unsupported, err := New().DownloadEpisode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL}}, t.TempDir())
	if path != "" || !unsupported || !errors.Is(err, download.ErrUnsupportedMedia) {
		t.Fatalf("got path=%q unsupported=%v error=%v", path, unsupported, err)
	}
}

func TestOperationsComposeMetadataAdapters(t *testing.T) {
	path := t.TempDir() + "/episode.mp3"
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	episode := catalog.Episode{Title: "Episode"}
	changed, err := New().NormalizeMP3(path, "Podcast", episode)
	if err != nil || !changed {
		t.Fatalf("NormalizeMP3() changed=%v error=%v", changed, err)
	}
	needs, err := New().NeedsNormalization(path, episode)
	if err != nil || needs {
		t.Fatalf("NeedsNormalization() needs=%v error=%v", needs, err)
	}
}
