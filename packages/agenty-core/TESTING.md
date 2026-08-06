# Testing agenty-core

This document describes the current agenty-core test suite and how to run it. For the
Chinese version, see [TESTING-CN.md](./TESTING-CN.md).

## §1. Test scope

| Area | Environment | Covered behavior | Default suite |
| --- | --- | --- | --- |
| Domain | In-memory values | Aggregate invariants, Session transitions and replay, event and content serialization, Provider model lifecycle, slug and reasoning effort mapping validation | Yes |
| Application | In-memory repository fakes | Agent, Provider, and Session use cases; execution-loop completion, tool continuation, per-model token limits, multi-session concurrency, cancellation, shutdown, validation, error mapping, and pending-event lifecycle | Yes |
| RPC | Buffers, fake handlers, and synthetic time | JSON-RPC/NDJSON framing, notifications, batches, invalid requests, line limits, chunk assembly, and cleanup | Yes |
| Config, logging, and storage | `t.TempDir()`, real files, and local SQLite | Config file + env override merging, singleton Manager, log level/format/path selection, JSON repositories, append-only transcripts, SQLite projections, and schema initialization | Yes |
| Complete wiring | Isolated filesystem and SQLite state | Repository initialization and RPC-to-application-to-storage flows, including asynchronous session start/stop | With `integration` |
| Executable E2E | Real `cmd` subprocesses with isolated data directories and local HTTP stubs | stdio JSON-RPC business workflows, agent-loop completion/cancellation, parallel sessions, upstream request conversion, startup failure, chunk registration, restart persistence, and process isolation | With `e2e` |

The `integration` build tag currently enables:

- `pkg/infra/initialize/initialize_test.go` for complete repository setup and
  lifecycle.
- `pkg/infra/rpc/adapter/adapter_test.go` for full RPC adapter flows, including
  chunked input.

The `e2e` build tag enables `test/e2e`. `TestMain` builds the core binary once; every
test starts its own process with a unique `AGENTY_DATA_DIR`. The test-side client uses
only the public NDJSON protocol and does not import core implementation packages.

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
  their isolated data directory. Tests that exercise env overrides set those
  variables explicitly on the child.
- Agent-loop E2E tests use local `httptest` HTTP servers as provider endpoints. Their
  environment must allow loopback port binding; a sandbox that rejects `listen` must
  rerun those tests in an allowed environment.
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

The live LLM integration cases use the following environment variables:

- `OPENAI_API_KEY`, optionally `OPENAI_BASE_URL`, `OPENAI_RESPONSES_MODEL`, and
  `OPENAI_CHAT_MODEL`.
- `ANTHROPIC_API_KEY`, optionally `ANTHROPIC_BASE_URL` and `ANTHROPIC_MODEL`.
- `GEMINI_API_KEY`, optionally `GEMINI_BASE_URL` and `GEMINI_MODEL`.

Each provider subtest calls both `Invoke` and `Stream`. A missing API key produces a
visible `t.Skip` message and does not fail the integration suite. Request/response
conversion tests remain in the default offline suite and never require credentials.

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
LLM integration cases are the only tests that access external services, and only when
their provider API key is present. E2E cases focus on observable process contracts.
Exhaustive parser permutations, the physical 64 MiB line limit, and chunk assembler
validation remain in the faster RPC tests instead of being duplicated with large
subprocess payloads.

The execution E2E cases use a local OpenAI Chat Completions-compatible stub, not an
external provider. They verify the 65,536 default output-token clamp, complete message
and usage persistence, concurrent sessions, duplicate-start rejection, stop-driven
terminal cancellation, and the public IPC result shapes.

Two implementation boundaries affect the tests:

- `ConversationRepository.Save` has no cross-storage rollback if appending JSONL
  succeeds but updating the SQLite projection fails.
- After `Server.Serve` is canceled, a goroutine blocked on input exits only when its
  underlying reader is closed.
