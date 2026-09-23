# podsync User Guide

`podsync` synchronizes configured podcasts to a Rockbox iPod. The iPod is the
authoritative state location: the SQLite database and TOML configuration live
under `Podsync/` on the device.

## Safety Model

Podsync can detect a single mounted device in standard user mount locations.
Pass the mounted device root explicitly when more than one device is present
or when the mount is non-standard:

```bash
podsync status -device-root /media/hansmontana/HANSPOD
```

Set `PODSYNC_DEVICE_ROOT` to use a non-standard mount without repeating the
flag.

Podsync does not mount or eject devices. Linux desktop environments or a
container host must mount the iPod first. One-shot commands can initialize a
new device; daemon mode only processes an initialized device containing
`Podsync/podsync.db`.

Run a read-only device integrity check with:

```bash
podsync verify
```

This validates every manifest-managed file as an existing, non-empty regular
file without changing the device.

Before the first real sync:

1. Mount the iPod normally.
2. Check the mount path and device contents.
3. Run `sync --dry-run`.
4. Review the selected episode, playlist, and deletion counts.
5. Run `sync` only after the plan is acceptable.

Normal syncs inspect selected existing MP3 tags and repair missing metadata.
Use `-skip-verify-media` to use fast filesystem checks only. Large transfers are
processed in batches targeting 5 GiB or 200 episodes, whichever comes first.

Only manifest-listed podsync paths are eligible for deletion. Podsync does not
delete `AUDIO/`, `iPod_Control/`, or other files created outside podsync. Sync
can write or replace its selected generated `Podcasts/<logical-feed>/...` and
`Playlists/...` destinations even when they were not in an earlier manifest.
Existing generated audio is reused when it is a non-empty regular file with a
matching delivery signature and expected length. MP3 metadata is inspected by
default for selected existing episodes.
Individual episode downloads are limited to 2 GiB and RSS responses to 32 MiB.

## Build

Requirements:

- Go 1.25.7 or later

Build the command:

```bash
go build -o podsync ./cmd/podsync
```

Run the test suite:

```bash
go test -count=1 ./...
```

## Linux Daemon

The optional Linux-only daemon waits for an initialized, already-mounted iPod
and runs one `update` per mount session. It polls for device appearance and
does not mount or eject the device. The existing `update` command remains a
one-shot command and can be used to initialize a new device first.

Run it directly:

```bash
podsync daemon
```

For a user-level systemd service, copy
`contrib/systemd/podsync-daemon.service` to
`~/.config/systemd/user/podsync-daemon.service`, adjust `ExecStart` if needed,
then enable it:

```bash
systemctl --user daemon-reload
systemctl --user enable --now podsync-daemon.service
```

View daemon logs with:

```bash
journalctl --user -u podsync-daemon.service
```

For a future Linux container deployment, mount the iPod on the host and bind
mount it into the container at a stable path. Pass that path explicitly:

```bash
podsync daemon -device-root /ipod
```

The container needs network access and write access to the mounted iPod. Keep
the SQLite database and TOML configuration on the iPod; do not create a second
authoritative host or container database. USB mounting and ejecting remain host
responsibilities.

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

To initialize a device, first run `reconcile -device-root ... -config ...`.
After that, config-less device commands use `Podsync/podsync.toml`.

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

Feed and enclosure URLs must use HTTPS. Podsync rejects remote hosts that
resolve to loopback, private, link-local, unspecified, or multicast addresses.

`limit = 0` or an omitted limit means no limit. Valid ordering values are
`newest_first` and `oldest_first`.

`feed.order`, `briefing.section.order`, and `briefing.section.limit` are
required. A briefing section uses its referenced logical feed's source and
filter, but uses its own ordering, limit, and `unplayed_only` setting.

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

`refresh` fetches up to four sources concurrently and persists successful source
results as one batch. Failed sources retain their previous state. Cancellation
does not persist the in-progress batch. `sync` uses stored episodes and does not fetch RSS; use
`reconcile` or `feed add`, then configure a logical `[[feed]]` entry, then
`refresh` and `sync` for new feed content.

