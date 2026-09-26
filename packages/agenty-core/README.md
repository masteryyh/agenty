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
| Providers | Embedded catalog; custom providers use `~/.agenty/providers/<provider-code>.json` | Built-in metadata/models are read-only; built-in files store only API keys |
| MCP servers | `~/.agenty/mcp/<server-name>.json` | One concise configuration file per server, mode `0600` |
| MCP OAuth credentials | `~/.agenty/mcp-auth/<server-name>.json` | OAuth tokens, mode `0600`, kept outside server config |
| Core log | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` | Structured text diagnostics (`core.jsonl` in JSONL mode) |

A session's messages and rounds are never stored in SQLite; the `sessions` table is a
summary projection that can be rebuilt by replaying the JSONL transcript. Its current
configuration projection includes the selected model, its `context_window`, and the
reasoning effort.

## Domain layer

The domain layer is split by bounded context. Aggregates reference each other only by
identity (UUIDv7 for the conversation family, path-safe codes for providers, and opaque
upstream codes for models).

```
pkg/domain/
├── shared/        Shared kernel: Code, ModelRef, ReasoningEffort, Metadata, Event, ID
├── conversation/  Session aggregate (Session → Round → Message), content blocks, events
├── catalog/       Provider aggregate (Provider → Model)
└── mcp/           MCP server configuration, status, and tool projection
```

The conversation transcript is event-sourced: each JSONL line is a domain event
(`session_started`, `session_model_set`, `session_reasoning_effort_set`,
`session_cwd_set`, `round_started`, `message_appended`, `round_ended`, ...), and a
`Session` aggregate can be reconstructed with `conversation.ReplaySession`. A Session
holds the current configuration for future rounds, while `RoundStarted` snapshots the
model, context window, reasoning effort, and working directory used by that round.

### Reasoning effort

Agenty exposes exactly six provider-independent reasoning effort levels: `off`, `low`,
`medium`, `high`, `xhigh`, and `max`. Reasoning models store supported enabled levels
in a `reasoningEfforts` array:

```json
{
  "reasoningEfforts": ["low", "medium", "high", "xhigh", "max"]
}
```

An explicit empty array identifies a non-reasoning model. Missing upstream capability
data defaults to all five enabled Agenty levels. Provider adapters send the selected level
unchanged; unsupported levels are reported through the normal round error flow.
Provider-specific levels such as `minimal` are not exposed.

## Agent-loop runtime

`pkg/infra/modelcall/` performs one stateless provider-neutral model invocation. It receives
only connection/model configuration (base URL, API type, API key, model code, and
capabilities) plus model-facing context and tools, then returns parsed responses and stream
events. `pkg/infra/agentloop/` owns the atomic model/tool loop and calls `modelcall` directly.
The loop owns continuation, tool dispatch, usage accounting, cancellation, and the iteration
limit. It has no session repository, model catalog, prompt renderer, tool registry, MCP
client, skill scanner, or compaction policy.
`pkg/infra/session` is the host that coordinates multiple sessions, persisted rounds,
resources, event dispatch, and shutdown.

The session host builds a request, invokes the atomic loop, turns assistant responses and
tool results into session messages, and dispatches every runtime event to `OnEvent` consumers.
Custom models use `8192` output tokens when omitted;
built-in models use the exact limit from the embedded catalog. `pkg/infra/prompt` renders the
base system prompt. `pkg/infra/compaction` owns the token estimate, threshold policy, and
ephemeral summary conversation; automatic compaction and `/compact` use the same executor.
Compaction keeps the existing system and message prefix, appends an in-memory user
instruction, clears all tool definitions, and calls the model once through `modelcall.Call`.
A tool-use response fails compaction and is never executed or persisted. The loop currently
permits at most 20 LLM/tool iterations. The production registry in
`pkg/infra/tools` implements the `ToolRuntime` port, executes one tool batch concurrently,
and returns results in call order. `pkg/infra/tools/builtin/` provides `read_file`,
`apply_patch`, `str_replace_based_edit_tool`, `grep`, `glob`, and `ls`; `cmd/main.go` registers
them explicitly. The default file-tool dialect exposes the text editor, whose `view` command
replaces `read_file` and whose writes delegate to the bundled `fileedit text_editor` helper.
The official Anthropic Messages provider receives its server-defined text editor declaration;
compatible providers receive the same contract as a function tool. `/codex-mode` permanently
switches one Responses API session to `read_file` plus free-form `apply_patch`, delegated to
`fileedit apply_patch`; it cannot then switch to another API type. Relative paths resolve from
the round's captured session working directory.

The session host exposes lifecycle and the atomic loop exposes per-call hook ports. The
infrastructure contract in `pkg/infra/middleware` defines a flat `Middleware` structure and
`MiddlewareManager`; the manager collects its non-nil hooks in registration order, compiles
them once, and rejects later registration.
The available phases are `BeforeSessionStart`, `BeforeRound`, `AfterRound`,
`BeforeModelCall`, `AfterModelCall`, `BeforeToolCall`, `AfterToolCall`, and
`AfterSessionStop`, plus `OnEvent`. `BeforeRound` runs before a persisted round is allocated, so middleware
can transform the incoming content, system prompt, tool runtime, or queue hidden context
atomically before the round is written. Hooks may replace their context with a derived
`context.Context`; the manager carries it through the chain and the loop uses it for later
model and tool calls. Lifecycle context types and the compiled lifecycle
bridge live in `pkg/infra/middleware`; the atomic loop only receives the model/tool hook
bridge it needs during execution. AgentLoop and every hook context receive an event emitter;
`OnEvent` is the shared consumer path for their emitted events.

Production middleware lives beside its infrastructure implementation: `infra/skill` scans
skill files and appends the session-level skill catalog to the system prompt, `infra/mcp`
projects the current MCP tool snapshot for each round, `infra/metadata` injects full or
changed session metadata, `infra/compaction` owns automatic compaction policy, and
`infra/tools` owns the mutable dynamic tool registry. `infra/storage` persists pending
session events, and `infra/rpc` projects runtime events into CLI notifications. `cmd/main.go`
registers them in the order Skill, MCP, Metadata, Compaction, Storage, RPC notification, and the
permission middleware (implemented in `infra/hitl`),
passes the compiled hook chains into `infra/session.Engine`; persistence therefore happens
before a client observes the corresponding notification.

`infra/metadata` includes the current session permission mode in the hidden
`<permission-mode>` metadata field. `infra/hitl` intercepts every tool call through
`BeforeToolCall` and reads that session mode. In `ask` mode, a before hook can
provide `Result` to replace execution; the loop merges these results with executed
results in call order and delivers both through `AfterToolCall` and the normal event path. In
`yolo` mode, calls continue without an approval interruption. The mode is stored on `Session`
and can be changed while a round is running; switching to `yolo` releases pending approvals.
Built-in tools supply display-only approval previews alongside their implementation;
previews resolve through the same tool snapshot used for execution. Other tools show
their name and formatted arguments.

## Infrastructure layer

The infrastructure layer (`pkg/infra/`) implements the domain repositories using the
filesystem + SQLite storage model.

```
pkg/infra/
├── config/             Load config file + env overrides into a merged singleton; resolve data-dir paths
├── initialize/         OpenRepositories: one-call setup of all stores
├── catalogdata/        Embedded built-in provider/model JSON
├── modelcall/          Stateless model-call contract, protocol adapters, and response parsing
├── agentloop/          Atomic model/tool loop that invokes modelcall directly
├── logging/            slog setup, environment parsing, and daily log path
├── storage/            Repository implementations + SQLite connection factory
│   ├── db.go           OpenDB/OpenIsolatedDB + sessions schema
│   ├── catalog.go      CatalogRepository (embedded built-ins plus custom provider JSON)
│   ├── conversation.go ConversationRepository (JSONL transcript + SQLite projection)
│   └── middleware.go   Session-persistence event consumer
├── compaction/         Automatic compaction middleware and request threshold policy
├── hitl/               Tool approval middleware and pending decisions
├── mcp/                MCP client registry and stdio/Streamable HTTP/SSE transports
├── middleware/         Middleware contract, hook contexts, and MiddlewareManager
├── metadata/            Session metadata middleware and hidden context messages
├── prompt/             Base system-prompt renderer
├── session/            Session execution host, persisted round lifecycle, and event dispatch
├── skill/              Skill discovery registry and skill middleware
├── tools/              Mutable infrastructure tool registry and round snapshots
└── rpc/                stdio JSON-RPC 2.0 interface layer and notification event consumer
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

