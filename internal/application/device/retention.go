package device

import (
	"path/filepath"
	"strings"
)

func managedPathsForSkippedEpisodes(managed []string, skipped map[string]struct{}) []string {
	if len(managed) == 0 || len(skipped) == 0 {
		return nil
	}
	markers := make(map[string]struct{}, len(skipped))
	for relative := range skipped {
		if marker := managedPathIdentityMarker(relative); marker != "" {
			markers[marker] = struct{}{}
		}
	}
	if len(markers) == 0 {
		return nil
	}
	kept := make([]string, 0)
	for _, relative := range managed {
		if !strings.HasPrefix(filepath.ToSlash(relative), "Podcasts/") {
			continue
		}
		if _, exists := markers[managedPathIdentityMarker(relative)]; exists {
			kept = append(kept, relative)
		}
	}
	return kept
}

func managedPathIdentityMarker(relative string) string {
	name := filepath.Base(filepath.FromSlash(relative))
	marker := strings.LastIndex(name, " -- ")
	if marker < 0 {
		return ""
	}
	return strings.TrimSuffix(name[marker+4:], filepath.Ext(name))
}

func appendManagedManifestPaths(content []byte, retained []string) []byte {
	text := strings.TrimRight(string(content), "\n")
	lines := make([]string, 0)
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	existing := make(map[string]struct{}, len(lines)+len(retained))
	for _, line := range lines {
		existing[strings.TrimSpace(line)] = struct{}{}
	}
	for _, relative := range retained {
		if _, exists := existing[relative]; exists {
			continue
		}
		lines = append(lines, relative)
		existing[relative] = struct{}{}
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}
