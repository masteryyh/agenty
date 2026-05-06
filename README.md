# Agenty

A self-hosted AI agent framework with ReAct-pattern agentic looping, multi-provider LLM support, tool calling, and long-term memory.

This project is still under construction, expect frequent updates and breaking changes.

## Quick Start

### Prerequisites

- Go 1.26+
- SQLite with FTS5 enabled, or PostgreSQL 18 with `pgvector` and `pg_search`
- At least one LLM provider API key

### Building

```bash
git clone https://github.com/masteryyh/agenty.git
cd agenty
make build
```

This produces a single binary:
- `bin/agenty` — the unified binary (backend server + interactive CLI)

### Configuration

Copy and edit the example configuration:

```bash
cp agenty.yaml my-config.yaml
```

```yaml
# my-config.yaml
port: 8080
debug: false

db:
  type: sqlite

# Optional PostgreSQL configuration:
# db:
#   type: postgres
#   host: 127.0.0.1
#   port: 5432
#   username: postgres
#   password: your_password
#   database: agenty

# Optional: HTTP Basic Auth (for daemon mode)
auth:
  enabled: false
  username: admin
  password: secret

# Optional: Connect to a remote backend instead of running locally
# server:
#   url: http://your-server:8080
#   username: admin   # if auth is enabled on the remote server
#   password: secret
```

### Running Modes

Agenty ships as a single binary with three operating modes:

**Daemon mode** — run the backend HTTP API server:

```bash
./bin/agenty --daemon --config my-config.yaml
```

On first start, Agenty auto-provisions preset providers and models and creates a default agent.

**Local mode** — interactive CLI that connects directly to a local database (no separate server process needed):

```bash
./bin/agenty --config my-config.yaml
```

**Remote mode** — interactive CLI that connects to a remote backend server. Configure `server.url` in your config file:

```bash
./bin/agenty --config my-config.yaml
```

## Supported LLM Providers

Agenty ships with pre-seeded configurations for these providers. Simply add your API key via `agenty provider update`.

| Provider | API Type | Notable Models |
|----------|----------|----------------|
| **OpenAI** | `openai` | `gpt-5.2`, `gpt-5.3-codex`, `gpt-4o` |
| **Anthropic** | `anthropic` | `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5` |
| **Google Gemini** | `gemini` | `gemini-3.1-pro-preview`, `gemini-3-flash-preview` |
| **Kimi** | `kimi` | `kimi-k2.5` (default model) |
| **BigModel** | `bigmodel` | GLM family models |
| **Qwen** | `qwen` | `qwen3.6-plus`, `qwen3-max` |
| **DeepSeek** | `deepseek` | `deepseek-v4-flash`, `deepseek-v4-pro` |
| **OpenAI-Completions** | `openai-legacy` | Any OpenAI /v1/chat/completions endpoint |

All providers support **extended thinking** configuration with per-model thinking levels (e.g., `low`, `medium`, `high`, `max`).

---

## Configuration Reference

| Key | Default | Description |
|-----|---------|-------------|
| `port` | `8080` | HTTP server listen port (daemon mode only) |
| `debug` | `false` | Enable debug mode and the debug tool |
| `db.type` | `sqlite` | Database backend: `sqlite` or `postgres` |
| `db.host` | `127.0.0.1` | PostgreSQL host |
| `db.port` | `5432` | PostgreSQL port |
| `db.username` | `postgres` | PostgreSQL user |
| `db.password` | *(required for PostgreSQL)* | PostgreSQL password |
| `db.database` | `agenty` | PostgreSQL database name |
| `db.sqliteVectorExtensionPath` | `os.UserConfigDir()/agenty/vector.{so,dylib,dll}` | sqlite-vector extension path; missing files are downloaded automatically |
| `auth.enabled` | `false` | Enable HTTP Basic Auth (daemon mode only) |
| `auth.username` | — | Basic auth username |
| `auth.password` | — | Basic auth password |
| `server.url` | — | Remote backend URL (enables remote mode) |
| `server.username` | — | Remote backend Basic Auth username |
| `server.password` | — | Remote backend Basic Auth password |

All settings can be overridden with environment variables using the `AGENTY_` prefix (e.g., `AGENTY_DB_PASSWORD=secret`, `AGENTY_SERVER_URL=http://remote:8080`).

## Database Setup

SQLite is the default local backend. The database file is stored under `os.UserConfigDir()/agenty/agenty.db`, so it survives binary updates and reinstalls.

SQLite startup requires:

- FTS5 support in the SQLite driver. When building from source with `go-sqlite3`, use the `sqlite_fts5` build tag if your build does not enable it by default.
- The sqlite-vector native extension. Agenty checks `db.sqliteVectorExtensionPath` or `os.UserConfigDir()/agenty/vector.{so,dylib,dll}` first; if the file is missing, it fetches the latest sqlite-vector GitHub release, selects the current OS/CPU asset, downloads it, and installs the extracted library at that path.

PostgreSQL remains supported for daemon deployments:

```sql
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_search;

CREATE DATABASE agenty;
```

Agenty initializes tables from embedded SQL schema files at startup.

## License

Copyright © 2026 masteryyh

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
