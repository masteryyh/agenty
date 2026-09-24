# Agenty

[简体中文](./README.zh-CN.md)

Agenty is a local-first AI agent application. The current product path consists of
`agenty-cli`, `agenty-core`, the Rust `file-editor` helper, and the self-extracting
`agenty-bootstrap` launcher.
The CLI communicates with core exclusively through line-delimited JSON-RPC 2.0 over
the child process's stdin/stdout; it does not start an HTTP server.

The current core supports provider/model management, persistent sessions, streaming model
output, agentic tool loops, session compaction, built-in filesystem tools, local skills, and
MCP client connections. Memory and remote-client mode remain hidden until equivalent core
implementations exist.

## Quick start

Download the archive for your operating system and architecture from the
[latest release](https://github.com/masteryyh/agenty/releases/latest), extract it,
and install the `agenty` executable:

```bash
chmod +x agenty
sudo install -m 755 agenty /usr/local/bin/agenty
agenty
```

On first run, the launcher verifies and extracts the bundled CLI, core, and patch helper into
`~/.agenty/bin/{cli,core,fileedit}`. The CLI starts core as a child process and opens a setup
wizard. The wizard creates one provider and one chat model through the existing `provider.*`
IPC methods, then calls `initialize.complete` to persist the global default session model.

## Runtime model

The launcher contains three XZ-compressed payloads and their decompressed SHA3-256
digests. Matching extracted files are reused; missing or mismatched files are verified
and atomically replaced. The CLI resolves core in this order:

1. `AGENTY_CORE_BIN`
2. `packages/agenty-core/bin/agenty-core` during repository development
3. `~/.agenty/bin/core` from the launcher

Before starting core, the CLI prepends core's directory to `PATH`, making the bundled
`fileedit` command available to core.

Core reads one compact JSON-RPC message per stdin line and writes responses and
notifications to stdout. After `session.start`, core sends ordered `session.event`
notifications for round lifecycle, persisted messages, model stream deltas, tool calls,
and the terminal round status. Notifications may arrive before the `session.start`
response, so clients must subscribe before sending the request. Core exits when stdin
reaches EOF.

The TUI currently exposes `/provider`, `/model`, `/mcp`, `/cwd`, `/effort`, `/status`, `/codex-mode`,
`/new`, `/resume`, `/help`, and `/exit`. Type `$` in the composer to search for an installed
skill and insert it as a structured reference. Core scans the data directory's `skills/`
folder first, followed by `~/.agents/skills` and `~/.claude/skills`; set `AGENTY_DATA_DIR` to
change the first location.

## Configuration and storage

Core stores data under `~/.agenty` by default. Pass `--data-dir <path>` to the CLI or set
`AGENTY_DATA_DIR` for core to use another root. Important files are:

| Data | Path |
| --- | --- |
| Configuration | `~/.agenty/config.json` |
| Skills | `~/.agenty/skills/`, then `~/.agents/skills/` and `~/.claude/skills/` |
| Session transcripts | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` |
| Session index | `~/.agenty/agenty.sqlite` |
| Providers and models | Built-in catalog is embedded in the core binary; custom providers use `~/.agenty/providers/<provider-code>.json`, while built-in provider files store only API keys |
| MCP server configuration | `~/.agenty/mcp/<server-name>.json` (one concise JSON file per server, mode `0600`) |
| MCP OAuth credentials | `~/.agenty/mcp-auth/<server-name>.json` (mode `0600`, separate from configuration) |
| Model discovery cache | Kept only in the running core process for 8 hours; it is refreshed on demand and does not survive a core restart |
| Patch transaction locks | `~/.agenty/locks/` |
| Logs | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` |

`AGENTY_LOG_LEVEL` accepts `debug`, `info`, `warn`, or `error`.
`AGENTY_LOG_FORMAT` accepts `text` or `jsonl`.

`/mcp` manages stdio, Streamable HTTP, and legacy SSE servers. A server file contains `type`,
`enabled`, and transport-specific `command`/`args`/`env` or `url`/`headers` fields; `args` is a
JSON string array with one entry per process argument. It does not store a working directory.
Environment and header values may reference environment variables; headers can be edited from
the collapsed `Advanced Options` section in the TUI.
Streamable HTTP supports the Go SDK's OAuth authorization-code flow, including dynamic client
registration and loopback browser login; `/mcp` opens the authorization URL and reports the
connection state. SSE remains available for existing servers and is marked deprecated in the
TUI. Connections start in the background with bounded concurrency, and a round snapshots the
currently connected tool set when it begins, so a server that connects later becomes available
from the next round.

## Development

```bash
pnpm install
pnpm cli:dev
```

`pnpm cli:dev` builds `agenty-core` first, then attaches the source TUI directly to the
terminal. Useful focused commands include:

```bash
pnpm core:test
pnpm core:test:integration
pnpm core:test:e2e
pnpm cli:typecheck
pnpm bootstrap:test
pnpm build
pnpm run update
pnpm tidyup
pnpm clean
pnpm deepclean
```

The build version comes from `AGENTY_VERSION` in the process environment, then from
the ignored root `.env`; it defaults to `dev`. Copy `.env.example` to `.env` if you
want a persistent local version, then run `pnpm build` for a complete launcher build.
The default build includes file-editor, core, CLI and bootstrap. Build Inspector
separately with `pnpm inspector:build`.

`pnpm run update` updates dependencies for the pnpm, Go, and Cargo modules. `pnpm tidyup`
runs `go fmt`, `go vet`, and `go mod tidy` in every Go module. `pnpm clean` removes
all root and workspace build outputs; `pnpm deepclean` additionally removes local
caches, dependency stores, `node_modules`, and generated temporary files. Environment
files and tracked lockfiles are preserved.

## Session inspector

[Agenty Inspector](./packages/agenty-inspector/README.md) is a separate read-only web
debugger for local session transcripts. Run `pnpm inspector:dev` and open
`http://127.0.0.1:5173`, or use `pnpm inspector:build` followed by
`pnpm inspector:start` for the standalone executable on port 4318. It reads
`AGENTY_DATA_DIR` (default `~/.agenty`) and exposes messages, raw events, tool
relations and diagnostics without starting core.

## License

Licensed under the Apache License 2.0. See [LICENSE](./LICENSE).

Copyright (c) 2026 masteryyh
