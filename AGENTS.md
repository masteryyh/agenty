# AGENTS.md

## Project Overview

Agenty is an AI agent application written primarily in Go 1.26. The Go binary is an HTTP server, while `agenty-cli` provides local interactive and remote client modes. Core capabilities include chat sessions, model/provider management, tool calling, agentic looping, MCP integration, skills, memory, and searchable knowledge.

The Go backend uses Gin, GORM, SQLite/PostgreSQL, embedded SQL schema files, provider adapters, and a tool registry. The repository is a pnpm + Turborepo monorepo: the Go backend lives in `packages/agenty-runtime/`, a TypeScript terminal client lives in `packages/agenty-cli/`, and a small Rust self-extracting launcher lives in `packages/agenty-bootstrap/`.

## Project Structure

The repository is a pnpm + Turborepo monorepo with three workspaces: `packages/agenty-cli` (TypeScript CLI), `packages/agenty-runtime` (Go backend) and `packages/agenty-bootstrap` (Rust launcher). Go paths below (`cmd/`, `pkg/...`) are relative to `packages/agenty-runtime/`.

- `packages/agenty-runtime/`: Go HTTP backend (Go module `github.com/masteryyh/agenty`). Its package script invokes `go build` directly, consumes ambient `AGENTY_VERSION` and optional Go target/output variables, and produces `bin/agenty` by default. The result is packed into the launcher by `packages/agenty-bootstrap`.
- `cmd/`: Go application entrypoint. `cmd/main.go` delegates to `pkg/cli/cmd`.
- `packages/agenty-cli/`: TypeScript/React OpenTUI terminal UI — the primary Agenty TUI. In local mode it forks the runtime with `--port` on a random port; in remote mode it connects to an existing server.
  - `src/api/`: HTTP client and API DTOs for the CLI.
  - `src/commands/`: Slash command registry and parsing.
  - `src/components/`: OpenTUI React components (chat, overlays, setup wizard).
  - `src/components/ui/`: Shared OpenTUI primitives, with one component per file.
  - `src/tui/`: OpenTUI renderer lifecycle context.
  - `src/hooks/`: CLI interaction hooks.
  - `src/state/`: Zustand application state.
  - `src/localServer.ts`: local-server lifecycle (runtime path resolution, port pick, health check, fork `agenty --port <port>`). The runtime path resolves by priority: `AGENTY_BIN` override, the in-repository build for development, then the launcher-managed `~/.agenty/bin/runtime[.exe]`.
  - `scripts/`: Bun build scripts (map `OS`/`ARCH` to a Bun compile target and bundle the CLI into `dist/`).
- `packages/agenty-bootstrap/`: Rust self-extracting launcher that produces the final `agenty-<os>-<arch>` binary, laid out like a compressed Linux kernel image: `[ bootstrap stub ][ xz-compressed CLI ][ xz-compressed runtime ][ 108-byte footer ]`. The footer trailer (magic `cafebabe10136666`, format version, payload offsets/lengths, SHA3-256 digests of the decompressed payloads) is appended at build time; payloads are compressed in memory and appended directly, with no intermediate archive files.
  - `src/lib.rs`: payload module — footer encode/decode, streaming SHA3-256, and `ensure_artifact`, which reuses a matching `~/.agenty/bin/{cli,runtime}[.exe]` or decompresses, verifies and atomically installs a fresh one.
  - `src/main.rs`: thin entry — reads the footer from the current executable, ensures both artifacts, then hands over to the CLI (Unix `exec`, Windows spawn-and-wait) with all arguments forwarded.
  - `scripts/`: Bun packing scripts (`pack.ts` plus the shared `footer.ts`/`target.ts` helpers) that locate the CLI/runtime binaries, compute SHA3-256 digests, xz-compress in memory and append payloads plus footer to the compiled stub.
