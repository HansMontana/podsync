package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/HansMontana/podsync/internal/episode"
)

func TestEpisodesStagesBeforeApplyingAndDeletesOnlyManagedFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Podcasts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Podcasts/old.mp3"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Music"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Music/user.mp3"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	current := episode.Episode{FeedID: 1, GUID: "one", Title: "One", Enclosure: episode.Enclosure{URL: server.URL, Type: "audio/mpeg"}}
	if err := Episodes(context.Background(), server.Client(), filepath.Join(t.TempDir(), "staging"), root, []episode.Episode{current}, nil, []string{"Podcasts/old.mp3"}); err != nil {
		t.Fatalf("Episodes() returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Podcasts/old.mp3")); !os.IsNotExist(err) {
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
	old := filepath.Join(root, "Podcasts/old.mp3")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	current := episode.Episode{FeedID: 1, GUID: "one", Enclosure: episode.Enclosure{URL: server.URL}}
	if err := Episodes(context.Background(), server.Client(), t.TempDir(), root, []episode.Episode{current}, nil, []string{"Podcasts/old.mp3"}); err == nil {
		t.Fatal("Episodes() accepted a failed download")
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("device changed after failed download: %v", err)
	}
}
