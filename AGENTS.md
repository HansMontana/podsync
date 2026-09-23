# AGENTS.md

## Purpose

This file contains durable instructions for coding agents working on `podsync`.

Treat these instructions as project constraints, not suggestions. Inspect the repository before changing it: current code, tests, migrations, `go.mod`, and Git state are more authoritative than stale examples in this file.

Keep this file durable. Do not turn it into a session log, TODO list, or record of temporary branch/commit state.

## Project

`podsync` is a Go application for synchronizing podcasts to an iPod.

Module:

```text
github.com/HansMontana/podsync
```

The project is intentionally built in small, understandable increments. Favor simple domain models, explicit persistence, strong regression tests, and small diffs.

## The most important architecture rule

> **The iPod is the source of truth.**

The user may run podsync from multiple computers. Durable podcast state must therefore travel with the iPod rather than belong to whichever host happened to run the last sync.

Conceptually:

```text
Computer A ─┐
Computer B ─┼──> iPod / podsync SQLite database
Computer C ─┘
```

Do not introduce a second authoritative host-side database.

### Durable device state

The iPod should contain durable state that needs to survive hosts and travel with the device, including:

- the podsync SQLite database
- podcast/feed state
- episode state and preferences
- device-specific podsync configuration where appropriate
- other state that must be shared between computers

The exact on-device path can evolve. Preserve the architectural boundary even if paths change.

The current layout helper uses these device-relative paths:

- `Podsync/podsync.db` for the SQLite database
- `Podsync/podsync.toml` for device-local configuration
- `Podcasts/` for episode audio
- `Playlists/` for generated playlists

The CLI accepts an explicit mounted device root or detects one mounted in a
standard user mount location. Automatic mounting and ejection are not
implemented.

### Transient host state

The host may contain disposable working state, including:

- downloaded audio before transfer
- staging files
- generated playlists before transfer
- intermediate sync plans
- temporary directories
- caches
- other rebuildable artifacts

A host should be able to lose this transient state without losing the user's authoritative podsync state.

Prefer:

```text
read iPod state
-> calculate desired result
-> do temporary work on host
-> apply final copies/deletions to iPod
-> persist durable state on iPod
```

Avoid using the iPod as a scratch/work directory.

## Simplicity: Ponytail mindset

Default to the simplest reliable solution.

The goal is **not** minimum lines at all costs, minimum dependency count, or clever code. The goal is the smallest amount of project-owned complexity that solves the real problem clearly.

Before building something, ask:

1. Does this behavior actually need to exist now?
2. Does the repository already solve it?
3. Is there a mature, focused dependency that solves it simply and reliably?
4. Does the Go standard library or platform solve it just as simply?
5. Otherwise, what is the smallest clear implementation we can own?

Stop as soon as there is a good solution.

### Dependencies are not a defect

Do **not** optimize for dependency count.

If a mature dependency is the simplest reliable solution, use it. Do not reinvent established functionality merely to keep `go.mod` short.

Evaluate a dependency based on things that actually matter:

- does it solve the required problem well?
- is its API simpler than maintaining custom code?
- is it reasonably maintained?
- is it focused enough for the job?
- does it fit the project's portability requirements?
- does it introduce meaningful runtime/build/operational costs?
- does it have licensing or security implications?
- would custom code genuinely be easier to understand and maintain?

A dependency can be preferable even when the standard library could technically be made to do the job with substantially more plumbing.

Likewise, do not add a library for a trivial operation that is clearer in a few lines of Go.

The rule is:

> **Do not reinvent the wheel, and do not install a wheel factory to move one box.**

Existing examples of reasonable dependency choices include SQLite and a migration runner rather than implementing either subsystem ourselves.

### Minimal production complexity

Avoid:

- speculative abstractions
- interfaces with no useful boundary
- factories that only call constructors
- wrappers that add no semantics
- premature generalization
- future-proofing for imagined requirements
- duplicate implementations of library functionality
- unnecessary configuration
- unnecessary layers between domain code and straightforward infrastructure

A small explicit type is good when it represents a real concept or boundary.

Do not flatten useful domain types merely to reduce the number of files or declarations.

Prefer boring code that is easy to understand, test, replace, and delete.

## Communication: Caveman mindset

Agent communication should normally be concise and technical.

Prefer:

- result first
- concrete findings
- exact commands
- short explanations
- one direct question when clarification is genuinely required

