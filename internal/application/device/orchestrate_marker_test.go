package device

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/HansMontana/podsync/internal/adapters/devicefs"
	"github.com/HansMontana/podsync/internal/adapters/media"
	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func TestEpisodeSyncResultReportsMediaButNotPlaylistChanges(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	resolver := media.Resolver{1: "podcast"}
	root := t.TempDir()
	layout := devicefs.Layout{Root: root}
	called := false
	result, err := EpisodesWithResolverAndProgressAndWarningsWithOptionsResult(
		context.Background(), server.Client(), t.TempDir(), root,
		[]catalog.Episode{{FeedID: 1, GUID: "episode", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/ogg"}}},
		nil, nil, nil, resolver,
		EpisodeSyncOptions{
			MediaOps: realMediaOperations(), Files: realDeviceFiles(),
			BeforeMediaMutation: func() error {
				called = true
				if err := layout.SavePendingTagCacheUpdate(); err != nil {
					return err
				}
				if _, err := os.Stat(layout.PendingTagCacheUpdatePath()); err != nil {
					return err
				}
				return nil
			},
		},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("media sync returned error: %v", err)
	}
	if !result.MediaChanged {
		t.Fatal("media sync did not report a media change")
	}
	if !called {
		t.Fatal("media mutation callback was not called")
	}

	playlistResult, err := EpisodesWithResolverAndProgressAndWarningsWithOptionsResult(
		context.Background(), server.Client(), t.TempDir(), t.TempDir(),
		nil, []PlaylistFile{{Relative: "Playlists/current.m3u8", Content: []byte("#EXTM3U\n")}}, nil, nil, resolver,
		EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("playlist sync returned error: %v", err)
	}
	if playlistResult.MediaChanged {
		t.Fatal("playlist-only sync reported a media change")
	}
}

func TestChunkedSyncPersistsTagCacheIntentBeforeMediaApply(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	layout := devicefs.Layout{Root: root}
	staging := t.TempDir()
	resolver := media.Resolver{1: "podcast"}
	episodes := make([]catalog.Episode, chunkEpisodeLimit+1)
	for i := range episodes {
		episodes[i] = catalog.Episode{
			FeedID: 1, GUID: fmt.Sprintf("episode-%d", i),
			Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/ogg"},
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := EpisodesWithResolverAndProgressAndWarningsWithOptions(
		ctx, server.Client(), staging, root, episodes, nil, nil, nil, resolver,
		EpisodeSyncOptions{
			MediaOps: realMediaOperations(), Files: realDeviceFiles(),
			BeforeMediaMutation: layout.SavePendingTagCacheUpdate,
		}, nil,
		func(progress FileProgress) {
			if progress.Phase == "copy" && progress.Completed == chunkEpisodeLimit {
				cancel()
			}
		}, nil,
	)
	if err == nil {
		t.Fatal("interrupted chunked sync succeeded")
	}
	pending, pendingErr := layout.LoadPendingTagCacheUpdate()
	if pendingErr != nil || !pending {
		t.Fatalf("pending=%v after interrupted media apply, error=%v", pending, pendingErr)
	}
}