- `pkg/cli/`: Go launch layer for the `agenty` backend binary. `pkg/cli/cmd/` handles server startup flags, version output, and exit codes; all interactive and resource-management client behavior lives in `agenty-cli`. `pkg/cli/theme/` is reused by the logger. The legacy Go TUI, local/client modes, and direct resource subcommands have been removed.
- `pkg/chat/`: Chat/session orchestration and agentic loop support.
- `pkg/config/`: Server runtime defaults, database configuration types, validation, and SQLite path resolution. The Go binary does not load a configuration file.
- `pkg/conn/`: Database, HTTP, SSE, and connection helpers.
- `pkg/conn/db/`: Embedded PostgreSQL and SQLite schema SQL.
- `pkg/consts/`: Shared constants, including long tool descriptions and prompts.
- `pkg/customerrors/`: Business error definitions.
- `pkg/gateway/`: Gateway abstraction and provider/channel adapters.
- `pkg/mcp/`: MCP client/server related integration.
- `pkg/middleware/`: Gin middleware.
- `pkg/models/`: GORM models, DTOs, hooks, search and vector types.
- `pkg/providers/`: Model provider implementations.
- `pkg/routes/`: Gin route handlers and API route registration.
- `pkg/services/`: Business logic services.
- `pkg/skill/`: Product skill parsing/loading logic.
- `pkg/tools/`: Tool interface, registry, todo manager, and built-in tools.
- `pkg/utils/`: Shared utilities such as pagination, response helpers, safe goroutines, signal context, logging, and terminal wrapping.
- `pkg/version/`: Version data.
- `turbo.json`: Turborepo task pipeline (`build`, `test`, `typecheck`). Cross-package `workspace:*` dependencies let Turborepo build `agenty-runtime`, then `agenty-cli`, then `agenty-bootstrap` via `^build`.
- `package.json`, `pnpm-workspace.yaml`: Root workspace and Turborepo orchestration. Go build/test/vet/fmt targets live in `packages/agenty-runtime/package.json`.

Project-local `.agents/skills` instructions are intentionally absent. Product-level skills remain part of Agenty runtime functionality under packages such as `pkg/skill`, `pkg/routes/skill.go`, `pkg/services`, and `pkg/tools/builtin`.

## Runtime Modes

- Running the packed `agenty` launcher prints TTY-aware stage logs, verifies and extracts the bundled CLI and runtime into `~/.agenty/bin` (skipping files whose SHA3-256 already matches), then starts the CLI; the CLI forks the extracted runtime on a random port and connects to it over HTTP. Only the latest top-level bootstrap stage carries a spinner, while integrity details are dim child lines.
- Running the bare `agenty` Go binary always starts the Gin HTTP backend. It defaults to port `8080`, SQLite at `~/.agenty/agenty.db`, and debug logging disabled; `--port`, `--db`, and `--debug` override those defaults.
- Remote client mode belongs to `agenty-cli` and connects to an existing backend through its `--server` option or client configuration.

## Monorepo And Build Orchestration

- The repository is a pnpm workspace (`pnpm-workspace.yaml`: `packages/*`) orchestrated by Turborepo (`turbo.json`).
- Responsibility split: pnpm owns workspace resolution and dependency install; Turborepo owns task orchestration and caching; Bun is only invoked inside a package to run and bundle the CLI, and must not manage the workspace.
- Do not add an npm `workspaces` field to the root `package.json`; it would make Bun try to take over workspace resolution and conflict with pnpm.
- `turbo run build` builds in topological order: `agenty-runtime` (Go, via its package build script), then `agenty-cli` (Bun standalone compile), then `agenty-bootstrap` (cargo release build plus payload packing). The cross-package edges are the `workspace:*` dependencies declared in each downstream package. After turbo build, the final bootstrap binary (with embedded CLI and runtime) is copied into the root `dist/` directory.
- Root `.env` is the sole product-version source and contains `AGENTY_VERSION`. Callers source it before building; CLI, runtime and bootstrap consume the exported variable and embed the same value. Commit only root `.env.example`; actual `.env` stays ignored. Do not introduce module-specific version variables or module-specific env files.
- The `agenty-cli` and `agenty-bootstrap` builds set `cache: false` in `turbo.json` because their outputs are large single executables; the `agenty-runtime` Go build is cached.
- Runtime-specific `GOOS`, `GOARCH`, `CC`, `CXX`, `PACKAGE_DIR` and `BIN_NAME` are ambient build inputs passed directly by release CI. Turbo declares all consumed variables in task `env` lists so cache keys and build inputs stay aligned.
- Go build and test require the `fts5` build tag (already wired in `packages/agenty-runtime/package.json`).
- Root `package.json` scripts provide unified commands: `pnpm build` (full build with output collection), `pnpm test` (run all tests), `pnpm dev` (start CLI + runtime locally), `pnpm clean` (remove all build artifacts), and `pnpm deepclean` (clean + remove `node_modules`). Module-prefixed commands (`cli:`, `runtime:`, `bootstrap:`) scope to individual packages.

