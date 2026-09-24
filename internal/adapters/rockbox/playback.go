package rockbox

import (
	"encoding/hex"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/HansMontana/podsync/internal/adapters/media"
	"github.com/HansMontana/podsync/internal/domain/catalog"
	domainplayback "github.com/HansMontana/podsync/internal/domain/playback"
)

type Record struct {
	Path       string
	Known      bool
	PlayCount  int
	LastPlayed time.Time
	Skipped    bool
}

type hashedState struct {
	domainplayback.State
	Directory string
}

func MergeRecords(records []Record) []Record {
	byPath := make(map[string]Record, len(records))
	for _, record := range records {
		if !record.Known {
			continue
		}
		key := normalizePath(record.Path)
		if key == "" {
			continue
		}
		current := byPath[key]
		current.Path = record.Path
		current.Known = true
		current.PlayCount += record.PlayCount
		current.Skipped = current.Skipped || record.Skipped
		if record.LastPlayed.After(current.LastPlayed) {
			current.LastPlayed = record.LastPlayed
		}
		byPath[key] = current
	}
	result := make([]Record, 0, len(byPath))
	for _, record := range byPath {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		return normalizePath(result[i].Path) < normalizePath(result[j].Path)
	})
	return result
}

// PreferRecords keeps primary records when both sources contain a path and
// uses fallback records only for paths missing from the primary source.
func PreferRecords(primary, fallback []Record) []Record {
	byPath := make(map[string]Record, len(primary)+len(fallback))
	for _, record := range primary {
		if !record.Known {
			continue
		}
		if key := normalizePath(record.Path); key != "" {
			byPath[key] = record
		}
	}
	for _, record := range fallback {
		if !record.Known {
			continue
		}
		key := normalizePath(record.Path)
		if key == "" {
			continue
		}
		if current, exists := byPath[key]; !exists {
			byPath[key] = record
		} else {
			if record.PlayCount > 0 && current.PlayCount == 0 {
				// A valid log proves playback after an uncommitted TagCache update.
				current.PlayCount = record.PlayCount
			}
			if record.LastPlayed.After(current.LastPlayed) {
				current.LastPlayed = record.LastPlayed
			}
			current.Skipped = current.Skipped || record.Skipped
			byPath[key] = current
		}
	}

	result := make([]Record, 0, len(byPath))
	for _, record := range byPath {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		return normalizePath(result[i].Path) < normalizePath(result[j].Path)
	})
	return result
}

// ForEpisodesWithResolver matches device records by current logical media
// paths. Episodes without a trusted matching record remain unknown and
// therefore unplayed.
func ForEpisodesWithResolver(episodes []catalog.Episode, records []Record, resolver media.Resolver) map[string]domainplayback.State {
	byPath := make(map[string]domainplayback.State, len(records))
	byIdentityHash := make(map[string]hashedState, len(records))
	for _, record := range records {
		if !record.Known {
			continue
		}
		key := normalizePath(record.Path)
		if key == "" {
			continue
		}
		byPath[key] = mergeState(byPath[key], record)
		if hash := identityHashFromPath(key); hash != "" {
			byIdentityHash[hash] = hashedState{State: mergeState(byIdentityHash[hash].State, record), Directory: path.Dir(key)}
		}
	}

	result := make(map[string]domainplayback.State, len(episodes))
	for _, current := range episodes {
		relative := normalizePath(resolver.RelativePathFor(current))
		state, exists := byPath[relative]
		if !exists {
			if hashed, found := byIdentityHash[media.IdentityHash(current)]; found && hashed.Directory == path.Dir(relative) {
				state, exists = hashed.State, true
			}
		}
		if exists {
			result[current.IdentityKey()] = state
		} else {
			result[current.IdentityKey()] = domainplayback.State{}
		}
	}
	return result
}

func mergeState(current domainplayback.State, record Record) domainplayback.State {
	current.Known = true
	if record.PlayCount > current.PlayCount {
		current.PlayCount = record.PlayCount
	}
	if record.LastPlayed.After(current.LastPlayed) {
		current.LastPlayed = record.LastPlayed
	}
	current.Skipped = current.Skipped || record.Skipped
	return current
}

func identityHashFromPath(value string) string {
	name := path.Base(value)
	marker := strings.LastIndex(name, " -- ")
	if marker < 0 {
		return ""
	}
	hash := strings.TrimSuffix(name[marker+4:], path.Ext(name))
	if len(hash) != 12 {
		return ""
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return ""
	}
	return hash
}

func normalizePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.TrimPrefix(value, "/")
	if strings.HasPrefix(value, "<") {
		if volumeEnd := strings.Index(value, ">/"); volumeEnd > 0 {
			value = value[volumeEnd+2:]
		}
	}
	clean := path.Clean(value)
	if clean == "." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}
