# agenty-core

The next-generation core runtime for Agenty, being built from scratch to replace
`agenty-runtime`. It is designed around a local-first storage model (filesystem +
SQLite) and a Domain-Driven Design (DDD) domain layer.

## Storage model

The filesystem is the source of truth; SQLite is a query-side projection.

| Data | Location | Role |
| --- | --- | --- |
| Session transcript | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` | Write model — append-only event log (source of truth) |
| Session index | `~/.agenty/agenty.sqlite` → `sessions` | Read model — projection for fast listing/search |
| Global config | `~/.agenty/config.json` | Application configuration |
| Providers | `~/.agenty/providers/<slug>/provider.json` | Catalog aggregate |
| Models | `~/.agenty/providers/<slug>/models/<model-slug>.json` | Catalog aggregate member |
| Agents | `~/.agenty/agents/<slug>.json` | Agent aggregate |

A session's messages and rounds are never stored in SQLite; the `sessions` table is a
summary projection that can be rebuilt by replaying the JSONL transcript. Its current
configuration projection includes the selected model, its `context_window`, and the
thinking effort.

## Domain layer

The domain layer is split by bounded context. Aggregates reference each other only by
identity (UUIDv7 for the conversation family, kebab-case slugs for agents, providers,
and models).

```
pkg/domain/
├── shared/        Shared kernel: Slug, ModelRef, ThinkingEffort, Metadata, Event, ID
├── conversation/  Session aggregate (Session → Round → Message), content blocks, events
├── agent/         Agent aggregate
└── catalog/       Provider aggregate (Provider → Model)
```

The conversation transcript is event-sourced: each JSONL line is a domain event
(`session_started`, `session_model_set`, `session_thinking_effort_set`,
`session_cwd_set`, `round_started`, `message_appended`, `round_ended`, ...), and a
`Session` aggregate can be reconstructed with `conversation.ReplaySession`. A Session
holds the current configuration for future rounds, while `RoundStarted` snapshots the
model, context window, thinking effort, and working directory used by that round.

## Infrastructure layer

The infrastructure layer (`pkg/infra/`) implements the domain repositories using the
filesystem + SQLite storage model.

```
pkg/infra/
├── config/             Load ~/.agenty/config.json and resolve data-dir paths
├── initialize/         OpenRepositories: one-call setup of all stores
├── storage/            Repository implementations + SQLite connection factory
│   ├── db.go           OpenDB/OpenIsolatedDB + sessions schema
│   ├── agent.go        AgentRepository (agent JSON files)
│   ├── catalog.go      CatalogRepository (provider/model JSON files, DeleteModel)
│   └── conversation.go ConversationRepository (JSONL transcript + SQLite projection)
└── rpc/                stdio JSON-RPC 2.0 interface layer
    ├── message.go      Request/Response/Notification/Error/ID wire types
    ├── codes.go        standard + server-defined error codes
    ├── handler.go      Handler interface + Dispatcher
    ├── server.go       NDJSON-over-stdio Server (batch, notifications, cancel)
    └── adapter/        application services -> JSON-RPC method handlers