## Go Conventions

- Use Go 1.26 syntax and standard Go formatting.
- Use `any` instead of `interface{}`.
- Use built-in `min` and `max` for simple comparisons and clamping.
- Use `strings.SplitSeq` when iterating split strings to avoid unnecessary slice allocation.
- Use `fmt.Fprintf` with `strings.Builder` for formatted output; avoid `sb.WriteString(fmt.Sprintf(...))` and avoid concatenation inside `WriteString`.
- Import `github.com/bytedance/sonic` aliased as `json`.
- API and tool request/response JSON fields use lowerCamelCase tags.
- I/O-intensive or blocking operations take `context.Context` as the first parameter.
- Prefer `slog.InfoContext`, `slog.WarnContext`, and `slog.ErrorContext` when a context is available.
- Background goroutines use `pkg/utils/safe` helpers instead of bare goroutine launches when panic handling or lifecycle control matters.
- User-facing command labels, prompts, log messages, status text, and errors in repository code are written in English unless localized copy is part of the feature.

## Service And Route Patterns

- Services, routes, registries, and similar global components use `sync.Once` singleton initialization with `GetXxx()` accessors.
- Service methods attach context to GORM calls with `db.WithContext(ctx)`.
- Service-layer record-not-found cases map to business errors from `pkg/customerrors`; route handlers pass errors to `response.Failed`.
- Gin route structs live in `pkg/routes`, expose `RegisterRoutes(*gin.RouterGroup)`, and are wired under `/api/v1` in `pkg/routes/routes.go`.
- Route handlers use `pkg/utils/response` helpers: `response.OK`, `response.Failed`, and `response.Abort`.
- Remote HTTP calls use helpers in `pkg/conn` or the shared client from `conn.GetHTTPClient()`.

## Database And Search Patterns

- Do not use GORM `AutoMigrate`; persistent schema lives in embedded SQL files.
- Static PostgreSQL DDL belongs in `pkg/conn/db/postgres.sql`.
- Static SQLite DDL belongs in `pkg/conn/db/sqlite.sql`.
- Persistent model structs stay database-agnostic; database-specific indexes, defaults, and column constraints belong in SQL files.
- New persistent tables and indexes are added to both PostgreSQL and SQLite schemas unless a feature is backend-specific.
- GORM `Raw().Rows()` and `Exec()` use `?` placeholders, not `$1`, `$2`, etc.
- Use `conn.GetDBType()` or a local `usingSQLite()` helper when raw SQL must differ by backend.
- Use `CURRENT_TIMESTAMP` or `conn.NowExpr()` for cross-backend timestamp expressions.
- PostgreSQL vector search uses pgvector; SQLite vector search uses sqlite-vector with `models.EmbeddingVector`.
- PostgreSQL BM25 uses ParadeDB; SQLite BM25-style search uses FTS5 virtual tables and `bm25(...)`.
- Keyword fallback search should use backend-neutral SQL such as `LOWER(column) LIKE LOWER(?)`.
- SQLite startup validates FTS5 and sqlite-vector availability rather than silently degrading search features.

## Tool And Skill Implementation