Avoid:

- greetings and sign-offs
- praise and filler
- narrating obvious tool operations
- repeating the user's request
- large status reports for small changes
- verbose summaries when the diff already tells the story
- vague hedging when evidence is clear

Do not apply terse prose to technical artifacts. Code, commands, SQL, identifiers, error messages, and migration instructions must remain exact and readable.

Use more explanation when it materially helps with:

- architecture decisions
- Go concepts that are non-obvious
- database migrations
- destructive operations
- Git history rewriting
- data-loss risks
- concurrency
- security
- subtle dependency tradeoffs
- a direct request for explanation

"Caveman" means low-noise communication, not missing reasoning.

## Testing philosophy

Be **lenient about test quantity and strict about production complexity**.

Tests are not where this project tries to save lines.

Agents need executable evidence of intended behavior so later changes can detect regressions. A larger test diff is acceptable when production code remains simple.

This is a perfectly reasonable change shape:

```text
5 lines of production code
60 lines of tests
```

if those tests protect meaningful behavior.

### Tests that are welcome

Use whichever level best protects the behavior:

- focused unit tests
- table-driven tests
- domain behavior tests
- repository tests
- SQLite tests
- migration tests
- functional tests
- integration-style tests across internal packages
- filesystem tests using temporary directories
- close/reopen persistence tests
- regression tests for bugs actually encountered
- end-to-end-ish tests where they provide substantially better confidence

Functional tests are explicitly allowed.

Do not force every behavior into an isolated unit test when a functional test catches the real regression more reliably.

### Regression tests are project memory

When fixing a bug, preferably:

```text
reproduce bug with test
-> observe failure
-> fix root/shared cause
-> observe test pass
-> keep test permanently
```

Choose the highest useful behavioral boundary for the regression.

If a bug only becomes visible when several components interact, a functional regression test is preferable to mocking those interactions into irrelevance.

Do not delete a valuable test merely because another test overlaps with it. Some overlap is acceptable when tests exercise an important invariant through different paths.

### Avoid useless tests

Do not add tests merely to inflate coverage.

Low-value examples include:

- testing that Go assignment works
- testing trivial getters with no behavior
- asserting private implementation details that callers do not care about
- mocks whose only purpose is reproducing the implementation
- snapshotting huge outputs when a small behavioral assertion is clearer
- tests that make refactoring harder without protecting a product invariant

Test behavior, contracts, persistence, boundaries, and regressions.

### Test helpers are allowed

Test-only helpers, fixtures, builders, fakes, and small test types are welcome when they:

- reduce noisy setup
- make intent clearer
- express several regression cases cleanly
- remain understandable
- do not hide the important behavior being tested

Do not contort tests to avoid a useful helper.

## Development workflow

Prefer a red → green → inspect workflow, but do not make TDD ceremonial.

For new behavior:

```text
1. Pick one small behavior or invariant.
2. Add the smallest useful test at the appropriate level.
3. Run it.
4. If it fails for the intended reason, implement the behavior.
5. If it is already green, recognize it as a characterization/regression test.
6. Run focused tests.
7. Run the complete suite.
8. Inspect the diff.
9. Commit a coherent increment when authorized.
```

A test does not need to be artificially made red. Existing behavior may already satisfy a newly written regression test.

If a test fails because of an unrelated problem—bad import, typo, package mismatch, missing driver—fix that problem before claiming the intended behavior is red.

### Standard verification

Useful commands:

```bash
go fmt ./...
go test -count=1 ./...
git diff --check
```

For focused persistence work:

```bash
go test -count=1 ./internal/adapters/sqlitecatalog
```

`-count=1` is intentional when verifying a change because it avoids relying on cached results.

Run verification proportionate to the change. Before declaring a code change complete, normally run the full suite.

If tests cannot be run, say exactly why.

## Go conventions

Use idiomatic, readable Go.

Prefer explicit straightforward code over clever compression.

### Package names

Domain package directories are intentionally singular:

```text
internal/domain/catalog
internal/domain/curation
internal/domain/playback
```

Keep domain package names singular unless a real design reason requires otherwise.

The module path is case-sensitive:

```text
github.com/HansMontana/podsync
```

Use that capitalization exactly.

All `.go` files in a directory must use the same package declaration.

### Errors

Return errors with useful context.

Prefer wrapping:

```go
return fmt.Errorf("load feeds: %w", err)
```

