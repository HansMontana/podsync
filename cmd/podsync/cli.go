package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/HansMontana/podsync/internal/version"
)

func runContext(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return nil
	}
	if args[0] == "--help" || args[0] == "-h" {
		printUsage(os.Stdout)
		return nil
	}
	if args[0] == "version" || args[0] == "--version" {
		if len(args) > 1 {
			return fmt.Errorf("unexpected argument %q", args[1])
		}
		fmt.Println(versionText())
		return nil
	}
	if args[0] == "help" {
		if len(args) > 2 {
			return fmt.Errorf("unexpected argument %q", args[2])
		}
		if len(args) == 1 {
			printUsage(os.Stdout)
			return nil
		}
		return printCommandHelp(args[1])
	}
	switch args[0] {
	case "validate-config":
		return validateConfig(args[1:])
	case "reconcile":
		return reconcile(ctx, args[1:], false)
	case "refresh":
		return reconcile(ctx, args[1:], true)
	case "update":
		return update(ctx, args[1:])
	case "status":
		return status(args[1:])
	case "verify":
		return verify(args[1:])
	case "feed":
		return feedCommand(ctx, args[1:])
	case "playlist":
		return generatePlaylist(ctx, args[1:], false)
	case "briefing":
		return generatePlaylist(ctx, args[1:], true)
	case "sync":
		return sync(ctx, args[1:])
	case "daemon":
		return daemon(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func usageText() string {
	return `Usage: podsync <command> [options]

Commands:
  validate-config  Validate a TOML configuration file.
  reconcile        Add configured source feeds to device state.
  refresh          Reconcile and refresh all configured source feeds.
  update           Refresh feeds, sync selected media, and verify the device.
  status           Show device state and playback summary.
  verify           Verify manifest-managed device files without changing them.
  feed             List, add, or remove source feeds.
  playlist         Generate one logical-feed playlist.
  briefing         Generate one briefing playlist.
  sync             Download selected audio and apply device files.
  daemon           Run one update per mounted device session (Linux only).
  version          Show the podsync version.

Use "podsync help <command>" or "podsync <command> --help" for details.`
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, usageText())
}

func printCommandHelp(command string) error {
	var text string
	switch command {
	case "validate-config":
		text = "Usage: podsync validate-config -config PATH\n\nValidate configuration without changing a device."
	case "reconcile":
		text = "Usage: podsync reconcile [-device-root PATH] [-config PATH]\n\nReconcile configured source feeds into the device database."
	case "refresh":
		text = "Usage: podsync refresh [-device-root PATH] [-config PATH]\n\nReconcile and refresh all configured source feeds."
	case "update":
		text = "Usage: podsync update [-device-root PATH] [-config PATH] [-staging PATH] [-skip-verify-media]\n\nRefresh feeds, sync selected media, and verify the device."
	case "status":
		text = "Usage: podsync status [-device-root PATH]\n\nShow feed, episode, and playback counts."
	case "verify":
		text = "Usage: podsync verify [-device-root PATH]\n\nVerify manifest-managed device files without changing them."
	case "feed":
		text = "Usage: podsync feed <list|add|remove> [options]\n\n" +
			"  list    Usage: podsync feed list [-device-root PATH]\n" +
			"  add     Usage: podsync feed add -device-root PATH -id ID -url URL [-config PATH]\n" +
			"  remove  Usage: podsync feed remove -device-root PATH -id ID [-config PATH]\n\n" +
			"Manage source feeds in device-local configuration."
	case "playlist":
		text = "Usage: podsync playlist [-device-root PATH] -id FEED [-config PATH]\n\nGenerate one logical-feed playlist."
	case "briefing":
		text = "Usage: podsync briefing [-device-root PATH] -id BRIEFING [-config PATH]\n\nGenerate one briefing playlist."
	case "sync":
		text = "Usage: podsync sync [-device-root PATH] [-config PATH] [-staging PATH] [-dry-run] [-skip-verify-media]\n\nApply selected audio, playlists, and managed-file cleanup."
	case "daemon":
		text = "Usage: podsync daemon [-device-root PATH] [-config PATH] [-staging PATH] [-poll-interval DURATION] [-skip-verify-media]\n\nRun one update per mounted device session on Linux."
	case "version":
		text = "Usage: podsync version\n\nShow the podsync version."
	default:
		return fmt.Errorf("unknown help topic %q\n\n%s", command, usageText())
	}
	fmt.Println(text)
	return nil
}

func versionText() string {
	return "podsync " + version.Version
}

func newFlagSet(name, usage string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "%s\n\nOptions:\n", usage)
		flags.PrintDefaults()
	}
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) (bool, error) {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			flags.SetOutput(os.Stdout)
			break
		}
	}
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return true, nil
	}
	if err == nil && flags.NArg() > 0 {
		return false, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	return false, err
}
