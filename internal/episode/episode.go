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
	Author      string
	Description string
	Enclosure   Enclosure
	PublishedAt time.Time
	Duration    time.Duration
}

// Enclosure identifies the downloadable media attached to an episode.
type Enclosure struct {
	URL    string
	Type   string
	Length int64
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

	if e.Enclosure.URL != "" || other.Enclosure.URL != "" {
		return e.Enclosure.URL != "" &&
			other.Enclosure.URL != "" &&
			e.Enclosure.URL == other.Enclosure.URL
	}

	return e.fingerprint() == other.fingerprint()
}

// IdentityKey returns a stable key for matching an episode across refreshes.
func (e Episode) IdentityKey() string {
	if e.GUID != "" {
		return fmt.Sprintf("guid:%d:%s", e.FeedID, e.GUID)
	}
	if e.Enclosure.URL != "" {
		return fmt.Sprintf("audio:%d:%s", e.FeedID, e.Enclosure.URL)
	}
	return "fingerprint:" + e.fingerprint()
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