Do not silently swallow errors unless failure is deliberately irrelevant and the code makes that obvious.

### Sets

Idiomatic Go set-like maps are fine:

```go
map[int64]struct{}
```

Existence checks may use:

```go
if _, exists := feedIDs[f.ID]; exists {
    // ...
}
```

Do not replace clear idiomatic Go with Java-shaped abstractions merely because the project owner has Java experience.

When introducing a non-obvious Go idiom, explain it briefly if the user is reviewing the work interactively.

## Domain model

Domain rules should live in domain code, not be accidentally defined by SQLite schema or filesystem layout.

Do not make database implementation details the domain model.

### Feed

`feed.Feed` represents a podcast feed.

Feed identity already includes URL normalization behavior. Reuse the existing normalization/identity logic rather than creating competing normalization in persistence or sync code.

State validation protects feed invariants such as unique IDs and unique normalized feed identities.

Inspect the current struct before changing persistence; do not rely on a copied struct definition in this file.

### Episode

`episode.Episode` represents a podcast episode.

Episode identity is intentionally hierarchical.

First, episodes must belong to the same `FeedID`.

Then:

```text
GUID
  ↓ if unavailable
AudioURL
  ↓ if unavailable
fallback fingerprint
```

More precisely:

1. Different `FeedID` => different identity.
2. If either episode has a GUID, both must have a non-empty equal GUID.
3. Otherwise, if either episode has an AudioURL, both must have a non-empty equal AudioURL.
4. Otherwise compare the fallback fingerprint.

The fallback fingerprint uses:

- `FeedID`
- `Title`
- `PublishedAt` normalized to UTC and formatted consistently
- `Duration`

The fingerprint is SHA-256 based.

`Description` deliberately does **not** participate in identity. A publisher correcting show notes must not create a new episode.

Local filenames and filesystem paths must not determine episode identity.

Do not add a synthetic `Episode.ID` merely because SQL tables commonly have integer primary keys. Add domain identity fields only when the domain needs them.

## Catalog

`catalog.Catalog` represents durable podsync state.

Established validation rules include:

- feed IDs are unique
- normalized feed URLs/identities are unique
- episodes reference known feed IDs
- duplicate episode identities are rejected

Treat `Catalog.Validate()` as an important domain integrity boundary.

When adding a mutation or persistence path, consider whether invalid state must be rejected before durable data is changed.

Do not put disposable host work state into `State` for convenience.

## Persistence

Persistence uses SQLite.

Conceptually:

```text
Catalog
  |
  v
application/catalog.Repository
  |
  v
adapters/sqlitecatalog.SQLiteRepository
  |
  v
SQLite database on iPod
```

The repository boundary currently has the shape:

```go
type Repository interface {
    Load() (catalog.Catalog, error)
    Save(catalog.Catalog) error
    Close() error
}
```

Inspect current code before relying on this exact signature.

`Close()` releases database resources. It does not delete state and is not a substitute for `Save()`.

Production repository implementation belongs in production `.go` files, not `_test.go` files.

A previous mistake placed the SQLite implementation in a test file, which allowed package tests to compile while production code lacked the implementation. The durable lesson is:

> **Tests must exercise production implementations; never let `_test.go` accidentally provide production behavior.**

## SQLite

SQLite uses:

```text
modernc.org/sqlite
```

The pure-Go implementation is useful because podsync should not acquire a CGO requirement merely for database access.

Do not replace it casually with a CGO-backed driver without discussing the portability/build tradeoff.

Use `database/sql` normally.

### Persistence tests should be file-backed

For durable persistence behavior, prefer:

```go
dbPath := filepath.Join(t.TempDir(), "state.db")
```

over testing only with `:memory:`.

An important persistence test shape is:

```text
create database
-> run migrations
-> save state
-> close database
-> reopen same file
-> load
-> verify same durable state
```

This matters because podsync's real requirement is persistence across program executions and computers, not merely reads through one live SQL connection.

## Database migrations

Schema belongs in explicit SQL migration files.

Current structure is approximately:

```text
internal/adapters/sqlitecatalog/
├── migrations/
│   └── 001_initial.sql
├── repository.go
├── repository_compatibility_test.go
└── repository_persistence_test.go
```

Migrations are embedded into the executable with Go `embed`.

Migration runner:

```text
github.com/pressly/goose/v3
```

