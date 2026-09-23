package download

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func TestEpisodeDownloadsToStaging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("audio data"))
	}))
	defer server.Close()

	staging := filepath.Join(t.TempDir(), "staging")
	path, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, staging)
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

	if _, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL}}, t.TempDir()); err == nil {
		t.Fatal("Episode() accepted a failed response")
	}
}

func TestEpisodeRejectsNonAudioResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video"))
	}))
	defer server.Close()

	_, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, t.TempDir())
	if !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("Episode() error = %v, want unsupported media", err)
	}
}

func TestEpisodeRejectsHTMLResponseDeclaredAsAudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>not audio</body></html>"))
	}))
	defer server.Close()

	_, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, t.TempDir())
	if !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("Episode() error = %v, want unsupported media", err)
	}
}

func TestEpisodeRejectsVideoSignatureWithoutContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'})
	}))
	defer server.Close()

	_, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, t.TempDir())
	if !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("Episode() error = %v, want unsupported media", err)
	}
}

func TestEpisodeAllowsEnclosureLengthAboveHistoricalLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	path, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg", Length: 1<<30 + 1}}, t.TempDir())
	if err != nil {
		t.Fatalf("Episode() rejected large advisory response length: %v", err)
	}
	if data, readErr := os.ReadFile(path); readErr != nil || string(data) != "audio" {
		t.Fatalf("got data %q, error %v", data, readErr)
	}
}

func TestEpisodeRejectsMissingAudioURL(t *testing.T) {
	if _, err := Episode(context.Background(), nil, catalog.Episode{}, t.TempDir()); err == nil {
		t.Fatal("Episode() accepted an episode without an audio URL")
	}
}

func TestEpisodeRejectsEmptyDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	if _, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL}}, t.TempDir()); err == nil {
		t.Fatal("Episode() accepted an empty download")
	}
}

func TestEpisodeAllowsAdvisoryEnclosureSizeMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()
	path, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg", Length: 6}}, t.TempDir())
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

func TestEpisodeRejectsResponseAboveMaximumSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "2147483649")
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	_, err := Episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, t.TempDir())
	if !errors.Is(err, ErrDownloadTooLarge) {
		t.Fatalf("Episode() error = %v, want size error", err)
	}
}

func TestEpisodeRejectsChunkedResponseAboveMaximumSize(t *testing.T) {
	const testLimit int64 = 2 << 20
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		for written := int64(0); written <= testLimit; written += 1 << 20 {
			chunk := make([]byte, 1<<20)
			if remaining := testLimit + 1 - written; remaining < int64(len(chunk)) {
				chunk = chunk[:remaining]
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	_, err := episode(context.Background(), server.Client(), catalog.Episode{Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}, t.TempDir(), testLimit)
	if !errors.Is(err, ErrDownloadTooLarge) {
		t.Fatalf("Episode() error = %v, want size error", err)
	}
}
