package device

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HansMontana/podsync/internal/adapters/devicefs"
	"github.com/HansMontana/podsync/internal/adapters/media"
	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func TestMissingEpisodeChunksUsesEpisodeFallbackLimit(t *testing.T) {
	episodes := make([]catalog.Episode, chunkEpisodeLimit+1)
	for i := range episodes {
		episodes[i] = catalog.Episode{FeedID: 1, GUID: fmt.Sprintf("episode-%d", i), Enclosure: catalog.Enclosure{URL: "https://example.com/episode.mp3"}}
	}

	chunks, err := missingEpisodeChunks(t.TempDir(), episodes, media.Resolver{1: "podcast"}, map[string]string{}, realDeviceFiles())
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
	episodes := make([]catalog.Episode, chunkEpisodeLimit+1)
	var manifest strings.Builder
	for i := range episodes {
		episodes[i] = catalog.Episode{FeedID: 1, GUID: fmt.Sprintf("episode-%d", i), Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/ogg"}}
		manifest.WriteString(resolver.RelativePathFor(episodes[i]))
		manifest.WriteByte('\n')
	}
	playlists := []PlaylistFile{{Relative: "Podsync/managed-files.txt", Content: []byte(manifest.String())}}

	ctx, cancel := context.WithCancel(context.Background())
	var copied int
	err := EpisodesWithResolverAndProgressAndWarningsWithOptions(ctx, server.Client(), staging, root, episodes, playlists, nil, nil, resolver, EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()}, nil, func(progress FileProgress) {
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
	managed, err := (devicefs.Layout{Root: root}).LoadManagedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), staging, root, resumedEpisodes, resumedPlaylists, managed, nil, resolver, EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()}, nil, nil, nil); err != nil {
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

func TestSingleBatchInterruptionPreservesPendingOwnership(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	staging := t.TempDir()
	resolver := media.Resolver{1: "podcast"}
	episodeValue := catalog.Episode{FeedID: 1, GUID: "episode-1", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/ogg"}}
	relative := resolver.RelativePathFor(episodeValue)
	playlists := []PlaylistFile{{Relative: "Podsync/managed-files.txt", Content: []byte(relative + "\n")}}
	ctx, cancel := context.WithCancel(context.Background())
	err := EpisodesWithResolverAndProgressAndWarningsWithOptions(ctx, server.Client(), staging, root, []catalog.Episode{episodeValue}, playlists, nil, nil, resolver, EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()}, nil, func(progress FileProgress) {
		if progress.Phase == "copy" {
			cancel()
		}
	}, nil)
	if err == nil {
		t.Fatal("interrupted sync succeeded")
	}
	pending, err := (devicefs.Layout{Root: root}).LoadPendingManagedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0] != relative {
		t.Fatalf("got pending ownership %v, want %q", pending, relative)
	}

	secondEpisode := catalog.Episode{FeedID: 1, GUID: "episode-2", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/ogg"}}
	secondRelative := resolver.RelativePathFor(secondEpisode)
	secondPlaylists := []PlaylistFile{{Relative: "Podsync/managed-files.txt", Content: []byte(secondRelative + "\n")}}
	secondContext, secondCancel := context.WithCancel(context.Background())
	secondErr := EpisodesWithResolverAndProgressAndWarningsWithOptions(secondContext, server.Client(), staging, root, []catalog.Episode{secondEpisode}, secondPlaylists, nil, nil, resolver, EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()}, nil, func(progress FileProgress) {
		if progress.Phase == "copy" {
			secondCancel()
		}
	}, nil)
	if secondErr == nil {
		t.Fatal("second interrupted sync succeeded")
	}
	pending, err = (devicefs.Layout{Root: root}).LoadPendingManagedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0] != relative || pending[1] != secondRelative {
		t.Fatalf("got pending ownership after second interruption %v, want both paths", pending)
	}
	if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), staging, root, []catalog.Episode{episodeValue}, playlists, pending, nil, resolver, EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()}, nil, nil, nil); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	pending, err = (devicefs.Layout{Root: root}).LoadPendingManagedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending ownership remains after resume: %v", pending)
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
			episodes := make([]catalog.Episode, chunkEpisodeLimit+1)
			var manifest strings.Builder
			for i := range episodes {
				episodes[i] = catalog.Episode{FeedID: 1, GUID: fmt.Sprintf("episode-%d", i), Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/ogg"}}
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
			err := EpisodesWithResolverAndProgressAndWarningsWithOptions(ctx, server.Client(), staging, root, episodes, playlists, initialManaged, nil, resolver, EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()}, nil, func(progress FileProgress) {
				if progress.Phase == test.phase {
					cancel()
				}
			}, nil)
			if err == nil {
				t.Fatal("interrupted finalization succeeded")
			}
			pending, err := (devicefs.Layout{Root: root}).LoadPendingManagedPaths()
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) == 0 {
				t.Fatal("interrupted finalization lost pending ownership")
			}

			managed, err := (devicefs.Layout{Root: root}).LoadManagedPaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := EpisodesWithResolverAndProgressAndWarningsWithOptions(context.Background(), server.Client(), staging, root, episodes, playlists, managed, nil, resolver, EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()}, nil, nil, nil); err != nil {
				t.Fatalf("resume failed: %v", err)
			}
			pending, err = (devicefs.Layout{Root: root}).LoadPendingManagedPaths()
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
