package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/device"
	"github.com/HansMontana/podsync/internal/feed"
	"github.com/HansMontana/podsync/internal/state"
	syncer "github.com/HansMontana/podsync/internal/sync"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: podsync <validate-config|reconcile|refresh> [options]")
	}
	switch args[0] {
	case "validate-config":
		return validateConfig(args[1:])
	case "reconcile":
		return reconcile(args[1:], false)
	case "refresh":
		return reconcile(args[1:], true)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func validateConfig(args []string) error {
	flags := flag.NewFlagSet("validate-config", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("-config is required")
	}
	if _, err := config.Load(*configPath); err != nil {
		return err
	}
	fmt.Println("configuration is valid")
	return nil
}

func reconcile(args []string, refresh bool) error {
	flags := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	deviceRoot := flags.String("device-root", "", "mounted iPod root")
	configPath := flags.String("config", "", "path to podsync TOML configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *deviceRoot == "" || *configPath == "" {
		return fmt.Errorf("-device-root and -config are required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	layout := device.Layout{Root: *deviceRoot}
	if err := os.MkdirAll(layout.StateDirectory(), 0o755); err != nil {
		return fmt.Errorf("create device state directory: %w", err)
	}
	repository, err := state.NewSQLiteRepository(layout.DatabasePath())
	if err != nil {
		return err
	}
	defer repository.Close()

	current, err := repository.Load()
	if err != nil {
		return err
	}
	current, err = cfg.EnsureSources(current)
	if err != nil {
		return err
	}
	if err := repository.Save(current); err != nil {
		return fmt.Errorf("save configured sources: %w", err)
	}
	if !refresh {
		fmt.Printf("reconciled %d source feeds\n", len(cfg.Sources))
		return nil
	}

	client := http.DefaultClient
	for _, source := range cfg.Sources {
		normalized, err := feed.NormalizeURL(source.URL)
		if err != nil {
			return fmt.Errorf("normalize source %q: %w", source.ID, err)
		}
		current, err = repository.Load()
		if err != nil {
			return err
		}
		var feedID int64
		for _, known := range current.Feeds {
			knownURL, normalizeErr := feed.NormalizeURL(known.URL)
			if normalizeErr == nil && knownURL == normalized {
				feedID = known.ID
				break
			}
		}
		if feedID == 0 {
			return fmt.Errorf("source %q was not reconciled", source.ID)
		}
		if err := syncer.RefreshFeed(context.Background(), repository, client, feedID); err != nil {
			return fmt.Errorf("refresh source %q: %w", source.ID, err)
		}
		fmt.Printf("refreshed %s\n", source.ID)
	}
	return nil
}
