package playlists

import "github.com/HansMontana/podsync/internal/domain/curation"

// Filename returns a readable, safe filename for a playlist title.
func Filename(title, fallback string) string {
	return curation.PlaylistFilename(title, fallback)
}