Run the normal production workflow, which refreshes feeds, syncs selected media,
and verifies all managed files:

```bash
podsync update
```

Deep existing-MP3 metadata verification runs by default. To skip it:

```bash
podsync update -skip-verify-media
```

The update workflow stops when refresh has no successful sources or sync fails.
When some sources refresh successfully and others fail, it syncs the available
data, runs final verification, and returns a non-zero status describing the
refresh failures.

Show state and playback counts:

```bash
podsync status -device-root /media/hansmontana/HANSPOD
```

List source feeds without requiring a config file:

```bash
podsync feed list -device-root /media/hansmontana/HANSPOD
```

The output columns are durable numeric feed ID, RSS feed name, and URL. Use the
source ID from configuration, rather than the listed numeric ID or RSS name,
with `feed remove -id`.

Add or remove a source feed. `feed add` creates only the RSS source; add a
logical `[[feed]]` configuration entry before `sync` can select its episodes:

```bash
podsync feed add \
  -device-root /media/hansmontana/HANSPOD \
  -id technology \
  -url https://example.com/technology.xml

podsync feed remove \
  -device-root /media/hansmontana/HANSPOD \
  -id technology
```

Removal is rejected while a logical feed still references the source. It deletes
the matching source feed and its episodes from the device database immediately.
Previously managed media remains until a later sync removes it.

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

Preview desired managed paths without downloading or changing device files:

```bash
podsync sync \
  -device-root /media/hansmontana/HANSPOD \
  -dry-run
```

Apply a sync:

```bash
podsync sync -device-root /media/hansmontana/HANSPOD
```

Downloads first go to host-side staging. An explicit `-staging` path must be
outside the device root. Device files are written through temporary files and
renames. Managed deletions happen only after downloads and playlist writes
succeed. A supplied configuration and reconciled state are persisted after file
application succeeds; a final persistence failure can leave new files with the
previous configuration/state and is recoverable by rerunning sync.

Normal syncs inspect selected existing MP3 tags and repair missing metadata; use
`-skip-verify-media` to avoid this work when needed. This is slower on large
archives. Newly downloaded MP3 files are always normalized.

## Logging

CLI logs are written to stderr in a concise timestamped format:

```text
2026-08-17T12:51:28.508+02:00	INFO	sync	Starting sync
2026-08-17T12:51:31.102+02:00	WARN	sync	Keeping an unreadable existing file
```

Help text remains plain command output. Runtime logs report command lifecycle,
sync milestones, recoverable warnings, failures, and final summaries without
printing every individual file by default.

## Playback State

Podsync reads Rockbox playback information without writing Rockbox databases.
Sources are used in this order:

1. TagCache records are preferred when a path exists there.
2. A valid positive `.rockbox/playback.log` record supersedes a zero-count
   TagCache record; otherwise the log fills paths missing from TagCache.
3. Missing or untrusted records are treated as unplayed.

A playback-log entry counts as played only when at least 90% of the episode
duration was reached. Short previews and abandoned partial listens remain
unplayed.

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

During sync, podsync normalizes metadata on MP3 files using the RSS episode and
feed state. The episode title is written as the title, the feed name as album
and artist, `Podcast` as genre, and the publication year when available. Existing
selected managed MP3 files are checked by default. Other audio formats are
copied without metadata changes.

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

- Database migrations are forward-only. Downgrading podsync requires restoring
  a device backup made with the older version.
- An older protected podsync binary rejects a device database containing a
  migration version it does not understand. Keep computers sharing an iPod on
  compatible podsync versions.
- Released migration identifiers are immutable: never reuse or redefine a
  migration version after release.
- Automatic mounting and ejection are not implemented.
- Automatic detection requires a single mounted device in a standard location,
  or `PODSYNC_DEVICE_ROOT` for a non-standard location.
- Broader validation against real Rockbox playback and TagCache data remains.
