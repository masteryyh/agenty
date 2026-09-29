# Testing agenty-core

This guide describes the current HTTP/2 core test suite. For Chinese, see TESTING-CN.md.

## Test scope

| Area | Environment | Coverage | Build tag |
| --- | --- | --- | --- |
| Domain and application | In-memory values and repository fakes | Aggregate invariants, initialization, provider/session use cases, round lifecycle, cancellation, permission decisions, concurrency, and error mapping | Default |
| HTTP responses and stream broker | httptest, local HTTP/2, synthetic time | Response envelopes, arbitrary/null data, encoding failures, full-duplex stream commands, shared topic sequence, replay, cursor expiry, snapshots, slow consumers, and persisted-event handoff | Default |
| Config, logging, storage | Temporary directories and local SQLite | Config/env merging, logging setup, JSONL transcripts, projection rebuilds, and repository persistence | Default |
| Complete wiring | Isolated filesystem and SQLite | Repository initialization and application-to-storage flows | integration |
| Executable E2E | Real core child processes and isolated data directories | REST lifecycle, actual HTTP/2 transport, stream ordering, CRUD journey, provider fixtures, cancellation, concurrency, restart persistence, and closed-stdin behavior | e2e |
| Bun and compiled CLI transport | Windows named pipe plus real Bun and compiled CLI | Bidirectional Node HTTP/2 connection, REST request during an open event stream, and compiled CLI handshake | Windows opt-in |
| Bootstrap lifecycle | Real extracted core/CLI child processes | Same-directory attachment, different-directory parallel owners, cleanup after CLI/core/bootstrap exit | Windows ignored integration |

The e2e test client uses only public HTTP resources, does not import core implementation
packages, and checks REST paths and stable error codes. There is no JSON-RPC dispatcher or
chunk assembler.

## Test environment

- Go 1.26 or newer is required.
- CGO and a C compiler are required by SQLite.
- All tests use temporary data roots; they do not access or modify the user's ~/.agenty.
- Tests that mutate process environment are not parallelized.
- E2E child processes receive their own AGENTY_DATA_DIR and log configuration.
- Provider E2E fixtures bind loopback HTTP ports. A sandbox that blocks loopback listening
  must rerun the same tests in an allowed environment.
- Optional live-provider tests use OPENAI_API_KEY, ANTHROPIC_API_KEY, and GEMINI_API_KEY.
  Missing keys skip the corresponding test; configured but invalid keys fail normally.
- Windows named-pipe transport checks require AGENTY_BUN_BIN and AGENTY_COMPILED_CLI.
  Without both values, the opt-in integration test skips.
- Bootstrap process lifecycle tests are marked ignored because they start and terminate
  real child processes. Select them explicitly in the Windows development environment.
- Local MCP stdio fixtures are external child processes and use isolated test resources.

## Commands

Run Go commands from packages/agenty-core:

- go test ./...
- go test -tags=integration ./...
- go test -tags=e2e -count=1 -parallel=8 ./test/e2e
- go test -race -tags=e2e -count=1 -parallel=4 ./test/e2e
- go test -race -count=1 ./...
- go test -shuffle=on -count=10 ./...

From the repository root, use pnpm core:test, core:test:integration, core:test:e2e,
core:test:e2e:race, core:test:race, or core:test:repeat.

To run the Windows Bun/CLI named-pipe check, set AGENTY_BUN_BIN and AGENTY_COMPILED_CLI,
then run the httpapi package test named
TestBunAndCompiledCLIUseNamedPipeHTTP2Bidirectionally.

Bootstrap unit tests run with cargo test in packages/agenty-bootstrap. CLI validation
uses pnpm cli:typecheck and the CLI package's Bun test suite. The integrated launcher is
built with pnpm bootstrap:build or pnpm build.

## Verification boundaries

The standard Go suites use local fixtures and do not need provider credentials. A passing
transport test proves the named-pipe HTTP/2 connection and full-duplex behavior; it does not
by itself prove a complete provider-backed user journey. The e2e suite exercises the actual
core process with local provider fixtures. Release signing, macOS/Linux process lifecycle,
and real upstream provider availability require their respective environments.
