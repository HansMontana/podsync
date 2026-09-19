package playlists

import (
	"strings"
	"unicode"
)

// Filename returns a readable, safe filename for a playlist title.
func Filename(title, fallback string) string {
	if strings.TrimSpace(title) == "" {
		title = fallback
	}

	var result strings.Builder
	for _, character := range strings.TrimSpace(title) {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || character == ' ' || strings.ContainsRune("._-'()&", character) {
			result.WriteRune(character)
			continue
		}
		result.WriteByte(' ')
	}

	name := strings.Trim(strings.Join(strings.Fields(result.String()), " "), " .-_'")
	if name == "" {
		return fallback
	}
	return name
}
