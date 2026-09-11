# Agenty

[English](./README.md)

Agenty 是一个本地优先的 AI agent 应用。当前产品链路由 `agenty-cli`、
`agenty-core`、Rust `patch-applier` helper 和自解压 launcher `agenty-bootstrap` 组成。
CLI 仅通过子进程
stdin/stdout 上的逐行 JSON-RPC 2.0 与 core 通信，不再启动 HTTP server。

core 当前支持 provider/model 管理、持久化会话、模型流式输出、agent 工具循环、会话压缩、
内置文件工具、本地 Skills 和 MCP client 连接。memory 和远程客户端模式要等 core 提供对等
实现后再开放。

## 快速开始

从[最新 release](https://github.com/masteryyh/agenty/releases/latest)下载与你的系统和
架构匹配的压缩包，解压并安装 `agenty`：

```bash
chmod +x agenty
sudo install -m 755 agenty /usr/local/bin/agenty
agenty
```

首次运行时，launcher 会校验并释放内置的 CLI 和 core 到
`~/.agenty/bin/{cli,core,apply_patch}`。CLI 启动 core 子进程并打开初始化向导；向导通过
已有的 `provider.*` IPC methods 创建一个 provider 和一个聊天 model，最后调用
`initialize.complete` 保存全局默认会话模型。

## 运行模型

launcher 内含三个 XZ 压缩 payload 及其解压内容的 SHA3-256 摘要。已释放文件摘要一致时
直接复用；缺失或不一致时会重新解压、校验并原子替换。CLI 按以下顺序查找 core：

1. `AGENTY_CORE_BIN`
2. 仓库开发环境中的 `packages/agenty-core/bin/agenty-core`
3. launcher 释放的 `~/.agenty/bin/core`

CLI 启动 core 前会把 core 所在目录放到 `PATH` 首位，使 core 和 shell 工具调用可以找到
同目录中的 `apply_patch`。

core 从 stdin 逐行读取紧凑 JSON-RPC message，并把 response 和 notification 写到 stdout。
调用 `session.start` 后，core 会持续发送有序的 `session.event` 通知，覆盖 round 生命周期、
已持久化消息、模型流式增量、工具调用和 round 终态。通知可能早于 `session.start` response
到达，因此 client 必须先订阅事件再发送请求。stdin EOF 时 core 退出。

TUI 当前开放 `/provider`、`/model`、`/mcp`、`/cwd`、`/effort`、`/status`、
`/new`、`/resume`、`/help` 和 `/exit`。在输入框中输入 `$` 可以搜索并插入已安装的
Skill 结构化引用。core 会依次扫描数据目录下的 `skills/`、`~/.agents/skills` 和
`~/.claude/skills`；可以通过 `AGENTY_DATA_DIR` 更改第一个目录。

## 配置与存储

core 默认把数据保存在 `~/.agenty`。可向 CLI 传入 `--data-dir <path>`，或为 core 设置
`AGENTY_DATA_DIR` 以切换数据根目录。主要文件如下：

| 数据 | 路径 |
| --- | --- |
| 配置 | `~/.agenty/config.json` |
| Skills | `~/.agenty/skills/`，然后是 `~/.agents/skills/` 和 `~/.claude/skills/` |
| 会话 transcript | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` |
| 会话索引 | `~/.agenty/agenty.sqlite` |
| Providers 和 models | 内置 catalog 固化在 core 二进制中；自定义 provider 使用 `~/.agenty/providers/<provider-code>.json`，内置 provider 文件仅保存 API key |
| MCP server 配置 | `~/.agenty/mcp/<server-name>.json`（每个 server 一个简洁 JSON 文件，权限 `0600`） |
| MCP OAuth 凭据 | `~/.agenty/mcp-auth/<server-name>.json`（权限 `0600`，与配置分开保存） |
| 模型发现缓存 | 仅保存在运行中的 core 进程内，有效期 8 小时；按需刷新，core 重启后不会保留 |
| Patch 事务锁 | `~/.agenty/locks/` |
| 日志 | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` |

`AGENTY_LOG_LEVEL` 接受 `debug`、`info`、`warn` 或 `error`；
`AGENTY_LOG_FORMAT` 接受 `text` 或 `jsonl`。

`/mcp` 可管理 stdio、Streamable HTTP 和 legacy SSE server。server 文件包含 `type`、
`enabled`，以及按 transport 选择的 `command`/`args`/`env` 或 `url`/`headers` 字段，
其中 `args` 是每个进程参数一个元素的 JSON 字符串数组。不保存工作目录；环境变量和
header 值可以引用环境变量；TUI 中的 headers 位于默认收起的 `Advanced Options` 下。
Streamable HTTP 使用 Go SDK 提供的 OAuth authorization-code 流程，自动进行动态 client
registration 和 loopback 浏览器登录；`/mcp` 会打开授权 URL 并显示连接状态。SSE 继续兼容已有
server，但会在 TUI 中标记为 deprecated。连接在后台以有限并发启动；每个 round 开始时会快照
当时已连接的工具，较晚完成连接的 server 从下一轮开始可用。

## 开发

```bash
pnpm install
pnpm cli:dev
```

`pnpm cli:dev` 会先构建 `agenty-core`，再把源码 TUI 直接连接到当前终端。常用定向命令：

```bash
pnpm core:test
pnpm core:test:integration
pnpm core:test:e2e
pnpm cli:typecheck
pnpm bootstrap:test
pnpm build
```

release 版本来自根目录导出的 `AGENTY_VERSION`。把 `.env.example` 复制为被忽略的 `.env`，
加载环境变量后运行 `pnpm build`，即可构建完整 launcher。

## 许可证

本项目使用 Apache License 2.0，详见 [LICENSE](./LICENSE)。

Copyright (c) 2026 masteryyh

## 会话调试工具

[Agenty Inspector](./packages/agenty-inspector/README.md) 是独立的只读网页调试工具。
执行 `pnpm inspector:dev` 后访问 `http://127.0.0.1:5173`；也可执行
`pnpm inspector:build` 与 `pnpm inspector:start`，使用默认端口 4318 的独立程序。
它沿用 `AGENTY_DATA_DIR`（默认 `~/.agenty`），展示消息、原始事件、工具关联和诊断问题，
无需启动 core。
