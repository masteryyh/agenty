# Agenty

[English](./README.md)

Agenty 是一个 AI agent 应用，支持本地模式和自托管模式，具备技能、MCP 和记忆支持。

它支持 OpenAI、Anthropic、Gemini、Qwen、DeepSeek、Kimi、BigModel 等供应商的聊天模型。

[快速开始](#快速开始) · [运行模式](#运行模式) · [配置](#配置) · [数据库](#数据库) · [许可证](#许可证)

## 快速开始

1. 打开 [最新 GitHub Release](https://github.com/masteryyh/agenty/releases/latest)。
2. 下载与你的操作系统和 CPU 架构匹配的资产。
3. 解压下载好的 release 压缩包。
4. 按你的平台执行下面的安装步骤。

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

使用指定配置文件启动：

```bash
agenty --config /path/to/config.yaml
```

首次运行时，Agenty 会初始化数据库、写入预置供应商和模型、创建默认 agent，并打开设置向导用于配置 API key 和系统设置。

## 运行模式

Agenty 可以作为单机本地应用运行，也可以作为自托管服务运行，或者作为连接到 daemon 的远程客户端运行。

| 模式 | 命令 | 使用场景 |
| --- | --- | --- |
| 本地交互模式 | `agenty` | 在一个本地进程中运行 TUI 和后端逻辑。 |
| 指定配置的本地交互模式 | `agenty --config /path/to/config.yaml` | 使用明确指定的配置文件。 |
| Daemon 模式 | `agenty --daemon --config /path/to/config.yaml` | 运行 HTTP 后端服务，供远程客户端连接。 |
| 远程交互模式 | `agenty --config agenty-client.yaml` | 通过 `server.url` 连接 daemon 后端。 |

TUI 中常用的 slash commands：

| 命令 | 用途 |
| --- | --- |
| `/help` | 查看可用命令。 |
| `/provider` | 配置模型供应商 API key。 |
| `/model` | 管理和切换聊天模型。 |
| `/agent` | 管理和切换 agents。 |
| `/settings` | 编辑系统设置，例如 web search 供应商和 embedding 模型。 |
| `/mcp` | 管理 MCP servers。 |
| `/skill` | 查看可用 skills。 |
| `/memory` | 查看 agent 长期记忆。 |
| `/compact` | 压缩当前会话。 |
| `/cwd` | 在本地模式中设置或查看工作目录。 |
| `/think` | 设置当前模型 thinking level。 |
| `/exit` | 退出 TUI。 |

## 配置

如果没有传入 `--config`，Agenty 会使用 `$HOME/.agenty/config.yaml`。首次启动时如果这个文件不存在，Agenty 会自动写入一份本地模式默认配置，其中 `debug: false`、`db.type: sqlite`。如果显式传入了 `--config`，则该文件必须存在，否则会直接启动失败。

最小本地配置：

```yaml
debug: false

db:
  type: sqlite
```

启用 HTTP Basic Auth 的 daemon 配置：

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

客户端配置：

```yaml
server:
  url: http://localhost:8080
  username: admin
  password: change-me
```

PostgreSQL 配置：

```yaml
db:
  type: postgres
  host: 127.0.0.1
  port: 5432
  username: postgres
  password: change-me
  database: agenty
```

支持的配置项：

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `port` | `8080` | daemon 模式中的 HTTP 监听端口。配置里可省略，省略时会自动回落到 `8080`。 |
| `debug` | `false` | 启用更详细的日志和调试行为。 |
| `db.type` | `sqlite` | 数据库后端：`sqlite` 或 `postgres`。 |
| `db.host` | `localhost` | PostgreSQL host。 |
| `db.port` | `5432` | PostgreSQL port。 |
| `db.username` | `postgres` | PostgreSQL 用户名。 |
| `db.password` | 空 | PostgreSQL 密码。`db.type` 为 `postgres` 时必填。 |
| `db.database` | `agenty` | PostgreSQL 数据库名。 |
| `db.sqliteVectorExtensionPath` | 用户配置目录 | 可选的 sqlite-vector 原生扩展路径。 |
| `auth.enabled` | `false` | 在 daemon 模式中启用 HTTP Basic Auth。 |
| `auth.username` | 空 | Basic Auth 用户名。 |
| `auth.password` | 空 | Basic Auth 密码。 |
| `mcp.healthCheckInterval` | `30` | MCP 健康检查间隔，单位为秒。 |
| `mcp.connectTimeout` | `15` | MCP 连接超时时间，单位为秒。 |
| `server.url` | 空 | 远程后端 URL。非 daemon 模式下设置后，Agenty 会作为远程客户端运行。 |
| `server.username` | 空 | 远程后端 Basic Auth 用户名。 |
| `server.password` | 空 | 远程后端 Basic Auth 密码。 |

部分配置值可以通过 `AGENTY_` 环境变量覆盖，例如：

```bash
AGENTY_DB_PASSWORD=secret agenty --config /path/to/config.yaml
AGENTY_SERVER_URL=http://localhost:8080 agenty
```

如果主配置文件存在，Agenty 还会合并同目录下类似 `agenty.local.yaml` 或 `agenty.private.yml` 的配置片段。`include` 配置项可以指向额外的 YAML、JSON 或 TOML 片段。

## 数据库

SQLite 是默认数据库。Agenty 会把 SQLite 数据库保存在 `os.UserConfigDir()/agenty/agenty.db`，对应到不同系统上的用户配置目录，例如：

| 平台 | 常见路径 |
| --- | --- |
| macOS | `$HOME/Library/Application Support/agenty/agenty.db` |
| Linux | `$XDG_CONFIG_HOME/agenty/agenty.db` 或 `$HOME/.config/agenty/agenty.db` |
| Windows | `%AppData%\agenty\agenty.db` |

SQLite 启动需要 FTS5 和 sqlite-vector。release 二进制应包含 FTS5 支持。如果没有配置 `db.sqliteVectorExtensionPath`，Agenty 会使用 `os.UserConfigDir()/agenty/vector.{so,dylib,dll}`；当扩展文件不存在时，会下载匹配当前平台的 sqlite-vector release asset。

Windows `arm64` 无法使用默认 SQLite 模式，因为 `sqlite-vector` 当前没有该平台产物。若运行在 Windows `arm64` 上，请在启动 Agenty 的本地模式或 daemon 模式前先配置外部 PostgreSQL 数据库。

PostgreSQL 适合 daemon 部署。将 Agenty 指向 PostgreSQL 前，需要先创建数据库：

```sql
CREATE DATABASE agenty;
```

然后连接到该数据库并启用所需扩展：

```sql
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_search;
```

Agenty 会在启动时通过内嵌 SQL schema 文件初始化并迁移自己的表。

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