- `ProviderService` — provider CRUD plus model sub-resource operations
  (`AddModel`/`RemoveModel`).
- `InitializeService` — first-run state, default model persistence, and completion validation.
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
| Skill | `skill.list` |
| Provider | `provider.create`, `provider.get`, `provider.list`, `provider.listModels`, `provider.update`, `provider.delete`, `provider.addModel`, `provider.removeModel` |
| Session | `session.create`, `session.get`, `session.list`, `session.delete`, `session.setTitle`, `session.setModel`, `session.setReasoningEffort`, `session.setCwd`, `session.setPermissionMode`, `session.start`, `session.compact`, `session.stop`, `session.resolveToolApproval` |
| MCP | `mcp.list`, `mcp.get`, `mcp.logs`, `mcp.create`, `mcp.update`, `mcp.enable`, `mcp.reconnect`, `mcp.login`, `mcp.logout`, `mcp.remove` |
| Chunk | `chunk.begin`, `chunk.part`, `chunk.commit`, `chunk.abort` |

MCP server configurations live in `<AGENTY_DATA_DIR>/mcp/<server-name>.json`; the file name is
the server name. Server names contain only ASCII letters, digits, `_`, and `-`, and are
case-insensitive (`GitHub` and `github` refer to the same server). `stdio` entries use `command`, a JSON string array `args` (one process argument
per element), and optional `env`; `http` and `sse` entries use Streamable HTTP or legacy SSE
`url` and optional `headers`. No `cwd` field is persisted. Values in `env` and `headers`
expand the core process environment at connection time.

