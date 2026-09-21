package device

import (
	"fmt"
	"os"
)

// RejectSymlink prevents device-local state files from redirecting outside the device.
func RejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect device state path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("device state path is a symlink: %q", path)
	}
	return nil
}
