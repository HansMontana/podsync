package playback

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	tagCacheMagic    uint32 = 0x54434810
	tagCacheTagCount        = 23
	tagFilename             = 4
	tagPlayCount            = 15
	tagLastPlayed           = 18
	tagCacheDeleted  uint32 = 0x0001
)

// ParseTagCache reads the Rockbox TagCache master and filename index files.
// It imports only playback fields and leaves all episode identity decisions to
// ForEpisodes.
func ParseTagCache(directory string) ([]Record, error) {
	masterPath := filepath.Join(directory, "database_idx.tcd")
	filenamePath := filepath.Join(directory, fmt.Sprintf("database_%d.tcd", tagFilename))

	master, order, entrySize, err := openMaster(masterPath)
	if err != nil {
		return nil, err
	}
	defer master.Close()
	filename, err := os.Open(filenamePath)
	if err != nil {
		return nil, fmt.Errorf("open TagCache filename index: %w", err)
	}
	defer filename.Close()

	if _, err := filename.Seek(12, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek TagCache filename index: %w", err)
	}
	var records []Record
	for {
		var header [8]byte
		if _, err := io.ReadFull(filename, header[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return nil, fmt.Errorf("read TagCache filename entry: %w", err)
		}
		length := int64(order.Uint32(header[0:4]))
		indexID := int64(int32(order.Uint32(header[4:8])))
		if length <= 0 || length > 2<<20 {
			return nil, fmt.Errorf("invalid TagCache filename length %d", length)
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(filename, data); err != nil {
			return nil, fmt.Errorf("read TagCache filename: %w", err)
		}
		path := strings.TrimRight(string(data), "\x00")
		if path == "" || indexID < 0 {
			continue
		}

		entry, err := readMasterEntry(master, order, entrySize, indexID)
		if err != nil {
			return nil, err
		}
		if entry.flag&tagCacheDeleted != 0 {
			continue
		}
		records = append(records, Record{
			Path:       path,
			Known:      true,
			PlayCount:  int(entry.values[tagPlayCount]),
			LastPlayed: time.Unix(int64(entry.values[tagLastPlayed]), 0).UTC(),
		})
	}
	return MergeRecords(records), nil
}

type tagCacheEntry struct {
	values [tagCacheTagCount]int32
	flag   uint32
}

func openMaster(path string) (*os.File, binary.ByteOrder, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("open TagCache master index: %w", err)
	}
	var header [12]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		file.Close()
		return nil, nil, 0, fmt.Errorf("read TagCache master header: %w", err)
	}
	var order binary.ByteOrder = binary.LittleEndian
	if order.Uint32(header[0:4]) != tagCacheMagic {
		order = binary.BigEndian
		if order.Uint32(header[0:4]) != tagCacheMagic {
			file.Close()
			return nil, nil, 0, fmt.Errorf("invalid TagCache magic")
		}
	}
	entrySize := int64(tagCacheTagCount*4 + 4)
	return file, order, entrySize, nil
}

func readMasterEntry(file *os.File, order binary.ByteOrder, entrySize, indexID int64) (tagCacheEntry, error) {
	if indexID > (1<<31)/entrySize {
		return tagCacheEntry{}, fmt.Errorf("TagCache index ID out of range: %d", indexID)
	}
	if _, err := file.Seek(20+indexID*entrySize, io.SeekStart); err != nil {
		return tagCacheEntry{}, fmt.Errorf("seek TagCache master entry: %w", err)
	}
	data := make([]byte, entrySize)
	if _, err := io.ReadFull(file, data); err != nil {
		return tagCacheEntry{}, fmt.Errorf("read TagCache master entry: %w", err)
	}
	var entry tagCacheEntry
	for i := range entry.values {
		entry.values[i] = int32(order.Uint32(data[i*4:]))
	}
	entry.flag = order.Uint32(data[tagCacheTagCount*4:])
	return entry, nil
}
