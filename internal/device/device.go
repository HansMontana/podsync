package device

import (
	"bufio"
	"fmt"
	"io"
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

func (l Layout) PendingManifestPath() string {
	return filepath.Join(l.StateDirectory(), "pending-managed-files.txt")
}

func (l Layout) ManifestRelativePath() string {
	return "Podsync/managed-files.txt"
}

func (l Layout) LoadManagedPaths() ([]string, error) {
	paths, err := l.LoadCommittedManagedPaths()
	if err != nil {
		return nil, err
	}
	pending, err := loadPathList(l.PendingManifestPath())
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(paths)+len(pending))
	for _, path := range paths {
		seen[path] = struct{}{}
	}
	for _, path := range pending {
		if _, exists := seen[path]; !exists {
			paths = append(paths, path)
			seen[path] = struct{}{}
		}
	}
	return paths, nil
}

func (l Layout) LoadCommittedManagedPaths() ([]string, error) {
	return loadPathList(l.ManifestPath())
}

func (l Layout) LoadPendingManagedPaths() ([]string, error) {
	return loadPathList(l.PendingManifestPath())
}

func (l Layout) SavePendingManagedPaths(paths []string) error {
	for _, path := range paths {
		if err := validateOwnedPath(path); err != nil {
			return err
		}
	}
	return writePathList(l.PendingManifestPath(), paths)
}

func (l Layout) ClearPendingManagedPaths() error {
	err := os.Remove(l.PendingManifestPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func loadPathList(path string) ([]string, error) {
	if err := RejectSymlink(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
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

func writePathList(path string, paths []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create path-list directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".podsync-paths-*")
	if err != nil {
		return fmt.Errorf("create temporary path list: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	for _, current := range paths {
		if _, err := io.WriteString(temporary, current+"\n"); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("write path list: %w", err)
		}
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync path list: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close path list: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install path list: %w", err)
	}
	return nil
}

func validateOwnedPath(path string) error {
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean != path || filepath.IsAbs(path) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("invalid pending managed path %q", path)
	}
	if !strings.HasPrefix(clean, "Podcasts/") && !strings.HasPrefix(clean, "Playlists/") {
		return fmt.Errorf("invalid pending managed path %q", path)
	}
	return nil
}
