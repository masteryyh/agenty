# agenty-core

[English](README.md) | [简体中文](README-CN.md)

The core runtime for Agenty. It is designed around a local-first storage model
(filesystem + SQLite) and a Domain-Driven Design (DDD) domain layer.

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
| Core log | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` | Structured text diagnostics (`core.jsonl` in JSONL mode) |

A session's messages and rounds are never stored in SQLite; the `sessions` table is a
summary projection that can be rebuilt by replaying the JSONL transcript. Its current
configuration projection includes the selected model, its `context_window`, and the
reasoning effort.

## Domain layer

The domain layer is split by bounded context. Aggregates reference each other only by
identity (UUIDv7 for the conversation family, kebab-case slugs for agents, providers,
and models).

```
pkg/domain/
├── shared/        Shared kernel: Slug, ModelRef, ReasoningEffort, Metadata, Event, ID
├── conversation/  Session aggregate (Session → Round → Message), content blocks, events
├── agent/         Agent aggregate
└── catalog/       Provider aggregate (Provider → Model)
```

The conversation transcript is event-sourced: each JSONL line is a domain event
(`session_started`, `session_model_set`, `session_reasoning_effort_set`,
`session_cwd_set`, `round_started`, `message_appended`, `round_ended`, ...), and a
`Session` aggregate can be reconstructed with `conversation.ReplaySession`. A Session
holds the current configuration for future rounds, while `RoundStarted` snapshots the
model, context window, reasoning effort, and working directory used by that round.

### Reasoning effort

Agenty exposes exactly six provider-independent reasoning effort levels: `off`, `low`,
`medium`, `high`, `xhigh`, and `max`. A model stores a `reasoningEffortMapping` object
whose keys are provider-native effort names and whose values are Agenty effort levels:

```json
{
  "reasoningEffortMapping": {
    "none": "off",
    "minimal": "low",
    "low": "low",
    "medium": "medium",
    "high": "high"
  }
}
```

The mapping allows multiple native efforts to normalize to the same Agenty effort.
A model whose mapping has no enabled effort does not support reasoning. Only the six
Agenty levels above are valid mapping values; native effort names are provider-specific.

## Agent-loop runtime

`pkg/agentloop/` is the dedicated Agent runtime module. It owns the provider-neutral
model-calling contract, tool contract, JSON Schema, thread-safe tool registry, and the
`Engine` that manages multiple sessions. Different sessions can run concurrently, one
session permits one active round, and `Engine` owns cancellation and shutdown for all
active rounds.

Each loop resolves the Agent system prompt, rebuilds the effective conversation context,
converts it through the selected provider adapter, invokes the LLM, persists the
assistant response, and repeats when tool calls are returned. Every model invocation uses
the global `8192` output-token limit; the legacy per-model field is ignored and retained
only for wire compatibility. Automatic compaction runs when the estimated context reaches
`contextWindow * 90%`. `/compact` triggers the same flow
manually. Compaction stores only the generated summary and compaction audit data in a
`session_compacted` event. During replay and request construction, the effective model
context is rebuilt from the transcript as up to three recent user messages, the summary,
metadata, and up to five recent assistant messages; the original JSONL transcript remains
unchanged. Reasoning and unresolved tool-use blocks are omitted from retained messages.
The compaction request keeps the existing system, message, and tool prefix intact, appends
only an in-memory user instruction, and keeps any compaction tool calls and results in an
ephemeral buffer. Switching to a model whose 90% context threshold is reached first
compacts with the current model, trims retained context to fit the target when necessary,
then persists the model change. The loop currently permits at most 20 LLM/tool
iterations. The shared registry implements the `ToolRuntime` port, executes one tool
batch concurrently, and returns results in call order. `pkg/agentloop/builtin/` provides
the production filesystem tools `read_file`, `write_file`, `patch_file`, `delete_file`,
`grep`, `glob`, and `ls`; `cmd/main.go` registers them explicitly. Relative paths resolve
from the round's captured session working directory, while absolute paths remain valid.

## Infrastructure layer

The infrastructure layer (`pkg/infra/`) implements the domain repositories using the
filesystem + SQLite storage model.

```
pkg/infra/
├── config/             Load config file + env overrides into a merged singleton; resolve data-dir paths
├── initialize/         OpenRepositories: one-call setup of all stores
├── llm/                Provider SDK adapters implementing the agentloop caller contract
├── logging/            slog setup, environment parsing, and daily log path
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
- `InitializeService` — first-run state and completion validation; provider/model/agent data
  is written through their regular services.
- `SessionService` — session CRUD and configuration mutations
  (`SetTitle`/`SetModel`/`SetReasoningEffort`/`SetCwd`).

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
produce a single array response. Diagnostics go to the core log file so stdout
stays a clean JSON-RPC stream. The server shuts down on stdin EOF, SIGINT or
SIGTERM. stderr is reserved for failures that prevent the log file itself from
being initialized or closed.

