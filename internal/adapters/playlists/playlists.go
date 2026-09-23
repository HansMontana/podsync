package playlists

import (
	"strconv"
	"strings"
	"time"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

type Track struct {
	Episode catalog.Episode
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
