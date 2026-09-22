package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/HansMontana/podsync/internal/episode"
)

// Resolver maps durable source-feed IDs to logical-feed IDs for device paths.
type Resolver map[int64]string

// RelativePathFor returns the logical-feed path for an episode.
func (r Resolver) RelativePathFor(e episode.Episode) string {
	logicalID := r[e.FeedID]

	hash := IdentityHash(e)
	title := sanitizeTitle(e.Title)
	date := "unknown-date"
	if !e.PublishedAt.IsZero() {
		date = e.PublishedAt.UTC().Format("2006-01-02")
	}
	name := fmt.Sprintf("%s - %s -- %s%s", title, date, hash, extension(e))
	return path.Join("Podcasts", logicalID, name)
}

// IdentityHash returns the short identity suffix used in managed filenames.
func IdentityHash(e episode.Episode) string {
	sum := sha256.Sum256([]byte(e.IdentityKey()))
	return hex.EncodeToString(sum[:])[:12]
}

func sanitizeTitle(value string) string {
	var result strings.Builder
	for _, character := range strings.TrimSpace(value) {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || strings.ContainsRune(" .,_-'()&!", character) {
			result.WriteRune(character)
			continue
		}
		result.WriteRune(' ')
	}
	clean := strings.Join(strings.Fields(result.String()), " ")
	clean = strings.Trim(clean, " .-_'")
	if clean == "" {
		return "episode"
	}
	const maxTitleRunes = 120
	runes := []rune(clean)
	if len(runes) > maxTitleRunes {
		clean = strings.TrimSpace(string(runes[:maxTitleRunes]))
	}
	return clean
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
