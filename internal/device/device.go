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
	return filepath.Join(l.Root, ".rockbox", "tagcache")
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

func (l Layout) SaveManagedPaths(paths []string) error {
	if err := os.MkdirAll(l.StateDirectory(), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temporary, err := os.CreateTemp(l.StateDirectory(), ".podsync-managed-*")
	if err != nil {
		return fmt.Errorf("create managed-files manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	for _, path := range paths {
		if _, err := fmt.Fprintln(temporary, path); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("write managed-files manifest: %w", err)
		}
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close managed-files manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, l.ManifestPath()); err != nil {
		return fmt.Errorf("install managed-files manifest: %w", err)
	}
	return nil
}
