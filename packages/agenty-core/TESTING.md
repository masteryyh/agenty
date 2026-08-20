# Testing agenty-core

This document describes the current agenty-core test suite and how to run it. For the
Chinese version, see [TESTING-CN.md](./TESTING-CN.md).

## §1. Test scope

| Area | Environment | Covered behavior | Default suite |
| --- | --- | --- | --- |
| Domain | In-memory values | Aggregate invariants, Session transitions and replay, event and content serialization, Provider model lifecycle, code and reasoning effort mapping validation | Yes |
| Application | In-memory repository fakes | Agent, Provider, and Session use cases; execution-loop completion, tool continuation, per-model token limits, multi-session concurrency, cancellation, shutdown, validation, error mapping, and pending-event lifecycle | Yes |
| Built-in tools | `t.TempDir()` and real filesystem operations | Registration, relative path resolution, ranged reads, create/overwrite, exact patching, safe single-file deletion, regular-expression search, recursive globbing, directory listing, output limits, and error paths | Yes |
| RPC | Buffers, fake handlers, and synthetic time | JSON-RPC/NDJSON framing, notifications, batches, invalid requests, line limits, chunk assembly, and cleanup | Yes |
| Config, logging, and storage | `t.TempDir()`, real files, and local SQLite | Config file + env override merging, singleton Manager, log level/format/path selection, JSON repositories, append-only transcripts, SQLite projections, and schema initialization | Yes |
| Complete wiring | Isolated filesystem and SQLite state | Repository initialization and RPC-to-application-to-storage flows, including asynchronous session start/stop | With `integration` |
| Executable E2E | Real `cmd` subprocesses, isolated data directories, a typed IPC client, local provider fixtures, and optional live upstreams | Complete client journeys, all 28 public RPC methods, ordered session event notifications, four provider protocols, built-in tool definitions, multi-turn conversations, completion/failure/cancellation, same-channel concurrency, shutdown during execution, restart persistence, and stdio boundaries | With `e2e` |

The `integration` build tag currently enables:

- `pkg/infra/initialize/initialize_test.go` for complete repository setup and
  lifecycle.
- `pkg/infra/rpc/adapter/adapter_test.go` for full RPC adapter flows, including
  chunked input.

The `e2e` build tag enables `test/e2e`. `TestMain` builds the core binary once; every
test starts its own process with a unique `AGENTY_DATA_DIR`. The typed test client uses
only the public NDJSON protocol, supports concurrent request-ID routing, notifications,
batches, and chunks, and does not import core implementation packages.
`blackbox_test.go` continuously enforces this dependency boundary.

The suite intentionally skips pure DTOs, trivial struct construction, thin getters,
and constructors that only assign fields. This includes `Agent.New`, `NewID`,
`ModelRef.String`, and `TokenUsage.Add`. Command wiring and process-terminating signal
paths are also outside the unit-test scope.

## §2. Test environment

- Go 1.26 or newer is required.
- CGO and a working C compiler are required by `github.com/mattn/go-sqlite3`.
- Filesystem and SQLite tests use per-test temporary directories and do not access the
  user's `~/.agenty` directory.
- Application tests use independent in-memory repository fakes.
- Tests that set `AGENTY_DATA_DIR`, `AGENTY_LOG_LEVEL`, or `AGENTY_LOG_FORMAT` are not
  parallel because environment variables are process-global.
- E2E tests set the data directory on each child process and clear the logging
  environment variables so the child is driven by its config file (seeded with
  info/text defaults). They do not mutate the test runner's environment, so
  independent workflows use `t.Parallel()` safely and write logs only inside
  their isolated data directory.
- Agent-loop E2E tests use local `httptest` servers to emulate OpenAI Responses,
  OpenAI Chat Completions, Anthropic Messages, and Google GenAI. Their environment
  must allow loopback port binding; a sandbox that rejects `listen` must rerun the
  same command in an allowed environment.
- `TestLiveProviderConversationsThroughIPC` uses the same typed client and a real core
  subprocess for optional live conversations. Each provider checks its API key
  independently. A missing or whitespace-only key skips only that subtest; configured
  providers still run. An invalid configured key fails normally instead of being
  treated as absent.
- Chunk expiration tests use `testing/synctest` instead of real-time waits.

Run Go commands from `packages/agenty-core/`. The module's pnpm commands can be run
there directly. From the repository root, use the corresponding `pnpm core:*` command.

## §3. Running tests

