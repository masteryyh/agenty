# AGENTS.md

## Project overview

Agenty is a local-first pnpm + Turborepo monorepo. The active product path has four workspaces:

- packages/agenty-core: Go 1.26 core, exposed to its local client through HTTP/2 over a Unix domain socket or Windows named pipe.
- packages/agenty-cli: Bun/TypeScript/React OpenTUI client.
- packages/file-editor: Rust transactional filesystem helper for V4A patches and text-editor operations.
- packages/agenty-bootstrap: Rust self-extracting supervisor and launcher.

The optional agenty-inspector workspace is a read-only local web debugger. It uses core's config path resolution and transcript reader/decoder/domain replay. It must not initialize or write the inspected data directory.

The bootstrap owns core process startup and shutdown. The CLI connects only with the bootstrap-provided IPC environment; it must not spawn core or fall back to stdio, HTTP/1, or fetch. Run the TUI directly in the user's terminal. Do not run it as a Turbo task because Turbo pipes stdio and breaks terminal capability handshakes.

## Core structure and contracts

- cmd/main.go: resolves and locks the canonical data directory, initializes repositories and services, then serves local HTTP/2.
- pkg/domain: provider-neutral execution, catalog, conversation, and shared domain types.
- pkg/application: provider, initialization, and session use-case services.
- pkg/infra/modelcall: provider-neutral model-call contract and protocol adapters.
- pkg/infra/agentloop: atomic model/tool loop and event hook bridge.
- pkg/infra/session: concurrent session host, persisted round lifecycle, event dispatch, and shutdown.
- pkg/infra/config: merged config and data-path manager.
- pkg/infra/storage: filesystem source-of-truth repositories, SQLite session index, and persistence middleware.
- pkg/api/routes: Gin REST route registration and handlers, HTTP/2 stream transport, and JSON error responses.
- pkg/api/middlewares: shared HTTP request logging and panic recovery middleware.
- pkg/infra/httpapi: event broker, replay, snapshots, topic barriers, and session event projection.
- pkg/infra/instance: canonical data-directory locking, per-directory IPC address, and runtime metadata.

The API uses lowerCamelCase JSON. CRUD routes are ordinary HTTP resources under /v1: initialization, providers and their models, sessions and rounds, skills, MCP servers, and tool-approval decisions. Long-running round creation returns 202 Accepted with its round ID. Cancellation names that round ID so a stale request cannot cancel a later round. Errors use an HTTP status and a stable error code.

POST /v1/stream is a bidirectional HTTP/2 stream. The request body carries newline-delimited subscribe and unsubscribe commands; the response carries newline-delimited event frames. This framing is only for the stream, not a request/response RPC tunnel. Clients subscribe before starting work. Topic sequence numbers are shared by all consumers and continue across rounds while the core lives. Cursors combine streamId and sequence. Replay is bounded; expired or foreign cursors receive a snapshot and a consistent cursor before live frames. A core restart creates a new streamId and clients recover from the current snapshot.

Persist session changes before publishing their events. Keep topic sequence assignment, replay recording, and fanout ordered. Each consumer has a bounded queue; disconnect a slow consumer so it can replay or resync. Consumer disconnect must not cancel an accepted round. Same-session execution is serialized; different sessions may run concurrently.

The bootstrap sets exactly these core/client environment variables: AGENTY_TRANSPORT (uds or named_pipe), AGENTY_CORE_ADDR, AGENTY_DATA_DIR, and AGENTY_IPC_VERSION. The canonical data directory is the ownership boundary: the same directory has one core, and distinct directories may run in parallel. A second bootstrap may attach to a compatible existing core; its CLI does not own that core. Runtime metadata is only a discovery hint; verify the live handshake. Do not add instance-ID environment variables or alternate IPC aliases.

Data is local-first:

- Config: AGENTY_DATA_DIR/config.json, defaulting to ~/.agenty/config.json.
- Sessions: append-only JSONL under AGENTY_DATA_DIR/sessions.
- Session projection: AGENTY_DATA_DIR/agenty.sqlite.
- Providers: AGENTY_DATA_DIR/providers.
- Logs: AGENTY_DATA_DIR/logs.
- Runtime lock and discovery metadata: AGENTY_DATA_DIR/runtime.

## CLI structure and contracts

