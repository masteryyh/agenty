# Agenty

[简体中文](./README.zh-CN.md)

Agenty is an AI agent application that supports both local mode and self-hosted mode, with skills, MCP and memory support.

It supports chat models from providers such as OpenAI, Anthropic, Gemini, Qwen, DeepSeek, Kimi, and BigModel.

[Quick Start](#quick-start) · [Run Modes](#run-modes) · [Configuration](#configuration) · [Database](#database) · [License](#license)

## Quick Start

1. Open the [latest GitHub Release](https://github.com/masteryyh/agenty/releases/latest).
2. Download the asset that matches your operating system and CPU architecture.
3. Extract the downloaded release archive.
4. Follow the install steps for your platform.

### Linux

```bash
chmod +x agenty
sudo install -m 755 agenty /usr/local/bin/agenty
```

### macOS

```bash
chmod +x agenty
sudo install -m 755 agenty /usr/local/bin/agenty
```

Start the interactive TUI:

```bash
agenty
```

On first run, the launcher verifies and extracts the bundled CLI and runtime into `~/.agenty/bin`, then starts the CLI, which forks the runtime on a random local port. The runtime initializes its database, seeds preset providers and models, and creates the default agent; the CLI detects the uninitialized state and opens a setup wizard to configure provider API keys, web search, and default models.

To run only the HTTP backend standalone, execute `~/.agenty/bin/runtime` after the first launch. It defaults to port `8080`, SQLite at `~/.agenty/agenty.db`, and debug logging disabled.

## Run Modes

Agenty ships as a single self-extracting artifact: the `agenty` launcher, a small Rust binary that carries the XZ-compressed `agenty-cli` (React OpenTUI terminal UI plus resource-management subcommands) and the `agenty` Go runtime (HTTP backend) appended after its own code, along with the SHA3-256 digests of both payloads.

On startup the launcher checks `~/.agenty/bin/cli` and `~/.agenty/bin/runtime`: a file whose SHA3-256 matches the embedded digest is reused, while a missing or mismatched one is decompressed, verified, and atomically replaced. It then starts the CLI, which forks the runtime on a random local port. Set `AGENTY_BIN` only when you intentionally want the CLI to use an unmanaged runtime binary during development.

| Mode | Command | Use case |
| --- | --- | --- |
| Local interactive mode | `agenty` | Run the TUI; the launcher verifies or extracts `~/.agenty/bin/{cli,runtime}`, starts the CLI, and the CLI starts the runtime on a random local port. |
| Local interactive mode (dev) | `pnpm cli:dev` | Build `agenty-runtime` through Turborepo, then run the TUI from source against the in-repository runtime. |
| Local interactive mode with database | `agenty --db /path/to/agenty.db` | Use an explicit SQLite database for the local server. |
| Server mode | `~/.agenty/bin/runtime` | Run the extracted HTTP backend service with default settings. |
| Remote interactive mode | `agenty --server http://host:8080` | Connect the TUI to a remote `agenty` server instead of spawning a local one. |
| Resource-management CLI | `agenty <subcommand>` | Initialize and manage agents, providers, models, settings, MCP servers, and global skills. See `agenty --help`. |

On first run, `agenty-cli` detects an uninitialized system and opens a setup wizard to configure provider API keys, web search, and default models.

Common slash commands inside the TUI:

The TUI supports mouse-wheel scrolling, clickable lists and actions, and mouse text selection with OSC52 clipboard copy when the terminal supports it.

| Command | Purpose |
| --- | --- |
| `/help` | Show available commands. |
| `/provider` | Manage model providers. |
| `/model` | Switch the chat model. |
| `/agents` | Manage agents and switch the current agent. |
| `/config` | View and edit system settings. |
| `/mcp` | Manage MCP servers. |
| `/skill` | Browse available skills. |
| `/compact` | Compact the current conversation. |
| `/cwd` | Set or show the session working directory. |
| `/think` | Set the current model thinking level. |
| `/status` | Show current session status. |
| `/new` | Start a new session. |
| `/resume` | Resume a previous session. |
| `/exit` | Quit the TUI. |

## Configuration

The Go backend does not read or create a configuration file. Its runtime settings are command-line flags:

| Flag | Default | Description |
| --- | --- | --- |
| `--port` | `8080` | HTTP listen port. |
| `--db` | `~/.agenty/agenty.db` | SQLite database file. `~` is expanded to the current user's home directory. |
| `--debug` | disabled | Enable debug logging and Gin debug mode. |
| `--version`, `-v` | disabled | Print version information and exit. |

For example:

```bash
agenty --port 9090 --db /srv/agenty/agenty.db --debug
```

The CLI passes `--db` and `--debug` through to its local backend in local mode. Its remote-client settings remain separate; see `agenty --help` for `--server` and `--client-config`.

## Database

SQLite is the active database backend. By default, Agenty stores it at `~/.agenty/agenty.db`; use `--db` to select another file. The parent directory is created automatically.

SQLite startup requires FTS5 and sqlite-vector. The release binary is expected to include FTS5 support. Agenty stores the sqlite-vector native extension next to the selected database and downloads the matching release asset when the extension is missing.

Windows `arm64` cannot currently run the server because sqlite-vector does not provide that platform and PostgreSQL selection is not exposed through the CLI.

The PostgreSQL implementation and schema remain in the codebase for a later decision, but the current server CLI does not expose PostgreSQL connection parameters. A future PostgreSQL deployment would still require creating the database:

```sql
CREATE DATABASE agenty;
```

Then connect to that database and enable the required extensions:

```sql
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_search;
```

Agenty initializes and migrates its own tables from embedded SQL schema files at startup.

## License

This project is licensed under the Apache License 2.0. For more details, see the LICENSE file in the repository.

Copyright (c) 2026 masteryyh

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[https://www.apache.org/licenses/LICENSE-2.0](https://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
