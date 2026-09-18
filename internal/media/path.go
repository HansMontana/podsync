package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/HansMontana/podsync/internal/episode"
)

// RelativePath returns a stable, device-relative path for an episode.
func RelativePath(e episode.Episode) string {
	sum := sha256.Sum256([]byte(e.IdentityKey()))
	name := hex.EncodeToString(sum[:])[:16] + extension(e)
	return path.Join("Podcasts", fmt.Sprintf("feed-%d", e.FeedID), name)
}

func extension(e episode.Episode) string {
	switch strings.ToLower(strings.TrimSpace(e.Enclosure.Type)) {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a", "audio/m4a":
		return ".m4a"
	case "audio/ogg":
		return ".ogg"
	case "audio/opus":
		return ".opus"
	case "audio/flac":
		return ".flac"
	}

	parsed, err := url.Parse(e.Enclosure.URL)
	if err == nil {
		if ext := path.Ext(parsed.Path); ext != "" && len(ext) <= 8 {
			return strings.ToLower(ext)
		}
	}
	return ".audio"
}
