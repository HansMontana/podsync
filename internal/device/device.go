package device

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Layout struct {
	Root string
}

func (l Layout) StateDirectory() string {
	return filepath.Join(l.Root, "Podsync")
}

func (l Layout) DatabasePath() string {
	return filepath.Join(l.StateDirectory(), "podsync.db")
}

func (l Layout) ConfigPath() string {
	return filepath.Join(l.StateDirectory(), "podsync.toml")
}

func (l Layout) AudioDirectory() string {
	return filepath.Join(l.Root, "Podcasts")
}

func (l Layout) PlaylistDirectory() string {
	return filepath.Join(l.Root, "Playlists")
}

func (l Layout) PlaybackLogPaths() ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(l.Root, ".rockbox", "playback*.log"))
	if err != nil {
		return nil, fmt.Errorf("find playback logs: %w", err)
	}
	return paths, nil
}

func (l Layout) TagCacheDirectory() string {
	return filepath.Join(l.Root, ".rockbox")
}

func (l Layout) ManifestPath() string {
	return filepath.Join(l.StateDirectory(), "managed-files.txt")
}

func (l Layout) ManifestRelativePath() string {
	return "Podsync/managed-files.txt"
}

func (l Layout) LoadManagedPaths() ([]string, error) {
	file, err := os.Open(l.ManifestPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open managed-files manifest: %w", err)
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if path != "" {
			paths = append(paths, path)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read managed-files manifest: %w", err)
	}
	return paths, nil
}
