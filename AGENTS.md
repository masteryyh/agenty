# AGENTS.md

## Project overview

Agenty is a local-first AI agent application organized as a pnpm + Turborepo monorepo.
The active product path has three workspaces:

- `packages/agenty-core`: Go 1.26 core process and stdio JSON-RPC 2.0 server.
- `packages/agenty-cli`: Bun/TypeScript/React OpenTUI client.
- `packages/agenty-bootstrap`: Rust self-extracting launcher.

The CLI starts core as a child process and communicates only through NDJSON messages on
stdin/stdout. There is no HTTP or remote-client compatibility layer.
Skills, MCP, memory, compaction, and other capabilities that core has not implemented
must remain hidden or empty in the CLI.

## Core structure and contracts

- `cmd/main.go`: opens repositories, registers built-in tools and RPC adapters, and
  serves stdio until EOF or process cancellation.
- `pkg/domain`: provider-neutral agent, catalog, conversation, and shared domain types.
- `pkg/application`: agent, provider, initialization, and session use-case services.
- `pkg/agentloop`: concurrent session engine, provider streaming contract, tool runtime,
  and built-in filesystem tools.
- `pkg/infra/config`: merged config and data-path manager.
- `pkg/infra/storage`: filesystem source-of-truth repositories plus SQLite session index.
- `pkg/infra/llm`: provider adapters.
- `pkg/infra/rpc`: NDJSON JSON-RPC server, notification writer, chunking, and adapters.

The setup contract is `initialize.already`, then the regular `provider.create`,
`provider.addModel`, and `agent.create` methods, followed by `initialize.complete`.
Completion is valid only when the selected agent, provider, and non-embedding model exist
and the agent's default model matches that exact provider/model reference.

After `session.start`, core continuously writes `session.event` notifications. Events are
scoped by `sessionId` and `roundId`, carry a per-round monotonically increasing
`sequence`, and include round lifecycle, persisted messages, provider-neutral model
stream events, and the terminal round status. A notification may precede the
`session.start` response; clients must subscribe before sending the request and route
notifications independently from responses. Responses and notifications share one
write lock so NDJSON records cannot interleave.

Core data is local-first:

- Config: `~/.agenty/config.json`
- Sessions: append-only JSONL under `~/.agenty/sessions/`
- Session projection: `~/.agenty/agenty.sqlite`
- Providers/models: `~/.agenty/providers/`
- Agents: `~/.agenty/agents/`
- Logs: `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log`

`AGENTY_DATA_DIR` overrides the data root. `AGENTY_LOG_LEVEL` and
`AGENTY_LOG_FORMAT` override file logging settings.

## CLI structure and contracts

- `src/core/rpc.ts`: request correlation, NDJSON framing, notification routing, and
  transport shutdown.
- `src/localCore.ts`: resolves and owns the child core process. Resolution order is
  `AGENTY_CORE_BIN`, repository core build, then `~/.agenty/bin/core[.exe]`.
- `src/api`: core-native IPC types and the typed client projection used by UI/direct CLI.
- `src/state/store.ts`: initialization, session lifecycle, optimistic user messages, and
  continuous `session.event` consumption.
- `src/components`: OpenTUI screens and overlays.
- `src/cli`: direct agent/provider/model/init commands.

CLI state must preserve the core contract rather than recreate backend business rules.
Event consumers must tolerate notifications arriving before the start response, filter
by session, detect sequence gaps, and converge to the persisted session after the
terminal event. Unsupported core features must not leave callable commands or visible
overlays.

OpenTUI includes native libraries. `packages/agenty-cli/scripts/build.ts` maps `OS` and
`ARCH` to explicit Bun compile targets; Linux also uses `OPENTUI_LIBC`. Run the TUI
directly in the user's terminal. Do not run it as a Turbo task because Turbo pipes stdio
and breaks terminal capability handshakes.

## Bootstrap and build orchestration

The bootstrap artifact layout is:

`[bootstrap stub][xz CLI][xz core][108-byte footer]`

The footer stores offsets, lengths, and SHA3-256 digests of decompressed payloads.
`src/lib.rs` and `scripts/footer.ts` are one wire contract; changing the layout requires
updating both golden tests and incrementing `FORMAT_VERSION`. Compression uses
`@napi-rs/lzma`; Rust decompression uses statically linked vendored liblzma. Code signing
must happen after payload packing.

pnpm owns workspace resolution, Turborepo owns build ordering/caching, Bun builds the
CLI and packs payloads, Go builds core, and Cargo builds the launcher. Do not add an npm
`workspaces` field. The dependency graph builds core, then CLI, then bootstrap.

Root `.env` is the single `AGENTY_VERSION` source and stays ignored; only
`.env.example` is committed. Release CI passes target-specific `GOOS`, `GOARCH`, `CC`,
`CXX`, `PACKAGE_DIR`, `BIN_NAME`, `OS`, `ARCH`, and `OPENTUI_LIBC` inputs.

Important commands:

- `pnpm build`, `pnpm test`, `pnpm clean`
- `pnpm core:build`, `core:test`, `core:test:integration`, `core:test:e2e`,
  `core:test:e2e:race`, `core:test:race`, `core:test:repeat`, `core:tidyup`
- `pnpm cli:build`, `cli:dev`, `cli:typecheck`
- `pnpm bootstrap:build`, `bootstrap:test`

For Go tests, reuse `GOCACHE=/private/tmp/agenty-go-cache` when sandbox restrictions make
the default cache unavailable.

## Go conventions

- Use Go 1.26 syntax, standard formatting, `any`, built-in `min`/`max`, and
  `strings.SplitSeq` where appropriate.
- Import `github.com/bytedance/sonic` aliased as `json` outside the dependency-free RPC
  wire layer, which intentionally uses `encoding/json` for `RawMessage` support.
- JSON fields use lowerCamelCase.
- Blocking operations accept `context.Context` first; use context-aware `slog` methods.
- Wrap errors with `%w`, keep domain/application validation structured, and avoid panic
  for expected failures.
- Persistent writes use explicit repositories and event-sourced session mutations; do
  not treat SQLite as the transcript source of truth.
- Built-in tool contracts live in `pkg/agentloop`; implementations live in
  `pkg/agentloop/builtin`. Tool names and input argument fields use `snake_case`;
  persisted and RPC JSON fields retain lowerCamelCase.
- User-facing product text is English unless a localized copy is explicitly required.

## Change and verification discipline

- Preserve unrelated worktree changes and keep edits scoped to the confirmed feature.
- When changing an IPC field, trace the producer, wire type, client projection, store,
  UI consumer, and tests together.
- Cover event ordering, notification-before-response, terminal failure/cancellation,
  multi-session isolation, and transport framing where relevant.
- Report focused unit/integration/E2E/build checks separately from unperformed real TUI,
  network-provider, release-signing, or platform-matrix validation.

## Response marker

Respond to the user with a meow in the user's preferred language after the message, so
the user knows this file was loaded.
