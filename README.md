# Agenty

A self-hosted AI agent framework with ReAct-pattern agentic looping, multi-provider LLM support, tool calling, and long-term memory.

This project is still under construction, expect frequent updates and breaking changes.

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 18 with the [pgvector](https://github.com/pgvector/pgvector) extension enabled
- At least one LLM provider API key

### Building

```bash
git clone https://github.com/masteryyh/agenty.git
cd agenty
make all
```

This produces:
- `bin/agenty-server` — the backend REST API server
- `bin/agenty-cli` — the interactive CLI client

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
  host: 127.0.0.1
  port: 5432
  username: postgres
  password: your_password
  database: agenty

# Optional: HTTP Basic Auth
auth:
  enabled: false
  username: admin
  password: secret

# Optional: Enable long-term memory tools
embedding:
  baseUrl: https://api.openai.com/v1
  apiKey: sk-...
  model: text-embedding-3-small
```

### Running the Server

```bash
./bin/agenty-server --config my-config.yaml
```

On first start, Agenty auto-provisions preset providers and models and creates a default agent.

### Using the CLI

```bash
# Configure the CLI (saved to ~/.agenty/config.yaml)
./bin/agenty-cli config set

# Start an interactive chat session
./bin/agenty-cli chat

# Or point to a remote server
./bin/agenty-cli --url http://your-server:8080 chat
```

## Supported LLM Providers

Agenty ships with pre-seeded configurations for these providers. Simply add your API key via `agenty provider update`.

| Provider | API Type | Notable Models |
|----------|----------|----------------|
| **OpenAI** | `openai` | `gpt-5.2`, `gpt-5.3-codex`, `gpt-4o` |
| **Anthropic** | `anthropic` | `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5` |
| **Google Gemini** | `gemini` | `gemini-3.1-pro-preview`, `gemini-3-flash-preview` |
| **Kimi** | `kimi` | `kimi-k2.5` (default model) |
| **OpenAI-Completions** | `openai_legacy` | Any OpenAI /v1/chat/completions endpoint |

All providers support **extended thinking** configuration with per-model thinking levels (e.g., `low`, `medium`, `high`, `max`).

---

## Configuration Reference

| Key | Default | Description |
|-----|---------|-------------|
| `port` | `8080` | HTTP server listen port |
| `debug` | `false` | Enable debug mode and the debug tool |
| `db.host` | `127.0.0.1` | PostgreSQL host |
| `db.port` | `5432` | PostgreSQL port |
| `db.username` | `postgres` | PostgreSQL user |
| `db.password` | *(required)* | PostgreSQL password |
| `db.database` | `agenty` | PostgreSQL database name |
| `auth.enabled` | `false` | Enable HTTP Basic Auth |
| `auth.username` | — | Basic auth username |
| `auth.password` | — | Basic auth password |
| `embedding.baseUrl` | — | Embedding API base URL |
| `embedding.apiKey` | — | Embedding API key |
| `embedding.model` | `text-embedding-3-small` | Embedding model name |

All settings can be overridden with environment variables using the `AGENTY_` prefix (e.g., `AGENTY_DB_PASSWORD=secret`).

## Database Setup

Agenty requires PostgreSQL with the `pgvector` extension for memory features.

```sql
-- Enable pgvector (run as superuser)
CREATE EXTENSION IF NOT EXISTS vector;

-- Create database
CREATE DATABASE agenty;
```

Agenty handles table creation automatically via GORM `AutoMigrate` on startup.

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
