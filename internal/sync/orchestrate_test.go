package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HansMontana/podsync/internal/adapters/devicefs"
	"github.com/HansMontana/podsync/internal/adapters/media"
	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func TestEpisodesStagesBeforeApplyingAndDeletesOnlyManagedFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Podcasts/podcast-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Music"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Music/user.mp3"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	current := catalog.Episode{FeedID: 1, GUID: "one", Title: "One", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	if err := Episodes(context.Background(), server.Client(), filepath.Join(t.TempDir(), "staging"), root, []catalog.Episode{current}, nil, []string{"Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}); err != nil {
		t.Fatalf("Episodes() returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3")); !os.IsNotExist(err) {
		t.Fatalf("old managed file remains, error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Music/user.mp3")); err != nil {
		t.Fatalf("unmanaged file was removed: %v", err)
	}
}

func TestEpisodesDoesNotChangeDeviceWhenDownloadFails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	root := t.TempDir()
	old := filepath.Join(root, "Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	current := catalog.Episode{FeedID: 1, GUID: "one", Enclosure: catalog.Enclosure{URL: server.URL}}
	if err := Episodes(context.Background(), server.Client(), t.TempDir(), root, []catalog.Episode{current}, nil, []string{"Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}); err == nil {
		t.Fatal("Episodes() accepted a failed download")
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("device changed after failed download: %v", err)
	}
}

func TestEpisodesSkipsUnsupportedMediaAndRemovesItFromPlaylists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/video" {
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	resolver := media.Resolver{1: "podcast-1"}
	video := catalog.Episode{FeedID: 1, GUID: "video", Title: "Video", Enclosure: catalog.Enclosure{URL: server.URL + "/video", Type: "audio/mpeg"}}
	audio := catalog.Episode{FeedID: 1, GUID: "audio", Title: "Audio", Enclosure: catalog.Enclosure{URL: server.URL + "/audio", Type: "audio/mpeg"}}
	videoPath := resolver.RelativePathFor(video)
	audioPath := resolver.RelativePathFor(audio)
	playlist := devicefs.PlaylistFile{Relative: "Playlists/test.m3u8", Content: []byte("#EXTM3U\n#EXTINF:0,Video\n../" + videoPath + "\n#EXTINF:0,Audio\n../" + audioPath + "\n")}
	if err := Episodes(context.Background(), server.Client(), t.TempDir(), root, []catalog.Episode{video, audio}, []devicefs.PlaylistFile{playlist}, nil); err != nil {
		t.Fatalf("Episodes() returned error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "Playlists/test.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), videoPath) || !strings.Contains(string(content), audioPath) {
		t.Fatalf("playlist = %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(videoPath))); !os.IsNotExist(err) {
		t.Fatalf("unsupported media was copied, error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(audioPath))); err != nil {
		t.Fatalf("audio was not copied: %v", err)
	}
}

func TestEpisodesReusesExistingMatchingFile(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	current := catalog.Episode{FeedID: 1, GUID: "one", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg", Length: 5}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	var progress struct {
		completed int
		total     int
		reused    bool
	}
	if err := EpisodesWithProgress(context.Background(), server.Client(), t.TempDir(), root, []catalog.Episode{current}, nil, nil, nil, func(completed, total int, current catalog.Episode, reused bool) {
		progress.completed = completed
		progress.total = total
		progress.reused = reused
	}, nil); err != nil {
		t.Fatalf("Episodes() returned error: %v", err)
	}
	if progress.completed != 1 || progress.total != 1 || !progress.reused {
		t.Fatalf("got progress %+v", progress)
	}
	if requests.Load() != 0 {
		t.Fatalf("downloaded existing file %d times", requests.Load())
	}
}

func TestEpisodesRetagsExistingMP3WithWrongKnownSize(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	current := catalog.Episode{FeedID: 1, GUID: "one", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg", Length: 5}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), t.TempDir(), root, []catalog.Episode{current}, nil, nil, nil, media.Resolver{1: "podcast-1"}, EpisodeSyncOptions{VerifyMedia: true}, nil, nil, nil); err != nil {
		t.Fatalf("Episodes() returned error: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("downloaded existing file %d times", requests.Load())
	}
	if info, err := os.Stat(destination); err != nil || info.Size() <= int64(len("old")) {
		t.Fatalf("existing file was not retagged, info %v, error %v", info, err)
	}
}

func TestEpisodesFastPathDoesNotInspectExistingMP3(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("unexpected download"))
	}))
	defer server.Close()

	root := t.TempDir()
	current := catalog.Episode{FeedID: 1, GUID: "one", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("existing audio without tags"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Episodes(context.Background(), server.Client(), t.TempDir(), root, []catalog.Episode{current}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatalf("downloaded existing file %d times", requests.Load())
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "existing audio without tags" {
		t.Fatalf("existing file changed to %q, error %v", data, err)
	}
}

func TestEpisodesRedownloadsUnreadableExistingMP3(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("replacement audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	current := catalog.Episode{FeedID: 1, GUID: "one", Title: "One", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("unreadable"), 0o000); err != nil {
		t.Fatal(err)
	}

	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), t.TempDir(), root, []catalog.Episode{current}, nil, nil, nil, media.Resolver{1: "podcast-1"}, EpisodeSyncOptions{VerifyMedia: true}, nil, nil, nil); err != nil {
		t.Fatalf("Episodes() returned error: %v", err)
	}
	_ = os.Chmod(destination, 0o644)

	if requests.Load() != 1 {
		t.Fatalf("downloaded replacement %d times", requests.Load())
	}
	data, err := os.ReadFile(destination)
	if err != nil || len(data) <= len("replacement audio") {
		t.Fatalf("replacement was not installed, size=%d, error=%v", len(data), err)
	}
}

func TestEpisodesKeepsUnreadableExistingMP3WhenRedownloadFails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	root := t.TempDir()
	current := catalog.Episode{FeedID: 1, GUID: "one", Title: "One", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("unreadable"), 0o000); err != nil {
		t.Fatal(err)
	}

	warnings := 0
	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), t.TempDir(), root, []catalog.Episode{current}, nil, []string{relative}, nil, media.Resolver{1: "podcast-1"}, EpisodeSyncOptions{VerifyMedia: true}, nil, nil, func(string) {
		warnings++
	}); err != nil {
		t.Fatalf("Episodes() returned error: %v", err)
	}
	_ = os.Chmod(destination, 0o644)

	if warnings != 2 {
		t.Fatalf("got %d warnings, want 2", warnings)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "unreadable" {
		t.Fatalf("existing file changed to %q, error %v", data, err)
	}
}

func TestRetryExistingReadRetriesBeforeFailing(t *testing.T) {
	attempts := 0
	err := retryExistingRead(context.Background(), func() error {
		attempts++
		if attempts < existingReadAttempts {
			return os.ErrInvalid
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryExistingRead() returned error: %v", err)
	}
	if attempts != existingReadAttempts {
		t.Fatalf("got %d attempts, want %d", attempts, existingReadAttempts)
	}
}