- src/core/http2.ts: bootstrap-environment validation, HTTP/2 requests, full-duplex stream, and core identity handshake.
- src/api: typed REST client and DTOs.
- src/state/store.ts: connection/session lifetime subscriptions, replay/resync, optimistic user messages, and round lifecycle.
- src/components: OpenTUI screens and overlays.
- src/cli: direct provider/model/init commands using the same core client.

Subscribe for the connection or session lifetime, not only while sending a message. Handle events that arrive before a round-start response, filter by session, deduplicate by topic cursor, resync after gaps, and converge to persisted state after terminal events. Stale connection callbacks must not mutate a newer connection. Exiting a CLI disconnects only that client; only an explicit Stop action cancels work.

CLI startup requires the bootstrap IPC environment and must report a clear error when it is missing or incompatible. Do not search for or launch core in the CLI. Pass the client's current working directory when creating a session.

Unsupported core features must not leave callable commands or visible overlays. MCP server stdio transport remains a valid external-tool capability and is unrelated to the core IPC transport.

## Bootstrap and build orchestration

The bootstrap artifact layout is:

[bootstrap stub][xz CLI][xz core][xz fileedit][156-byte footer]

The footer stores offsets, lengths, and SHA3-256 digests for the three decompressed payloads. src/lib.rs and scripts/footer.ts are one wire contract; a layout change requires updating golden tests and incrementing FORMAT_VERSION. Compression uses @napi-rs/lzma; Rust decompression uses statically linked vendored liblzma. Code signing happens after payload packing.

The bootstrap extracts payloads into version-addressed paths derived from all three payload digests, prepares fileedit on PATH, resolves --data-dir, starts or attaches to core, verifies HTTP/2 protocol and data-directory identity, then launches the CLI with real terminal stdio. An owning bootstrap gracefully stops its child core when its CLI exits and kills remaining children if the bootstrap is terminated. An attaching bootstrap only manages its own CLI.

pnpm owns workspace resolution, Turborepo owns build ordering/caching, Bun builds the CLI and packs payloads, Go builds core, and Cargo builds the patch helper and launcher. Do not add an npm workspaces field. The dependency graph builds the patch helper before core, then CLI and bootstrap.

Builds resolve AGENTY_VERSION from the process environment first, then the ignored root .env, and otherwise use dev. Only .env.example is committed. Release CI passes target-specific GOOS, GOARCH, CC, CXX, PACKAGE_DIR, BIN_NAME, OS, ARCH, and OPENTUI_LIBC inputs.

Important commands:

- pnpm run update, pnpm build, pnpm test, pnpm clean, pnpm deepclean, pnpm tidyup
- pnpm core:build, core:test, core:test:integration, core:test:e2e, core:test:e2e:race, core:test:race, core:test:repeat, core:tidyup
- pnpm cli:build, cli:dev, cli:typecheck
- pnpm bootstrap:build, bootstrap:test

pnpm run update updates pnpm, Go, and Cargo dependencies across the workspace. pnpm clean removes build outputs; pnpm deepclean also removes local caches, dependency stores, node_modules, and other generated temporary files. pnpm tidyup runs go fmt ./..., go vet ./..., and go mod tidy in every Go module.

## Go conventions

- Use Go 1.26 syntax, standard formatting, any, built-in min/max, and strings.SplitSeq where appropriate.
- Import github.com/bytedance/sonic aliased as json in business and storage code. HTTP wire DTOs may use encoding/json when they need RawMessage or standard wire behavior.
- Blocking operations accept context.Context first; use context-aware slog methods.
- Wrap errors with %w, keep domain/application validation structured, and avoid panic for expected failures.
- Persistent writes use explicit repositories and event-sourced session mutations; do not treat SQLite as the transcript source of truth.
- Built-in tool contracts live in pkg/infra/modelcall; implementations live in pkg/infra/tools/builtin. Tool names and input argument fields use snake_case; persisted and API JSON fields retain lowerCamelCase.

## Change and verification discipline

- Preserve unrelated worktree changes and keep edits scoped to the confirmed feature.
- When changing IPC or event fields, trace the producer, wire type, client projection, store, UI consumer, and tests together.
- Cover event ordering, notification-before-start-response, terminal failure/cancellation, multi-session isolation, replay gaps, resync, and transport framing where relevant.
- Report focused unit/integration/E2E/build checks separately from unperformed real TUI, network-provider, release-signing, or platform-matrix validation.

## Response marker

Respond to the user with a meow in the user's preferred language after the message, so they know this file was loaded.
