# Agenty

[简体中文](./README.zh-CN.md)

Agenty is a local-first AI agent application. The current product path consists of
`agenty-cli`, `agenty-core`, the Rust `patch-applier` helper, and the self-extracting
`agenty-bootstrap` launcher.
The CLI communicates with core exclusively through line-delimited JSON-RPC 2.0 over
the child process's stdin/stdout; it does not start an HTTP server.

The current core supports provider/model/agent management, persistent sessions,
streaming model output, agentic tool loops, session compaction, and built-in filesystem
tools. Skills, MCP, memory, and remote-client mode remain hidden until equivalent core
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
`~/.agenty/bin/{cli,core,apply_patch}`. The CLI starts core as a child process and opens a setup
wizard. The wizard creates one provider, one chat model, and one default agent through
the existing `provider.*` and `agent.*` IPC methods, then calls `initialize.complete`.

## Runtime model

The launcher contains three XZ-compressed payloads and their decompressed SHA3-256
digests. Matching extracted files are reused; missing or mismatched files are verified
and atomically replaced. The CLI resolves core in this order:

1. `AGENTY_CORE_BIN`
2. `packages/agenty-core/bin/agenty-core` during repository development
3. `~/.agenty/bin/core` from the launcher

Before starting core, the CLI prepends core's directory to `PATH`, making the bundled
`apply_patch` command available to core and shell tool calls.

Core reads one compact JSON-RPC message per stdin line and writes responses and
notifications to stdout. After `session.start`, core sends ordered `session.event`
notifications for round lifecycle, persisted messages, model stream deltas, tool calls,
and the terminal round status. Notifications may arrive before the `session.start`
response, so clients must subscribe before sending the request. Core exits when stdin
reaches EOF.

The TUI currently exposes `/provider`, `/model`, `/agents`, `/cwd`, `/effort`, `/status`,
`/new`, `/resume`, `/help`, and `/exit`. Features not yet implemented by core are hidden.

## Configuration and storage

Core stores data under `~/.agenty` by default. Pass `--data-dir <path>` to the CLI or set
`AGENTY_DATA_DIR` for core to use another root. Important files are:

| Data | Path |
| --- | --- |
| Configuration | `~/.agenty/config.json` |
| Session transcripts | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` |
| Session index | `~/.agenty/agenty.sqlite` |
| Providers and models | Built-in catalog is embedded in the core binary; custom providers use `~/.agenty/providers/<provider-code>.json`, built-in provider files store only API keys; core automatically discovers empty configured catalogs when listing providers/models and caches them under `~/.agenty/providers/.models/` for 8 hours |
| Agents | `~/.agenty/agents/` |
| Logs | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` |

`AGENTY_LOG_LEVEL` accepts `debug`, `info`, `warn`, or `error`.
`AGENTY_LOG_FORMAT` accepts `text` or `jsonl`.

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
```

The release version comes from the exported root `AGENTY_VERSION` value. Copy
`.env.example` to the ignored `.env`, source it, and run `pnpm build` for a complete
launcher build.

## License

Licensed under the Apache License 2.0. See [LICENSE](./LICENSE).

Copyright (c) 2026 masteryyh
