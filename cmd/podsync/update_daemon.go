package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	devicefs "github.com/HansMontana/podsync/internal/adapters/devicefs"
	"github.com/HansMontana/podsync/internal/adapters/logging"
)

func update(ctx context.Context, args []string) error {
	flags := newFlagSet("update", "Usage: podsync update [-device-root PATH] [-config PATH] [-staging PATH] [-skip-verify-media]")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration (defaults to device config)")
	stagingDir := flags.String("staging", "", "host-side staging directory (defaults to a temporary directory)")
	skipVerifyMedia := flags.Bool("skip-verify-media", false, "skip inspection and repair of existing MP3 metadata")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	return runUpdate(ctx, updateOptions{
		deviceRoot:      *deviceRoot,
		configPath:      *configPath,
		stagingDir:      *stagingDir,
		skipVerifyMedia: *skipVerifyMedia,
	})
}

type updateOptions struct {
	deviceRoot      string
	configPath      string
	stagingDir      string
	skipVerifyMedia bool
	lock            *devicefs.Lock
	identity        fs.FileInfo
}

func runUpdate(ctx context.Context, options updateOptions) error {
	logger := logging.New(os.Stderr).WithComponent("update")
	started := time.Now()
	logger.Info("Starting update")
	resolvedRoot, identity, err := devicefs.ResolveRootIdentity(options.deviceRoot)
	if err != nil {
		return err
	}
	if options.identity != nil {
		if err := devicefs.VerifyRootIdentity(resolvedRoot, options.identity); err != nil {
			return fmt.Errorf("verify detected device: %w", err)
		}
		identity = options.identity
	}
	if err := os.MkdirAll(filepath.Join(resolvedRoot, "Podsync"), 0o755); err != nil {
		return fmt.Errorf("prepare device lock: %w", err)
	}
	lock := options.lock
	if lock == nil {
		lock, err = devicefs.AcquireLock(resolvedRoot)
	}
	if err != nil {
		return err
	}
	if options.lock == nil {
		defer lock.Close()
	}
	refreshReport, err := reconcileDeviceWithLockContext(ctx, resolvedRoot, options.configPath, true, lock)
	if err != nil {
		return fmt.Errorf("update refresh: %w", err)
	}
	refreshErr := refreshReport.FailureError()
	if refreshErr != nil && refreshReport.Refreshed == 0 {
		return fmt.Errorf("update refresh: %w", refreshErr)
	}
	logger.Info(fmt.Sprintf("Refresh complete: %d succeeded, %d failed", refreshReport.Refreshed, len(refreshReport.Failures)))
	if refreshErr != nil {
		logger.Warn("Refresh had failures; continuing with available feed data")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := devicefs.VerifyRootIdentity(resolvedRoot, identity); err != nil {
		return fmt.Errorf("before update sync: %w", err)
	}
	if err := syncDeviceContext(ctx, syncOptions{deviceRoot: resolvedRoot, configPath: options.configPath, stagingDir: options.stagingDir, skipVerifyMedia: options.skipVerifyMedia, lock: lock, identity: identity}); err != nil {
		return fmt.Errorf("update sync: %w", err)
	}
	logger.Info("Sync complete")
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := devicefs.VerifyRootIdentity(resolvedRoot, identity); err != nil {
		return fmt.Errorf("before update verification: %w", err)
	}
	verified, err := verifyDevice(resolvedRoot)
	if err != nil {
		return fmt.Errorf("update verification: %w", err)
	}
	logger.Info(fmt.Sprintf("Device verification complete: %d managed files", verified))
	logger.Info(fmt.Sprintf("Update complete in %s", time.Since(started).Round(time.Millisecond)))
	if refreshErr != nil {
		return fmt.Errorf("update completed with refresh failures: %w", refreshErr)
	}
	return nil
}

type daemonOptions struct {
	deviceRoot      string
	configPath      string
	stagingDir      string
	pollInterval    time.Duration
	skipVerifyMedia bool
}

type daemonDevice struct {
	root     string
	identity fs.FileInfo
}

type daemonDetector func() (daemonDevice, bool, error)
type daemonRunner func(context.Context, updateOptions) error

func daemon(args []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("daemon mode is currently supported on Linux only")
	}
	flags := newFlagSet("daemon", "Usage: podsync daemon [-device-root PATH] [-config PATH] [-staging PATH] [-poll-interval DURATION] [-skip-verify-media]")
	deviceRoot := flags.String("device-root", "", "mounted iPod root (auto-detected if omitted)")
	configPath := flags.String("config", "", "path to podsync TOML configuration (defaults to device config)")
	stagingDir := flags.String("staging", "", "host-side staging directory (defaults to a temporary directory)")
	pollInterval := flags.Duration("poll-interval", 30*time.Second, "how often to check for a mounted iPod")
	skipVerifyMedia := flags.Bool("skip-verify-media", false, "skip inspection and repair of existing MP3 metadata")
	if help, err := parseFlags(flags, args); err != nil {
		return err
	} else if help {
		return nil
	}
	if *pollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runDaemon(ctx, daemonOptions{
		deviceRoot:      *deviceRoot,
		configPath:      *configPath,
		stagingDir:      *stagingDir,
		pollInterval:    *pollInterval,
		skipVerifyMedia: *skipVerifyMedia,
	})
}

func runDaemon(ctx context.Context, options daemonOptions) error {
	logger := commandLogger("daemon")
	return daemonLoop(ctx, options, func() (daemonDevice, bool, error) {
		return detectDaemonDevice(options.deviceRoot)
	}, func(ctx context.Context, update updateOptions) error {
		return runUpdate(ctx, update)
	}, logger)
}

func daemonLoop(ctx context.Context, options daemonOptions, detect daemonDetector, run daemonRunner, logger *logging.Logger) error {
	var session *daemonDevice
	ticker := time.NewTicker(options.pollInterval)
	defer ticker.Stop()

	for {
		current, present, err := detect()
		if err != nil {
			if logger != nil {
				logger.Warn("device detection failed: " + err.Error())
			}
			present = false
			session = nil
		}
		if !present {
			if session != nil && logger != nil {
				logger.Info("Device removed")
			}
			session = nil
		} else if session == nil || session.root != current.root || !os.SameFile(session.identity, current.identity) {
			if logger != nil {
				logger.Info("Device detected; running one update")
			}
			err := run(ctx, updateOptions{
				deviceRoot:      current.root,
				configPath:      options.configPath,
				stagingDir:      options.stagingDir,
				skipVerifyMedia: options.skipVerifyMedia,
				identity:        current.identity,
			})
			if ctx.Err() != nil {
				return nil
			}
			if err != nil && logger != nil {
				logger.Error("Update failed: " + err.Error())
			} else if logger != nil {
				logger.Info("Update complete for device session")
				deviceSession := current
				session = &deviceSession
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func detectDaemonDevice(explicitRoot string) (daemonDevice, bool, error) {
	root := explicitRoot
	if root == "" {
		var err error
		root, err = devicefs.ResolveRoot("")
		if err != nil {
			if errors.Is(err, devicefs.ErrNoDevice) {
				return daemonDevice{}, false, nil
			}
			return daemonDevice{}, false, err
		}
	}
	if !devicefs.LooksLikeDeviceRoot(root) {
		return daemonDevice{}, false, nil
	}
	resolvedRoot, identity, err := devicefs.ResolveRootIdentity(root)
	if err != nil {
		if os.IsNotExist(err) {
			return daemonDevice{}, false, nil
		}
		return daemonDevice{}, false, err
	}
	return daemonDevice{root: resolvedRoot, identity: identity}, true, nil
}