| Module command | Root command | Purpose |
| --- | --- | --- |
| `pnpm test` | `pnpm core:test` | All tests without `integration` or `e2e` build tags |
| `pnpm test:integration` | `pnpm core:test:integration` | Default suite plus integration-tagged tests |
| `pnpm test:e2e` | `pnpm core:test:e2e` | Real-binary E2E tests with up to eight parallel workflows |
| `pnpm test:e2e:race` | `pnpm core:test:e2e:race` | Race-instrumented E2E harness and core binary |
| `pnpm test:race` | `pnpm core:test:race` | Default suite with the race detector and no result-cache reuse |
| `pnpm test:repeat` | `pnpm core:test:repeat` | Ten shuffled runs for isolation checks |

End-to-end tests use the `e2e` build tag so `pnpm core:test` remains the complete fast
suite without complex integration or process environments.

The corresponding Go commands are:

```sh
go test ./...
go test -tags=integration ./...
go test -tags=e2e -count=1 -parallel=8 ./test/e2e
go test -race -tags=e2e -count=1 -parallel=4 ./test/e2e
go test -race -count=1 ./...
go test -shuffle=on -count=10 ./...
```

The live LLM integration and optional live E2E cases use these environment variables:

- `OPENAI_API_KEY`, optionally `OPENAI_BASE_URL`, `OPENAI_RESPONSES_MODEL`, and
  `OPENAI_CHAT_MODEL`.
- `ANTHROPIC_API_KEY`, optionally `ANTHROPIC_BASE_URL` and `ANTHROPIC_MODEL`.
- `GEMINI_API_KEY`, optionally `GEMINI_BASE_URL` and `GEMINI_MODEL`.

Each integration provider subtest calls both `Invoke` and `Stream`; live E2E runs one
real non-streaming conversation through stdio IPC. Both skip with a visible `t.Skip`
message when the corresponding API key is missing. Request/response conversion tests
and local-provider-fixture E2E remain offline and never require credentials.

Run a package or one test while developing:

```sh
go test ./pkg/domain/conversation
go test ./pkg/domain/conversation -run '^TestSessionLifecycleAndReplay$' -count=1
```

Run integration tests with the race detector when changing cross-layer behavior:

```sh
go test -race -tags=integration -count=1 ./...
```

If the default Go cache is not writable in a sandbox, use a writable cache:

```sh
GOCACHE=/private/tmp/agenty-core-go-cache go test ./...
```

Generate a coverage report with:

```sh
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out
```

## §4. Current status and boundaries

The default suite snapshot verified on 2026-07-22 has 70.1% statement coverage.
`pkg/domain/conversation` is at 92.8%, `pkg/infra/rpc` at 91.8%, and
`pkg/application` at 76.4%. Coverage is reported as a snapshot because intentionally
untested construction and wiring code lowers the module total.

Storage/RPC integration tests and all E2E tests use local files and SQLite. Optional
LLM integration and `TestLiveProviderConversationsThroughIPC` access external services
only when their corresponding provider API key is present; all other E2E scenarios use
local provider fixtures. E2E cases focus on observable process contracts. Exhaustive
parser permutations, the physical 64 MiB line limit, and chunk assembler validation
remain in the faster RPC tests instead of being duplicated with large subprocess
payloads.

The E2E system treats core as a black box composed of stdin, stdout, stderr, exit
status, and public provider HTTP requests. A complete typed-client journey creates
and updates Agents, Providers/Models, and Sessions, continues a multi-turn conversation
across a process restart, and queries persisted behavior through IPC without asserting
SQLite, JSONL, or repository layout. Provider fixtures cover OpenAI Responses, OpenAI
Chat Completions, Anthropic Messages, and Google GenAI. The scenarios verify the
8,192 output-token clamp, upstream failure, concurrent sessions over one IPC client,
duplicate-start and running-delete rejection, stop-driven cancellation, and recovery
after the process exits during execution.

The journey exercises all 28 current public methods: 2 Initialize, 5 Agent, 7 Provider,
10 Session, and 4 Chunk methods. Session events, batches, exact request IDs, malformed-JSON
recovery, a final line without a newline, stdin EOF, and startup failure remain
process-level protocol scenarios. Exhaustive parser and invalid-chunk permutations
remain in the lower-level RPC suite.

Two implementation boundaries affect the tests:

- `ConversationRepository.Save` has no cross-storage rollback if appending JSONL
  succeeds but updating the SQLite projection fails.
- After `Server.Serve` is canceled, a goroutine blocked on input exits only when its
  underlying reader is closed.
