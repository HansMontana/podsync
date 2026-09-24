package device

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	result, err := EpisodesWithResolverAndProgressAndWarningsWithOptionsResult(
		context.Background(), server.Client(), t.TempDir(), root,
		[]catalog.Episode{{FeedID: 1, GUID: "episode", Enclosure: catalog.Enclosure{URL: server.URL, Type: "audio/ogg"}}},
		nil, nil, nil, resolver,
		EpisodeSyncOptions{MediaOps: realMediaOperations(), Files: realDeviceFiles()},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("media sync returned error: %v", err)
	}
	if !result.MediaChanged {
		t.Fatal("media sync did not report a media change")
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
