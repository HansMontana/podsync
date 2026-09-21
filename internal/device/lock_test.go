package device

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireLockRejectsConcurrentOwner(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Podsync"), 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := AcquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := AcquireLock(root)
	if err == nil {
		second.Close()
		t.Fatal("second lock acquisition succeeded")
	}
	if !strings.Contains(err.Error(), "another podsync operation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAcquireLockCanBeReacquiredAfterClose(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Podsync"), 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := AcquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "link")
	if err := os.WriteFile(target, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := RejectSymlink(link); err == nil {
		t.Fatal("RejectSymlink accepted a symlink")
	}
}
