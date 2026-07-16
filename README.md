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

Start the backend with its defaults (port `8080`, SQLite at `~/.agenty/agenty.db`, debug logging disabled):

```bash
agenty
```

On first run, `agenty` initializes its database, seeds preset providers and models, and creates the default agent. When you start `agenty-cli` against a fresh system, it detects the uninitialized state and opens a setup wizard to configure provider API keys, web search, and default models.

## Run Modes

Agenty ships as two artifacts: the `agenty` Go binary (HTTP backend only) and `agenty-cli` (React OpenTUI terminal UI plus resource-management subcommands, embedding `agenty` for local use).

| Mode | Command | Use case |
| --- | --- | --- |
| Local interactive mode | `agenty-cli` | Run the TUI; agenty-cli spawns an embedded `agenty` on a random local port and connects to it. |
| Local interactive mode (dev) | `pnpm cli:dev` | Run the TUI from source; reuses the `agenty-runtime` binary or builds it via `make build` on first run. |
| Local interactive mode with database | `agenty-cli --db /path/to/agenty.db` | Use an explicit SQLite database for the embedded server. |
| Server mode | `agenty` | Run the HTTP backend service with default settings. |
| Remote interactive mode | `agenty-cli --server http://host:8080` | Connect the TUI to a remote `agenty` server instead of spawning a local one. |
| Resource-management CLI | `agenty-cli <subcommand>` | Initialize and manage agents, providers, models, settings, MCP servers, and global skills. See `agenty-cli --help`. |

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

`agenty-cli` passes `--db` and `--debug` through to its embedded backend in local mode. Its remote-client settings remain separate; see `agenty-cli --help` for `--server` and `--client-config`.

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
