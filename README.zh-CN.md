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

启动交互式 TUI：

```bash
agenty
```

首次运行时，launcher 会校验并将内置的 CLI 与 runtime 释放到 `~/.agenty/bin`，随后启动 CLI，由 CLI 在随机本地端口 fork runtime。runtime 会初始化数据库、写入预置供应商和模型、创建默认 agent；CLI 检测到未初始化状态后会打开设置向导，用于配置供应商 API key、web search 和默认模型。

如果只想单独运行 HTTP 后端，可在首次启动后执行 `~/.agenty/bin/runtime`，其默认端口为 `8080`、SQLite 位于 `~/.agenty/agenty.db`、关闭 debug 日志。

## 运行模式

Agenty 以单一自解压产物发布：`agenty` launcher，一个小型 Rust 二进制，自身代码之后追加了 XZ 压缩的 `agenty-cli`（React OpenTUI 终端界面与资源管理子命令）和 `agenty` Go runtime（HTTP 后端），并内置两个 payload 解压内容的 SHA3-256 摘要。

启动时 launcher 会检查 `~/.agenty/bin/cli` 和 `~/.agenty/bin/runtime`：SHA3-256 与内置摘要一致的文件直接复用；缺失或不一致的文件会重新解压、校验并原子替换。之后 launcher 启动 CLI，由 CLI 在随机本地端口 fork runtime。仅在开发阶段明确需要 CLI 使用非托管 runtime 时设置 `AGENTY_BIN`。

| 模式 | 命令 | 使用场景 |
| --- | --- | --- |
| 本地交互模式 | `agenty` | 运行 TUI；launcher 校验或释放 `~/.agenty/bin/{cli,runtime}` 并启动 CLI，CLI 再在随机本地端口启动 runtime。 |
| 本地交互模式（开发） | `pnpm cli:dev` | 通过 Turborepo 构建 `agenty-runtime`，再从源码运行 TUI 并直接使用仓库内的 runtime。 |
| 指定数据库的本地交互模式 | `agenty --db /path/to/agenty.db` | 为本地 server 指定 SQLite 数据库。 |
| Server 模式 | `~/.agenty/bin/runtime` | 使用默认设置运行释放出的 HTTP 后端服务。 |
| 远程交互模式 | `agenty --server http://host:8080` | 将 TUI 连接到远程 `agenty` server，而不是启动本地 server。 |
| 资源管理 CLI | `agenty <子命令>` | 初始化并管理 agents、providers、models、settings、MCP servers 和全局 skills。详见 `agenty --help`。 |

首次运行时，`agenty-cli` 会检测到未初始化状态，并打开设置向导用于配置供应商 API key、web search 和默认模型。

TUI 中常用的 slash commands：

TUI 支持鼠标滚轮、列表与操作项点击，以及鼠标选择文本；终端支持 OSC52 时，选中的文本会自动复制到剪贴板。

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

CLI 在本地模式下会把 `--db` 和 `--debug` 透传给本地后端。远程客户端设置保持独立，`--server` 和 `--client-config` 详见 `agenty --help`。

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
