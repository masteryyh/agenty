# Agenty Inspector

A read-only local web application for exploring Agenty session transcripts. It
shares core's data-directory resolver, JSONL framing, event decoder and session
replay. It works without a running core process, SQLite index, configuration file,
provider credentials or completed setup wizard.

## Run

From the repository root:

```bash
pnpm install
pnpm inspector:dev
```

Open `http://127.0.0.1:5173`. Development runs the Go API on port 4318 and Vite on
5173. `INSPECTOR_API_PORT` changes the development API port; the frontend proxy
receives the same value. Ctrl-C stops both processes.

The default data directory is `~/.agenty`. The same environment variable as core
selects another directory:

```bash
AGENTY_DATA_DIR="$HOME/agenty-data" pnpm inspector:dev
```

Relative `AGENTY_DATA_DIR` values passed through pnpm resolve against its invocation
directory (`INIT_CWD`). Direct binary invocation resolves them against the process
working directory, matching core. No shell expansion is applied to quoted `~`.

For a single executable with embedded web assets:

```bash
pnpm inspector:build
pnpm inspector:start
pnpm inspector:start -- --port 4320 --open
```

The binary is `packages/agenty-inspector/bin/agenty-inspector` (`.exe` on Windows).
It can be run from any directory without Node, pnpm, a separate web directory or an
Agenty installation. `--port 0` selects an available port and prints its address.
The executable always binds to `127.0.0.1`.

Inspector is built independently of the CLI/core/patch/bootstrap release payloads.
The default root `pnpm build` excludes Inspector; use `pnpm inspector:build` for its
standalone binary. The build uses the same `AGENTY_VERSION` resolution as the other
modules.

## Explore a session

- **Messages**: session settings, rounds and a simplified message stream.
  User, Assistant and Tool entries are labeled separately. Tool calls and
  results use code blocks; structured result content is decoded once so its
  actual whitespace is preserved. Hidden text is shown from its decoded content
  with whitespace preserved. Plain reasoning is italicized; encrypted reasoning
  is replaced by a short availability notice.
- **Events**: physical JSONL order, event sequence, mapped event names, time,
  summaries and parse errors. Filter by event type or round, or search the
  complete raw text.
- **Issues**: malformed records, sequence anomalies, session/round/message
  inconsistencies, missing terminal records and ambiguous/unmatched tool results.
- **Inspector panel**: event-only details with an `Info` tab for structured event
  data and a `Raw` tab for formatted JSON. Tool relations link to their original
  event records; source messages are never moved or rewritten.

Session search covers titles, IDs and relative file paths. Model/date filters
and **With issues** narrow the list. Dates use the server's local time and source
file modification date. Session list ordering uses file modification time, then
stable file ID. Multiple files claiming the same session ID remain separate entries.

The data directory is scanned every second. New file generations are announced via
SSE; **Follow** loads new snapshots automatically. Without Follow, the current view
stays frozen until **Load latest snapshot** or Refresh is selected. Inspecting a
source turns Follow off. Closing the inspector panel clears the selected event.
The URL saves the session, view and selected event record.

## Reading and resource boundaries

- Only `sessions/**/*.jsonl` is read. Filesystem access is rooted inside that
  directory; symlinks cannot escape it. Inspector never initializes or writes an
  Agenty data directory and never opens its SQLite database or provider files.
- Missing/unreadable directories produce a visible status; scanning retries so a
  subsequently created directory can be discovered without restarting.
- File order is authoritative. Empty physical lines are preserved in the event
  browser and skipped by core replay. A valid final JSON record without a newline
  can be replayed; an incomplete trailing fragment remains visible but unapplied.
- An undecodable event or content block stops replay at the valid prefix. Later
  raw records remain accessible. Unknown fields remain available in the raw record.
  Invalid UTF-8 also exposes the exact original bytes as base64.
- Each snapshot reads up to the initial file size. Appends produce a new snapshot;
  truncation or replacement invalidates current discovery state. A read that
  detects an in-place rewrite returns a retryable conflict. Concurrent in-place
  overwrites cannot provide a transactional snapshot; normal core writes append.
- Snapshot revisions are content hashes. API subqueries require the selected
  revision. Up to eight snapshots share a 128 MiB estimated cache budget. Expired
  revisions return HTTP 409; deleted sessions return 404. Large snapshots can be
  served uncached while their source revision still exists.
- A transcript may be up to **64 MiB**. Larger files stay visible in the list with
  an explicit error and are not silently truncated. Parsing is serialized across
  scanning and cache misses to bound transient memory. This is a local debugging
  tool, not an unbounded trace archive.
- Event/session lists use virtual rows and server pagination. Round messages are
  loaded in pages of 30. Long text is displayed in 20,000-character segments,
  with complete Copy available. Full-text search runs on the server and results
  are paginated.
- Raw HTML in Markdown is disabled. Remote images are not fetched; supported
  embedded raster images can be previewed explicitly. Host/origin checks and a
  content security policy protect the local HTTP surface. API responses use
  `Cache-Control: no-store`; the browser only persists its theme preference.

## Interpretation limits

**Replayed context is not a captured provider request.** Historical system prompts,
tool definitions and final model-budget trimming are not fully persisted. The
inspector does not substitute current configuration for historical facts.

JSONL event `seq`, round `sequence`, and live RPC notification `sequence` are distinct.
`wroteAt` is populated from event occurrence time, not an independent disk-flush time.
A `running` round without a terminal event does not establish that core is alive.
Token deltas, exact tool durations, failed partial model streams and the internal
compaction execution cannot be reconstructed from these files.

Message, round and compaction usage stay separate. Round usage may already include
automatic compaction, so the UI does not add these overlapping counters together.

## Development and validation

```bash
pnpm inspector:test
pnpm inspector:typecheck
pnpm --filter agenty-inspector generate
pnpm exec eslint packages/agenty-inspector

# From packages/agenty-inspector:
go test -race ./...
go vet ./...

# Shared-reader regressions, from packages/agenty-core:
go test ./pkg/infra/transcript ./pkg/infra/storage ./pkg/domain/conversation
```

When the default Go cache is not writable, prefix commands with
`GOCACHE=/private/tmp/agenty-go-cache`.

`cmd/generate` emits `web/src/generated.ts` from Go DTOs and all core content-block
variants. Tests and builds check the generated file for drift. `go test ./...`
does not need built web assets; release builds use the `production` build tag to
embed `web/dist`. Backend tests use synthetic transcripts only.

The workspace references core through a local Go `replace`. Turbo inputs explicitly
include core Go sources and module files so changes invalidate Inspector builds.
