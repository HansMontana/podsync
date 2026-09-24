 # podsync

`podsync` is a Go application for synchronizing podcasts to an iPod running
Rockbox.

The project is being built incrementally. The iPod is intended to be the
source of truth, so durable podsync state travels with the device rather than
belonging to one host computer.

## Current status

Implemented:

- Podcast feed and episode domain models
- Episode identity based on feed ID, GUID, audio URL, or a fallback fingerprint
- SQLite persistence with embedded Goose migrations
- Standard RSS parsing with `github.com/mmcdole/gofeed`
- Audio enclosure metadata and filtering
- Conditional RSS refresh using ETag and Last-Modified headers
- Initial feed refresh reconciliation and regression tests
- Validated source-feed, logical-feed, filter, ordering, and briefing configuration
- Device-local TOML configuration persistence and mounted-device layout handling
- Logical-feed and briefing playlist generation with stable episode paths
- Host-side bounded downloads and safe filesystem sync planning/application
- Chunked syncs for large transfers with resumable intermediate batches
- Global preparation progress across chunked transfers
- Rockbox playback-log and TagCache parsing with stable media-path matching, with unknown records treated as unplayed
- Partial Rockbox playback is tracked as skipped for briefing consumption without counting as completed playback
- Existing-media reuse with default MP3 metadata verification and an opt-out
- Linux daemon mode for one update per mounted-device session
- Automatic mounted-device detection, read-only managed-file verification, and `PODSYNC_DEVICE_ROOT` support
- CLI workflows for validation, reconciliation, refresh, update, feed management, status, verification, playlist, briefing, and sync
- Semantic-versioned releases with Linux amd64/arm64 and experimental macOS Apple Silicon CLI artifacts

Remaining validation work:

- Broader validation against real Rockbox playback and TagCache data

See `AGENTS.md` for the durable project architecture and roadmap.
See [`docs/user-guide.md`](docs/user-guide.md) for installation, configuration,
CLI usage, sync safety, and Rockbox playback guidance.
See [`examples/`](examples/) for ordinary podcast and German daily-briefing
configuration examples.

## Architecture

The intended state boundary is:

```text
host
  | read state and feed data
  v
calculate a sync plan
  |
  v
host-side temporary downloads and processing
  |
  v
iPod: final audio, playlists, configuration, and podsync database
```

The host may lose caches and staging files without losing authoritative
podcast state.

## Development

Requirements:

- Go 1.25.7 or later
- Linux is the currently supported application platform

Run the tests:

```bash
go test -count=1 ./...
```

The CLI can detect one mounted device in standard user mount locations. Pass an
explicit root when more than one device is mounted or when using a non-standard
mount:

```bash
podsync refresh -device-root /media/ipod -config ./configs/briefing.toml
podsync feed list -device-root /media/ipod
podsync sync -device-root /media/ipod -dry-run
```

For a configured device, `update` combines refresh, sync, deep verification of
selected existing MP3s, and final device verification. Use
`-skip-verify-media` to skip deep existing-MP3 metadata verification:

```bash
podsync update
podsync update -skip-verify-media
```

## Releases

Releases use semantic-version tags such as `v0.1.0`. Pushing a matching tag
runs the private-repository release workflow, which tests and publishes Linux
amd64, Linux arm64, and macOS Apple Silicon (`darwin/arm64`) archives with
SHA-256 checksums. The macOS artifact supports the CLI; daemon mode remains
Linux-only.

Show the version of a local build:

```bash
podsync version
podsync --version
```

`sync` uses already refreshed episode state. For new content, use
`reconcile` or `feed add`, then configure a logical `[[feed]]` entry, then
`refresh` and `sync`. Refresh fetches up to four sources concurrently and saves
the refreshed device state as one batch.

Linux daemon mode waits for an initialized, already-mounted device and runs one
`update` per mount session. It does not mount or eject devices. See the user
guide for the systemd service example and container deployment notes.

Format and verify a change:

```bash
go fmt ./...
git diff --check
```

Normal syncs inspect selected existing MP3 tags and repair missing metadata.
Use `-skip-verify-media` to use fast filesystem checks only. Large transfers
are processed in batches targeting 5 GiB or 200 episodes, whichever comes
first. `podsync verify` checks all manifest-managed files without changing the
device. CLI logs use timestamped `INFO`, `WARN`, and `ERROR` lines on stderr;
help text remains plain command output.

## AI and agentic development disclosure

This project has been developed with agentic assistance through OpenCode.
AI assistance has primarily used OpenAI GPT-5.6 models, including GPT-5.6
Luna and GPT-5.6 Luna Fast. Other OpenAI models may also have been used.

The human project author remains responsible for the project’s direction,
review, and published changes.

## License

Copyright (C) 2026 HansMontana

`podsync` is licensed under the GNU General Public License, version 3 or any
later version. See `LICENSE` for the complete license text.
