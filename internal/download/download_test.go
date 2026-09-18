package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/HansMontana/podsync/internal/episode"
)

func TestEpisodeDownloadsToStaging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("audio data"))
	}))
	defer server.Close()

	staging := filepath.Join(t.TempDir(), "staging")
	result, err := Episode(context.Background(), server.Client(), episode.Episode{Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, staging)
	if err != nil {
		t.Fatalf("Episode() returned error: %v", err)
	}
	if result.Bytes != int64(len("audio data")) {
		t.Fatalf("got %d bytes", result.Bytes)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("ReadFile() returned error: %v", err)
	}
	if string(data) != "audio data" {
		t.Fatalf("got %q", data)
	}
	if filepath.Dir(result.Path) != staging {
		t.Fatalf("download escaped staging directory: %q", result.Path)
	}
}

func TestEpisodeRejectsFailedResponse(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	if _, err := Episode(context.Background(), server.Client(), episode.Episode{Enclosure: episode.Enclosure{URL: server.URL}}, t.TempDir()); err == nil {
		t.Fatal("Episode() accepted a failed response")
	}
}

func TestEpisodeRejectsMissingAudioURL(t *testing.T) {
	if _, err := Episode(context.Background(), nil, episode.Episode{}, t.TempDir()); err == nil {
		t.Fatal("Episode() accepted an episode without an audio URL")
	}
}
