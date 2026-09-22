package device

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
)

var ErrNoDevice = errors.New("no podsync device detected")

// ResolveRoot returns an explicit root or detects one mounted in a standard
// user mount location. PODSYNC_DEVICE_ROOT is useful when the mount location
// is non-standard.
func ResolveRoot(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if configured := os.Getenv("PODSYNC_DEVICE_ROOT"); configured != "" {
		return configured, nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("detect mounted device: identify current user: %w", err)
	}
	parents := []string{
		filepath.Join("/media", currentUser.Username),
		filepath.Join("/run/media", currentUser.Username),
		"/Volumes",
	}
	var candidates []string
	for _, parent := range parents {
		entries, readErr := os.ReadDir(parent)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return "", fmt.Errorf("inspect mount directory %q: %w", parent, readErr)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(parent, entry.Name())
			if looksLikeDeviceRoot(candidate) {
				candidates = append(candidates, candidate)
			}
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return "", fmt.Errorf("%w; pass -device-root PATH or set PODSYNC_DEVICE_ROOT", ErrNoDevice)
	}
	if len(candidates) > 1 {
		return "", fmt.Errorf("multiple possible podsync devices detected: %v; pass -device-root PATH", candidates)
	}
	return candidates[0], nil
}

// LooksLikeDeviceRoot reports whether root contains an initialized podsync
// database and is safe to consider for daemon processing.
func LooksLikeDeviceRoot(root string) bool {
	return looksLikeDeviceRoot(root)
}

func looksLikeDeviceRoot(root string) bool {
	stateDirectory := filepath.Join(root, "Podsync")
	stateInfo, err := os.Lstat(stateDirectory)
	if err != nil || !stateInfo.IsDir() || stateInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	databaseInfo, err := os.Lstat(filepath.Join(stateDirectory, "podsync.db"))
	return err == nil && databaseInfo.Mode().IsRegular()
}
