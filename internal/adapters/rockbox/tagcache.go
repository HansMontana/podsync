package rockbox

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	tagCacheMagic        uint32 = 0x54434810
	tagCacheTagCount            = 23
	tagCacheMasterHeader        = 24
	tagFilename                 = 4
	tagPlayCount                = 15
	tagLastPlayed               = 18
	tagCacheDeleted      uint32 = 0x0001
	maxTagCacheEntries          = 1_000_000
	maxTagCacheFileSize  int64  = 256 << 20
)

// ParseTagCache reads the Rockbox TagCache master and filename index files.
// It imports only playback fields and leaves all episode identity decisions to
// ForEpisodes.
func ParseTagCache(directory string) ([]Record, error) {
	masterPath := filepath.Join(directory, "database_idx.tcd")
	filenamePath := filepath.Join(directory, fmt.Sprintf("database_%d.tcd", tagFilename))

	master, order, entrySize, entryCount, err := openMaster(masterPath)
	if err != nil {
		return nil, err
	}
	defer master.Close()
	filename, err := openRegularFile(filenamePath, "TagCache filename index")
	if err != nil {
		return nil, fmt.Errorf("open TagCache filename index: %w", err)
	}
	defer filename.Close()

	filenameHeader, err := readTagHeader(filename, order)
	if err != nil {
		return nil, fmt.Errorf("read TagCache filename header: %w", err)
	}
	if filenameHeader.count != entryCount {
		return nil, fmt.Errorf("TagCache entry counts do not match")
	}
	var records []Record
	for range entryCount {
		var header [8]byte
		if _, err := io.ReadFull(filename, header[:]); err != nil {
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
		terminator := bytes.IndexByte(data, 0)
		if terminator < 0 {
			return nil, fmt.Errorf("TagCache filename has no terminator")
		}
		path := string(data[:terminator])
		if path == "" || indexID < 0 {
			continue
		}

		if indexID >= entryCount {
			return nil, fmt.Errorf("TagCache index ID out of range: %d", indexID)
		}
		entry, err := readMasterEntry(master, order, entrySize, indexID)
		if err != nil {
			return nil, err
		}
		if entry.flag&tagCacheDeleted != 0 {
			continue
		}
		playCount := entry.values[tagPlayCount]
		if playCount < 0 {
			return nil, fmt.Errorf("invalid TagCache play count %d", playCount)
		}
		records = append(records, Record{
			Path:      path,
			Known:     true,
			PlayCount: int(playCount),
			// TagCache lastplayed is an internal ordinal, not a wall-clock time.
		})
	}
	return MergeRecords(records), nil
}

type tagCacheEntry struct {
	values [tagCacheTagCount]int32
	flag   uint32
}

type tagHeader struct{ count int64 }

func openMaster(path string) (*os.File, binary.ByteOrder, int64, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("open TagCache master index: %w", err)
	}
	var header [tagCacheMasterHeader]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		file.Close()
		return nil, nil, 0, 0, fmt.Errorf("read TagCache master header: %w", err)
	}
	var order binary.ByteOrder = binary.LittleEndian
	if order.Uint32(header[0:4]) != tagCacheMagic {
		order = binary.BigEndian
		if order.Uint32(header[0:4]) != tagCacheMagic {
			file.Close()
			return nil, nil, 0, 0, fmt.Errorf("invalid TagCache magic")
		}
	}
	count := int64(order.Uint32(header[8:12]))
	if count > maxTagCacheEntries {
		file.Close()
		return nil, nil, 0, 0, fmt.Errorf("TagCache master index exceeds %d entries", maxTagCacheEntries)
	}
	dirty := order.Uint32(header[20:24])
	if dirty != 0 {
		file.Close()
		return nil, nil, 0, 0, fmt.Errorf("TagCache master index is dirty")
	}
	entrySize := int64(tagCacheTagCount*4 + 4)
	info, err := file.Stat()
	if err != nil || info.Size() > maxTagCacheFileSize || info.Size() < tagCacheMasterHeader+count*entrySize {
		file.Close()
		return nil, nil, 0, 0, fmt.Errorf("truncated TagCache master index")
	}
	return file, order, entrySize, count, nil
}

func readTagHeader(file *os.File, order binary.ByteOrder) (tagHeader, error) {
	var header [12]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return tagHeader{}, err
	}
	if order.Uint32(header[0:4]) != tagCacheMagic {
		return tagHeader{}, fmt.Errorf("invalid magic")
	}
	info, err := file.Stat()
	if err != nil {
		return tagHeader{}, err
	}
	if info.Size() > maxTagCacheFileSize {
		return tagHeader{}, fmt.Errorf("TagCache filename index exceeds %d bytes", maxTagCacheFileSize)
	}
	if int64(order.Uint32(header[4:8])) != info.Size()-12 {
		return tagHeader{}, fmt.Errorf("invalid data size")
	}
	return tagHeader{count: int64(order.Uint32(header[8:12]))}, nil
}

func openRegularFile(path, description string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", description, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", description)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", description, err)
	}
	return file, nil
}

func readMasterEntry(file *os.File, order binary.ByteOrder, entrySize, indexID int64) (tagCacheEntry, error) {
	if indexID > (1<<31)/entrySize {
		return tagCacheEntry{}, fmt.Errorf("TagCache index ID out of range: %d", indexID)
	}
	if _, err := file.Seek(tagCacheMasterHeader+indexID*entrySize, io.SeekStart); err != nil {
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
