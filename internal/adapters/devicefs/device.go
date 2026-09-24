package devicefs

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type Layout struct {
	Root string
}

type PendingFeedRemoval struct {
	URL string `json:"url"`
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

func (l Layout) PendingSyncConfigPath() string {
	return filepath.Join(l.StateDirectory(), "pending-sync.toml")
}

func (l Layout) ClearPendingSyncConfig() error {
	path := l.PendingSyncConfigPath()
	if err := RejectSymlink(path); err != nil {
		return err
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove pending sync configuration: %w", err)
	}
	return syncDirectory(filepath.Dir(path))
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

func (l Layout) MediaSignaturesPath() string {
	return filepath.Join(l.StateDirectory(), "media-signatures.json")
}

func (l Layout) LoadMediaSignatures() (map[string]string, error) {
	if err := RejectSymlink(l.MediaSignaturesPath()); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(l.MediaSignaturesPath())
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read media signatures: %w", err)
	}
	var signatures map[string]string
	if err := json.Unmarshal(content, &signatures); err != nil {
		return nil, fmt.Errorf("decode media signatures: %w", err)
	}
	if signatures == nil {
		signatures = map[string]string{}
	}
	for path := range signatures {
		if err := validateOwnedPath(path); err != nil {
			return nil, fmt.Errorf("validate media signature path: %w", err)
		}
	}
	return signatures, nil
}

func (l Layout) SaveMediaSignatures(signatures map[string]string) error {
	for path, signature := range signatures {
		if err := validateOwnedPath(path); err != nil {
			return fmt.Errorf("validate media signature path: %w", err)
		}
		if strings.TrimSpace(signature) == "" {
			return fmt.Errorf("media signature for %q is empty", path)
		}
	}
	content, err := json.Marshal(signatures)
	if err != nil {
		return fmt.Errorf("encode media signatures: %w", err)
	}
	return writeAtomic(l.MediaSignaturesPath(), content, 0o600, "media signatures")
}

func (l Layout) PendingFeedRemovalPath() string {
	return filepath.Join(l.StateDirectory(), "pending-feed-removal.json")
}

func (l Layout) SavePendingFeedRemoval(url string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("pending feed removal URL cannot be empty")
	}
	content, err := json.Marshal(PendingFeedRemoval{URL: url})
	if err != nil {
		return fmt.Errorf("encode pending feed removal: %w", err)
	}
	return writeAtomic(l.PendingFeedRemovalPath(), content, 0o600, "pending feed removal")
}

func (l Layout) LoadPendingFeedRemoval() (PendingFeedRemoval, error) {
	if err := RejectSymlink(l.PendingFeedRemovalPath()); err != nil {
		return PendingFeedRemoval{}, err
	}
	content, err := os.ReadFile(l.PendingFeedRemovalPath())
	if os.IsNotExist(err) {
		return PendingFeedRemoval{}, nil
	}
	if err != nil {
		return PendingFeedRemoval{}, fmt.Errorf("read pending feed removal: %w", err)
	}
	var pending PendingFeedRemoval
	if err := json.Unmarshal(content, &pending); err != nil {
		return PendingFeedRemoval{}, fmt.Errorf("decode pending feed removal: %w", err)
	}
	if strings.TrimSpace(pending.URL) == "" {
		return PendingFeedRemoval{}, fmt.Errorf("pending feed removal URL is empty")
	}
	return pending, nil
}

func (l Layout) ClearPendingFeedRemoval() error {
	err := os.Remove(l.PendingFeedRemovalPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(l.PendingFeedRemovalPath()))
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
	if err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(l.PendingManifestPath()))
}

func EnsureStateDirectory(root string) error {
	path := filepath.Join(root, "Podsync")
	if err := RejectSymlink(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create device state directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect device state directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("device state directory is not a real directory")
	}
	return nil
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
	var content strings.Builder
	for _, current := range paths {
		content.WriteString(current)
		content.WriteByte('\n')
	}
	return writeAtomic(path, []byte(content.String()), 0o600, "path list")
}

func writeAtomic(path string, content []byte, mode os.FileMode, description string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", description, err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".podsync-atomic-*")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", description, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set %s permissions: %w", description, err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write %s: %w", description, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync %s: %w", description, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s: %w", description, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install %s: %w", description, err)
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
		return fmt.Errorf("sync directory: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close directory after sync: %w", closeErr)
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
