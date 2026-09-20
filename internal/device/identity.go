package device

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ResolveRootIdentity resolves a device root and captures the filesystem
// identity that must remain stable for a multi-phase operation.
func ResolveRootIdentity(explicit string) (string, fs.FileInfo, error) {
	root, err := ResolveRoot(explicit)
	if err != nil {
		return "", nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("resolve device root: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", nil, fmt.Errorf("inspect device root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("device root is not a real directory")
	}
	return root, info, nil
}

// VerifyRootIdentity rejects a device replacement or mount change between
// phases of a multi-step operation.
func VerifyRootIdentity(root string, expected fs.FileInfo) error {
	current, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect device root: %w", err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(expected, current) {
		return fmt.Errorf("device root changed during operation")
	}
	return nil
}
