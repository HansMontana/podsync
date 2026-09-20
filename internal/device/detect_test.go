package device

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRootUsesExplicitRoot(t *testing.T) {
	if got, err := ResolveRoot("/explicit/device"); err != nil || got != "/explicit/device" {
		t.Fatalf("ResolveRoot() = %q, %v", got, err)
	}
}

func TestResolveRootUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("PODSYNC_DEVICE_ROOT", "/configured/device")
	if got, err := ResolveRoot(""); err != nil || got != "/configured/device" {
		t.Fatalf("ResolveRoot() = %q, %v", got, err)
	}
}

func TestLooksLikeDeviceRoot(t *testing.T) {
	root := t.TempDir()
	if looksLikeDeviceRoot(root) {
		t.Fatal("empty directory detected as device")
	}
	if err := os.Mkdir(filepath.Join(root, "Podsync"), 0o755); err != nil {
		t.Fatal(err)
	}
	if looksLikeDeviceRoot(root) {
		t.Fatal("uninitialized Podsync directory was detected")
	}
	if err := os.WriteFile(filepath.Join(root, "Podsync", "podsync.db"), []byte("database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !looksLikeDeviceRoot(root) {
		t.Fatal("initialized Podsync directory was not detected")
	}
}

func TestVerifyRootIdentityRejectsReplacement(t *testing.T) {
	root := t.TempDir()
	_, identity, err := ResolveRootIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRootIdentity(root, identity); err != nil {
		t.Fatalf("VerifyRootIdentity() rejected unchanged root: %v", err)
	}
	other := t.TempDir()
	if err := VerifyRootIdentity(other, identity); err == nil {
		t.Fatal("VerifyRootIdentity() accepted a different root")
	}
}
