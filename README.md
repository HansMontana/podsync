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
- Rockbox playback-log and TagCache parsing with stable media-path matching, with unknown records treated as unplayed
- CLI workflows for validation, reconciliation, refresh, feed management, status, playlist, briefing, and sync

Not yet implemented:

- Automatic iPod detection
- Full interrupted-sync end-to-end coverage against a real Rockbox layout

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

- Go 1.25 or later

Run the tests:

```bash
go test -count=1 ./...
```

The CLI requires an explicit mounted device root because automatic device
detection is not implemented:

```bash
podsync refresh -device-root /media/ipod -config ./configs/briefing.toml
podsync feed list -device-root /media/ipod
podsync sync -device-root /media/ipod --dry-run
```

`sync` uses already refreshed episode state. For new content, use
`reconcile`/`feed add`, then `refresh`, then `sync`.

Format and verify a change:

```bash
go fmt ./...
git diff --check
```

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
