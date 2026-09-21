package download

import (
	"context"
	"errors"
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

func TestEpisodeRejectsNonAudioResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video"))
	}))
	defer server.Close()

	_, err := Episode(context.Background(), server.Client(), episode.Episode{Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, t.TempDir())
	if !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("Episode() error = %v, want unsupported media", err)
	}
}

func TestEpisodeRejectsVideoSignatureWithoutContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'})
	}))
	defer server.Close()

	_, err := Episode(context.Background(), server.Client(), episode.Episode{Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, t.TempDir())
	if !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("Episode() error = %v, want unsupported media", err)
	}
}

func TestEpisodeRejectsOversizedResponseAsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "1073741825")
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	_, err := Episode(context.Background(), server.Client(), episode.Episode{Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, t.TempDir())
	if !errors.Is(err, ErrOversizedMedia) {
		t.Fatalf("Episode() error = %v, want oversized media", err)
	}
}

func TestEpisodeRejectsMissingAudioURL(t *testing.T) {
	if _, err := Episode(context.Background(), nil, episode.Episode{}, t.TempDir()); err == nil {
		t.Fatal("Episode() accepted an episode without an audio URL")
	}
}

func TestEpisodeRejectsEmptyDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	if _, err := Episode(context.Background(), server.Client(), episode.Episode{Enclosure: episode.Enclosure{URL: server.URL}}, t.TempDir()); err == nil {
		t.Fatal("Episode() accepted an empty download")
	}
}

func TestEpisodeAllowsAdvisoryEnclosureSizeMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()
	path, err := Episode(context.Background(), server.Client(), episode.Episode{Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg", Length: 6}}, t.TempDir())
	if err != nil {
		t.Fatalf("Episode() rejected an advisory size mismatch: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "audio" {
		t.Fatalf("got %q", data)
	}
}
