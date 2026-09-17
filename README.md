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

Not yet implemented:

- iPod-local TOML configuration
- Logical feeds and filtered partitions
- Per-feed playlist ordering
- Daily briefing generation
- Rockbox playback-statistics import
- Audio download and device synchronization
- A complete command-line interface

See `AGENTS.md` for the durable project architecture and roadmap.

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
