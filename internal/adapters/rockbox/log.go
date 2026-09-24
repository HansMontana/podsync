package rockbox

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	maxLogLine    = 1 << 20
	maxLogSize    = 64 << 20
	maxLogRecords = 100_000
)

// ParseLog reads Rockbox playback.log entries. Each data line has the form
// timestamp:elapsed:length:path; comment and malformed lines are ignored.
func ParseLog(reader io.Reader) ([]Record, error) {
	byPath := make(map[string]Record)
	counting := &countingReader{reader: reader}
	buffered := bufio.NewReaderSize(io.LimitReader(counting, maxLogSize+1), 64*1024)
	var line []byte
	overlong := false
	for {
		fragment, err := buffered.ReadSlice('\n')
		if overlong {
			if err == nil {
				overlong = false
			}
			if err == io.EOF {
				break
			}
			if err != nil && err != bufio.ErrBufferFull {
				return nil, fmt.Errorf("read Rockbox playback log: %w", err)
			}
			continue
		}
		if len(line)+len(fragment) > maxLogLine {
			line = nil
			overlong = true
			if err == nil {
				overlong = false
			}
		} else {
			line = append(line, fragment...)
		}
		if err == nil {
			parseLogLine(byPath, line)
			if len(byPath) > maxLogRecords {
				return nil, fmt.Errorf("Rockbox playback log exceeds %d records", maxLogRecords)
			}
			line = nil
			continue
		}
		if err == io.EOF {
			if len(line) > 0 {
				parseLogLine(byPath, line)
			}
			if len(byPath) > maxLogRecords {
				return nil, fmt.Errorf("Rockbox playback log exceeds %d records", maxLogRecords)
			}
			if counting.bytesRead > maxLogSize {
				return nil, fmt.Errorf("Rockbox playback log exceeds %d bytes", maxLogSize)
			}
			break
		}
		if err != bufio.ErrBufferFull {
			return nil, fmt.Errorf("read Rockbox playback log: %w", err)
		}
	}

	result := make([]Record, 0, len(byPath))
	for _, record := range byPath {
		result = append(result, record)
	}
	return result, nil
}

type countingReader struct {
	reader    io.Reader
	bytesRead int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	count, err := r.reader.Read(p)
	r.bytesRead += int64(count)
	return count, err
}

func parseLogLine(byPath map[string]Record, raw []byte) {
	line := strings.TrimSpace(string(raw))
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	parts := strings.SplitN(line, ":", 4)
	if len(parts) != 4 {
		return
	}
	timestamp, timestampErr := strconv.ParseInt(parts[0], 10, 64)
	elapsed, elapsedErr := strconv.ParseInt(parts[1], 10, 64)
	length, lengthErr := strconv.ParseInt(parts[2], 10, 64)
	if timestampErr != nil || elapsedErr != nil || lengthErr != nil || timestamp <= 0 || elapsed <= 0 || length <= 0 || elapsed > length || strings.TrimSpace(parts[3]) == "" {
		return
	}
	key := normalizePath(parts[3])
	if key == "" {
		return
	}
	current := byPath[key]
	current.Path = parts[3]
	if elapsed >= length-length/10 {
		current.PlayCount++
	} else {
		current.Skipped = true
	}
	playedAt := time.Unix(timestamp, 0).UTC()
	if playedAt.After(current.LastPlayed) {
		current.LastPlayed = playedAt
	}
	current.Known = true
	byPath[key] = current
}
