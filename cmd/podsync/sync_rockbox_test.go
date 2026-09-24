package main

import (
	"os"
	"testing"

	devicefs "github.com/HansMontana/podsync/internal/adapters/devicefs"
)

func TestMaybeRequestRockboxTagCacheUpdateRequiresOptInAndMediaChange(t *testing.T) {
	root := t.TempDir()
	layout := devicefs.Layout{Root: root}
	if err := os.Mkdir(layout.TagCacheDirectory(), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := maybeRequestRockboxTagCacheUpdate(layout, false, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.TagCacheUpdateMarkerPath()); !os.IsNotExist(err) {
		t.Fatalf("marker created without opt-in, error: %v", err)
	}
	if err := maybeRequestRockboxTagCacheUpdate(layout, true, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.TagCacheUpdateMarkerPath()); !os.IsNotExist(err) {
		t.Fatalf("marker created without media change, error: %v", err)
	}
	if err := maybeRequestRockboxTagCacheUpdate(layout, true, true, false); err != nil {
		t.Fatalf("enabled media change returned error: %v", err)
	}
	if _, err := os.Stat(layout.TagCacheUpdateMarkerPath()); err != nil {
		t.Fatalf("marker was not created: %v", err)
	}
}

func TestMaybeRequestRockboxTagCacheUpdateRetriesPendingRequest(t *testing.T) {
	layout := devicefs.Layout{Root: t.TempDir()}
	if err := layout.SavePendingTagCacheUpdate(); err != nil {
		t.Fatal(err)
	}
	if err := maybeRequestRockboxTagCacheUpdate(layout, true, false, true); err == nil {
		t.Fatal("request succeeded without a Rockbox directory")
	}
	pending, err := layout.LoadPendingTagCacheUpdate()
	if err != nil || !pending {
		t.Fatalf("pending=%v after failed request, error=%v", pending, err)
	}
	if err := os.Mkdir(layout.TagCacheDirectory(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := maybeRequestRockboxTagCacheUpdate(layout, true, false, true); err != nil {
		t.Fatalf("retry returned error: %v", err)
	}
	pending, err = layout.LoadPendingTagCacheUpdate()
	if err != nil || pending {
		t.Fatalf("pending=%v after successful retry, error=%v", pending, err)
	}
}