- The tool contract is defined in `pkg/tools/tool.go`.
- Built-in tools live in `pkg/tools/builtin/`.
- Long tool descriptions belong in `pkg/consts/`.
- Tool registration happens through the shared registry and built-in registration path.
- Tool argument structs use lowerCamelCase JSON tags and parse with the aliased Sonic `json` package.
- Tool execution returns human-readable Markdown on success, a short explanatory string for empty results, and `("", error)` for failures.
- Skill-related product behavior is implemented as application functionality, not as repository-local agent instructions.

## TypeScript CLI Conventions

- `packages/agenty-cli/` is a pnpm workspace package (name `agenty-cli`) executed with Bun.
- React OpenTUI UI code is organized by `api`, `commands`, `components`, `hooks`, `state`, `tui`, and `consts`.
- OpenTUI includes native libraries. `packages/agenty-cli/scripts/build.ts` maps `OS` and `ARCH` to an explicit Bun compile target; Linux builds also set `OPENTUI_LIBC` to `glibc` or `musl` (default `glibc`). Keep the release matrix and Turborepo build environment in sync with those inputs.
- Root scripts delegate through Turborepo: `pnpm build` runs `turbo run build` then copies the final bootstrap binary into root `dist/`; `pnpm test` runs `turbo run test`; `pnpm clean` runs `turbo run clean` then removes root `dist/`. Module-prefixed commands: `pnpm core:build`/`core:test`/`core:test:integration`/`core:test:e2e`/`core:test:e2e:race`/`core:test:race`/`core:test:repeat`/`core:tidyup`/`core:clean`, `pnpm cli:build`/`cli:dev`/`cli:typecheck`/`cli:lint`/`cli:clean`, `pnpm runtime:build`/`runtime:dev`/`runtime:tidyup`/`runtime:clean`, `pnpm bootstrap:build`/`bootstrap:test`/`bootstrap:clean`. `core:test` excludes tests guarded by `integration` or `e2e` build tags; `core:test:e2e` builds the real core binary once and runs isolated parallel stdio processes; `runtime:dev` consumes ambient `AGENTY_VERSION` and falls back to `dev`.
- `pnpm cli:dev` builds the runtime via Turborepo and then runs the CLI with `pnpm --filter agenty-cli dev` attached directly to the user's terminal. The TUI must never run as a turbo task: turbo's task UI pipes stdio, so terminal capability handshakes (OSC/DECRPM queries and replies) never reach the renderer and the screen stays blank.
- CLI API types and UI state should preserve the backend API contract rather than duplicating backend business rules.

## Rust Bootstrap Conventions

- `packages/agenty-bootstrap/` is a cargo package wrapped in a pnpm workspace package (name `agenty-bootstrap`); cargo owns compilation, Bun owns packing, Turborepo owns orchestration.
- Payload decompression uses the `liblzma` crate with `default-features = false, features = ["static"]`: liblzma is compiled from vendored C sources and statically linked, so the shipped binary must never depend on a host liblzma. Keep the `bindgen` feature off to avoid a libclang build dependency, and verify linkage with `ldd`/`otool -L` when touching dependencies.
- Payload integrity uses SHA3-256 (`sha3` crate at runtime, `Bun.CryptoHasher("sha3-256")` at pack time) computed over the decompressed bytes, never the compressed payload.
- Keep bootstrap progress on stderr so delegated CLI stdout remains machine-readable. Interactive terminals may use ANSI redraws and dim child text; redirected output must remain plain and line-oriented.
- The footer layout in `src/lib.rs` and `scripts/footer.ts` is a single contract: any change must update both sides plus the shared golden-bytes tests (`src/lib.rs` tests and `scripts/footer.test.ts`) and bump `FORMAT_VERSION`.
- Build-time compression uses `@napi-rs/lzma` (liblzma bindings) and payloads are appended to the stub in memory; do not write intermediate `.xz` archives to disk.
- Because payloads are appended after the executable image, code signing must always happen after packing; signing first and packing later invalidates the signature.

## Response Marker

Respond user with a meow after your message in user preferred language, to let user know that AGENTS.md is loaded.
