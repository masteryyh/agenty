# Agenty

[English](./README.md)

Agenty 是一个本地优先的 AI agent 应用。当前产品链路由 `agenty-cli`、
`agenty-core` 和自解压 launcher `agenty-bootstrap` 组成。CLI 仅通过子进程
stdin/stdout 上的逐行 JSON-RPC 2.0 与 core 通信，不再启动 HTTP server。

core 当前支持 provider/model/agent 管理、持久化会话、模型流式输出、agent 工具循环
以及内置文件工具。Skills、MCP、memory、会话压缩和远程客户端模式要等 core 提供对等
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
`~/.agenty/bin/{cli,core}`。CLI 启动 core 子进程并打开初始化向导；向导通过
已有的 `provider.*` 和 `agent.*` IPC methods 创建一个 provider、一个聊天 model 和一个默认 agent，
最后调用 `initialize.complete` 标记初始化完成。

## 运行模型

launcher 内含两个 XZ 压缩 payload 及其解压内容的 SHA3-256 摘要。已释放文件摘要一致时
直接复用；缺失或不一致时会重新解压、校验并原子替换。CLI 按以下顺序查找 core：

1. `AGENTY_CORE_BIN`
2. 仓库开发环境中的 `packages/agenty-core/bin/agenty-core`
3. launcher 释放的 `~/.agenty/bin/core`

core 从 stdin 逐行读取紧凑 JSON-RPC message，并把 response 和 notification 写到 stdout。
调用 `session.start` 后，core 会持续发送有序的 `session.event` 通知，覆盖 round 生命周期、
已持久化消息、模型流式增量、工具调用和 round 终态。通知可能早于 `session.start` response
到达，因此 client 必须先订阅事件再发送请求。stdin EOF 时 core 退出。

TUI 当前开放 `/provider`、`/model`、`/agents`、`/cwd`、`/think`、`/status`、
`/new`、`/resume`、`/help` 和 `/exit`。core 尚未实现的功能暂不展示。

## 配置与存储

core 默认把数据保存在 `~/.agenty`。可向 CLI 传入 `--data-dir <path>`，或为 core 设置
`AGENTY_DATA_DIR` 以切换数据根目录。主要文件如下：

| 数据 | 路径 |
| --- | --- |
| 配置 | `~/.agenty/config.json` |
| 会话 transcript | `~/.agenty/sessions/<yyyy>/<mm>/<dd>/<session-id>.jsonl` |
| 会话索引 | `~/.agenty/agenty.sqlite` |
| Providers 和 models | `~/.agenty/providers/` |
| Agents | `~/.agenty/agents/` |
| 日志 | `~/.agenty/logs/<yyyy>/<mm>/<dd>/core.log` |

`AGENTY_LOG_LEVEL` 接受 `debug`、`info`、`warn` 或 `error`；
`AGENTY_LOG_FORMAT` 接受 `text` 或 `jsonl`。

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