This intentionally provides a Flyway-like workflow:

```text
versioned SQL files
-> embedded into binary
-> applied on database open/startup
```

Do not replace readable migration files with large `CREATE TABLE` strings hidden inside repository code.

### Migration safety

Normal podsync operation applies forward migrations.

Never automatically roll the user's iPod database backwards.

A migration may contain a Goose `Down` section for tooling, but normal application execution must not invoke it.

Treat migrations as durable data evolution, not disposable test setup.

For a migration already used on real databases, prefer adding a new migration over editing historical schema. Before the project has users/data depending on a migration, amendments may be reasonable, but inspect repository/history context first.

SQL files should end with a newline.

### Persisted feed and episode fields

The current schema persists feed metadata, including ETag and Last-Modified,
and episode fields through a foreign-key relationship to the feed. These fields
are durable iPod state, not host-only cache data.

When evolving persisted fields, inspect the current domain model, migration,
repository, and persistence tests. Add a forward migration for deployed
schemas, preserve the existing episode identity rules, and do not invent a
synthetic domain ID unless the domain requires one.

## Transactions and writes

Durable state changes should be atomic where partial application would corrupt state.

Use transactions where they simplify correctness.

The current repository uses a simple whole-set replacement strategy inside one
transaction:

```text
BEGIN
DELETE existing persisted rows
INSERT desired rows
COMMIT
```

This is acceptable while the state remains small. Revisit the strategy only
when measured scale or new durable state requires it; prefer simple correct
transactions over elaborate incremental machinery.

## Sync design

Separate **planning** from **application** where practical.

A useful conceptual model:

```text
current durable state
+ current feed data
+ device contents
+ user preferences
        |
        v
     sync plan
        |
        v
host downloads / processing
        |
        v
copy/delete final device artifacts
        |
        v
persist resulting durable state
```

A sync plan may be transient. Do not persist it merely because it exists as a type.

Avoid changing device files before enough work has succeeded to know what the desired result is.

When destructive deletion behavior is introduced, tests should cover it carefully.

RSS enclosure lengths are advisory for existing MP3 files. A persisted delivery
signature determines whether a previously signed MP3 changed; an unsigned
existing MP3 may be reused without rejecting it because its file size differs
from the publisher's reported length. Preparation progress must count each
selected episode once across chunked preparation and finalization.

## Dependency and Go-version discipline

Dependencies are welcome when they reduce complexity.

Dependency changes still deserve inspection.

After changing dependencies, inspect:

```bash
git diff -- go.mod go.sum
```

Use:

```bash
go mod tidy
```

when appropriate.

Do not casually raise the project's Go version merely because `go get` rewrote `go.mod`.

If a dependency requires a newer Go version, determine that explicitly and make the upgrade an intentional project decision.

Likewise, do not pin the project to an obsolete Go version merely to avoid a justified upgrade.

The principle is intentionality, not dependency avoidance.

## Git workflow

The user prefers a clean Git history and reviews diffs before committing.

Do not push unless explicitly asked.

Do not commit unless explicitly asked or the task clearly authorizes a commit.

### Before commit

Normally inspect:

```bash
git status --short
git diff --stat
git diff
git diff --check
go test -count=1 ./...
```

When staged:

```bash
git diff --cached --stat
git diff --cached
```

Keep commits coherent. Avoid mixing unrelated cleanup with feature work.

### Signed commits

Commits are SSH-signed.

When making an authorized commit:

```bash
git commit -S -m "<message>"
```

Verify when useful:

```bash
git log -1 --show-signature
```

Do not disable signing just to make a commit succeed.

### Amend local mistakes

If the most recent commit contains a mistake and has **not** been pushed, prefer fixing it and amending the commit when that keeps history coherent:

```bash
git add -A
git commit --amend --no-edit -S
```

Do not rewrite pushed history without explicit approval.

Before amending, verify that the commit is actually local/unpushed rather than assuming.

### Gitmoji

The user likes gitmoji commit messages.

Examples of the project's style:

```text
✨ Add episode identity model and tests
✨ Add feed URL normalization and identity
🧱 Add persistent state invariants
🗃️ Add SQLite state persistence
```

Choose a gitmoji that communicates the nature of the change. Do not add one mechanically when it makes the message less clear.

## Diff discipline

Keep production diffs focused.

Do not:

