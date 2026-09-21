package playback

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ParseLog reads Rockbox playback.log entries. Each data line has the form
// timestamp:elapsed:length:path; comment and malformed lines are ignored.
func ParseLog(reader io.Reader) ([]Record, error) {
	byPath := make(map[string]Record)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) != 4 {
			continue
		}
		timestamp, timestampErr := strconv.ParseInt(parts[0], 10, 64)
		elapsed, elapsedErr := strconv.ParseInt(parts[1], 10, 64)
		length, lengthErr := strconv.ParseInt(parts[2], 10, 64)
		minimumPlayed := length - length/10
		if timestampErr != nil || elapsedErr != nil || lengthErr != nil || timestamp <= 0 || elapsed <= 0 || length <= 0 || elapsed > length || strings.TrimSpace(parts[3]) == "" || elapsed < minimumPlayed {
			continue
		}
		key := normalizePath(parts[3])
		if key == "" {
			continue
		}
		current := byPath[key]
		current.Path = parts[3]
		current.PlayCount++
		playedAt := time.Unix(timestamp, 0).UTC()
		if playedAt.After(current.LastPlayed) {
			current.LastPlayed = playedAt
		}
		current.Known = true
		byPath[key] = current
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Rockbox playback log: %w", err)
	}

	result := make([]Record, 0, len(byPath))
	for _, record := range byPath {
		result = append(result, record)
	}
	return result, nil
}
