# AGENTS.md

## Project Overview

Agenty is an AI agent application written primarily in Go 1.26. The Go binary is an HTTP server, while `agenty-cli` provides local interactive and remote client modes. Core capabilities include chat sessions, model/provider management, tool calling, agentic looping, MCP integration, skills, memory, and searchable knowledge.

The Go backend uses Gin, GORM, SQLite/PostgreSQL, embedded SQL schema files, provider adapters, and a tool registry. The repository is a pnpm + Turborepo monorepo: the Go backend lives in `packages/agenty-runtime/`, and a TypeScript terminal client lives in `apps/cli/`, built with Bun, React, Ink, Zustand, and pnpm.

## Project Structure

The repository is a pnpm + Turborepo monorepo with two workspaces: `apps/cli` (TypeScript CLI) and `packages/agenty-runtime` (Go backend). Go paths below (`cmd/`, `pkg/...`) are relative to `packages/agenty-runtime/`.

- `packages/agenty-runtime/`: Go HTTP backend (Go module `github.com/masteryyh/agenty`). Built via its own `Makefile` (`make build` → `bin/agenty`) and consumed by `apps/cli` as an embedded binary.
- `cmd/`: Go application entrypoint. `cmd/main.go` delegates to `pkg/cli/cmd`.
- `apps/cli/`: TypeScript/React Ink terminal UI — the primary Agenty TUI. In local mode it embeds the `agenty` binary and starts it with `--port` on a random port; in remote mode it connects to an existing server.
  - `src/api/`: HTTP client and API DTOs for the CLI.
  - `src/commands/`: Slash command registry and parsing.
  - `src/components/`: Ink UI components (chat, overlays, setup wizard).
  - `src/hooks/`: CLI interaction hooks.
  - `src/state/`: Zustand application state.
  - `src/localServer.ts`: embedded-server lifecycle (binary discovery, port pick, health check, fork `agenty --port <port>`).
  - `scripts/`: Bun build scripts (bundles the CLI and embeds `bin/agenty` into `dist/`).
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
- `turbo.json`: Turborepo task pipeline (`build`, `test`, `typecheck`). `apps/cli` declares a `workspace:*` dependency on `agenty-runtime` so Turborepo builds the Go binary first via `^build`.
- `package.json`, `pnpm-workspace.yaml`: Root workspace and Turborepo orchestration. Go build/test/vet/fmt targets live in `packages/agenty-runtime/Makefile`.

Project-local `.agents/skills` instructions are intentionally absent. Product-level skills remain part of Agenty runtime functionality under packages such as `pkg/skill`, `pkg/routes/skill.go`, `pkg/services`, and `pkg/tools/builtin`.

## Runtime Modes

- Running `agenty` always starts the Gin HTTP backend. It defaults to port `8080`, SQLite at `~/.agenty/agenty.db`, and debug logging disabled; `--port`, `--db`, and `--debug` override those defaults.
- Local interactive mode runs `agenty-cli`, which forks an embedded `agenty` subprocess on a random port and connects to it over HTTP.
- Remote client mode belongs to `agenty-cli` and connects to an existing backend through its `--server` option or client configuration.

## Monorepo And Build Orchestration

- The repository is a pnpm workspace (`pnpm-workspace.yaml`: `apps/*`, `packages/*`) orchestrated by Turborepo (`turbo.json`).
- Responsibility split: pnpm owns workspace resolution and dependency install; Turborepo owns task orchestration and caching; Bun is only invoked inside a package to run and bundle the CLI, and must not manage the workspace.
- Do not add an npm `workspaces` field to the root `package.json`; it would make Bun try to take over workspace resolution and conflict with pnpm.
- `turbo run build` builds in topological order: `agenty-runtime` (Go, via `make build`) then `agenty-cli` (Bun compile embedding the Go binary). The cross-package edge is the `agenty-runtime` `workspace:*` dependency in `apps/cli/package.json`.
- The `agenty-cli` build sets `cache: false` in `turbo.json` because its output is a large single executable; the `agenty-runtime` Go build is cached.
- Go build and test require the `fts5` build tag (already wired in `packages/agenty-runtime/Makefile`).

## Go Conventions

- New Go source files use the repository Apache 2.0 license header.
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

- `apps/cli/` is a pnpm workspace package (name `agenty-cli`) executed with Bun.
- React Ink UI code is organized by `api`, `commands`, `components`, `hooks`, `state`, and `consts`.
- Root scripts delegate through Turborepo: `pnpm build`/`pnpm test`/`pnpm typecheck` run `turbo run <task>`; `pnpm cli:typecheck` and `pnpm cli:build` filter to the CLI.
- CLI API types and UI state should preserve the backend API contract rather than duplicating backend business rules.

## Response Marker

Respond user with a meow after your message in user preferred language, to let user know that AGENTS.md is loaded.