```

## Application layer

The application layer (`pkg/application/`) hosts use-case services that orchestrate
the domain aggregates and repositories, classify failures into a small set of
business error codes, and keep mutations consistent with the event-sourced Session
aggregate (load -> mutate -> save -> clear pending events).

Each service consumes the smallest repository interface required by its use cases.
Production wires the filesystem/SQLite repositories, while unit tests use isolated
in-memory fakes without opening files or a database.

- `AgentService` — agent CRUD (`Create`/`Get`/`List`/`Update`/`Delete`).
- `ProviderService` — provider CRUD plus model sub-resource operations
  (`AddModel`/`RemoveModel`).
- `SessionService` — session CRUD and configuration mutations
  (`SetTitle`/`SetModel`/`SetThinkingEffort`/`SetCwd`).

`application.Error` carries a `Code` (NotFound/AlreadyExists/Validation/Internal)
that the interface layer maps to a structured JSON-RPC error code.

## Interface layer

The interface layer is a stdio JSON-RPC 2.0 server. The protocol core lives in
`pkg/infra/rpc/` and the method adapters in `pkg/infra/rpc/adapter/`. The
`cmd/main.go` entrypoint opens the repositories, wires the services, registers
handlers, and serves requests.

Transport is line-delimited JSON (NDJSON): one JSON-RPC message per line on
stdin, one response per line on stdout. Each line must be a single compact JSON
value produced by `json.Marshal` (no `MarshalIndent`); unescaped control bytes
or multi-line JSON would split one message across lines and corrupt framing.
Notifications (requests without an `id`) produce no response; batches (arrays)
produce a single array response. Diagnostics go to stderr so stdout stays a
clean JSON-RPC stream. The server shuts down on stdin EOF, SIGINT or SIGTERM.

An inbound line exceeding 64 MiB is not fatal: the server discards it, replies
with `-32003` message too large (`id: null`, `data.maxLineBytes`), and keeps
serving. Because the discarded line has no parseable `id`, a sender that gets
`-32003` must stop pipelining and resend the payload via the chunked upload
protocol below.

Methods follow a `resource.action` naming:

| Group | Methods |
| --- | --- |
| Agent | `agent.create`, `agent.get`, `agent.list`, `agent.update`, `agent.delete` |
| Provider | `provider.create`, `provider.get`, `provider.list`, `provider.update`, `provider.delete`, `provider.addModel`, `provider.removeModel` |
| Session | `session.create`, `session.get`, `session.list`, `session.delete`, `session.setTitle`, `session.setModel`, `session.setThinkingEffort`, `session.setCwd` |
| Chunk | `chunk.begin`, `chunk.part`, `chunk.commit`, `chunk.abort` |

### Chunked uploads

A request whose `params` exceed the 64 MiB per-line cap is uploaded in shards
and assembled server-side before the real method runs:

1. `chunk.begin` `{requestId, method, totalSize?, chunkCount?}` opens a session.
2. `chunk.part` `{requestId, index, data}` appends one shard; indices must be
   contiguous from zero. `data` is the base64 of a raw slice of the params JSON
   text, so any split point is safe.
3. `chunk.commit` `{requestId}` assembles the shards in index order, validates
   the result as JSON, dispatches `method` in-process, and returns the real
   method's result (or its structured error, with the real method's error code)
   under the commit request's `id`.
4. `chunk.abort` `{requestId}` cancels an in-flight session.

NDJSON is ordered and the server dispatches on a single goroutine, so a sender
may pipeline `begin` + `part`s + `commit` without waiting for intermediate
responses. Sessions live in process memory and are reaped after 5 min idle; an
interrupted upload must restart with a new `chunk.begin`. Total assembled
payload is capped at 256 MiB (`-32004` if exceeded).

Error codes: standard JSON-RPC (`-32700` parse, `-32600` invalid request,
`-32601` method not found, `-32602` invalid params, `-32603` internal) plus
server-defined `-32001` not found, `-32002` already exists, `-32003` message
too large, and `-32004` chunk payload too large. Application validation errors
map to `-32602`.

Example:

```
$ echo '{"jsonrpc":"2.0","id":1,"method":"agent.create","params":{"slug":"dev","name":"Dev"}}' | go run ./cmd
{"jsonrpc":"2.0","id":1,"result":{"slug":"dev","name":"Dev",...}}
```

Note: the `rpc` and `adapter` packages use `encoding/json` (RawMessage-native,
dependency-free) rather than sonic; the application and storage layers still use
sonic per the project convention.

## Development commands

Run module-scoped commands from the repository root:

```sh
pnpm core:build             # compile all agenty-core packages
pnpm core:test              # all tests except integration and e2e build tags
pnpm core:test:integration  # default suite plus integration-tagged tests
pnpm core:test:e2e          # real-binary stdio workflows in isolated processes
pnpm core:test:e2e:race     # e2e harness and core binary with race detection
pnpm core:test:race         # default suite with the race detector
pnpm core:test:repeat       # shuffled repeated run for isolation checks
pnpm core:tidyup            # go fmt, go vet, and go mod tidy
pnpm core:clean             # remove Go build and test caches for the module
```

There is intentionally no local service command yet. End-to-end tests use the `e2e`
build tag so they stay outside the default `core:test` suite.

## Testing

The default suite covers domain behavior, application services with in-memory
repository fakes, protocol framing, configuration, and isolated storage adapter
contracts. Full repository wiring and RPC-to-disk paths use the `integration`
build tag.

The `test/e2e` package builds `cmd` once, launches the real binary over stdio, and gives
each parallel test process its own `AGENTY_DATA_DIR`. It covers public Agent,
Provider/Model, Session, JSON-RPC, chunking, startup, restart persistence, and process
isolation contracts without accessing the user's data directory.

All filesystem and SQLite tests use per-test temporary directories. Tests that
change `AGENTY_DATA_DIR` are not parallelized because environment variables are
process-global.

See [TESTING.md](./TESTING.md) for the full testing strategy and command guide, or
[TESTING-CN.md](./TESTING-CN.md) for the Chinese version.

## Status

The domain, infrastructure, application and stdio JSON-RPC interface layers are
implemented. The HTTP API and CLI integration against this core are not yet
implemented.