The central registry starts enabled servers asynchronously with bounded parallelism. It records
`connecting`, `connected`, `auth-required`, and `error` states and emits `mcp.event` notifications.
Streamable HTTP uses the official SDK OAuth authorization-code handler with dynamic client
registration; `mcp.login` exposes a loopback callback URL to the CLI and stores tokens
under `mcp-auth`, including the OAuth client configuration needed to refresh them. Tools are
registered as `mcp__<server-name>__<tool-name>`; characters outside the provider-safe
ASCII set are replaced with `_` and the exposed name is limited to 64 characters. A session round
captures the connected tool registry at round start, so a connection that completes later is
visible from the next round. `mcp.logs` returns the latest bounded, in-memory diagnostics for a
server, including connection errors and stdio child stderr; sensitive configured values are
redacted. Core closes all active MCP sessions and stdio child processes during shutdown.

`skill.list` returns the discovered skill registry and non-fatal diagnostics. Core scans
`<AGENTY_DATA_DIR>/skills` first (`~/.agenty/skills` by default), then `~/.agents/skills` and
`~/.claude/skills`; an earlier directory name shadows a later one. A skill is advertised from
its `SKILL.md` frontmatter `name` and `description`. A frontmatter name that differs from the
directory name remains available for explicit references but is not included in the automatic
system-prompt catalog and produces a warning.

`provider.list` accepts an optional `{providerCode}`. Without it, core discovers all
configured providers whose catalog is empty in parallel; with it, only that provider is
eligible for discovery. `provider.listModels` accepts `{providerCode}` and exposes the same
core-owned discovery path for direct callers. Successful discovery is cached in the running
core process for 8 hours. It is not written to disk and does not survive a core
restart. Expired entries remain available as stale data while a subsequent list refreshes them.
It maps common `id`/name/token-limit fields, follows provider pagination, defaults missing or
non-positive context/output limits to `256000` and `65536`, and represents missing reasoning
capability as an empty `reasoningEfforts` array. Transactional `fileedit` locks live under
`~/.agenty/locks/`; each lock records its helper PID and full target path.

`session.start` accepts `{id, content}` and returns the persisted round's identifiers
and `running` status immediately; the engine continues the full agent turn
asynchronously. While it runs, core writes `session.event` JSON-RPC notifications with
`round_started`, `message_appended`, `model_stream`, `permission_mode_changed`, `tool_dialect_changed`, and
`round_ended` event types.
Every event carries `sessionId`, `roundId`, and a per-round monotonically increasing
`sequence`; model events also carry the provider-neutral stream event and loop
`iteration`. A notification may be written before the `session.start` response because
the round starts concurrently, so clients must subscribe before issuing the request and
route notifications independently from responses. `round_ended` carries the terminal
`completed`, `failed`, or `cancelled` status, token usage, and an optional error.