### Logging

`agenty-core` uses the standard library `slog` package. Logs are appended to the
file for the process start date under
`~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log`. When `AGENTY_DATA_DIR` is set, the
`logs` directory moves under that data directory as well.

`AGENTY_LOG_LEVEL` / `logging.level` accept `debug`, `info`, `warn`, or `error`
and default to `info`. `AGENTY_LOG_FORMAT` / `logging.format` accept `text` or
`jsonl` and default to `text`. Both can be set in the `logging` section of the
config file or via the `AGENTY_LOG_LEVEL` / `AGENTY_LOG_FORMAT` environment
variables; environment values take precedence over the file when set, so they
serve as temporary overrides without editing the file, while an empty or
whitespace-only environment value falls back to the file value. JSONL mode uses
`slog.JSONHandler` and writes one JSON object per line to `core.jsonl`; text
mode uses `slog.TextHandler`. Values are case-insensitive, surrounding
whitespace is ignored, and an unsupported value fails startup.

The config file and environment are merged once into a process-wide
`config.Manager` singleton during startup; `logging`, `initialize`, and other
modules read from this single source instead of parsing environment variables
or paths independently.

An inbound line exceeding 64 MiB is not fatal: the server discards it, replies
with `-32003` message too large (`id: null`, `data.maxLineBytes`), and keeps
serving. Because the discarded line has no parseable `id`, a sender that gets
`-32003` must stop pipelining and resend the payload via the chunked upload
protocol below.

Methods follow a `resource.action` naming:

| Group | Methods |
| --- | --- |
| Initialize | `initialize.already`, `initialize.complete` |
| Agent | `agent.create`, `agent.get`, `agent.list`, `agent.update`, `agent.delete` |
| Provider | `provider.create`, `provider.get`, `provider.list`, `provider.update`, `provider.delete`, `provider.addModel`, `provider.removeModel` |
| Session | `session.create`, `session.get`, `session.list`, `session.delete`, `session.setTitle`, `session.setModel`, `session.setReasoningEffort`, `session.setCwd`, `session.start`, `session.compact`, `session.stop` |
| Chunk | `chunk.begin`, `chunk.part`, `chunk.commit`, `chunk.abort` |

`session.start` accepts `{id, content}` and returns the persisted round's identifiers
and `running` status immediately; the engine continues the full agent turn
asynchronously. While it runs, core writes `session.event` JSON-RPC notifications with
`round_started`, `message_appended`, `model_stream`, and `round_ended` event types.
Every event carries `sessionId`, `roundId`, and a per-round monotonically increasing
`sequence`; model events also carry the provider-neutral stream event and loop
`iteration`. A notification may be written before the `session.start` response because
the round starts concurrently, so clients must subscribe before issuing the request and
route notifications independently from responses. `round_ended` carries the terminal
`completed`, `failed`, or `cancelled` status, token usage, and an optional error.

`session.stop` accepts `{id}` and requests cancellation. Starting a second round for the
same session, or deleting that session while it is running, returns `already exists`.
Different sessions can run in parallel.

`session.compact` accepts `{id}` and performs a temporary summarization request using the
current conversation plus a user-only compaction instruction. It emits
`session.compaction` notifications with `started`, `completed`, or `failed` states, and
persists a `session_compacted` event containing the generated summary without changing the
public transcript projection. Retained user, metadata, and assistant messages are derived
from the transcript during replay.

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
build tag. The same tag enables optional live LLM SDK tests. They read
`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, and `GEMINI_API_KEY` from the environment
and report a skip, rather than a failure, when a key is absent.

The `test/e2e` package builds `cmd` once, launches the real binary over stdio, and gives
each parallel test process its own `AGENTY_DATA_DIR`. It covers public Agent,
Provider/Model, Session, agent-loop start/stop and parallel execution, JSON-RPC,
chunking, startup, restart persistence, and process isolation contracts without
accessing the user's data directory.

All filesystem and SQLite tests use per-test temporary directories. Tests that
change `AGENTY_DATA_DIR` are not parallelized because environment variables are
process-global.

See [TESTING.md](./TESTING.md) for the full testing strategy and command guide, or
[TESTING-CN.md](./TESTING-CN.md) for the Chinese version.

## Status

The domain, agent-loop runtime, infrastructure, application and stdio JSON-RPC interface
layers are implemented. Infrastructure also provides unified non-streaming and streaming SDK
callers for OpenAI Responses, OpenAI Chat Completions, Anthropic Messages, and Google
GenAI. The execution engine currently uses the non-streaming caller; streaming agent
turn delivery, command and todo tools, the HTTP API, and CLI integration against this
core are not yet implemented.
