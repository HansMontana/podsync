package playback

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestParseTagCacheLittleEndian(t *testing.T) {
	directory := writeTagCacheFixture(t, binary.LittleEndian, false)
	records, err := ParseTagCache(directory)
	if err != nil {
		t.Fatalf("ParseTagCache() returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got records %+v", records)
	}
	if records[0].Path != "/Podcasts/feed-2/episode.mp3" || records[0].PlayCount != 4 || !records[0].LastPlayed.IsZero() {
		t.Fatalf("got record %+v", records[0])
	}
}

func TestParseTagCacheBigEndian(t *testing.T) {
	directory := writeTagCacheFixture(t, binary.BigEndian, false)
	records, err := ParseTagCache(directory)
	if err != nil {
		t.Fatalf("ParseTagCache() returned error: %v", err)
	}
	if len(records) != 1 || records[0].PlayCount != 4 {
		t.Fatalf("got records %+v", records)
	}
}

func TestParseTagCacheSkipsDeletedEntries(t *testing.T) {
	directory := writeTagCacheFixture(t, binary.LittleEndian, true)
	records, err := ParseTagCache(directory)
	if err != nil {
		t.Fatalf("ParseTagCache() returned error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("got deleted records %+v", records)
	}
}

func writeTagCacheFixture(t *testing.T, order binary.ByteOrder, deleted bool) string {
	t.Helper()
	directory := t.TempDir()
	entrySize := tagCacheTagCount*4 + 4
	master := make([]byte, 20+entrySize)
	order.PutUint32(master[0:4], tagCacheMagic)
	order.PutUint32(master[4:8], uint32(entrySize))
	order.PutUint32(master[8:12], 1)
	base := 20
	order.PutUint32(master[base+tagPlayCount*4:], 4)
	order.PutUint32(master[base+tagLastPlayed*4:], 1700000000)
	if deleted {
		order.PutUint32(master[base+tagCacheTagCount*4:], tagCacheDeleted)
	}
	if err := os.WriteFile(filepath.Join(directory, "database_idx.tcd"), master, 0o644); err != nil {
		t.Fatal(err)
	}

	// Rockbox aligns values and may leave padding after the terminating NUL.
	data := append([]byte("/Podcasts/feed-2/episode.mp3\x00"), 0, 0, 0)
	filename := make([]byte, 12+8+len(data))
	order.PutUint32(filename[0:4], tagCacheMagic)
	order.PutUint32(filename[4:8], uint32(len(filename)-12))
	order.PutUint32(filename[8:12], 1)
	order.PutUint32(filename[12:16], uint32(len(data)))
	order.PutUint32(filename[16:20], 0)
	copy(filename[20:], data)
	if err := os.WriteFile(filepath.Join(directory, "database_4.tcd"), filename, 0o644); err != nil {
		t.Fatal(err)
	}
	return directory
}
