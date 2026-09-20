package sync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/media"
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

	current := episode.Episode{FeedID: 1, GUID: "one", Title: "One", Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	if err := Episodes(context.Background(), server.Client(), filepath.Join(t.TempDir(), "staging"), root, []episode.Episode{current}, nil, []string{"Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}); err != nil {
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

	current := episode.Episode{FeedID: 1, GUID: "one", Enclosure: episode.Enclosure{URL: server.URL}}
	if err := Episodes(context.Background(), server.Client(), t.TempDir(), root, []episode.Episode{current}, nil, []string{"Podcasts/podcast-1/aaaaaaaaaaaaaaaa.mp3"}); err == nil {
		t.Fatal("Episodes() accepted a failed download")
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("device changed after failed download: %v", err)
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
	current := episode.Episode{FeedID: 1, GUID: "one", Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg", Length: 5}}
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
	if err := EpisodesWithProgress(context.Background(), server.Client(), t.TempDir(), root, []episode.Episode{current}, nil, nil, nil, func(completed, total int, current episode.Episode, reused bool) {
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
	current := episode.Episode{FeedID: 1, GUID: "one", Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg", Length: 5}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), t.TempDir(), root, []episode.Episode{current}, nil, nil, nil, media.Resolver{1: "podcast-1"}, EpisodeSyncOptions{VerifyMedia: true}, nil, nil, nil); err != nil {
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
	current := episode.Episode{FeedID: 1, GUID: "one", Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("existing audio without tags"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Episodes(context.Background(), server.Client(), t.TempDir(), root, []episode.Episode{current}, nil, nil); err != nil {
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
	current := episode.Episode{FeedID: 1, GUID: "one", Title: "One", Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("unreadable"), 0o000); err != nil {
		t.Fatal(err)
	}

	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), t.TempDir(), root, []episode.Episode{current}, nil, nil, nil, media.Resolver{1: "podcast-1"}, EpisodeSyncOptions{VerifyMedia: true}, nil, nil, nil); err != nil {
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
	current := episode.Episode{FeedID: 1, GUID: "one", Title: "One", Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	relative := (media.Resolver{1: "podcast-1"}).RelativePathFor(current)
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("unreadable"), 0o000); err != nil {
		t.Fatal(err)
	}

	warnings := 0
	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), t.TempDir(), root, []episode.Episode{current}, nil, []string{relative}, nil, media.Resolver{1: "podcast-1"}, EpisodeSyncOptions{VerifyMedia: true}, nil, nil, func(string) {
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

func TestMissingEpisodeChunksUsesEpisodeFallbackLimit(t *testing.T) {
	episodes := make([]episode.Episode, chunkEpisodeLimit+1)
	for i := range episodes {
		episodes[i] = episode.Episode{FeedID: 1, GUID: fmt.Sprintf("episode-%d", i), Enclosure: episode.Enclosure{URL: "https://example.com/episode.mp3"}}
	}

	chunks, err := missingEpisodeChunks(t.TempDir(), episodes, media.Resolver{1: "podcast"})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || len(chunks[0]) != chunkEpisodeLimit || len(chunks[1]) != 1 {
		t.Fatalf("got chunk sizes %v", []int{len(chunks[0]), len(chunks[1])})
	}
}

func TestChunkedSyncResumesBeforeFinalization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	staging := t.TempDir()
	resolver := media.Resolver{1: "podcast"}
	episodes := make([]episode.Episode, chunkEpisodeLimit+1)
	var manifest strings.Builder
	for i := range episodes {
		episodes[i] = episode.Episode{FeedID: 1, GUID: fmt.Sprintf("episode-%d", i), Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/ogg"}}
		manifest.WriteString(resolver.RelativePathFor(episodes[i]))
		manifest.WriteByte('\n')
	}
	playlists := []PlaylistFile{{Relative: "Podsync/managed-files.txt", Content: []byte(manifest.String())}}

	ctx, cancel := context.WithCancel(context.Background())
	var copied int
	err := EpisodesWithResolverAndProgressAndWarningsWithOptions(ctx, server.Client(), staging, root, episodes, playlists, nil, nil, resolver, EpisodeSyncOptions{}, nil, func(progress FileProgress) {
		if progress.Phase == "copy" {
			copied++
			if copied == chunkEpisodeLimit {
				cancel()
			}
		}
	}, nil)
	if err == nil {
		t.Fatal("interrupted sync succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, "Podsync/managed-files.txt")); !os.IsNotExist(err) {
		t.Fatalf("interrupted sync committed manifest, error: %v", err)
	}

	resumedEpisodes := episodes[:1]
	resumedManifest := resolver.RelativePathFor(resumedEpisodes[0]) + "\n"
	resumedPlaylists := []PlaylistFile{{Relative: "Podsync/managed-files.txt", Content: []byte(resumedManifest)}}
	managed, err := (device.Layout{Root: root}).LoadManagedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), staging, root, resumedEpisodes, resumedPlaylists, managed, nil, resolver, EpisodeSyncOptions{}, nil, nil, nil); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Podsync/managed-files.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != resumedManifest {
		t.Fatalf("final manifest does not contain the resumed desired set")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(resolver.RelativePathFor(episodes[1])))); !os.IsNotExist(err) {
		t.Fatalf("stale pending episode remains, error: %v", err)
	}
}

func TestChunkedSyncRecoversAfterFinalizationInterruption(t *testing.T) {
	tests := []struct {
		name        string
		phase       string
		playlists   []PlaylistFile
		initialFile string
	}{
		{
			name:  "playlist",
			phase: "playlist",
			playlists: []PlaylistFile{
				{Relative: "Playlists/briefing.m3u8", Content: []byte("podcast episode\n")},
				{Relative: "Podsync/managed-files.txt", Content: nil},
			},
		},
		{
			name:        "deletion",
			phase:       "delete",
			initialFile: "Podcasts/stale/old.mp3\n",
			playlists:   []PlaylistFile{{Relative: "Podsync/managed-files.txt", Content: nil}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("audio"))
			}))
			defer server.Close()

			root := t.TempDir()
			staging := t.TempDir()
			resolver := media.Resolver{1: "podcast"}
			episodes := make([]episode.Episode, chunkEpisodeLimit+1)
			var manifest strings.Builder
			for i := range episodes {
				episodes[i] = episode.Episode{FeedID: 1, GUID: fmt.Sprintf("episode-%d", i), Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/ogg"}}
				manifest.WriteString(resolver.RelativePathFor(episodes[i]))
				manifest.WriteByte('\n')
			}
			playlists := append([]PlaylistFile(nil), test.playlists...)
			playlists[len(playlists)-1].Content = []byte(manifest.String())

			if test.initialFile != "" {
				path := filepath.Join(root, filepath.FromSlash("Podcasts/stale/old.mp3"))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(root, "Podsync"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "Podsync/managed-files.txt"), []byte(test.initialFile), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			initialManaged := []string(nil)
			if test.initialFile != "" {
				initialManaged = []string{"Podcasts/stale/old.mp3"}
			}

			ctx, cancel := context.WithCancel(context.Background())
			err := EpisodesWithResolverAndProgressAndWarningsWithOptions(ctx, server.Client(), staging, root, episodes, playlists, initialManaged, nil, resolver, EpisodeSyncOptions{}, nil, func(progress FileProgress) {
				if progress.Phase == test.phase {
					cancel()
				}
			}, nil)
			if err == nil {
				t.Fatal("interrupted finalization succeeded")
			}
			pending, err := (device.Layout{Root: root}).LoadPendingManagedPaths()
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) == 0 {
				t.Fatal("interrupted finalization lost pending ownership")
			}

			managed, err := (device.Layout{Root: root}).LoadManagedPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), staging, root, episodes, playlists, managed, nil, resolver, EpisodeSyncOptions{}, nil, nil, nil); err != nil {
				t.Fatalf("resume failed: %v", err)
			}
			pending, err = (device.Layout{Root: root}).LoadPendingManagedPaths()
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 0 {
				t.Fatalf("pending ownership remains after resume: %v", pending)
			}
			if test.initialFile != "" {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash("Podcasts/stale/old.mp3"))); !os.IsNotExist(err) {
					t.Fatalf("stale file remains after resume, error: %v", err)
				}
			}
		})
	}
}
