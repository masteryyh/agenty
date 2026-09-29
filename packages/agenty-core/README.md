# agenty-core

The Go core owns local-first configuration, provider/model catalogs, persistent sessions,
agent execution, tools, skills, and MCP connections. The CLI reaches it through local
HTTP/2; there is no stdio RPC or remote-client compatibility layer.

## Storage model

The filesystem is the source of truth and SQLite is a rebuildable query projection.

| Data | Default location | Role |
| --- | --- | --- |
| Session transcript | ~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl | Append-only domain event log |
| Session index | ~/.agenty/agenty.sqlite | Query-side projection for listing and search |
| Global config | ~/.agenty/config.json | Application defaults |
| Providers | Embedded catalog plus ~/.agenty/providers | Built-ins are read-only; custom providers have JSON files |
| MCP servers | ~/.agenty/mcp | One configuration file per server |
| MCP credentials | ~/.agenty/mcp-auth | OAuth credentials separate from configuration |
| Logs | ~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log | Structured diagnostics |
| Runtime | ~/.agenty/runtime | Ownership lock and discovery metadata |

AGENTY_DATA_DIR selects the root. Core canonicalizes it and holds a directory-specific
process lock before initializing business storage. The same canonical directory has one
core owner; different directories can run at the same time. Runtime metadata is only a
discovery hint and is checked against the live core handshake.

## Architecture

- pkg/domain contains provider-neutral conversation, catalog, MCP, and shared types.
- pkg/application contains initialization, provider, and session use cases.
- pkg/infra/modelcall adapts provider protocols to a stateless model-call interface.
- pkg/infra/agentloop owns the model/tool loop, usage accounting, cancellation, and limit.
- pkg/infra/session coordinates concurrent sessions, persisted rounds, and event hooks.
- pkg/infra/storage implements JSONL repositories, SQLite projections, and persistence hooks.
- pkg/api/routes registers Gin REST routes and HTTP handlers, including the HTTP/2 stream endpoint.
- pkg/api/middlewares provides shared request logging and panic recovery.
- pkg/infra/httpapi provides event replay, snapshots, topic barriers, and session event projection.
- pkg/infra/instance owns data-directory canonicalization, locking, and IPC addressing.
- cmd/main.go assembles repositories, middleware, application services, and the listener.

Session transcript JSONL remains the source of truth. Persist mutations before publishing
their corresponding stream event. SQLite can be rebuilt by replaying the transcript.

## Local API

Core serves HTTP/2 prior knowledge over a Unix-domain socket on macOS/Linux and a
byte-mode named pipe on Windows. The CLI uses Node's HTTP/2 and net APIs. Bootstrap
starts or attaches to core, validates data-directory and protocol identity, then starts
the CLI with the user's real terminal stdio.

Bootstrap provides exactly these IPC variables:

- AGENTY_TRANSPORT: uds or named_pipe.
- AGENTY_CORE_ADDR: the socket path or named pipe address.
- AGENTY_DATA_DIR: canonical data directory.
- AGENTY_IPC_VERSION: IPC compatibility version.

The API contract is v1. GET /v1/system reports the live process identity, canonical data
directory, initialization state, API contract, IPC version, and stream ID. CLI startup
fails clearly if its bootstrap IPC environment is missing or incompatible.

JSON CRUD resources use lowerCamelCase fields. Ordinary responses contain
`{"code":200,"message":"ok","data":...}`: code matches the HTTP status, and data is any JSON
value, including null. Failures preserve the HTTP error status, put the error message in
message, set data to null, and include a stable errorCode such as not_found. CLI and
bootstrap unwrap data; NDJSON stream frames retain their separate format. The route groups are:

| Resource | Routes |
| --- | --- |
| Initialization | GET and POST /v1/initialization |
| Providers and models | /v1/providers and /v1/providers/{code}/models |
| Sessions and rounds | /v1/sessions, session settings, /rounds, /rounds/{roundId}/cancel, and /compact |
| Skills | GET /v1/skills |
| MCP | /v1/mcp and server-specific get/update/logs/enable/reconnect/login/logout routes |
| Tool approvals | POST /v1/tool-approvals/{approvalId}/resolution |
| Core lifecycle | GET /v1/system and POST /v1/system/shutdown |