`session.stop` accepts `{id}` and requests cancellation. Starting a second round for the
same session, or deleting that session while it is running, returns `already exists`.
Different sessions can run in parallel.

The default `ask` mode requires a separate user decision for every tool call, including
read-only and MCP tools. `session.event` also carries `permission_mode_changed` with the
previous and current mode, and `tool_approval_requested` with
`approval: {approvalId, toolCall, cwd, preview: {title, detail}}`. Respond using
`session.resolveToolApproval` with `{sessionId, roundId, approvalId, decision}`;
`decision` must be `allow` or `deny`. An accepted decision emits
`tool_approval_resolved` with `resolution` containing those same fields. Both events
use the existing round sequence. Clients must handle the next request arriving before
the previous decision's RPC response and clear requests only by matching identity.

Calls in one batch are approved in order in `ask` mode, then only allowed calls execute in
parallel. In `auto` mode every MCP call is sent to the automatic reviewer; built-in calls
that pass deterministic safety checks can proceed directly, and other calls fall back to
the reviewer or a manual approval. In `yolo` mode all calls proceed without approval. Call
`session.setPermissionMode` with `{id, permissionMode: "ask" | "auto" | "yolo"}` to switch a session;
only the latest requested change is kept in memory. It is persisted and takes effect
immediately before the next agent-loop `BeforeModelCall`, including the first call of a
later round. Switching back to the effective mode clears the pending change. Current
approvals still require a decision; switching modes does not release them. Session
responses expose `pendingPermissionMode` while a change is queued. Pending changes
survive round completion or cancellation in the same core process, but not a restart.
A denial produces an error `tool_result` with the original `toolUseId` and text
`The user denied this tool call. The tool was not executed.` The result is persisted
and sent to the next model invocation; denial does not fail the round. Pending approvals
are in memory, consume at most one decision, and expire on cancellation or shutdown.
Duplicate, mismatched or expired decisions are rejected. Approvals are not restored
after restarting core.

The TUI displays the current permission mode and marks queued changes as pending in the
input status line. Unfinished tools in a cancelled round display `Cancelled`, including
when the session is reopened; completed tool results retain their status.
Use Shift+Tab to cycle `ask`, `auto`, and `yolo`. In `ask` mode, the TUI opens a tool approval overlay automatically. It shows built-in tool-specific
content or generic tool arguments in a scrollable preview. Use arrows/Tab to choose,
Enter to confirm, Y to allow once, or N/Esc to deny. Deny is selected initially;
Ctrl+C retains its exit behavior. Submission errors remain visible for retry.

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
$ echo '{"jsonrpc":"2.0","id":1,"method":"provider.list","params":{}}' | go run ./cmd
{"jsonrpc":"2.0","id":1,"result":[...]}
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
pnpm core:tidyup            # go fmt, go vet, and go mod tidy for agenty-core only
pnpm core:clean             # remove agenty-core build artifacts
```

Use `pnpm tidyup` from the repository root to run the same Go maintenance commands in
all Go modules.

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
each parallel test process its own `AGENTY_DATA_DIR`. It covers public Provider/Model,
Session, agent-loop start/stop and parallel execution, JSON-RPC,
chunking, startup, restart persistence, and process isolation contracts without
accessing the user's data directory.

All filesystem and SQLite tests use per-test temporary directories. Tests that
change `AGENTY_DATA_DIR` are not parallelized because environment variables are
process-global.

See [TESTING.md](./TESTING.md) for the full testing strategy and command guide, or
[TESTING-CN.md](./TESTING-CN.md) for the Chinese version.

## Status

The domain, agent-loop runtime, infrastructure, application and stdio JSON-RPC interface
layers are implemented. Infrastructure also provides stateless non-streaming and streaming
model calls for OpenAI Responses, OpenAI Chat Completions, Anthropic Messages, and Google
GenAI. The execution engine invokes `modelcall` directly; streaming agent turn delivery,
command and todo tools, the HTTP API, and CLI integration against this core are not yet
implemented.
