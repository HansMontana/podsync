package playlists

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
)

type Track struct {
	Episode episode.Episode
	Path    string
}

// M3U returns a Rockbox-compatible extended M3U playlist.
func M3U(tracks []Track) []byte {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")
	for _, track := range tracks {
		builder.WriteString("#EXTINF:")
		builder.WriteString(strconv.FormatInt(int64(track.Episode.Duration/time.Second), 10))
		builder.WriteString(",")
		builder.WriteString(strings.ReplaceAll(track.Episode.Title, "\n", " "))
		builder.WriteString("\n")
		builder.WriteString(track.Path)
		builder.WriteString("\n")
	}
	return []byte(builder.String())
}

func ValidateTracks(tracks []Track) error {
	for i, track := range tracks {
		if strings.TrimSpace(track.Path) == "" {
			return fmt.Errorf("track %d has an empty path", i)
		}
	}
	return nil
}
