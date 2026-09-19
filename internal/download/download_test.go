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
	path, err := Episode(context.Background(), server.Client(), episode.Episode{Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, staging)
	if err != nil {
		t.Fatalf("Episode() returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() returned error: %v", err)
	}
	if string(data) != "audio data" {
		t.Fatalf("got %q", data)
	}
	if filepath.Dir(path) != staging {
		t.Fatalf("download escaped staging directory: %q", path)
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

func TestEpisodeRejectsEmptyAndWrongSizedDownloads(t *testing.T) {
	for _, current := range []episode.Episode{
		{Enclosure: episode.Enclosure{URL: "", Length: 1}},
		{Enclosure: episode.Enclosure{Length: 5}},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		current.Enclosure.URL = server.URL
		if _, err := Episode(context.Background(), server.Client(), current, t.TempDir()); err == nil {
			t.Fatal("Episode() accepted an invalid download")
		}
		server.Close()
	}
}
