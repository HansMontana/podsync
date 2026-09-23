package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestDaemonRunsOncePerDeviceSession(t *testing.T) {
	root := t.TempDir()
	identity, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	current := daemonDevice{root: root, identity: identity}
	checks := 0
	runs := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = daemonLoop(ctx, daemonOptions{pollInterval: time.Millisecond}, func() (daemonDevice, bool, error) {
		checks++
		switch checks {
		case 1, 2, 4:
			return current, true, nil
		case 3:
			return daemonDevice{}, false, nil
		default:
			return daemonDevice{}, false, nil
		}
	}, func(context.Context, updateOptions) error {
		runs++
		if runs == 2 {
			cancel()
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("daemonLoop() returned error: %v", err)
	}
	if runs != 2 {
		t.Fatalf("daemon ran %d times, want one run per device session", runs)
	}
}

func TestDaemonRetriesFailedUpdateBeforeCompletingSession(t *testing.T) {
	root := t.TempDir()
	identity, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	current := daemonDevice{root: root, identity: identity}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0

	err = daemonLoop(ctx, daemonOptions{pollInterval: time.Millisecond}, func() (daemonDevice, bool, error) {
		return current, true, nil
	}, func(context.Context, updateOptions) error {
		runs++
		if runs == 1 {
			return errors.New("transient update failure")
		}
		cancel()
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("daemonLoop() returned error: %v", err)
	}
	if runs != 2 {
		t.Fatalf("daemon ran %d times, want retry followed by success", runs)
	}
}
