# podsync User Guide

`podsync` synchronizes configured podcasts to a Rockbox iPod. The iPod is the
authoritative state location: the SQLite database and TOML configuration live
under `Podsync/` on the device.

## Safety Model

The current application does not detect, mount, or eject devices. Always pass
the mounted device root explicitly:

```bash
podsync status -device-root /media/hansmontana/HANSPOD
```

Before the first real sync:

1. Mount the iPod normally.
2. Check the mount path and device contents.
3. Run `sync --dry-run`.
4. Review the planned episode count and deletions.
5. Run `sync` only after the plan is acceptable.

Podsync manages only paths recorded in its device-local manifest. It does not
manage or delete `AUDIO/`, `iPod_Control/`, or other files created outside
podsync. Existing files at podsync-generated paths are reused when they are
non-empty regular files and their known enclosure size matches.

## Build

Requirements:

- Go 1.25 or later

Build the command:

```bash
go build -o podsync ./cmd/podsync
```

Run the test suite:

```bash
go test -count=1 ./...
```

## Configuration

Validate a host-side TOML file before using it:

```bash
podsync validate-config -config ./configs/briefing.toml
```

Normal device commands copy an explicitly supplied config to:

```text
Podsync/podsync.toml
```

If `-config` is omitted, commands read that device-local file.

Example source and logical-feed configuration:

```toml
[[source]]
id = "news"
url = "https://example.com/news.xml"

[[feed]]
id = "world"
title = "World News"
source = "news"
order = "newest_first"
limit = 5
unplayed_only = true

[feed.filter]
title_contains = "World"
```

Logical-feed selection is a rolling queue. Played episodes are filtered first,
then older unplayed episodes fill the configured limit.

`limit = 0` or an omitted limit means no limit. Valid ordering values are
`newest_first` and `oldest_first`.

## Briefings

Briefings contain ordered sections:

```toml
[[briefing]]
id = "morning"
title = "Morning Briefing"

[[briefing.section]]
feed = "world"
order = "newest_first"
limit = 1
unplayed_only = true
```

Briefing sections behave differently from rolling logical-feed playlists. A
section first selects its configured episode window. If that window contains a
played episode and `unplayed_only = true`, that section is omitted. It is not
backfilled, and later sections continue normally.

## Commands

Show all commands:

```bash
podsync --help
podsync help
```

Show command-specific help:

```bash
podsync sync --help
podsync help sync
```

Reconcile configured source feeds into the device database:

```bash
podsync reconcile \
  -device-root /media/hansmontana/HANSPOD \
  -config ./configs/briefing.toml
```

Refresh all configured source feeds:

```bash
podsync refresh -device-root /media/hansmontana/HANSPOD
```

Show state and playback counts:

```bash
podsync status -device-root /media/hansmontana/HANSPOD
```

List source feeds without requiring a config file:

```bash
podsync feed list -device-root /media/hansmontana/HANSPOD
```

Add or remove a source feed:

```bash
podsync feed add \
  -device-root /media/hansmontana/HANSPOD \
  -id technology \
  -url https://example.com/technology.xml

podsync feed remove \
  -device-root /media/hansmontana/HANSPOD \
  -id technology
```

Removal is rejected while a logical feed still references the source. Removing
a source from configuration does not immediately delete its durable database
rows; obsolete media is removed only from paths podsync previously recorded as
managed during a later sync.

Generate one logical-feed playlist:

```bash
podsync playlist \
  -device-root /media/hansmontana/HANSPOD \
  -id world
```

Generate one briefing playlist:

```bash
podsync briefing \
  -device-root /media/hansmontana/HANSPOD \
  -id morning
```

Preview a sync without downloading or changing device files:

```bash
podsync sync \
  -device-root /media/hansmontana/HANSPOD \
  -dry-run
```

Apply a sync:

```bash
podsync sync -device-root /media/hansmontana/HANSPOD
```

Downloads first go to host-side staging. Device files are written through
temporary files and renames. Managed deletions happen only after downloads and
playlist writes succeed.

## Playback State

Podsync reads Rockbox playback information without writing Rockbox databases.
Sources are used in this order:

1. TagCache records are authoritative when a path exists there.
2. `.rockbox/playback.log` fills paths missing from TagCache.
3. Missing or untrusted records are treated as unplayed.

Rockbox playback logging can be enabled from its playback/settings menu. The
log is useful for newly played files before TagCache is refreshed.

After syncing new audio, Rockbox’s own database should be updated from the
device, usually through:

```text
Settings -> General Settings -> Database -> Update Now
```

This is not required merely to play a generated path-based playlist, but it is
needed for Rockbox database browsing and for TagCache statistics on newly added
files.

## Storage and Deletion

The SQLite database stores current feed and episode metadata, not audio files
or playback logs. Refresh replaces the current episode set for each feed, so
historical RSS episodes do not accumulate indefinitely.

Host staging files are removed after a normal or failed sync. A forcibly killed
process can leave a temporary staging directory under `/tmp`; it is safe to
remove stale `podsync-staging-*` directories after confirming no sync is
running.

On the iPod, only manifest-listed podsync paths can be deleted. Played episodes
can be removed from a logical feed using `unplayed_only = true`; this is the
intended rolling-queue behavior. Files outside podsync-managed paths are not
deleted.

## Current Limitations

- Device autodiscovery is not implemented.
- Automatic mounting and ejection are not implemented.
- A mounted device root must be supplied explicitly.
- Real-device validation is still required for final hardware confidence.