Creating a round returns 202 Accepted with its round ID while execution continues
asynchronously. Cancel names both the session and round, so a delayed cancel cannot stop a
later round. The session engine permits one active round per session and concurrent rounds
for different sessions. Disconnecting a client does not cancel accepted work.

## Event stream

POST /v1/stream is a true bidirectional HTTP/2 stream. The client keeps the request body
open and sends newline-delimited subscribe or unsubscribe commands; core immediately
flushes response headers and writes newline-delimited frames as events arrive. This
framing is confined to the stream and is not an RPC envelope or JSON tunnel.

Supported topics are mcp and session:<sessionId>. Each topic has a monotonic sequence
shared by every consumer. A cursor combines the opaque streamId and last successfully
applied sequence. Clients ignore duplicates and detect gaps. Subscribing with a valid
cursor replays retained events and atomically joins live delivery. A stale cursor, cursor
from another stream, or cursor ahead of the topic receives a current snapshot and
consistent cursor before live events. Core restart changes streamId; the client then
resynchronizes from persisted state and current runtime state.

Snapshots include the session projection and current execution/approval state. Bounded
replay storage and bounded per-consumer queues keep slow consumers from blocking execution;
a slow consumer is disconnected and can resume or request a fresh snapshot. Round events
include persisted messages, model output, tool and approval changes, compaction state,
and terminal status. Domain round sequence and JSONL sequence remain separate from the
HTTP event topic sequence.

## Capabilities

Providers support CRUD, model discovery, model add/remove, and global initialization.
Sessions support create/list/get/delete, title/model/reasoning/cwd/permission updates,
Codex mode, asynchronous rounds, cancellation, and compaction. Skills are discovered from
the configured data directory and supported user skill directories. MCP supports stdio,
Streamable HTTP, and legacy SSE as external server transports. The CLI's MCP stdio
subprocesses are unrelated to core IPC.

The permission middleware supports ask, auto, and yolo modes. Pending tool approvals are
in memory, the first valid decision wins, and resolved events synchronize all consumers.
A round captures its working directory and current tool snapshot when it starts.

The model/tool loop uses OpenAI Responses, OpenAI Chat Completions, Anthropic Messages,
and Google GenAI adapters. Model output is streamed to subscribers while completed
messages and round lifecycle changes are persisted.

## Development and tests

Run workspace scripts from the repository root:

- pnpm core:build
- pnpm core:test
- pnpm core:test:integration
- pnpm core:test:e2e
- pnpm core:test:e2e:race
- pnpm core:test:race
- pnpm core:test:repeat
- pnpm core:tidyup

Core builds require CGO because the SQLite driver uses cgo. The build defaults
`CGO_ENABLED` to `1` and rejects `CGO_ENABLED=0`. On Windows, install a GCC-compatible
compiler such as MinGW-w64 or LLVM-MinGW; MSVC `cl.exe` cannot build the core. Set `CC`
to that toolchain's `gcc.exe` or `clang.exe` in the current process before building, and
add its `bin` directory to the current `PATH` if the toolchain needs companion binaries:

```powershell
$env:CC = 'C:\path\to\llvm-mingw\bin\clang.exe'
$env:PATH = "C:\path\to\llvm-mingw\bin;$env:PATH"
pnpm core:build
```

`CC` and `CXX` are passed through to Go and included in the Turbo build cache key.

Run Go commands from packages/agenty-core. Default tests cover domain, use cases, HTTP
routes and event broker, middleware, and storage. Integration tests cover full repository
wiring. The e2e build tag launches real core processes in isolated temporary data
directories and tests REST requests, streaming events, provider fixtures, concurrency,
cancellation, restart persistence, and process behavior. Optional live-provider tests
require provider credentials and are skipped when keys are absent.

The Windows HTTP/2 integration test can additionally exercise a real Bun client and a
compiled CLI over a named pipe when AGENTY_BUN_BIN and AGENTY_COMPILED_CLI are set.
Bootstrap lifecycle tests that start and terminate real child processes are ignored by
default and can be selected explicitly in the Windows development environment.

See TESTING.md for the test matrix and TESTING-CN.md for the Chinese guide.
