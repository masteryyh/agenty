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

使用默认设置启动后端（端口 `8080`、SQLite 位于 `~/.agenty/agenty.db`、关闭 debug 日志）：

```bash
agenty
```

首次运行时，`agenty` 会初始化数据库、写入预置供应商和模型、创建默认 agent。当你在一个全新系统上启动 `agenty-cli` 时，它会检测到未初始化状态，并打开设置向导用于配置供应商 API key、web search 和默认模型。

## 运行模式

Agenty 由两个产物组成：`agenty` Go 二进制（仅提供 HTTP 后端）和 `agenty-cli`（React Ink TUI 与资源管理子命令，本地使用时会内嵌 `agenty` 二进制）。

| 模式 | 命令 | 使用场景 |
| --- | --- | --- |
| 本地交互模式 | `agenty-cli` | 运行 TUI；agenty-cli 会在随机本地端口启动一个内嵌的 `agenty` 子进程并连接它。 |
| 本地交互模式（开发） | `pnpm cli:dev` | 从源码运行 TUI；复用 `agenty-runtime` 二进制，缺失时首次会自动执行 `make build`。 |
| 指定数据库的本地交互模式 | `agenty-cli --db /path/to/agenty.db` | 为内嵌 server 指定 SQLite 数据库。 |
| Server 模式 | `agenty` | 使用默认设置运行 HTTP 后端服务。 |
| 远程交互模式 | `agenty-cli --server http://host:8080` | 将 TUI 连接到远程 `agenty` server，而不是启动本地 server。 |
| 资源管理 CLI | `agenty-cli <子命令>` | 初始化并管理 agents、providers、models、settings、MCP servers 和全局 skills。详见 `agenty-cli --help`。 |

首次运行时，`agenty-cli` 会检测到未初始化状态，并打开设置向导用于配置供应商 API key、web search 和默认模型。

TUI 中常用的 slash commands：

| 命令 | 用途 |
| --- | --- |
| `/help` | 查看可用命令。 |
| `/provider` | 管理模型供应商。 |
| `/model` | 切换聊天模型。 |
| `/agents` | 管理 agents 并切换当前 agent。 |
| `/config` | 查看和编辑系统设置。 |
| `/mcp` | 管理 MCP servers。 |
| `/skill` | 浏览可用 skills。 |
| `/compact` | 压缩当前会话。 |
| `/cwd` | 设置或查看会话工作目录。 |
| `/think` | 设置当前模型 thinking level。 |
| `/status` | 查看当前会话状态。 |
| `/new` | 开启新会话。 |
| `/resume` | 恢复历史会话。 |
| `/exit` | 退出 TUI。 |

## 配置

Go 后端不再读取或创建配置文件，运行参数统一通过 CLI flags 传入：

| Flag | 默认值 | 说明 |
| --- | --- | --- |
| `--port` | `8080` | HTTP 监听端口。 |
| `--db` | `~/.agenty/agenty.db` | SQLite 数据库文件；`~` 会展开为当前用户主目录。 |
| `--debug` | 关闭 | 启用 debug 日志和 Gin debug 模式。 |
| `--version`、`-v` | 关闭 | 输出版本信息后退出。 |

例如：

```bash
agenty --port 9090 --db /srv/agenty/agenty.db --debug
```

`agenty-cli` 在本地模式下会把 `--db` 和 `--debug` 透传给内嵌后端。远程客户端设置保持独立，`--server` 和 `--client-config` 详见 `agenty-cli --help`。

## 数据库

SQLite 是当前启用的数据库后端。Agenty 默认把数据库保存在 `~/.agenty/agenty.db`，可通过 `--db` 指定其他文件；父目录会自动创建。

SQLite 启动需要 FTS5 和 sqlite-vector。release 二进制应包含 FTS5 支持。Agenty 会把 sqlite-vector 原生扩展放在所选数据库的同一目录；扩展不存在时会下载匹配当前平台的 release asset。

Windows `arm64` 当前无法运行 server，因为 sqlite-vector 没有该平台产物，同时 CLI 暂未暴露 PostgreSQL 选择能力。

PostgreSQL 实现和 schema 暂时保留在代码中，等待后续决策，但当前 server CLI 不提供 PostgreSQL 连接参数。未来若重新开放 PostgreSQL 部署，仍需先创建数据库：

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