- reformat unrelated files
- rename unrelated symbols
- reorganize directories during an unrelated fix
- update dependencies unrelated to the task
- "clean up" working code opportunistically
- introduce abstractions for hypothetical future work

Tests may legitimately be broader than the production diff when they provide regression protection.

When a task exposes a nearby bug that blocks the requested work, fix it if necessary and explain it briefly. Otherwise leave unrelated work alone.

## Agent autonomy

Inspect first. Do not guess file contents.

Before editing a subsystem, read the relevant production code and tests.

Use existing project vocabulary.

Do not invent requirements because they are common in other podcast applications.

When several solutions are valid:

- choose the simplest reliable one
- prefer an established dependency when it removes meaningful custom machinery
- prefer clear domain behavior over framework patterns
- preserve the iPod-as-source-of-truth architecture
- preserve regression visibility

Ask before making a decision only when the missing information materially changes product behavior, data format, destructive behavior, or architecture and cannot be inferred safely from the repository.

Do not ask permission for routine implementation details when the requested task already authorizes the work.

## Destructive operations

Be conservative around:

- deleting device files
- deleting database rows
- changing identity semantics
- changing migration history
- force pushing
- rewriting pushed commits
- resetting user changes
- replacing configuration
- destructive schema migrations

Never discard uncommitted user work to simplify an agent task.

Before a destructive operation, establish why it is necessary and what data can be affected.

## Security and secrets

Do not commit:

- credentials
- tokens
- private keys
- personal secrets
- machine-specific secret configuration

Do not print secrets into logs or tests.

Keep test fixtures synthetic.

Do not weaken TLS verification or similar security controls merely to make development easier.

## Roadmap

The following product direction is intentional, but should still be implemented in small verified increments:

1. Complete standard RSS support with `github.com/mmcdole/gofeed`, while keeping HTTP fetching, caching, and domain mapping in podsync. (Implemented.)
2. Store user-authored configuration on the iPod alongside the SQLite database. (Implemented for explicit device roots.)
3. Distinguish source feeds from logical feeds and filtered partitions, so one RSS feed can provide several independently configured podcast entities. (Implemented.)
4. Support per-feed episode ordering: newest-first, oldest-first, or both where useful. (Implemented.)
5. Generate playlists for logical feeds using the configured ordering. (Implemented.)
6. Define daily briefings as ordered sections containing selected logical feeds and per-section episode limits. (Implemented.)
7. Read Rockbox playback statistics, including play count and last-played information, from the iPod. (Playback-log and binary TagCache import implemented.)
8. Match Rockbox playback records to stable episode identities without making local paths the domain identity. (Implemented for current stable media paths; migration of older path formats remains intentionally unsupported.)
9. Add configurable selection rules such as unplayed-only and newest-episode limits. (Implemented.)
10. Detect the iPod and apply sync plans safely: download to transient host storage, copy final files to the iPod, and remove obsolete files conservatively. (Implemented with explicit-root and standard-location detection.)
11. Transfer generated playlists and preserve the configured briefing and feed order. (Implemented.)
12. Add a CLI for feed management, refresh, briefing generation, sync, update, verification, and status. (Implemented.)
13. Add end-to-end tests covering configuration, feed refresh, Rockbox playback state, playlist generation, and interrupted syncs. (Synthetic coverage and read-only real-device verification implemented; broader real-device validation remains.)

The roadmap does not authorize speculative implementation. Each increment must preserve the iPod-as-source-of-truth boundary and establish its behavior with tests before the next layer is added.

### Playback state rule

Playback state comes from the iPod or Rockbox when available. If an episode's
play count cannot currently be read, matched, or trusted, treat the episode as
unplayed. Do not invent a host-side played state that overrides the device.

## Definition of done

A change is normally complete when:

- the requested behavior is implemented
- the implementation is no more complicated than necessary
- meaningful regression tests exist at the right level
- focused tests pass
- the full relevant test suite passes
- formatting is clean
- `git diff --check` is clean
- dependency/migration changes were inspected
- no unrelated changes slipped into the diff
- durable state still follows the iPod-as-source-of-truth rule

Do not report success if a required verification failed. Report the failure and its cause.

## Final principle

Optimize these three things differently:

```text
production code: minimize owned complexity
tests: maximize useful regression confidence
communication: minimize noise
```

Dependencies are tools, not moral failures.

Use a good wheel when one exists.

Write your own wheel only when doing so is actually simpler.
