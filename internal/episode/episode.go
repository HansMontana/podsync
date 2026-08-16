package episode

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Episode represents a podcast episode known to podsync.
//
// The GUID is the stable identity supplied by the podcast feed.
// Filenames and local paths deliberately do not form part of the identity.
type Episode struct {
	FeedID      int64
	GUID        string
	Title       string
	Description string
	AudioURL    string
	PublishedAt time.Time
	Duration    time.Duration
}

func (e Episode) SameIdentity(other Episode) bool {
	if e.FeedID != other.FeedID {
		return false
	}

	if e.GUID != "" || other.GUID != "" {
		return e.GUID != "" &&
			other.GUID != "" &&
			e.GUID == other.GUID
	}

	if e.AudioURL != "" || other.AudioURL != "" {
		return e.AudioURL != "" &&
			other.AudioURL != "" &&
			e.AudioURL == other.AudioURL
	}

	return e.fingerprint() == other.fingerprint()
}

func (e Episode) fingerprint() string {
	data := fmt.Sprintf(
		"%d\x00%s\x00%s\x00%d",
		e.FeedID,
		e.Title,
		e.PublishedAt.UTC().Format(time.RFC3339Nano),
		e.Duration,
	)

	sum := sha256.Sum256([]byte(data))

	return hex.EncodeToString(sum[:])
}
