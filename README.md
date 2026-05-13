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

To use a specific configuration file:

```bash
agenty --config /path/to/config.yaml
```

On first run, Agenty initializes its database, seeds preset providers and models, creates the default agent, and opens a setup wizard for API keys and system settings.

## Run Modes

Agenty can run as a single local app, a self-hosted daemon, or a remote client connected to that daemon.

| Mode | Command | Use case |
| --- | --- | --- |
| Local interactive mode | `agenty` | Run the TUI and backend logic in one local process. |
| Local interactive mode with config | `agenty --config /path/to/config.yaml` | Use an explicit config file. |
| Daemon mode | `agenty --daemon --config /path/to/config.yaml` | Run the HTTP backend service for remote clients. |
| Remote interactive mode | `agenty --config agenty-client.yaml` | Connect the TUI to a daemon through `server.url`. |

Common slash commands inside the TUI:

| Command | Purpose |
| --- | --- |
| `/help` | Show available commands. |
| `/provider` | Configure model provider API keys. |
| `/model` | Manage and switch chat models. |
| `/agent` | Manage and switch agents. |
| `/settings` | Edit system settings such as web search provider and embedding model. |
| `/mcp` | Manage MCP servers. |
| `/skill` | View available skills. |
| `/memory` | View agent long-term memories. |
| `/compact` | Compact the current conversation. |
| `/cwd` | Set or show the working directory in local mode. |
| `/think` | Set the current model thinking level. |
| `/exit` | Quit the TUI. |

## Configuration

If you do not pass `--config`, Agenty uses `$HOME/.agenty/config.yaml`. On first run, if that file does not exist, Agenty creates it automatically with a local-mode default configuration (`debug: false` and `db.type: sqlite`). If you pass `--config`, that exact file must exist or startup fails.

Minimal local configuration:

```yaml
debug: false

db:
  type: sqlite
```

Daemon configuration with HTTP Basic Auth:

```yaml
debug: false
port: 8080

db:
  type: sqlite

auth:
  enabled: true
  username: admin
  password: change-me
```

Client configuration:

```yaml
server:
  url: http://localhost:8080
  username: admin
  password: change-me
```

PostgreSQL configuration:

```yaml
db:
  type: postgres
  host: 127.0.0.1
  port: 5432
  username: postgres
  password: change-me
  database: agenty
```

Supported configuration keys:

| Key | Default | Description |
| --- | --- | --- |
| `port` | `8080` | HTTP listen port in daemon mode. Optional in config; Agenty falls back to `8080` when omitted. |
| `debug` | `false` | Enables verbose logging and debug behavior. |
| `db.type` | `sqlite` | Database backend: `sqlite` or `postgres`. |
| `db.host` | `localhost` | PostgreSQL host. |
| `db.port` | `5432` | PostgreSQL port. |
| `db.username` | `postgres` | PostgreSQL username. |
| `db.password` | empty | PostgreSQL password. Required when `db.type` is `postgres`. |
| `db.database` | `agenty` | PostgreSQL database name. |
| `db.sqliteVectorExtensionPath` | user config dir | Optional sqlite-vector native extension path. |
| `auth.enabled` | `false` | Enables HTTP Basic Auth in daemon mode. |
| `auth.username` | empty | Basic Auth username. |
| `auth.password` | empty | Basic Auth password. |
| `mcp.healthCheckInterval` | `30` | MCP health-check interval in seconds. |
| `mcp.connectTimeout` | `15` | MCP connection timeout in seconds. |
| `server.url` | empty | Remote backend URL. If set outside daemon mode, Agenty runs as a remote client. |
| `server.username` | empty | Remote backend Basic Auth username. |
| `server.password` | empty | Remote backend Basic Auth password. |

Several configuration values can be overridden with `AGENTY_` environment variables, for example:

```bash
AGENTY_DB_PASSWORD=secret agenty --config /path/to/config.yaml
AGENTY_SERVER_URL=http://localhost:8080 agenty
```

If the main config file is found, Agenty also merges sibling fragments named like `agenty.local.yaml` or `agenty.private.yml`. The `include` key can point to additional YAML, JSON, or TOML fragments.

## Database

SQLite is the default database. Agenty stores the SQLite database at `os.UserConfigDir()/agenty/agenty.db`, which maps to the platform user configuration directory such as:

| Platform | Typical path |
| --- | --- |
| macOS | `$HOME/Library/Application Support/agenty/agenty.db` |
| Linux | `$XDG_CONFIG_HOME/agenty/agenty.db` or `$HOME/.config/agenty/agenty.db` |
| Windows | `%AppData%\agenty\agenty.db` |

SQLite startup requires FTS5 and sqlite-vector. The release binary is expected to include FTS5 support. If `db.sqliteVectorExtensionPath` is not configured, Agenty uses `os.UserConfigDir()/agenty/vector.{so,dylib,dll}` and downloads the matching sqlite-vector release asset when the extension is missing.

Windows `arm64` cannot use the default SQLite mode because `sqlite-vector` does not provide that platform. On Windows `arm64`, configure an external PostgreSQL database before starting Agenty in local or daemon mode.

PostgreSQL is intended for daemon deployments. Before pointing Agenty at PostgreSQL, create the database:

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
